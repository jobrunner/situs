// This file is in-package on purpose: closeIndex is unexported, and the
// shutdown contract it guards — the index is always closed, and a close failure
// is never swallowed — is only reachable from inside.
package app

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
	"github.com/jobrunner/situs/internal/config"
)

func newShutdownFixture(closeIndex func() error) *App {
	cfg := &config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	return &App{
		Config:     cfg,
		Logger:     slog.New(slog.DiscardHandler),
		HTTPServer: httpapi.NewServer(cfg.Server.Addr(), httpapi.Deps{}, slog.New(slog.DiscardHandler), httpapi.Options{}),
		closeIndex: closeIndex,
	}
}

// A failing index close must surface even though the HTTP server stopped
// cleanly: a half-closed index is exactly what an operator needs to hear about.
func TestShutdownReportsAFailingIndexClose(t *testing.T) {
	wantErr := errors.New("disk went away")
	situs := newShutdownFixture(func() error { return wantErr })

	err := situs.Shutdown(context.Background())

	if !errors.Is(err, wantErr) {
		t.Errorf("Shutdown() = %v, want it to wrap %v", err, wantErr)
	}
}

// The index is closed on every shutdown, not only when the HTTP server failed.
func TestShutdownAlwaysClosesTheIndex(t *testing.T) {
	closed := false
	situs := newShutdownFixture(func() error { closed = true; return nil })

	if err := situs.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() = %v, want no error", err)
	}
	if !closed {
		t.Error("closeIndex was not called — the index would leak its file handle")
	}
}

// A nil closeIndex is the pre-New state; shutting down must not panic on it.
func TestShutdownToleratesAnUnwiredIndex(t *testing.T) {
	situs := newShutdownFixture(nil)

	if err := situs.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown() = %v, want no error", err)
	}
}
