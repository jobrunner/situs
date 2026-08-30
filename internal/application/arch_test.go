package application_test

import (
	"go/build"
	"strings"
	"testing"
)

// Species-role ingest resolves against a local crosswalk file now, not
// hostus — this test holds that internal/application never reaches back for
// the hostus adapter, not even for the resolver types kept around as dead
// code (see the plan's deliberate-deviation note).
func TestApplicationDoesNotImportTheHostusAdapter(t *testing.T) {
	pkg, err := build.Import("github.com/jobrunner/situs/internal/application", "", 0)
	if err != nil {
		t.Fatalf("importing internal/application: %v", err)
	}
	// Without this the test would pass vacuously if build.Import ever stopped
	// reporting imports — an assertion over an empty list proves nothing.
	if len(pkg.Imports) == 0 {
		t.Fatal("build.Import reported no imports for internal/application — the assertion below would be vacuous")
	}
	for _, imp := range pkg.Imports {
		if strings.Contains(imp, "internal/adapters/hostus") {
			t.Errorf("internal/application imports %q — species-role ingest must stay free of hostus", imp)
		}
	}
}
