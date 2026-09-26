package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// seedForCoverage legt einen Habitattyp mit Arten an und verteilt sie auf
// Gebiete. concepts bildet Konzept-ID auf die Gebiete ab, in denen die Art
// vorkommt; eine leere Liste heisst "Art bekannt, aber ohne Verbreitungsdaten".
func seedForCoverage(t *testing.T, db *DB, code string, concepts map[string][]string) {
	t.Helper()
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: code}
	level := 3
	if err := tx.UpsertHabitatType(domain.HabitatType{Key: key, Level: &level, NameEN: code}); err != nil {
		t.Fatalf("UpsertHabitatType: %v", err)
	}
	for cid, areas := range concepts {
		id := cid
		if err := tx.UpsertSpeciesRole(domain.SpeciesRole{
			Key: key, ConceptID: &id, VerbatimName: cid, Role: "constant", Provenance: "observed",
		}); err != nil {
			t.Fatalf("UpsertSpeciesRole: %v", err)
		}
		for _, a := range areas {
			if err := tx.UpsertDistribution(cid, domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: a}); err != nil {
				t.Fatalf("UpsertDistribution: %v", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// Die Abdeckung ist der Anteil der Arten eines Typs, die im Gebiet verbreitet
// sind. Sie ist eine Plausibilitaetspruefung, kein Filter: ein Typ mit
// Abdeckung 0 bleibt in der Antwort, er rutscht nur nach hinten.
func TestHabitatAreaCoverage_AnteilDerImGebietVerbreitetenArten(t *testing.T) {
	db := openTestDB(t)
	seedForCoverage(t, db, "T17", map[string][]string{
		"c1": {"GER"}, "c2": {"GER"}, "c3": {"SPA"}, "c4": {"SPA"},
	})

	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T17"}
	got, err := db.HabitatAreaCoverage(context.Background(), []domain.HabitatTypeKey{key}, "GER")
	if err != nil {
		t.Fatalf("HabitatAreaCoverage: %v", err)
	}
	if v := got[key]; v < 0.49 || v > 0.51 {
		t.Errorf("Abdeckung = %.3f, erwartet 0.5 (zwei von vier Arten in GER)", v)
	}
}

// Ein Typ, dessen Arten gar keine Verbreitungsdaten tragen, ist nicht
// unplausibel — er ist unbeurteilbar. Er darf nicht als 0 erscheinen.
func TestHabitatAreaCoverage_OhneVerbreitungsdatenKeinEintrag(t *testing.T) {
	db := openTestDB(t)
	seedForCoverage(t, db, "U11", map[string][]string{"c9": {}})

	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "U11"}
	got, err := db.HabitatAreaCoverage(context.Background(), []domain.HabitatTypeKey{key}, "GER")
	if err != nil {
		t.Fatalf("HabitatAreaCoverage: %v", err)
	}
	if _, ok := got[key]; ok {
		t.Error("ein Typ ohne jede Verbreitungsangabe darf keinen Abdeckungswert bekommen")
	}
}

// Leere Schluesselliste: eine Abfrage ohne Kandidaten ist keine Abfrage.
func TestHabitatAreaCoverage_LeereListe(t *testing.T) {
	db := openTestDB(t)
	got, err := db.HabitatAreaCoverage(context.Background(), nil, "GER")
	if err != nil {
		t.Fatalf("HabitatAreaCoverage: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Ergebnis = %v, erwartet leer", got)
	}
}
