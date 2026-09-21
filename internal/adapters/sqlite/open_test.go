package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqlitedriver "modernc.org/sqlite"
)

// seedIndex builds a usable index at path through the read-write opener and
// closes it again, so the read-only tests below start from what an ingest
// leaves behind.
func seedIndex(t *testing.T, path string) {
	t.Helper()
	db, err := OpenForIngest(t.Context(), path)
	if err != nil {
		t.Fatalf("OpenForIngest(%q) = %v, want no error", path, err)
	}
	if err := db.FinalizeForServing(t.Context()); err != nil {
		t.Fatalf("FinalizeForServing = %v, want no error", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close = %v, want no error", err)
	}
}

// chmodDir changes a directory's mode through an os.Root anchored at its
// parent. A directory needs its execute bit to stay traversable, which plain
// os.Chmod cannot express within the project's gosec rules — G302 rejects any
// mode carrying an execute bit — and a lint suppression is not available
// either: the budget for them is zero.
func chmodDir(t *testing.T, dir string, mode fs.FileMode) {
	t.Helper()
	root, err := os.OpenRoot(filepath.Dir(dir))
	if err != nil {
		t.Fatalf("opening the parent of %q: %v", dir, err)
	}
	defer func() { _ = root.Close() }()
	if err := root.Chmod(filepath.Base(dir), mode); err != nil {
		t.Fatalf("chmod %q to %o: %v", dir, mode, err)
	}
}

func TestOpenReadOnlyReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.sqlite")
	seedIndex(t, path)

	db, err := OpenReadOnly(t.Context(), path)
	if err != nil {
		t.Fatalf("OpenReadOnly = %v, want no error", err)
	}
	defer func() { _ = db.Close() }()

	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM habitat_type`).Scan(&n); err != nil {
		t.Fatalf("counting habitat types: %v", err)
	}
}

func TestOpenReadOnlyRefusesWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.sqlite")
	seedIndex(t, path)

	db, err := OpenReadOnly(t.Context(), path)
	if err != nil {
		t.Fatalf("OpenReadOnly = %v, want no error", err)
	}
	defer func() { _ = db.Close() }()

	_, err = db.ExecContext(t.Context(),
		`INSERT INTO habitat_typology (id, scheme, version, name) VALUES ('x', 'x', 'x', 'x')`)
	if err == nil {
		t.Fatal("INSERT through a read-only handle succeeded, want a readonly error")
	}
	if !strings.Contains(err.Error(), "readonly") {
		t.Errorf("INSERT error = %v, want it to name the read-only database", err)
	}
}

// The operational promise: a serving index is ONE file. No -wal, no -shm, and
// therefore nothing a container has to release before the file can be replaced.
func TestOpenReadOnlyLeavesNoSidecarFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.sqlite")
	seedIndex(t, path)

	db, err := OpenReadOnly(t.Context(), path)
	if err != nil {
		t.Fatalf("OpenReadOnly = %v, want no error", err)
	}
	defer func() { _ = db.Close() }()

	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM habitat_type`).Scan(&n); err != nil {
		t.Fatalf("counting habitat types: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %q: %v", dir, err)
	}
	for _, e := range entries {
		if e.Name() != "index.sqlite" {
			t.Errorf("read-only serving created %q — the index must stay a single file", e.Name())
		}
	}
}

// The same promise from the other side: the whole directory may be read-only.
func TestOpenReadOnlyWorksInAReadOnlyDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.sqlite")
	seedIndex(t, path)

	chmodDir(t, dir, 0o500)
	t.Cleanup(func() { chmodDir(t, dir, 0o700) })

	db, err := OpenReadOnly(t.Context(), path)
	if err != nil {
		t.Fatalf("OpenReadOnly in a read-only directory = %v, want no error", err)
	}
	defer func() { _ = db.Close() }()

	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM habitat_type`).Scan(&n); err != nil {
		t.Fatalf("counting habitat types in a read-only directory: %v", err)
	}
}

// Serving an index that is not there must fail loudly. The read-write opener
// would create an empty one and answer NOT_FOUND for everything behind a green
// /health/ready — an outage that looks like a data problem.
func TestOpenReadOnlyFailsOnMissingIndex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "absent.sqlite")

	db, err := OpenReadOnly(t.Context(), path)
	if err == nil {
		_ = db.Close()
		t.Fatal("OpenReadOnly on a missing index succeeded, want an error")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("OpenReadOnly created the missing index — serving must never conjure one")
	}
	if !strings.Contains(err.Error(), "no such file") {
		t.Errorf("error = %v, want it to say what an operator should check", err)
	}
}

// A file that opens but carries no index is the failure this switch exists to
// remove: an interrupted copy, a foreign database, or the anonymous temporary
// database an empty path produces. All three ping happily, and without a probe
// the service would come up green and answer INTERNAL_ERROR for every query.
func TestOpenReadOnlyRefusesAFileThatIsNotAnIndex(t *testing.T) {
	zeroByte := filepath.Join(t.TempDir(), "index.sqlite")
	if err := os.WriteFile(zeroByte, nil, 0o600); err != nil {
		t.Fatalf("writing the empty file: %v", err)
	}

	for _, tc := range []struct{ name, path string }{
		{"leere Datei", zeroByte},
		{"leerer Pfad", ""},
	} {
		db, err := OpenReadOnly(t.Context(), tc.path)
		if err == nil {
			_ = db.Close()
			t.Errorf("%s: OpenReadOnly succeeded, want the unusable index reported", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), "habitat_type") {
			t.Errorf("%s: error = %v, want it to name the missing table", tc.name, err)
		}
	}
}

// The upgrade case worth a sentence: an index built before this release still
// carries WAL mode, and a read-only handle on it needs the -shm sidecar. On a
// read-only mount that fails with a bare SQLITE_READONLY_DIRECTORY, which says
// nothing about what to do.
func TestOpenReadOnlyExplainsAWALIndexOnAReadOnlyMount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.sqlite")

	db, err := OpenForIngest(t.Context(), path) // no FinalizeForServing: stays in WAL
	if err != nil {
		t.Fatalf("OpenForIngest = %v, want no error", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close = %v, want no error", err)
	}
	chmodDir(t, dir, 0o500)
	t.Cleanup(func() { chmodDir(t, dir, 0o700) })

	ro, err := OpenReadOnly(t.Context(), path)
	if err == nil {
		_ = ro.Close()
		t.Fatal("OpenReadOnly on a WAL index in a read-only directory succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "situs ingest") {
		t.Errorf("error = %v, want it to name the ingest that finalizes the index", err)
	}
}

// A path is data, not URI syntax. Unescaped, modernc.org/sqlite truncates the
// DSN at the first '?' and SQLite percent-decodes '%' — so both openers have to
// escape, and both have to end up at the file the caller named.
func TestBothOpenersSurviveURICharactersInThePath(t *testing.T) {
	for _, name := range []string{"with?chars", "with#chars", "feature%2Fx", "plain"} {
		base := t.TempDir()
		dir := filepath.Join(base, name)
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatalf("mkdir %q: %v", dir, err)
		}
		path := filepath.Join(dir, "index.sqlite")

		seedIndex(t, path)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("OpenForIngest under %q did not create the index it was given: %v", name, err)
			continue
		}
		// Truncation at '?' would put the index one directory up, under a
		// name nobody asked for — and the ingest would report success.
		entries, err := os.ReadDir(base)
		if err != nil {
			t.Fatalf("reading %q: %v", base, err)
		}
		for _, e := range entries {
			if e.Name() != name {
				t.Errorf("OpenForIngest under %q also created %q", name, e.Name())
			}
		}

		db, err := OpenReadOnly(t.Context(), path)
		if err != nil {
			t.Errorf("OpenReadOnly under %q = %v, want no error", name, err)
			continue
		}
		var n int
		if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM habitat_type`).Scan(&n); err != nil {
			t.Errorf("counting habitat types under %q: %v", name, err)
		}
		_ = db.Close()
	}
}

// A space needs no escaping and must not get any: it has to keep working
// exactly as before.
func TestOpenReadOnlyAcceptsASpaceInThePath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "with space")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
	path := filepath.Join(dir, "index.sqlite")
	seedIndex(t, path)

	db, err := OpenReadOnly(t.Context(), path)
	if err != nil {
		t.Fatalf("OpenReadOnly(%q) = %v, want no error", path, err)
	}
	defer func() { _ = db.Close() }()

	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM habitat_type`).Scan(&n); err != nil {
		t.Fatalf("counting habitat types: %v", err)
	}
}

func TestFinalizeForServingLeavesTheIndexOutOfWAL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.sqlite")

	db, err := OpenForIngest(t.Context(), path)
	if err != nil {
		t.Fatalf("OpenForIngest = %v, want no error", err)
	}
	var mode string
	if err := db.QueryRowContext(t.Context(), `PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("reading journal_mode: %v", err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Fatalf("journal_mode during ingest = %q, want wal — the premise of this test is gone", mode)
	}

	if err := db.FinalizeForServing(t.Context()); err != nil {
		t.Fatalf("FinalizeForServing = %v, want no error", err)
	}
	if err := db.QueryRowContext(t.Context(), `PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("reading journal_mode after finalizing: %v", err)
	}
	if !strings.EqualFold(mode, "delete") {
		t.Errorf("journal_mode after FinalizeForServing = %q, want delete", mode)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close = %v, want no error", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %q: %v", dir, err)
	}
	for _, e := range entries {
		if e.Name() != "index.sqlite" {
			t.Errorf("FinalizeForServing left %q behind — the index must ship as a single file", e.Name())
		}
	}
}

// FinalizeForServing has to work after the pool has handed out more than one
// connection: leaving WAL needs every other connection gone, and the ingest
// runs many statements before it.
func TestFinalizeForServingAfterConcurrentConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.sqlite")
	db, err := OpenForIngest(t.Context(), path)
	if err != nil {
		t.Fatalf("OpenForIngest = %v, want no error", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	held := make([]*sql.Conn, 0, 4)
	for range 4 {
		c, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("opening a pooled connection: %v", err)
		}
		held = append(held, c)
	}
	for _, c := range held {
		if err := c.Close(); err != nil {
			t.Fatalf("returning a pooled connection: %v", err)
		}
	}

	if err := db.FinalizeForServing(ctx); err != nil {
		t.Fatalf("FinalizeForServing after several pooled connections = %v, want no error", err)
	}
}

// An in-memory index has no journal mode to switch, so finalizing one is a
// mistake the caller has to hear about rather than a silent no-op.
func TestFinalizeForServingRefusesAnIndexThatCannotLeaveWAL(t *testing.T) {
	db, err := OpenForIngest(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("OpenForIngest(:memory:) = %v, want no error", err)
	}
	defer func() { _ = db.Close() }()

	err = db.FinalizeForServing(t.Context())
	if err == nil {
		t.Fatal("FinalizeForServing on an in-memory index = nil, want an error")
	}
	if !strings.Contains(err.Error(), "want delete") {
		t.Errorf("error = %v, want it to name the journal mode it ended up with", err)
	}
}

// The checkpoint cannot truncate the WAL while another connection is reading
// the index, and a half-finalized index must not pass for a finished one.
func TestFinalizeForServingFailsWhileAnotherConnectionReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.sqlite")
	db, err := OpenForIngest(t.Context(), path)
	if err != nil {
		t.Fatalf("OpenForIngest = %v, want no error", err)
	}
	defer func() { _ = db.Close() }()

	reader, err := OpenForIngest(t.Context(), path)
	if err != nil {
		t.Fatalf("second OpenForIngest = %v, want no error", err)
	}
	defer func() { _ = reader.Close() }()

	tx, err := reader.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("beginning the competing read: %v", err)
	}
	var n int
	if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM habitat_type`).Scan(&n); err != nil {
		t.Fatalf("holding a read open: %v", err)
	}

	if err := db.FinalizeForServing(t.Context()); err == nil {
		t.Error("FinalizeForServing with a competing reader = nil, want an error")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("ending the competing read: %v", err)
	}
}

// A closed pool can hand out no connection; the error has to name what failed.
func TestFinalizeForServingOnAClosedIndexFails(t *testing.T) {
	db, err := OpenForIngest(t.Context(), filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatalf("OpenForIngest = %v, want no error", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close = %v, want no error", err)
	}

	err = db.FinalizeForServing(t.Context())
	if err == nil {
		t.Fatal("FinalizeForServing on a closed index = nil, want an error")
	}
	if !strings.Contains(err.Error(), "sole connection") {
		t.Errorf("error = %v, want it to name the connection it could not take", err)
	}
}

// The two FinalizeForServing branches that need a database answering something
// other than SQLite would: a failing checkpoint, and a failing switch out of
// WAL after a clean checkpoint.
func TestFinalizeForServingReportsAFailingCheckpoint(t *testing.T) {
	db := &DB{DB: newStubDB(t, stubModeRowsErr)}

	err := db.FinalizeForServing(t.Context())
	if err == nil {
		t.Fatal("FinalizeForServing = nil, want the failing checkpoint reported")
	}
	if !strings.Contains(err.Error(), "checkpointing the WAL") {
		t.Errorf("error = %v, want it to name the checkpoint", err)
	}
}

func TestFinalizeForServingReportsAFailingSwitchOutOfWAL(t *testing.T) {
	db := &DB{DB: newStubDB(t, stubModeCheckpointedThenFails)}

	err := db.FinalizeForServing(t.Context())
	if err == nil {
		t.Fatal("FinalizeForServing = nil, want the failing journal_mode switch reported")
	}
	if !strings.Contains(err.Error(), "leaving WAL mode") {
		t.Errorf("error = %v, want it to name the journal mode switch", err)
	}
}

// readOnlyHint speaks only about the two codes it knows. Anything else gets no
// invented advice — a wrong next step costs more than none.
func TestReadOnlyHintStaysSilentOnEverythingElse(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"kein SQLite-Fehler", errors.New("connection reset")},
		{"anderer SQLite-Code", &sqlitedriver.Error{}},
	} {
		if hint := readOnlyHint(tc.err); hint != "" {
			t.Errorf("%s: readOnlyHint = %q, want empty", tc.name, hint)
		}
	}
}
