package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jobrunner/situs/internal/app"
	"github.com/jobrunner/situs/internal/config"
)

func TestNewWiresAServerThatServesTheOperationsSurface(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 0},
		Index:  config.IndexConfig{Path: filepath.Join(t.TempDir(), "index.sqlite")},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	situs, err := app.New(context.Background(), cfg, logger, "1.2.3")
	if err != nil {
		t.Fatalf("New() = %v, want no error", err)
	}
	if situs.Tracer == nil {
		t.Error("Tracer is nil — the composition root must always inject one (NoOp when disabled)")
	}
	if situs.Index == nil {
		t.Error("Index is nil — the read API has nothing to answer from")
	}

	rec := httptest.NewRecorder()
	situs.HTTPServer.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /health/ready status = %d, want 200", rec.Code)
	}

	if err := situs.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown() = %v, want no error", err)
	}
}

// Start after Shutdown must not resurrect the server: it returns
// http.ErrServerClosed and binds nothing. Asserting it here also keeps Start
// itself under test without a test ever occupying a port.
func TestStartAfterShutdownDoesNotServeAgain(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 0},
		Index:  config.IndexConfig{Path: filepath.Join(t.TempDir(), "index.sqlite")},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	situs, err := app.New(context.Background(), cfg, logger, "1.2.3")
	if err != nil {
		t.Fatalf("New() = %v, want no error", err)
	}
	if err := situs.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() = %v, want no error", err)
	}
	if err := situs.Start(context.Background()); !errors.Is(err, http.ErrServerClosed) {
		t.Errorf("Start() after Shutdown = %v, want http.ErrServerClosed", err)
	}
}

// An index that cannot be opened must fail startup loudly instead of serving an
// empty read API.
func TestNewFailsWhenTheIndexCannotBeOpened(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 0},
		Index:  config.IndexConfig{Path: filepath.Join(t.TempDir(), "no-such-dir", "index.sqlite")},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	if _, err := app.New(context.Background(), cfg, logger, "1.2.3"); err == nil {
		t.Error("New() = nil error, want the unopenable index reported")
	}
}
