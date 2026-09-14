package application

import (
	"context"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// These two tests pin fakeRepo.SearchSpeciesNames itself against the two
// behaviors output.Repository.SearchSpeciesNames and its real sqlite
// adapter guarantee: stable name ordering and SQL LIMIT semantics
// (including LIMIT 0 meaning zero rows). Task 2's application-level tests
// build on fakeRepo, so a mismatch here would silently teach the wrong
// behavior.

func TestFakeRepo_SearchSpeciesNames_OrdersResultsByName(t *testing.T) {
	r := &fakeRepo{speciesNames: []domain.SpeciesName{
		{VerbatimName: "Fagus sylvatica"},
		{VerbatimName: "Abies alba"},
		{VerbatimName: "Fagus orientalis"},
	}}

	got, err := r.SearchSpeciesNames(context.Background(), "", 20)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	want := []string{"Abies alba", "Fagus orientalis", "Fagus sylvatica"}
	if len(got) != len(want) {
		t.Fatalf("got %d hits, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].VerbatimName != name {
			t.Errorf("got[%d] = %q, want %q (order = %v)", i, got[i].VerbatimName, name, got)
		}
	}
}

func TestFakeRepo_SearchSpeciesNames_LimitZeroReturnsNoRows(t *testing.T) {
	r := &fakeRepo{speciesNames: []domain.SpeciesName{
		{VerbatimName: "Fagus sylvatica"},
		{VerbatimName: "Abies alba"},
	}}

	got, err := r.SearchSpeciesNames(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d hits for limit=0, want 0 (matches SQL's LIMIT 0)", len(got))
	}
}
