package app_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jobrunner/situs/internal/app"
	"github.com/jobrunner/situs/internal/config"
)

func TestNewWiresAServerThatServesTheOperationsSurface(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	situs, err := app.New(context.Background(), cfg, logger, "1.2.3")
	if err != nil {
		t.Fatalf("New() = %v, want no error", err)
	}
	if situs.Tracer == nil {
		t.Error("Tracer is nil — the composition root must always inject one (NoOp when disabled)")
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
