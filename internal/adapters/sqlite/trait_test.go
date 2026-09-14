package sqlite

import (
	"context"
	"strings"
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

func TestTraitsForConcepts_GroupsByConceptAndOmitsUnknownOnes(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTraitVocabulary("eive", "1.0"); err != nil {
		t.Fatalf("UpsertTraitVocabulary: %v", err)
	}
	nw := 2.5
	for conceptID, values := range map[string][]domain.TraitValue{
		"wcvp:concept:1": {
			{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.2, NicheWidth: &nw},
			{Vocab: "eive", VocabVersion: "1.0", Dim: "N", Value: 3.1, NicheWidth: &nw},
		},
		"wcvp:concept:2": {
			{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 6.8, NicheWidth: &nw},
		},
	} {
		for _, v := range values {
			if err := tx.UpsertTraitValue(conceptID, v); err != nil {
				t.Fatalf("UpsertTraitValue: %v", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.TraitsForConcepts(context.Background(),
		[]string{"wcvp:concept:1", "wcvp:concept:2", "wcvp:concept:999"})
	if err != nil {
		t.Fatalf("TraitsForConcepts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d concepts, want 2 (the unknown one must be absent, not empty)", len(got))
	}
	if len(got["wcvp:concept:1"]) != 2 {
		t.Errorf("concept 1 has %d values, want 2", len(got["wcvp:concept:1"]))
	}
	if _, present := got["wcvp:concept:999"]; present {
		t.Error("the unknown concept is present in the map; it must be absent")
	}
	if got["wcvp:concept:2"][0].NicheWidth == nil {
		t.Error("niche width was lost on the way out")
	}
}

func TestTraitsForConcepts_EmptyInputIsEmptyMapNotError(t *testing.T) {
	db := openTestDB(t)

	got, err := db.TraitsForConcepts(context.Background(), nil)
	if err != nil {
		t.Fatalf("TraitsForConcepts: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d entries, want none", len(got))
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

// A repinned vocabulary (e.g. EIVE 1.0 -> 1.1) must not leave the old
// version's rows behind: DeleteTraitValuesForVocab is what IngestTraits calls
// before writing the new version, and it must drop every version of the
// named vocabulary, not just one.
func TestDeleteTraitValuesForVocab_RemovesEveryVersion(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTraitValue("wcvp:6", domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.2}); err != nil {
		t.Fatalf("UpsertTraitValue 1.0: %v", err)
	}
	if err := tx.UpsertTraitValue("wcvp:6", domain.TraitValue{Vocab: "eive", VocabVersion: "1.1", Dim: "M", Value: 4.3}); err != nil {
		t.Fatalf("UpsertTraitValue 1.1: %v", err)
	}
	if err := tx.UpsertTraitValue("wcvp:6", domain.TraitValue{Vocab: "tichy2023", VocabVersion: "2.0", Dim: "T", Value: 5.0}); err != nil {
		t.Fatalf("UpsertTraitValue tichy2023: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	tx2, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx2.DeleteTraitValuesForVocab("eive"); err != nil {
		t.Fatalf("DeleteTraitValuesForVocab: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	sets, err := db.Traits(ctx, "wcvp:6", nil)
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 1 || sets[0].Vocab != "tichy2023" {
		t.Fatalf("sets = %+v, want only tichy2023 left (both eive versions deleted)", sets)
	}
}

// Every read must surface a query failure instead of an empty answer, same
// contract as the rest of the read side.
func TestTraitReads_QueryErrorsAreReturned(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	ctx := context.Background()

	cases := map[string]func() error{
		"Traits":            func() error { _, err := db.Traits(ctx, "wcvp:1", nil); return err },
		"KnownVocabs":       func() error { _, err := db.KnownVocabs(ctx); return err },
		"TraitsForConcepts": func() error { _, err := db.TraitsForConcepts(ctx, []string{"wcvp:1"}); return err },
	}
	for name, call := range cases {
		if err := call(); err == nil {
			t.Errorf("%s on a closed database = nil error, want an error", name)
		} else if !strings.HasPrefix(err.Error(), "sqlite: ") {
			t.Errorf("%s error = %q, want the adapter's own context prefixed", name, err)
		}
	}
}

// The rows.Err()/Scan paths, exercised deterministically with the stub driver
// instead of racing a cancellation against row iteration — same technique as
// read_test.go's TestReads_RowsIterationAndScanErrorsAreReturned.
func TestTraitReads_RowsIterationAndScanErrorsAreReturned(t *testing.T) {
	ctx := context.Background()
	cases := map[string]struct {
		call func(db *DB) error
		rows string
		scan string
	}{
		"Traits": {
			call: func(db *DB) error { _, err := db.Traits(ctx, "wcvp:1", nil); return err },
			rows: "iterating traits", scan: "scanning trait value",
		},
		"KnownVocabs": {
			call: func(db *DB) error { _, err := db.KnownVocabs(ctx); return err },
			rows: "known vocabs", scan: "known vocabs",
		},
		"TraitsForConcepts": {
			call: func(db *DB) error { _, err := db.TraitsForConcepts(ctx, []string{"wcvp:1"}); return err },
			rows: "iterating trait values", scan: "scanning trait value",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for mode, want := range map[stubMode]string{stubModeRowsErr: tc.rows, stubModeScanErr: tc.scan} {
				err := tc.call(&DB{DB: newStubDB(t, mode)})
				if err == nil {
					t.Fatalf("%s in mode %v = nil error, want an error", name, mode)
				}
				if !strings.Contains(err.Error(), want) {
					t.Errorf("%s error = %q, want it to name %q", name, err, want)
				}
			}
		})
	}
}
