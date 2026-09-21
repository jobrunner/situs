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
	"io"
	"os"
	"path/filepath"
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
			fmt.Errorf("opening sqlite index %q read-only%s: %w", path, readOnlyHint(path, err), err),
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

// readOnlyHint explains a failed read-only open, for the two result codes that
// mean "the environment is wrong" rather than "the query is wrong".
//
// Which of the two arrives says nothing about the cause — the same broken index
// answers SQLITE_READONLY_DIRECTORY from a chmod'ed directory and
// SQLITE_CANTOPEN from a read-only bind mount (measured against the 0.11.0
// image with /db mounted :ro). So the code only decides WHETHER to explain;
// what is wrong is read from the file itself.
func readOnlyHint(path string, err error) string {
	var sqliteErr *sqlitedriver.Error
	if !errors.As(err, &sqliteErr) {
		return ""
	}
	switch sqliteErr.Code() {
	case sqlite3.SQLITE_CANTOPEN, sqlite3.SQLITE_READONLY_DIRECTORY:
		return indexFileHint(path)
	default:
		return ""
	}
}

// sqliteMagic opens every SQLite database file, and walWriteVersion is what
// byte 18 of that header carries while the database is in WAL mode. Both are
// part of the on-disk format, which SQLite keeps stable by contract.
const (
	sqliteMagic      = "SQLite format 3\x00"
	sqliteHeaderPeek = 20
	walWriteVersion  = 2
)

// indexFileHint looks at the file and says what is wrong with it. The WAL case
// is the one that matters in practice: an index built before 0.11.0 is still in
// WAL mode, a read-only handle on it needs the -shm sidecar, and a read-only
// mount cannot have one. That failure looks exactly like a missing file from
// the outside, and guessing "no such file" sends an operator to check
// permissions that are perfectly fine.
func indexFileHint(path string) string {
	header, err := peekHeader(path)
	if err != nil {
		return " (no such file, or not readable by this process)"
	}
	if len(header) < sqliteHeaderPeek || string(header[:len(sqliteMagic)]) != sqliteMagic {
		return " (not a SQLite database)"
	}
	if header[18] == walWriteVersion {
		return " (this index is still in WAL mode, and a read-only handle on it needs a -shm sidecar it cannot create here" +
			" — rebuild it with `situs ingest` of this release, or finalize the file in place where it is writable:" +
			" sqlite3 <index> 'PRAGMA wal_checkpoint(TRUNCATE); PRAGMA journal_mode=DELETE;')"
	}
	return " (the file is readable and finalized, so the directory itself must be unreadable or not traversable for this process)"
}

// peekHeader reads the first bytes of path through an os.Root confined to its
// directory — the same confinement the CSV ingest uses.
func peekHeader(path string) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	f, err := root.Open(filepath.Base(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	header := make([]byte, sqliteHeaderPeek)
	n, err := io.ReadFull(f, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return header[:n], nil
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
