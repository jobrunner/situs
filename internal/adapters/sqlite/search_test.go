package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// seedSearchDB builds an index with three names: two resolved, one not.
func seedSearchDB(t *testing.T) *DB {
	t.Helper()
	db := openTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTypology(domain.Typology{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}); err != nil {
		t.Fatalf("UpsertTypology: %v", err)
	}
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T17"}
	if err := tx.UpsertHabitatType(domain.HabitatType{Key: key, NameEN: "Beech forest"}); err != nil {
		t.Fatalf("UpsertHabitatType: %v", err)
	}
	fagus := "wcvp:concept:83891"
	abies := "wcvp:concept:381621"
	for _, r := range []domain.SpeciesRole{
		{Key: key, VerbatimName: "Fagus sylvatica", Role: "constant", ConceptID: &fagus},
		{Key: key, VerbatimName: "Abies alba", Role: "constant", ConceptID: &abies},
		{Key: key, VerbatimName: "Fagus orientalis", Role: "diagnostic"},
	} {
		if err := tx.UpsertSpeciesRole(r); err != nil {
			t.Fatalf("UpsertSpeciesRole %q: %v", r.VerbatimName, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return db
}

func TestSearchSpeciesNames_SubstringCaseInsensitiveOrdered(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "fagus", 20)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d hits, want 2 (both Fagus names, case-insensitively)", len(got))
	}
	// Ordered by name: "Fagus orientalis" sorts before "Fagus sylvatica".
	if got[0].VerbatimName != "Fagus orientalis" || got[1].VerbatimName != "Fagus sylvatica" {
		t.Errorf("order = %q, %q; want Fagus orientalis before Fagus sylvatica",
			got[0].VerbatimName, got[1].VerbatimName)
	}
}

// An unresolved name must come back WITH the others, carrying a nil concept
// id — never filtered out, or the search would overstate the index.
func TestSearchSpeciesNames_KeepsUnresolvedNamesWithNilConceptID(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "orientalis", 20)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d hits, want 1", len(got))
	}
	if got[0].ConceptID != nil {
		t.Errorf("ConceptID = %v, want nil for the unresolved name", *got[0].ConceptID)
	}
}

func TestSearchSpeciesNames_ResolvedNameCarriesItsConceptID(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "sylvatica", 20)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d hits, want 1", len(got))
	}
	if got[0].ConceptID == nil || *got[0].ConceptID != "wcvp:concept:83891" {
		t.Errorf("ConceptID = %v, want wcvp:concept:83891", got[0].ConceptID)
	}
}

func TestSearchSpeciesNames_LimitCapsTheResult(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "a", 1)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d hits, want exactly the 1 the limit allows", len(got))
	}
}

// A percent sign in the QUERY must be a literal, not a wildcard — otherwise
// "%" alone would dump the whole index.
func TestSearchSpeciesNames_EscapesLikeWildcardsInTheQuery(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "%", 20)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d hits for a literal %%, want 0 (no stored name contains one)", len(got))
	}
}

func TestSearchSpeciesNames_UnderscoreIsLiteralToo(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "_", 20)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d hits for a literal underscore, want 0", len(got))
	}
}

// Two rows can share a verbatim_name while carrying different concept ids
// (e.g. the same name recorded under two roles, resolved to two different
// concepts by a future ingest). ORDER BY verbatim_name alone leaves such a
// tie to sqlite's discretion; concept_id as a second key makes the order
// deterministic across repeated calls.
func TestSearchSpeciesNames_SameNameDifferentConceptIDsHaveAStableOrder(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTypology(domain.Typology{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}); err != nil {
		t.Fatalf("UpsertTypology: %v", err)
	}
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T17"}
	if err := tx.UpsertHabitatType(domain.HabitatType{Key: key, NameEN: "Beech forest"}); err != nil {
		t.Fatalf("UpsertHabitatType: %v", err)
	}
	conceptA := "wcvp:concept:100"
	conceptB := "wcvp:concept:200"
	for _, r := range []domain.SpeciesRole{
		{Key: key, VerbatimName: "Fagus sylvatica", Role: "diagnostic", ConceptID: &conceptB},
		{Key: key, VerbatimName: "Fagus sylvatica", Role: "constant", ConceptID: &conceptA},
	} {
		if err := tx.UpsertSpeciesRole(r); err != nil {
			t.Fatalf("UpsertSpeciesRole %q/%q: %v", r.VerbatimName, r.Role, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	for i := 0; i < 5; i++ {
		got, err := db.SearchSpeciesNames(context.Background(), "sylvatica", 20)
		if err != nil {
			t.Fatalf("SearchSpeciesNames: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d hits, want 2 (both concept ids for the shared name)", len(got))
		}
		if got[0].ConceptID == nil || *got[0].ConceptID != conceptA {
			t.Errorf("got[0].ConceptID = %v, want %s", got[0].ConceptID, conceptA)
		}
		if got[1].ConceptID == nil || *got[1].ConceptID != conceptB {
			t.Errorf("got[1].ConceptID = %v, want %s", got[1].ConceptID, conceptB)
		}
	}
}

func TestSearchSpeciesNames_NoMatchIsEmptyNotError(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "Quercus", 20)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d hits, want none", len(got))
	}
}
