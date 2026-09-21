// open.go holds the two entry points into the same index. Which one a caller
// takes is an architectural decision, not a convenience: only the ingest may
// write, and serving has to survive a read-only file, a read-only directory
// and an index that is replaced underneath it.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	sqlitedriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// OpenForIngest opens the index at path read-write, verifies it is reachable
// and applies the embedded schema.sql — CREATE TABLE/INDEX IF NOT EXISTS
// statements, so on an already-current schema they are a no-op. It is the
// ingest's opener and the only one that may create an index; serve uses
// OpenReadOnly, which refuses a file that is not there.
//
// What it never does is an ALTER TABLE migration: call Migrate right after it
// to add columns an older index lacks.
func OpenForIngest(ctx context.Context, path string) (*DB, error) {
	dsn := fileURI(path)
	sqlDB, err := sql.Open(DriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite index %q: %w", path, err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("pinging sqlite index %q: %w", path, err), sqlDB.Close())
	}
	// WAL is an ingest-time choice: it keeps the write path from rewriting a
	// rollback journal per transaction. FinalizeForServing takes it back out
	// again, because a served index must not need the -wal/-shm sidecars.
	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON"} {
		if _, err := sqlDB.ExecContext(ctx, pragma); err != nil {
			return nil, errors.Join(fmt.Errorf("applying %q to %q: %w", pragma, path, err), sqlDB.Close())
		}
	}
	if _, err := sqlDB.ExecContext(ctx, schema); err != nil {
		return nil, errors.Join(fmt.Errorf("applying schema to %q: %w", path, err), sqlDB.Close())
	}
	return &DB{DB: sqlDB}, nil
}

// OpenReadOnly opens the index at path for serving. Three properties follow
// from `mode=ro`, and all three are what the serve path needs:
//
//   - Nothing can write. Not a stray statement, not a schema application, not
//     a journal-mode switch — SQLite refuses them at the file handle.
//   - A missing index is an error instead of a new, empty one. The read-write
//     opener would create it, and the service would answer NOT_FOUND for every
//     query behind a green /health/ready.
//   - No -wal/-shm sidecars are created, provided the ingest left the index out
//     of WAL (FinalizeForServing does). The served index is then a single file
//     that can be replaced underneath a running container, and the directory
//     itself may be read-only.
//
// Deliberately NOT set: `immutable=1`. It would switch off SQLite's own change
// detection, and that detection is the second line of defense when someone
// replaces the file while this handle is open.
func OpenReadOnly(ctx context.Context, path string) (*DB, error) {
	sqlDB, err := sql.Open(DriverName, fileURI(path)+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite index %q read-only: %w", path, err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, errors.Join(
			fmt.Errorf("opening sqlite index %q read-only%s: %w", path, readOnlyHint(err), err),
			sqlDB.Close())
	}
	// A ping is not enough. An empty file, a half-finished copy, a foreign
	// database and the anonymous temporary database of an empty path all open
	// without complaint, and the service would then come up green and answer
	// INTERNAL_ERROR for every query.
	if err := verifyServeSchema(ctx, sqlDB); err != nil {
		return nil, errors.Join(fmt.Errorf("sqlite index %q is not usable: %w", path, err), sqlDB.Close())
	}
	return &DB{DB: sqlDB}, nil
}

// readOnlyHint turns the two SQLite result codes an operator actually meets
// into the next step. Both are failures of the environment, not of the query,
// and both look alike from the outside: the service does not come up.
//
//   - SQLITE_CANTOPEN — the file is not there, or the process may not read it.
//   - SQLITE_READONLY_DIRECTORY — the directory cannot be written to, and
//     SQLite wants to write: the index still carries WAL mode, so even a pure
//     reader needs the -shm sidecar. An index built before this release is
//     exactly that, and re-running the ingest finalizes it.
func readOnlyHint(err error) string {
	var sqliteErr *sqlitedriver.Error
	if !errors.As(err, &sqliteErr) {
		return ""
	}
	switch sqliteErr.Code() {
	case sqlite3.SQLITE_CANTOPEN:
		return " (no such file, or not readable by this process)"
	case sqlite3.SQLITE_READONLY_DIRECTORY:
		return " (the directory is not writable and this index still carries WAL mode — re-run `situs ingest` with this release to finalize it, or mount the directory writable)"
	default:
		return ""
	}
}

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

// uriEscaper turns the three characters that mean something else inside a
// SQLite URI into their percent-escapes. '%' has to go first, or it would
// escape the escapes.
//
// This is not cosmetic. Measured against modernc.org/sqlite v1.56.0, an
// unescaped path is silently truncated at its first '?' — the driver splits
// the DSN there — so `situs ingest --db '/srv/with?chars/index.sqlite'` built
// and populated a file called `/srv/with` and reported success, and a
// read-only open lost `mode=ro` with the rest of the query string and came
// back WRITABLE. A '%' is the mirror image: SQLite percent-decodes the URI
// filename, so `/srv/feature%2Fx/index.sqlite` resolved to a path that does
// not exist. Directory names like `feature%2Fbranch` are what CI checkouts
// produce, so this is not an exotic case.
var uriEscaper = strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23")

// fileURI turns a filesystem path into a SQLite URI filename. ":memory:" is
// passed through: it is not a path, and the URI form of it means something
// subtly different.
func fileURI(path string) string {
	if path == ":memory:" {
		return path
	}
	return "file:" + uriEscaper.Replace(path)
}

// FinalizeForServing leaves the index as a single file: the WAL is checkpointed
// into the database and the journal mode is switched back to DELETE.
//
// The ingest is the only place that can do this — the switch out of WAL needs
// to be the sole connection to the database, which no serving process can
// promise. Without it the index still *works* when served read-only, but
// SQLite then needs to create the -shm sidecar even for a pure reader, which
// takes a writable directory and makes replacing the file a multi-file affair.
func (d *DB) FinalizeForServing(ctx context.Context) error {
	// Leaving WAL fails while another connection to the same database is open,
	// and an ingest has used several. Dropping the idle limit to zero closes
	// the pooled ones. Deliberately NOT SetMaxOpenConns(1): if some connection
	// were still checked out, that would make the Conn call below block until
	// the context is done — the ingest would hang at its very last step after
	// hours of work instead of reporting what is wrong. The journal_mode
	// assertion further down catches that case and names it.
	d.SetMaxIdleConns(0)

	conn, err := d.Conn(ctx)
	if err != nil {
		return fmt.Errorf("sqlite: taking the sole connection to finalize: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var busy, logFrames, checkpointed int
	if err := conn.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).
		Scan(&busy, &logFrames, &checkpointed); err != nil {
		return fmt.Errorf("sqlite: checkpointing the WAL: %w", err)
	}
	if busy != 0 {
		return fmt.Errorf("sqlite: the WAL could not be checkpointed, another connection is reading the index")
	}

	// journal_mode answers with the mode that is now in force rather than
	// failing, so the answer is the assertion.
	var mode string
	if err := conn.QueryRowContext(ctx, `PRAGMA journal_mode=DELETE`).Scan(&mode); err != nil {
		return fmt.Errorf("sqlite: leaving WAL mode: %w", err)
	}
	if !strings.EqualFold(mode, "delete") {
		return fmt.Errorf("sqlite: journal mode after finalizing is %q, want delete", mode)
	}
	return nil
}
