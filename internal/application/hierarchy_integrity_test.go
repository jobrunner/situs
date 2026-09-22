// hierarchy_integrity_test.go checks the spec's central promise about the
// syntaxa hierarchy — no row but a formation is parentless, and every row
// reaches a formation in at most three steps — against a REAL IngestSyntaxa
// run over small CSVs and a real sqlite index, not a hand-seeded fixture that
// is correct by construction. A fixture seeded with UpsertSyntaxon directly
// proves nothing about the ingest: it would stay green even if writeHierarchy
// let a dangling parent_code through (see the Orphans check
// checkDanglingParents adds in syntaxa_ingest.go).
package application

import (
	"context"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/adapters/sqlite"
	"github.com/jobrunner/situs/internal/domain"
)

func TestIngestSyntaxaHierarchieHatKeineWaisenUndErreichtJedeFormation(t *testing.T) {
	repo := openIntegrityDB(t)
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	if _, err := IngestSyntaxa(context.Background(), repo, dir); err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}

	all, err := repo.AllSyntaxa(context.Background())
	if err != nil {
		t.Fatalf("AllSyntaxa: %v", err)
	}
	byID := map[string]domain.Syntaxon{}
	for _, s := range all {
		byID[s.ID] = s
	}
	for _, s := range all {
		if s.Rank != domain.SyntaxonRankFormation && s.ParentID == "" {
			t.Errorf("%s: elternlos und keine Formation", s.ID)
		}
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

// The violating variant: without checkDanglingParents (Befund 2's fix), this
// CSV would leave CA01 written with a parent_code that names no syntaxon
// anywhere in the file — exactly the defect the clean test above cannot
// detect on its own. IngestSyntaxa must refuse it, not silently index a
// dangling pointer.
func TestIngestSyntaxaHierarchieMitBaumelndemParentWirdAbgelehnt(t *testing.T) {
	repo := openIntegrityDB(t)
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: "code,rank,name,author,parent_code,eea_code\n" +
			"CA,class,Testklasse,Moor 1950,,TST\n" +
			"CA01,order,Testordnung,Moor 1960,CA99,TST-01\n",
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief mit baumelndem parent_code durch")
	}
	if !strings.Contains(err.Error(), "CA01") {
		t.Errorf("Fehler nennt CA01 nicht: %v", err)
	}
}

// TestIngestSyntaxaHierarchieMitZyklusWirdAbgelehnt is
// TestIngestSyntaxaHierarchieMitBaumelndemParentWirdAbgelehnt's sibling for
// Befund 3: CA01 and CA01A point at each other, so checkDanglingParents'
// "was the parent written" check passes both — proving the cycle check must
// live in the ingest, not just be provable against the fake repo.
func TestIngestSyntaxaHierarchieMitZyklusWirdAbgelehnt(t *testing.T) {
	repo := openIntegrityDB(t)
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: "code,rank,name,author,parent_code,eea_code\n" +
			"CA,class,Testklasse,Moor 1950,,TST\n" +
			"CA01,order,Testordnung,Moor 1960,CA01A,TST-01\n" +
			"CA01A,alliance,Testverband,Moor 1970,CA01,TST-01A\n",
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief mit einem Zyklus im parent_code durch")
	}
	if !strings.Contains(err.Error(), "CA01") || !strings.Contains(err.Error(), "CA01A") {
		t.Errorf("Fehler nennt nicht beide Zyklus-Mitglieder CA01/CA01A: %v", err)
	}
}

// openIntegrityDB opens a real, on-disk sqlite index the way `situs ingest`
// would (schema applied, WAL journal) — shared by this file and
// distribution_integrity_test.go, both of which need the actual adapter
// rather than the fake repository the rest of this package's tests use.
func openIntegrityDB(t *testing.T) *sqlite.DB {
	t.Helper()
	db, err := sqlite.OpenForIngest(t.Context(), t.TempDir()+"/integrity.sqlite")
	if err != nil {
		t.Fatalf("OpenForIngest: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
