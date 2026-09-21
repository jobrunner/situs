package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// seedHierarchy builds a small but representative syntaxa hierarchy:
// formation C, class CA, order CA01, alliance CA01A — a full four-step chain
// rooted at a formation — plus one EEA-only alliance whose parent is not the
// source's own but a derived one, matching what IngestSyntaxa produces for a
// EUNIS row with no FloraVeg counterpart.
func seedHierarchy(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, s := range []domain.Syntaxon{
		{ID: "C", Rank: domain.SyntaxonRankFormation, Name: "Formation C",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA", Rank: domain.SyntaxonRankClass, Name: "Class CA", ParentID: "C",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01", Rank: domain.SyntaxonRankOrder, Name: "Order CA01", ParentID: "CA",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01A", Rank: domain.SyntaxonRankAlliance, Name: "Alliance CA01A", ParentID: "CA01",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		// An EEA-only alliance the FloraVeg hierarchy has no counterpart for:
		// its parent was derived (sibling consensus / name match), not read
		// straight from a source hierarchy.
		{ID: "ZZ-01A", Rank: domain.SyntaxonRankAlliance, Name: "EEA-only alliance", ParentID: "CA01",
			AltCode: "ZZZ-01A", Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived},
	} {
		if err := tx.UpsertSyntaxon(s); err != nil {
			t.Fatalf("UpsertSyntaxon(%s): %v", s.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// The promise from the spec as a test: no row except the formations is
// without a parent, and every one reaches a formation in at most three
// steps.
func TestHierarchieHatKeineWaisen(t *testing.T) {
	db := openTestDB(t)
	seedHierarchy(t, db)

	rows, err := db.QueryContext(context.Background(),
		`SELECT id FROM syntaxon WHERE rank <> 'formation' AND parent_id = ''`)
	if err != nil {
		t.Fatalf("Abfrage: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var orphans []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		orphans = append(orphans, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("elternlose Nicht-Formationen: %v", orphans)
	}
}

func TestJedeZeileErreichtEineFormation(t *testing.T) {
	db := openTestDB(t)
	seedHierarchy(t, db)

	all, err := db.AllSyntaxa(context.Background())
	if err != nil {
		t.Fatalf("AllSyntaxa: %v", err)
	}
	byID := map[string]domain.Syntaxon{}
	for _, s := range all {
		byID[s.ID] = s
	}
	for _, s := range all {
		cur, steps := s, 0
		for cur.Rank != domain.SyntaxonRankFormation {
			if steps > 3 {
				t.Fatalf("%s erreicht nach 3 Schritten keine Formation", s.ID)
			}
			next, ok := byID[cur.ParentID]
			if !ok {
				t.Fatalf("%s: parent_id %q zeigt ins Leere", cur.ID, cur.ParentID)
			}
			cur, steps = next, steps+1
		}
	}
}
