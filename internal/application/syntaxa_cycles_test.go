package application

import (
	"slices"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// checkCycles is exercised directly, not only through IngestSyntaxa: every
// cycle is necessarily also a rank violation (a rank-correct chain can only
// run formation <- class <- order <- alliance and therefore terminates), so
// an end-to-end test would fail on checkParentRanks' error even with this
// check switched off. The two are separate diagnoses of one broken file —
// which rows sit in the loop, and which edge takes the wrong step — and this
// test pins the first of them on its own.
func TestCheckCyclesMeldetNurDieZeilenImKreis(t *testing.T) {
	rows := []hierarchyRow{
		{code: "CA", rank: domain.SyntaxonRankClass, parentCode: ""},
		{code: "CA01", rank: domain.SyntaxonRankOrder, parentCode: "CA01A"},
		{code: "CA01A", rank: domain.SyntaxonRankAlliance, parentCode: "CA01"},
	}
	written := map[string]bool{"CA": true, "CA01": true, "CA01A": true}
	rep := &SyntaxaReport{}

	checkCycles(rows, written, rep)

	slices.Sort(rep.Orphans)
	if !slices.Equal(rep.Orphans, []string{"CA01", "CA01A"}) {
		t.Errorf("Orphans = %v, erwartet [CA01 CA01A]", rep.Orphans)
	}
}

// A class is never part of a cycle: writeHierarchy derives its parent from
// its own code and validates it against the known formations before writing
// the row at all. Pinning that exclusion keeps the class branch from being
// the one that happens to catch a cycle.
func TestCheckCyclesLaesstKlassenAus(t *testing.T) {
	rows := []hierarchyRow{{code: "CA", rank: domain.SyntaxonRankClass, parentCode: "CA"}}
	rep := &SyntaxaReport{}

	checkCycles(rows, map[string]bool{"CA": true}, rep)

	if len(rep.Orphans) != 0 {
		t.Errorf("Orphans = %v, erwartet leer", rep.Orphans)
	}
}
