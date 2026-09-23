package sqlite

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// LocalizationsByEntityType is what keeps a list route from issuing one query
// per row: GET /v1/syntaxa?lang=de lists 1326 alliances, and the 25 formation
// labels behind them are one table scan, not 1326 lookups.
func TestLocalizationsByEntityType_BucketsByKeyAndKeepsTheTotalOrder(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, l := range []domain.Localization{
		{EntityType: "syntaxon", EntityKey: "P", Field: "name",
			Value: "Vegetation der Hoch- und Niedermoore", Source: "situs", Provenance: "situs"},
		{EntityType: "syntaxon", EntityKey: "P", Field: "vernacular",
			Value: "Moorvegetation", Source: "situs", Provenance: "situs"},
		{EntityType: "syntaxon", EntityKey: "A", Field: "name",
			Value: "Vegetation der arktischen Zone", Source: "situs", Provenance: "situs"},
		// A row of another entity type must not leak into the syntaxon bucket:
		// the two share one table and only the column keeps them apart.
		{EntityType: "habitat_type", EntityKey: "annex1:6510", Field: "name",
			Value: "Magere Flachland-Maehwiesen", Source: "eur-lex", Provenance: "official"},
	} {
		l.Lang = "de"
		if err := tx.UpsertLocalization(l); err != nil {
			t.Fatalf("UpsertLocalization(%s/%s): %v", l.EntityType, l.EntityKey, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.LocalizationsByEntityType(ctx, "syntaxon", "de")
	if err != nil {
		t.Fatalf("LocalizationsByEntityType: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entity keys %v, want 2 (A and P)", len(got), got)
	}
	fields := make([]string, 0, len(got["P"]))
	for _, l := range got["P"] {
		fields = append(fields, l.Field)
	}
	// Same total order Localization promises: (provenance, field, source).
	if want := []string{"name", "vernacular"}; !slices.Equal(fields, want) {
		t.Errorf("fields of P = %q, want %q", fields, want)
	}
	if v := got["A"][0].Value; v != "Vegetation der arktischen Zone" {
		t.Errorf("A name = %q", v)
	}
	if _, leaked := got["annex1:6510"]; leaked {
		t.Error("a habitat_type row leaked into the syntaxon bucket")
	}
}

// An entity type the index has no rows for is an empty map, not an error: a
// language nobody translated yet is the normal case, not a defect.
func TestLocalizationsByEntityType_UnknownTypeIsEmpty(t *testing.T) {
	db := openTestDB(t)
	got, err := db.LocalizationsByEntityType(t.Context(), "syntaxon", "de")
	if err != nil {
		t.Fatalf("LocalizationsByEntityType: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want no rows", got)
	}
}

// The three failure paths, same construction as Localization's: a closed
// database for the query, and the stub driver for the two row-level ones.
func TestLocalizationsByEntityType_QueryErrorIsReturned(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := db.LocalizationsByEntityType(t.Context(), "syntaxon", "de"); err == nil {
		t.Fatal("LocalizationsByEntityType on a closed database = nil error, want an error")
	}
}

func TestLocalizationsByEntityType_RowsIterationErrorIsReturned(t *testing.T) {
	db := &DB{DB: newStubDB(t, stubModeRowsErr)}
	if _, err := db.LocalizationsByEntityType(context.Background(), "syntaxon", "de"); err == nil {
		t.Fatal("LocalizationsByEntityType with a rows-iteration error = nil error, want an error")
	} else if !strings.Contains(err.Error(), "reading localizations") {
		t.Errorf("error = %q, want it to name the rows.Err() failure", err)
	}
}

func TestLocalizationsByEntityType_ScanErrorIsReturned(t *testing.T) {
	db := &DB{DB: newStubDB(t, stubModeScanErr)}
	if _, err := db.LocalizationsByEntityType(context.Background(), "syntaxon", "de"); err == nil {
		t.Fatal("LocalizationsByEntityType with a scan error = nil error, want an error")
	} else if !strings.Contains(err.Error(), "scanning localizations") {
		t.Errorf("error = %q, want it to name the Scan failure", err)
	}
}
