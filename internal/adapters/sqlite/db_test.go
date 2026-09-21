package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jobrunner/situs/internal/adapters/sqlite"
	"github.com/jobrunner/situs/internal/domain"
)

func TestOpenInMemoryIndexIsUsable(t *testing.T) {
	db, err := sqlite.OpenForIngest(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("OpenForIngest(:memory:) = %v, want no error", err)
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
	if _, err := sqlite.OpenForIngest(t.Context(), t.TempDir()); err == nil {
		t.Error("OpenForIngest(<directory>) = nil error, want a failure")
	}
}

// createPreMigrationSpeciesRoleTable creates the species_role shape from
// before provenance/derived_from existed, bypassing schema.sql, so a test
// can simulate a repinned index.
func createPreMigrationSpeciesRoleTable(t *testing.T, path string) {
	t.Helper()
	raw, err := sql.Open(sqlite.DriverName, path)
	if err != nil {
		t.Fatalf("opening raw connection: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE species_role (
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
}

// Open alone must never write to the index — serve is read-only and may sit
// on read-only media, so Open on a pre-migration index must still succeed;
// only a query touching the new columns is expected to fail.
func TestOpenAloneDoesNotMigrateAnOlderSchema(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "pre-provenance.sqlite")
	createPreMigrationSpeciesRoleTable(t, path)

	db, err := sqlite.OpenForIngest(ctx, path)
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
	}); err == nil {
		t.Error("UpsertSpeciesRole without Migrate = nil error, want 'no such column' — Open must not have migrated")
	}
}

// Migrate, called explicitly (as cmd/situs ingest does right after Open),
// adds the missing columns and makes the index usable again.
func TestMigrateAddsSpeciesRoleProvenanceColumnsToAnOlderSchema(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "pre-provenance.sqlite")
	createPreMigrationSpeciesRoleTable(t, path)

	db, err := sqlite.OpenForIngest(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate on a pre-migration index = %v, want no error", err)
	}

	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertSpeciesRole(domain.SpeciesRole{
		Key: key, VerbatimName: "Bromus erectus", Role: "diagnostic", Provenance: "observed",
	}); err != nil {
		t.Fatalf("UpsertSpeciesRole after Migrate = %v, want the new columns to exist", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.SpeciesRoles(ctx, key, "")
	if err != nil {
		t.Fatalf("SpeciesRoles after Migrate: %v", err)
	}
	if len(got) != 1 || got[0].Provenance != "observed" {
		t.Errorf("SpeciesRoles = %+v, want one row with Provenance observed", got)
	}
}

// Migrate must be idempotent on an already-migrated index — a second ingest
// run against the same file must not fail on "duplicate column".
func TestMigrateTwiceOnTheSameFileDoesNotFail(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "reopened.sqlite")

	db, err := sqlite.OpenForIngest(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate = %v, want the already-present columns to be a no-op", err)
	}
}

func TestMigrateFuegtSyntaxonSpaltenHinzu(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "old.sqlite")
	db, err := sqlite.OpenForIngest(ctx, path)
	if err != nil {
		t.Fatalf("OpenForIngest: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// An index that does not yet have the four columns: create it fresh
	// without them.
	if _, err := db.ExecContext(ctx, `DROP TABLE syntaxon`); err != nil {
		t.Fatalf("DROP: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`CREATE TABLE syntaxon (id TEXT PRIMARY KEY, rank TEXT NOT NULL, name TEXT NOT NULL,
		 author TEXT NOT NULL DEFAULT '', parent_id TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("CREATE: %v", err)
	}

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, col := range []string{"alt_code", "source", "parent_provenance", "life_form_group"} {
		var n int
		row := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pragma_table_info('syntaxon') WHERE name = ?`, col)
		if err := row.Scan(&n); err != nil {
			t.Fatalf("pragma_table_info(%s): %v", col, err)
		}
		if n != 1 {
			t.Errorf("Spalte %s fehlt nach Migrate", col)
		}
	}
}

func TestMigrateIstIdempotentFuerSyntaxon(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "twice.sqlite")
	db, err := sqlite.OpenForIngest(ctx, path)
	if err != nil {
		t.Fatalf("OpenForIngest: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for i := range 2 {
		if err := db.Migrate(ctx); err != nil {
			t.Fatalf("Migrate Durchlauf %d: %v", i+1, err)
		}
	}
}
