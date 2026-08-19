// Package app is the composition root: the only package that may import every
// layer. It builds the adapters, injects them into the ports and owns the
// lifecycle.
package app

import (
	"context"
	"log/slog"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
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
	Tracer     output.Tracer
	Health     *application.HealthService
	HTTPServer *httpapi.Server
}

// New builds the application bottom-up: driven adapters first, then the use
// cases, then the driving HTTP adapter.
func New(_ context.Context, cfg *config.Config, logger *slog.Logger, version string) (*App, error) {
	a := &App{
		Config: cfg,
		Logger: logger,
		Tracer: output.NoOpTracer{},
	}

	// The index does not exist yet (Task 3), so readiness is a constant for now.
	a.Health = application.NewHealthService(true)

	a.HTTPServer = httpapi.NewServer(cfg.Server.Addr(), a.Health, logger, httpapi.Options{
		ServiceName: "situs",
		Version:     version,
		ReadTimeout: cfg.Server.ReadTimeout,
	})
	return a, nil
}

// Start serves HTTP until Shutdown is called.
func (a *App) Start(_ context.Context) error {
	a.Logger.Info("serving", "addr", a.Config.Server.Addr())
	return a.HTTPServer.Start()
}

// Shutdown stops every component that owns resources.
func (a *App) Shutdown(ctx context.Context) error {
	return a.HTTPServer.Shutdown(ctx)
}
