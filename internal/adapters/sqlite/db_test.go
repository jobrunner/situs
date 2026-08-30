package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jobrunner/situs/internal/adapters/sqlite"
	"github.com/jobrunner/situs/internal/domain"
)

func TestOpenInMemoryIndexIsUsable(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) = %v, want no error", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing index: %v", err)
		}
	})

	var got int
	if err := db.QueryRow("SELECT 1").Scan(&got); err != nil {
		t.Fatalf("querying the opened index: %v", err)
	}
	if got != 1 {
		t.Errorf("SELECT 1 = %d, want 1", got)
	}
}

func TestOpenUnreachablePathFails(t *testing.T) {
	// A directory can never be opened as a database file.
	if _, err := sqlite.Open(t.Context(), t.TempDir()); err == nil {
		t.Error("Open(<directory>) = nil error, want a failure")
	}
}

// A repinned index created before species_role.provenance/derived_from
// existed must not fail ingest with "no such column" — Open must add the
// missing columns to the existing table, not just declare them in
// CREATE TABLE IF NOT EXISTS (which never touches an already-existing table).
func TestOpenAddsSpeciesRoleProvenanceColumnsToAnOlderSchema(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "pre-provenance.sqlite")

	// Simulate the pre-this-feature schema directly, bypassing schema.sql.
	raw, err := sql.Open(sqlite.DriverName, path)
	if err != nil {
		t.Fatalf("opening raw connection: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `CREATE TABLE species_role (
		typology_id   TEXT NOT NULL,
		code          TEXT NOT NULL,
		concept_id    TEXT,
		verbatim_name TEXT NOT NULL,
		role          TEXT NOT NULL,
		fidelity      REAL,
		constancy     REAL,
		PRIMARY KEY (typology_id, code, verbatim_name, role)
	)`); err != nil {
		t.Fatalf("creating the pre-migration species_role table: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("closing raw connection: %v", err)
	}

	db, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open on a pre-migration index = %v, want no error", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertSpeciesRole(domain.SpeciesRole{
		Key: key, VerbatimName: "Bromus erectus", Role: "diagnostic", Provenance: "observed",
	}); err != nil {
		t.Fatalf("UpsertSpeciesRole after migration = %v, want the new columns to exist", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.SpeciesRoles(ctx, key, "")
	if err != nil {
		t.Fatalf("SpeciesRoles after migration: %v", err)
	}
	if len(got) != 1 || got[0].Provenance != "observed" {
		t.Errorf("SpeciesRoles = %+v, want one row with Provenance observed", got)
	}
}

// Open must be idempotent on an already-migrated index: calling it twice
// against the same file must not fail on "duplicate column".
func TestOpenTwiceOnTheSameFileDoesNotFailMigration(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "reopened.sqlite")

	db1, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("closing first connection: %v", err)
	}

	db2, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open = %v, want the already-present columns to be a no-op", err)
	}
	t.Cleanup(func() { _ = db2.Close() })
}
