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
