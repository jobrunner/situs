package sqlite_test

import (
	"testing"

	"github.com/jobrunner/situs/internal/adapters/sqlite"
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
