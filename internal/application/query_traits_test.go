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

func TestQueryService_Traits_FiltersByAKnownVocab(t *testing.T) {
	repo := newFakeRepo()
	repo.traitVocabs = []fakeTraitVocab{{Vocab: "eive", Version: "1.0"}, {Vocab: "tichy2023", Version: "2.0"}}
	repo.traitValues = []fakeTraitValue{
		{ConceptID: "wcvp:1", Value: domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.2}},
		{ConceptID: "wcvp:1", Value: domain.TraitValue{Vocab: "tichy2023", VocabVersion: "2.0", Dim: "T", Value: 5.1}},
	}
	q := NewQueryService(repo)
	sets, err := q.Traits(context.Background(), "wcvp:1", "eive")
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 1 || sets[0].Vocab != "eive" {
		t.Fatalf("sets = %+v, want exactly one eive set", sets)
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

// A KnownVocabs failure must surface, not be swallowed into "unknown vocab":
// otherwise a database outage would misreport as a client typo.
func TestQueryService_Traits_KnownVocabsErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	wantErr := errors.New("index unreadable")
	repo.knownVocabsErr = wantErr
	q := NewQueryService(repo)
	_, err := q.Traits(context.Background(), "wcvp:1", "eive")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want it to wrap %v", err, wantErr)
	}
}

// A Traits failure (the data fetch itself, not the ?vocab= validation) must
// surface too.
func TestQueryService_Traits_TraitsErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	wantErr := errors.New("index unreadable")
	repo.traitsErr = wantErr
	q := NewQueryService(repo)
	_, err := q.Traits(context.Background(), "wcvp:1", "")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want it to wrap %v", err, wantErr)
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
