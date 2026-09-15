package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestExplorerRoute_ServesHTMLAtRoot(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Error("body does not look like an HTML document")
	}
}

// The page must work in the field, offline. A single remote asset would
// leave it blank exactly when it is needed.
func TestExplorerPage_LoadsNoRemoteAssets(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	remote := regexp.MustCompile(`(?i)(src|href)\s*=\s*["']https?://`)
	if loc := remote.FindString(rec.Body.String()); loc != "" {
		t.Errorf("page references a remote asset (%q); it must be fully self-contained", loc)
	}
}

// / must not swallow unknown paths — a typo'd API path has to stay a 404.
func TestExplorerRoute_DoesNotActAsCatchAll(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/nonexistent", nil))

	if rec.Code == http.StatusOK {
		t.Errorf("status = 200 for an unknown path; / is acting as a catch-all")
	}
}

// The area filter is a choice, not a guess: the page fills it from /v1/areas
// so one picks a code the index can actually answer, instead of typing one
// that comes back as INVALID_QUERY.
func TestExplorerPage_FillsTheAreaFilterFromTheAreasRoute(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rec.Body.String()
	if !strings.Contains(body, `fetch("/v1/areas")`) {
		t.Error("page does not load /v1/areas; the area filter would stay a free-text guess")
	}
	if !strings.Contains(body, `<select id="g-area"`) {
		t.Error(`the area filter is not a <select id="g-area">`)
	}
}
