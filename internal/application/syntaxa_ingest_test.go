package application

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// captureSlog redirects the default logger into a buffer for the duration of
// the test and restores it afterward — used to assert on a warning's
// presence/absence, not just that the code path ran. Shared across ingest
// tests (area_ingest_test.go, description_ingest_test.go); it lived in the
// now-deleted syntaxa_hierarchy_ingest_test.go before this file took it over.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

type syntaxaFiles struct {
	formations, hierarchy, eunis, links string
}

func writeSyntaxaDir(t *testing.T, f syntaxaFiles) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"syntaxa_formations.csv":   f.formations,
		"syntaxa_hierarchy.csv":    f.hierarchy,
		"syntaxa.csv":              f.eunis,
		"habitat_type_syntaxa.csv": f.links,
	} {
		if content == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("Schreiben von %s: %v", name, err)
		}
	}
	return dir
}

const (
	minimalFormations = "letter,name_en,life_form_group\n" +
		"C,Vegetation of the nemoral forest zone,phanerogam\n" +
		"R,Epigaeic bryophyte and lichen vegetation,bryophyte_lichen\n"
	// The author column mirrors a real phytosociological citation shape
	// (surname + year); the German word for "author" misspell-lints as a
	// typo of "Author", so the fixture uses the surnames "Moor"/"Braun"
	// instead — the brief's own values, only the placeholder changed.
	minimalHierarchy = "code,rank,name,author,parent_code,eea_code\n" +
		"CA,class,Testklasse,Moor 1950,,TST\n" +
		"CA01,order,Testordnung,Moor 1960,CA,TST-01\n" +
		"CA01A,alliance,Testverband,Moor 1970,CA01,TST-01A\n" +
		"RA,class,Moosklasse,Braun 1980,,MOO\n" +
		"RA01,order,Moosordnung,Braun 1981,RA,MOO-01\n" +
		"RA01A,alliance,Moosverband,Braun 1982,RA01,MOO-01A\n"
)

func TestIngestSyntaxaSchreibtFormationenAusDemBuchstaben(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})

	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if rep.FormationsWritten != 2 {
		t.Errorf("FormationsWritten = %d, erwartet 2", rep.FormationsWritten)
	}
	c := repo.syntaxonByID("C")
	if c.Rank != domain.SyntaxonRankFormation || c.ParentID != "" ||
		c.LifeFormGroup != domain.LifeFormPhanerogam {
		t.Errorf("Formation C = %+v", c)
	}
	// A formation is a root: the domain invariant says ParentProvenance must
	// be "official" whenever ParentID is empty. UpsertSyntaxon binds the
	// column explicitly, so the schema default never gets a chance to apply
	// it — the ingest itself must set it.
	if c.ParentProvenance != domain.ParentProvenanceOfficial {
		t.Errorf("Formation C ParentProvenance = %q, erwartet %q", c.ParentProvenance, domain.ParentProvenanceOfficial)
	}
	if got := repo.syntaxonByID("R").LifeFormGroup; got != domain.LifeFormBryophyteLichen {
		t.Errorf("Formation R Lebensform = %q", got)
	}
}

func TestIngestSyntaxaHaengtKlassenAnIhreFormation(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if rep.ClassesWritten != 2 || rep.OrdersWritten != 2 || rep.AlliancesWritten != 2 {
		t.Errorf("Zaehler = %d/%d/%d, erwartet 2/2/2",
			rep.ClassesWritten, rep.OrdersWritten, rep.AlliancesWritten)
	}
	for id, wantParent := range map[string]string{
		"CA": "C", "CA01": "CA", "CA01A": "CA01", "RA": "R", "RA01A": "RA01",
	} {
		if got := repo.syntaxonByID(id).ParentID; got != wantParent {
			t.Errorf("%s.ParentID = %q, erwartet %q", id, got, wantParent)
		}
	}
	if got := repo.syntaxonByID("CA01A"); got.EEACode != "TST-01A" ||
		got.Source != domain.SyntaxonSourceEVC ||
		got.ParentProvenance != domain.ParentProvenanceOfficial {
		t.Errorf("Verband = %+v", got)
	}
}

func TestIngestSyntaxaUebernimmtAutorschaftUndNameAusFloraVeg(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	if _, err := IngestSyntaxa(context.Background(), repo, dir); err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	got := repo.syntaxonByID("CA01A")
	if got.Name != "Testverband" || got.Author != "Moor 1970" {
		t.Errorf("Name/Author = %q/%q", got.Name, got.Author)
	}
}

func TestIngestSyntaxaBrichtOhneFormationsdateiAb(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		hierarchy: minimalHierarchy,
		eunis:     "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief ohne syntaxa_formations.csv durch")
	}
	if !strings.Contains(err.Error(), "syntaxa_formations.csv") {
		t.Errorf("Fehler benennt die Datei nicht: %v", err)
	}
}

func TestIngestSyntaxaBrichtOhneHierarchiedateiAb(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		eunis:      "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief ohne syntaxa_hierarchy.csv durch")
	}
	if !strings.Contains(err.Error(), "syntaxa_hierarchy.csv") {
		t.Errorf("Fehler benennt die Datei nicht: %v", err)
	}
}

func TestIngestSyntaxaBrichtBeiDoppeltemFormationsbuchstabenAb(t *testing.T) {
	// A duplicated letter would silently overwrite the earlier formation:
	// one of the two names simply disappears, and every class under the
	// letter lands under the survivor without anything being reported.
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations + "C,Vegetation of the nemoral forest zone (Dublette),phanerogam\n",
		hierarchy:  minimalHierarchy,
		eunis:      "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa akzeptierte einen doppelten Formationsbuchstaben")
	}
	if !strings.Contains(err.Error(), "C") || !strings.Contains(err.Error(), fileFormations) {
		t.Errorf("Fehler benennt Buchstabe und Datei nicht: %v", err)
	}
	// Weder Commit noch Rollback: die Pruefung laeuft, bevor eine
	// Transaktion aufgeht, der Index wird also gar nicht erst angefasst.
	if repo.committed || repo.rolledBack {
		t.Errorf("erwartet keine Transaktion, committed=%v rolledBack=%v", repo.committed, repo.rolledBack)
	}
}

func TestIngestSyntaxaBrichtBeiLeeremFormationsbuchstabenAb(t *testing.T) {
	// An empty letter would be written as a formation with the id "" — a row
	// nothing can reach and that no class could ever point at.
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations + ",Vegetation ohne Buchstabe,phanerogam\n",
		hierarchy:  minimalHierarchy,
		eunis:      "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa akzeptierte eine Formationszeile ohne Buchstaben")
	}
	if !strings.Contains(err.Error(), fileFormations) {
		t.Errorf("Fehler benennt die Datei nicht: %v", err)
	}
}

func TestIngestSyntaxaBrichtBeiFehlenderEEACodeSpalteAb(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy:  "code,rank,name,author,parent_code\nCA,class,K,,\n",
		eunis:      "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	if _, err := IngestSyntaxa(context.Background(), repo, dir); err == nil {
		t.Fatal("IngestSyntaxa akzeptierte eine CSV ohne eea_code")
	}
}

func TestIngestSyntaxaUeberspringtKlasseMitUnbekanntemBuchstaben(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: "code,rank,name,author,parent_code,eea_code\n" +
			"ZA,class,Klasse ohne Formation,,,ZZZ\n",
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if rep.ClassesWritten != 0 || rep.SkippedRows != 1 {
		t.Errorf("ClassesWritten/SkippedRows = %d/%d, erwartet 0/1",
			rep.ClassesWritten, rep.SkippedRows)
	}
	if repo.has("ZA") {
		t.Error("ZA wurde geschrieben, obwohl Formation Z fehlt")
	}
}

func TestIngestSyntaxaBrichtBeiBaumelndemParentCodeAb(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: "code,rank,name,author,parent_code,eea_code\n" +
			"CA,class,Testklasse,Moor 1950,,TST\n" +
			// CA99 does not exist anywhere in this file: a stale parent_code,
			// not a skipped class.
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

// TestIngestSyntaxaBrichtBeiZyklusImParentCodeAb covers what
// checkDanglingParents alone cannot see: CA01 and CA01A point at each other,
// so both rows ARE written and both have a written parent — the plain
// "was the parent written" check passes them cleanly, even though neither
// ever reaches the formation C. checkCycles must catch this.
func TestIngestSyntaxaBrichtBeiZyklusImParentCodeAb(t *testing.T) {
	repo := newFakeRepo()
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

func TestIngestSyntaxaBrichtAbWennOrdnungAnUebersprungenerKlasseHaengt(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: "code,rank,name,author,parent_code,eea_code\n" +
			// ZA's formation letter Z is unknown -> ZA is skipped
			// (SkippedRows), but its order ZA01 is written regardless and
			// must not be allowed to dangle.
			"ZA,class,Klasse ohne Formation,,,ZZZ\n" +
			"ZA01,order,Ordnung ohne Klasse,,ZA,ZZZ-01\n",
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief mit einer Ordnung an uebersprungener Klasse durch")
	}
	if !strings.Contains(err.Error(), "ZA01") {
		t.Errorf("Fehler nennt ZA01 nicht: %v", err)
	}
}

func TestFormationOfLeeresCodeLiefertLeereFormation(t *testing.T) {
	if got := formationOf(""); got != "" {
		t.Errorf("formationOf(\"\") = %q, erwartet \"\"", got)
	}
}

func TestIngestSyntaxaUeberspringtUnbekanntenRang(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: "code,rank,name,author,parent_code,eea_code\n" +
			"CF,family,Unbekannter Rang,,,CFX\n",
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if rep.SkippedRows != 1 {
		t.Errorf("SkippedRows = %d, erwartet 1", rep.SkippedRows)
	}
	if repo.has("CF") {
		t.Error("CF wurde geschrieben, obwohl der Rang unbekannt ist")
	}
}

func TestIngestSyntaxaBrichtBeiBeginFehlerAb(t *testing.T) {
	repo := newFakeRepo()
	repo.beginErr = fmt.Errorf("boom")
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz Begin-Fehler durch")
	}
	if !strings.Contains(err.Error(), "beginning syntaxa ingest transaction") {
		t.Errorf("Fehler = %v, erwartet Begin-Kontext", err)
	}
}

func TestIngestSyntaxaRollbackBeiFehlerhaftemUpsertFormation(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertSyntaxon"
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy:  "code,rank,name,author,parent_code,eea_code\n",
		eunis:      "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz fehlschlagendem Formation-Upsert durch")
	}
	if !strings.Contains(err.Error(), "upserting formation") {
		t.Errorf("Fehler = %v, erwartet Formation-Kontext", err)
	}
	if !repo.rolledBack {
		t.Error("rolledBack = false, erwartet true nach fehlgeschlagenem Upsert")
	}
	if repo.committed {
		t.Error("committed = true, obwohl der Upsert fehlschlug")
	}
}

func TestIngestSyntaxaRollbackBeiFehlerhaftemUpsertHierarchie(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertSyntaxon"
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: "letter,name_en,life_form_group\n",
		hierarchy: "code,rank,name,author,parent_code,eea_code\n" +
			"CA01A,alliance,Testverband,Moor 1970,CA01,TST-01A\n",
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz fehlschlagendem Hierarchie-Upsert durch")
	}
	if !strings.Contains(err.Error(), "upserting alliance CA01A") {
		t.Errorf("Fehler = %v, erwartet Hierarchie-Kontext", err)
	}
	if !repo.rolledBack {
		t.Error("rolledBack = false, erwartet true nach fehlgeschlagenem Upsert")
	}
}

func TestIngestSyntaxaMeldetFehlgeschlagenesRollback(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertSyntaxon"
	repo.rollbackErr = fmt.Errorf("rollback boom")
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy:  "code,rank,name,author,parent_code,eea_code\n",
		eunis:      "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz Rollback-Fehler durch")
	}
	if !strings.Contains(err.Error(), "rollback also failed") {
		t.Errorf("Fehler = %v, erwartet Hinweis auf fehlgeschlagenes Rollback", err)
	}
}

func TestIngestSyntaxaSchreibtNurEeaEinheitenOhneFloraVegGegenstueck(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		// TST-01A has a FloraVeg counterpart (CA01A), EIG-01A does not, but
		// a name match resolves its parent (Task 7): a genuinely
		// unresolvable EIG-01A would fail the whole ingest as an orphan.
		formations: minimalFormations,
		hierarchy: minimalHierarchy +
			"RA02A,alliance,Eigenverband,Moor 1990,RA01,XYZ-01A\n",
		eunis: "id,rank,name,parent_id\n" +
			"TST-01A,alliance,Testverband Moor 1970,\n" +
			"EIG-01A,alliance,Eigenverband Moor 1990,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if rep.EunisOnly != 1 {
		t.Errorf("EunisOnly = %d, erwartet 1", rep.EunisOnly)
	}
	if repo.has("TST-01A") {
		t.Error("TST-01A wurde als eigene Zeile geschrieben, obwohl CA01A sie vertritt")
	}
	eig := repo.syntaxonByID("EIG-01A")
	if eig.Source != domain.SyntaxonSourceEUNIS || eig.EEACode != "" {
		t.Errorf("EIG-01A = %+v, erwartet source=eunis und leeren EEACode", eig)
	}
	if eig.Name != "Eigenverband Moor 1990" {
		t.Errorf("Name = %q, erwartet den EUNIS-Kombistring unveraendert", eig.Name)
	}
}

func TestIngestSyntaxaLoestKantenUeberDenEEACodeAuf(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\nTST-01A,alliance,Testverband Moor 1970,\n",
		links: "typology_id,code,syntaxon_id\neunis@2021,T11,TST-01A\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if rep.LinksWritten != 1 || rep.LinksRemapped != 1 {
		t.Errorf("LinksWritten/LinksRemapped = %d/%d, erwartet 1/1",
			rep.LinksWritten, rep.LinksRemapped)
	}
	if got := repo.linkTargets("eunis@2021", "T11"); len(got) != 1 || got[0] != "CA01A" {
		t.Errorf("Kanten = %v, erwartet [CA01A]", got)
	}
}

func TestIngestSyntaxaBehaeltKanteAufEeaEigeneEinheit(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		// A name match resolves EIG-01A's parent (Task 7); an unresolvable
		// row would fail the whole ingest as an orphan.
		formations: minimalFormations,
		hierarchy: minimalHierarchy +
			"RA02A,alliance,Eigenverband,Moor 1990,RA01,XYZ-01A\n",
		eunis: "id,rank,name,parent_id\nEIG-01A,alliance,Eigenverband Moor 1990,\n",
		links: "typology_id,code,syntaxon_id\neunis@2021,T11,EIG-01A\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if rep.LinksRemapped != 0 {
		t.Errorf("LinksRemapped = %d, erwartet 0", rep.LinksRemapped)
	}
	if got := repo.linkTargets("eunis@2021", "T11"); len(got) != 1 || got[0] != "EIG-01A" {
		t.Errorf("Kanten = %v, erwartet [EIG-01A]", got)
	}
}

func TestIngestSyntaxaMeldetKanteAufUnbekanntesSyntaxon(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n",
		links: "typology_id,code,syntaxon_id\neunis@2021,T11,GIBTSNICHT\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if len(rep.UnknownLinkTargets) != 1 || rep.UnknownLinkTargets[0] != "GIBTSNICHT" {
		t.Errorf("UnknownLinkTargets = %v", rep.UnknownLinkTargets)
	}
	if rep.LinksWritten != 0 {
		t.Errorf("LinksWritten = %d, erwartet 0", rep.LinksWritten)
	}
}

func TestIngestSyntaxaFuehrtEeaOrdnungMitFloraVegGegenstueckZusammen(t *testing.T) {
	// The ASP-03/KC03 case: an EEA order that FloraVeg carries
	// identically. The habitat type's edge must survive, just pointing
	// at the primary code.
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\nTST-01,order,Testordnung Moor 1960,\n",
		links: "typology_id,code,syntaxon_id\neunis@2021,T11,TST-01\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if repo.has("TST-01") {
		t.Error("TST-01 blieb als Dublette von CA01 im Index")
	}
	if got := repo.linkTargets("eunis@2021", "T11"); len(got) != 1 || got[0] != "CA01" {
		t.Errorf("Kanten = %v, erwartet [CA01]", got)
	}
	if rep.LinksRemapped != 1 {
		t.Errorf("LinksRemapped = %d, erwartet 1", rep.LinksRemapped)
	}
}

func TestIngestSyntaxaScheitertAnKollidierendemEEACode(t *testing.T) {
	// Two FloraVeg rows carrying the SAME eea_code. The EEA code is the
	// documented migration path from the old ids, so a duplicate makes
	// GET /v1/syntaxon/TST-01A resolvable to either row — the ingest must
	// fail on it, not build such an index.
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: minimalHierarchy +
			"RA02A,alliance,Doppelverband,Braun 1990,RA01,TST-01A\n",
		eunis: "id,rank,name,parent_id\nTST-01A,alliance,Testverband Moor 1970,\n",
		links: "typology_id,code,syntaxon_id\nannex1,9130,TST-01A\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz doppeltem EEA-Code durch")
	}
	if !strings.Contains(err.Error(), "TST-01A") {
		t.Errorf("Fehler = %v, erwartet den kollidierenden Code TST-01A", err)
	}
	if len(rep.EEACodeCollisions) != 1 || rep.EEACodeCollisions[0] != "TST-01A" {
		t.Errorf("EEACodeCollisions = %v, erwartet [TST-01A]", rep.EEACodeCollisions)
	}
	if repo.has("RA02A") {
		t.Error("RA02A wurde geschrieben, obwohl der Ingest abbrechen sollte")
	}
}

func TestIngestSyntaxaNenntJedenKollidierendenEEACodeGenauEinmal(t *testing.T) {
	// The operator must be able to repair the source in one pass, so the
	// error names EVERY colliding code, sorted and each exactly once —
	// three rows on one code are still one entry. RA05A/RA06A carry no
	// eea_code at all: the empty string is not a code that can collide.
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: minimalHierarchy +
			"RA02A,alliance,Doppelverband,Braun 1990,RA01,TST-01A\n" +
			"RA03A,alliance,Drittverband,Braun 1991,RA01,TST-01A\n" +
			"RA04A,alliance,Viertverband,Braun 1992,RA01,MOO-01A\n" +
			"RA05A,alliance,Leerverband,Braun 1993,RA01,\n" +
			"RA06A,alliance,Zweitleerverband,Braun 1994,RA01,\n",
		eunis: "id,rank,name,parent_id\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz doppelter EEA-Codes durch")
	}
	want := []string{"MOO-01A", "TST-01A"}
	if !slices.Equal(rep.EEACodeCollisions, want) {
		t.Fatalf("EEACodeCollisions = %v, erwartet %v", rep.EEACodeCollisions, want)
	}
	for _, code := range want {
		if strings.Count(err.Error(), code) != 1 {
			t.Errorf("Fehler = %v, erwartet %s genau einmal", err, code)
		}
	}
}

func TestIngestSyntaxaRollbackBeiFehlerhaftemUpsertEunisOnly(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertSyntaxon"
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: "letter,name_en,life_form_group\n",
		hierarchy:  "code,rank,name,author,parent_code,eea_code\n",
		eunis:      "id,rank,name,parent_id\nEIG-01A,alliance,Eigenverband Moor 1990,\n",
		links:      "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz fehlschlagendem EUNIS-Only-Upsert durch")
	}
	if !strings.Contains(err.Error(), "upserting eunis-only EIG-01A") {
		t.Errorf("Fehler = %v, erwartet EUNIS-Only-Kontext", err)
	}
	if !repo.rolledBack {
		t.Error("rolledBack = false, erwartet true nach fehlgeschlagenem Upsert")
	}
}

func TestIngestSyntaxaRollbackBeiFehlerhaftemLinkSyntaxon(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "LinkSyntaxon"
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: "letter,name_en,life_form_group\n",
		hierarchy:  "code,rank,name,author,parent_code,eea_code\n",
		eunis:      "id,rank,name,parent_id\nEIG-01A,alliance,Eigenverband Moor 1990,\n",
		links:      "typology_id,code,syntaxon_id\neunis@2021,T11,EIG-01A\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz fehlschlagendem LinkSyntaxon durch")
	}
	if !strings.Contains(err.Error(), "linking T11 to EIG-01A") {
		t.Errorf("Fehler = %v, erwartet Link-Kontext", err)
	}
	if !repo.rolledBack {
		t.Error("rolledBack = false, erwartet true nach fehlgeschlagenem Link")
	}
}

func TestIngestSyntaxaUeberspringtKanteMitUngueltigerTypologyId(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n",
		links: "typology_id,code,syntaxon_id\n,T11,CA01A\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if rep.LinksWritten != 0 {
		t.Errorf("LinksWritten = %d, erwartet 0", rep.LinksWritten)
	}
	if rep.SkippedRows != 1 {
		t.Errorf("SkippedRows = %d, erwartet 1", rep.SkippedRows)
	}
}

func TestIngestSyntaxaMeldetCommitFehler(t *testing.T) {
	repo := newFakeRepo()
	repo.commitErr = fmt.Errorf("commit boom")
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz Commit-Fehler durch")
	}
	if !strings.Contains(err.Error(), "committing syntaxa ingest transaction") {
		t.Errorf("Fehler = %v, erwartet Commit-Kontext", err)
	}
}
