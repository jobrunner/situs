package httpapi

// These tests need access to unexported state: the configured http.Server and
// the recovery middleware, which no mounted route can trigger from outside.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// The config key server.read_timeout must reach the listener; a configured
// timeout that never leaves the config struct is a knob that lies.
func TestReadTimeoutFromOptionsReachesTheHTTPServer(t *testing.T) {
	s := NewServer(":0", Deps{Health: stubReady{}}, quietLogger(), Options{ReadTimeout: 7 * time.Second})

	if s.server.ReadTimeout != 7*time.Second {
		t.Errorf("http.Server.ReadTimeout = %v, want the configured 7s", s.server.ReadTimeout)
	}
	if s.server.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("http.Server.ReadHeaderTimeout = %v, want 10s", s.server.ReadHeaderTimeout)
	}
}

// The error envelope is a cross-cutting invariant: every handler in every later
// task returns this shape, and the recovery middleware is its first producer.
func TestRecoveryMiddlewareAnswersWithTheErrorEnvelope(t *testing.T) {
	s := NewServer(":0", Deps{Health: stubReady{}}, quietLogger(), Options{})

	router := s.Router()
	router.HandleFunc("/v1/panic", func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}).Methods(http.MethodGet)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/panic", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 — the panic must be recovered, not propagated", rec.Code)
	}
	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %q: %v", rec.Body, err)
	}
	if got.Error.Code != CodeInternalError {
		t.Errorf("error.code = %q, want %q", got.Error.Code, CodeInternalError)
	}
	if got.Error.Message == "" {
		t.Error("error.message is empty — the envelope always carries a message")
	}
}

type stubReady struct{}

func (stubReady) Ready(context.Context) bool { return true }
