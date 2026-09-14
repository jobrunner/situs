package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

func TestSearchSpecies_MapsHitsIncludingUnresolvedOnes(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesNames = []domain.SpeciesName{
		{VerbatimName: "Fagus sylvatica", ConceptID: strPtr("wcvp:concept:83891")},
		{VerbatimName: "Fagus orientalis"},
	}

	got, err := NewQueryService(repo).SearchSpecies(context.Background(), "fagus", 20)
	if err != nil {
		t.Fatalf("SearchSpecies: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d hits, want 2", len(got))
	}
	// Looked up by name, not by index position: this test's intent is that
	// both hit kinds survive the mapping, not that they arrive in a
	// particular order — the repository port's own sorting guarantee is
	// covered elsewhere (TestFakeRepo_SearchSpeciesNames_OrdersResultsByName,
	// TestSearchSpeciesNames_SubstringCaseInsensitiveOrdered).
	byName := map[string]*string{}
	for _, h := range got {
		byName[h.VerbatimName] = h.ConceptID
	}
	if id := byName["Fagus sylvatica"]; id == nil || *id != "wcvp:concept:83891" {
		t.Errorf("Fagus sylvatica ConceptID = %v, want wcvp:concept:83891", id)
	}
	if id, present := byName["Fagus orientalis"]; !present {
		t.Error("Fagus orientalis is missing; an unresolved name must not be dropped")
	} else if id != nil {
		t.Errorf("Fagus orientalis ConceptID = %v, want nil", *id)
	}
}

func TestSearchSpecies_EmptyQueryIsInvalid(t *testing.T) {
	for _, q := range []string{"", "   "} {
		_, err := NewQueryService(newFakeRepo()).SearchSpecies(context.Background(), q, 20)
		if !errors.Is(err, input.ErrInvalidQuery) {
			t.Errorf("SearchSpecies(%q) error = %v, want ErrInvalidQuery", q, err)
		}
	}
}

// Zero means "unset" and takes the default; a negative or oversized value is
// a malformed request, not something to silently clamp.
func TestSearchSpecies_LimitZeroTakesTheDefault(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesNames = make([]domain.SpeciesName, 50)
	for i := range repo.speciesNames {
		repo.speciesNames[i] = domain.SpeciesName{VerbatimName: "Aaa" + strings.Repeat("a", i)}
	}

	got, err := NewQueryService(repo).SearchSpecies(context.Background(), "aaa", 0)
	if err != nil {
		t.Fatalf("SearchSpecies: %v", err)
	}
	if len(got) != input.DefaultSearchLimit {
		t.Errorf("got %d hits, want the default limit %d", len(got), input.DefaultSearchLimit)
	}
}

func TestSearchSpecies_RejectsNegativeAndOversizedLimit(t *testing.T) {
	for _, limit := range []int{-1, input.MaxSearchLimit + 1} {
		_, err := NewQueryService(newFakeRepo()).SearchSpecies(context.Background(), "fagus", limit)
		if !errors.Is(err, input.ErrInvalidQuery) {
			t.Errorf("SearchSpecies(limit=%d) error = %v, want ErrInvalidQuery", limit, err)
		}
	}
}

func TestSearchSpecies_RepositoryErrorIsWrapped(t *testing.T) {
	repo := newFakeRepo()
	repo.searchErr = errors.New("disk on fire")

	_, err := NewQueryService(repo).SearchSpecies(context.Background(), "fagus", 20)
	if err == nil {
		t.Fatal("SearchSpecies with a failing repository = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "searching species names") {
		t.Errorf("error = %q, want it to name the operation", err)
	}
}

func TestSearchSpecies_NoMatchIsAnEmptySliceNotNil(t *testing.T) {
	got, err := NewQueryService(newFakeRepo()).SearchSpecies(context.Background(), "quercus", 20)
	if err != nil {
		t.Fatalf("SearchSpecies: %v", err)
	}
	if got == nil {
		t.Error("result is nil; want an empty slice so it marshals to [] not null")
	}
}
