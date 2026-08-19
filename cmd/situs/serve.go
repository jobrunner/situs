package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jobrunner/situs/internal/app"
	"github.com/jobrunner/situs/internal/config"
)

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "HTTP-Dienst starten",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context())
		},
	}
}

func runServe(ctx context.Context) error {
	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}
	logger := setupLogger(cfg.Logging, os.Stdout)

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	situs, err := app.New(ctx, cfg, logger, Version)
	if err != nil {
		return err
	}

	serverErr := make(chan error, 1)
	go func() { serverErr <- situs.Start(ctx) }()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "error", err)
			return err
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()
	return situs.Shutdown(shutdownCtx)
}
