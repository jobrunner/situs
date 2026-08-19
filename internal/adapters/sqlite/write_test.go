package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

func (d *DB) countHabitatTypes(ctx context.Context) (int, error) {
	var n int
	if err := d.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM habitat_type").Scan(&n); err != nil {
		return 0, fmt.Errorf("counting habitat_type rows: %w", err)
	}
	return n, nil
}

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(t.Context(), filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestIngestTx_RoundTripsAHabitatType(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	level := 3
	if err := tx.UpsertTypology(domain.Typology{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}); err != nil {
		t.Fatalf("UpsertTypology: %v", err)
	}
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	if err := tx.UpsertHabitatType(domain.HabitatType{Key: key, Level: &level, NameEN: "Low and medium altitude hay meadow"}); err != nil {
		t.Fatalf("UpsertHabitatType: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.HabitatType(ctx, key)
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	if got.NameEN != "Low and medium altitude hay meadow" {
		t.Errorf("NameEN = %q, want the ingested name", got.NameEN)
	}
	if got.Level == nil || *got.Level != 3 {
		t.Errorf("Level = %v, want 3", got.Level)
	}
	if got.Priority != nil {
		t.Errorf("Priority = %v, want nil — a EUNIS type has no priority flag at all", *got.Priority)
	}
}

// The nullable columns of habitat_type must round-trip NULL as nil in BOTH
// directions: a level that was never given must not come back as 0, and a
// priority that was never given must not come back as false. That difference is
// the whole point of the pointer fields, and nothing else in the suite pins it.
func TestIngestTx_HabitatTypeNullablesRoundTripBothWays(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	priority := true
	unset := domain.HabitatTypeKey{Typology: "annex1", Code: "0000"}
	set := domain.HabitatTypeKey{Typology: "annex1", Code: "6510"}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertHabitatType(domain.HabitatType{Key: unset, NameEN: "no level, no priority"}); err != nil {
		t.Fatalf("UpsertHabitatType(unset): %v", err)
	}
	if err := tx.UpsertHabitatType(domain.HabitatType{Key: set, NameEN: "Lowland hay meadows", Priority: &priority}); err != nil {
		t.Fatalf("UpsertHabitatType(set): %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	gotUnset, err := db.HabitatType(ctx, unset)
	if err != nil {
		t.Fatalf("HabitatType(unset): %v", err)
	}
	if gotUnset.Level != nil {
		t.Errorf("Level = %v, want nil — an absent level must not become 0", *gotUnset.Level)
	}
	if gotUnset.Priority != nil {
		t.Errorf("Priority = %v, want nil — an absent flag must not become false", *gotUnset.Priority)
	}

	gotSet, err := db.HabitatType(ctx, set)
	if err != nil {
		t.Fatalf("HabitatType(set): %v", err)
	}
	if gotSet.Priority == nil || !*gotSet.Priority {
		t.Errorf("Priority = %v, want a non-nil true", gotSet.Priority)
	}
}

// Re-ingesting the same source must not duplicate or fail — ingest is rerun
// whenever an artifact is repinned.
func TestIngestTx_UpsertIsIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}

	for i, name := range []string{"first", "second"} {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Begin %d: %v", i, err)
		}
		if err := tx.UpsertTypology(domain.Typology{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}); err != nil {
			t.Fatalf("UpsertTypology %d: %v", i, err)
		}
		if err := tx.UpsertHabitatType(domain.HabitatType{Key: key, NameEN: name}); err != nil {
			t.Fatalf("UpsertHabitatType %d: %v", i, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit %d: %v", i, err)
		}
	}

	got, err := db.HabitatType(ctx, key)
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	if got.NameEN != "second" {
		t.Errorf("NameEN = %q, want %q (the later ingest wins)", got.NameEN, "second")
	}
	n, err := db.countHabitatTypes(ctx)
	if err != nil {
		t.Fatalf("countHabitatTypes: %v", err)
	}
	if n != 1 {
		t.Errorf("habitat_type rows = %d, want 1 (upsert, not insert)", n)
	}
}

// Rollback must leave nothing behind — a failed ingest may not half-populate.
func TestIngestTx_RollbackDiscards(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTypology(domain.Typology{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}); err != nil {
		t.Fatalf("UpsertTypology: %v", err)
	}
	if err := tx.UpsertHabitatType(domain.HabitatType{
		Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, NameEN: "x",
	}); err != nil {
		t.Fatalf("UpsertHabitatType: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	n, err := db.countHabitatTypes(ctx)
	if err != nil {
		t.Fatalf("countHabitatTypes: %v", err)
	}
	if n != 0 {
		t.Errorf("habitat_type rows = %d after rollback, want 0", n)
	}
}

func TestDB_HabitatType_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	_, err := db.HabitatType(ctx, domain.HabitatTypeKey{Typology: "eunis@2021", Code: "does-not-exist"})
	if !errors.Is(err, output.ErrNotFound) {
		t.Errorf("HabitatType error = %v, want it to wrap output.ErrNotFound", err)
	}
}

// UpsertCrosswalk, UpsertSyntaxon, LinkSyntaxon, UpsertSpeciesRole and
// UpsertLocalization must each be idempotent, and nullable columns (concept_id,
// fidelity, constancy) must round-trip NULL as nil, not a placeholder.
func TestIngestTx_UpsertsCrosswalkSyntaxonSpeciesRoleAndLocalization(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	from := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	to := domain.HabitatTypeKey{Typology: "annex1", Code: "6510"}

	ingestOnce := func(qualifier domain.Qualifier, fidelity *float64) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if err := tx.UpsertCrosswalk(domain.Crosswalk{From: from, To: to, Qualifier: qualifier}); err != nil {
			t.Fatalf("UpsertCrosswalk: %v", err)
		}
		if err := tx.UpsertSyntaxon(domain.Syntaxon{ID: "arrhenatherion", Rank: "alliance", Name: "Arrhenatherion"}); err != nil {
			t.Fatalf("UpsertSyntaxon: %v", err)
		}
		if err := tx.LinkSyntaxon(from, "arrhenatherion"); err != nil {
			t.Fatalf("LinkSyntaxon: %v", err)
		}
		if err := tx.UpsertSpeciesRole(domain.SpeciesRole{
			Key: from, VerbatimName: "Arrhenatherum elatius", Role: "diagnostic", Fidelity: fidelity,
		}); err != nil {
			t.Fatalf("UpsertSpeciesRole: %v", err)
		}
		if err := tx.UpsertLocalization(domain.Localization{
			EntityType: "habitat_type", EntityKey: from.String(), Lang: "de", Field: "name",
			Value: "Glatthaferwiese", Source: "eunis-de", Provenance: "official",
		}); err != nil {
			t.Fatalf("UpsertLocalization: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}

	ingestOnce(domain.QualifierNarrower, nil)
	ingestOnce(domain.QualifierSame, floatPtr(0.8)) // repinned: qualifier and fidelity change

	assertCrosswalkAndSpeciesRoleAfterRepin(ctx, t, db, from, to)
	assertNoDuplicateRows(ctx, t, db)
}

func TestCrosswalksTo_ReturnsOnlyCrosswalksToTheGivenTypology(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	annex1 := domain.Crosswalk{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}
	version := domain.Crosswalk{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "eunis@2012", Code: "E22"},
		Qualifier: domain.QualifierNarrower,
	}
	if err := tx.UpsertCrosswalk(annex1); err != nil {
		t.Fatalf("UpsertCrosswalk(annex1): %v", err)
	}
	if err := tx.UpsertCrosswalk(version); err != nil {
		t.Fatalf("UpsertCrosswalk(version): %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.CrosswalksTo(ctx, "annex1")
	if err != nil {
		t.Fatalf("CrosswalksTo: %v", err)
	}
	if len(got) != 1 || got[0] != annex1 {
		t.Errorf("CrosswalksTo(annex1) = %+v, want exactly [%+v]", got, annex1)
	}
}

// Two documented guarantees rest on these two queries returning rows in a
// stable order: application.officialOrCuratedName promises byte-identical
// derived labels across repeated ingests, and DeriveGermanLabels' "first
// crosswalk wins" is order-dependent by design. Provenance ranking alone does
// not disambiguate two official rows from different sources, so the SQL, not
// sqlite's discretion, has to fix the order.
// The seed is chosen so the asserted order differs from the order the query
// plan would produce on its own: the WHERE pins the first four primary-key
// columns, so sqlite_autoindex_localization_1 hands rows back sorted by source
// (arge, bfn, eur-lex). Clustering by provenance first reorders them, so
// deleting the ORDER BY fails this test rather than passing by coincidence.
func TestLocalization_OrdersRowsByProvenanceThenSource(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, l := range []domain.Localization{
		// Two official rows from different sources: the case provenance ranking
		// alone cannot disambiguate, which is why the SQL has to.
		{Source: "arge", Provenance: "official"},
		{Source: "bfn", Provenance: "curated"},
		{Source: "eur-lex", Provenance: "official"},
	} {
		if err := tx.UpsertLocalization(domain.Localization{
			EntityType: "habitat_type", EntityKey: "annex1:6510", Lang: "de", Field: "name",
			Value: "Wiese laut " + l.Source, Source: l.Source, Provenance: l.Provenance,
		}); err != nil {
			t.Fatalf("UpsertLocalization(%s): %v", l.Source, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.Localization(ctx, "habitat_type", "annex1:6510", "de", "name")
	if err != nil {
		t.Fatalf("Localization: %v", err)
	}
	pairs := make([]string, 0, len(got))
	for _, l := range got {
		pairs = append(pairs, l.Provenance+"/"+l.Source)
	}
	// Index order would be arge, bfn, eur-lex; provenance clusters curated first.
	want := []string{"curated/bfn", "official/arge", "official/eur-lex"}
	if !slices.Equal(pairs, want) {
		t.Errorf("Localization order = %q, want %q (ORDER BY provenance, source)", pairs, want)
	}
}

// Same construction: idx_crosswalk_to(to_typology, to_code) drives the WHERE, so
// without the ORDER BY the rows arrive sorted by to_code (R11->6110, R99->6130,
// R11->6520). Ordering by from_code first moves R99 last, so the clause is
// actually guarded here too.
func TestCrosswalksTo_OrdersRowsDeterministically(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, c := range []domain.Crosswalk{
		{From: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R99"},
			To: domain.HabitatTypeKey{Typology: "annex1", Code: "6130"}, Qualifier: domain.QualifierApproximate},
		{From: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R11"},
			To: domain.HabitatTypeKey{Typology: "annex1", Code: "6520"}, Qualifier: domain.QualifierSame},
		// Same from_code as the row above, so the to_code tie-break is exercised.
		{From: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R11"},
			To: domain.HabitatTypeKey{Typology: "annex1", Code: "6110"}, Qualifier: domain.QualifierSame},
	} {
		if err := tx.UpsertCrosswalk(c); err != nil {
			t.Fatalf("UpsertCrosswalk(%+v): %v", c, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.CrosswalksTo(ctx, "annex1")
	if err != nil {
		t.Fatalf("CrosswalksTo: %v", err)
	}
	pairs := make([]string, 0, len(got))
	for _, c := range got {
		pairs = append(pairs, c.From.Code+"->"+c.To.Code)
	}
	want := []string{"R11->6110", "R11->6520", "R99->6130"}
	if !slices.Equal(pairs, want) {
		t.Errorf("CrosswalksTo order = %q, want %q (ORDER BY from_typology, from_code, to_code)", pairs, want)
	}
}

func TestCrosswalksTo_QueryErrorIsReturned(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := db.CrosswalksTo(t.Context(), "annex1"); err == nil {
		t.Fatal("CrosswalksTo on a closed database = nil error, want an error")
	}
}

func TestLocalization_QueryErrorIsReturned(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := db.Localization(t.Context(), "habitat_type", "annex1:6510", "de", "name"); err == nil {
		t.Fatal("Localization on a closed database = nil error, want an error")
	}
}

// The rows.Err() and Scan error returns of CrosswalksTo and Localization are
// exercised deterministically via a stub driver (see stub_driver_test.go)
// instead of racing a context cancellation against row iteration.
func TestCrosswalksTo_RowsIterationErrorIsReturned(t *testing.T) {
	db := &DB{DB: newStubDB(t, stubModeRowsErr)}
	if _, err := db.CrosswalksTo(context.Background(), "annex1"); err == nil {
		t.Fatal("CrosswalksTo with a rows-iteration error = nil error, want an error")
	} else if !strings.Contains(err.Error(), "reading crosswalks") {
		t.Errorf("error = %q, want it to name the rows.Err() failure", err)
	}
}

func TestCrosswalksTo_ScanErrorIsReturned(t *testing.T) {
	db := &DB{DB: newStubDB(t, stubModeScanErr)}
	if _, err := db.CrosswalksTo(context.Background(), "annex1"); err == nil {
		t.Fatal("CrosswalksTo with a scan error = nil error, want an error")
	} else if !strings.Contains(err.Error(), "scanning crosswalk") {
		t.Errorf("error = %q, want it to name the Scan failure", err)
	}
}

func TestLocalization_RowsIterationErrorIsReturned(t *testing.T) {
	db := &DB{DB: newStubDB(t, stubModeRowsErr)}
	if _, err := db.Localization(context.Background(), "habitat_type", "annex1:6510", "de", "name"); err == nil {
		t.Fatal("Localization with a rows-iteration error = nil error, want an error")
	} else if !strings.Contains(err.Error(), "reading localization") {
		t.Errorf("error = %q, want it to name the rows.Err() failure", err)
	}
}

func TestLocalization_ScanErrorIsReturned(t *testing.T) {
	db := &DB{DB: newStubDB(t, stubModeScanErr)}
	if _, err := db.Localization(context.Background(), "habitat_type", "annex1:6510", "de", "name"); err == nil {
		t.Fatal("Localization with a scan error = nil error, want an error")
	} else if !strings.Contains(err.Error(), "scanning localization") {
		t.Errorf("error = %q, want it to name the Scan failure", err)
	}
}

func TestLocalization_ReturnsEveryRowForTheKeyAndOmitsOthers(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	key := domain.HabitatTypeKey{Typology: "annex1", Code: "6510"}
	official := domain.Localization{
		EntityType: "habitat_type", EntityKey: key.String(), Lang: "de", Field: "name",
		Value: "Magere Flachland-Mähwiesen", Source: "ffh-richtlinie-de", Provenance: "official",
	}
	if err := tx.UpsertLocalization(official); err != nil {
		t.Fatalf("UpsertLocalization(official): %v", err)
	}
	// A different field on the same entity must not be returned.
	if err := tx.UpsertLocalization(domain.Localization{
		EntityType: "habitat_type", EntityKey: key.String(), Lang: "de", Field: "description",
		Value: "irrelevant", Source: "other", Provenance: "curated",
	}); err != nil {
		t.Fatalf("UpsertLocalization(other field): %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.Localization(ctx, "habitat_type", key.String(), "de", "name")
	if err != nil {
		t.Fatalf("Localization: %v", err)
	}
	if len(got) != 1 || got[0] != official {
		t.Errorf("Localization(name) = %+v, want exactly [%+v]", got, official)
	}
}

func assertCrosswalkAndSpeciesRoleAfterRepin(
	ctx context.Context, t *testing.T, db *DB, from, to domain.HabitatTypeKey,
) {
	t.Helper()

	var qualifier string
	if err := db.QueryRowContext(ctx,
		"SELECT qualifier FROM habitat_type_crosswalk WHERE from_typology = ? AND from_code = ? AND to_typology = ? AND to_code = ?",
		string(from.Typology), from.Code, string(to.Typology), to.Code,
	).Scan(&qualifier); err != nil {
		t.Fatalf("querying crosswalk: %v", err)
	}
	if qualifier != string(domain.QualifierSame) {
		t.Errorf("qualifier = %q, want %q (the later ingest wins)", qualifier, domain.QualifierSame)
	}

	var conceptID *string
	var fidelity *float64
	if err := db.QueryRowContext(ctx,
		"SELECT concept_id, fidelity FROM species_role WHERE typology_id = ? AND code = ? AND verbatim_name = ? AND role = ?",
		string(from.Typology), from.Code, "Arrhenatherum elatius", "diagnostic",
	).Scan(&conceptID, &fidelity); err != nil {
		t.Fatalf("querying species_role: %v", err)
	}
	if conceptID != nil {
		t.Errorf("concept_id = %v, want nil (unresolved verbatim name)", *conceptID)
	}
	if fidelity == nil || *fidelity != 0.8 {
		t.Errorf("fidelity = %v, want 0.8 (the later ingest wins)", fidelity)
	}
}

func assertNoDuplicateRows(ctx context.Context, t *testing.T, db *DB) {
	t.Helper()

	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habitat_type_crosswalk").Scan(&n); err != nil {
		t.Fatalf("counting habitat_type_crosswalk: %v", err)
	}
	if n != 1 {
		t.Errorf("habitat_type_crosswalk rows = %d, want 1 (upsert, not insert)", n)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habitat_type_syntaxon").Scan(&n); err != nil {
		t.Fatalf("counting habitat_type_syntaxon: %v", err)
	}
	if n != 1 {
		t.Errorf("habitat_type_syntaxon rows = %d, want 1 (link is idempotent)", n)
	}
}

func floatPtr(f float64) *float64 { return &f }

// Every Upsert, Commit and Rollback wraps the driver error instead of
// swallowing it — exercised here via a transaction that is already closed.
func TestIngestTx_MethodsWrapErrorsOnAClosedTransaction(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	cases := map[string]func() error{
		"UpsertTypology":    func() error { return tx.UpsertTypology(domain.Typology{ID: "eunis@2021"}) },
		"UpsertHabitatType": func() error { return tx.UpsertHabitatType(domain.HabitatType{Key: key}) },
		"UpsertCrosswalk": func() error {
			return tx.UpsertCrosswalk(domain.Crosswalk{From: key, To: key, Qualifier: domain.QualifierSame})
		},
		"UpsertSyntaxon": func() error { return tx.UpsertSyntaxon(domain.Syntaxon{ID: "x", Rank: "class", Name: "x"}) },
		"LinkSyntaxon":   func() error { return tx.LinkSyntaxon(key, "x") },
		"UpsertSpeciesRole": func() error {
			return tx.UpsertSpeciesRole(domain.SpeciesRole{Key: key, VerbatimName: "x", Role: "diagnostic"})
		},
		"UpsertLocalization": func() error {
			return tx.UpsertLocalization(domain.Localization{EntityType: "habitat_type", EntityKey: "x", Lang: "de", Field: "name", Value: "x", Source: "x", Provenance: "official"})
		},
		"Commit":   func() error { return tx.Commit() },
		"Rollback": func() error { return tx.Rollback() },
	}
	for name, call := range cases {
		err := call()
		if err == nil {
			t.Errorf("%s on a closed transaction = nil error, want an error", name)
			continue
		}
		// Wrapping, not just failing: the driver's cause must stay reachable
		// with errors.Is, and the message must say which operation failed.
		if !errors.Is(err, sql.ErrTxDone) {
			t.Errorf("%s error = %v, want it to wrap sql.ErrTxDone", name, err)
		}
		if !strings.HasPrefix(err.Error(), "sqlite: ") {
			t.Errorf("%s error = %q, want the adapter's own context prefixed", name, err)
		}
	}
}
