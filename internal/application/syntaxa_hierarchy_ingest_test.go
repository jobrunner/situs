package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// A stat error that is not os.IsNotExist (here: ENOTDIR, because a path
// component is a plain file, not a directory) must be returned, not silently
// treated as "no hierarchy file yet".
func TestIngestSyntaxaHierarchy_StatErrorOtherThanNotExistIsReturned(t *testing.T) {
	repo := newFakeRepo()
	dir := t.TempDir()
	notADir := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", notADir, err)
	}
	path := filepath.Join(notADir, "syntaxa_hierarchy.csv")

	if _, err := IngestSyntaxaHierarchy(context.Background(), repo, path); err == nil {
		t.Fatal("IngestSyntaxaHierarchy = nil error, want the stat error surfaced")
	}
}

// A CSV missing a required column fails readHierarchyRows, and that error
// must surface from IngestSyntaxaHierarchy unwrapped-but-reported, not be
// mistaken for the "no file" case.
func TestIngestSyntaxaHierarchy_MalformedCSVHeaderIsReturned(t *testing.T) {
	repo := newFakeRepo()
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir, "code,rank,name,parent_code\nAA,class,X,\n") // missing "author"

	if _, err := IngestSyntaxaHierarchy(context.Background(), repo, path); err == nil {
		t.Fatal("IngestSyntaxaHierarchy = nil error, want the missing-column error surfaced")
	}
}

// A row with a rank this file does not recognize is skipped, not fatal — the
// well-formed rows around it are still ingested.
func TestIngestSyntaxaHierarchy_SkipsRowsWithAnUnknownRank(t *testing.T) {
	repo := newFakeRepo()
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir,
		"code,rank,name,author,parent_code\n"+
			"AA,class,Salicetea purpureae,Moor 1958,\n"+
			"AAF,family,Salicaceae,,\n") // unknown rank, must be skipped not fatal

	rep, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestSyntaxaHierarchy: %v", err)
	}
	if rep.ClassesWritten != 1 {
		t.Errorf("ClassesWritten = %d, want 1 (the unknown-rank row must not be counted or abort)", rep.ClassesWritten)
	}
	if len(repo.syntaxa) != 1 {
		t.Errorf("syntaxa = %+v, want only the class row written", repo.syntaxa)
	}
}

// An existing syntaxon whose rank is not "alliance" (e.g. one of the class
// rows this very file just wrote) must be skipped by the matching pass, not
// fed into longestPrefixMatch.
func TestIngestSyntaxaHierarchy_MatchingPassSkipsNonAllianceExistingSyntaxa(t *testing.T) {
	repo := newFakeRepo()
	repo.syntaxa = append(repo.syntaxa,
		domain.Syntaxon{ID: "AA", Rank: "class", Name: "Cakilion edentulae"}, // not an alliance: must be skipped
		domain.Syntaxon{ID: "CAK-01C", Rank: "alliance", Name: "Cakilion edentulae Br.-Bl. 1931"},
	)
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir,
		"code,rank,name,author,parent_code\n"+
			"AA01A,alliance,Cakilion edentulae,Br.-Bl. 1931,AA01\n")

	rep, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestSyntaxaHierarchy: %v", err)
	}
	if rep.AlliancesMatched != 1 {
		t.Errorf("AlliancesMatched = %d, want 1 (only the alliance-ranked entry may match)", rep.AlliancesMatched)
	}
	for _, u := range repo.authorUpdates {
		if u.id == "AA" {
			t.Errorf("authorUpdates = %+v, want the class-ranked entry AA never touched", repo.authorUpdates)
		}
	}
}

// A malformed row (skipped > 0) must not abort the run — the warning is
// logged and the well-formed rows are still committed.
func TestIngestSyntaxaHierarchy_LogsAndContinuesOnSkippedRows(t *testing.T) {
	repo := newFakeRepo()
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir,
		"code,rank,name,author,parent_code\n"+
			"AA,class,Salicetea purpureae,Moor 1958,\n"+
			"AAF,family,Salicaceae,,\n") // skipped: unknown rank

	rep, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestSyntaxaHierarchy: %v", err)
	}
	if !repo.committed {
		t.Error("committed = false, want the transaction committed despite the skipped row")
	}
	if rep.ClassesWritten != 1 {
		t.Errorf("ClassesWritten = %d, want 1", rep.ClassesWritten)
	}
}

// ingestHierarchyRows failing (here: UpsertSyntaxon) must roll the
// transaction back and surface the original error, when the rollback itself
// succeeds.
func TestIngestSyntaxaHierarchy_UpsertSyntaxonErrorRollsBackAndReturnsTheError(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertSyntaxon"
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir, "code,rank,name,author,parent_code\nAA,class,X,,\n")

	if _, err := IngestSyntaxaHierarchy(context.Background(), repo, path); err == nil {
		t.Fatal("IngestSyntaxaHierarchy = nil error, want the UpsertSyntaxon error surfaced")
	}
	if !repo.rolledBack {
		t.Error("rolledBack = false, want the transaction rolled back")
	}
	if repo.committed {
		t.Error("committed = true, want the failed ingest never committed")
	}
}

// When the rollback itself also fails, both errors must be reported (the
// rollback failure wrapped alongside the original cause), not silently
// dropped.
func TestIngestSyntaxaHierarchy_RollbackFailureIsReportedAlongsideTheOriginalError(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertSyntaxon"
	repo.rollbackErr = fmt.Errorf("connection lost")
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir, "code,rank,name,author,parent_code\nAA,class,X,,\n")

	_, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err == nil {
		t.Fatal("IngestSyntaxaHierarchy = nil error, want the combined error surfaced")
	}
	if !strings.Contains(err.Error(), "rollback also failed") {
		t.Errorf("error = %q, want it to mention the rollback failure too", err)
	}
}

// A Commit failure on an otherwise-successful ingest must be returned.
func TestIngestSyntaxaHierarchy_CommitErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.commitErr = fmt.Errorf("disk full")
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir, "code,rank,name,author,parent_code\nAA,class,X,,\n")

	if _, err := IngestSyntaxaHierarchy(context.Background(), repo, path); err == nil {
		t.Fatal("IngestSyntaxaHierarchy = nil error, want the Commit error surfaced")
	}
}

// UpsertSyntaxonAuthor failing during the matching pass must abort and roll
// back, same as any other write in this transaction.
func TestIngestSyntaxaHierarchy_UpsertSyntaxonAuthorErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertSyntaxonAuthor"
	repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{
		ID: "CAK-01C", Rank: "alliance", Name: "Cakilion edentulae Br.-Bl. 1931",
	})
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir,
		"code,rank,name,author,parent_code\n"+
			"AA01A,alliance,Cakilion edentulae,Br.-Bl. 1931,AA01\n")

	if _, err := IngestSyntaxaHierarchy(context.Background(), repo, path); err == nil {
		t.Fatal("IngestSyntaxaHierarchy = nil error, want the UpsertSyntaxonAuthor error surfaced")
	}
}
