package sqlite

import (
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func TestSyntaxonIDsByAltCode(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertSyntaxon(domain.Syntaxon{ID: "AA01A", Rank: domain.SyntaxonRankAlliance,
		Name: "X", AltCode: "PAP-01A", Source: domain.SyntaxonSourceEVC,
		ParentProvenance: domain.ParentProvenanceOfficial}); err != nil {
		t.Fatalf("UpsertSyntaxon(AA01A): %v", err)
	}
	if err := tx.UpsertSyntaxon(domain.Syntaxon{ID: "AA01B", Rank: domain.SyntaxonRankAlliance,
		Name: "Y", Source: domain.SyntaxonSourceEVC,
		ParentProvenance: domain.ParentProvenanceOfficial}); err != nil {
		t.Fatalf("UpsertSyntaxon(AA01B): %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.SyntaxonIDsByAltCode(ctx)
	if err != nil {
		t.Fatalf("SyntaxonIDsByAltCode: %v", err)
	}
	if len(got) != 1 || got["PAP-01A"] != "AA01A" {
		t.Errorf("Karte = %v, erwartet genau PAP-01A->AA01A", got)
	}
}
