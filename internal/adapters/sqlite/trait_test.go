package sqlite

import (
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func TestTraitValue_RoundTripsWithNicheWidthAndNSystems(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	nw, n := 2.47, 1
	tv := domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.22, NicheWidth: &nw, NSystems: &n}
	if err := tx.UpsertTraitValue("wcvp:1", tv); err != nil {
		t.Fatalf("UpsertTraitValue: %v", err)
	}
	if err := tx.UpsertTraitVocabulary("eive", "1.0"); err != nil {
		t.Fatalf("UpsertTraitVocabulary: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	sets, err := db.Traits(ctx, "wcvp:1", nil)
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("len(sets) = %d, want 1", len(sets))
	}
	if len(sets[0].Values) != 1 {
		t.Fatalf("len(Values) = %d, want 1", len(sets[0].Values))
	}
	got := sets[0].Values[0]
	if got.NicheWidth == nil || *got.NicheWidth != nw {
		t.Errorf("NicheWidth = %v, want %v", got.NicheWidth, nw)
	}
	if got.NSystems == nil || *got.NSystems != n {
		t.Errorf("NSystems = %v, want %v", got.NSystems, n)
	}
}

func TestTraitValue_NicheWidthAndNSystemsStayNilWhenAbsent(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	tv := domain.TraitValue{Vocab: "tichy2023", VocabVersion: "2.0", Dim: "T", Value: 4.7}
	if err := tx.UpsertTraitValue("wcvp:2", tv); err != nil {
		t.Fatalf("UpsertTraitValue: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	sets, err := db.Traits(ctx, "wcvp:2", nil)
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	got := sets[0].Values[0]
	if got.NicheWidth != nil {
		t.Errorf("NicheWidth = %v, want nil", *got.NicheWidth)
	}
	if got.NSystems != nil {
		t.Errorf("NSystems = %v, want nil", *got.NSystems)
	}
}

func TestTraits_GroupsPerVocabularyNeverMixed(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTraitValue("wcvp:3", domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.2}); err != nil {
		t.Fatalf("UpsertTraitValue eive: %v", err)
	}
	if err := tx.UpsertTraitValue("wcvp:3", domain.TraitValue{Vocab: "tichy2023", VocabVersion: "2.0", Dim: "M", Value: 5.1}); err != nil {
		t.Fatalf("UpsertTraitValue tichy2023: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	sets, err := db.Traits(ctx, "wcvp:3", nil)
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("len(sets) = %d, want 2 (never merged across vocabularies)", len(sets))
	}
	for _, s := range sets {
		if len(s.Values) != 1 {
			t.Errorf("vocab %s: len(Values) = %d, want 1", s.Vocab, len(s.Values))
		}
	}
}

func TestTraits_FiltersByVocab(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTraitValue("wcvp:4", domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.2}); err != nil {
		t.Fatalf("UpsertTraitValue eive: %v", err)
	}
	if err := tx.UpsertTraitValue("wcvp:4", domain.TraitValue{Vocab: "midolo2023", VocabVersion: "3", Dim: "disturbance_severity", Value: 0.7}); err != nil {
		t.Fatalf("UpsertTraitValue midolo2023: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	sets, err := db.Traits(ctx, "wcvp:4", []string{"midolo2023"})
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 1 || sets[0].Vocab != "midolo2023" {
		t.Fatalf("sets = %+v, want exactly one midolo2023 set", sets)
	}
}

func TestUpsertTraitValue_IsIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	upsert := func(value float64) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if err := tx.UpsertTraitValue("wcvp:5", domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: value}); err != nil {
			t.Fatalf("UpsertTraitValue: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}
	upsert(1.0)
	upsert(2.0) // repinned value overwrites, no duplicate row

	sets, err := db.Traits(ctx, "wcvp:5", nil)
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 1 || len(sets[0].Values) != 1 {
		t.Fatalf("sets = %+v, want exactly one set with one value", sets)
	}
	if sets[0].Values[0].Value != 2.0 {
		t.Errorf("Value = %v, want 2.0 (repinned overwrite, no duplicate)", sets[0].Values[0].Value)
	}
}

func TestKnownVocabs_ListsDistinctIngestedVocabularies(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTraitVocabulary("eive", "1.0"); err != nil {
		t.Fatalf("UpsertTraitVocabulary: %v", err)
	}
	if err := tx.UpsertTraitVocabulary("tichy2023", "2.0"); err != nil {
		t.Fatalf("UpsertTraitVocabulary: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.KnownVocabs(ctx)
	if err != nil {
		t.Fatalf("KnownVocabs: %v", err)
	}
	want := []string{"eive", "tichy2023"}
	if len(got) != len(want) {
		t.Fatalf("KnownVocabs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("KnownVocabs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTraits_UnknownConceptReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	sets, err := db.Traits(t.Context(), "wcvp:does-not-exist", nil)
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("sets = %+v, want empty", sets)
	}
}
