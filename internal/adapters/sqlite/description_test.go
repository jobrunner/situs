package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

var r22key = domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}

func writeDescription(t *testing.T, db *DB, d domain.HabitatDescription) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertDescription(d); err != nil {
		t.Fatalf("UpsertDescription: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestDescription_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	want := domain.HabitatDescription{
		Key:    r22key,
		TextEN: "Meadows of lowland and montane areas.",
		Source: "floraveg:factsheets:2021-06-01",
	}
	writeDescription(t, db, want)

	got, err := db.Description(context.Background(), r22key)
	if err != nil {
		t.Fatalf("Description: %v", err)
	}
	if got != want {
		t.Errorf("description = %+v, want %+v", got, want)
	}
}

// A habitat type without a factsheet is the normal case — 264 of 7937 types
// carry one. That has to be a clean "not found", not an empty-but-present row.
func TestDescription_AbsentIsNotFound(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Description(context.Background(), r22key)
	if !errors.Is(err, output.ErrNotFound) {
		t.Errorf("Description of an undescribed type = %v, want ErrNotFound", err)
	}
}

// Same key in another typology is another habitat type entirely.
func TestDescription_IsScopedToTheTypology(t *testing.T) {
	db := openTestDB(t)
	writeDescription(t, db, domain.HabitatDescription{Key: r22key, TextEN: "Meadows."})

	_, err := db.Description(context.Background(),
		domain.HabitatTypeKey{Typology: "eunis@2012", Code: "R22"})
	if !errors.Is(err, output.ErrNotFound) {
		t.Errorf("Description in another typology = %v, want ErrNotFound", err)
	}
}

func TestUpsertDescription_IsIdempotentAndReplacesTheText(t *testing.T) {
	db := openTestDB(t)
	writeDescription(t, db, domain.HabitatDescription{Key: r22key, TextEN: "Old.", Source: "a"})
	writeDescription(t, db, domain.HabitatDescription{Key: r22key, TextEN: "New.", Source: "b"})

	got, err := db.Description(context.Background(), r22key)
	if err != nil {
		t.Fatalf("Description: %v", err)
	}
	if got.TextEN != "New." || got.Source != "b" {
		t.Errorf("description = %+v, want the re-ingested text and source", got)
	}
}

func TestUpsertDescription_WriteErrorIsReturned(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	err = tx.UpsertDescription(domain.HabitatDescription{Key: r22key, TextEN: "Meadows."})
	if err == nil {
		t.Fatal("UpsertDescription on a rolled-back transaction = nil error, want an error")
	}
	if !strings.HasPrefix(err.Error(), "sqlite: ") {
		t.Errorf("error = %q, want the adapter's own context prefixed", err)
	}
}

func TestDescription_QueryErrorIsReturned(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err := db.Description(context.Background(), r22key)
	if err == nil || errors.Is(err, output.ErrNotFound) {
		t.Errorf("Description on a closed database = %v, want a real error", err)
	}
	if !strings.HasPrefix(err.Error(), "sqlite: ") {
		t.Errorf("error = %q, want the adapter's own context prefixed", err)
	}
}

// Provenance travels with the text: the EUNIS descriptions are the official
// factsheet wording, the Annex I ones are written by situs. A reader must be
// able to tell which is which.
func TestDescription_CarriesProvenance(t *testing.T) {
	db := openTestDB(t)
	want := domain.HabitatDescription{
		Key:    domain.HabitatTypeKey{Typology: "annex1", Code: "6230"},
		TextEN: "Closed swards on acidic soils.", Source: "situs@0.8.0",
		Provenance: domain.DescriptionProvenanceSitus,
	}
	writeDescription(t, db, want)

	got, err := db.Description(context.Background(), want.Key)
	if err != nil {
		t.Fatalf("Description: %v", err)
	}
	if got != want {
		t.Errorf("description = %+v, want %+v", got, want)
	}
}

// An index written before the column existed must not break: the ingest adds
// it, and rows written earlier are official factsheet text.
func TestMigrate_AddsDescriptionProvenanceToAnOlderIndex(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `DROP TABLE habitat_description`); err != nil {
		t.Fatalf("dropping the table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE habitat_description (
		typology_id TEXT NOT NULL, code TEXT NOT NULL,
		description_en TEXT NOT NULL, source TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (typology_id, code))`); err != nil {
		t.Fatalf("creating the old table: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO habitat_description VALUES ('eunis@2021','R22','Meadows.','floraveg')`); err != nil {
		t.Fatalf("seeding a pre-migration row: %v", err)
	}

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	got, err := db.Description(ctx, domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"})
	if err != nil {
		t.Fatalf("Description after Migrate: %v", err)
	}
	if got.Provenance != domain.DescriptionProvenanceOfficial {
		t.Errorf("provenance = %q, want %q for a row written before the column existed",
			got.Provenance, domain.DescriptionProvenanceOfficial)
	}
}

// Both halves of the migration have to surface their failure: the caller runs
// this right before an ingest, and a silently half-migrated index fails later
// with a "no such column" nobody can trace back.
func TestMigrateOnAClosedDatabaseFailsForEachStep(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(*DB) error{
		"species_role":        func(db *DB) error { return db.Migrate(ctx) },
		"habitat_description": func(db *DB) error { return db.Migrate(ctx) },
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			db := openTestDB(t)
			if err := db.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if err := call(db); err == nil {
				t.Fatal("Migrate on a closed database = nil error, want an error")
			}
		})
	}
}

// The ALTER TABLE itself can fail — here because the table is gone. The error
// must name the column it was adding, not just "migrating schema".
func TestMigrateReportsWhichColumnItFailedToAdd(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `DROP TABLE habitat_description`); err != nil {
		t.Fatalf("dropping the table: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`CREATE VIEW habitat_description AS SELECT 1 AS typology_id`); err != nil {
		t.Fatalf("creating the view: %v", err)
	}

	err := db.Migrate(ctx)
	if err == nil {
		t.Fatal("Migrate against a view = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "habitat_description.provenance") {
		t.Errorf("error = %q, want it to name the column", err)
	}
}
