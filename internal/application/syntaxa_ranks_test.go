package application

import (
	"context"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// TestIngestSyntaxaBrichtBeiElternteilVomFalschenRangAb covers what neither
// checkDanglingParents nor checkCycles can see: CA01A's parent CA is written
// and the chain reaches formation C in two steps, so both checks pass — but
// the edge skips the order level GET /v1/syntaxon/{id} promises.
func TestIngestSyntaxaBrichtBeiElternteilVomFalschenRangAb(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: "code,rank,name,author,parent_code,eea_code\n" +
			"CA,class,Testklasse,Moor 1950,,TST\n" +
			"CA01,order,Testordnung,Moor 1960,CA,TST-01\n" +
			"CA01A,alliance,Testverband,Moor 1970,CA,TST-01A\n",
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief mit einem Verband unter einer Klasse durch")
	}
	if !strings.Contains(err.Error(), "CA01A") {
		t.Errorf("Fehler nennt CA01A nicht: %v", err)
	}
	if repo.committed {
		t.Error("commit haette nach dem Rangbruch nicht laufen duerfen")
	}
}

// The EEA-only rows are the other half: their parent is not read from a
// column but derived, and the derivation always yields an ORDER (an
// alliance's parent). A row of any other rank therefore lands one level too
// high — measured against the pinned artifacts all 16 EEA-only rows are
// alliances, so this is the hole, not the normal case.
func TestIngestSyntaxaBrichtBeiAbgeleitetemElternteilVomFalschenRangAb(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		// Name matches FloraVeg's alliance "Testverband", so the derivation
		// gives this row the order CA01 as its parent — but the row calls
		// itself an order, and an order under an order is not a hierarchy.
		eunis: "id,rank,name,parent_id\nAND-01A,order,Testverband Morariu 1957,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief mit einer Ordnung unter einer Ordnung durch")
	}
	if !strings.Contains(err.Error(), "AND-01A") {
		t.Errorf("Fehler nennt AND-01A nicht: %v", err)
	}
}

// A rank that is not on the ladder at all cannot have a correct parent
// either. readHierarchy already skips such a row, but writeEunisOnly takes
// the rank straight from syntaxa.csv.
func TestIngestSyntaxaBrichtBeiEeaZeileMitRangAusserhalbDerLeiterAb(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\nAND-01A,association,Testverband Morariu 1957,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief mit dem Rang association durch")
	}
	if !strings.Contains(err.Error(), "association") {
		t.Errorf("Fehler nennt den Rang nicht: %v", err)
	}
}

func TestIngestSyntaxaAkzeptiertDieVersprocheneRangfolge(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if len(rep.WrongRankParents) != 0 {
		t.Errorf("WrongRankParents = %v, erwartet leer", rep.WrongRankParents)
	}
}

func TestParentRankOfKenntDieLeiterUndIhreEnden(t *testing.T) {
	for rank, want := range map[string]string{
		domain.SyntaxonRankClass:    domain.SyntaxonRankFormation,
		domain.SyntaxonRankOrder:    domain.SyntaxonRankClass,
		domain.SyntaxonRankAlliance: domain.SyntaxonRankOrder,
	} {
		got, ok := parentRankOf(rank)
		if !ok || got != want {
			t.Errorf("parentRankOf(%q) = %q/%v, erwartet %q/true", rank, got, ok, want)
		}
	}
	// A formation has no parent rank, and neither has a rank off the ladder.
	for _, rank := range []string{domain.SyntaxonRankFormation, "association", ""} {
		if got, ok := parentRankOf(rank); ok {
			t.Errorf("parentRankOf(%q) = %q/true, erwartet keinen Elternrang", rank, got)
		}
	}
}
