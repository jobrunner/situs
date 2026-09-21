package application

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	minimalHierarchy = "code,rank,name,author,parent_code,alt_code\n" +
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
	if got := repo.syntaxonByID("CA01A"); got.AltCode != "TST-01A" ||
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

func TestIngestSyntaxaBrichtBeiFehlenderAltcodeSpalteAb(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy:  "code,rank,name,author,parent_code\nCA,class,K,,\n",
		eunis:      "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	if _, err := IngestSyntaxa(context.Background(), repo, dir); err == nil {
		t.Fatal("IngestSyntaxa akzeptierte eine CSV ohne alt_code")
	}
}

func TestIngestSyntaxaUeberspringtKlasseMitUnbekanntemBuchstaben(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: "code,rank,name,author,parent_code,alt_code\n" +
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

func TestFormationOfLeeresCodeLiefertLeereFormation(t *testing.T) {
	if got := formationOf(""); got != "" {
		t.Errorf("formationOf(\"\") = %q, erwartet \"\"", got)
	}
}

func TestIngestSyntaxaUeberspringtUnbekanntenRang(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: "code,rank,name,author,parent_code,alt_code\n" +
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
		hierarchy:  "code,rank,name,author,parent_code,alt_code\n",
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
		hierarchy: "code,rank,name,author,parent_code,alt_code\n" +
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
		hierarchy:  "code,rank,name,author,parent_code,alt_code\n",
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
