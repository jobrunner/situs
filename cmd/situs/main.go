// Command situs serves the local, read-only EUNIS habitat-type index.
//
// main stays thin: it parses flags, loads the config, builds the logger and
// hands over to the composition root in internal/app.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/jobrunner/situs/internal/adapters/telemetry"
	"github.com/jobrunner/situs/internal/config"
)

// Build information, injected via -ldflags.
var (
	Version   = "dev"
	BuildTime = "unknown"
)

var configFile string

func main() {
	if err := newRootCmd().Execute(); err != nil {
		// cobra already printed the error.
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "situs",
		Short:         "Lokaler, schreibgeschützter Dienst für EUNIS-Habitattypen",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.PersistentFlags().StringVar(&configFile, "config", "", "path to a config file (env wins over file)")
	root.AddCommand(newServeCmd(), newVersionCmd(), newIngestCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Version und Build-Zeitpunkt ausgeben",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "situs %s (built %s)\n", Version, BuildTime)
			return err
		},
	}
}

func setupLogger(cfg config.LoggingConfig, w io.Writer) *slog.Logger {
	return slog.New(telemetry.NewSpanContextHandler(buildHandler(cfg, w)))
}

// installLogger builds the service logger and also makes it slog's default.
// Both are needed: components take the returned logger by injection, while
// adapters that log incidentally (the hostus client's batch downshift, the
// ingest's skipped-row warnings) reach for the package-level slog. Without the
// default those records would bypass SITUS_LOG_* entirely.
func installLogger(cfg config.LoggingConfig, w io.Writer) *slog.Logger {
	logger := setupLogger(cfg, w)
	slog.SetDefault(logger)
	return logger
}

func buildHandler(cfg config.LoggingConfig, w io.Writer) slog.Handler {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}
	if cfg.Format == "text" {
		return slog.NewTextHandler(w, opts)
	}
	return slog.NewJSONHandler(w, opts)
}

func parseLevel(level string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		return slog.LevelInfo
	}
	return l
}
