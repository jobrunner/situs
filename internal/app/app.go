// Package app is the composition root: the only package that may import every
// layer. It builds the adapters, injects them into the ports and owns the
// lifecycle.
package app

import (
	"context"
	"fmt"
	"log/slog"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
	"github.com/jobrunner/situs/internal/adapters/sqlite"
	"github.com/jobrunner/situs/internal/application"
	"github.com/jobrunner/situs/internal/config"
	"github.com/jobrunner/situs/internal/ports/output"
)

// App holds every wired component.
type App struct {
	Config *config.Config
	Logger *slog.Logger
	// Tracer is never nil; it is the NoOp implementation until the OTLP adapter
	// is wired.
	Tracer output.Tracer
	// Index is held as the port, not as *sqlite.DB: the concrete type embeds
	// *sql.DB, so holding it would leak the raw SQL surface past the port.
	Index      output.Repository
	Health     *application.HealthService
	HTTPServer *httpapi.Server

	closeIndex func() error
}

// New builds the application bottom-up: driven adapters first, then the use
// cases, then the driving HTTP adapter.
func New(ctx context.Context, cfg *config.Config, logger *slog.Logger, version string) (*App, error) {
	db, err := sqlite.Open(ctx, cfg.Index.Path)
	if err != nil {
		return nil, fmt.Errorf("opening the index %q: %w", cfg.Index.Path, err)
	}

	a := &App{
		Config:     cfg,
		Logger:     logger,
		Tracer:     output.NoOpTracer{},
		Index:      db,
		closeIndex: db.Close,
	}

	// The index was opened and its schema applied, so the read paths are usable.
	a.Health = application.NewHealthService(true)

	// No name resolver is wired here on purpose: serving is autark. hostus is an
	// ingest-time dependency only (see cmd/situs/ingest.go), and arch_test.go
	// keeps this package free of its adapter.
	a.HTTPServer = httpapi.NewServer(cfg.Server.Addr(), httpapi.Deps{
		Health: a.Health,
		Query:  application.NewQueryService(a.Index),
	}, logger, httpapi.Options{
		ServiceName: "situs",
		Version:     version,
		ReadTimeout: cfg.Server.ReadTimeout,
	})
	return a, nil
}

// Start serves HTTP until Shutdown is called.
func (a *App) Start(_ context.Context) error {
	a.Logger.Info("serving", "addr", a.Config.Server.Addr(), "index", a.Config.Index.Path)
	return a.HTTPServer.Start()
}

// Shutdown stops every component that owns resources.
func (a *App) Shutdown(ctx context.Context) error {
	err := a.HTTPServer.Shutdown(ctx)
	if a.closeIndex != nil {
		if closeErr := a.closeIndex(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing the index: %w", closeErr)
		}
	}
	return err
}
