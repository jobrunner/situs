# Habitat-Matching Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `POST /v1/habitat-types/match` nimmt eine Liste von Konzept-IDs und liefert eine nach Log-Likelihood sortierte Rangliste der Habitattypen.

**Architecture:** Der Score ist ein naives Bayes über `P(Art | Habitat)`, wobei `constancy/100` diese Wahrscheinlichkeit bereits ist. Die Kandidatenmenge entsteht aus den vorhandenen `SpeciesRolesByConcept`-Abfragen — ein Habitat ohne jede Eingabe-Art hat den konstanten Score `k·log(MISS)` und ist damit rangneutral, muss also nie berechnet werden. Die Gebietsabdeckung kommt aus einer neuen Aggregat-Abfrage über alle Kandidaten auf einmal.

**Tech Stack:** Go 1.26, stdlib, `modernc.org/sqlite`, `gorilla/mux`. Keine neue Abhängigkeit.

**Spec:** `docs/superpowers/specs/2026-09-26-habitat-matching-design.md`

## Global Constraints

- Go-Abhängigkeiten unverändert — keine neue direkte Dependency (`gomodguard_v2` würde sie abweisen).
- Die Leseseite bleibt autark: Eingabe sind **Konzept-IDs**, niemals verbatim Namen; `internal/app` darf den hostus-Adapter nicht importieren (`internal/app/arch_test.go`).
- Fehlerhülle: nur `INVALID_QUERY`, `NOT_FOUND`, `INTERNAL_ERROR`.
- Jede Route deklariert `.Methods()` und steht in **beiden** byte-identischen OpenAPI-Kopien.
- SQL sind statische Strings mit `?`-Platzhaltern, niemals konkateniert (gosec G201/G202).
- `make verify` grün vor jedem Commit — über eine Datei prüfen, nie `| tail` mit `&&` (der Exit-Code wäre der von `tail`).
- Zero `//nolint`, zero `#nosec`, zero `TODO`/`FIXME`.
- Antwort trägt **Rang und rohen Score**, ausdrücklich keine Prozente.

## Review Focus

1. **Leere `concept_ids`-Liste** → `INVALID_QUERY`, nicht eine Rangliste aus dem Nichts (Task 4 pinnt es).
2. **Alle Eingabe-IDs unbekannt** → `200` mit leerer `matches`-Liste und jedem `input`-Eintrag `known: false`; kein 404, denn die Frage war beantwortbar (Task 3).
3. **Ein Habitattyp führt die Art zweimal in derselben Rolle** (die zwei bekannten Duplikate, `R1N`/`wcvp:concept:2570774`) → die Art darf den Score nicht doppelt heben (Task 1).
4. **`area` mit unbekanntem Code** → `INVALID_QUERY`, nicht stillschweigend ignoriert (Task 4).
5. **`limit` außerhalb 1–50** → `INVALID_QUERY`; `limit` größer als die Kandidatenzahl liefert einfach alle (Task 4).

---

### Task 1: Der Score im Domain-Paket

**Files:**
- Create: `internal/domain/matching.go`
- Test: `internal/domain/matching_test.go`

**Interfaces:**
- Produces: `MatchParams` (Konstanten), `type MatchCandidate struct`, `func ScoreCandidate(c MatchCandidate, inputCount int) float64`

- [ ] **Step 1: Write the failing test**

```go
package domain_test

import (
	"math"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// constancy IS P(species | habitat): 99 means the species occurs in 99 % of
// this type's relevés. A species the type does not list costs MISS, which is
// the whole point — a hard AND filter loses the true habitat in 96 % of cases
// as soon as one unlisted species is named.
func TestScoreCandidate_FehlendeArtKostetMissTerm(t *testing.T) {
	mit := domain.MatchCandidate{Hits: []domain.MatchHit{{P: 0.99}}}
	ohne := domain.MatchCandidate{}

	sMit := domain.ScoreCandidate(mit, 1)
	sOhne := domain.ScoreCandidate(ohne, 1)

	if sMit <= sOhne {
		t.Errorf("Treffer (%.3f) muss besser bewertet sein als kein Treffer (%.3f)", sMit, sOhne)
	}
	if want := math.Log(domain.MatchMiss); math.Abs(sOhne-want) > 1e-9 {
		t.Errorf("ohne Treffer = %.5f, erwartet log(MISS) = %.5f", sOhne, want)
	}
}

// Dieselbe Art zweimal in derselben Rolle gibt es im Index zwei Mal
// (R1N/wcvp:concept:2570774). Sie darf den Score nicht doppelt heben.
func TestScoreCandidate_ZaehltJedeArtNurEinmal(t *testing.T) {
	einmal := domain.MatchCandidate{Hits: []domain.MatchHit{{ConceptID: "c1", P: 0.9}}}
	doppelt := domain.MatchCandidate{Hits: []domain.MatchHit{
		{ConceptID: "c1", P: 0.9}, {ConceptID: "c1", P: 0.3},
	}}

	if a, b := domain.ScoreCandidate(einmal, 1), domain.ScoreCandidate(doppelt, 1); math.Abs(a-b) > 1e-9 {
		t.Errorf("doppelte Zeile derselben Art aendert den Score: %.5f vs %.5f", a, b)
	}
}

// Von zwei Zeilen derselben Art gilt die hoechste Wahrscheinlichkeit: die Art
// IST da, und die guenstigste Rolle beschreibt das am besten.
func TestScoreCandidate_NimmtDieHoechsteWahrscheinlichkeitJeArt(t *testing.T) {
	c := domain.MatchCandidate{Hits: []domain.MatchHit{
		{ConceptID: "c1", P: 0.2}, {ConceptID: "c1", P: 0.8},
	}}
	want := math.Log(0.8)
	if got := domain.ScoreCandidate(c, 1); math.Abs(got-want) > 1e-9 {
		t.Errorf("Score = %.5f, erwartet log(0.8) = %.5f", got, want)
	}
}

// Der Treuegrad hebt eine Kennart ueber eine Begleitart gleicher Stetigkeit.
func TestScoreCandidate_TreuegradHebtDieKennart(t *testing.T) {
	ohne := domain.MatchCandidate{Hits: []domain.MatchHit{{ConceptID: "c1", P: 0.5}}}
	mit := domain.MatchCandidate{Hits: []domain.MatchHit{{ConceptID: "c1", P: 0.5, Fidelity: 40}}}

	if domain.ScoreCandidate(mit, 1) <= domain.ScoreCandidate(ohne, 1) {
		t.Error("Fidelity erhoeht den Score nicht")
	}
}

// Ein Gebiet, in dem die Arten des Typs kaum vorkommen, drueckt ihn nach
// hinten — aber es schliesst ihn nicht aus.
func TestScoreCandidate_GebietsabdeckungDruecktOhneAuszuschliessen(t *testing.T) {
	gut := domain.MatchCandidate{Hits: []domain.MatchHit{{ConceptID: "c1", P: 0.9}}, AreaCoverage: 0.9, HasArea: true}
	schlecht := domain.MatchCandidate{Hits: []domain.MatchHit{{ConceptID: "c1", P: 0.9}}, AreaCoverage: 0.01, HasArea: true}

	s := domain.ScoreCandidate(schlecht, 1)
	if s >= domain.ScoreCandidate(gut, 1) {
		t.Error("geringe Gebietsabdeckung senkt den Score nicht")
	}
	if math.IsInf(s, -1) || math.IsNaN(s) {
		t.Errorf("Score = %v; eine Abdeckung von 0 darf nicht ausschliessen", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run ScoreCandidate -v`
Expected: FAIL — `undefined: domain.MatchCandidate`

- [ ] **Step 3: Write minimal implementation**

```go
package domain

import "math"

// Die vier Parameter des Matchings. Gesetzt, nicht gelernt — deshalb als
// benannte Konstanten, damit eine spaetere Kalibrierung an echten Aufnahmen
// eine Aenderung an einer Stelle ist. Begruendung je Wert in
// docs/superpowers/specs/2026-09-26-habitat-matching-design.md.
const (
	// MatchMiss ist P(Art | Habitat) fuer eine Art, die der Typ nicht fuehrt.
	// log(0.02) ~= -3.9: spuerbar, aber nicht ausschliessend. Das ist der Kern
	// des Verfahrens — ein harter UND-Filter verliert bei einer einzigen
	// Stoerart in 96 % der Faelle den richtigen Typ.
	MatchMiss = 0.02
	// MatchDefaultP gilt fuer Zeilen, die nur als diagnostic gefuehrt werden
	// und deshalb fidelity statt constancy tragen.
	MatchDefaultP = 0.35
	// MatchFidelityWeight hebt eine Kennart ueber eine Begleitart.
	MatchFidelityWeight = 0.012
	// MatchAreaWeight ist stark genug, um geografisch Unmoegliches zu
	// verdraengen, und zu schwach, um Plausibles zu unterdruecken.
	MatchAreaWeight = 3.0
	// matchAreaFloor haelt log() von der Null fern.
	matchAreaFloor = 0.02
)

// MatchHit ist eine Artenzeile des Kandidaten, die zur Eingabe passt.
type MatchHit struct {
	ConceptID string
	P         float64 // P(Art | Habitat)
	Fidelity  float64 // 0, wenn die Zeile keinen Treuegrad fuehrt
}

// MatchCandidate ist ein Habitattyp mit seinen Treffern zur Eingabe.
type MatchCandidate struct {
	Key          HabitatTypeKey
	Hits         []MatchHit
	AreaCoverage float64
	HasArea      bool
}

// ScoreCandidate berechnet den Log-Likelihood. inputCount ist die Zahl der
// bekannten Eingabe-Arten; jede, die der Typ nicht fuehrt, kostet log(MISS).
func ScoreCandidate(c MatchCandidate, inputCount int) float64 {
	// Je Art gilt die hoechste Wahrscheinlichkeit: die Art IST da, und die
	// guenstigste Rolle beschreibt das am besten. Zugleich verhindert das,
	// dass die zwei bekannten Doppelzeilen den Score doppelt heben.
	best := make(map[string]MatchHit, len(c.Hits))
	for _, h := range c.Hits {
		if cur, ok := best[h.ConceptID]; !ok || h.P > cur.P {
			best[h.ConceptID] = h
		}
	}

	score := 0.0
	for _, h := range best {
		score += math.Log(h.P)
		score += MatchFidelityWeight * h.Fidelity
	}
	for i := len(best); i < inputCount; i++ {
		score += math.Log(MatchMiss)
	}
	if c.HasArea {
		score += MatchAreaWeight * math.Log(math.Max(c.AreaCoverage, matchAreaFloor))
	}
	return score
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/ -run ScoreCandidate -v`
Expected: PASS, alle fünf

- [ ] **Step 5: Commit**

```bash
git add internal/domain/matching.go internal/domain/matching_test.go
git commit -m "feat(domain): der Log-Likelihood-Score fuer das Habitat-Matching"
```

---

### Task 2: Gebietsabdeckung im Repository

**Files:**
- Create: `internal/adapters/sqlite/read_matching.go`
- Modify: `internal/ports/output/repository.go` (Methode zur Schnittstelle)
- Test: `internal/adapters/sqlite/read_matching_test.go`

**Interfaces:**
- Consumes: `domain.HabitatTypeKey`
- Produces: `HabitatAreaCoverage(ctx context.Context, keys []domain.HabitatTypeKey, areaCode string) (map[domain.HabitatTypeKey]float64, error)`

- [ ] **Step 1: Write the failing test**

```go
package sqlite_test

import (
	"context"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// Die Abdeckung ist der Anteil der Arten eines Typs, die im Gebiet verbreitet
// sind. Sie ist eine Plausibilitaetspruefung, kein Filter: ein Typ mit
// Abdeckung 0 bleibt in der Antwort, er rutscht nur nach hinten.
func TestHabitatAreaCoverage_AnteilDerImGebietVerbreitetenArten(t *testing.T) {
	db := newTestDB(t)
	seedHabitatWithSpecies(t, db, "T17", []string{"c1", "c2", "c3", "c4"})
	seedDistribution(t, db, map[string][]string{
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
	db := newTestDB(t)
	seedHabitatWithSpecies(t, db, "U11", []string{"c9"})

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
	db := newTestDB(t)
	got, err := db.HabitatAreaCoverage(context.Background(), nil, "GER")
	if err != nil {
		t.Fatalf("HabitatAreaCoverage: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Ergebnis = %v, erwartet leer", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/sqlite/ -run HabitatAreaCoverage -v`
Expected: FAIL — `db.HabitatAreaCoverage undefined`

- [ ] **Step 3: Write minimal implementation**

`internal/adapters/sqlite/read_matching.go`:

```go
package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
)

// HabitatAreaCoverage liefert je Schluessel den Anteil der Arten, die im
// Gebiet verbreitet sind. Typen, zu denen keine einzige Art eine
// Verbreitungsangabe traegt, fehlen im Ergebnis: unbeurteilbar ist nicht
// dasselbe wie unplausibel.
//
// Eine Abfrage fuer alle Kandidaten statt einer je Kandidat. Die
// Platzhalterliste wird aus der ANZAHL der Schluessel gebaut, nie aus ihren
// Werten — die bleiben gebundene Parameter (gosec G201).
func (d *DB) HabitatAreaCoverage(ctx context.Context, keys []domain.HabitatTypeKey, areaCode string) (map[domain.HabitatTypeKey]float64, error) {
	out := map[domain.HabitatTypeKey]float64{}
	if len(keys) == 0 || areaCode == "" {
		return out, nil
	}

	args := make([]any, 0, len(keys)*2+1)
	args = append(args, areaCode)
	for _, k := range keys {
		args = append(args, string(k.Typology), k.Code)
	}
	pairs := strings.TrimSuffix(strings.Repeat("(?,?),", len(keys)), ",")

	rows, err := d.QueryContext(ctx, `
		SELECT s.typology_id, s.code,
		       CAST(SUM(CASE WHEN d.area_code IS NOT NULL THEN 1 ELSE 0 END) AS REAL)
		         / COUNT(DISTINCT s.concept_id) AS coverage
		FROM species_role s
		LEFT JOIN species_distribution d
		       ON d.concept_id = s.concept_id AND d.area_code = ?
		WHERE s.concept_id IS NOT NULL AND s.concept_id <> ''
		  AND (s.typology_id, s.code) IN (VALUES `+pairs+`)
		  AND EXISTS (SELECT 1 FROM species_distribution x WHERE x.concept_id = s.concept_id)
		GROUP BY s.typology_id, s.code`, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying area coverage: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var typology, code string
		var cov float64
		if err := rows.Scan(&typology, &code, &cov); err != nil {
			return nil, fmt.Errorf("sqlite: scanning area coverage: %w", err)
		}
		out[domain.HabitatTypeKey{Typology: domain.TypologyID(typology), Code: code}] = cov
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading area coverage: %w", err)
	}
	return out, nil
}
```

Dazu in `internal/ports/output/repository.go` zur `Repository`-Schnittstelle:

```go
	// HabitatAreaCoverage returns, per key, the share of the type's species
	// that occur in areaCode. Keys whose species carry no distribution data at
	// all are absent from the result: unjudgeable is not implausible.
	HabitatAreaCoverage(ctx context.Context, keys []domain.HabitatTypeKey, areaCode string) (map[domain.HabitatTypeKey]float64, error)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/sqlite/ -run HabitatAreaCoverage -v`
Expected: PASS. Schlägt der `VALUES`-Ausdruck fehl, prüfe mit
`sqlite3 situs.sqlite "SELECT 1 WHERE ('a','b') IN (VALUES ('a','b'));"`, ob die
Zeilenwert-Syntax verfügbar ist; falls nicht, ersetze sie durch eine
`OR`-Kette aus `(typology_id = ? AND code = ?)`, ebenfalls aus der Anzahl
gebaut.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/sqlite/read_matching.go internal/adapters/sqlite/read_matching_test.go internal/ports/output/repository.go
git commit -m "feat(sqlite): Gebietsabdeckung je Habitattyp in einer Abfrage"
```

---

### Task 3: Der Use-Case

**Files:**
- Create: `internal/application/matching.go`
- Modify: `internal/ports/input/services.go` (Port + Antworttypen)
- Test: `internal/application/matching_test.go`

**Interfaces:**
- Consumes: `domain.ScoreCandidate`, `Repository.SpeciesRolesByConcept`, `Repository.HabitatAreaCoverage`
- Produces: `MatchHabitatTypes(ctx context.Context, req input.MatchRequest) (input.MatchResult, error)`

- [ ] **Step 1: Write the failing test**

```go
package application_test

import (
	"context"
	"testing"

	"github.com/jobrunner/situs/internal/ports/input"
)

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

// Sind alle Eingaben unbekannt, ist die Antwort leer — aber 200, nicht 404.
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
// haetten den identischen Score k*log(MISS) und waeren rangneutral — sie zu
// berechnen kostet nur Zeit.
func TestMatchHabitatTypes_NurTypenMitMindestensEinemTreffer(t *testing.T) {
	svc := newMatchService(t)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{"wcvp:c1"}, Typology: "eunis@2021", Level: 3, Limit: 50,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
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
```

Dazu der Fixture-Helfer `newMatchService` in derselben Datei: ein Fake-Repository
mit `T17` (führt c1 constancy 99, c2 constancy 80, c3 diagnostic fidelity 31),
`T18` (führt nur c1 constancy 40) und `R1A` (führt c2 constancy 20) — gebaut
nach dem Muster von `internal/application/fakerepo_search_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/ -run MatchHabitatTypes -v`
Expected: FAIL — `svc.MatchHabitatTypes undefined`

- [ ] **Step 3: Write minimal implementation**

In `internal/ports/input/services.go` die Typen und die Port-Methode:

```go
// MatchRequest ist die Anfrage an POST /v1/habitat-types/match.
type MatchRequest struct {
	ConceptIDs []string
	Typology   string
	Level      int
	Area       string
	Limit      int
}

// MatchInput spiegelt eine Eingabe-ID zurueck.
type MatchInput struct {
	ConceptID string `json:"concept_id"`
	Known     bool   `json:"known"`
	Reason    string `json:"reason,omitempty"`
}

// MatchEntry ist ein Rang der Antwort. Score ist ein Log-Likelihood, keine
// Wahrscheinlichkeit: belastbar ist die Reihenfolge, nicht der Betrag.
type MatchEntry struct {
	Typology string  `json:"typology"`
	Code     string  `json:"code"`
	NameEN   string  `json:"name_en"`
	NameDE   string  `json:"name_de,omitempty"`
	Score    float64 `json:"score"`
	Matched  int     `json:"matched"`
	Of       int     `json:"of"`
}

// MatchResult ist die Antwort. Beide Listen sind immer da, auch leer.
type MatchResult struct {
	Input   []MatchInput `json:"input"`
	Matches []MatchEntry `json:"matches"`
}
```

und zur `QueryService`-Schnittstelle:

```go
	// MatchHabitatTypes ranks habitat types by how well they explain a set of
	// observed concept ids. The score is a log-likelihood, never a probability.
	MatchHabitatTypes(ctx context.Context, req MatchRequest) (MatchResult, error)
```

`internal/application/matching.go`:

```go
package application

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// MatchHabitatTypes ordnet Habitattypen danach, wie gut sie eine beobachtete
// Artenliste erklaeren.
//
// Kandidat ist nur, wer mindestens eine Eingabe-Art fuehrt: jeder andere Typ
// traegt den identischen Score k*log(MISS) und ist damit rangneutral. Das
// erspart den vollen Durchlauf ueber alle Typen der Ebene.
func (q *QueryService) MatchHabitatTypes(ctx context.Context, req input.MatchRequest) (input.MatchResult, error) {
	res := input.MatchResult{Input: make([]input.MatchInput, 0, len(req.ConceptIDs)), Matches: []input.MatchEntry{}}

	hits := map[domain.HabitatTypeKey][]domain.MatchHit{}
	bekannt := 0
	for _, id := range req.ConceptIDs {
		entry := input.MatchInput{ConceptID: id}
		if !strings.HasPrefix(id, indexBackbone+":") { // dieselbe Pruefung wie der Batch, query.go:329
			entry.Reason = input.ReasonUnknownBackbone
			res.Input = append(res.Input, entry)
			continue
		}
		rollen, err := q.repo.SpeciesRolesByConcept(ctx, id)
		if err != nil {
			return input.MatchResult{}, fmt.Errorf("matching %q: %w", id, err)
		}
		if len(rollen) == 0 {
			entry.Reason = input.ReasonUnknownConcept
			res.Input = append(res.Input, entry)
			continue
		}
		entry.Known = true
		bekannt++
		res.Input = append(res.Input, entry)

		for _, r := range rollen {
			if string(r.Key.Typology) != req.Typology {
				continue
			}
			hits[r.Key] = append(hits[r.Key], domain.MatchHit{
				ConceptID: id, P: rollenWahrscheinlichkeit(r), Fidelity: wert(r.Fidelity),
			})
		}
	}
	if bekannt == 0 || len(hits) == 0 {
		return res, nil
	}

	keys := make([]domain.HabitatTypeKey, 0, len(hits))
	for k := range hits {
		keys = append(keys, k)
	}
	abdeckung := map[domain.HabitatTypeKey]float64{}
	if req.Area != "" {
		var err error
		if abdeckung, err = q.repo.HabitatAreaCoverage(ctx, keys, req.Area); err != nil {
			return input.MatchResult{}, fmt.Errorf("matching area coverage: %w", err)
		}
	}

	for _, k := range keys {
		ht, err := q.repo.HabitatType(ctx, k)
		if err != nil {
			return input.MatchResult{}, fmt.Errorf("matching habitat type %s: %w", k, err)
		}
		if req.Level > 0 && ht.Level != req.Level {
			continue
		}
		cov, hat := abdeckung[k]
		c := domain.MatchCandidate{Key: k, Hits: hits[k], AreaCoverage: cov, HasArea: hat}
		gezaehlt := map[string]struct{}{}
		for _, h := range hits[k] {
			gezaehlt[h.ConceptID] = struct{}{}
		}
		res.Matches = append(res.Matches, input.MatchEntry{
			Typology: string(k.Typology), Code: k.Code, NameEN: ht.NameEN,
			Score: domain.ScoreCandidate(c, bekannt), Matched: len(gezaehlt), Of: bekannt,
		})
	}

	sort.Slice(res.Matches, func(i, j int) bool {
		if res.Matches[i].Score != res.Matches[j].Score {
			return res.Matches[i].Score > res.Matches[j].Score
		}
		return res.Matches[i].Code < res.Matches[j].Code // stabil bei Gleichstand
	})
	if req.Limit > 0 && len(res.Matches) > req.Limit {
		res.Matches = res.Matches[:req.Limit]
	}
	return res, nil
}

// rollenWahrscheinlichkeit liest constancy als P(Art | Habitat). Zeilen ohne
// Stetigkeit — die nur als diagnostic gefuehrten und alle aus Aggregaten
// abgeleiteten — bekommen den Vorgabewert.
func rollenWahrscheinlichkeit(r domain.SpeciesRole) float64 {
	if r.Constancy != nil && *r.Constancy > 0 {
		return math.Min(*r.Constancy/100, 0.99)
	}
	return domain.MatchDefaultP
}

func wert(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/application/ -run MatchHabitatTypes -v`
Expected: PASS, alle fünf

- [ ] **Step 5: Commit**

```bash
git add internal/application/matching.go internal/application/matching_test.go internal/ports/input/services.go
git commit -m "feat(application): Habitattypen nach Log-Likelihood ordnen"
```

---

### Task 4: Route, Handler, OpenAPI

**Files:**
- Create: `internal/adapters/http/matching.go`
- Modify: `internal/adapters/http/server.go` (Routeneintrag)
- Modify: `internal/adapters/http/openapi.yaml` **und** `api/openapi/openapi.yaml`
- Test: `internal/adapters/http/matching_test.go`

**Interfaces:**
- Consumes: `input.QueryService.MatchHabitatTypes`, `s.decodeConceptIDs`
- Produces: `POST /v1/habitat-types/match`

- [ ] **Step 1: Write the failing test**

```go
package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postMatch(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/habitat-types/match", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(rec, req)
	return rec
}

func TestMatchRoute_LiefertRangliste(t *testing.T) {
	rec := postMatch(t, `{"concept_ids":["wcvp:concept:1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"input"`, `"matches"`, `"score"`, `"matched"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("Antwort enthaelt %s nicht: %s", want, rec.Body.String())
		}
	}
}

// Nach nichts zu fragen ist ein Fehler des Aufrufers, keine Frage.
func TestMatchRoute_LeereListeIstInvalidQuery(t *testing.T) {
	rec := postMatch(t, `{"concept_ids":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "INVALID_QUERY") {
		t.Errorf("Fehlerhuelle fehlt: %s", rec.Body.String())
	}
}

// Ein unbekanntes Gebiet darf nicht stillschweigend ignoriert werden. Die
// Pruefung liegt im Use-Case (dort ist der Repository-Zugang), der Handler
// bildet den Fehler auf INVALID_QUERY ab — deshalb prueft dieser Test die
// Route Ende zu Ende und nicht den Handler allein.
func TestMatchRoute_UnbekanntesGebietIstInvalidQuery(t *testing.T) {
	rec := postMatch(t, `{"concept_ids":["wcvp:concept:1"],"area":"XXX"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestMatchRoute_LimitAusserhalbDerGrenzen(t *testing.T) {
	for _, body := range []string{
		`{"concept_ids":["wcvp:concept:1"],"limit":0}`,
		`{"concept_ids":["wcvp:concept:1"],"limit":51}`,
		`{"concept_ids":["wcvp:concept:1"],"limit":-1}`,
	} {
		if rec := postMatch(t, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s -> Status %d, want 400", body, rec.Code)
		}
	}
}

// Ein altes Feld darf eine klare 400 bekommen, keine leere Antwort.
func TestMatchRoute_UnbekanntesFeldIstInvalidQuery(t *testing.T) {
	if rec := postMatch(t, `{"names":["Fagus sylvatica"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400", rec.Code)
	}
}

// Nur POST. Die Vertragspruefung verlangt ausserdem, dass die Route in beiden
// OpenAPI-Kopien steht — das prueft TestRoutesMatchOpenAPISpec.
func TestMatchRoute_NurPost(t *testing.T) {
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/habitat-types/match", nil))
	if rec.Code == http.StatusOK {
		t.Error("GET auf die Match-Route antwortet 200")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/http/ -run MatchRoute -v`
Expected: FAIL — 404, die Route gibt es nicht

- [ ] **Step 3: Write minimal implementation**

`internal/adapters/http/matching.go`:

```go
package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

const (
	matchDefaultLimit = 10
	matchMaxLimit     = 50
)

type matchBody struct {
	ConceptIDs []string `json:"concept_ids"`
	Typology   string   `json:"typology"`
	Level      *int     `json:"level"`
	Area       string   `json:"area"`
	Limit      *int     `json:"limit"`
}

// handleHabitatTypeMatch ordnet Habitattypen nach einer beobachteten
// Artenliste. Der zurueckgegebene score ist ein Log-Likelihood: belastbar ist
// die Reihenfolge, nicht der Betrag.
func (s *Server) handleHabitatTypeMatch(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBatchBodyBytes))
	dec.DisallowUnknownFields()
	var body matchBody
	if err := dec.Decode(&body); err != nil {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, fmt.Sprintf("malformed body: %v", err))
		return
	}
	if len(body.ConceptIDs) == 0 {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "concept_ids must not be empty")
		return
	}

	limit := matchDefaultLimit
	if body.Limit != nil {
		limit = *body.Limit
		if limit < 1 || limit > matchMaxLimit {
			s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
				fmt.Sprintf("limit must be between 1 and %d", matchMaxLimit))
			return
		}
	}
	req := input.MatchRequest{
		ConceptIDs: body.ConceptIDs,
		Typology:   typologyOrDefault(body.Typology),
		Level:      3,
		Area:       body.Area,
		Limit:      limit,
	}
	if body.Level != nil {
		req.Level = *body.Level
	}

	res, err := s.deps.Query.MatchHabitatTypes(r.Context(), req)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}
```

Zwei Hilfen, die es so noch nicht gibt und die in derselben Datei entstehen:

```go
// typologyOrDefault spiegelt habitatTypeKey aus habitat.go: eine leere Angabe
// faellt auf eunis@2021 zurueck.
func typologyOrDefault(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return string(domain.DefaultTypologyID)
	}
	return strings.TrimSpace(raw)
}
```

Die Gebietsprüfung hat **kein** `domain.IsKnownAreaCode` — es gibt sie nicht.
Der Dienst prüft Gebietscodes über das Repository
(`q.repo.KnownAreaCodes(ctx, domain.SchemeWGSRPDL3)`, siehe
`internal/application/area.go:39`). Die Prüfung gehört deshalb **in den
Use-Case**, nicht in den Handler: dort liegt der Repository-Zugang, und es ist
derselbe Mechanismus wie beim vorhandenen `?area=`. Der Handler reicht `Area`
durch, der Use-Case gibt bei unbekanntem Code einen Fehler zurück, den
`writeQueryError` als `INVALID_QUERY` abbildet — prüfe in
`internal/application/area.go`, welchen Fehlertyp der bestehende Pfad dafür
verwendet, und nimm denselben.

In `internal/adapters/http/server.go`, bei den `/v1`-Routen:

```go
	r.HandleFunc("/v1/habitat-types/match", s.handleHabitatTypeMatch).Methods(http.MethodPost)
```

In **beiden** OpenAPI-Kopien, unter `paths:`:

```yaml
  /v1/habitat-types/match:
    post:
      summary: Habitattypen nach einer beobachteten Artenliste ordnen
      description: >-
        Nimmt die im Gelände notierten Konzept-IDs und ordnet die Habitattypen
        danach, wie gut sie diese Liste erklären. score ist ein
        Log-Likelihood, ausdrücklich keine Wahrscheinlichkeit: die Parameter
        sind gesetzt und nicht kalibriert, und die Unabhängigkeitsannahme
        trifft bei gemeinsam auftretenden Arten nicht zu. Belastbar ist die
        Reihenfolge, nicht der Betrag — matched/of macht nachvollziehbar,
        worauf sie beruht. Ein Typ, der eine genannte Art nicht führt, wird
        abgewertet, aber nicht ausgeschlossen. area ist eine
        Plausibilitätsprüfung, kein Filter. Fels und Geröll (Formation U)
        sowie der Großteil der Ruderalvegetation führen keine Kennarten und
        können deshalb nie erscheinen.
      operationId: matchHabitatTypes
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [concept_ids]
              properties:
                concept_ids:
                  type: array
                  items: {type: string}
                  minItems: 1
                typology: {type: string, default: "eunis@2021"}
                level: {type: integer, default: 3}
                area: {type: string}
                limit: {type: integer, minimum: 1, maximum: 50, default: 10}
      responses:
        "200":
          description: Rangliste und die zurückgespiegelten Eingaben.
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/MatchResult"
        "400":
          $ref: "#/components/responses/InvalidQuery"
```

und unter `components.schemas`:

```yaml
    MatchResult:
      type: object
      required: [input, matches]
      properties:
        input:
          type: array
          items:
            type: object
            required: [concept_id, known]
            properties:
              concept_id: {type: string}
              known: {type: boolean}
              reason: {type: string, enum: [unknown_backbone, unknown_concept]}
        matches:
          type: array
          items:
            type: object
            required: [typology, code, name_en, score, matched, of]
            properties:
              typology: {type: string}
              code: {type: string}
              name_en: {type: string}
              name_de: {type: string}
              score:
                type: number
                description: >-
                  Log-Likelihood, keine Wahrscheinlichkeit. Nur die
                  Reihenfolge ist belastbar.
              matched: {type: integer}
              of: {type: integer}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/http/ -v`
Expected: PASS, inklusive `TestRoutesMatchOpenAPISpec` und
`TestOpenAPICopiesAreIdentical`. Schlägt die Kopienprüfung fehl:
`cp internal/adapters/http/openapi.yaml api/openapi/openapi.yaml`

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/matching.go internal/adapters/http/matching_test.go internal/adapters/http/server.go internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
git commit -m "feat(http): POST /v1/habitat-types/match"
```

---

### Task 5: Die Umkehr in der Dokumentation

**Files:**
- Modify: `CLAUDE.md` (Abschnitt „Known ceiling")
- Modify: `docs/reference/http-api.md` (Ko-Kennarten-Absatz + neuer Routenabschnitt + Routentabelle)
- Modify: `docs/reference/measured-index.md` (die Kennzahlen der Spec)

Diese Task hat keinen eigenen Testzyklus — sie wird durch `make docs` und das
Drift-Gate geprüft. Sie ist trotzdem eine eigene Task, weil ein Reviewer sie
unabhängig annehmen oder ablehnen kann.

- [ ] **Step 1: `CLAUDE.md` — die Ausnahme herausnehmen**

Im Abschnitt „Known ceiling" den Satz

```
Also deliberately out of scope: scoring/ranking (plant set →
ranked habitats), the ESy rule engine and the EUNIS-2012 key (both need cover
and region data), and full plot classification.
```

ersetzen durch

```
Also deliberately out of scope: the ESy rule engine and the EUNIS-2012 key
(both need cover and region data), and full plot classification.

**Scoring/ranking was out of scope and no longer is.** `POST
/v1/habitat-types/match` ranks habitat types for an observed species list —
see `docs/superpowers/specs/2026-09-26-habitat-matching-design.md`, which
revises the foundation decision and carries the measurement that motivated
it. The score is a log-likelihood, never a probability: only the ordering is
sound.
```

- [ ] **Step 2: `docs/reference/http-api.md` — den Ko-Kennarten-Absatz richtigstellen**

Der Absatz beginnt mit „**Ko-Kennarten** … haben absichtlich **keinen** eigenen
Endpunkt". Er bleibt inhaltlich richtig für die Ko-Kennarten-Frage, muss aber
den neuen Endpunkt abgrenzen. Am Ende des Absatzes anfügen:

```markdown
Das galt und gilt für die Ko-Kennarten. Für die **umgekehrte** Frage — welche
Habitattypen erklären eine beobachtete Artenliste? — gibt es seit
`POST /v1/habitat-types/match` sehr wohl einen zusammenfassenden Endpunkt. Die
Entscheidung von 2026-08-18 ist dort bewusst revidiert worden, weil die
Trennschärfe messbar in den Daten liegt; siehe den Abschnitt unten.
```

- [ ] **Step 3: `docs/reference/http-api.md` — Routentabelle und neuer Abschnitt**

In die Routentabelle am Dateianfang:

```markdown
| `POST /v1/habitat-types/match` | Artenliste → Rangliste der Habitattypen (Log-Likelihood) |
```

Und ein eigener Abschnitt vor „## Die zwei Arten-Pfade", der trägt: den
Algorithmus in zwei Sätzen, die vier Parameter mit ihren Werten, die Aussage
„`score` ist ein Log-Likelihood, keine Wahrscheinlichkeit" samt Begründung, die
Feststellung, dass `area` Plausibilitätsprüfung und nicht Filter ist, und die
Grenze: Formation U führt keine einzige Kennart (alle 36 Level-3-Typen), V nur
12 von 31.

- [ ] **Step 4: `docs/reference/measured-index.md` — die Kennzahlen**

Einen Abschnitt „Trennschärfe der Level-3-Habitattypen" mit den gemessenen
Werten und ihren Abfragen: 198 Typen mit Artenliste von 270, 3561 Arten, 62 %
in genau einem Typ, 53 % der Habitatpaare ohne gemeinsame Art, und die
Abnahmetabelle aus der Spec (82 % Rang 1 bei drei Arten, 90 % bei fünf).

- [ ] **Step 5: Prüfen und committen**

```bash
make docs
bash .claude/skills/doc-drift-check/scripts/check-doc-drift.sh
git add CLAUDE.md docs/reference/http-api.md docs/reference/measured-index.md
git commit -m "docs: die kein-Scoring-Entscheidung umkehren und begruenden"
```

---

### Task 6: Das Explorer-Panel

**Files:**
- Modify: `internal/adapters/http/explorer.html`
- Test: `internal/adapters/http/explorer_match_test.go`

Die Referenz sagt zu, dass der Explorer **jeden** Lese-Endpunkt ausprobierbar
macht. Ohne Panel wäre diese Zusage gebrochen.

- [ ] **Step 1: Write the failing test**

```go
package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExplorerPage_HatDasMatchPanel(t *testing.T) {
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`id="match"`,
		`"/v1/habitat-types/match"`,
		`id="match-ids"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Seite enthaelt %q nicht; das Match-Panel fehlt", want)
		}
	}
}

// Die Seite sagt im Untertitel, sie rechne nichts selbst aus. Das bleibt wahr:
// die Rangliste kommt vom Dienst. Der Untertitel muss das trotzdem einordnen,
// sonst widerspricht er dem neuen Panel.
func TestExplorerPage_UntertitelOrdnetDieRanglisteEin(t *testing.T) {
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rec.Body.String()
	if strings.Contains(body, "Es wird nichts berechnet oder zusammengefasst, was der Dienst") &&
		!strings.Contains(body, "Rangliste") {
		t.Error("der Untertitel schliesst jede Berechnung aus, das Match-Panel zeigt aber eine Rangliste des Dienstes")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/http/ -run 'ExplorerPage_HatDasMatch|UntertitelOrdnet' -v`
Expected: FAIL — `id="match"` fehlt

- [ ] **Step 3: Write minimal implementation**

Ein Panel nach dem Muster der bestehenden (`<section class="panel" id="match">`):
ein `<textarea id="match-ids">` für die Konzept-IDs (eine je Zeile), die
globalen `area`- und `lang`-Felder werden mitgenutzt, ein „Abfragen"-Knopf, der
`POST /v1/habitat-types/match` absetzt und die Antwort in das vorhandene
Roh-JSON-`<pre>` schreibt — plus eine kleine Tabelle mit `code`, `name_en`,
`score`, `matched`/`of`.

Der Untertitel wird ergänzt um den Halbsatz, dass die Rangliste des
Match-Panels vom Dienst berechnet wird und die Seite sie nur anzeigt.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/http/ -v`
Expected: PASS, inklusive `TestExplorerPage_LoadsNoRemoteAssets` (kein externes
`href`!) und `TestExplorerPage_FormularelementeSprengenDenViewportNicht`

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/explorer.html internal/adapters/http/explorer_match_test.go
git commit -m "feat(explorer): Panel fuer die Habitattyp-Rangliste"
```

---

### Task 7: Abnahme gegen den echten Index

**Files:**
- Create: `internal/application/matching_integrity_test.go`

Nach dem Muster von `internal/application/distribution_integrity_test.go`: ein
Test gegen einen Fixture-Index, der die Zusagen der Spec festhält, die eine
Unit nicht abdeckt.

- [ ] **Step 1: Write the failing test**

```go
package application_test

import (
	"context"
	"testing"

	"github.com/jobrunner/situs/internal/ports/input"
)

// Die namentliche Zusage der Spec: drei Arten des Waldmeister-Buchenwalds
// muessen T17 auf Rang 1 bringen, mit Abstand zum zweiten.
func TestMatching_FagusAnemoneGaliumErgibtT17(t *testing.T) {
	svc := newMatchServiceFromFixture(t) // Fixture mit T17, T1F, T1E, T18, T32

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{conceptFagus, conceptAnemone, conceptGalium},
		Typology:   "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) == 0 || got.Matches[0].Code != "T17" {
		t.Fatalf("Rang 1 = %v, erwartet T17", got.Matches)
	}
	if len(got.Matches) > 1 && got.Matches[0].Score-got.Matches[1].Score < 0.5 {
		t.Errorf("Abstand zum zweiten nur %.3f; die Spec verlangt einen deutlichen",
			got.Matches[0].Score-got.Matches[1].Score)
	}
}

// Eine Stoerart darf das richtige Habitat nicht aus der Liste werfen — genau
// das leistet der harte Filter nicht, und genau deshalb gibt es MISS.
func TestMatching_StoerartWirftDasRichtigeHabitatNichtHeraus(t *testing.T) {
	svc := newMatchServiceFromFixture(t)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{conceptFagus, conceptAnemone, conceptGalium, conceptStoerart},
		Typology:   "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) == 0 || got.Matches[0].Code != "T17" {
		t.Fatalf("mit einer Stoerart ist Rang 1 = %v, erwartet weiterhin T17", got.Matches)
	}
	if got.Matches[0].Matched != 3 || got.Matches[0].Of != 4 {
		t.Errorf("matched/of = %d/%d, erwartet 3/4", got.Matches[0].Matched, got.Matches[0].Of)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/ -run 'TestMatching_' -v`
Expected: FAIL — `newMatchServiceFromFixture` fehlt

- [ ] **Step 3: Write minimal implementation**

Den Fixture-Helfer bauen: fünf Habitattypen mit realistischen Stetigkeiten
(T17 führt Fagus 99 / Anemone 60 / Galium 55; T1F führt Fagus 40 / Galium 30;
T1E führt Anemone 50; T18 führt Fagus 80; T32 führt Fagus 30) und eine
Störart, die keiner davon führt.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/application/ -v`
Expected: PASS

- [ ] **Step 5: Volle Prüfkette und Commit**

```bash
make verify > /tmp/verify.log 2>&1; echo "EXIT=$?"   # nie per Pipe pruefen
bash .claude/skills/doc-drift-check/scripts/check-doc-drift.sh
git add internal/application/matching_integrity_test.go
git commit -m "test(matching): die namentlichen Zusagen der Spec festhalten"
```

---

## Reihenfolge und Abhängigkeiten

Task 1 → 2 → 3 → 4 sind eine Kette; jede baut auf den Signaturen der
vorigen auf. Task 5 (Doku) und Task 6 (Explorer) hängen nur an Task 4. Task 7
hängt an Task 3.
