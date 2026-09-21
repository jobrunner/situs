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
// the adapter deliberately has no pacing of its own, because how fast to call
// is the caller's decision, and ingest is the only caller. Measured against the
// real service: 0.07s works, and the index's 3135 concepts take about four
// minutes — an ingest run is offline maintenance, not latency-critical.
const hostusDistributionPause = 70 * time.Millisecond

// maxLoggedConceptFailures caps how many individual per-concept failures get
// their own log line. Beyond that, the run-end aggregate line (which always
// fires once len(failed) > 0) says how many there were — a real outage on
// this call must not put thousands of nearly identical lines in the log.
const maxLoggedConceptFailures = 3

// factsheetSource names the artifact behind every ingested description, so a
// reader can tell which factsheet version stands behind the text. Pinned in
// pipelines/floraveg-factsheets/build.sh; bump both together.
const factsheetSource = "floraveg:eunis-habitat-factsheets:2021-06-01"

// annex1DescriptionSource names what the Annex I descriptions were written
// FROM: the official Interpretation Manual, the EUNIS crosswalk of this index
// and its own species data. situs wrote the wording; these are its sources.
const annex1DescriptionSource = "situs:derived-from:eur28+eunis@2021"

// pacedDistributionSource wraps a DistributionSource that has no pacing of
// its own (Areas issues one hostus request per concept) and spaces those
// requests out, one concept at a time, so a full ingest run does not fail in
// a wall of 429s.
//
// It also tolerates individual concept requests failing instead of
// discarding the whole batch: a timeout on the last few hundred concepts must
// not throw away minutes of work and leave the index unfiltered.
// FailedConcepts reports how many of the last Areas call's requests were
// tolerated this way.
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
	var csvDir, dbPath, crosswalkPath, aggregateMembersPath string

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
			if crosswalkPath == "" {
				crosswalkPath = filepath.Join(csvDir, "eurosl_crosswalk.csv")
			}
			if aggregateMembersPath == "" {
				aggregateMembersPath = filepath.Join(csvDir, "aggregate_members.csv")
			}
			return runIngest(cmd, cfg, csvDir, dbPath, crosswalkPath, aggregateMembersPath)
		},
	}
	cmd.Flags().StringVar(&csvDir, "csv-dir", "", "directory holding the pipeline CSVs (required)")
	cmd.Flags().StringVar(&dbPath, "db", "",
		"path to the sqlite index file (default: index.path / SITUS_INDEX_PATH)")
	cmd.Flags().StringVar(&crosswalkPath, "crosswalk", "",
		"path to eurosl_crosswalk.csv (default: <csv-dir>/eurosl_crosswalk.csv)")
	cmd.Flags().StringVar(&aggregateMembersPath, "aggregate-members", "",
		"path to aggregate_members.csv (default: <csv-dir>/aggregate_members.csv)")
	return cmd
}

// ingestOutput bundles both reports plus the measured hostus resolution rate
// (spec open point 3) into the one JSON object the command prints.
//
// DistributionFailed lives here rather than in application.DistributionReport
// because it is a property of pacedDistributionSource, which lives here too:
// how many per-concept requests were tolerated is not something
// IngestDistribution can know through the DistributionSource port.
type ingestOutput struct {
	application.IngestReport
	Species            application.SpeciesReport
	ResolutionRate     float64
	Distribution       application.DistributionReport
	DistributionFailed int
	Localizations      int
	DerivedLabels      int
	SyntaxaHierarchy   application.SyntaxaHierarchyReport
	AreaNames          application.AreaReport
	Descriptions       application.DescriptionReport
	Traits             application.TraitReport
}

// localOverlays bundles the two ingest steps that read nothing but a local
// CSV and ask no service at all.
type localOverlays struct {
	areas        application.AreaReport
	descriptions application.DescriptionReport
}

// ingestLocalOverlays writes the area names and the habitat descriptions.
// Area names depend on nothing and nothing depends on them: they are a pure
// overlay on the area codes the distribution step writes later. Descriptions
// must run after IngestCSV, because every row is checked against the habitat
// type it belongs to, and those have to be in the index first.
func ingestLocalOverlays(ctx context.Context, db *sqlite.DB, csvDir string) (localOverlays, error) {
	var out localOverlays

	areaCSV := filepath.Join(csvDir, "wgsrpd_areas.csv")
	areas, err := application.IngestAreas(ctx, db, areaCSV)
	if err != nil {
		return localOverlays{}, fmt.Errorf("ingesting area names from %q: %w", areaCSV, err)
	}
	out.areas = areas

	// Two description files, two kinds of text. The EUNIS factsheets are their
	// authors' own wording; the Annex I descriptions are written by situs from
	// the Interpretation Manual, the EUNIS crosswalk and the index's species
	// data. Which file a row came from is the only thing that decides its
	// provenance — never anything about the row itself.
	for _, src := range []struct{ file, source, provenance string }{
		{"habitat_descriptions.csv", factsheetSource, domain.DescriptionProvenanceOfficial},
		{"annex1_descriptions.csv", annex1DescriptionSource, domain.DescriptionProvenanceSitus},
	} {
		path := filepath.Join(csvDir, src.file)
		report, err := application.IngestDescriptions(ctx, db, path, src.source, src.provenance)
		if err != nil {
			return localOverlays{}, fmt.Errorf("ingesting habitat descriptions from %q: %w", path, err)
		}
		out.descriptions.Written += report.Written
		out.descriptions.SkippedRows += report.SkippedRows
		out.descriptions.SkippedUnknownCode += report.SkippedUnknownCode
	}

	return out, nil
}

// ingestLocalizationFiles reads both localization files through the same code
// path and sums their rows: localizations.csv carries the labels (produced by
// pipelines/eurlex), localizations_descriptions.csv the German factsheet
// descriptions (hand-curated in data/). Keeping them apart keeps eurlex'
// strict merge from having to know about descriptions; both are optional.
func ingestLocalizationFiles(ctx context.Context, db *sqlite.DB, csvDir string) (int, error) {
	total := 0
	for _, name := range []string{"localizations.csv", "localizations_descriptions.csv"} {
		path := filepath.Join(csvDir, name)
		n, err := application.IngestLocalizations(ctx, db, path)
		if err != nil {
			return 0, fmt.Errorf("ingesting localizations from %q: %w", path, err)
		}
		total += n
	}
	return total, nil
}

// sealIndex runs the two steps that belong after the last write.
//
// The first is a measurement: hostus.entry_backbone is configurable, the prefix
// the batch route accepts is compiled in. Point the first at another backbone
// and the batch route stops answering anything — worth a warning at the one
// moment the mismatch is created.
//
// The second leaves the index as a single file. Serving opens it read-only
// (sqlite.OpenReadOnly), and a read-only handle on an index still in WAL mode
// would need the -shm sidecar — which takes a writable directory and turns
// replacing the index underneath a running container into a multi-file
// operation. The ingest is the only place that can do it: leaving WAL requires
// being the sole connection to the database.
func sealIndex(ctx context.Context, db *sqlite.DB, dbPath string) error {
	info, err := application.NewQueryService(db).IndexInfo(ctx)
	if err != nil {
		return fmt.Errorf("measuring the finished index: %w", err)
	}
	application.WarnOnForeignBackbones(ctx, info.ConceptBackbones)

	if err := db.FinalizeForServing(ctx); err != nil {
		return fmt.Errorf("finalizing sqlite index %q for serving: %w", dbPath, err)
	}
	return nil
}

func runIngest(cmd *cobra.Command, cfg *config.Config, csvDir, dbPath, crosswalkPath, aggregateMembersPath string) error {
	ctx := cmd.Context()

	// A dropped row's only record is this log stream — route it through the
	// configured logger (SITUS_LOG_*), not slog's unconfigured default.
	installLogger(cfg.Logging, os.Stdout)

	db, err := sqlite.OpenForIngest(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("opening sqlite index %q: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	// Ingest-only: a repinned index predating species_role.provenance/
	// derived_from needs the column added before anything writes to it.
	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migrating sqlite index %q: %w", dbPath, err)
	}

	report, err := application.IngestCSV(ctx, db, csvDir)
	if err != nil {
		return fmt.Errorf("ingesting %q: %w", csvDir, err)
	}

	// Runs right after the EUNIS alliances are indexed (IngestCSV, above) and
	// before species/localization/derivation, which do not depend on it and
	// which it does not depend on.
	hierarchyCSV := filepath.Join(csvDir, "syntaxa_hierarchy.csv")
	hierarchyReport, err := application.IngestSyntaxaHierarchy(ctx, db, hierarchyCSV)
	if err != nil {
		return fmt.Errorf("ingesting syntaxa hierarchy from %q: %w", hierarchyCSV, err)
	}

	overlays, err := ingestLocalOverlays(ctx, db, csvDir)
	if err != nil {
		return err
	}

	speciesReport, err := application.IngestSpeciesRoles(ctx, db,
		filepath.Join(csvDir, "species_roles.csv"), crosswalkPath, aggregateMembersPath)
	if err != nil {
		return fmt.Errorf("ingesting species roles from %q: %w", csvDir, err)
	}

	// resolver stays needed for the distribution step below — Areas(), not
	// Resolve(); species-name resolution no longer calls hostus at all.
	resolver := hostus.NewClient(cfg.Hostus.BaseURL, &http.Client{Timeout: cfg.Hostus.Timeout}, cfg.Hostus.BatchSize, cfg.Hostus.EntryBackbone)

	// Runs after IngestSpeciesRoles (it needs the indexed concept ids) and
	// before the localization/derivation steps, which do not depend on it.
	distSrc := &pacedDistributionSource{src: resolver, pause: hostusDistributionPause}
	distributionReport, err := application.IngestDistribution(ctx, db, distSrc)
	if err != nil {
		return fmt.Errorf("ingesting species distribution: %w", err)
	}

	traitCSVPaths := map[string]string{
		"eive":       filepath.Join(csvDir, "eive_traits.csv"),
		"tichy2023":  filepath.Join(csvDir, "tichy_traits.csv"),
		"midolo2023": filepath.Join(csvDir, "midolo_traits.csv"),
	}
	traitReport, err := application.IngestTraits(ctx, db, resolver, traitCSVPaths)
	if err != nil {
		return fmt.Errorf("ingesting traits: %w", err)
	}

	localizations, err := ingestLocalizationFiles(ctx, db, csvDir)
	if err != nil {
		return err
	}

	// Derivation runs last: it depends on both the crosswalks (ingested
	// above) and the official Annex I labels (just ingested) being present.
	derivedLabels, err := application.DeriveGermanLabels(ctx, db)
	if err != nil {
		return fmt.Errorf("deriving German labels: %w", err)
	}

	if err := sealIndex(ctx, db, dbPath); err != nil {
		return err
	}

	out := ingestOutput{
		IngestReport:       report,
		Species:            speciesReport,
		ResolutionRate:     speciesReport.ResolutionRate(),
		Distribution:       distributionReport,
		DistributionFailed: distSrc.FailedConcepts(),
		Localizations:      localizations,
		DerivedLabels:      derivedLabels,
		SyntaxaHierarchy:   hierarchyReport,
		AreaNames:          overlays.areas,
		Descriptions:       overlays.descriptions,
		Traits:             traitReport,
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
