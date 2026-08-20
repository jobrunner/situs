package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
)

func serve(t *testing.T, srv *httpapi.Server, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestInfoReportsServiceNameAndVersion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	srv := httpapi.NewServer(":0", testDeps(stubHealth{ready: true}), logger, httpapi.Options{Version: "1.2.3"})

	rec := serve(t, srv, http.MethodGet, "/v1/info")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Service string `json:"service"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %q: %v", rec.Body, err)
	}
	if got.Service != "situs" {
		t.Errorf("service = %q, want %q", got.Service, "situs")
	}
	if got.Version != "1.2.3" {
		t.Errorf("version = %q, want the injected build version", got.Version)
	}
}

// The point of the index block: a client checks the backbone before it sends
// concept ids that could never match.
func TestInfoDescribesTheIndex(t *testing.T) {
	rec := serve(t, newTestServer(t, seededQueryService()), http.MethodGet, "/v1/info")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Index struct {
			ConceptBackbones   []string `json:"concept_backbones"`
			SpeciesWithConcept int      `json:"species_with_concept"`
			AreaScheme         string   `json:"area_scheme"`
			AreasWithData      int      `json:"areas_with_data"`
		} `json:"index"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %q: %v", rec.Body, err)
	}
	if len(got.Index.ConceptBackbones) != 1 || got.Index.ConceptBackbones[0] != "wcvp" {
		t.Errorf("concept_backbones = %v, want [wcvp]", got.Index.ConceptBackbones)
	}
	if got.Index.SpeciesWithConcept != 2 || got.Index.AreasWithData != 3 {
		t.Errorf("index = %+v, want the measured figures passed through", got.Index)
	}
	if got.Index.AreaScheme != "wgsrpd_l3" {
		t.Errorf("area_scheme = %q, want wgsrpd_l3", got.Index.AreaScheme)
	}
}

// A failing index must not be answered with a half-truth: no index block at all
// beats one silently filled with zeros.
func TestInfoFailsLoudlyWhenTheIndexCannotBeDescribed(t *testing.T) {
	q := seededQueryService()
	q.indexInfoErr = errors.New("index unreadable")

	rec := serve(t, newTestServer(t, q), http.MethodGet, "/v1/info")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Errorf("body = %q, want the INTERNAL_ERROR envelope", rec.Body)
	}
}

func TestLivenessIsIndependentOfReadiness(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	srv := httpapi.NewServer(":0", testDeps(stubHealth{ready: false}), logger, httpapi.Options{})

	if rec := serve(t, srv, http.MethodGet, "/health/live"); rec.Code != http.StatusOK {
		t.Errorf("GET /health/live status = %d, want 200 even while not ready", rec.Code)
	}
}

func TestReadinessFollowsTheHealthPort(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ready bool
		want  int
	}{
		{name: "ready", ready: true, want: http.StatusOK},
		{name: "not ready", ready: false, want: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
			srv := httpapi.NewServer(":0", testDeps(stubHealth{ready: tc.ready}), logger, httpapi.Options{})

			if rec := serve(t, srv, http.MethodGet, "/health/ready"); rec.Code != tc.want {
				t.Errorf("GET /health/ready status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestOpenAPIEndpointServesTheEmbeddedSpec(t *testing.T) {
	rec := serve(t, newTestServer(t, seededQueryService()), http.MethodGet, "/openapi")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("Content-Type = %q, want application/yaml", ct)
	}
	if !strings.Contains(rec.Body.String(), "openapi: 3.0.3") {
		t.Errorf("body = %q, want the embedded specification", rec.Body)
	}
}

func TestMetricsEndpointServesPrometheusText(t *testing.T) {
	rec := serve(t, newTestServer(t, seededQueryService()), http.MethodGet, "/metrics")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Errorf("body = %q, want the Go collector's metrics", rec.Body)
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	if rec := serve(t, newTestServer(t, seededQueryService()), http.MethodGet, "/v1/nope"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestWrongMethodIsMethodNotAllowed(t *testing.T) {
	if rec := serve(t, newTestServer(t, seededQueryService()), http.MethodPost, "/v1/info"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestShutdownStopsAServerThatWasNeverStarted(t *testing.T) {
	if err := newTestServer(t, seededQueryService()).Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown() = %v, want no error", err)
	}
}
