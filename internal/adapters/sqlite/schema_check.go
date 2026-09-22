// schema_check.go answers one question: is the file that was just opened an
// index this binary can serve? Serving no longer applies schema.sql — the
// read-only handle could not — so the check that used to happen implicitly on
// every start has to happen explicitly here.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// schemaTables is the table list schema.sql actually creates, read from the
// embedded statements rather than repeated here. A hand-kept copy would drift
// the first time someone adds a table, and it would drift silently — the check
// below would keep passing.
var schemaTables = regexp.MustCompile(`CREATE TABLE IF NOT EXISTS ([a-z_]+)`)

// migratedColumns are the columns Migrate adds to an existing index, each with
// the static PRAGMA that reads its table. They are where version skew actually
// shows up: an index from 0.8.0 carries every table but not
// habitat_description.provenance, and a 0.9.0 binary serving it answers every
// habitat-type request with INTERNAL_ERROR. Checking tables alone would let
// that through.
//
// The PRAGMA is spelled out per entry rather than built from the table name:
// every statement in this package is a literal, and PRAGMA takes no bound
// parameter.
var migratedColumns = []struct {
	table   string
	pragma  string
	columns []string
}{
	{"species_role", `PRAGMA table_info(species_role)`, []string{"provenance", "derived_from"}},
	{"habitat_description", `PRAGMA table_info(habitat_description)`, []string{"provenance"}},
	{"syntaxon", `PRAGMA table_info(syntaxon)`, []string{"eea_code", "source", "parent_provenance", "life_form_group"}},
}

// verifyServeSchema reports whether the opened file is an index this binary can
// serve. It is deliberately more than a "does one table exist" probe: a foreign
// SQLite file may well have a habitat_type table of its own.
func verifyServeSchema(ctx context.Context, db *sql.DB) error {
	if err := verifyTables(ctx, db); err != nil {
		return err
	}
	return verifyMigratedColumns(ctx, db)
}

// verifyTables checks that every table schema.sql creates is there.
func verifyTables(ctx context.Context, db *sql.DB) error {
	present, err := tableNames(ctx, db)
	if err != nil {
		return err
	}
	missing := []string{}
	for _, m := range schemaTables.FindAllStringSubmatch(schema, -1) {
		if !present[m[1]] {
			missing = append(missing, m[1])
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"missing table(s) %s — an empty file, an interrupted copy, an index from an older release, or not a situs index at all; run `situs ingest` with this release",
			strings.Join(missing, ", "))
	}
	return nil
}

// tableNames reads the table names the opened file actually carries.
func tableNames(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		return nil, fmt.Errorf("reading the table list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	present := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scanning the table list: %w", err)
		}
		present[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating the table list: %w", err)
	}
	return present, nil
}

// verifyMigratedColumns checks the columns an older index is missing even when
// every table is there.
func verifyMigratedColumns(ctx context.Context, db *sql.DB) error {
	for _, t := range migratedColumns {
		columns, err := tableColumns(ctx, db, t.pragma)
		if err != nil {
			return fmt.Errorf("reading %s columns: %w", t.table, err)
		}
		for _, want := range t.columns {
			if !columns[want] {
				return fmt.Errorf(
					"%s has no %s column — the index predates this release; run `situs ingest` with it",
					t.table, want)
			}
		}
	}
	return nil
}
