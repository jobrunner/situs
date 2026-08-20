package main

import (
	"context"
	"encoding/json"
	"fmt"
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

// pacedDistributionSource wraps a DistributionSource that has no pacing of
// its own (Areas issues one hostus request per concept) and spaces those
// requests out, one concept at a time, so a full ingest run does not fail in
// a wall of 429s.
type pacedDistributionSource struct {
	src   output.DistributionSource
	pause time.Duration
}

func (p pacedDistributionSource) Areas(ctx context.Context, conceptIDs []string) (map[string][]domain.Area, error) {
	out := map[string][]domain.Area{}
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
			return nil, err
		}
		for k, v := range areas {
			out[k] = v
		}
	}
	return out, nil
}

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
	distSrc := pacedDistributionSource{src: resolver, pause: hostusDistributionPause}
	distributionReport, err := application.IngestDistribution(ctx, db, distSrc)
	if err != nil {
		return fmt.Errorf("ingesting species distribution: %w", err)
	}

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
