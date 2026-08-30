package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func writeHierarchyCSV(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "syntaxa_hierarchy.csv")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing syntaxa_hierarchy.csv: %v", err)
	}
	return path
}

func TestIngestSyntaxaHierarchy_WritesClassAndOrderRows(t *testing.T) {
	repo := newFakeRepo()
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir,
		"code,rank,name,author,parent_code\n"+
			"AA,class,Salicetea purpureae,Moor 1958,\n"+
			"AA01,order,Salicetalia purpureae,Moor 1958,AA\n")

	rep, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestSyntaxaHierarchy: %v", err)
	}
	if rep.ClassesWritten != 1 || rep.OrdersWritten != 1 {
		t.Errorf("ClassesWritten/OrdersWritten = %d/%d, want 1/1", rep.ClassesWritten, rep.OrdersWritten)
	}
	if len(repo.syntaxa) != 2 {
		t.Fatalf("syntaxa = %+v, want 2 rows written", repo.syntaxa)
	}
}

func TestIngestSyntaxaHierarchy_MatchesLongestAlliancePrefixAndSetsAuthorParent(t *testing.T) {
	repo := newFakeRepo()
	// An EUNIS alliance already in the index, as ingestSyntaxa would have left
	// it: full historical combi-string name, no author, no parent.
	repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{
		ID: "CAK-01C", Rank: "alliance", Name: "Cakilion edentulae Br.-Bl. 1931",
	})
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir,
		"code,rank,name,author,parent_code\n"+
			"AA01A,alliance,Cakilion edentulae,Br.-Bl. 1931,AA01\n")

	rep, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestSyntaxaHierarchy: %v", err)
	}
	if rep.AlliancesMatched != 1 || rep.AlliancesUnmatched != 0 {
		t.Errorf("AlliancesMatched/Unmatched = %d/%d, want 1/0", rep.AlliancesMatched, rep.AlliancesUnmatched)
	}
	if len(repo.authorUpdates) != 1 {
		t.Fatalf("authorUpdates = %+v, want exactly one", repo.authorUpdates)
	}
	got := repo.authorUpdates[0]
	if got.id != "CAK-01C" || got.author != "Br.-Bl. 1931" || got.parentID != "AA01" {
		t.Errorf("authorUpdate = %+v, want {CAK-01C, Br.-Bl. 1931, AA01}", got)
	}
}

func TestIngestSyntaxaHierarchy_UnmatchedAllianceIsCountedNotAborted(t *testing.T) {
	repo := newFakeRepo()
	repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{
		ID: "XYZ-01", Rank: "alliance", Name: "Nomatchion nowhereii",
	})
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir,
		"code,rank,name,author,parent_code\n"+
			"AA01A,alliance,Cakilion edentulae,Br.-Bl. 1931,AA01\n")

	rep, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestSyntaxaHierarchy: %v", err)
	}
	if rep.AlliancesMatched != 0 || rep.AlliancesUnmatched != 1 {
		t.Errorf("AlliancesMatched/Unmatched = %d/%d, want 0/1", rep.AlliancesMatched, rep.AlliancesUnmatched)
	}
	if len(repo.authorUpdates) != 0 {
		t.Errorf("authorUpdates = %+v, want none for an unmatched alliance", repo.authorUpdates)
	}
}

// The two FloraVeg candidate rows below share the exact same name string
// "Salicion albae" (length 14) — both are genuine prefixes of the EUNIS name
// "Salicion albae Soó 1930", and since they are literally the same string
// they necessarily tie at the same longest length. This is the case the
// brief flagged as needing verification: the original draft used
// "Salicion albae" vs "Salicion alberti", but "Salicion alberti" is not
// actually a prefix of "Salicion albae Soó 1930" at all (they diverge at
// "alb[a|e]"), so that pairing never reached the tie branch. Two identical
// candidate names is the reliable way to force a genuine ambiguous tie.
func TestIngestSyntaxaHierarchy_AmbiguousEqualLengthPrefixesAreNeverGuessed(t *testing.T) {
	repo := newFakeRepo()
	repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{
		ID: "AMB-01", Rank: "alliance", Name: "Salicion albae Soó 1930",
	})
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir,
		"code,rank,name,author,parent_code\n"+
			"AA01A,alliance,Salicion albae,Soó 1930,AA01\n"+
			"AA02A,alliance,Salicion albae,Nowak 1960,AA02\n")

	rep, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestSyntaxaHierarchy: %v", err)
	}
	if len(rep.AmbiguousMatches) != 1 || rep.AmbiguousMatches[0] != "AMB-01" {
		t.Errorf("AmbiguousMatches = %+v, want [AMB-01]", rep.AmbiguousMatches)
	}
	if len(repo.authorUpdates) != 0 {
		t.Errorf("authorUpdates = %+v, want none for an ambiguous match", repo.authorUpdates)
	}
}

func TestIngestSyntaxaHierarchy_MissingCSVIsNotAnError(t *testing.T) {
	repo := newFakeRepo()
	path := filepath.Join(t.TempDir(), "syntaxa_hierarchy.csv") // never written

	rep, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestSyntaxaHierarchy: %v, want no error for a missing file", err)
	}
	if rep.ClassesWritten != 0 || rep.OrdersWritten != 0 || rep.AlliancesMatched != 0 ||
		rep.AlliancesUnmatched != 0 || len(rep.AmbiguousMatches) != 0 {
		t.Errorf("report = %+v, want the zero report", rep)
	}
}

func TestIngestSyntaxaHierarchy_BeginErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.beginErr = context.DeadlineExceeded
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir, "code,rank,name,author,parent_code\nAA,class,X,,\n")

	if _, err := IngestSyntaxaHierarchy(context.Background(), repo, path); err == nil {
		t.Fatal("IngestSyntaxaHierarchy = nil error, want the Begin error surfaced")
	}
}

func TestIngestSyntaxaHierarchy_AllSyntaxaErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.allSyntaxaErr = context.DeadlineExceeded
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir, "code,rank,name,author,parent_code\nAA,class,X,,\n")

	if _, err := IngestSyntaxaHierarchy(context.Background(), repo, path); err == nil {
		t.Fatal("IngestSyntaxaHierarchy = nil error, want the AllSyntaxa error surfaced")
	}
}
