package application

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func seedTraitDir(t *testing.T, eive, tichy, midolo string) string {
	t.Helper()
	dir := t.TempDir()
	if eive != "" {
		writeCSV(t, dir, "eive_traits.csv", eive)
	}
	if tichy != "" {
		writeCSV(t, dir, "tichy_traits.csv", tichy)
	}
	if midolo != "" {
		writeCSV(t, dir, "midolo_traits.csv", midolo)
	}
	return dir
}

func traitCSVPaths(dir string) map[string]string {
	return map[string]string{
		"eive":       dir + "/eive_traits.csv",
		"tichy2023":  dir + "/tichy_traits.csv",
		"midolo2023": dir + "/midolo_traits.csv",
	}
}

func TestIngestTraits_ResolvesAndStoresByConceptID(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n",
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|tichy2023|2.0|T|4.7||\n",
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|midolo2023|3|disturbance_severity|0.7||\n")

	repo := newFakeRepo()
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}
	rep, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir))
	if err != nil {
		t.Fatalf("IngestTraits: %v", err)
	}
	if rep.Rows != 3 || rep.Resolved != 3 || rep.Unresolved != 0 {
		t.Errorf("Rows/Resolved/Unresolved = %d/%d/%d, want 3/3/0", rep.Rows, rep.Resolved, rep.Unresolved)
	}
	if len(repo.traitValues) != 3 {
		t.Fatalf("len(traitValues) = %d, want 3", len(repo.traitValues))
	}
	for _, tv := range repo.traitValues {
		if tv.ConceptID != "wcvp:1" {
			t.Errorf("ConceptID = %q, want wcvp:1", tv.ConceptID)
		}
	}
	if len(repo.traitVocabs) != 3 {
		t.Fatalf("len(traitVocabs) = %d, want 3 (one per vocab file actually read)", len(repo.traitVocabs))
	}
}

func TestIngestTraits_PerVocabBreakdown(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n"+
			"Unknown plant|eive|1.0|M|3.0|1.0|1\n",
		"", "")
	repo := newFakeRepo()
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}
	rep, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir))
	if err != nil {
		t.Fatalf("IngestTraits: %v", err)
	}
	vr, ok := rep.PerVocab["eive"]
	if !ok {
		t.Fatal("PerVocab has no \"eive\" entry")
	}
	if vr.Rows != 2 || vr.Resolved != 1 || vr.Unresolved != 1 {
		t.Errorf("eive VocabReport = %+v, want Rows=2 Resolved=1 Unresolved=1", vr)
	}
	if rep.Unresolved != 1 {
		t.Errorf("rep.Unresolved = %d, want 1", rep.Unresolved)
	}
	// An unresolved taxon's row is dropped, never stored with a nil concept
	// id — trait_value's primary key requires one, unlike species_role.
	if len(repo.traitValues) != 1 {
		t.Errorf("len(traitValues) = %d, want 1 (unresolved row discarded)", len(repo.traitValues))
	}
}

func TestIngestTraits_MissingFileIsSkippedNotAborted(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n",
		"", "")
	repo := newFakeRepo()
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}
	rep, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir))
	if err != nil {
		t.Fatalf("IngestTraits: %v", err)
	}
	want := map[string]bool{"tichy2023": true, "midolo2023": true}
	if len(rep.Skipped) != 2 {
		t.Fatalf("Skipped = %v, want 2 entries", rep.Skipped)
	}
	for _, s := range rep.Skipped {
		if !want[s] {
			t.Errorf("Skipped contains unexpected vocab %q", s)
		}
	}
	if _, ok := rep.PerVocab["tichy2023"]; ok {
		t.Error("PerVocab should not carry an entry for a skipped vocab")
	}
}

func TestIngestTraits_ResolverErrorAbortsTheIngest(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n",
		"", "")
	repo := newFakeRepo()
	_, err := IngestTraits(context.Background(), repo, erroringResolver{}, traitCSVPaths(dir))
	if err == nil {
		t.Fatal("IngestTraits with a failing resolver: want error, got nil")
	}
	if len(repo.traitValues) != 0 {
		t.Errorf("traitValues = %v, want none written on resolver failure", repo.traitValues)
	}
}

// Every unparseable-field branch of readTraitRows in one pass: an empty dim,
// a bad value, a bad niche_width, a bad n_systems and a vocab column that
// disagrees with the file's own vocabulary all skip their row instead of
// aborting the file or being trusted at face value.
func TestReadTraitRows_SkipsEveryUnparseableOrMismatchedRow(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0||4.22|2.47|1\n"+ // empty dim
			"Inula hirta|eive|1.0|M|not-a-number|2.47|1\n"+ // bad value
			"Inula hirta|eive|1.0|M|4.22|not-a-number|1\n"+ // bad niche_width
			"Inula hirta|eive|1.0|M|4.22|2.47|not-a-number\n"+ // bad n_systems
			"Inula hirta|tichy2023|1.0|M|4.22|2.47|1\n"+ // vocab column != map key "eive"
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n", // the one well-formed row
		"", "")

	repo := newFakeRepo()
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}
	rep, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir))
	if err != nil {
		t.Fatalf("IngestTraits: %v", err)
	}
	vr := rep.PerVocab["eive"]
	if vr.Rows != 1 || vr.Resolved != 1 {
		t.Errorf("eive VocabReport = %+v, want exactly the one well-formed row", vr)
	}
	if len(repo.traitValues) != 1 {
		t.Fatalf("len(traitValues) = %d, want 1 (every malformed/mismatched row skipped)", len(repo.traitValues))
	}
}

// A repinned vocabulary must not leave the previous version's rows behind:
// two ingests of the same vocab under different vocab_version must leave
// only the second version in the index.
func TestIngestTraits_RepinnedVersionReplacesThePriorOne(t *testing.T) {
	dir1 := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n", "", "")
	repo := newFakeRepo()
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}
	if _, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir1)); err != nil {
		t.Fatalf("first IngestTraits: %v", err)
	}

	dir2 := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.1|M|4.50|2.10|2\n", "", "")
	if _, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir2)); err != nil {
		t.Fatalf("second IngestTraits: %v", err)
	}

	if len(repo.traitValues) != 1 {
		t.Fatalf("len(traitValues) = %d, want 1 (old version replaced, not accumulated)", len(repo.traitValues))
	}
	got := repo.traitValues[0].Value
	if got.VocabVersion != "1.1" || got.Value != 4.50 {
		t.Errorf("surviving trait value = %+v, want vocab_version 1.1 / value 4.50", got)
	}
}

func TestIngestTraits_ReadErrorIsReturned(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n", "", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := newFakeRepo()
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}
	if _, err := IngestTraits(ctx, repo, resolver, traitCSVPaths(dir)); err == nil {
		t.Fatal("IngestTraits with a canceled context = nil error, want an error")
	}
}

func TestIngestTraits_BeginErrorIsReturned(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n", "", "")

	repo := newFakeRepo()
	repo.beginErr = errors.New("boom")
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}
	if _, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir)); err == nil {
		t.Fatal("IngestTraits with a Begin error = nil error, want an error")
	}
}

func TestIngestTraits_CommitErrorIsReturned(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n", "", "")

	repo := newFakeRepo()
	repo.commitErr = errors.New("disk full")
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}
	if _, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir)); err == nil {
		t.Fatal("IngestTraits with a Commit error = nil error, want an error")
	}
}

// Each repository failure the write side of one vocab can hit must roll back
// and surface, never commit a half-written trait ingest.
func TestIngestTraits_RepositoryErrorPerEntity(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n", "", "")

	for _, failOn := range []string{"DeleteTraitValuesForVocab", "UpsertTraitValue", "UpsertTraitVocabulary"} {
		t.Run(failOn, func(t *testing.T) {
			repo := newFakeRepo()
			repo.failOn = failOn
			resolver := fakeResolver{"Inula hirta": "wcvp:1"}

			if _, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir)); err == nil {
				t.Fatalf("IngestTraits with %s failing = nil error, want an error", failOn)
			}
			if repo.committed {
				t.Errorf("ingest committed despite %s failing", failOn)
			}
			if !repo.rolledBack {
				t.Errorf("ingest did not roll back after %s failed", failOn)
			}
		})
	}
}

// A rollback failure must not hide the original cause.
func TestIngestTraits_RollbackErrorIsWrappedWithTheOriginal(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n", "", "")

	repo := newFakeRepo()
	repo.failOn = "UpsertTraitValue"
	repo.rollbackErr = errors.New("connection lost")
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}

	_, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir))
	if err == nil {
		t.Fatal("IngestTraits = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "connection lost") {
		t.Errorf("error = %q, want it to mention the rollback failure too", err)
	}
}

// A rollback failure on the DeleteTraitValuesForVocab error path must not
// hide the original cause either — the same guarantee as
// TestIngestTraits_RollbackErrorIsWrappedWithTheOriginal, pinned separately
// because it is a distinct branch in IngestTraits.
func TestIngestTraits_RollbackErrorAfterDeleteFailureIsWrapped(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n", "", "")

	repo := newFakeRepo()
	repo.failOn = "DeleteTraitValuesForVocab"
	repo.rollbackErr = errors.New("connection lost")
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}

	_, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir))
	if err == nil {
		t.Fatal("IngestTraits = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "connection lost") {
		t.Errorf("error = %q, want it to mention the rollback failure too", err)
	}
}

func TestIngestTraits_NicheWidthAndNSystemsStayNilWhenColumnsAreEmpty(t *testing.T) {
	dir := seedTraitDir(t, "", "taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
		"Inula hirta|tichy2023|2.0|T|4.7||\n", "")
	repo := newFakeRepo()
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}
	if _, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir)); err != nil {
		t.Fatalf("IngestTraits: %v", err)
	}
	if len(repo.traitValues) != 1 {
		t.Fatalf("len(traitValues) = %d, want 1", len(repo.traitValues))
	}
	tv := repo.traitValues[0].Value
	if tv.NicheWidth != nil {
		t.Errorf("NicheWidth = %v, want nil", *tv.NicheWidth)
	}
	if tv.NSystems != nil {
		t.Errorf("NSystems = %v, want nil", *tv.NSystems)
	}
}
