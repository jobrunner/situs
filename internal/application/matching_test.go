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
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
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

// Jeder Fehler des Index muss durchgereicht werden statt eine halbe Rangliste
// zu liefern — eine unvollstaendige Ordnung waere schlimmer als keine.
func TestMatchHabitatTypes_ReichtIndexfehlerDurch(t *testing.T) {
	boom := errors.New("index kaputt")

	t.Run("KnownAreaCodes", func(t *testing.T) {
		svc := newMatchService(t)
		svc.repo.(*fakeRepo).areasErr = boom
		_, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
			ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Area: "GER",
		})
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, erwartet den Indexfehler", err)
		}
	})

	t.Run("SpeciesRolesByConcept", func(t *testing.T) {
		svc := newMatchService(t)
		svc.repo.(*fakeRepo).speciesRolesErr = boom
		_, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
			ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3,
		})
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, erwartet den Indexfehler", err)
		}
	})

	t.Run("HabitatType", func(t *testing.T) {
		svc := newMatchService(t)
		svc.repo.(*fakeRepo).habitatTypeErr = boom
		_, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
			ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3,
		})
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, erwartet den Indexfehler", err)
		}
	})
}

// Eine BEKANNTE Typologie, zu der keine Artenzeile gehoert, ergibt eine leere
// Liste — kein Fehler, denn die Frage war beantwortbar. Der Unterschied zur
// unbekannten Typologie ist der Punkt: dort ist es ein Tippfehler.
func TestMatchHabitatTypes_BekannteTypologieOhneTrefferGibtLeereListe(t *testing.T) {
	svc := newMatchService(t)
	svc.repo.(*fakeRepo).typologies = append(svc.repo.(*fakeRepo).typologies,
		domain.Typology{ID: "eunis@2012", Scheme: "eunis", Version: "2012"})

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2012", Level: 3, Limit: 10,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) != 0 {
		t.Errorf("Matches = %d, erwartet 0", len(got.Matches))
	}
}

// Eine Ebene, die kein Kandidat hat, filtert alles weg — ebenfalls ohne Fehler.
func TestMatchHabitatTypes_LevelFiltertKandidaten(t *testing.T) {
	svc := newMatchService(t)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 2, Limit: 10,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) != 0 {
		t.Errorf("Matches = %d, erwartet 0 (alle Fixtures sind Level 3)", len(got.Matches))
	}
}

// Mit gueltigem Gebiet laeuft der Abdeckungspfad durch: der Typ, dessen Arten
// im Gebiet vorkommen, steht vor dem, dessen Arten anderswo wachsen.
func TestMatchHabitatTypes_GebietOrdnetUm(t *testing.T) {
	svc := newMatchService(t)
	repo := svc.repo.(*fakeRepo)
	repo.knownAreaCodes = map[string][]string{domain.SchemeWGSRPDL3: {"GER", "SPA"}}
	repo.distribution = []fakeDistribution{
		{ConceptID: "wcvp:c1", Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "SPA"}},
	}

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Area: "GER", Limit: 10,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) == 0 {
		t.Fatal("keine Treffer; ein Gebiet darf nicht ausschliessen, nur abwerten")
	}
}

// Bei gleichem Score entscheidet der Code — sonst waere die Reihenfolge von
// der Durchlaufreihenfolge einer Map abhaengig und damit von Lauf zu Lauf
// verschieden.
func TestMatchHabitatTypes_GleichstandWirdStabilGeordnet(t *testing.T) {
	repo := newFakeRepo()
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	level := 3
	for _, code := range []string{"T99", "T11", "T55"} {
		k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: code}
		repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: code})
		id, c := "wcvp:c1", 50.0
		repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
			Key: k, ConceptID: &id, VerbatimName: "c1", Role: "constant",
			Constancy: &c, Provenance: "observed",
		})
	}
	svc := NewQueryService(repo)

	var erste []string
	for lauf := 0; lauf < 5; lauf++ {
		got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
			ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 10,
		})
		if err != nil {
			t.Fatalf("MatchHabitatTypes: %v", err)
		}
		codes := make([]string, 0, len(got.Matches))
		for _, m := range got.Matches {
			codes = append(codes, m.Code)
		}
		if lauf == 0 {
			erste = codes
			if len(codes) != 3 || codes[0] != "T11" {
				t.Fatalf("Reihenfolge = %v, erwartet T11 zuerst (Code entscheidet bei Gleichstand)", codes)
			}
			continue
		}
		for i := range codes {
			if codes[i] != erste[i] {
				t.Fatalf("Lauf %d ergab %v, Lauf 0 ergab %v — die Ordnung ist nicht stabil", lauf, codes, erste)
			}
		}
	}
}

// Auch ein Fehler der Abdeckungsabfrage wird durchgereicht: eine Rangliste
// ohne den Gebietsterm waere eine andere Antwort als die gestellte Frage.
func TestMatchHabitatTypes_ReichtAbdeckungsfehlerDurch(t *testing.T) {
	boom := errors.New("abdeckung kaputt")
	svc := newMatchService(t)
	repo := svc.repo.(*fakeRepo)
	repo.knownAreaCodes = map[string][]string{domain.SchemeWGSRPDL3: {"GER"}}
	repo.coverageErr = boom

	_, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Area: "GER",
	})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, erwartet den Abdeckungsfehler", err)
	}
}

// Eine unbekannte Typologie ist ein Tippfehler, kein leeres Ergebnis: sonst
// ist "quatsch@1" nicht von "diese Arten passen nirgends" zu unterscheiden.
// Dieselbe Haltung wie beim Gebiet.
func TestMatchHabitatTypes_UnbekannteTypologieIstEinFehler(t *testing.T) {
	svc := newMatchService(t)

	_, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "quatsch@1", Level: 3, Limit: 10,
	})
	if !errors.Is(err, input.ErrUnknownTypology) {
		t.Errorf("err = %v, erwartet input.ErrUnknownTypology", err)
	}
}

// Eine doppelt genannte Art darf weder den Score druecken noch matched/of
// verfaelschen: die Zahl, mit der die Antwort ihre Reihenfolge begruendet,
// muss stimmen.
func TestMatchHabitatTypes_DoppelteEingabeZaehltEinmal(t *testing.T) {
	svc := newMatchService(t)
	einfach, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1", "wcvp:c2"}, Typology: "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	doppelt, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1", "wcvp:c2", "wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if doppelt.Matches[0].Of != einfach.Matches[0].Of {
		t.Errorf("of = %d bei doppelter Nennung, %d bei einfacher — die Dublette zaehlt mit",
			doppelt.Matches[0].Of, einfach.Matches[0].Of)
	}
	if doppelt.Matches[0].Score != einfach.Matches[0].Score {
		t.Errorf("Score %.3f vs %.3f — die Dublette kostet einen unberechtigten MISS",
			doppelt.Matches[0].Score, einfach.Matches[0].Score)
	}
	// Die Eingabe wird trotzdem vollstaendig zurueckgespiegelt, wie beim Batch.
	if len(doppelt.Input) != 3 {
		t.Errorf("Input = %d Eintraege, erwartet 3 — jede Eingabe wird gespiegelt", len(doppelt.Input))
	}
}

// Die Antwort nennt, welche Art wofuer gesprochen hat — im Gelaende die
// Anschlussfrage: welche der anderen Kennarten suche ich jetzt?
func TestMatchHabitatTypes_NenntDieTreffendenArten(t *testing.T) {
	svc := newMatchService(t)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1", "wcvp:c2"}, Typology: "eunis@2021", Level: 3, Limit: 1,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) == 0 {
		t.Fatal("keine Treffer")
	}
	if len(got.Matches[0].Species) != 2 {
		t.Fatalf("Species = %d, erwartet 2", len(got.Matches[0].Species))
	}
	for _, sp := range got.Matches[0].Species {
		if sp.ConceptID == "" || sp.Role == "" {
			t.Errorf("unvollstaendiger Eintrag: %+v", sp)
		}
	}
}

// Ohne typology faellt die Route auf die Vorgabe zurueck; die Pruefung darf
// dann nicht anschlagen.
func TestMatchHabitatTypes_LeereTypologieWirdNichtGeprueft(t *testing.T) {
	svc := newMatchService(t)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	// Ohne Typologie passt keine Artenzeile (sie tragen alle eunis@2021),
	// aber ein Fehler ist es nicht.
	if len(got.Matches) != 0 {
		t.Errorf("Matches = %d, erwartet 0", len(got.Matches))
	}
}

// Auch ein Fehler der Typologie-Abfrage wird durchgereicht.
func TestMatchHabitatTypes_ReichtTypologiefehlerDurch(t *testing.T) {
	boom := errors.New("typologien kaputt")
	svc := newMatchService(t)
	svc.repo.(*fakeRepo).typologyErr = boom

	_, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3,
	})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, erwartet den Typologiefehler", err)
	}
}

// constancy 100 heisst "in jeder Aufnahme dieses Typs" — das ist P = 1,0 und
// nicht 0,99. Ein stilles Kappen verschoebe die Rangfolge gegen einen
// Kandidaten mit echten 99. Gemessen fuehrt der Index solche Zeilen.
func TestMatchHabitatTypes_HundertProzentBleibenHundertProzent(t *testing.T) {
	repo := newFakeRepo()
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	level := 3
	setzen := func(code string, constancy float64) {
		k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: code}
		repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: code})
		id, c := "wcvp:c1", constancy
		repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
			Key: k, ConceptID: &id, VerbatimName: "c1", Role: "constant",
			Constancy: &c, Provenance: "observed",
		})
	}
	setzen("T01", 100) // voll stet
	setzen("T02", 99)  // fast voll stet
	svc := NewQueryService(repo)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) != 2 {
		t.Fatalf("Matches = %d, erwartet 2", len(got.Matches))
	}
	if got.Matches[0].Code != "T01" {
		t.Errorf("Rang 1 = %s, erwartet T01 — eine Stetigkeit von 100 darf nicht auf 99 gekappt werden",
			got.Matches[0].Code)
	}
	if got.Matches[0].Score == got.Matches[1].Score {
		t.Error("100 und 99 ergeben denselben Score; das Kappen macht sie ununterscheidbar")
	}
}

// Ein Wert ausserhalb 0..100 ist ein Datenfehler und wird sichtbar begrenzt,
// nicht still verrechnet.
func TestMatchHabitatTypes_UnplausibleStetigkeitWirdBegrenzt(t *testing.T) {
	repo := newFakeRepo()
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	level := 3
	k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T01"}
	repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: "T01"})
	id, c := "wcvp:c1", 150.0
	repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
		Key: k, ConceptID: &id, VerbatimName: "c1", Role: "constant",
		Constancy: &c, Provenance: "observed",
	})

	got, err := NewQueryService(repo).MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) != 1 {
		t.Fatalf("Matches = %d, erwartet 1", len(got.Matches))
	}
	// P darf 1 nicht ueberschreiten: log(1.5) waere positiv und der Kandidat
	// bekaeme einen Bonus fuer einen kaputten Wert.
	if got.Matches[0].Score > 0 {
		t.Errorf("Score = %.3f; eine Stetigkeit ueber 100 darf keinen Bonus ergeben",
			got.Matches[0].Score)
	}
}

// Der Fake muss dieselbe Definition rechnen wie SQLite, sonst koennen
// Anwendungstests mit einem Verhalten gruen sein, das die echte Ablage nicht
// hat. Genau der Zaehlfehler — Zeilen statt Konzepte — steckte im SQL und ist
// dort behoben.
func TestFakeRepo_AbdeckungZaehltKonzepteNichtZeilen(t *testing.T) {
	repo := newFakeRepo()
	level := 3
	k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T32"}
	repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: "T32"})
	// c1 traegt drei Rollen und kommt in GER vor, c2 eine Rolle und kommt
	// anderswo vor. Konzeptweise sind das 1 von 2 = 0,5. Zeilenweise waeren es
	// 3 von 4 = 0,75 — und genau daran wird der Unterschied sichtbar.
	eins, zwei := "wcvp:c1", "wcvp:c2"
	for _, role := range []string{"constant", "diagnostic", "dominant"} {
		repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
			Key: k, ConceptID: &eins, VerbatimName: "c1", Role: role, Provenance: "observed",
		})
	}
	repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
		Key: k, ConceptID: &zwei, VerbatimName: "c2", Role: "constant", Provenance: "observed",
	})
	repo.distribution = []fakeDistribution{
		{ConceptID: eins, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
		{ConceptID: zwei, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "SPA"}},
	}

	got, err := repo.HabitatAreaCoverage(context.Background(), []domain.HabitatTypeKey{k}, "GER")
	if err != nil {
		t.Fatalf("HabitatAreaCoverage: %v", err)
	}
	if v := got[k]; v < 0.49 || v > 0.51 {
		t.Errorf("Abdeckung = %.3f, erwartet 0.5 (ein Konzept von zweien) — "+
			"der Fake zaehlt Zeilen statt Konzepte", v)
	}
}

// Ein Typ ohne jeden Treffer erscheint nie — auch nicht mit perfekter
// Gebietsabdeckung. Rechnerisch haette er den besseren Score: 3*log(MISS)
// plus 3*log(1,0) = -11,74 schlaegt einen Kandidaten mit einem Treffer und
// schlechter Abdeckung (-16,92). Fachlich waere das falsch. Die Artenliste
// ist die Evidenz, das Gebiet nur ein Korrektiv — ein Typ, zu dem keine
// einzige notierte Art passt, ist kein Kandidat, so einleuchtend die Geografie
// auch sein mag.
func TestMatchHabitatTypes_GebietHebtKeinenTypOhneTreffer(t *testing.T) {
	repo := newFakeRepo()
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.knownAreaCodes = map[string][]string{domain.SchemeWGSRPDL3: {"GER"}}
	level := 3
	anlegen := func(code, concept, areaCode string) {
		k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: code}
		repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: code})
		id, c := concept, 90.0
		repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
			Key: k, ConceptID: &id, VerbatimName: concept, Role: "constant",
			Constancy: &c, Provenance: "observed",
		})
		repo.distribution = append(repo.distribution, fakeDistribution{
			ConceptID: concept, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: areaCode},
		})
	}
	anlegen("T01", "wcvp:treffer", "SPA") // Treffer, aber im Gebiet unplausibel
	anlegen("T99", "wcvp:fremd", "GER")   // kein Treffer, im Gebiet perfekt

	got, err := NewQueryService(repo).MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:treffer"}, Typology: "eunis@2021", Level: 3,
		Area: "GER", Limit: 50,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	for _, m := range got.Matches {
		if m.Code == "T99" {
			t.Errorf("T99 erscheint in der Liste, fuehrt aber keine der notierten Arten — " +
				"das Gebiet darf keinen Typ ohne Treffer heben")
		}
	}
	if len(got.Matches) != 1 || got.Matches[0].Code != "T01" {
		t.Errorf("Matches = %+v, erwartet allein T01", got.Matches)
	}
}

// Eine ausdrueckliche Stetigkeit von 0 heisst, die Art kommt in keiner
// Aufnahme dieses Typs vor. Das ist eine Angabe, keine fehlende Angabe — und
// sie macht den Typ nicht zum Kandidaten. Ein Typ, der die Art gar nicht
// fuehrt, und einer, der sie mit 0 fuehrt, sind fuer die Antwort dasselbe:
// beide erscheinen nicht.
func TestMatchHabitatTypes_AusdruecklicheNullIstKeinTreffer(t *testing.T) {
	repo := newFakeRepo()
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	level := 3
	setzen := func(code string, constancy *float64) {
		k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: code}
		repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: code})
		id := "wcvp:c1"
		repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
			Key: k, ConceptID: &id, VerbatimName: "c1", Role: "constant",
			Constancy: constancy, Provenance: "observed",
		})
	}
	null := 0.0
	setzen("T01", &null) // ausdrueckliche Null: kein Kandidat
	setzen("T02", nil)   // keine Angabe: Treffer mit dem Vorgabewert
	svc := NewQueryService(repo)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) != 1 || got.Matches[0].Code != "T02" {
		t.Fatalf("Matches = %+v, erwartet allein T02 — die Nullzeile ist kein Treffer", got.Matches)
	}
}

// species muss dieselbe Evidenz zeigen, die der Score gezaehlt hat: je Art
// einen Eintrag, und zwar die Zeile, die tatsaechlich zaehlte. Drei Zeilen
// fuer eine Art auszuweisen, waehrend matched sie einmal zaehlt, laesst die
// Antwort mehr Belege behaupten, als in die Rangfolge eingingen.
func TestMatchHabitatTypes_SpeciesZeigtJedeArtEinmal(t *testing.T) {
	repo := newFakeRepo()
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	level := 3
	k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T17"}
	repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: "T17"})
	id := "wcvp:c1"
	for _, r := range []struct {
		role      string
		constancy float64
	}{{"constant", 99}, {"dominant", 80}} {
		c := r.constancy
		repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
			Key: k, ConceptID: &id, VerbatimName: "c1", Role: r.role,
			Constancy: &c, Provenance: "observed",
		})
	}
	f := 31.0
	repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
		Key: k, ConceptID: &id, VerbatimName: "c1", Role: "diagnostic",
		Fidelity: &f, Provenance: "observed",
	})

	got, err := NewQueryService(repo).MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) != 1 {
		t.Fatalf("Matches = %d, erwartet 1", len(got.Matches))
	}
	m := got.Matches[0]
	if len(m.Species) != m.Matched {
		t.Errorf("species hat %d Eintraege, matched zaehlt %d — die Antwort weist mehr Belege aus, "+
			"als in die Rangfolge eingingen", len(m.Species), m.Matched)
	}
	// Ausgewiesen wird die Zeile, die der Score genutzt hat: die hoechste
	// Stetigkeit, plus den hoechsten Treuegrad derselben Art.
	if len(m.Species) == 1 {
		sp := m.Species[0]
		if sp.Constancy == nil || *sp.Constancy != 99 {
			t.Errorf("Constancy = %v, erwartet 99 (die Zeile, die zaehlte)", sp.Constancy)
		}
		if sp.Fidelity == nil || *sp.Fidelity != 31 {
			t.Errorf("Fidelity = %v, erwartet 31 (der hoechste Treuegrad derselben Art)", sp.Fidelity)
		}
	}
}

// Die Verdichtung der Belege in allen Richtungen: kommt die staerkere Zeile
// zuerst, bleibt sie stehen und nimmt nur den hoeheren Treuegrad auf; kommt
// sie spaeter, ersetzt sie die schwaechere.
func TestMatchHabitatTypes_BelegNimmtDieStaerkereZeileUnabhaengigVonDerReihenfolge(t *testing.T) {
	baue := func(reihenfolge []struct {
		role      string
		constancy float64
		fidelity  float64
	}) input.MatchSpecies {
		t.Helper()
		repo := newFakeRepo()
		repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
		level := 3
		k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T17"}
		repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: "T17"})
		id := "wcvp:c1"
		for _, r := range reihenfolge {
			row := domain.SpeciesRole{
				Key: k, ConceptID: &id, VerbatimName: "c1", Role: r.role, Provenance: "observed",
			}
			if r.constancy > 0 {
				c := r.constancy
				row.Constancy = &c
			}
			if r.fidelity > 0 {
				f := r.fidelity
				row.Fidelity = &f
			}
			repo.speciesRoles = append(repo.speciesRoles, row)
		}
		got, err := NewQueryService(repo).MatchHabitatTypes(context.Background(), input.MatchRequest{
			ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 1,
		})
		if err != nil {
			t.Fatalf("MatchHabitatTypes: %v", err)
		}
		if len(got.Matches) != 1 || len(got.Matches[0].Species) != 1 {
			t.Fatalf("erwartet ein Match mit einem Beleg, bekam %+v", got.Matches)
		}
		return got.Matches[0].Species[0]
	}

	typ := []struct {
		role      string
		constancy float64
		fidelity  float64
	}{{"diagnostic", 0, 31}, {"constant", 99, 0}}
	vorwaerts := baue(typ)
	rueckwaerts := baue([]struct {
		role      string
		constancy float64
		fidelity  float64
	}{typ[1], typ[0]})

	for name, sp := range map[string]input.MatchSpecies{"schwach zuerst": vorwaerts, "stark zuerst": rueckwaerts} {
		if sp.Constancy == nil || *sp.Constancy != 99 {
			t.Errorf("%s: Constancy = %v, erwartet 99", name, sp.Constancy)
		}
		if sp.Fidelity == nil || *sp.Fidelity != 31 {
			t.Errorf("%s: Fidelity = %v, erwartet 31", name, sp.Fidelity)
		}
	}
}

// hoehere waehlt den groesseren von zwei optionalen Werten und behandelt das
// Fehlen als "kein Wert", nicht als Null — sonst schluege eine fehlende
// Angabe eine vorhandene.
func TestHoehere(t *testing.T) {
	zwei, fuenf := 2.0, 5.0
	faelle := []struct {
		name string
		a, b *float64
		want *float64
	}{
		{"beide fehlen", nil, nil, nil},
		{"nur b", nil, &fuenf, &fuenf},
		{"nur a", &zwei, nil, &zwei},
		{"b groesser", &zwei, &fuenf, &fuenf},
		{"a groesser", &fuenf, &zwei, &fuenf},
		{"gleich", &zwei, &zwei, &zwei},
	}
	for _, f := range faelle {
		t.Run(f.name, func(t *testing.T) {
			got := hoehere(f.a, f.b)
			switch {
			case f.want == nil && got != nil:
				t.Errorf("got = %v, erwartet nil", *got)
			case f.want != nil && got == nil:
				t.Errorf("got = nil, erwartet %v", *f.want)
			case f.want != nil && *got != *f.want:
				t.Errorf("got = %v, erwartet %v", *got, *f.want)
			}
		})
	}
}

// Der Fake muss auch das Schema so filtern wie SQLite: die echte Ablage joint
// auf area_scheme = wgsrpd_l3. Ohne das zaehlte eine evc_territory-Zeile mit
// demselben Code hier mit und dort nicht — Anwendungstests koennten mit einem
// Abdeckungsverhalten gruen sein, das die echte Ablage nicht hat.
func TestFakeRepo_AbdeckungFiltertDasSchema(t *testing.T) {
	repo := newFakeRepo()
	level := 3
	k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T17"}
	repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: "T17"})
	id := "wcvp:c1"
	repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
		Key: k, ConceptID: &id, VerbatimName: "c1", Role: "constant", Provenance: "observed",
	})
	// Die Art kommt in einem EVC-Territorium namens "GER" vor, nicht im
	// WGSRPD-Gebiet GER. Fuer die Artverbreitung zaehlt allein wgsrpd_l3.
	repo.distribution = []fakeDistribution{
		{ConceptID: id, Area: domain.Area{Scheme: domain.SchemeEVCTerritory, Code: "GER"}},
	}

	got, err := repo.HabitatAreaCoverage(context.Background(), []domain.HabitatTypeKey{k}, "GER")
	if err != nil {
		t.Fatalf("HabitatAreaCoverage: %v", err)
	}
	if v, ok := got[k]; ok && v > 0 {
		t.Errorf("Abdeckung = %.3f; eine Zeile aus einem anderen Gebietsschema darf nicht zaehlen", v)
	}
}

// level 0 heisst "keine Ebenenpruefung". Der Unterschied zu level 3 ist an
// einem Typ sichtbar, der eine andere Ebene traegt: mit 0 bleibt er drin.
func TestMatchHabitatTypes_LevelNullFiltertNicht(t *testing.T) {
	repo := newFakeRepo()
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	zwei, drei := 2, 3
	for _, e := range []struct {
		code  string
		level *int
	}{{"T17", &drei}, {"T1", &zwei}} {
		k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: e.code}
		repo.types = append(repo.types, domain.HabitatType{Key: k, Level: e.level, NameEN: e.code})
		id, c := "wcvp:c1", 90.0
		repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
			Key: k, ConceptID: &id, VerbatimName: "c1", Role: "constant",
			Constancy: &c, Provenance: "observed",
		})
	}
	svc := NewQueryService(repo)

	mitDrei, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 50,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	mitNull, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 0, Limit: 50,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(mitDrei.Matches) != 1 {
		t.Errorf("level 3 ergab %d Treffer, erwartet 1 (nur T17)", len(mitDrei.Matches))
	}
	if len(mitNull.Matches) != 2 {
		t.Errorf("level 0 ergab %d Treffer, erwartet 2 — 0 heisst keine Ebenenpruefung",
			len(mitNull.Matches))
	}
}

// limit genau auf der Zahl der Kandidaten darf nicht kuerzen.
func TestMatchHabitatTypes_LimitAufDerGrenzeKuerztNicht(t *testing.T) {
	svc := newMatchService(t)
	alle, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1", "wcvp:c2"}, Typology: "eunis@2021", Level: 3, Limit: 50,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	n := len(alle.Matches)
	genau, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1", "wcvp:c2"}, Typology: "eunis@2021", Level: 3, Limit: n,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(genau.Matches) != n {
		t.Errorf("limit %d kuerzte auf %d Treffer", n, len(genau.Matches))
	}
}

// Zwei Zeilen derselben Art mit GLEICHER Wahrscheinlichkeit: die zuerst
// eingetroffene bleibt stehen, damit die Antwort nicht von der Zeilenfolge
// der Ablage abhaengt.
func TestMatchHabitatTypes_BelegBleibtBeiGleichstandStehen(t *testing.T) {
	repo := newFakeRepo()
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	level := 3
	k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T17"}
	repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: "T17"})
	id := "wcvp:c1"
	for _, role := range []string{"constant", "dominant"} {
		c := 80.0 // beide identisch
		repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
			Key: k, ConceptID: &id, VerbatimName: "c1", Role: role,
			Constancy: &c, Provenance: "observed",
		})
	}

	got, err := NewQueryService(repo).MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) != 1 || len(got.Matches[0].Species) != 1 {
		t.Fatalf("erwartet ein Match mit einem Beleg, bekam %+v", got.Matches)
	}
	if got.Matches[0].Species[0].Role != "constant" {
		t.Errorf("Rolle = %q, erwartet constant — bei Gleichstand bleibt die erste Zeile stehen",
			got.Matches[0].Species[0].Role)
	}
}

// Eine Stetigkeit von genau 0 ergibt genau den MISS-Wert — nicht irgendetwas
// Kleines, sondern denselben Wert wie eine Art, die der Typ gar nicht fuehrt.
func TestRollenWahrscheinlichkeit_NullErgibtGenauMiss(t *testing.T) {
	null := 0.0
	if got := rollenWahrscheinlichkeit(domain.SpeciesRole{Constancy: &null}); got != domain.MatchMiss {
		t.Errorf("bei constancy 0 = %v, erwartet MatchMiss %v", got, domain.MatchMiss)
	}
	negativ := -5.0
	if got := rollenWahrscheinlichkeit(domain.SpeciesRole{Constancy: &negativ}); got != domain.MatchMiss {
		t.Errorf("bei constancy -5 = %v, erwartet MatchMiss %v", got, domain.MatchMiss)
	}
	winzig := 0.5 // 0,5 Prozent, also P = 0,005
	if got := rollenWahrscheinlichkeit(domain.SpeciesRole{Constancy: &winzig}); got != 0.005 {
		t.Errorf("bei constancy 0,5 = %v, erwartet 0.005 — ein kleiner Wert ist keine Null", got)
	}
	if got := rollenWahrscheinlichkeit(domain.SpeciesRole{}); got != domain.MatchDefaultP {
		t.Errorf("ohne constancy = %v, erwartet MatchDefaultP %v", got, domain.MatchDefaultP)
	}
}

// limit 0 heisst "keine Begrenzung" — der Use-Case kuerzt dann nicht. Der
// Handler laesst 0 gar nicht erst durch (400), aber der Use-Case ist auch
// ohne ihn benutzbar und darf bei 0 nicht die leere Liste liefern.
func TestMatchHabitatTypes_LimitNullKuerztNicht(t *testing.T) {
	svc := newMatchService(t)
	req := input.MatchRequest{ConceptIDs: []string{"wcvp:c1", "wcvp:c2"}, Typology: "eunis@2021", Level: 3}

	req.Limit = 50
	alle, err := svc.MatchHabitatTypes(context.Background(), req)
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	req.Limit = 0
	ohne, err := svc.MatchHabitatTypes(context.Background(), req)
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(ohne.Matches) != len(alle.Matches) {
		t.Errorf("limit 0 ergab %d Treffer, limit 50 ergab %d — 0 darf nicht auf null kuerzen",
			len(ohne.Matches), len(alle.Matches))
	}
}

// Dieselbe ID mehrfach zu nennen darf den Index nicht mehrfach befragen. Die
// Antwort spiegelt die Eingabe trotzdem vollstaendig zurueck.
func TestMatchHabitatTypes_FragtJedeIDNurEinmalAb(t *testing.T) {
	svc := newMatchService(t)
	repo := svc.repo.(*fakeRepo)

	ids := make([]string, 20)
	for i := range ids {
		ids[i] = "wcvp:c1"
	}
	vorher := repo.speciesRolesByConceptCalls
	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: ids, Typology: "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if n := repo.speciesRolesByConceptCalls - vorher; n != 1 {
		t.Errorf("%d Abfragen fuer 20-mal dieselbe ID, erwartet 1", n)
	}
	if len(got.Input) != 20 {
		t.Errorf("Input = %d Eintraege, erwartet 20 — jede Eingabe wird gespiegelt", len(got.Input))
	}
	if got.Matches[0].Of != 1 {
		t.Errorf("of = %d, erwartet 1", got.Matches[0].Of)
	}
}

// Auch eine unbekannte ID, die doppelt genannt wird, behaelt bei jedem
// Vorkommen ihren Grund — sonst waere die zweite Nennung stillschweigend
// "bekannt", nur weil die erste schon abgefragt wurde.
func TestMatchHabitatTypes_UnbekannteIDBehaeltIhrenGrundAuchDoppelt(t *testing.T) {
	svc := newMatchService(t)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:fehlt", "wcvp:fehlt", "wcvp:c1"},
		Typology:   "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Input) != 3 {
		t.Fatalf("Input = %d Eintraege, erwartet 3", len(got.Input))
	}
	for i, e := range got.Input[:2] {
		if e.Known {
			t.Errorf("Input[%d] ist als bekannt gemeldet, die ID gibt es aber nicht", i)
		}
		if e.Reason != input.ReasonUnknownConcept {
			t.Errorf("Input[%d].Reason = %q, erwartet unknown_concept", i, e.Reason)
		}
	}
	if got.Matches[0].Of != 1 {
		t.Errorf("of = %d, erwartet 1 — nur wcvp:c1 ist bekannt", got.Matches[0].Of)
	}
}

// strconv.ParseFloat akzeptiert "NaN" und "Inf". Solche Werte duerfen nicht
// in den Index gelangen: ein NaN im Score macht die Antwort unserialisierbar,
// und zwar erst NACH dem 200 — der Aufrufer bekaeme einen abgebrochenen Body.
// Der richtige Ort dafuer ist der Ingest, wo ein solcher Wert ein Datenfehler
// ist, nicht das Ausliefern.
func TestParseOptionalFloat_WeistNichtEndlicheWerteZurueck(t *testing.T) {
	for _, roh := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf", "Infinity"} {
		if _, err := parseOptionalFloat(roh); err == nil {
			t.Errorf("parseOptionalFloat(%q) = kein Fehler; ein nicht endlicher Messwert ist ein Datenfehler", roh)
		}
	}
	for _, roh := range []string{"", "0", "-40", "99.5"} {
		if _, err := parseOptionalFloat(roh); err != nil {
			t.Errorf("parseOptionalFloat(%q) = %v, erwartet keinen Fehler", roh, err)
		}
	}
}

// Eine Zeile mit ausdruecklicher Stetigkeit 0 sagt: die Art kommt in diesem
// Typ NICHT vor. Der Score behandelt sie folgerichtig als Fehltreffer — dann
// darf sie aber auch nicht als Treffer gezaehlt, als Beleg ausgewiesen oder
// zum Grund werden, dass der Typ ueberhaupt Kandidat wird.
func TestMatchHabitatTypes_NullstetigkeitIstKeinTrefferUndKeinBeleg(t *testing.T) {
	repo := newFakeRepo()
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	level := 3
	legeAn := func(code string, constancy float64) {
		k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: code}
		repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: code})
		id, c := "wcvp:c1", constancy
		repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
			Key: k, ConceptID: &id, VerbatimName: "c1", Role: "constant",
			Constancy: &c, Provenance: "observed",
		})
	}
	legeAn("T01", 90) // echter Treffer
	legeAn("T99", 0)  // ausdrueckliches Nicht-Vorkommen
	svc := NewQueryService(repo)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 50,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	for _, m := range got.Matches {
		if m.Code == "T99" {
			t.Errorf("T99 steht in der Liste, fuehrt die Art aber mit Stetigkeit 0 — " +
				"ein ausdrueckliches Nicht-Vorkommen macht keinen Kandidaten")
		}
	}
	if len(got.Matches) != 1 || got.Matches[0].Code != "T01" {
		t.Fatalf("Matches = %+v, erwartet allein T01", got.Matches)
	}
	if got.Matches[0].Matched != 1 {
		t.Errorf("matched = %d, erwartet 1", got.Matches[0].Matched)
	}
}

// Fuehrt derselbe Typ die Art einmal mit 0 und einmal mit einer echten
// Stetigkeit, zaehlt die echte — und nur sie erscheint als Beleg.
func TestMatchHabitatTypes_NullstetigkeitVerdraengtDenEchtenBelegNicht(t *testing.T) {
	repo := newFakeRepo()
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	level := 3
	k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T17"}
	repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: "T17"})
	id := "wcvp:c1"
	for _, c := range []float64{0, 70} {
		wert := c
		rolle := "dominant"
		if c == 0 {
			rolle = "constant"
		}
		repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
			Key: k, ConceptID: &id, VerbatimName: "c1", Role: rolle,
			Constancy: &wert, Provenance: "observed",
		})
	}

	got, err := NewQueryService(repo).MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) != 1 || len(got.Matches[0].Species) != 1 {
		t.Fatalf("erwartet ein Match mit einem Beleg, bekam %+v", got.Matches)
	}
	sp := got.Matches[0].Species[0]
	if sp.Constancy == nil || *sp.Constancy != 70 {
		t.Errorf("Beleg-Constancy = %v, erwartet 70 — die Nullzeile darf nicht ausgewiesen werden", sp.Constancy)
	}
}

// Die Grenze, an der mein erster Anlauf danebenlag: MatchMiss ist 0,02, und
// eine Stetigkeit von 2 Prozent ergibt normalisiert exakt denselben Wert. Auf
// den normalisierten Wert zu pruefen verwirft damit eine legitime, nur sehr
// seltene Art als Nicht-Vorkommen. Entschieden wird am ROHWERT.
func TestMatchHabitatTypes_ZweiProzentIstEinTrefferKeineNull(t *testing.T) {
	repo := newFakeRepo()
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	level := 3
	setzen := func(code string, constancy float64) {
		k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: code}
		repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: code})
		id, c := "wcvp:c1", constancy
		repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
			Key: k, ConceptID: &id, VerbatimName: "c1", Role: "constant",
			Constancy: &c, Provenance: "observed",
		})
	}
	setzen("T02", 2) // zwei Prozent: selten, aber vorhanden
	setzen("T00", 0) // ausdrueckliches Nicht-Vorkommen

	got, err := NewQueryService(repo).MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 50,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) != 1 {
		t.Fatalf("Matches = %+v, erwartet genau T02", got.Matches)
	}
	if got.Matches[0].Code != "T02" {
		t.Errorf("Rang 1 = %s, erwartet T02 — zwei Prozent sind ein Treffer, keine Null",
			got.Matches[0].Code)
	}
	if len(got.Matches[0].Species) != 1 {
		t.Errorf("species = %d Eintraege, erwartet 1 — die Art ist belegt",
			len(got.Matches[0].Species))
	}
}
