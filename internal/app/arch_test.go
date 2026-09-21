package app_test

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// The point of the autark runtime: serving needs no hostus. A stray import here
// would reintroduce the dependency without anyone noticing, so it is a test and
// not a comment.
func TestServePathDoesNotImportTheHostusAdapter(t *testing.T) {
	pkg, err := build.Import("github.com/jobrunner/situs/internal/app", "", 0)
	if err != nil {
		t.Fatalf("importing internal/app: %v", err)
	}
	// Without this the test would pass vacuously if build.Import ever stopped
	// reporting imports — an assertion over an empty list proves nothing.
	if len(pkg.Imports) == 0 {
		t.Fatal("build.Import reported no imports for internal/app — the assertion below would be vacuous")
	}
	for _, imp := range pkg.Imports {
		if strings.Contains(imp, "internal/adapters/hostus") {
			t.Errorf("internal/app imports %q — the serve path must stay free of hostus", imp)
		}
	}
}

// serveSideSQLiteEntryPoints are the sqlite adapter's identifiers the
// composition root may reach for. It is an allowlist on purpose: a
// write-capable entry point has to be added here deliberately, and adding it is
// the moment someone has to justify why serving needs to write.
var serveSideSQLiteEntryPoints = map[string]bool{
	"OpenReadOnly": true,
}

// Serving opens the index read-only, and that is not a style preference. A
// read-write handle would create an index that is not there (green health,
// empty answers for everything), would put the file into WAL mode and so make
// even a pure reader need the -wal/-shm sidecars, and would require a writable
// directory. All three break replacing the index underneath a running
// container.
//
// The rule cannot be expressed as an import ban — internal/app has to import
// the sqlite adapter — so it is checked on the use.
func TestServePathUsesOnlyReadOnlySQLiteEntryPoints(t *testing.T) {
	// GoFiles excludes _test.go by construction: the rule is about the shipped
	// composition root, not about what a test may set up.
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("reading internal/app: %v", err)
	}
	if len(pkg.GoFiles) == 0 {
		t.Fatal("build.ImportDir reported no Go files for internal/app")
	}

	fset := token.NewFileSet()
	used := map[string]bool{}
	for _, name := range pkg.GoFiles {
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "sqlite" {
				used[sel.Sel.Name] = true
			}
			return true
		})
	}

	// The composition root does open the index, so an empty result means the
	// walk found nothing rather than that everything is fine.
	if len(used) == 0 {
		t.Fatal("no sqlite.* use found in internal/app — the assertion below would be vacuous")
	}

	forbidden := []string{}
	for name := range used {
		if !serveSideSQLiteEntryPoints[name] {
			forbidden = append(forbidden, name)
		}
	}
	sort.Strings(forbidden)
	for _, name := range forbidden {
		t.Errorf("internal/app uses sqlite.%s — the serve path may only use %v", name, sortedKeys(serveSideSQLiteEntryPoints))
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
