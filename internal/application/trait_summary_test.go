package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

func seedTraitSummaryRepo() *fakeRepo {
	repo := newFakeRepo()
	nw1, nw2 := 1.0, 2.0
	repo.traitValues = []fakeTraitValue{
		{ConceptID: "wcvp:concept:1", Value: domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4, NicheWidth: &nw1}},
		{ConceptID: "wcvp:concept:1", Value: domain.TraitValue{Vocab: "tichy2023", VocabVersion: "2.0", Dim: "M", Value: 5}},
		{ConceptID: "wcvp:concept:2", Value: domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 8, NicheWidth: &nw2}},
	}
	return repo
}

func TestSpeciesTraitSummary_GroupsPerVocabularyAndDimension(t *testing.T) {
	got, err := NewQueryService(seedTraitSummaryRepo()).SpeciesTraitSummary(
		context.Background(), []string{"wcvp:concept:1", "wcvp:concept:2"})
	if err != nil {
		t.Fatalf("SpeciesTraitSummary: %v", err)
	}
	if got.Requested != 2 || got.Known != 2 {
		t.Errorf("Requested/Known = %d/%d, want 2/2", got.Requested, got.Known)
	}
	eive, ok := got.Vocabularies["eive"]
	if !ok {
		t.Fatal("no eive entry")
	}
	if eive.VocabVersion != "1.0" {
		t.Errorf("VocabVersion = %q, want 1.0", eive.VocabVersion)
	}
	m := eive.Dimensions["M"]
	if m.Mean != 6 {
		t.Errorf("eive M mean = %v, want 6", m.Mean)
	}
	// Tichý's M is a different scale and must be its own entry.
	if got.Vocabularies["tichy2023"].Dimensions["M"].Mean != 5 {
		t.Errorf("tichy M mean = %v, want 5 (never merged with eive)",
			got.Vocabularies["tichy2023"].Dimensions["M"].Mean)
	}
}

// Only EIVE carries niche widths, so only EIVE may report a weighted mean.
func TestSpeciesTraitSummary_WeightedMeanOnlyWhereNicheWidthsExist(t *testing.T) {
	got, err := NewQueryService(seedTraitSummaryRepo()).SpeciesTraitSummary(
		context.Background(), []string{"wcvp:concept:1", "wcvp:concept:2"})
	if err != nil {
		t.Fatalf("SpeciesTraitSummary: %v", err)
	}
	if got.Vocabularies["eive"].Dimensions["M"].MeanNicheWeighted == nil {
		t.Error("eive M has no weighted mean, want one")
	}
	if got.Vocabularies["tichy2023"].Dimensions["M"].MeanNicheWeighted != nil {
		t.Error("tichy M reports a weighted mean; it has no niche widths at all")
	}
}

func TestSpeciesTraitSummary_ReportsUnknownConceptsWithReasons(t *testing.T) {
	got, err := NewQueryService(seedTraitSummaryRepo()).SpeciesTraitSummary(
		context.Background(), []string{"wcvp:concept:1", "wcvp:concept:999", "gbif:concept:7"})
	if err != nil {
		t.Fatalf("SpeciesTraitSummary: %v", err)
	}
	if got.Requested != 3 || got.Known != 1 {
		t.Errorf("Requested/Known = %d/%d, want 3/1", got.Requested, got.Known)
	}
	if len(got.Unknown) != 2 {
		t.Fatalf("got %d unknown entries, want 2", len(got.Unknown))
	}
	byID := map[string]string{}
	for _, u := range got.Unknown {
		byID[u.ConceptID] = u.Reason
	}
	if byID["wcvp:concept:999"] != input.ReasonUnknownConcept {
		t.Errorf("reason for the right-backbone id = %q, want %q",
			byID["wcvp:concept:999"], input.ReasonUnknownConcept)
	}
	if byID["gbif:concept:7"] != input.ReasonUnknownBackbone {
		t.Errorf("reason for the foreign-backbone id = %q, want %q",
			byID["gbif:concept:7"], input.ReasonUnknownBackbone)
	}
}

func TestSpeciesTraitSummary_EmptyListIsInvalidQuery(t *testing.T) {
	_, err := NewQueryService(seedTraitSummaryRepo()).SpeciesTraitSummary(context.Background(), nil)
	if !errors.Is(err, input.ErrInvalidQuery) {
		t.Errorf("error = %v, want ErrInvalidQuery", err)
	}
}

// Nothing resolvable is a normal answer, not a failure: empty vocabularies,
// every id reported back.
func TestSpeciesTraitSummary_AllUnknownIsEmptyNotError(t *testing.T) {
	got, err := NewQueryService(seedTraitSummaryRepo()).SpeciesTraitSummary(
		context.Background(), []string{"wcvp:concept:999"})
	if err != nil {
		t.Fatalf("SpeciesTraitSummary: %v", err)
	}
	if len(got.Vocabularies) != 0 {
		t.Errorf("got %d vocabularies, want none", len(got.Vocabularies))
	}
	if len(got.Unknown) != 1 {
		t.Errorf("got %d unknown entries, want 1", len(got.Unknown))
	}
}

// When every requested id carries a foreign backbone prefix, wanted is empty
// and the repository must not be asked at all. traitsForConceptsErr proves
// it: if the code called TraitsForConcepts regardless, this injected error
// would surface and the test would fail.
func TestSpeciesTraitSummary_AllForeignBackboneNeverQueriesTheRepository(t *testing.T) {
	repo := seedTraitSummaryRepo()
	repo.traitsForConceptsErr = errors.New("must not be called")

	got, err := NewQueryService(repo).SpeciesTraitSummary(
		context.Background(), []string{"gbif:concept:1", "gbif:concept:2"})
	if err != nil {
		t.Fatalf("SpeciesTraitSummary: %v", err)
	}
	if got.Requested != 2 || got.Known != 0 {
		t.Errorf("Requested/Known = %d/%d, want 2/0", got.Requested, got.Known)
	}
	if len(got.Vocabularies) != 0 {
		t.Errorf("got %d vocabularies, want none", len(got.Vocabularies))
	}
	if len(got.Unknown) != 2 {
		t.Fatalf("got %d unknown entries, want 2", len(got.Unknown))
	}
	for _, u := range got.Unknown {
		if u.Reason != input.ReasonUnknownBackbone {
			t.Errorf("reason for %q = %q, want %q", u.ConceptID, u.Reason, input.ReasonUnknownBackbone)
		}
	}
}

func TestSpeciesTraitSummary_RepositoryErrorIsWrapped(t *testing.T) {
	repo := seedTraitSummaryRepo()
	repo.traitsForConceptsErr = errors.New("disk on fire")

	_, err := NewQueryService(repo).SpeciesTraitSummary(context.Background(), []string{"wcvp:concept:1"})
	if err == nil {
		t.Fatal("want an error when the repository fails")
	}
}

// A dimension where no species carried a value must be absent, not a row of
// zeroes.
func TestSpeciesTraitSummary_DimensionWithoutValuesIsAbsent(t *testing.T) {
	got, err := NewQueryService(seedTraitSummaryRepo()).SpeciesTraitSummary(
		context.Background(), []string{"wcvp:concept:2"})
	if err != nil {
		t.Fatalf("SpeciesTraitSummary: %v", err)
	}
	if _, present := got.Vocabularies["eive"].Dimensions["N"]; present {
		t.Error("dimension N is present although no species carried a value for it")
	}
}
