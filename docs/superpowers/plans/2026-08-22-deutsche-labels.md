# Deutsche Labels Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** situs liefert deutsche Habitattyp-Namen — amtlich aus EUR-Lex für
Annex I, abgeleitet für 29 EUNIS-Typen, von situs verfasst für 241 — und ein
Client kann die drei Fälle nie verwechseln.

**Architecture:** Der bestehende Overlay bleibt additiv. Neu sind: ein vierter
Provenienz-Wert `situs`, ein `vernacular`-Feld, `name_de` als Objekt auf der API,
und eine Pipeline `pipelines/eurlex/`, die den Rechtstext zu CSV normalisiert.
Der Go-Ingest liest weiterhin nur CSV.

**Tech Stack:** Go 1.26 (stdlib + gorilla/mux + modernc.org/sqlite),
`python3` stdlib-only für die Pipeline.

**Spec:** `docs/superpowers/specs/2026-08-22-deutsche-labels-design.md`

## Global Constraints

- Erlaubte Bibliotheken unverändert. **Keine** neue Go-Abhängigkeit; die
  Extraktion des Rechtstexts bleibt in `pipelines/eurlex/` (bash + `python3`,
  stdlib only).
- Zero `//nolint` / `#nosec`, zero `TODO`/`FIXME`/`HACK`/`XXX` in Go-Dateien.
  `.debt-budget` ist 0.
- `.coverage-floors`, `.mutation-thresholds` und `.codecharta-ratchet.json` sind
  raise-only Ratchets. Nach jedem Task: `make verify` grün.
- SQL sind statische Strings mit `?`-Platzhaltern, niemals konkateniert.
- Provenienz-Vokabular: `official` | `curated` | `derived` | `situs`.
  Auflösung `official > curated > derived > situs`.
- **`situs` ist niemals Ableitungssaat.**
- `name_en` bleibt die Identität. Der Overlay ersetzt nie, er ergänzt.
- Shell-Skripte müssen auf bash 3.2 laufen (kein `mapfile`).
- Kommentare englisch und sparsam, nur wo sie ein *warum* erklären.

---

### Task 1: Vierter Provenienz-Wert `situs`

**Files:**
- Modify: `internal/application/localize.go` (Konstanten, Ingest-Validierung)
- Modify: `internal/application/query.go:271-288` (`preferredLabel`)
- Test: `internal/application/localize_test.go`, `internal/application/query_test.go`

**Interfaces:**
- Produces: Konstante `provenanceSitus = "situs"`; `preferredLabel` respektiert
  die vierstufige Reihenfolge.

- [ ] **Step 1: Failing test für die Reihenfolge**

In `internal/application/query_test.go`:

```go
// situs ist der schwächste Anspruch: eine selbst verfasste Übersetzung verliert
// gegen jede Quelle, die von außen belegt ist.
func TestPreferredLabelRanksSitusLast(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rows  []domain.Localization
		want  string
	}{
		{"situs allein", []domain.Localization{
			{Provenance: "situs", Value: "situs label"}}, "situs"},
		{"derived schlaegt situs", []domain.Localization{
			{Provenance: "situs", Value: "situs label"},
			{Provenance: "derived", Value: "derived label"}}, "derived"},
		{"curated schlaegt derived", []domain.Localization{
			{Provenance: "situs", Value: "s"},
			{Provenance: "derived", Value: "d"},
			{Provenance: "curated", Value: "c"}}, "curated"},
		{"official schlaegt alles", []domain.Localization{
			{Provenance: "situs", Value: "s"},
			{Provenance: "curated", Value: "c"},
			{Provenance: "official", Value: "o"}}, "official"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, got := preferredLabel(tc.rows)
			if got != tc.want {
				t.Errorf("provenance = %q, want %q", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Test laufen lassen, Fehlschlag bestätigen**

`go test ./internal/application/ -run TestPreferredLabelRanksSitusLast -v`
Erwartet: FAIL — `situs` wird noch nicht aufgelöst, `preferredLabel` gibt `""`.

- [ ] **Step 3: Konstante und Reihenfolge ergänzen**

In `localize.go` bei den Provenienz-Konstanten:

```go
	// situs is the weakest claim: situs itself translated this, and no external
	// source stands behind it. It must be distinguishable from every other value
	// on the wire, and it may never seed a derivation (see DeriveGermanLabels).
	provenanceSitus = "situs"
```

In `query.go`, `preferredLabel`:

```go
	for _, provenance := range []string{
		provenanceOfficial, provenanceCurated, provenanceDerived, provenanceSitus,
	} {
```

Den Doc-Kommentar über `preferredLabel` auf `official > curated > derived > situs`
korrigieren.

- [ ] **Step 4: Test grün**

`go test ./internal/application/ -run TestPreferredLabelRanksSitusLast`

- [ ] **Step 5: Failing test für die Ingest-Validierung**

In `internal/application/localize_test.go`:

```go
// Eine situs-Zeile muss den Ingest passieren; eine erfundene Provenienz nicht.
func TestIngestLocalizations_AcceptsSitusAndRejectsUnknownProvenance(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "localizations.csv",
		"entity_type,entity_key,lang,field,value,source,provenance\n"+
			"habitat_type,eunis@2021:R22,de,name,Maehwiese,situs@test,situs\n"+
			"habitat_type,eunis@2021:R23,de,name,Irgendwas,x,erfunden\n")

	repo := newFakeRepo()
	n, err := IngestLocalizations(context.Background(), repo, filepath.Join(dir, "localizations.csv"))
	if err != nil {
		t.Fatalf("IngestLocalizations: %v", err)
	}
	if n != 1 {
		t.Errorf("count = %d, want 1 (die situs-Zeile, nicht die erfundene)", n)
	}
	if len(repo.localizations) != 1 || repo.localizations[0].Provenance != "situs" {
		t.Errorf("localizations = %+v, want exactly one situs row", repo.localizations)
	}
}
```

Falls `fakeRepo` die Localizations nicht sammelt, ein Feld `localizations
[]domain.Localization` ergänzen und in `UpsertLocalization` anhängen.

- [ ] **Step 6: Test laufen lassen, Fehlschlag bestätigen**

Erwartet: FAIL — `situs` wird von der Validierung verworfen, count = 0.

- [ ] **Step 7: Validierung erweitern**

In `IngestLocalizations`:

```go
			switch provenance {
			case provenanceOfficial, provenanceCurated, provenanceDerived, provenanceSitus:
			default:
				skip(line, fmt.Errorf("provenance %q is none of official/curated/derived/situs", provenance))
				return nil
			}
```

- [ ] **Step 8: Tests grün, verify**

`go test ./internal/application/ && make verify`

- [ ] **Step 9: Commit**

```bash
git add internal/application/
git commit -m "feat(localization): add the situs provenance value

situs is the weakest claim in the vocabulary: situs itself translated the label
and no external source stands behind it. Resolution becomes
official > curated > derived > situs, and the ingest accepts it as a fourth
value instead of skipping the row."
```

---

### Task 2: `situs` darf niemals Ableitungssaat sein

**Files:**
- Modify: `internal/application/localize.go` (`officialOrCuratedName` — nur Doku/Test, kein Verhaltenswechsel)
- Test: `internal/application/localize_test.go`

**Interfaces:**
- Consumes: `provenanceSitus` aus Task 1.

Der Code ist heute schon korrekt — `officialOrCuratedName` lässt nur `official`
und `curated` zu. Der Punkt dieses Tasks ist, dass die Zusage **getestet** ist,
statt in einem Kommentar zu stehen: sie ist die tragende Invariante der Spec.

- [ ] **Step 1: Failing test schreiben**

```go
// Provenienz-Wäsche: würde eine von situs erfundene Bezeichnung eine Ableitung
// speisen, käme sie als "derived" wieder heraus und sähe nachvollziehbar aus,
// obwohl am Anfang der Kette eine Erfindung steht.
func TestDeriveGermanLabels_NeverSeedsFromSitus(t *testing.T) {
	repo := newFakeRepo()
	// Ein '='-Crosswalk eunis@2021:R22 -> annex1:6510 …
	repo.crosswalks = []domain.Crosswalk{{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}}
	// … und der einzige deutsche Name des Annex-I-Typs ist von situs verfasst.
	repo.localizationsByKey = map[string][]domain.Localization{
		"annex1:6510": {{
			EntityType: "habitat_type", EntityKey: "annex1:6510",
			Lang: "de", Field: "name", Value: "Von situs erfunden",
			Source: "situs@test", Provenance: "situs",
		}},
	}

	n, err := DeriveGermanLabels(context.Background(), repo)
	if err != nil {
		t.Fatalf("DeriveGermanLabels: %v", err)
	}
	if n != 0 {
		t.Errorf("derived = %d, want 0 — a situs label must never seed a derivation", n)
	}
	for _, l := range repo.localizations {
		if l.Provenance == "derived" {
			t.Errorf("wrote a derived label %+v seeded from a situs value — provenance laundering", l)
		}
	}
}
```

`fakeRepo.Localization` muss dafür `localizationsByKey` bedienen; falls es das
schon über ein anderes Feld tut, dieses verwenden statt ein neues anzulegen.

- [ ] **Step 2: Test laufen lassen**

Erwartet: **PASS** — der Code ist bereits korrekt. Das ist der seltene Fall, in
dem der Test eine bestehende Zusage festnagelt statt neues Verhalten zu treiben.

**Wenn er fehlschlägt**, ist das ein echter Fund: dann `officialOrCuratedName`
so korrigieren, dass es `provenanceSitus` ausschließt, und den Fehlschlag im
Commit benennen.

- [ ] **Step 3: Beweisen, dass der Test beißt**

`officialOrCuratedName` testweise um `provenanceSitus` erweitern, Test laufen
lassen (muss FAIL zeigen), Änderung zurücknehmen. Ein Test, der eine Invariante
hält, ist wertlos, wenn er nicht fehlschlägt, sobald sie bricht.

- [ ] **Step 4: Doc-Kommentar schärfen**

Über `officialOrCuratedName` ergänzen:

```go
// situs is deliberately absent from the accepted set: a situs value is an
// invention, and letting it seed a derivation would return it marked "derived",
// which reads as traceable. TestDeriveGermanLabels_NeverSeedsFromSitus holds it.
```

- [ ] **Step 5: Commit**

```bash
git add internal/application/
git commit -m "test(localization): hold that situs never seeds a derivation

The code was already correct; the invariant was only a comment. Verified the
test bites by adding provenanceSitus to the accepted set and watching it fail."
```

---

### Task 3: `Localization` verliert den `field`-Parameter

**Files:**
- Modify: `internal/ports/output/repository.go:57`
- Modify: `internal/adapters/sqlite/read.go` (Implementierung)
- Modify: `internal/application/query.go:203`, `internal/application/localize.go` (Aufrufer)
- Test: `internal/adapters/sqlite/read_test.go`, die Fakes in `internal/application/ingest_test.go`

**Interfaces:**
- Produces: `Localization(ctx context.Context, entityType, entityKey, lang string) ([]domain.Localization, error)` — liefert **alle** Felder für (entity, lang).

**Warum:** `value` und `vernacular` gehören zusammen abgefragt. Mit
`field`-Filter bräuchte jeder Habitattyp zwei Queries; auf einer Liste mit 60
Typen sind das 60 unnötige. Das Filtern nach Feld ist Policy und gehört in die
Anwendung, nicht in den Port.

- [ ] **Step 1: Failing test in sqlite**

```go
// Ein Aufruf liefert alle Felder der Entität, damit name und vernacular nicht
// zwei Queries kosten.
func TestLocalizationReturnsEveryFieldOfTheEntity(t *testing.T) {
	db := newTestDB(t)
	tx := beginTx(t, db)
	for _, l := range []domain.Localization{
		{EntityType: "habitat_type", EntityKey: "eunis@2021:R22", Lang: "de",
			Field: "name", Value: "Maehwiese", Source: "situs@test", Provenance: "situs"},
		{EntityType: "habitat_type", EntityKey: "eunis@2021:R22", Lang: "de",
			Field: "vernacular", Value: "Glatthaferwiese", Source: "situs@test", Provenance: "situs"},
	} {
		if err := tx.UpsertLocalization(l); err != nil {
			t.Fatalf("UpsertLocalization: %v", err)
		}
	}
	commitTx(t, tx)

	got, err := db.Localization(context.Background(), "habitat_type", "eunis@2021:R22", "de")
	if err != nil {
		t.Fatalf("Localization: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("rows = %d, want 2 (name + vernacular in one call)", len(got))
	}
}
```

Die Helfer (`newTestDB`, `beginTx`, `commitTx`) heißen im Repo möglicherweise
anders — die vorhandenen aus `read_test.go` verwenden, keine neuen anlegen.

- [ ] **Step 2: Test laufen lassen**

Erwartet: Compile-Fehler — `Localization` erwartet noch fünf Argumente.

- [ ] **Step 3: Port und Implementierung ändern**

`internal/ports/output/repository.go`:

```go
	// Localization returns every localized field of one entity in one language,
	// ordered so a caller sees official rows first. Filtering by field is the
	// caller's policy: name and vernacular belong to the same answer.
	Localization(ctx context.Context, entityType, entityKey, lang string) ([]domain.Localization, error)
```

In `internal/adapters/sqlite/read.go` das `AND field = ?` aus dem SQL und das
Argument entfernen; die `ORDER BY`-Klausel unverändert lassen.

- [ ] **Step 4: Aufrufer anpassen**

`query.go:203` — der `nameField`-Parameter fällt weg:

```go
		labels, err := q.repo.Localization(ctx, "habitat_type", key.String(), deLang)
```

In `localize.go` dort, wo die Ableitungssaat geholt wird, ebenfalls; das Filtern
auf `field == nameField` wandert in den Aufrufer.

- [ ] **Step 5: Fakes anpassen**

Alle `fakeRepo.Localization`-Implementierungen auf die neue Signatur ziehen. Der
Compiler nennt jede Stelle: `go build ./... && go vet ./...`.

- [ ] **Step 6: Tests grün, verify**

`make verify`

- [ ] **Step 7: Commit**

```bash
git add internal/
git commit -m "refactor(ports): Localization returns every field of an entity

name and vernacular belong to one answer, so filtering by field in the port cost
a second query per habitat type — 60 extra queries on a 60-type list. Field
selection is caller policy and moves into the application."
```

---

### Task 4: `vernacular` in der Anwendung

**Files:**
- Modify: `internal/application/localize.go` (Feldkonstante)
- Modify: `internal/application/query.go` (`preferredLabel` → Label-Objekt)
- Test: `internal/application/query_test.go`

**Interfaces:**
- Produces: `input.GermanLabel{Value, Vernacular, Provenance, Source}` (in Task 5
  definiert; hier zunächst als applikationsinterner Rückgabetyp von
  `preferredLabel`, damit Task 5 nur die Wire-Form anfasst).

- [ ] **Step 1: Failing test**

```go
// vernacular ist optional: es fehlt, wo kein etablierter deutscher Begriff
// denselben Umfang trägt. Ein leerer String wäre eine Behauptung, kein Fehlen.
func TestPreferredLabelCarriesVernacularWhenPresent(t *testing.T) {
	rows := []domain.Localization{
		{Field: "name", Value: "Tief- und mittelmontane Mähwiese",
			Provenance: "situs", Source: "situs@test"},
		{Field: "vernacular", Value: "Glatthaferwiese",
			Provenance: "situs", Source: "situs@test"},
	}
	got := preferredLabel(rows)
	if got.Value != "Tief- und mittelmontane Mähwiese" {
		t.Errorf("Value = %q", got.Value)
	}
	if got.Vernacular != "Glatthaferwiese" {
		t.Errorf("Vernacular = %q, want the established term", got.Vernacular)
	}
	if got.Provenance != "situs" || got.Source != "situs@test" {
		t.Errorf("Provenance/Source = %q/%q", got.Provenance, got.Source)
	}
}

// Ohne vernacular-Zeile bleibt das Feld leer — und die name-Zeile trägt trotzdem.
func TestPreferredLabelWithoutVernacular(t *testing.T) {
	got := preferredLabel([]domain.Localization{
		{Field: "name", Value: "Kryptogamen- und annuellenreiche Vegetation auf Silikatfels",
			Provenance: "situs", Source: "situs@test"},
	})
	if got.Value == "" {
		t.Fatal("Value is empty")
	}
	if got.Vernacular != "" {
		t.Errorf("Vernacular = %q, want empty — no established term fits this type", got.Vernacular)
	}
}
```

- [ ] **Step 2: Test laufen lassen** — Compile-Fehler, `preferredLabel` gibt zwei Strings zurück.

- [ ] **Step 3: Implementieren**

Feldkonstante ergänzen:

```go
	vernacularField = "vernacular"
```

`preferredLabel` umbauen: erst die gewinnende Provenienz über die
`name`-Zeilen bestimmen, dann die `vernacular`-Zeile **derselben Provenienz**
dazunehmen.

```go
// preferredLabel picks the German label to serve, ordered
// official > curated > derived > situs, and attaches the vernacular term of the
// SAME provenance — mixing an official name with a situs vernacular would make
// the provenance field a lie.
func preferredLabel(labels []domain.Localization) input.GermanLabel {
	names := map[string]domain.Localization{}
	vernaculars := map[string]domain.Localization{}
	for _, l := range labels {
		switch l.Field {
		case nameField:
			if _, seen := names[l.Provenance]; !seen {
				names[l.Provenance] = l
			}
		case vernacularField:
			if _, seen := vernaculars[l.Provenance]; !seen {
				vernaculars[l.Provenance] = l
			}
		}
	}
	for _, p := range []string{provenanceOfficial, provenanceCurated, provenanceDerived, provenanceSitus} {
		n, ok := names[p]
		if !ok {
			continue
		}
		return input.GermanLabel{
			Value:      n.Value,
			Vernacular: vernaculars[p].Value,
			Provenance: n.Provenance,
			Source:     n.Source,
		}
	}
	return input.GermanLabel{}
}
```

Der Kommentar nennt den Grund für „dieselbe Provenienz": ein amtlicher Name mit
einem situs-vernacular würde das Provenienz-Feld zur Lüge machen.

- [ ] **Step 4: Aufrufer in `summaryOf` anpassen** — `s.NameDE = preferredLabel(labels)`.

- [ ] **Step 5: Tests grün, verify**

- [ ] **Step 6: Commit**

```bash
git add internal/
git commit -m "feat(localization): carry the vernacular term alongside the name

The vernacular is attached only from the same provenance as the winning name:
an official name with a situs vernacular would make the provenance field a lie.
Absent when no established German term carries the same extent."
```

---

### Task 5: `name_de` wird ein Objekt auf der API

**Files:**
- Modify: `internal/ports/input/services.go:61-72`
- Modify: `internal/adapters/http/openapi.yaml` **und** `api/openapi/openapi.yaml` (byte-identisch halten)
- Test: `internal/adapters/http/handlers_test.go`, `internal/adapters/http/contract_test.go`

**Interfaces:**
- Produces: `input.GermanLabel` mit JSON-Tags; `HabitatTypeSummary.NameDE
  *GermanLabel` mit `omitempty`.

**Breaking Change.** Vertretbar, weil `name_de` und `name_de_provenance` beide
`omitempty` sind und bei 0 Localizations heute *immer* fehlen — es gibt keinen
Konsumenten. In der Spec begründet.

- [ ] **Step 1: Failing test**

```go
// name_de ist ein Objekt, damit ein Client value nicht ohne provenance lesen
// kann. Fehlt das Label, fehlt das Objekt — kein leeres Objekt auf der Leitung.
func TestHabitatTypeCarriesTheGermanLabelAsAnObject(t *testing.T) {
	// … Server mit einem Stub, der ein situs-Label liefert …
	var got struct {
		NameEN string `json:"name_en"`
		NameDE *struct {
			Value      string `json:"value"`
			Vernacular string `json:"vernacular"`
			Provenance string `json:"provenance"`
			Source     string `json:"source"`
		} `json:"name_de"`
	}
	// … Request, Decode …
	if got.NameDE == nil {
		t.Fatal("name_de missing, want the German overlay object")
	}
	if got.NameDE.Provenance != "situs" || got.NameDE.Source == "" {
		t.Errorf("provenance/source = %q/%q, want situs with a source",
			got.NameDE.Provenance, got.NameDE.Source)
	}
	if got.NameEN == "" {
		t.Error("name_en is empty — the identity must survive the overlay")
	}
}
```

Ein zweiter Test für den Fall ohne Label: `name_de` darf **nicht** im JSON
auftauchen (`!bytes.Contains(rec.Body.Bytes(), []byte(`"name_de"`))`) — dieses
Muster gibt es schon in `handlers_test.go:353`, es wird wiederverwendet.

- [ ] **Step 2: Test laufen lassen** — FAIL, `name_de` ist noch ein String.

- [ ] **Step 3: Port-Typ ändern**

```go
// GermanLabel is the additive German overlay of one entity. It is a struct, not
// a flat pair of fields, so a client cannot read the value without seeing the
// provenance that qualifies it.
type GermanLabel struct {
	Value string `json:"value"`
	// Vernacular is the established German term, present only where one exists
	// AND carries the same extent as the type. Absent is information.
	Vernacular string `json:"vernacular,omitempty"`
	// Provenance is official | curated | derived | situs.
	Provenance string `json:"provenance"`
	Source     string `json:"source"`
}
```

In `HabitatTypeSummary` die beiden Felder ersetzen:

```go
	NameDE *GermanLabel `json:"name_de,omitempty"`
```

- [ ] **Step 4: Alle Aufrufer anpassen** — `go build ./...` nennt sie. `preferredLabel` gibt einen Wert zurück; `summaryOf` setzt `s.NameDE = &l` nur, wenn `l.Value != ""`.

- [ ] **Step 5: Beide OpenAPI-Kopien**

```yaml
        name_de:
          $ref: '#/components/schemas/GermanLabel'
```

und ein neues Schema:

```yaml
    # name_de ist ein Objekt, nicht ein String mit Geschwisterfeld: ein Client
    # soll value nicht lesen können, ohne provenance daneben zu sehen.
    GermanLabel:
      type: object
      required: [value, provenance, source]
      properties:
        value:
          type: string
        vernacular:
          type: string
          description: >-
            Etablierter deutscher Begriff. Fehlt, wo keiner denselben Umfang
            trägt wie der Typ — ein leerer Wert wäre eine Behauptung.
        provenance:
          type: string
          enum: [official, curated, derived, situs]
        source:
          type: string
```

Danach die Byte-Gleichheit herstellen: `cp internal/adapters/http/openapi.yaml
api/openapi/openapi.yaml`.

- [ ] **Step 6: Tests grün, verify**

`make verify` — der Contract-Test prüft beide Richtungen Routen↔Spec.

- [ ] **Step 7: Commit**

```bash
git add internal/ api/
git commit -m "feat(api)!: name_de becomes an object carrying its provenance

BREAKING CHANGE: name_de was a string with a sibling name_de_provenance; it is
now an object with value, vernacular, provenance and source. Structural, so a
client cannot read the value without seeing what qualifies it. No consumer
breaks: both old fields were omitempty and always absent at 0 localizations."
```

---

### Task 6: Pipeline `pipelines/eurlex/`

**Files:**
- Create: `pipelines/eurlex/fetch.sh`, `pipelines/eurlex/extract.py`, `pipelines/eurlex/README.md`
- Create: `pipelines/eurlex/test_extract.py`
- Modify: `Makefile` (`pipeline-test` muss auch dieses Verzeichnis testen)

**Interfaces:**
- Produces: `localizations.csv` mit den Spalten
  `entity_type,entity_key,lang,field,value,source,provenance` und
  `report.json`.

- [ ] **Step 1: Failing test für die Extraktion**

`pipelines/eurlex/test_extract.py`, stdlib `unittest`, mit einem **eingebetteten
HTML-Ausschnitt** als Fixture (kein Netzzugriff im Test):

```python
FIXTURE = """
<p>1150 * Lagunen des Küstenraumes (Strandseen)</p>
<p>5330 Thermo-mediterrane Gebüschformationen und Vorwüsten</p>
<p>9370 * Palmhaine von Phönix</p>
"""

class TestExtract(unittest.TestCase):
    def test_extracts_code_and_german_name(self):
        rows = extract_annex1(FIXTURE)
        self.assertEqual(rows["1150"], "Lagunen des Küstenraumes (Strandseen)")
        self.assertEqual(rows["9370"], "Palmhaine von Phönix")

    def test_strips_the_priority_asterisk_from_the_name(self):
        # Der Stern markiert prioritäre Typen und ist keine Namensbestandteil;
        # die Priorität führt der Index schon in habitat_type.priority.
        self.assertNotIn("*", rows_of(FIXTURE)["1150"])

    def test_reports_what_it_found(self):
        report = build_report(extract_annex1(FIXTURE), index_codes={"1150", "5330"})
        self.assertEqual(report["eurlex_codes"], 3)
        self.assertEqual(report["missing_in_index"], ["9370"])
        self.assertEqual(report["without_official_name"], [])
```

- [ ] **Step 2: Tests laufen lassen** — FAIL, `extract.py` existiert nicht.

- [ ] **Step 3: `extract.py` implementieren**

stdlib only (`html.parser`, `csv`, `json`, `re`, `argparse`). Kernpunkte:

- Der Stern (`*`) markiert prioritäre Typen und wird **nicht** Teil des Namens.
- CELEX als Konstante: `CELEX = "01992L0043-20130701"`, `SOURCE =
  "eur-lex:31992L0043"`.
- `entity_key` ist `annex1:<code>`, `lang` ist `de`, `field` ist `name`,
  `provenance` ist `official`.
- Ausgabe zusätzlich `report.json` mit `eurlex_codes`, `missing_in_index`,
  `without_official_name` und deren Anzahl.

- [ ] **Step 4: `fetch.sh` implementieren**

bash 3.2-tauglich, `curl` auf die CELEX-URL, Ausgabe in eine lokale HTML-Datei.
Getrennt von `extract.py`, damit der Test ohne Netz läuft.

- [ ] **Step 5: Tests grün**

`cd pipelines/eurlex && python3 -m unittest discover`

- [ ] **Step 6: `pipeline-test` erweitern**

Im Makefile so, dass beide Pipelines getestet werden — nicht nur `pipelines/eunis`.

- [ ] **Step 7: Commit**

```bash
git add pipelines/eurlex/ Makefile
git commit -m "feat(pipeline): extract the official German Annex I names from EUR-Lex

python3 stdlib only, mirroring pipelines/eunis/: the XLSX/HTML parsing stays out
of the Go binary. Pinned to CELEX 01992L0043-20130701. The priority asterisk is
not part of the name — habitat_type.priority already carries it. Emits
report.json so the 205-vs-231 gap is a measured number, not an assumption."
```

---

### Task 7: Gegen den echten Index messen

**Files:**
- Modify: `docs/reference/measured-index.md`
- Modify: `docs/how-to/ingest.md` (der neue Pipeline-Schritt)

Kein Produktionscode. Dieser Task ist die Messung, die die Spec verlangt.

- [ ] **Step 1: Pipeline gegen EUR-Lex laufen lassen**

`bash pipelines/eurlex/fetch.sh && python3 pipelines/eurlex/extract.py …`

- [ ] **Step 2: Ingest laufen lassen und die Zahlen ablesen**

`./situs ingest --csv-dir <dir> --db situs.sqlite`, dann aus dem Report:
`Localizations`, `DerivedLabels`.

- [ ] **Step 3: Die Differenz aufschreiben**

`report.json` liefert `eurlex_codes`, `missing_in_index`,
`without_official_name`. **Erwartung aus der Spec: 205 im Index, das BfN nennt
231 für Anhang I.** Die Lücke mit Beispielcodes in
`docs/reference/measured-index.md` festhalten.

**Widerspricht die Messung dem Design — etwa weil EUR-Lex 231 Codes liefert und
26 keinen `habitat_type` finden — anhalten und berichten.** Nicht still
anpassen, keine Zeilen stillschweigend fallen lassen.

- [ ] **Step 4: Erwartung gegen Messung prüfen**

Zünden die 29 Ableitungen? `DerivedLabels` muss 29 sein. Weicht es ab, ist das
ein Fund und gehört in die Doku, nicht in eine Korrektur der Erwartung.

- [ ] **Step 5: Commit**

```bash
git add docs/
git commit -m "docs: measured German label coverage after the EUR-Lex ingest"
```

---

### Task 8: Die 241 verfassten Übersetzungen

**Files:**
- Create: `data/localizations-de-situs.csv`
- Modify: `pipelines/eurlex/` (Zusammenführung der beiden Quellen)

**Dieser Task ist bewusst der letzte.** Der Mechanismus ist vorher grün; keine
Fachentscheidung blockiert einen Test.

- [ ] **Step 1: Die 241 Codes und englischen Namen ziehen**

```sql
SELECT h.code, h.name_en
FROM habitat_type h
WHERE h.typology_id LIKE 'eunis%' AND h.level = 3
  AND NOT EXISTS (
    SELECT 1 FROM habitat_type_crosswalk c
    WHERE c.from_code = h.code AND c.to_typology = 'annex1' AND c.qualifier = '='
  )
ORDER BY h.code;
```

Die Zahl muss **241** sein. Weicht sie ab, anhalten und die Abweichung klären,
bevor eine Zeile geschrieben wird.

- [ ] **Step 2: Datei anlegen, thematisch gruppiert**

Spalten wie der Ingest-Vertrag, plus **eine Kontrollspalte `name_en_source`**,
die die Zusammenführung wieder entfernt — sie existiert nur, damit ein Reviewer
Original und Übersetzung nebeneinander sieht.

Gruppen in dieser Reihenfolge, je mit einer Kommentarzeile: Küste/Dünen (N),
Binnengewässer (C), Moore/Sümpfe (Q), Grasland (R), Gebüsch (S), Wald (T),
Fels/Geröll (U), künstlich (V).

- [ ] **Step 3: Übersetzen, Gruppe für Gruppe**

Regeln aus der Spec, beim Schreiben jeder Zeile:
- `name` ist **treu**: biogeografische Qualifikatoren werden mitübersetzt.
- `vernacular` nur, wenn ein etablierter Begriff **denselben Umfang** trägt.
  Im Zweifel **weglassen** — Zweifel wird nicht zugunsten der Lesbarkeit gelöst.
- Terminologie innerhalb einer Gruppe konsistent halten.

- [ ] **Step 4: Zusammenführung implementieren**

`extract.py` (oder ein zweites Skript) liest `data/localizations-de-situs.csv`,
verwirft `name_en_source` und schreibt die Zeilen mit den EUR-Lex-Zeilen in
**eine** `localizations.csv`. `source` wird auf `situs@<VERSION>` gesetzt.

- [ ] **Step 5: Failing test für die Zusagen der Datei**

`pipelines/eurlex/test_merge.py`:

```python
def test_vernacular_requires_a_name_row(self):
    # Eine vernacular-Zeile ohne name-Zeile ist ein Fehler: der geläufige
    # Begriff ist eine Ergänzung, nicht ein Ersatz.
    ...

def test_no_situs_row_for_a_type_with_an_equals_crosswalk(self):
    # Wo eine Ableitung existiert, wird nichts erfunden.
    ...

def test_every_row_is_level_3_eunis(self):
    ...
```

- [ ] **Step 6: Tests grün, Ingest laufen lassen, verify**

- [ ] **Step 7: Commit**

```bash
git add data/ pipelines/
git commit -m "feat(data): 241 situs-authored German names for EUNIS level 3

Every row carries provenance=situs: situs translated these and no external
source stands behind them. name is faithful to the English original including
its biogeographic qualifiers; vernacular is present only where an established
German term carries the same extent, and absent where it would assert an
equivalence that does not hold."
```

---

## Self-Review

**Spec-Abdeckung.** Umfang → T8/T1. Pipeline → T6. Verfasste Datei → T8.
Provenienz-Wert → T1. Auflösungsreihenfolge → T1. Saat-Invariante → T2.
API-Objekt → T5. `vernacular` → T4/T5. Messung/Bericht → T6/T7. Lizenz → T6
(`source`-Feld) und T7 (Doku). Kein Spec-Abschnitt ohne Task.

**Typkonsistenz.** `input.GermanLabel` wird in T5 definiert und in T4 benutzt —
T4 gibt den Typ zurück, bevor T5 die Wire-Form festlegt. Deshalb muss T4 den Typ
schon anlegen; das ist in T4 Step 3 so vorgesehen und in T5 nur noch um die
JSON-Tags und `omitempty` ergänzt. **Wer T4 und T5 in einer Sitzung macht, legt
den Typ einmal an.**

**Reihenfolge.** T3 (Port) muss vor T4 (vernacular) liegen, sonst braucht T4 eine
zweite Query. T8 zuletzt, damit keine Fachentscheidung einen Test blockiert.
