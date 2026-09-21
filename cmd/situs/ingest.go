package main

import (
	"context"
	"encoding/json"
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
)

// hostusDistributionPause is the gap between two hostus concept requests
// during ingest. hostus rate-limits at 20 req/s and answers 429 above that;
// the adapter deliberately has no pacing of its own, because how fast to call
// is the caller's decision, and ingest is the only caller. Measured against the
// real service: 0.07s works, and the index's 3135 concepts take about four
// minutes — an ingest run is offline maintenance, not latency-critical.
const hostusDistributionPause = 70 * time.Millisecond

// factsheetSource names the artifact behind every ingested description, so a
// reader can tell which factsheet version stands behind the text. Pinned in
// pipelines/floraveg-factsheets/build.sh; bump both together.
const factsheetSource = "floraveg:eunis-habitat-factsheets:2021-06-01"

// annex1DescriptionSource names what the Annex I descriptions were written
// FROM: the official Interpretation Manual, the EUNIS crosswalk of this index
// and its own species data. situs wrote the wording; these are its sources.
const annex1DescriptionSource = "situs:derived-from:eur28+eunis@2021"

// pacedDistributionSource, its Areas/FailedConcepts methods and
// maxLoggedConceptFailures moved to paced_distribution.go — a cohesive unit
// in its own right, and it kept this file's per-file complexity sum over
// the codecharta ratchet's cap. hostusDistributionPause stays here: it is
// runIngest's wiring decision (how fast THIS command paces the type it
// constructs), not a property of the type itself.

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
	Syntaxa            application.SyntaxaReport
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

// ingestSyntaxaPhase runs the syntaxa ingest and logs its non-fatal
// warnings, so runIngest itself gains exactly one branch instead of one per
// warning kind.
//
// Runs after IngestCSV (the habitat types must be in the index for the edge
// check) and before the localization steps. Unlike before, the hierarchy is
// not an optional addition: without it the index ends up with no syntaxa
// hierarchy at all, so the ingest fails instead of shipping it.
func ingestSyntaxaPhase(ctx context.Context, db *sqlite.DB, csvDir string) (application.SyntaxaReport, error) {
	rep, err := application.IngestSyntaxa(ctx, db, csvDir)
	if err != nil {
		return application.SyntaxaReport{}, fmt.Errorf("ingesting syntaxa from %q: %w", csvDir, err)
	}
	if len(rep.AltCodeCollisions) > 0 {
		slog.WarnContext(ctx, "syntaxa ingest found alt-code collisions", "codes", rep.AltCodeCollisions)
	}
	if len(rep.AmbiguousMatches) > 0 {
		slog.WarnContext(ctx, "syntaxa ingest found ambiguous name matches", "ids", rep.AmbiguousMatches)
	}
	if len(rep.UnknownLinkTargets) > 0 {
		slog.WarnContext(ctx, "syntaxa ingest dropped edges to unknown syntaxa", "targets", rep.UnknownLinkTargets)
	}
	if rep.ParentsDerived > 0 {
		slog.WarnContext(ctx, "syntaxa ingest derived parents from sibling consensus", "count", rep.ParentsDerived)
	}
	return rep, nil
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

	// Runs right after IngestCSV puts the habitat types in the index (the
	// edge check needs them) and before species/localization/derivation,
	// which do not depend on it and which it does not depend on.
	syntaxa, err := ingestSyntaxaPhase(ctx, db, csvDir)
	if err != nil {
		return err
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
		Syntaxa:            syntaxa,
		AreaNames:          overlays.areas,
		Descriptions:       overlays.descriptions,
		Traits:             traitReport,
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
