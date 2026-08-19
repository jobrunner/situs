package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log/slog"
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
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	return httpapi.NewServer(":0", httpapi.Deps{
		Health: stubHealth{ready: true},
		Query:  query,
		Names:  seededNameQueryService(),
	}, logger, httpapi.Options{})
}

// testDeps keeps the health-probe tests focused on the probe while still
// wiring a complete set of ports.
func testDeps(health input.HealthChecker) httpapi.Deps {
	return httpapi.Deps{Health: health, Query: seededQueryService(), Names: seededNameQueryService()}
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
