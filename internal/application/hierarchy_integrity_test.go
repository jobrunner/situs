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

// seedOldStyleSyntaxon writes one syntaxon row and one habitat-type edge the
// way a pre-quellenumkehr index carried them: an EEA-style id, source and
// eea_code both empty (the migration's DEFAULT ” on an old index the
// column was added to, not a value this ingest would ever write itself).
func seedOldStyleSyntaxon(t *testing.T, repo *sqlite.DB, id string, key domain.HabitatTypeKey) {
	t.Helper()
	tx, err := repo.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin (seed): %v", err)
	}
	if err := tx.UpsertSyntaxon(domain.Syntaxon{
		ID: id, Rank: domain.SyntaxonRankAlliance, Name: "Alter Verband",
	}); err != nil {
		t.Fatalf("UpsertSyntaxon (seed): %v", err)
	}
	if err := tx.LinkSyntaxon(key, id); err != nil {
		t.Fatalf("LinkSyntaxon (seed): %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit (seed): %v", err)
	}
}

// TestIngestSyntaxaAufAltemIndexErsetztStattVerdoppelt is the measured case
// from the repeat-ingest finding: an index built before the syntaxa
// quellenumkehr carries EEA-style rows (source/eea_code both ”, from the
// migration's DEFAULT ” on a column the old rows never populated) that
// SyntaxonIDsByEEACode-based relinking could not see, because it only ever
// looked at rows with a non-empty eea_code. Without ClearSyntaxa these rows
// and their edges survive next to the current FloraVeg rows and every read
// answers with duplicates. This test must fail without the fix.
func TestIngestSyntaxaAufAltemIndexErsetztStattVerdoppelt(t *testing.T) {
	repo := openIntegrityDB(t)
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T11"}
	seedOldStyleSyntaxon(t, repo, "OLD-STYLE-01A", key)

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
	for _, s := range all {
		if s.ID == "OLD-STYLE-01A" {
			t.Fatalf("OLD-STYLE-01A ueberlebte den Ingest: %+v", all)
		}
	}
	// minimalFormations (2) + minimalHierarchy (6): the current source's
	// full and only content, nothing from before it.
	if len(all) != 8 {
		t.Errorf("AllSyntaxa = %d Zeilen, erwartet genau 8 (nur die aktuelle Quelle)", len(all))
	}
	edges, err := repo.HabitatTypeKeysForSyntaxon(context.Background(), "OLD-STYLE-01A")
	if err != nil {
		t.Fatalf("HabitatTypeKeysForSyntaxon: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("OLD-STYLE-01A traegt noch Kanten: %v", edges)
	}
}

// TestIngestSyntaxaIstIdempotent runs the same ingest twice onto the same
// real index and requires identical row and edge counts — the replace-not-
// merge fix must not itself introduce duplicates on a repeat run.
func TestIngestSyntaxaIstIdempotent(t *testing.T) {
	repo := openIntegrityDB(t)
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\nTST-01A,alliance,Testverband Moor 1970,\n",
		links: "typology_id,code,syntaxon_id\neunis@2021,T11,TST-01A\n",
	})
	ctx := context.Background()
	if _, err := IngestSyntaxa(ctx, repo, dir); err != nil {
		t.Fatalf("erster IngestSyntaxa: %v", err)
	}
	firstAll, err := repo.AllSyntaxa(ctx)
	if err != nil {
		t.Fatalf("AllSyntaxa (1): %v", err)
	}
	firstEdges, err := repo.HabitatTypeKeysForSyntaxon(ctx, "CA01A")
	if err != nil {
		t.Fatalf("HabitatTypeKeysForSyntaxon (1): %v", err)
	}

	if _, err := IngestSyntaxa(ctx, repo, dir); err != nil {
		t.Fatalf("zweiter IngestSyntaxa: %v", err)
	}
	secondAll, err := repo.AllSyntaxa(ctx)
	if err != nil {
		t.Fatalf("AllSyntaxa (2): %v", err)
	}
	secondEdges, err := repo.HabitatTypeKeysForSyntaxon(ctx, "CA01A")
	if err != nil {
		t.Fatalf("HabitatTypeKeysForSyntaxon (2): %v", err)
	}

	if len(secondAll) != len(firstAll) {
		t.Errorf("Syntaxa-Zeilen: 1. Lauf %d, 2. Lauf %d, erwartet gleich", len(firstAll), len(secondAll))
	}
	if len(secondEdges) != len(firstEdges) || len(secondEdges) != 1 {
		t.Errorf("Kanten auf CA01A: 1. Lauf %d, 2. Lauf %d, erwartet je 1", len(firstEdges), len(secondEdges))
	}
}

// TestIngestSyntaxaBleibtAtomarBeiFehlerNachDemLeeren pins that ClearSyntaxa
// and every write live in the same transaction: a run that fails AFTER the
// clear (here: a dangling parent_code, caught after ClearSyntaxa in
// writeSyntaxa) must roll back the clear too, leaving the index exactly as
// it was before the run.
func TestIngestSyntaxaBleibtAtomarBeiFehlerNachDemLeeren(t *testing.T) {
	repo := openIntegrityDB(t)
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T11"}
	seedOldStyleSyntaxon(t, repo, "OLD-STYLE-01A", key)

	ctx := context.Background()
	before, err := repo.AllSyntaxa(ctx)
	if err != nil {
		t.Fatalf("AllSyntaxa (vorher): %v", err)
	}
	beforeEdges, err := repo.HabitatTypeKeysForSyntaxon(ctx, "OLD-STYLE-01A")
	if err != nil {
		t.Fatalf("HabitatTypeKeysForSyntaxon (vorher): %v", err)
	}

	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: "code,rank,name,author,parent_code,eea_code\n" +
			"CA,class,Testklasse,Moor 1950,,TST\n" +
			"CA01,order,Testordnung,Moor 1960,CA99,TST-01\n",
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	if _, err := IngestSyntaxa(ctx, repo, dir); err == nil {
		t.Fatal("IngestSyntaxa lief mit baumelndem parent_code durch")
	}

	after, err := repo.AllSyntaxa(ctx)
	if err != nil {
		t.Fatalf("AllSyntaxa (nachher): %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("Syntaxa nach fehlgeschlagenem Ingest = %d Zeilen, erwartet unveraendert %d", len(after), len(before))
	}
	afterEdges, err := repo.HabitatTypeKeysForSyntaxon(ctx, "OLD-STYLE-01A")
	if err != nil {
		t.Fatalf("HabitatTypeKeysForSyntaxon (nachher): %v", err)
	}
	if len(afterEdges) != len(beforeEdges) || len(afterEdges) != 1 {
		t.Errorf("Kanten auf OLD-STYLE-01A nach fehlgeschlagenem Ingest = %d, erwartet unveraendert 1", len(afterEdges))
	}
}
