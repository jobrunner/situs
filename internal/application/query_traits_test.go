package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

func TestQueryService_Traits_ReturnsEveryVocabularyWhenNoneRequested(t *testing.T) {
	repo := newFakeRepo()
	repo.traitVocabs = []fakeTraitVocab{{Vocab: "eive", Version: "1.0"}}
	repo.traitValues = []fakeTraitValue{
		{ConceptID: "wcvp:1", Value: domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.2}},
	}
	q := NewQueryService(repo)
	sets, err := q.Traits(context.Background(), "wcvp:1", "")
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 1 || sets[0].Vocab != "eive" {
		t.Fatalf("sets = %+v, want one eive set", sets)
	}
}

func TestQueryService_Traits_UnknownVocabIsInvalidQuery(t *testing.T) {
	repo := newFakeRepo()
	repo.traitVocabs = []fakeTraitVocab{{Vocab: "eive", Version: "1.0"}}
	q := NewQueryService(repo)
	_, err := q.Traits(context.Background(), "wcvp:1", "does-not-exist")
	if !errors.Is(err, input.ErrUnknownVocab) {
		t.Fatalf("err = %v, want input.ErrUnknownVocab", err)
	}
}

func TestQueryService_Traits_UnknownConceptReturnsEmptyNotError(t *testing.T) {
	repo := newFakeRepo()
	repo.traitVocabs = []fakeTraitVocab{{Vocab: "eive", Version: "1.0"}}
	q := NewQueryService(repo)
	sets, err := q.Traits(context.Background(), "wcvp:does-not-exist", "")
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("sets = %+v, want empty (no data is a normal answer, not an error)", sets)
	}
}
