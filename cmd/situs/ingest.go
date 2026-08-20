package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/jobrunner/situs/internal/adapters/hostus"
	"github.com/jobrunner/situs/internal/adapters/sqlite"
	"github.com/jobrunner/situs/internal/application"
	"github.com/jobrunner/situs/internal/config"
	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// hostusDistributionPause is the gap between two hostus concept requests
// during ingest. hostus rate-limits at 20 req/s and answers 429 above that;
// the adapter deliberately has no pacing of its own (it is also used from the
// serve path, where pacing does not belong), so the ingest path enforces it.
// Measured against the real service: 0.07s works, and ~3600 concepts take
// about 3 minutes — an ingest run is offline maintenance, not latency-critical.
const hostusDistributionPause = 70 * time.Millisecond

// maxLoggedConceptFailures caps how many individual per-concept failures get
// their own log line. Beyond that, the run-end aggregate line (which always
// fires once len(failed) > 0) says how many there were — a real outage on
// this call must not put ~3600 nearly identical lines in the log.
const maxLoggedConceptFailures = 3

// pacedDistributionSource wraps a DistributionSource that has no pacing of
// its own (Areas issues one hostus request per concept) and spaces those
// requests out, one concept at a time, so a full ingest run does not fail in
// a wall of 429s.
//
// It also tolerates individual concept requests failing instead of
// discarding the whole batch: a timeout on concept 3400 of 3600 must not
// throw away three minutes of work and leave the index unfiltered.
// FailedConcepts reports how many of the last Areas call's requests were
// tolerated this way — it is a property of this decorator, not of
// IngestDistribution, so the composition root (runIngest) reads it directly
// after the call instead of routing it through the DistributionSource port.
// A canceled/expired context is the one failure that is not tolerated —
// that is the run being told to stop, not a data problem, and it must fail
// here, not resurface as an unrelated error two ingest steps later. If every
// single request fails, Areas reports that as a whole-batch failure (nil
// map, error) so IngestDistribution treats it exactly like the previous
// all-or-nothing behavior: zeros in the report, plus the warning — and
// FailedConcepts resets to 0 for that call, since the count only means
// something for a call that otherwise returned a usable partial result.
type pacedDistributionSource struct {
	src    output.DistributionSource
	pause  time.Duration
	failed int
}

func (p *pacedDistributionSource) Areas(ctx context.Context, conceptIDs []string) (map[string][]domain.Area, error) {
	out := map[string][]domain.Area{}
	p.failed = 0
	var lastErr error
	for i, id := range conceptIDs {
		if i > 0 {
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(p.pause):
			}
		}
		areas, err := p.src.Areas(ctx, []string{id})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return out, err
			}
			p.failed++
			lastErr = err
			if p.failed <= maxLoggedConceptFailures {
				slog.WarnContext(ctx, "distribution request for one concept failed, continuing with the rest",
					"concept_id", id, "error", err)
			}
			continue
		}
		for k, v := range areas {
			out[k] = v
		}
	}
	if p.failed > 0 {
		slog.WarnContext(ctx, "some distribution requests failed, the index will be partially filtered",
			"failed", p.failed, "requested", len(conceptIDs))
	}
	if len(conceptIDs) > 0 && p.failed == len(conceptIDs) {
		err := fmt.Errorf("all %d distribution requests failed, last error: %w", p.failed, lastErr)
		p.failed = 0
		return nil, err
	}
	return out, nil
}

// FailedConcepts reports how many concept requests the last Areas call
// tolerated instead of aborting on. 0 both when nothing failed and when
// everything failed (see the type doc comment) — it answers "how many were
// skipped in an otherwise-successful run", not "was there any failure".
func (p *pacedDistributionSource) FailedConcepts() int { return p.failed }

func newIngestCmd() *cobra.Command {
	var csvDir, dbPath string

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Typologien, Habitattypen, Crosswalks, Syntaxa und Artenrollen aus CSVs laden",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if csvDir == "" {
				return fmt.Errorf("--csv-dir is required")
			}
			cfg, err := config.Load(configFile)
			if err != nil {
				return err
			}
			// serve reads only index.path. Defaulting --db to it keeps ingest and
			// serve pointed at the same file, so an operator cannot silently fill
			// one index while serving another empty one.
			if dbPath == "" {
				dbPath = cfg.Index.Path
			}
			if dbPath == "" {
				return fmt.Errorf("no index path: pass --db or set index.path (SITUS_INDEX_PATH)")
			}
			return runIngest(cmd, cfg, csvDir, dbPath)
		},
	}
	cmd.Flags().StringVar(&csvDir, "csv-dir", "", "directory holding the pipeline CSVs (required)")
	cmd.Flags().StringVar(&dbPath, "db", "",
		"path to the sqlite index file (default: index.path / SITUS_INDEX_PATH)")
	return cmd
}

// ingestOutput bundles both reports plus the measured hostus resolution rate
// (spec open point 3) into the one JSON object the command prints.
type ingestOutput struct {
	application.IngestReport
	Species        application.SpeciesReport
	ResolutionRate float64
	Distribution   application.DistributionReport
	Localizations  int
	DerivedLabels  int
}

func runIngest(cmd *cobra.Command, cfg *config.Config, csvDir, dbPath string) error {
	ctx := cmd.Context()

	// A dropped row's only record is this log stream — route it through the
	// configured logger (SITUS_LOG_*), not slog's unconfigured default.
	installLogger(cfg.Logging, os.Stdout)

	db, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("opening sqlite index %q: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	report, err := application.IngestCSV(ctx, db, csvDir)
	if err != nil {
		return fmt.Errorf("ingesting %q: %w", csvDir, err)
	}

	resolver := hostus.NewClient(cfg.Hostus.BaseURL, &http.Client{Timeout: cfg.Hostus.Timeout}, cfg.Hostus.BatchSize, cfg.Hostus.EntryBackbone)
	speciesReport, err := application.IngestSpeciesRoles(ctx, db, resolver, filepath.Join(csvDir, "species_roles.csv"))
	if err != nil {
		return fmt.Errorf("ingesting species roles from %q: %w", csvDir, err)
	}

	// Runs after IngestSpeciesRoles (it needs the indexed concept ids) and
	// before the localization/derivation steps, which do not depend on it.
	distSrc := &pacedDistributionSource{src: resolver, pause: hostusDistributionPause}
	distributionReport, err := application.IngestDistribution(ctx, db, distSrc)
	if err != nil {
		return fmt.Errorf("ingesting species distribution: %w", err)
	}
	// FailedConcepts is a fact about the paced decorator, not something
	// IngestDistribution can know without a type assertion to a concrete
	// dependency — so the composition root, which built distSrc and knows
	// its type, fills the report field in here.
	distributionReport.Failed = distSrc.FailedConcepts()

	localizations, err := application.IngestLocalizations(ctx, db, filepath.Join(csvDir, "localizations.csv"))
	if err != nil {
		return fmt.Errorf("ingesting localizations from %q: %w", csvDir, err)
	}

	// Derivation runs last: it depends on both the crosswalks (ingested
	// above) and the official Annex I labels (just ingested) being present.
	derivedLabels, err := application.DeriveGermanLabels(ctx, db)
	if err != nil {
		return fmt.Errorf("deriving German labels: %w", err)
	}

	out := ingestOutput{
		IngestReport:   report,
		Species:        speciesReport,
		ResolutionRate: speciesReport.ResolutionRate(),
		Distribution:   distributionReport,
		Localizations:  localizations,
		DerivedLabels:  derivedLabels,
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
