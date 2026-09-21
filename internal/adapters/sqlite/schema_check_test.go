package sqlite_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/adapters/sqlite"
)

func TestOpenReadOnlyLehntIndexOhneVerbreitungstabellenAb(t *testing.T) {
	for _, table := range []string{"syntaxon_distribution", "syntaxon_distribution_coverage"} {
		t.Run(table, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "alt.sqlite")
			db, err := sqlite.OpenForIngest(context.Background(), path)
			if err != nil {
				t.Fatalf("OpenForIngest: %v", err)
			}
			// An index built before this release: every other table, but not
			// this one. Dropping it is the only honest way to produce that
			// state from the current schema.
			if _, err := db.ExecContext(context.Background(), `DROP TABLE `+table); err != nil {
				t.Fatalf("DROP TABLE: %v", err)
			}
			if err := db.FinalizeForServing(context.Background()); err != nil {
				t.Fatalf("FinalizeForServing: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			_, err = sqlite.OpenReadOnly(context.Background(), path)
			if err == nil {
				t.Fatalf("OpenReadOnly hat einen Index ohne %s akzeptiert", table)
			}
			if !strings.Contains(err.Error(), table) {
				t.Errorf("Fehler benennt die fehlende Tabelle nicht: %v", err)
			}
		})
	}
}
