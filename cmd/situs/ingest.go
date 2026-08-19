package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jobrunner/situs/internal/adapters/hostus"
	"github.com/jobrunner/situs/internal/adapters/sqlite"
	"github.com/jobrunner/situs/internal/application"
	"github.com/jobrunner/situs/internal/config"
)

func newIngestCmd() *cobra.Command {
	var csvDir, dbPath string

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Typologien, Habitattypen, Crosswalks, Syntaxa und Artenrollen aus CSVs laden",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if csvDir == "" || dbPath == "" {
				return fmt.Errorf("both --csv-dir and --db are required")
			}
			return runIngest(cmd, csvDir, dbPath)
		},
	}
	cmd.Flags().StringVar(&csvDir, "csv-dir", "", "directory holding the pipeline CSVs (required)")
	cmd.Flags().StringVar(&dbPath, "db", "", "path to the sqlite index file (required)")
	return cmd
}

// ingestOutput bundles both reports plus the measured hostus resolution rate
// (spec open point 3) into the one JSON object the command prints.
type ingestOutput struct {
	application.IngestReport
	Species        application.SpeciesReport
	ResolutionRate float64
	Localizations  int
	DerivedLabels  int
}

func runIngest(cmd *cobra.Command, csvDir, dbPath string) error {
	ctx := cmd.Context()

	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}
	// A dropped row's only record is this log stream — route it through the
	// configured logger (SITUS_LOG_*), not slog's unconfigured default.
	slog.SetDefault(setupLogger(cfg.Logging, os.Stdout))

	db, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("opening sqlite index %q: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	report, err := application.IngestCSV(ctx, db, csvDir)
	if err != nil {
		return fmt.Errorf("ingesting %q: %w", csvDir, err)
	}

	resolver := hostus.NewClient(cfg.Hostus.BaseURL, &http.Client{Timeout: cfg.Hostus.Timeout}, cfg.Hostus.BatchSize)
	speciesReport, err := application.IngestSpeciesRoles(ctx, db, resolver, filepath.Join(csvDir, "species_roles.csv"))
	if err != nil {
		return fmt.Errorf("ingesting species roles from %q: %w", csvDir, err)
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
		Localizations:  localizations,
		DerivedLabels:  derivedLabels,
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
