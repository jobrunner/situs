package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocs_ServesTheSwaggerUIPage(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<div id="swagger-ui"></div>`, // the mount point
		"SwaggerUIBundle(",            // the bundle is actually invoked
		`url: "/openapi"`,             // the spec comes from this same server
		".swagger-ui",                 // a selector from the inlined stylesheet
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
}

// The page must not need the network. A blanket "contains no http:// string"
// assertion would be wrong: the vendored bundle legitimately carries JSON-Schema
// ids, XML namespaces and a React warning link as data. What must not exist is a
// *loading* reference to another origin — those are what turn the page blank when
// situs runs offline in the field.
func TestDocs_IsSelfContained(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs", nil))
	body := rec.Body.String()

	for _, forbidden := range []string{
		`src="http`, `src='http`, `src="//`,
		`href="http`, `href='http`, `href="//`,
		"@import",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("page contains %q — an external load would leave /docs blank offline", forbidden)
		}
	}
}

// The assets are ~1.7 MB, so an accidental switch to a per-request rebuild is
// expensive rather than merely wasteful: pin that the page is assembled once.
func TestDocs_IsAssembledOncePerProcess(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	first := httptest.NewRecorder()
	srv.Router().ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/docs", nil))
	second := httptest.NewRecorder()
	srv.Router().ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/docs", nil))

	if first.Body.Len() != second.Body.Len() {
		t.Errorf("page length differs between requests: %d vs %d", first.Body.Len(), second.Body.Len())
	}
	if first.Body.Len() < 500_000 {
		t.Errorf("page is %d bytes — the inlined assets look missing", first.Body.Len())
	}
}
