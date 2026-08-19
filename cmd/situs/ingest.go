package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/jobrunner/situs/internal/adapters/sqlite"
	"github.com/jobrunner/situs/internal/application"
	"github.com/jobrunner/situs/internal/config"
)

func newIngestCmd() *cobra.Command {
	var csvDir, dbPath string

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Typologien, Habitattypen, Crosswalks und Syntaxa aus CSVs laden",
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

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
