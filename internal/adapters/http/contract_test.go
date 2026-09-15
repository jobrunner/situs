package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
	"github.com/jobrunner/situs/internal/ports/input"
)

// TestRoutesMatchOpenAPISpec is the routes<->spec fitness function: it walks
// every registered route and every documented path+method and fails if either
// side has an entry the other lacks. A route added or renamed in code but not in
// the spec (or the reverse) fails the build instead of rotting.
func TestRoutesMatchOpenAPISpec(t *testing.T) {
	routes := routerSurface(t)
	spec := specSurface(t, filepath.Join(repoRoot(t), "internal", "adapters", "http", "openapi.yaml"))

	for op := range routes {
		if !spec[op] {
			t.Errorf("route %q is mounted but MISSING from openapi.yaml — document it (or the spec drifted)", op)
		}
	}
	for op := range spec {
		if !routes[op] {
			t.Errorf("openapi.yaml documents %q but no such route is mounted", op)
		}
	}
}

// TestOpenAPICopiesAreIdentical guards the two-copy invariant: the embedded spec
// and the copy external tooling reads must not diverge by a single byte.
func TestOpenAPICopiesAreIdentical(t *testing.T) {
	root := repoRoot(t)
	embedded := readFile(t, filepath.Join(root, "internal", "adapters", "http", "openapi.yaml"))
	mirrored := readFile(t, filepath.Join(root, "api", "openapi", "openapi.yaml"))

	if !bytes.Equal(embedded, mirrored) {
		t.Error("internal/adapters/http/openapi.yaml and api/openapi/openapi.yaml differ — " +
			"mirror them with: cp internal/adapters/http/openapi.yaml api/openapi/openapi.yaml")
	}
}

// The servers entry drives Swagger-UI's "Try it out" requests. An absolute URL
// with a hardcoded port aims past the port the service actually listens on as
// soon as the two differ — which is the normal case, since the port is
// configurable. A relative URL resolves against the origin that served the page
// and therefore always hits the right server.
func TestOpenAPIServerURLIsRelative(t *testing.T) {
	spec := string(readFile(t, filepath.Join(repoRoot(t), "internal", "adapters", "http", "openapi.yaml")))

	if !strings.Contains(spec, "- url: /") {
		t.Error(`the servers entry is not "- url: /" — a relative URL is what keeps "Try it out" working on any port`)
	}
	for _, hardcoded := range []string{"url: http://localhost", "url: https://localhost", "url: http://127.0.0.1"} {
		if strings.Contains(spec, hardcoded) {
			t.Errorf("the spec pins a server URL (%q); it would point past a service on any other port", hardcoded)
		}
	}
}

// routerSurface returns the set of "METHOD /path" the router mounts.
func routerSurface(t *testing.T) map[string]bool {
	t.Helper()
	srv := newTestServer(t, seededQueryService())

	got := map[string]bool{}
	err := srv.Router().Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		// A route without a path template is not part of the URL contract.
		if tmpl, tmplErr := route.GetPathTemplate(); tmplErr == nil {
			methods, methodsErr := route.GetMethods()
			if methodsErr != nil {
				// A path-bearing route without .Methods() matches every verb and
				// would escape the contract silently. That is a failure, not a skip.
				t.Errorf("route %q has no explicit .Methods() — it matches all verbs and escapes the OpenAPI contract", tmpl)
			}
			for _, m := range methods {
				got[strings.ToUpper(m)+" "+tmpl] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the router: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("the router walk yielded no routes")
	}
	return got
}

var (
	reTopLevelKey = regexp.MustCompile(`^[A-Za-z]`)
	rePathKey     = regexp.MustCompile(`^  (/\S*):\s*$`)
	reMethodKey   = regexp.MustCompile(`^    (get|post|put|patch|delete|head|options):\s*$`)
)

// specSurface returns the set of "METHOD /path" the spec documents.
// Deliberately a small line scanner instead of a YAML dependency: the file's
// shape is regular (2-space path keys under `paths:`, 4-space method keys), and
// the contract test must not pull a new import into the module for it.
func specSurface(t *testing.T, specPath string) map[string]bool {
	t.Helper()
	f, err := os.Open(filepath.Clean(specPath))
	if err != nil {
		t.Fatalf("opening %s: %v", specPath, err)
	}
	defer func() { _ = f.Close() }()

	want := map[string]bool{}
	inPaths := false
	curPath := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "paths:":
			inPaths = true
			curPath = ""
		case inPaths && reTopLevelKey.MatchString(line):
			// A new top-level key (components:, tags:, ...) ends the paths block.
			inPaths = false
		case inPaths:
			if m := rePathKey.FindStringSubmatch(line); m != nil {
				curPath = m[1]
			} else if m := reMethodKey.FindStringSubmatch(line); m != nil && curPath != "" {
				want[strings.ToUpper(m[1])+" "+curPath] = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanning %s: %v", specPath, err)
	}
	if len(want) == 0 {
		t.Fatalf("%s documents no path — the scanner found nothing to compare", specPath)
	}
	return want
}

// newTestServer builds a server with a stub health checker — enough to register
// every route.
func newTestServer(t *testing.T, query input.QueryService) *httpapi.Server {
	t.Helper()
	return newTestServerWithOptions(t, query, httpapi.Options{})
}

func newTestServerWithOptions(t *testing.T, query input.QueryService, opts httpapi.Options) *httpapi.Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	return httpapi.NewServer(":0", httpapi.Deps{
		Health: stubHealth{ready: true},
		Query:  query,
	}, logger, opts)
}

// TestCORSDisabledByDefaultIsByteIdentical pins the decided behavior: without
// any allowed origin configured, the service must answer exactly as it does
// today — no CORS headers at all, even for a cross-origin request.
func TestCORSDisabledByDefaultIsByteIdentical(t *testing.T) {
	srv := newTestServerWithOptions(t, seededQueryService(), httpapi.Options{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	req.Header.Set("Origin", "https://app.example.test")
	srv.Handler().ServeHTTP(rec, req)

	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	for _, h := range []string{
		"Access-Control-Allow-Origin", "Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers", "Access-Control-Max-Age", "Vary",
	} {
		if v := res.Header.Get(h); v != "" {
			t.Errorf("header %s = %q, want unset when CORS is not configured", h, v)
		}
	}
}

// TestCORSPreflightForEveryWritingRoute is the gate for the trap described in
// cors.go: it walks the route table for every non-GET operation and fires the
// preflight a browser would send.
//
// It catches both ways CORS silently breaks — a middleware registered with
// router.Use (mux bypasses it for the unmatched OPTIONS, so no headers come
// back) and an Access-Control-Allow-Methods answer that has drifted from the
// routes. Neither shows up in a unit test, in curl, or in a same-origin
// frontend; the usual discovery path is an integrator's bug report.
//
// Driving Handler() rather than Router() is the whole point: the bare router
// has no CORS layer, so the same test against Router() would prove nothing.
func TestCORSPreflightForEveryWritingRoute(t *testing.T) {
	const origin = "https://app.example.test"
	srv := newTestServerWithOptions(t, seededQueryService(), httpapi.Options{
		CORSAllowedOrigins: []string{origin},
	})

	ops := writingRouteOps(t, srv)
	if len(ops) == 0 {
		t.Skip("no writing routes registered yet — nothing to preflight")
	}

	for _, o := range ops {
		t.Run(o.method+" "+o.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, o.path, nil)
			req.Header.Set("Origin", origin)
			req.Header.Set("Access-Control-Request-Method", o.method)
			req.Header.Set("Access-Control-Request-Headers", "content-type")

			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			res := rec.Result()
			defer func() { _ = res.Body.Close() }()

			if res.StatusCode != http.StatusNoContent {
				t.Errorf("preflight status = %d, want 204 — is CORS registered with "+
					"router.Use instead of wrapping the router?", res.StatusCode)
			}
			if got := res.Header.Get("Access-Control-Allow-Origin"); got != origin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, origin)
			}
			if allow := res.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(allow, o.method) {
				t.Errorf("Access-Control-Allow-Methods = %q, missing %q — a browser "+
					"will block this endpoint", allow, o.method)
			}
		})
	}
}

// writingRouteOp is one non-GET/HEAD route the preflight test exercises.
type writingRouteOp struct{ method, path string }

// writingRouteOps derives the preflight test cases from the route table, so an
// endpoint added tomorrow is covered without touching the test. Path-var
// routes are skipped: httptest.NewRequest needs a concrete path, and every
// writing route in situs today is a fixed path anyway.
func writingRouteOps(t *testing.T, srv *httpapi.Server) []writingRouteOp {
	t.Helper()
	var ops []writingRouteOp
	err := srv.Router().Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tmpl, tErr := route.GetPathTemplate()
		if tErr == nil && !strings.Contains(tmpl, "{") {
			if methods, mErr := route.GetMethods(); mErr == nil {
				for _, m := range methods {
					// GET/HEAD are "simple requests" — no preflight, nothing to pin.
					if m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions {
						ops = append(ops, writingRouteOp{m, tmpl})
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk router: %v", err)
	}
	return ops
}

// A bare OPTIONS (no Origin, no Access-Control-Request-Method) is not a
// preflight and must still reach the router — otherwise enabling CORS quietly
// turns every OPTIONS into a 204.
func TestBareOptionsIsNotSwallowed(t *testing.T) {
	srv := newTestServerWithOptions(t, seededQueryService(), httpapi.Options{
		CORSAllowedOrigins: []string{"https://app.example.test"},
	})

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/v1/species/habitat-types", nil))

	if rec.Code == http.StatusNoContent {
		t.Error("bare OPTIONS answered 204 by the CORS layer; it must fall through to the router")
	}
}

// TestCORSRejectsUnlistedOrigin proves an origin outside the allow-list gets no
// Access-Control-Allow-Origin — the browser then refuses the response, even
// though the preflight itself still answers 204 (see corsHandler's comment on
// why the status is uniform).
func TestCORSRejectsUnlistedOrigin(t *testing.T) {
	srv := newTestServerWithOptions(t, seededQueryService(), httpapi.Options{
		CORSAllowedOrigins: []string{"https://allowed.example.test"},
	})

	req := httptest.NewRequest(http.MethodOptions, "/v1/species/habitat-types", nil)
	req.Header.Set("Origin", "https://not-allowed.example.test")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q for an unlisted origin, want unset", got)
	}
}

// TestCORSWildcardDoesNotLeakAcrossSchemeOrPort pins the decided rule: scheme
// and port must match exactly, even under a subdomain wildcard.
func TestCORSWildcardDoesNotLeakAcrossSchemeOrPort(t *testing.T) {
	srv := newTestServerWithOptions(t, seededQueryService(), httpapi.Options{
		CORSAllowedOrigins: []string{"https://*.fieldworksdiary.app"},
	})

	for _, origin := range []string{
		"http://app.fieldworksdiary.app",       // wrong scheme
		"https://app.fieldworksdiary.app:8443", // unlisted port
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("origin %q: Access-Control-Allow-Origin = %q, want unset", origin, got)
		}
	}
}

// TestCORSIgnoresAnUnparsableAllowedOrigin pins the "fails loud, not silent"
// choice at initCORS: a malformed entry is dropped with a logged warning
// rather than crashing the process or ending up matching everything.
func TestCORSIgnoresAnUnparsableAllowedOrigin(t *testing.T) {
	srv := newTestServerWithOptions(t, seededQueryService(), httpapi.Options{
		CORSAllowedOrigins: []string{"not-a-valid-origin", "https://allowed.example.test"},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/info", nil)
	req.Header.Set("Origin", "https://allowed.example.test")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://allowed.example.test" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the still-valid entry to remain usable", got)
	}
}

// testDeps keeps the health-probe tests focused on the probe while still
// wiring a complete set of ports.
func testDeps(health input.HealthChecker) httpapi.Deps {
	return httpapi.Deps{Health: health, Query: seededQueryService()}
}

type stubHealth struct{ ready bool }

func (s stubHealth) Ready(context.Context) bool { return s.ready }

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return data
}

// repoRoot walks up from the test's working directory until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting the working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found above the working directory")
		}
		dir = parent
	}
}
