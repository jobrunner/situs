package app_test

import (
	"go/build"
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
