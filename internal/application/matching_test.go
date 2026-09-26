package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// newMatchService baut eine kleine Welt: T17 fuehrt alle drei Arten mit hoher
// Stetigkeit, T18 nur die erste, R1A nur die zweite und schwach.
func newMatchService(t *testing.T) *QueryService {
	t.Helper()
	repo := newFakeRepo()
	level := 3
	mk := func(code string) domain.HabitatTypeKey {
		k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: code}
		repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: code})
		return k
	}
	t17, t18, r1a := mk("T17"), mk("T18"), mk("R1A")
	add := func(k domain.HabitatTypeKey, cid, role string, constancy, fidelity float64) {
		id := cid
		r := domain.SpeciesRole{Key: k, ConceptID: &id, VerbatimName: cid, Role: role, Provenance: "observed"}
		if constancy > 0 {
			c := constancy
			r.Constancy = &c
		}
		if fidelity > 0 {
			f := fidelity
			r.Fidelity = &f
		}
		repo.speciesRoles = append(repo.speciesRoles, r)
	}
	add(t17, "wcvp:c1", "constant", 99, 0)
	add(t17, "wcvp:c2", "constant", 80, 0)
	add(t17, "wcvp:c3", "diagnostic", 0, 31)
	add(t18, "wcvp:c1", "constant", 40, 0)
	add(r1a, "wcvp:c2", "constant", 20, 0)
	return NewQueryService(repo)
}

// Ein Typ, der alle drei Arten fuehrt, muss vor einem stehen, der nur eine
// fuehrt — das ist der ganze Zweck der Route.
func TestMatchHabitatTypes_OrdnetNachTrefferlage(t *testing.T) {
	svc := newMatchService(t)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1", "wcvp:c2", "wcvp:c3"},
		Typology:   "eunis@2021", Level: 3, Limit: 10,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) < 2 {
		t.Fatalf("nur %d Treffer, erwartet mindestens 2", len(got.Matches))
	}
	if got.Matches[0].Code != "T17" {
		t.Errorf("Rang 1 = %s, erwartet T17", got.Matches[0].Code)
	}
	if got.Matches[0].Score <= got.Matches[1].Score {
		t.Error("die Liste ist nicht absteigend nach Score sortiert")
	}
	if got.Matches[0].Matched != 3 || got.Matches[0].Of != 3 {
		t.Errorf("matched/of = %d/%d, erwartet 3/3", got.Matches[0].Matched, got.Matches[0].Of)
	}
}

// Unbekannte IDs sind keine Fehlersituation: die Frage war beantwortbar. Jede
// Eingabe wird zurueckgespiegelt, mit demselben reason-Vokabular wie der
// bestehende Batch-Endpunkt.
func TestMatchHabitatTypes_SpiegeltUnbekannteEingabenZurueck(t *testing.T) {
	svc := newMatchService(t)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1", "gbif:7777", "wcvp:unbekannt"},
		Typology:   "eunis@2021", Level: 3, Limit: 10,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Input) != 3 {
		t.Fatalf("Input = %d Eintraege, erwartet 3", len(got.Input))
	}
	byID := map[string]input.MatchInput{}
	for _, e := range got.Input {
		byID[e.ConceptID] = e
	}
	if !byID["wcvp:c1"].Known {
		t.Error("wcvp:c1 muesste bekannt sein")
	}
	if byID["gbif:7777"].Reason != input.ReasonUnknownBackbone {
		t.Errorf("gbif:7777 reason = %q, erwartet unknown_backbone", byID["gbif:7777"].Reason)
	}
	if byID["wcvp:unbekannt"].Reason != input.ReasonUnknownConcept {
		t.Errorf("wcvp:unbekannt reason = %q, erwartet unknown_concept", byID["wcvp:unbekannt"].Reason)
	}
}

// Sind alle Eingaben unbekannt, ist die Antwort leer — aber ohne Fehler.
func TestMatchHabitatTypes_AlleUnbekanntGibtLeereListe(t *testing.T) {
	svc := newMatchService(t)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"gbif:1", "gbif:2"},
		Typology:   "eunis@2021", Level: 3, Limit: 10,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: unerwarteter Fehler %v", err)
	}
	if len(got.Matches) != 0 {
		t.Errorf("Matches = %d, erwartet 0", len(got.Matches))
	}
}

// Kandidat ist nur, wer mindestens eine Eingabe-Art fuehrt. Alle uebrigen
// haetten den identischen Score k*log(MISS) und waeren rangneutral.
func TestMatchHabitatTypes_NurTypenMitMindestensEinemTreffer(t *testing.T) {
	svc := newMatchService(t)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 50,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) == 0 {
		t.Fatal("keine Treffer, erwartet T17 und T18")
	}
	for _, m := range got.Matches {
		if m.Matched == 0 {
			t.Errorf("%s steht in der Liste, fuehrt aber keine der Eingabe-Arten", m.Code)
		}
	}
}

// limit kuerzt die Liste, aendert aber nicht die Reihenfolge.
func TestMatchHabitatTypes_LimitKuerztDieListe(t *testing.T) {
	svc := newMatchService(t)
	req := input.MatchRequest{ConceptIDs: []string{"wcvp:c1", "wcvp:c2"}, Typology: "eunis@2021", Level: 3}

	req.Limit = 50
	alle, err := svc.MatchHabitatTypes(context.Background(), req)
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	req.Limit = 1
	eins, err := svc.MatchHabitatTypes(context.Background(), req)
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(eins.Matches) != 1 {
		t.Fatalf("Matches = %d, erwartet 1", len(eins.Matches))
	}
	if eins.Matches[0].Code != alle.Matches[0].Code {
		t.Error("limit aendert die Reihenfolge")
	}
}

// Ein unbekanntes Gebiet ist ein Fehler des Aufrufers, keine leise ignorierte
// Angabe — derselbe Fehlertyp wie beim vorhandenen ?area=.
func TestMatchHabitatTypes_UnbekanntesGebietIstEinFehler(t *testing.T) {
	svc := newMatchService(t)

	_, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 10,
		Area: "XXX",
	})
	if !errors.Is(err, input.ErrUnknownArea) {
		t.Errorf("err = %v, erwartet input.ErrUnknownArea", err)
	}
}

// Auch wenn keine Eingabe bekannt ist, bleibt ein falsches Gebiet ein Fehler:
// die Pruefung darf nicht davon abhaengen, ob es Kandidaten gibt.
func TestMatchHabitatTypes_UnbekanntesGebietAuchOhneTreffer(t *testing.T) {
	svc := newMatchService(t)

	_, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"gbif:1"}, Typology: "eunis@2021", Level: 3, Limit: 10,
		Area: "XXX",
	})
	if !errors.Is(err, input.ErrUnknownArea) {
		t.Errorf("err = %v, erwartet input.ErrUnknownArea auch ohne Kandidaten", err)
	}
}
