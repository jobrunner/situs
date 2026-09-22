# Syntaxa-Navigation — Implementierungsplan (Teilprojekt B)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Die nach Teilprojekt A vollständige Syntaxa-Hierarchie wird über HTTP navigierbar: von `GET /v1/syntaxa` (den 25 Formationen, filterbar nach Rang und Lebensform-Gruppe) führt reines Verfolgen von `children` über `GET /v1/syntaxon/{id}` zu jedem der 1326 Verbände, ohne eine ID vorher zu kennen, und jede Antwort trägt ihren Ahnenpfad mit.

**Architecture:** Vier neue Leseoperationen im SQLite-Adapter (`SyntaxonChildren`, `SyntaxonAncestors`, `SyntaxaByRank`, `HabitatTypeCountForSyntaxon`) plus `SyntaxonRanks`; zwei neue Use-Cases in einer **eigenen** Datei `internal/application/syntaxon_nav.go` (nicht in `query.go`, siehe Global Constraints); zwei neue Routen im HTTP-Adapter mit einem eigenen Handler-Paar in `internal/adapters/http/syntaxa.go`; ein gerendertes Explorer-Panel, das neben das bestehende Roh-JSON tritt statt es zu ersetzen.

**Tech Stack:** Go 1.26 (stdlib, `modernc.org/sqlite`, `gorilla/mux`, `cobra`, `viper`). Keine Pipeline-Änderung, kein Python, keine neue Abhängigkeit.

**Spec:** `docs/superpowers/specs/2026-09-21-syntaxa-navigation-design.md` (autoritativ). Setzt `docs/superpowers/specs/2026-09-21-syntaxa-quellenumkehr-design.md` und dessen Plan (`docs/superpowers/plans/2026-09-21-syntaxa-quellenumkehr.md`) als **abgeschlossen** voraus: ohne die Spalten `eea_code`, `source`, `parent_provenance`, `life_form_group`, ohne die Formationsebene und ohne die lückenlose `parent_id`-Kette hat dieser Plan nichts zu navigieren.

## Global Constraints

- `make verify` muss vor jedem Commit grün sein (fmt-check, vet, lint, test, arch, debt, build).
- Null `//nolint`, null `#nosec`, null `TODO`/`FIXME`/`HACK`/`XXX` in Go-Dateien. `.debt-budget` steht auf 0.
- Coverage-Floors sind ein Raise-Only-Ratchet (`.coverage-floors`). Aktuell: `internal/application` **100.0 %**, `internal/domain` 100.0 %, `internal/ports/input` 100 %, `internal/adapters/sqlite` 96 %, `internal/adapters/http` **88 %**, `cmd/situs` 74 %, `internal/ports/output` 66 %. Neuer Code in `internal/application` muss **vollständig** getestet sein — jeder Fehlerpfad der vier neuen Repository-Aufrufe braucht einen Test, sonst fällt das Gate. Ein Floor wird nicht gesenkt.
- SQL-Statements sind **statische Strings** mit `?`-Platzhaltern. Niemals Werte konkatenieren (gosec G201/G202 bricht den Build). Insbesondere: die Join-Tiefe des `life_form_group`-Filters wird **nicht** aus dem `rank`-Wert in die SQL-Zeichenkette gebaut.
- Keine neue direkte Abhängigkeit. `gomodguard_v2` bricht den Build, bis sie in `.golangci.yml` bewusst eingetragen ist. Dieses Teilprojekt braucht keine.
- **Komplexitäts-Ratchet (`.codecharta-ratchet.json`, `make codecharta`).** Der harte Deckel ist `function_complexity.default_cap = 10` — die komplexeste **Funktion** je Datei. `internal/application/query.go`, `ingest.go` und `localize.go` sitzen bereits bei **exakt 10**: jedes Wachstum ihrer schlimmsten Funktion bricht das Gate sofort. Der Deckel je Datei (`complexity.default_cap`) ist 50, Baselines: `internal/application/query.go` **72**, `ingest.go` 64, `cmd/situs/ingest.go` 12 (Funktionsbaseline für `runIngest`). Praktische Folge für diesen Plan, und **keine Geschmacksfrage**: die neuen Use-Cases kommen in eine eigene Datei `internal/application/syntaxon_nav.go`. Präzedenz steht im `_query_note` derselben Datei — `label.go` (Deutsche Labels) und `description.go` (Factsheet-Beschreibungen) wurden genau dafür aus `query.go` abgespalten, statt die Baseline zu heben. Baselines darf man **senken**, Heraufsetzen braucht eine schriftliche Begründung.
- OpenAPI liegt in zwei byte-identischen Kopien: `internal/adapters/http/openapi.yaml` und `api/openapi/openapi.yaml` (`TestOpenAPICopiesAreIdentical`). Der Vertragstest `TestRoutesMatchOpenAPISpec` prüft Routen↔Spec in **beiden** Richtungen und schlägt bei einer Route ohne explizites `.Methods()` an. `docs/reference/http-api.md` wächst mit.
- Fehlercodes sind genau drei: `INVALID_QUERY`, `NOT_FOUND`, `INTERNAL_ERROR`.
- Deutsch für `README.md`, Plan und Commit-Nachrichten. **Code-Kommentare und Doc-Kommentare sind englisch**, sparsam, und nur wo sie ein *Warum* erklären — in Produktions- **und** Testcode, in `.go`-Dateien wie in `explorer.html` (dessen bestehende JS-Kommentare sind ebenfalls englisch). Der Plan zu Teilprojekt A trägt in seinen Codeblöcken deutsche Kommentare; das ist ein Fehler dieses Plans und **kein** Muster, dem hier gefolgt wird. Jeder Kommentar in den Codeblöcken dieses Plans ist entsprechend englisch und wird unverändert übernommen.
- Conventional Commits. `VERSION` und `CHANGELOG.md` gehören release-please — niemals händisch anfassen.

## Zwei Testdoubles brechen, sobald ein Port wächst

Das ist kein Nebenaspekt, sondern der erste Grund, warum ein Task nicht kompiliert:

- `fakeRepo` in `internal/application/ingest_test.go` (Lesehälfte in `internal/application/query_test.go`) implementiert `output.Repository`. **Task 1 und Task 2** erweitern diesen Port und müssen `fakeRepo` im selben Task mitziehen.
- `fakeQueryService` in `internal/adapters/http/handlers_test.go` implementiert `input.QueryService`. **Task 3** erweitert diesen Port und muss `fakeQueryService` im selben Task mitziehen, sonst kompiliert `internal/adapters/http` nicht mehr.

Anders als in Teilprojekt A ist hier **kein** bewusster Bruch in der Mitte vorgesehen: jeder Task endet mit einem grünen `go test ./...`.

## Referenzmesswerte (aus Teilprojekt A, gemessen 2026-09-21)

| Größe | Wert |
|---|---|
| Formationen (`rank='formation'`) | 25 |
| Klassen | 150 |
| Ordnungen | 381 |
| Verbände | 1326 |
| Zeilen ohne Elternteil (außer Formationen) | 0 |
| Maximale Ahnenzahl (Verband → Formation) | **3 Schritte** (4 Knoten) |
| Verbände mit `life_form_group=bryophyte_lichen` (Sektionen R–T) | 137 |
| Verbände in den Sektionen R–Y (Kryptogamen insgesamt) | 190 |

`maxSyntaxonAncestors = 3` ist die **Schrittzahl**, nicht die Knotenzahl. Der Name sagt „Ahnen“, damit die Verwechslung mit der Tiefe 4 nicht passiert.

---

### Task 1: Kinder, Ahnen und Kantenzahl im Repository-Port

**Files:**
- Modify: `internal/ports/output/repository.go`
- Modify: `internal/adapters/sqlite/read_syntaxon.go`
- Test: `internal/adapters/sqlite/read_syntaxon_test.go`
- Test: `internal/application/query_test.go` (fakeRepo-Lesehälfte), `internal/application/ingest_test.go` (fakeRepo-Felder)

**Interfaces:**
- Consumes: `domain.Syntaxon` mit `EEACode`, `Source`, `ParentProvenance`, `LifeFormGroup` und die Konstanten `domain.SyntaxonRankFormation` / `…Class` / `…Order` / `…Alliance`, `domain.LifeFormPhanerogam` / `…BryophyteLichen` / `…Algae` (alle aus Teilprojekt A).
- Produces:
  - `Repository.SyntaxonChildren(ctx context.Context, parentID string) ([]domain.Syntaxon, error)`
  - `Repository.SyntaxonAncestors(ctx context.Context, id string) ([]domain.Syntaxon, error)`
  - `Repository.HabitatTypeCountForSyntaxon(ctx context.Context, syntaxonID string) (int, error)`
  - paketintern: `const maxSyntaxonAncestors = 3`, `func scanSyntaxa(rows *sql.Rows) ([]domain.Syntaxon, error)`

- [ ] **Step 1: Die failing Tests für Kinder und Kantenzahl schreiben**

In `internal/adapters/sqlite/read_syntaxon_test.go` (Paket `sqlite`, interner Testzugriff; der Helfer heißt **`openTestDB(t)`** aus `write_test.go` — nicht `newTestDB`, den es im Repo nicht gibt). Zuerst den Fixture-Helfer, dann die Fälle:

```go
// seedSyntaxaHierarchy fills the small world every navigation test asks about:
// two complete chains formation -> class -> order -> alliance, one phanerogam
// and one cryptogam, plus two alliances under the same order so the ordering
// promise is checkable.
func seedSyntaxaHierarchy(t *testing.T, db *DB) {
	t.Helper()
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// Deliberately NOT inserted in id order: the reads' ordering promise must
	// not come from the insertion order.
	for _, s := range []domain.Syntaxon{
		{ID: "CA01B", Rank: domain.SyntaxonRankAlliance, Name: "Zweiter Verband",
			Author: "Autor 1975", ParentID: "CA01", EEACode: "TST-01B",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01A", Rank: domain.SyntaxonRankAlliance, Name: "Erster Verband",
			Author: "Autor 1970", ParentID: "CA01", EEACode: "TST-01A",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01", Rank: domain.SyntaxonRankOrder, Name: "Testordnung",
			Author: "Autor 1960", ParentID: "CA", EEACode: "TST-01",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA", Rank: domain.SyntaxonRankClass, Name: "Testklasse",
			Author: "Autor 1950", ParentID: "C", EEACode: "TST",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "C", Rank: domain.SyntaxonRankFormation, Name: "Vegetation of the nemoral forest zone",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial,
			LifeFormGroup: domain.LifeFormPhanerogam},
		{ID: "RA01A", Rank: domain.SyntaxonRankAlliance, Name: "Moosverband",
			ParentID: "RA01", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "RA01", Rank: domain.SyntaxonRankOrder, Name: "Moosordnung",
			ParentID: "RA", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "RA", Rank: domain.SyntaxonRankClass, Name: "Moosklasse",
			ParentID: "R", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "R", Rank: domain.SyntaxonRankFormation, Name: "Epigaeic bryophyte and lichen vegetation",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial,
			LifeFormGroup: domain.LifeFormBryophyteLichen},
	} {
		if err := tx.UpsertSyntaxon(s); err != nil {
			t.Fatalf("UpsertSyntaxon(%s): %v", s.ID, err)
		}
	}
	for _, ty := range []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}} {
		if err := tx.UpsertTypology(ty); err != nil {
			t.Fatalf("UpsertTypology: %v", err)
		}
	}
	for _, h := range []domain.HabitatType{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T11"}, NameEN: "Wald"},
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T12"}, NameEN: "Anderer Wald"},
	} {
		if err := tx.UpsertHabitatType(h); err != nil {
			t.Fatalf("UpsertHabitatType(%s): %v", h.Key, err)
		}
	}
	for _, code := range []string{"T11", "T12"} {
		if err := tx.LinkSyntaxon(domain.HabitatTypeKey{Typology: "eunis@2021", Code: code}, "CA01A"); err != nil {
			t.Fatalf("LinkSyntaxon(%s): %v", code, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func openHierarchyDB(t *testing.T) *DB {
	t.Helper()
	db := openTestDB(t)
	seedSyntaxaHierarchy(t, db)
	return db
}

func syntaxonIDs(in []domain.Syntaxon) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, s.ID)
	}
	return out
}

func TestSyntaxonChildrenSindNachIDSortiert(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxonChildren(t.Context(), "CA01")
	if err != nil {
		t.Fatalf("SyntaxonChildren: %v", err)
	}
	if !slices.Equal(syntaxonIDs(got), []string{"CA01A", "CA01B"}) {
		t.Errorf("Kinder = %v, erwartet [CA01A CA01B]", syntaxonIDs(got))
	}
	// Subproject A's extra columns have to travel along, otherwise the API
	// withholds what the index knows.
	if got[0].EEACode != "TST-01A" || got[0].Source != domain.SyntaxonSourceEVC ||
		got[0].ParentProvenance != domain.ParentProvenanceOfficial {
		t.Errorf("CA01A = %+v, erwartet EEA-Code, Quelle und Elternteil-Provenienz", got[0])
	}
}

func TestSyntaxonChildrenEinesVerbandsSindLeerUndNichtNil(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxonChildren(t.Context(), "CA01A")
	if err != nil {
		t.Fatalf("SyntaxonChildren: %v", err)
	}
	if got == nil {
		t.Fatal("Kinder = nil; eine leere Liste ist die Antwort, nil wuerde als JSON-null durchschlagen")
	}
	if len(got) != 0 {
		t.Errorf("Kinder = %v, erwartet leer", syntaxonIDs(got))
	}
}

func TestHabitatTypeCountForSyntaxonZaehltNurDieEigenenKanten(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.HabitatTypeCountForSyntaxon(t.Context(), "CA01A")
	if err != nil {
		t.Fatalf("HabitatTypeCountForSyntaxon: %v", err)
	}
	if got != 2 {
		t.Errorf("Kantenzahl von CA01A = %d, erwartet 2", got)
	}
	// The order above it carries no edge of its own. 0 is the right answer and
	// not an error — which is why the field is called
	// direct_habitat_type_count.
	parent, err := db.HabitatTypeCountForSyntaxon(t.Context(), "CA01")
	if err != nil {
		t.Fatalf("HabitatTypeCountForSyntaxon(CA01): %v", err)
	}
	if parent != 0 {
		t.Errorf("Kantenzahl von CA01 = %d, erwartet 0", parent)
	}
}
```

Die Importzeile der Datei braucht `slices`.

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/adapters/sqlite/ -run 'TestSyntaxonChildren|TestHabitatTypeCountForSyntaxon' -v`
Expected: FAIL — `db.SyntaxonChildren undefined (type *DB has no field or method SyntaxonChildren)` und dasselbe für `HabitatTypeCountForSyntaxon`; die Datei kompiliert nicht.

- [ ] **Step 3: Kinder und Kantenzahl implementieren**

In `internal/adapters/sqlite/read_syntaxon.go` — Importzeile um `slices` erweitern, dann:

```go
// maxSyntaxonAncestors is the number of STEPS from the deepest rank to the
// root: alliance -> order -> class -> formation is three steps and four nodes.
// The name says "ancestors", not "depth", because confusing 3 with 4 is
// otherwise a matter of time. A fourth step means a cycle in the parent_id
// graph or a further rank; either is an index defect and is reported with the
// id that triggered it instead of being experienced as an endless loop.
const maxSyntaxonAncestors = 3

// scanSyntaxa drains rows whose SELECT names the nine syntaxon columns in
// exactly this order. One scan site instead of four: a future tenth column
// must not reach three readers and be forgotten in the fourth.
func scanSyntaxa(rows *sql.Rows) ([]domain.Syntaxon, error) {
	out := []domain.Syntaxon{}
	for rows.Next() {
		var s domain.Syntaxon
		if err := rows.Scan(&s.ID, &s.Rank, &s.Name, &s.Author, &s.ParentID,
			&s.EEACode, &s.Source, &s.ParentProvenance, &s.LifeFormGroup); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SyntaxonChildren returns the direct children of parentID, ordered by id. It
// runs over idx_syntaxon_parent, so a navigation step needs no table scan.
//
// parentID is never empty in the serving path: the route /v1/syntaxon/{id}
// cannot match a blank segment. An empty parentID would legitimately return
// the formations — that is what GET /v1/syntaxa answers, through SyntaxaByRank.
func (d *DB) SyntaxonChildren(ctx context.Context, parentID string) ([]domain.Syntaxon, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, rank, name, author, parent_id, eea_code, source, parent_provenance,
		        life_form_group
		 FROM syntaxon WHERE parent_id = ? ORDER BY id`, parentID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying children of syntaxon %q: %w", parentID, err)
	}
	defer func() { _ = rows.Close() }()

	out, err := scanSyntaxa(rows)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading children of syntaxon %q: %w", parentID, err)
	}
	return out, nil
}

// HabitatTypeCountForSyntaxon counts the edges of exactly this syntaxon without
// loading them. Not the descendants' edges: habitat_type_syntaxon links
// alliances, so a class would report 0 either way — but only this reading makes
// the 0 an honest statement instead of a wrong aggregate.
func (d *DB) HabitatTypeCountForSyntaxon(ctx context.Context, syntaxonID string) (int, error) {
	var n int
	row := d.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM habitat_type_syntaxon WHERE syntaxon_id = ?`, syntaxonID)
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("sqlite: counting habitat types of syntaxon %q: %w", syntaxonID, err)
	}
	return n, nil
}
```

Die drei bestehenden Leseoperationen `Syntaxon`, `Syntaxa` und `AllSyntaxa` bleiben unverändert (Teilprojekt A hat ihre Spaltenlisten schon auf neun Spalten gebracht); `Syntaxa` und `AllSyntaxa` dürfen ihre Scan-Schleife durch `scanSyntaxa` ersetzen, wenn das die Datei kleiner macht — nur dann, und ohne ihre Fehlermeldungen zu verändern.

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./internal/adapters/sqlite/ -run 'TestSyntaxonChildren|TestHabitatTypeCountForSyntaxon' -v`
Expected: PASS, drei Tests.

- [ ] **Step 5: Die failing Tests für den Ahnenpfad schreiben**

```go
func TestSyntaxonAncestorsAeussersteZuerst(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxonAncestors(t.Context(), "CA01A")
	if err != nil {
		t.Fatalf("SyntaxonAncestors: %v", err)
	}
	// Formation, class, order — in that order, so a client can print the path
	// unchanged as a breadcrumb trail.
	if !slices.Equal(syntaxonIDs(got), []string{"C", "CA", "CA01"}) {
		t.Errorf("Ahnen = %v, erwartet [C CA CA01]", syntaxonIDs(got))
	}
	if got[0].LifeFormGroup != domain.LifeFormPhanerogam {
		t.Errorf("Formation im Pfad = %+v, erwartet life_form_group phanerogam", got[0])
	}
}

func TestSyntaxonAncestorsEinerFormationSindLeerUndNichtNil(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxonAncestors(t.Context(), "C")
	if err != nil {
		t.Fatalf("SyntaxonAncestors: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Ahnen einer Formation = %v, erwartet eine leere, nicht-nil Liste", got)
	}
}

func TestSyntaxonAncestorsUnbekannteIDIstNotFound(t *testing.T) {
	db := openHierarchyDB(t)

	_, err := db.SyntaxonAncestors(t.Context(), "GIBTSNICHT")
	if !errors.Is(err, output.ErrNotFound) {
		t.Errorf("Fehler = %v, erwartet output.ErrNotFound", err)
	}
}

func TestSyntaxonAncestorsMeldetBaumelndeElternreferenz(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertSyntaxon(domain.Syntaxon{ID: "WAISE", Rank: domain.SyntaxonRankAlliance,
		Name: "Verband mit ins Leere zeigendem Elternteil", ParentID: "FEHLT",
		Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived}); err != nil {
		t.Fatalf("UpsertSyntaxon: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	_, err = db.SyntaxonAncestors(t.Context(), "WAISE")
	if err == nil {
		t.Fatal("SyntaxonAncestors hat eine baumelnde parent_id ueberbrueckt")
	}
	if !strings.Contains(err.Error(), "FEHLT") {
		t.Errorf("Fehler benennt das fehlende Elternteil nicht: %v", err)
	}
	// The decisive part: this is an index defect (500), not a missing result
	// (404). Were the error to wrap output.ErrNotFound, the use case would
	// turn it into a NOT_FOUND for an id that does exist.
	if errors.Is(err, output.ErrNotFound) {
		t.Error("der Fehler umwickelt output.ErrNotFound; eine baumelnde Referenz darf kein 404 werden")
	}
}

func TestSyntaxonAncestorsLaeuftBeiEinemZykelInDenTiefenfehler(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, s := range []domain.Syntaxon{
		{ID: "ZYK-A", Rank: domain.SyntaxonRankAlliance, Name: "A", ParentID: "ZYK-B",
			Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived},
		{ID: "ZYK-B", Rank: domain.SyntaxonRankOrder, Name: "B", ParentID: "ZYK-A",
			Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived},
	} {
		if err := tx.UpsertSyntaxon(s); err != nil {
			t.Fatalf("UpsertSyntaxon(%s): %v", s.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	_, err = db.SyntaxonAncestors(t.Context(), "ZYK-A")
	if err == nil {
		t.Fatal("SyntaxonAncestors lief in einem Zykel durch")
	}
	if !strings.Contains(err.Error(), "ZYK-A") {
		t.Errorf("Fehler benennt die ausloesende ID nicht: %v", err)
	}
}
```

Die Importzeile braucht `errors`, `strings`, `slices` und `output` (alle bereits in anderen Testdateien des Pakets verwendet).

- [ ] **Step 6: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/adapters/sqlite/ -run TestSyntaxonAncestors -v`
Expected: FAIL — `db.SyntaxonAncestors undefined`.

- [ ] **Step 7: Den Ahnenpfad implementieren**

```go
// SyntaxonAncestors walks parent_id to the root, OUTERMOST first (formation,
// then class, then order). Empty for a formation.
//
// Three failure modes, deliberately told apart: an unknown start id wraps
// output.ErrNotFound (a missing answer, 404); a parent_id pointing at a row
// that does not exist does NOT (an index defect, 500 — bridging it would serve
// a shortened breadcrumb trail as if it were complete); and more than
// maxSyntaxonAncestors steps means a cycle or a further rank, reported with the
// id that triggered it instead of looping.
func (d *DB) SyntaxonAncestors(ctx context.Context, id string) ([]domain.Syntaxon, error) {
	current, err := d.Syntaxon(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []domain.Syntaxon{}
	for steps := 0; current.ParentID != ""; steps++ {
		if steps == maxSyntaxonAncestors {
			return nil, fmt.Errorf(
				"sqlite: syntaxon %q has more than %d ancestors: the parent_id graph is cyclic or carries a further rank",
				id, maxSyntaxonAncestors)
		}
		parent, perr := d.Syntaxon(ctx, current.ParentID)
		if perr != nil {
			if errors.Is(perr, output.ErrNotFound) {
				return nil, fmt.Errorf("sqlite: syntaxon %q has parent_id %q, which no row carries",
					current.ID, current.ParentID)
			}
			return nil, fmt.Errorf("sqlite: walking the ancestors of %q: %w", id, perr)
		}
		out = append(out, parent)
		current = parent
	}
	slices.Reverse(out)
	return out, nil
}
```

Eine Abfrage je Ebene, höchstens vier insgesamt, jede über den Primärschlüssel — kein Full-Table-Scan, wie die prüfbare Zusage aus Abschnitt 9 des Specs es verlangt.

- [ ] **Step 8: Tests laufen lassen und grün sehen**

Run: `go test ./internal/adapters/sqlite/ -run 'TestSyntaxon|TestHabitatTypeCount' -v`
Expected: PASS, acht Tests.

- [ ] **Step 9: Den Port erweitern**

In `internal/ports/output/repository.go`, im `Repository`-Interface direkt nach `AllSyntaxa`:

```go
	// SyntaxonChildren returns the direct children of parentID, ordered by id.
	// An alliance has none: an empty slice is the answer, not an error — that
	// is the lower bound of the free data (see the known ceiling).
	SyntaxonChildren(ctx context.Context, parentID string) ([]domain.Syntaxon, error)
	// SyntaxonAncestors walks parent_id to the root, OUTERMOST first
	// (formation, class, order), and is empty for a formation. An unknown id
	// is ErrNotFound; a parent_id pointing at a missing row and a cycle are
	// both index defects and are reported as plain errors that do NOT wrap
	// ErrNotFound, so a caller cannot turn them into a 404 for an id that
	// exists.
	SyntaxonAncestors(ctx context.Context, id string) ([]domain.Syntaxon, error)
	// HabitatTypeCountForSyntaxon counts the edges of exactly this syntaxon,
	// not its descendants', and without loading them.
	HabitatTypeCountForSyntaxon(ctx context.Context, syntaxonID string) (int, error)
```

- [ ] **Step 10: fakeRepo nachziehen**

In `internal/application/ingest_test.go` die Fehler-Haken im Struct ergänzen (bei den anderen Lesefehlern, damit Task 3 jeden Fehlerpfad abdecken kann — `internal/application` steht auf 100 %):

```go
	// The navigation failures (subproject B): each fails exactly one of the
	// new reads, so a use-case test can pin that the failure surfaces.
	syntaxonChildrenErr  error
	syntaxonAncestorsErr error
	habitatTypeCountErr  error
```

In `internal/application/query_test.go`, bei der Lesehälfte von `fakeRepo` (direkt nach `Syntaxa`):

```go
func (r *fakeRepo) SyntaxonChildren(_ context.Context, parentID string) ([]domain.Syntaxon, error) {
	if r.syntaxonChildrenErr != nil {
		return nil, r.syntaxonChildrenErr
	}
	out := []domain.Syntaxon{}
	for _, s := range r.syntaxa {
		if s.ParentID == parentID {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// SyntaxonAncestors mirrors the sqlite adapter including the distinction that
// matters: an unknown start id wraps output.ErrNotFound, a dangling parent_id
// does not. A fake that collapsed the two would let a 404-for-an-existing-id
// bug pass the use-case test.
func (r *fakeRepo) SyntaxonAncestors(ctx context.Context, id string) ([]domain.Syntaxon, error) {
	if r.syntaxonAncestorsErr != nil {
		return nil, r.syntaxonAncestorsErr
	}
	current, err := r.Syntaxon(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []domain.Syntaxon{}
	for steps := 0; current.ParentID != ""; steps++ {
		if steps == 3 {
			return nil, fmt.Errorf("fakeRepo: syntaxon %q has more than 3 ancestors", id)
		}
		parent, perr := r.Syntaxon(ctx, current.ParentID)
		if perr != nil {
			return nil, fmt.Errorf("fakeRepo: syntaxon %q has parent_id %q, which no row carries",
				current.ID, current.ParentID)
		}
		out = append(out, parent)
		current = parent
	}
	slices.Reverse(out)
	return out, nil
}

func (r *fakeRepo) HabitatTypeCountForSyntaxon(_ context.Context, syntaxonID string) (int, error) {
	if r.habitatTypeCountErr != nil {
		return 0, r.habitatTypeCountErr
	}
	n := 0
	for _, link := range r.syntaxaLinks {
		if link.syntaxonID == syntaxonID {
			n++
		}
	}
	return n, nil
}
```

Die Importzeile von `query_test.go` braucht `slices`.

- [ ] **Step 11: Das ganze Modul testen**

Run: `go test ./...`
Expected: PASS. Bricht `internal/application` mit „missing method SyntaxonChildren“, fehlt Step 10.

- [ ] **Step 12: Commit**

```bash
git add internal/ports/output/repository.go internal/adapters/sqlite/ internal/application/
git commit -m "feat(sqlite): Kinder, Ahnenpfad und Kantenzahl eines Syntaxons lesen"
```

---

### Task 2: `SyntaxaByRank` mit rekursivem CTE und die Rangliste aus dem Index

**Files:**
- Modify: `internal/ports/output/repository.go`
- Modify: `internal/adapters/sqlite/read_syntaxon.go`
- Test: `internal/adapters/sqlite/read_syntaxon_test.go`
- Test: `internal/application/query_test.go`, `internal/application/ingest_test.go` (fakeRepo)

**Interfaces:**
- Consumes: `scanSyntaxa`, `maxSyntaxonAncestors` (Task 1); `seedSyntaxaHierarchy` (Task 1).
- Produces:
  - `Repository.SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string) ([]domain.Syntaxon, error)`
  - `Repository.SyntaxonRanks(ctx context.Context) ([]string, error)`
  - paketintern: `func (d *DB) syntaxaByRankRows(ctx context.Context, rank, lifeFormGroup string) (*sql.Rows, error)`

- [ ] **Step 1: Die failing Tests schreiben**

In `internal/adapters/sqlite/read_syntaxon_test.go`:

```go
func TestSyntaxaByRankOhneGruppenfilter(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankClass, "")
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if !slices.Equal(syntaxonIDs(got), []string{"CA", "RA"}) {
		t.Errorf("Klassen = %v, erwartet [CA RA]", syntaxonIDs(got))
	}
}

func TestSyntaxaByRankFiltertVerbaendeUeberDreiEbenenNachOben(t *testing.T) {
	db := openHierarchyDB(t)

	// Per subproject A the filter value sits on the formation row ONLY. An
	// alliance is three levels away — which is what this test exercises.
	got, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankAlliance, domain.LifeFormBryophyteLichen)
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if !slices.Equal(syntaxonIDs(got), []string{"RA01A"}) {
		t.Errorf("Moosverbaende = %v, erwartet [RA01A]", syntaxonIDs(got))
	}

	phanerogam, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankAlliance, domain.LifeFormPhanerogam)
	if err != nil {
		t.Fatalf("SyntaxaByRank(phanerogam): %v", err)
	}
	if !slices.Equal(syntaxonIDs(phanerogam), []string{"CA01A", "CA01B"}) {
		t.Errorf("phanerogame Verbaende = %v, erwartet [CA01A CA01B]", syntaxonIDs(phanerogam))
	}
}

func TestSyntaxaByRankFiltertAuchAufDerFormationsebene(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankFormation, domain.LifeFormBryophyteLichen)
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if !slices.Equal(syntaxonIDs(got), []string{"R"}) {
		t.Errorf("Formationen = %v, erwartet [R]", syntaxonIDs(got))
	}
}

func TestSyntaxaByRankUnbekannterRangIstEineLeereListe(t *testing.T) {
	db := openHierarchyDB(t)

	// Validating the value is the read side's job, not the repository's: here
	// "there are none" is the honest answer; the use case turns it into
	// INVALID_QUERY (task 3).
	got, err := db.SyntaxaByRank(t.Context(), "association", "")
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Ergebnis = %v, erwartet eine leere, nicht-nil Liste", got)
	}
}

func TestSyntaxaByRankLaeuftBeiEinemZykelNichtEndlos(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, s := range []domain.Syntaxon{
		{ID: "ZYK-A", Rank: domain.SyntaxonRankAlliance, Name: "A", ParentID: "ZYK-B",
			Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived},
		{ID: "ZYK-B", Rank: domain.SyntaxonRankOrder, Name: "B", ParentID: "ZYK-A",
			Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived},
	} {
		if err := tx.UpsertSyntaxon(s); err != nil {
			t.Fatalf("UpsertSyntaxon(%s): %v", s.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Without the step bound in the CTE this query does not fail — it never
	// returns at all. It has to terminate and yield an empty list: neither row
	// reaches a formation.
	got, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankAlliance, domain.LifeFormPhanerogam)
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Ergebnis = %v, erwartet leer — keine Zeile erreicht eine Formation", syntaxonIDs(got))
	}
}

func TestSyntaxonRanksLiefertDieVorhandenenRaengeSortiert(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxonRanks(t.Context())
	if err != nil {
		t.Fatalf("SyntaxonRanks: %v", err)
	}
	if !slices.Equal(got, []string{"alliance", "class", "formation", "order"}) {
		t.Errorf("Raenge = %v, erwartet [alliance class formation order]", got)
	}
}

func TestSyntaxonRanksEinesLeerenIndexIstLeerUndNichtNil(t *testing.T) {
	db := openTestDB(t)

	got, err := db.SyntaxonRanks(t.Context())
	if err != nil {
		t.Fatalf("SyntaxonRanks: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Raenge = %v, erwartet eine leere, nicht-nil Liste", got)
	}
}
```

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/adapters/sqlite/ -run 'TestSyntaxaByRank|TestSyntaxonRanks' -v`
Expected: FAIL — `db.SyntaxaByRank undefined` und `db.SyntaxonRanks undefined`.

- [ ] **Step 3: Implementieren**

In `internal/adapters/sqlite/read_syntaxon.go`:

```go
// SyntaxaByRank returns every syntaxon of rank, ordered by id. A non-empty
// lifeFormGroup keeps only those whose reachable formation carries that group.
func (d *DB) SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string) ([]domain.Syntaxon, error) {
	rows, err := d.syntaxaByRankRows(ctx, rank, lifeFormGroup)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying syntaxa of rank %q: %w", rank, err)
	}
	defer func() { _ = rows.Close() }()

	out, err := scanSyntaxa(rows)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading syntaxa of rank %q: %w", rank, err)
	}
	return out, nil
}

// syntaxaByRankRows picks between TWO static statements — never one assembled
// from the rank value.
//
// life_form_group is stored on formation rows alone, so the filtered variant
// has to join upwards, and the depth differs per rank (an alliance three steps,
// a class one). Building that depth from the rank string would be exactly the
// SQL string assembly gosec G201 forbids, and a statement per rank would be
// four near-identical literals that go stale the moment a further rank becomes
// a data row. One recursive CTE covers every depth instead.
//
// The step bound is not decoration: without it a cycle in parent_id would make
// this statement recurse until memory runs out — a hang rather than an error.
// It is passed as a parameter so the Go constant stays the single source of the
// number; the placeholders bind in the order they appear (steps, rank, group).
func (d *DB) syntaxaByRankRows(ctx context.Context, rank, lifeFormGroup string) (*sql.Rows, error) {
	if lifeFormGroup == "" {
		return d.QueryContext(ctx,
			`SELECT id, rank, name, author, parent_id, eea_code, source, parent_provenance,
			        life_form_group
			 FROM syntaxon WHERE rank = ? ORDER BY id`, rank)
	}
	return d.QueryContext(ctx,
		`WITH RECURSIVE up(id, root, steps) AS (
		   SELECT id, id, 0 FROM syntaxon
		   UNION ALL
		   SELECT u.id, s.parent_id, u.steps + 1
		   FROM up u JOIN syntaxon s ON s.id = u.root
		   WHERE s.parent_id <> '' AND u.steps < ?
		 )
		 SELECT s.id, s.rank, s.name, s.author, s.parent_id, s.eea_code, s.source,
		        s.parent_provenance, s.life_form_group
		 FROM syntaxon s
		 JOIN up ON up.id = s.id
		 JOIN syntaxon f ON f.id = up.root AND f.rank = 'formation'
		 WHERE s.rank = ? AND f.life_form_group = ?
		 ORDER BY s.id`,
		maxSyntaxonAncestors, rank, lifeFormGroup)
}

// SyntaxonRanks lists the distinct ranks the index carries, sorted. The read
// side validates ?rank= against THIS and not against a wired list: the schema
// deliberately has no CHECK on rank so a further rank can be a data row, and a
// fixed enum one layer up would have taken that freedom back and made a new
// rank unfindable.
func (d *DB) SyntaxonRanks(ctx context.Context) ([]string, error) {
	rows, err := d.QueryContext(ctx, `SELECT DISTINCT rank FROM syntaxon ORDER BY rank`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: listing syntaxon ranks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []string{}
	for rows.Next() {
		var rank string
		if err := rows.Scan(&rank); err != nil {
			return nil, fmt.Errorf("sqlite: scanning syntaxon rank: %w", err)
		}
		out = append(out, rank)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading syntaxon ranks: %w", err)
	}
	return out, nil
}
```

`'formation'` steht als Literal im CTE — es ist die Wurzelbedingung, kein Eingabewert; `domain.SyntaxonRankFormation` hält denselben Wert und ist durch den Domänentest aus Teilprojekt A festgenagelt.

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./internal/adapters/sqlite/ -run 'TestSyntaxaByRank|TestSyntaxonRanks' -v`
Expected: PASS, sieben Tests. Hängt `TestSyntaxaByRankLaeuftBeiEinemZykelNichtEndlos`, fehlt die Schrittgrenze `AND u.steps < ?`.

- [ ] **Step 5: Den Port erweitern**

In `internal/ports/output/repository.go`, direkt nach `HabitatTypeCountForSyntaxon`:

```go
	// SyntaxaByRank returns every syntaxon of rank, ordered by id. A non-empty
	// lifeFormGroup keeps only those whose reachable formation carries that
	// group — the value is stored on formation rows alone. A rank the index
	// does not carry yields an empty list here; turning that into an
	// INVALID_QUERY is the read side's job, which needs SyntaxonRanks for it.
	SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string) ([]domain.Syntaxon, error)
	// SyntaxonRanks lists the distinct ranks the index carries, sorted.
	SyntaxonRanks(ctx context.Context) ([]string, error)
```

`AllSyntaxa` bleibt unverändert bestehen: Teilprojekt A liest damit weiter alle Syntaxa für den Namensabgleich der 16 EEA-eigenen Einheiten. `SyntaxaByRank` tritt neben sie, es ersetzt sie nicht.

- [ ] **Step 6: fakeRepo nachziehen**

In `internal/application/ingest_test.go` zwei weitere Fehler-Haken:

```go
	syntaxaByRankErr  error
	syntaxonRanksErr  error
```

In `internal/application/query_test.go`:

```go
// SyntaxaByRank mirrors the adapter's contract: rank filter, then — if a group
// was asked for — the group of the formation the parent_id chain reaches. It
// walks the chain itself rather than reading a seeded answer, so a test cannot
// pass by seeding a group onto a non-formation row.
func (r *fakeRepo) SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string) ([]domain.Syntaxon, error) {
	if r.syntaxaByRankErr != nil {
		return nil, r.syntaxaByRankErr
	}
	out := []domain.Syntaxon{}
	for _, s := range r.syntaxa {
		if s.Rank != rank {
			continue
		}
		if lifeFormGroup != "" && r.formationGroupOf(ctx, s) != lifeFormGroup {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// formationGroupOf walks parent_id to the root and returns its group, or "" if
// the chain breaks or exceeds the measured depth.
func (r *fakeRepo) formationGroupOf(ctx context.Context, s domain.Syntaxon) string {
	current := s
	for steps := 0; current.ParentID != ""; steps++ {
		if steps == 3 {
			return ""
		}
		parent, err := r.Syntaxon(ctx, current.ParentID)
		if err != nil {
			return ""
		}
		current = parent
	}
	return current.LifeFormGroup
}

func (r *fakeRepo) SyntaxonRanks(_ context.Context) ([]string, error) {
	if r.syntaxonRanksErr != nil {
		return nil, r.syntaxonRanksErr
	}
	seen := map[string]bool{}
	out := []string{}
	for _, s := range r.syntaxa {
		if !seen[s.Rank] {
			seen[s.Rank] = true
			out = append(out, s.Rank)
		}
	}
	sort.Strings(out)
	return out, nil
}
```

- [ ] **Step 7: Das ganze Modul testen**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/ports/output/repository.go internal/adapters/sqlite/ internal/application/
git commit -m "feat(sqlite): Syntaxa nach Rang und Lebensform-Gruppe lesen, Raenge aus dem Index"
```

---

### Task 3: `SyntaxonDetail` und die zwei Use-Cases in eigener Datei

**Files:**
- Modify: `internal/ports/input/services.go`
- Create: `internal/application/syntaxon_nav.go`
- Modify: `internal/application/query.go` (nur `syntaxaOf` auf den geteilten Mapper umstellen)
- Test: `internal/application/syntaxon_nav_test.go`
- Test: `internal/adapters/http/handlers_test.go` (fakeQueryService)

**Warum eine eigene Datei, nicht `query.go`:** `internal/application/query.go` sitzt im Ratchet bei Funktionskomplexität **exakt 10** und Datei-Baseline **72**. Zwei weitere Use-Cases samt Mapper sprengen die Baseline sofort, und Heraufsetzen braucht eine schriftliche Begründung, die es hier nicht gibt: `label.go` und `description.go` wurden aus genau diesem Grund abgespalten. `syntaxon_nav.go` ist deshalb **vorgegeben, nicht optional**.

**Interfaces:**
- Consumes: `Repository.SyntaxonChildren`, `SyntaxonAncestors`, `HabitatTypeCountForSyntaxon` (Task 1), `SyntaxaByRank`, `SyntaxonRanks` (Task 2); `translateNotFound` aus `query.go`.
- Produces:
  - `input.SyntaxonDetail` (eingebettetes `input.SyntaxonRef` + `Ancestors`, `Children`, `DirectHabitatTypeCount`)
  - `input.QueryService.Syntaxon(ctx context.Context, id, lang string) (SyntaxonDetail, error)`
  - `input.QueryService.SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string) ([]SyntaxonRef, error)`
  - paketintern: `syntaxonRef(domain.Syntaxon) input.SyntaxonRef`, `syntaxonRefs([]domain.Syntaxon) []input.SyntaxonRef`, `lifeFormGroupOf(domain.Syntaxon, []domain.Syntaxon) string`, `(*QueryService).requireRank`

- [ ] **Step 1: Die failing Tests schreiben**

`internal/application/syntaxon_nav_test.go` (Paket `application_test`, wie die übrigen Tests des Pakets):

```go
// seedNavRepo builds the world the navigation tests ask about: a phanerogam
// chain C -> CA -> CA01 -> CA01A/CA01B, a cryptogam chain R -> RA -> RA01 ->
// RA01A, and two habitat-type edges on CA01A.
func seedNavRepo(t *testing.T) *fakeRepo {
	t.Helper()
	repo := newFakeRepo()
	repo.syntaxa = []domain.Syntaxon{
		{ID: "C", Rank: domain.SyntaxonRankFormation, Name: "Vegetation of the nemoral forest zone",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial,
			LifeFormGroup: domain.LifeFormPhanerogam},
		{ID: "CA", Rank: domain.SyntaxonRankClass, Name: "Testklasse", Author: "Autor 1950",
			ParentID: "C", EEACode: "TST", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01", Rank: domain.SyntaxonRankOrder, Name: "Testordnung", Author: "Autor 1960",
			ParentID: "CA", EEACode: "TST-01", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01A", Rank: domain.SyntaxonRankAlliance, Name: "Erster Verband", Author: "Autor 1970",
			ParentID: "CA01", EEACode: "TST-01A", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01B", Rank: domain.SyntaxonRankAlliance, Name: "Zweiter Verband", Author: "Autor 1975",
			ParentID: "CA01", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "R", Rank: domain.SyntaxonRankFormation, Name: "Epigaeic bryophyte and lichen vegetation",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial,
			LifeFormGroup: domain.LifeFormBryophyteLichen},
		{ID: "RA", Rank: domain.SyntaxonRankClass, Name: "Moosklasse", ParentID: "R",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "RA01", Rank: domain.SyntaxonRankOrder, Name: "Moosordnung", ParentID: "RA",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "RA01A", Rank: domain.SyntaxonRankAlliance, Name: "Moosverband", ParentID: "RA01",
			Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived},
	}
	for _, code := range []string{"T11", "T12"} {
		if err := repo.LinkSyntaxon(domain.HabitatTypeKey{Typology: "eunis@2021", Code: code}, "CA01A"); err != nil {
			t.Fatalf("LinkSyntaxon(%s): %v", code, err)
		}
	}
	return repo
}

func refIDs(in []input.SyntaxonRef) []string {
	out := make([]string, 0, len(in))
	for _, r := range in {
		out = append(out, r.ID)
	}
	return out
}

func TestSyntaxon_FormationHatLeerenAhnenpfadUndIhreKlassenAlsKinder(t *testing.T) {
	q := application.NewQueryService(seedNavRepo(t))

	got, err := q.Syntaxon(t.Context(), "C", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.Ancestors == nil || len(got.Ancestors) != 0 {
		t.Errorf("Ancestors = %v, erwartet leer und nicht nil", got.Ancestors)
	}
	if !slices.Equal(refIDs(got.Children), []string{"CA"}) {
		t.Errorf("Children = %v, erwartet [CA]", refIDs(got.Children))
	}
	if got.LifeFormGroup != domain.LifeFormPhanerogam {
		t.Errorf("LifeFormGroup = %q, erwartet phanerogam (auf der Zeile gespeichert)", got.LifeFormGroup)
	}
	if got.DirectHabitatTypeCount != 0 {
		t.Errorf("DirectHabitatTypeCount = %d, erwartet 0", got.DirectHabitatTypeCount)
	}
}

func TestSyntaxon_KlasseUndOrdnungTragenDenPfadAeussersteZuerst(t *testing.T) {
	q := application.NewQueryService(seedNavRepo(t))

	class, err := q.Syntaxon(t.Context(), "CA", "en")
	if err != nil {
		t.Fatalf("Syntaxon(CA): %v", err)
	}
	if !slices.Equal(refIDs(class.Ancestors), []string{"C"}) {
		t.Errorf("Ancestors(CA) = %v, erwartet [C]", refIDs(class.Ancestors))
	}
	order, err := q.Syntaxon(t.Context(), "CA01", "en")
	if err != nil {
		t.Fatalf("Syntaxon(CA01): %v", err)
	}
	if !slices.Equal(refIDs(order.Ancestors), []string{"C", "CA"}) {
		t.Errorf("Ancestors(CA01) = %v, erwartet [C CA]", refIDs(order.Ancestors))
	}
	if !slices.Equal(refIDs(order.Children), []string{"CA01A", "CA01B"}) {
		t.Errorf("Children(CA01) = %v, erwartet [CA01A CA01B]", refIDs(order.Children))
	}
}

func TestSyntaxon_VerbandHatDreiAhnenKeineKinderUndSeineKantenzahl(t *testing.T) {
	q := application.NewQueryService(seedNavRepo(t))

	got, err := q.Syntaxon(t.Context(), "CA01A", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if !slices.Equal(refIDs(got.Ancestors), []string{"C", "CA", "CA01"}) {
		t.Errorf("Ancestors = %v, erwartet [C CA CA01]", refIDs(got.Ancestors))
	}
	if got.Children == nil || len(got.Children) != 0 {
		t.Errorf("Children = %v, erwartet leer und nicht nil", got.Children)
	}
	if got.DirectHabitatTypeCount != 2 {
		t.Errorf("DirectHabitatTypeCount = %d, erwartet 2", got.DirectHabitatTypeCount)
	}
	if got.EEACode != "TST-01A" || got.Source != domain.SyntaxonSourceEVC ||
		got.ParentProvenance != domain.ParentProvenanceOfficial || got.Author != "Autor 1970" {
		t.Errorf("SyntaxonRef = %+v, erwartet die Herkunftsfelder aus Teilprojekt A", got.SyntaxonRef)
	}
}

func TestSyntaxon_LeitetDieLebensformGruppeAusDerFormationAb(t *testing.T) {
	q := application.NewQueryService(seedNavRepo(t))

	got, err := q.Syntaxon(t.Context(), "RA01A", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	// The value sits on formation row R alone. Derived, not read — and
	// verifiable, because ancestors travels in the same answer.
	if got.LifeFormGroup != domain.LifeFormBryophyteLichen {
		t.Errorf("LifeFormGroup = %q, erwartet bryophyte_lichen aus Formation R", got.LifeFormGroup)
	}
	if len(got.Ancestors) == 0 || got.Ancestors[0].ID != "R" {
		t.Errorf("Ancestors = %v, erwartet R an erster Stelle", refIDs(got.Ancestors))
	}
}

func TestSyntaxon_UnbekannteIDIstNotFound(t *testing.T) {
	q := application.NewQueryService(seedNavRepo(t))

	_, err := q.Syntaxon(t.Context(), "GIBTSNICHT", "en")
	if !errors.Is(err, input.ErrNotFound) {
		t.Errorf("Fehler = %v, erwartet input.ErrNotFound", err)
	}
}

func TestSyntaxon_BaumelndeElternreferenzIstEinInkonsistenterIndex(t *testing.T) {
	repo := seedNavRepo(t)
	repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{
		ID: "WAISE", Rank: domain.SyntaxonRankAlliance, Name: "Zeigt ins Leere",
		ParentID: "FEHLT", Source: domain.SyntaxonSourceEUNIS,
		ParentProvenance: domain.ParentProvenanceDerived})
	q := application.NewQueryService(repo)

	_, err := q.Syntaxon(t.Context(), "WAISE", "en")
	if err == nil {
		t.Fatal("Syntaxon hat eine baumelnde parent_id ueberbrueckt")
	}
	if !strings.Contains(err.Error(), "index is inconsistent") {
		t.Errorf("Fehler = %v, erwartet die Inkonsistenz-Meldung", err)
	}
	// A 404 would be wrong: WAISE exists, the index is broken.
	if errors.Is(err, input.ErrNotFound) {
		t.Error("der Fehler wird als NOT_FOUND klassifiziert, erwartet INTERNAL_ERROR")
	}
}

func TestSyntaxon_MeldetFehlerDerKinderAbfrage(t *testing.T) {
	repo := seedNavRepo(t)
	repo.syntaxonChildrenErr = errors.New("boom")
	q := application.NewQueryService(repo)

	if _, err := q.Syntaxon(t.Context(), "CA01", "en"); err == nil {
		t.Fatal("Syntaxon verschluckt den Fehler der Kinder-Abfrage")
	}
}

func TestSyntaxon_MeldetFehlerDerKantenzaehlung(t *testing.T) {
	repo := seedNavRepo(t)
	repo.habitatTypeCountErr = errors.New("boom")
	q := application.NewQueryService(repo)

	if _, err := q.Syntaxon(t.Context(), "CA01A", "en"); err == nil {
		t.Fatal("Syntaxon verschluckt den Fehler der Kantenzaehlung")
	}
}

func TestSyntaxaByRank_ListetEinenRang(t *testing.T) {
	q := application.NewQueryService(seedNavRepo(t))

	got, err := q.SyntaxaByRank(t.Context(), domain.SyntaxonRankClass, "")
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if !slices.Equal(refIDs(got), []string{"CA", "RA"}) {
		t.Errorf("Klassen = %v, erwartet [CA RA]", refIDs(got))
	}
}

func TestSyntaxaByRank_KombiniertRangUndGruppeAlsUnd(t *testing.T) {
	q := application.NewQueryService(seedNavRepo(t))

	got, err := q.SyntaxaByRank(t.Context(), domain.SyntaxonRankAlliance, domain.LifeFormBryophyteLichen)
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if !slices.Equal(refIDs(got), []string{"RA01A"}) {
		t.Errorf("Moosverbaende = %v, erwartet [RA01A]", refIDs(got))
	}
}

func TestSyntaxaByRank_UnbekannterRangIstInvalidQueryMitDenErlaubtenWerten(t *testing.T) {
	q := application.NewQueryService(seedNavRepo(t))

	_, err := q.SyntaxaByRank(t.Context(), "association", "")
	if !errors.Is(err, input.ErrInvalidQuery) {
		t.Fatalf("Fehler = %v, erwartet input.ErrInvalidQuery", err)
	}
	// An empty list would be the wrong answer: it looks like "there are none"
	// while it means "you mistyped".
	for _, want := range []string{"alliance", "class", "formation", "order"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Nachricht %q nennt den erlaubten Wert %q nicht", err.Error(), want)
		}
	}
}

func TestSyntaxaByRank_MeldetFehlerDerRanglisteUndDerAbfrage(t *testing.T) {
	ranksBroken := seedNavRepo(t)
	ranksBroken.syntaxonRanksErr = errors.New("boom")
	if _, err := application.NewQueryService(ranksBroken).
		SyntaxaByRank(t.Context(), domain.SyntaxonRankClass, ""); err == nil {
		t.Error("SyntaxaByRank verschluckt den Fehler der Rangliste")
	}

	queryBroken := seedNavRepo(t)
	queryBroken.syntaxaByRankErr = errors.New("boom")
	if _, err := application.NewQueryService(queryBroken).
		SyntaxaByRank(t.Context(), domain.SyntaxonRankClass, ""); err == nil {
		t.Error("SyntaxaByRank verschluckt den Fehler der Abfrage")
	}
}
```

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/application/ -run 'TestSyntaxon_|TestSyntaxaByRank_' -v`
Expected: FAIL — `q.Syntaxon undefined (type *application.QueryService has no field or method Syntaxon)` und dasselbe für `SyntaxaByRank`; das Testpaket kompiliert nicht.

- [ ] **Step 3: Das DTO und den Port schreiben**

In `internal/ports/input/services.go`, direkt nach `SyntaxonRef`:

```go
// SyntaxonDetail is a syntaxon with its surroundings: the way up and the direct
// children. Both answer the same question ("where am I and where can I go?")
// and therefore belong in the same response — a separate /children or
// /ancestors route would be a second way to the same data and would make a
// breadcrumb trail cost three requests.
//
// SyntaxonRef is EMBEDDED, so Go promotes its fields into this same JSON
// object: id, rank, name, author, parent_id, eea_code, source,
// parent_provenance and life_form_group are siblings of ancestors and children
// on the wire, not a nested object. The OpenAPI schema models that as allOf and
// a test pins it, because a nested schema and a flat wire format would be two
// contracts claiming to be one.
//
// life_form_group is deliberately NOT repeated here: a field of the same name
// in the outer struct would shadow the embedded one and put two fields on one
// JSON key. For every rank but formation the value is derived from the
// formation the ancestor path reaches — derived, not stored, and verifiable by
// the client because ancestors travels in the same response.
type SyntaxonDetail struct {
	SyntaxonRef

	// Ancestors is the way to the root, OUTERMOST first (formation, then
	// class, then order), and empty for a formation. The order is fixed so a
	// client can print it unchanged as a breadcrumb trail.
	//
	// No omitempty, and never nil: an empty list and a missing field are
	// different statements for a client.
	Ancestors []SyntaxonRef `json:"ancestors"`

	// Children are the direct children, ordered by id. Empty for an alliance —
	// the lower bound of the data, not an error. No omitempty, same reason as
	// Ancestors.
	Children []SyntaxonRef `json:"children"`

	// DirectHabitatTypeCount is the number of habitat types linking EXACTLY
	// this syntaxon, not its descendants'. For a class or formation it is
	// therefore almost always 0, because habitat_type_syntaxon links alliances
	// (and in one case an order). The name says so, so a client does not read
	// the 0 as "this class touches no EUNIS type"; aggregating over the
	// descendants is a question of its own.
	DirectHabitatTypeCount int `json:"direct_habitat_type_count"`
}
```

Im `QueryService`-Interface, nach `SyntaxonHabitatTypes`:

```go
	// Syntaxon returns one vegetation unit with its ancestor path and its
	// direct children — the whole navigation step in one answer. An unknown id
	// is ErrNotFound; a parent_id pointing at a missing row is an inconsistent
	// index and is reported as such, never bridged.
	//
	// lang is accepted and, for now, not consulted: syntaxa carry no German
	// labels yet (design, section 10). It is in the signature so adding them
	// later is not a contract change, and so the adapter's language(r) logic
	// stays uniform across routes.
	Syntaxon(ctx context.Context, id, lang string) (SyntaxonDetail, error)
	// SyntaxaByRank lists every syntaxon of rank, ordered by id, narrowed to
	// one life-form group when lifeFormGroup is non-empty (the two filters act
	// as AND). rank is validated against what the index carries: an unknown
	// value is ErrInvalidQuery naming the ranks that would have worked, never
	// an empty list that reads as "there are none".
	//
	// It returns SyntaxonRef and not SyntaxonDetail on purpose: the roots need
	// neither an ancestor path (empty) nor a child list (that is the next
	// step), and 25 details with 150 children each would be an answer nobody
	// asked for.
	SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string) ([]SyntaxonRef, error)
```

- [ ] **Step 4: Die Use-Cases implementieren**

`internal/application/syntaxon_nav.go` anlegen:

```go
package application

// syntaxon_nav.go holds the syntaxa navigation use cases.
//
// Its own file, not query.go: query.go sits at exactly the ratchet's
// per-function complexity cap of 10 and at a file baseline of 72
// (.codecharta-ratchet.json), and this project's practice is to split rather
// than raise a baseline — label.go and description.go were carved out of
// query.go for precisely this reason.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// Syntaxon answers one navigation step: the unit itself, the way up and the way
// down.
//
// The lang parameter is accepted and deliberately not consulted — syntaxa carry
// no German labels yet (design, section 10) — and is therefore named "_" here
// rather than pretending to be read.
func (q *QueryService) Syntaxon(ctx context.Context, id string, _ string) (input.SyntaxonDetail, error) {
	self, err := q.repo.Syntaxon(ctx, id)
	if err != nil {
		return input.SyntaxonDetail{}, translateNotFound(err, fmt.Sprintf("syntaxon %q", id))
	}
	// A dangling parent_id or a cycle gets the same treatment as a dangling
	// habitat-type edge in SyntaxonHabitatTypes: an index defect is reported,
	// not bridged with a shortened breadcrumb trail. The repository's error
	// does not wrap ErrNotFound, so this stays an INTERNAL_ERROR for an id
	// that does exist.
	ancestors, err := q.repo.SyntaxonAncestors(ctx, id)
	if err != nil {
		return input.SyntaxonDetail{}, fmt.Errorf(
			"index is inconsistent: walking the ancestors of syntaxon %q: %w", id, err)
	}
	children, err := q.repo.SyntaxonChildren(ctx, id)
	if err != nil {
		return input.SyntaxonDetail{}, fmt.Errorf("fetching the children of syntaxon %q: %w", id, err)
	}
	count, err := q.repo.HabitatTypeCountForSyntaxon(ctx, id)
	if err != nil {
		return input.SyntaxonDetail{}, fmt.Errorf("counting the habitat types of syntaxon %q: %w", id, err)
	}

	ref := syntaxonRef(self)
	ref.LifeFormGroup = lifeFormGroupOf(self, ancestors)
	return input.SyntaxonDetail{
		SyntaxonRef:            ref,
		Ancestors:              syntaxonRefs(ancestors),
		Children:               syntaxonRefs(children),
		DirectHabitatTypeCount: count,
	}, nil
}

// SyntaxaByRank lists one rank's syntaxa, optionally narrowed to a life-form
// group.
func (q *QueryService) SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string) ([]input.SyntaxonRef, error) {
	if err := q.requireRank(ctx, rank); err != nil {
		return nil, err
	}
	rows, err := q.repo.SyntaxaByRank(ctx, rank, lifeFormGroup)
	if err != nil {
		return nil, fmt.Errorf("fetching syntaxa of rank %q: %w", rank, err)
	}
	return syntaxonRefs(rows), nil
}

// requireRank rejects a rank the index does not carry. The allowed values come
// from the INDEX, not from a wired list: the schema deliberately has no CHECK
// on rank so a further rank can be a data row, and a fixed enum here would have
// taken that freedom back and made a new rank unfindable.
//
// The message names the ranks that would have worked. An empty list would look
// like "there are none" while meaning "you mistyped" — the same rule ?area= and
// ?vocab= already follow.
func (q *QueryService) requireRank(ctx context.Context, rank string) error {
	ranks, err := q.repo.SyntaxonRanks(ctx)
	if err != nil {
		return fmt.Errorf("listing the ranks the index carries: %w", err)
	}
	if slices.Contains(ranks, rank) {
		return nil
	}
	return fmt.Errorf("rank %q: the index carries %s: %w",
		rank, strings.Join(ranks, ", "), input.ErrInvalidQuery)
}

// syntaxonRef maps the domain entity onto the wire view. One mapper for every
// call site — query.go's syntaxaOf included — so a field added to
// domain.Syntaxon cannot reach three answers and be forgotten in the fourth.
func syntaxonRef(s domain.Syntaxon) input.SyntaxonRef {
	return input.SyntaxonRef{
		ID:               s.ID,
		Rank:             s.Rank,
		Name:             s.Name,
		Author:           s.Author,
		ParentID:         s.ParentID,
		EEACode:          s.EEACode,
		Source:           s.Source,
		ParentProvenance: s.ParentProvenance,
		LifeFormGroup:    s.LifeFormGroup,
	}
}

// syntaxonRefs keeps the slice non-nil: ancestors and children are serialized
// WITHOUT omitempty, and a nil slice would put a JSON null where the contract
// promises a list.
func syntaxonRefs(in []domain.Syntaxon) []input.SyntaxonRef {
	out := make([]input.SyntaxonRef, 0, len(in))
	for _, s := range in {
		out = append(out, syntaxonRef(s))
	}
	return out
}

// lifeFormGroupOf reports the group of the formation this syntaxon belongs to.
// The value is stored on formation rows only, so for every other rank it is
// read off the OUTERMOST ancestor — which is the formation, because ancestors
// are outermost-first. Empty stays empty: an unreachable formation is not a
// reason to invent a group.
func lifeFormGroupOf(self domain.Syntaxon, ancestors []domain.Syntaxon) string {
	if self.LifeFormGroup != "" {
		return self.LifeFormGroup
	}
	if len(ancestors) == 0 {
		return ""
	}
	return ancestors[0].LifeFormGroup
}
```

In `internal/application/query.go` `syntaxaOf` auf den geteilten Mapper umstellen — dasselbe Verhalten, eine Zuweisungsliste weniger, und `dupl` bekommt keine zweite fast gleiche Schleife:

```go
func (q *QueryService) syntaxaOf(ctx context.Context, key domain.HabitatTypeKey) ([]input.SyntaxonRef, error) {
	syntaxa, err := q.repo.Syntaxa(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("fetching syntaxa of %s: %w", key, err)
	}
	return syntaxonRefs(syntaxa), nil
}
```

- [ ] **Step 5: Tests laufen lassen und grün sehen**

Run: `go test ./internal/application/ -v`
Expected: PASS für das ganze Paket. Der Aufrufer-Test von `syntaxaOf` (Habitattyp-Detail) muss unverändert grün bleiben — er prüft, dass die Umstellung verhaltensgleich ist.

- [ ] **Step 6: fakeQueryService nachziehen**

Ohne diesen Step kompiliert `internal/adapters/http` nicht mehr: `fakeQueryService` implementiert `input.QueryService`, das gerade zwei Methoden bekommen hat.

In `internal/adapters/http/handlers_test.go`, im Struct:

```go
	// syntaxonDetails backs GET /v1/syntaxon/{id}; syntaxaByRank backs
	// GET /v1/syntaxa, keyed by "rank|life_form_group" so a test can pin the
	// handler's default and its group pass-through separately.
	syntaxonDetails map[string]input.SyntaxonDetail
	syntaxaByRank   map[string][]input.SyntaxonRef
	syntaxaErr      error
	// gotRank/gotLifeFormGroup record the last SyntaxaByRank call; rankCalls
	// counts it, so a test can prove a rejected filter never reached the port.
	gotRank          string
	gotLifeFormGroup string
	rankCalls        int
```

Die beiden Methoden:

```go
func (f *fakeQueryService) Syntaxon(_ context.Context, id, lang string) (input.SyntaxonDetail, error) {
	f.lang = lang
	if f.err != nil {
		return input.SyntaxonDetail{}, f.err
	}
	detail, ok := f.syntaxonDetails[id]
	if !ok {
		return input.SyntaxonDetail{}, fmt.Errorf("syntaxon %q: %w", id, input.ErrNotFound)
	}
	return detail, nil
}

// SyntaxaByRank restates the use case's contract closely enough for the adapter
// to be tested against it: an unknown rank is INVALID_QUERY with the allowed
// values in the message, never an empty list.
func (f *fakeQueryService) SyntaxaByRank(_ context.Context, rank, lifeFormGroup string) ([]input.SyntaxonRef, error) {
	f.rankCalls++
	f.gotRank, f.gotLifeFormGroup = rank, lifeFormGroup
	if f.syntaxaErr != nil {
		return nil, f.syntaxaErr
	}
	refs, ok := f.syntaxaByRank[rank+"|"+lifeFormGroup]
	if !ok {
		return nil, fmt.Errorf("rank %q: the index carries alliance, class, formation, order: %w",
			rank, input.ErrInvalidQuery)
	}
	return refs, nil
}
```

In `seededQueryService()` die beiden Karten ergänzen:

```go
		syntaxonDetails: map[string]input.SyntaxonDetail{
			"C": {
				SyntaxonRef: input.SyntaxonRef{ID: "C", Rank: "formation",
					Name: "Vegetation of the nemoral forest zone", LifeFormGroup: "phanerogam"},
				Ancestors: []input.SyntaxonRef{},
				Children: []input.SyntaxonRef{
					{ID: "CA", Rank: "class", Name: "Testklasse", ParentID: "C"},
				},
			},
			"BRO-01A": {
				SyntaxonRef: input.SyntaxonRef{ID: "BRO-01A", Rank: "alliance",
					Name: "Bromion erecti", Author: "Koch 1926", ParentID: "CA01",
					EEACode: "BRO-01A", Source: "evc", ParentProvenance: "official",
					LifeFormGroup: "phanerogam"},
				Ancestors: []input.SyntaxonRef{
					{ID: "C", Rank: "formation", Name: "Vegetation of the nemoral forest zone"},
					{ID: "CA", Rank: "class", Name: "Testklasse", ParentID: "C"},
					{ID: "CA01", Rank: "order", Name: "Testordnung", ParentID: "CA"},
				},
				Children:               []input.SyntaxonRef{},
				DirectHabitatTypeCount: 1,
			},
		},
		syntaxaByRank: map[string][]input.SyntaxonRef{
			"formation|": {{ID: "C", Rank: "formation",
				Name: "Vegetation of the nemoral forest zone", LifeFormGroup: "phanerogam"}},
			"formation|bryophyte_lichen": {{ID: "R", Rank: "formation",
				Name: "Epigaeic bryophyte and lichen vegetation", LifeFormGroup: "bryophyte_lichen"}},
			"class|": {{ID: "CA", Rank: "class", Name: "Testklasse", ParentID: "C"}},
		},
```

- [ ] **Step 7: Das ganze Modul testen**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/ports/input/services.go internal/application/ internal/adapters/http/handlers_test.go
git commit -m "feat(application): Syntaxon-Detail mit Ahnenpfad und Kindern, Liste je Rang"
```

---

### Task 4: Die zwei Routen, ihre Handler und der OpenAPI-Vertrag

**Files:**
- Create: `internal/adapters/http/syntaxa.go`
- Modify: `internal/adapters/http/server.go`
- Modify: `internal/adapters/http/openapi.yaml`
- Modify: `api/openapi/openapi.yaml`
- Test: `internal/adapters/http/syntaxa_test.go`

**Interfaces:**
- Consumes: `input.QueryService.Syntaxon`, `SyntaxaByRank` (Task 3); `language(r)`, `writeQueryError`, `writeJSON`, `writeError` aus dem Paket.
- Produces: die Routen `GET /v1/syntaxa` und `GET /v1/syntaxon/{id}`; `const defaultSyntaxonRank`; `func validLifeFormGroup(raw string) bool`.

- [ ] **Step 1: Die failing Tests schreiben**

`internal/adapters/http/syntaxa_test.go` (Paket `httpapi_test`; Helfer `newTestServer`, `seededQueryService` aus dem Paket):

```go
package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
	"github.com/jobrunner/situs/internal/ports/input"
)

func getSyntaxonJSON(t *testing.T, path string) (int, map[string]any) {
	t.Helper()
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %s: %v (body %q)", path, err, rec.Body.String())
	}
	return rec.Code, body
}

// The embedded SyntaxonRef fields have to sit FLAT in the same object — that is
// how Go serializes embedded structs, and exactly what the OpenAPI allOf
// describes. A nested object here would have schema and wire claim different
// things.
func TestSyntaxon_EingebetteteFelderStehenFlachImSelbenObjekt(t *testing.T) {
	code, body := getSyntaxonJSON(t, "/v1/syntaxon/BRO-01A")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	for _, key := range []string{
		"id", "rank", "name", "author", "parent_id", "eea_code", "source",
		"parent_provenance", "life_form_group", "ancestors", "children",
		"direct_habitat_type_count",
	} {
		if _, ok := body[key]; !ok {
			t.Errorf("Feld %q fehlt auf der ersten Ebene", key)
		}
	}
	if _, nested := body["SyntaxonRef"]; nested {
		t.Error("SyntaxonRef erscheint als verschachteltes Objekt; es muss promoviert werden")
	}
	if body["id"] != "BRO-01A" {
		t.Errorf("id = %v, erwartet BRO-01A", body["id"])
	}
}

func TestSyntaxon_AhnenpfadIstAeussersteZuerst(t *testing.T) {
	_, body := getSyntaxonJSON(t, "/v1/syntaxon/BRO-01A")
	ancestors, ok := body["ancestors"].([]any)
	if !ok || len(ancestors) != 3 {
		t.Fatalf("ancestors = %v, erwartet drei Einträge", body["ancestors"])
	}
	want := []string{"C", "CA", "CA01"}
	for i, a := range ancestors {
		got := a.(map[string]any)["id"]
		if got != want[i] {
			t.Errorf("ancestors[%d].id = %v, erwartet %q", i, got, want[i])
		}
	}
}

// children and ancestors carry NO omitempty: a missing field and an empty list
// are not the same statement for a client.
func TestSyntaxon_LeereListenBleibenImJSONSichtbar(t *testing.T) {
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxon/BRO-01A", nil))

	if !strings.Contains(rec.Body.String(), `"children":[]`) {
		t.Errorf("Antwort enthaelt kein leeres children-Array: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxon/C", nil))
	if !strings.Contains(rec.Body.String(), `"ancestors":[]`) {
		t.Errorf("Antwort einer Formation enthaelt kein leeres ancestors-Array: %s", rec.Body.String())
	}
}

func TestSyntaxon_FeldheisstDirectHabitatTypeCount(t *testing.T) {
	_, body := getSyntaxonJSON(t, "/v1/syntaxon/BRO-01A")
	if _, ok := body["habitat_type_count"]; ok {
		t.Error("habitat_type_count erscheint; das Feld heisst direct_habitat_type_count")
	}
	if got := body["direct_habitat_type_count"]; got != float64(1) {
		t.Errorf("direct_habitat_type_count = %v, erwartet 1", got)
	}
}

func TestSyntaxon_UnbekannteIDIst404(t *testing.T) {
	code, body := getSyntaxonJSON(t, "/v1/syntaxon/GIBTSNICHT")
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	env := body["error"].(map[string]any)
	if env["code"] != httpapi.CodeNotFound {
		t.Errorf("code = %v, erwartet NOT_FOUND", env["code"])
	}
}

// A path segment is not a query: a blank or whitespace-only {id} is 404, not
// INVALID_QUERY.
func TestSyntaxon_LeerraumPfadsegmentIst404(t *testing.T) {
	code, body := getSyntaxonJSON(t, "/v1/syntaxon/%20")
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	env := body["error"].(map[string]any)
	if env["code"] != httpapi.CodeNotFound {
		t.Errorf("code = %v, erwartet NOT_FOUND", env["code"])
	}
}

// ?lang= is passed through and answers in English until German syntaxa names
// get their own round (design, section 10) — the state of the data, not a
// special rule.
func TestSyntaxon_LangWirdAkzeptiertUndAntwortetEnglisch(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxon/BRO-01A?lang=de", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if q.lang != "de" {
		t.Errorf("durchgereichte Sprache = %q, erwartet de", q.lang)
	}
	if !strings.Contains(rec.Body.String(), "Bromion erecti") {
		t.Error("der englische Name fehlt; er bleibt die Identitaet")
	}
}

// An omitted ?rank= means the formations, not all 1882 rows. The default lives
// in the handler; the port knows no empty rank.
func TestSyntaxa_OhneRankSindDieFormationen(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxa", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if q.gotRank != "formation" {
		t.Errorf("durchgereichter Rang = %q, erwartet formation", q.gotRank)
	}
	var refs []input.SyntaxonRef
	if err := json.Unmarshal(rec.Body.Bytes(), &refs); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(refs) != 1 || refs[0].ID != "C" {
		t.Errorf("Antwort = %+v, erwartet die Formation C", refs)
	}
}

func TestSyntaxa_ReichtDieLebensformGruppeWeiter(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/v1/syntaxa?life_form_group=bryophyte_lichen", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if q.gotLifeFormGroup != "bryophyte_lichen" {
		t.Errorf("durchgereichte Gruppe = %q, erwartet bryophyte_lichen", q.gotLifeFormGroup)
	}
}

func TestSyntaxa_UnbekannterRangIst400MitDenErlaubtenWerten(t *testing.T) {
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxa?rank=association", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	env := body["error"].(map[string]any)
	if env["code"] != httpapi.CodeInvalidQuery {
		t.Errorf("code = %v, erwartet INVALID_QUERY", env["code"])
	}
	msg, _ := env["message"].(string)
	for _, want := range []string{"alliance", "class", "formation", "order"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Nachricht %q nennt den erlaubten Wert %q nicht", msg, want)
		}
	}
}

// The group is the one value set that is wired in (it stands as a CHECK in the
// schema). A typo must not reach the port at all.
func TestSyntaxa_UnbekannteLebensformGruppeIst400UndErreichtDenPortNicht(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxa?life_form_group=pilze", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if q.rankCalls != 0 {
		t.Errorf("SyntaxaByRank wurde %d mal gerufen, erwartet 0", q.rankCalls)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	msg := body["error"].(map[string]any)["message"].(string)
	for _, want := range []string{"phanerogam", "bryophyte_lichen", "algae"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Nachricht %q nennt den erlaubten Wert %q nicht", msg, want)
		}
	}
}

func TestSyntaxa_FehlerDesIndexIst500(t *testing.T) {
	q := seededQueryService()
	q.syntaxaErr = errFakeIndex
	srv := newTestServer(t, q)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxa", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}
```

`errFakeIndex` ist ein Paket-Level-`errors.New("fake index failure")` in `syntaxa_test.go`, falls das Testpaket noch keinen solchen Wert führt; ein bestehender gleichwertiger Wert wird stattdessen benutzt.

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/adapters/http/ -run 'TestSyntaxon_|TestSyntaxa_' -v`
Expected: FAIL — alle Anfragen antworten 404 aus dem Router (die Routen gibt es nicht), `TestSyntaxon_EingebetteteFelderStehenFlachImSelbenObjekt` scheitert beim Dekodieren bzw. an den fehlenden Feldern.

- [ ] **Step 3: Handler und Routen implementieren**

`internal/adapters/http/syntaxa.go` anlegen:

```go
package httpapi

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/jobrunner/situs/internal/domain"
)

// defaultSyntaxonRank is what GET /v1/syntaxa answers without ?rank=: the
// formations, the roots of the hierarchy.
//
// The default lives HERE and not in the port: the port knows no special case
// for an empty rank, and a default that dumped all 1882 rows would be an answer
// nobody asked for. Keeping the parameter checks in the handler is also what
// lets a further filter (subproject C adds ?area= and ?include=) be added
// without rebuilding the route.
const defaultSyntaxonRank = domain.SyntaxonRankFormation

// handleSyntaxa answers GET /v1/syntaxa?rank=&life_form_group= — the entry
// point of the hierarchy, and the only route a client needs to know by heart.
func (s *Server) handleSyntaxa(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rank := strings.TrimSpace(q.Get("rank"))
	if rank == "" {
		rank = defaultSyntaxonRank
	}
	group := strings.TrimSpace(q.Get("life_form_group"))
	if !validLifeFormGroup(group) {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			"life_form_group must be one of phanerogam, bryophyte_lichen, algae")
		return
	}
	// rank is NOT checked here: its allowed values are what the index carries
	// (SELECT DISTINCT rank), which only the use case can ask. It answers
	// ErrInvalidQuery naming them, and writeQueryError turns that into the same
	// 400 as the check above.
	refs, err := s.deps.Query.SyntaxaByRank(r.Context(), rank, group)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, refs)
}

// validLifeFormGroup checks the one filter whose value set is fixed: it stands
// as a CHECK in the schema and is not an extension point — unlike rank, which
// deliberately has no CHECK so a further rank can be a data row.
func validLifeFormGroup(raw string) bool {
	switch raw {
	case "", domain.LifeFormPhanerogam, domain.LifeFormBryophyteLichen, domain.LifeFormAlgae:
		return true
	default:
		return false
	}
}

// handleSyntaxon answers GET /v1/syntaxon/{id}: one syntaxon with its ancestor
// path and its direct children, so a client can walk the hierarchy without
// knowing any id but the one it just clicked.
func (s *Server) handleSyntaxon(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(mux.Vars(r)["id"])
	if id == "" {
		// A path segment is not a query, so a blank one is NOT_FOUND rather
		// than INVALID_QUERY. "/v1/syntaxon/" does not match this route at
		// all; "/v1/syntaxon/%20" does, and has to land in the same place.
		s.writeError(w, http.StatusNotFound, CodeNotFound, "syntaxon not found")
		return
	}
	detail, err := s.deps.Query.Syntaxon(r.Context(), id, language(r))
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, detail)
}
```

In `internal/adapters/http/server.go`, in `setupRoutes`, vor der bestehenden Syntaxon-Zeile:

```go
	r.HandleFunc("/v1/syntaxa", s.handleSyntaxa).Methods(http.MethodGet)
	r.HandleFunc("/v1/syntaxon/{id}", s.handleSyntaxon).Methods(http.MethodGet)
	r.HandleFunc("/v1/syntaxon/{id}/habitat-types", s.handleSyntaxonHabitatTypes).Methods(http.MethodGet)
```

`/v1/syntaxa` ist ein `/v1`-Pfad ohne Pfadparameter — wie `/v1/typologies` und `/v1/areas` — und zwischen `/v1/syntaxon/{id}` und `/v1/syntaxon/{id}/habitat-types` gibt es in mux keine Verdeckung: die Segmentzahl unterscheidet sie.

- [ ] **Step 4: Tests laufen lassen und Fehlschlag sehen (jetzt der Vertragstest)**

Run: `go test ./internal/adapters/http/ -run 'TestSyntaxon_|TestSyntaxa_|TestRoutesMatchOpenAPISpec' -v`
Expected: Die Handler-Tests PASS, `TestRoutesMatchOpenAPISpec` FAIL mit „route \"GET /v1/syntaxa\" is mounted but MISSING from openapi.yaml“ und derselben Meldung für `GET /v1/syntaxon/{id}`.

- [ ] **Step 5: OpenAPI ergänzen**

In `internal/adapters/http/openapi.yaml`, unter `paths:` direkt vor `/v1/syntaxon/{id}/habitat-types`:

```yaml
  /v1/syntaxa:
    get:
      summary: Die Wurzeln der Syntaxa-Hierarchie, filterbar
      description: >-
        Der Einstiegspunkt der Navigation. Ohne Parameter sind es die 25
        Formationen (die Wurzeln), **nicht** alle 1882 Zeilen — ein Vorgabewert,
        der die ganze Tabelle ausgibt, wäre eine Antwort, die niemand
        angefordert hat. `?rank=` liefert stattdessen alle Syntaxa eines Rangs
        (`?rank=alliance` sind 1326 Zeilen, bewusst ohne Paginierung: die Zahl
        ist fest und wächst nur mit einer neuen Quellfassung).
        `?life_form_group=` filtert über die Formation, zu der ein Syntaxon
        gehört; beide Parameter wirken kombiniert als UND. Die Antwort trägt
        `SyntaxonRef` und nicht `SyntaxonDetail`: die Wurzeln brauchen weder
        Ahnenpfad (leer) noch Kinderliste — die holt der nächste Schritt.
      operationId: syntaxa
      parameters:
        - $ref: "#/components/parameters/Rank"
        - $ref: "#/components/parameters/LifeFormGroup"
      responses:
        "200":
          description: Die Syntaxa des angefragten Rangs, nach `id` sortiert.
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/SyntaxonRef"
        "400":
          $ref: "#/components/responses/InvalidQuery"
        "500":
          $ref: "#/components/responses/InternalError"
  /v1/syntaxon/{id}:
    get:
      summary: Ein Syntaxon mit Ahnenpfad und direkten Kindern
      description: >-
        Ein Navigationsschritt in einer Antwort: wo bin ich, und wohin kann
        ich? `ancestors` ist der Weg zur Wurzel, **äußerste zuerst** (Formation,
        Klasse, Ordnung) und damit unverändert als Brotkrumenzeile ausgebbar;
        `children` sind die direkten Kinder, nach `id` sortiert. Beide Felder
        sind **immer** vorhanden, auch leer — ein Verband ohne Kinder und eine
        Formation ohne Ahnen sind der Normalfall an den Rändern der Daten, kein
        Fehler. Es gibt bewusst keine eigene `/children`- oder
        `/ancestors`-Route: sie wären ein zweiter Weg zu denselben Daten.
        `?lang=` wird angenommen; deutsche Syntaxa-Namen sind eine eigene Runde,
        bis dahin antworten die Namen englisch.
      operationId: syntaxon
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
            example: CA01A
        - $ref: "#/components/parameters/Lang"
      responses:
        "200":
          description: Das Syntaxon samt Umgebung.
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/SyntaxonDetail"
        "404":
          $ref: "#/components/responses/NotFound"
        "500":
          $ref: "#/components/responses/InternalError"
```

Unter `components: parameters:` nach `OnlyInArea`:

```yaml
    Rank:
      name: rank
      in: query
      required: false
      description: >-
        Rang der gelisteten Syntaxa. Weggelassen sind es die Formationen, nicht
        alle Zeilen. Die erlaubten Werte liest der Dienst aus dem Index
        (`SELECT DISTINCT rank`) statt sie zu verdrahten: das Schema hat bewusst
        kein CHECK auf `rank`, damit ein weiterer Rang eine Datenzeile sein
        kann. Ein unbekannter Wert ist INVALID_QUERY und die Nachricht nennt
        die Werte, die dieser Index führt — niemals eine leere Liste, die wie
        „gibt es nicht“ aussieht.
      schema:
        type: string
        example: alliance
    LifeFormGroup:
      name: life_form_group
      in: query
      required: false
      description: >-
        Lebensform-Gruppe der Formation, zu der ein Syntaxon gehört. Gespeichert
        ist der Wert nur auf Formationszeilen; für jeden anderen Rang filtert
        der Dienst über den Weg zur Formation. Die Wertemenge ist fest (sie
        steht als CHECK im Schema) und keine Erweiterungsstelle; ein unbekannter
        Wert ist INVALID_QUERY.
      schema:
        type: string
        enum: [phanerogam, bryophyte_lichen, algae]
```

Unter `components: schemas:` direkt nach `SyntaxonRef`:

```yaml
    SyntaxonDetail:
      description: >-
        Ein Syntaxon mit seiner Umgebung: der Weg nach oben und die direkten
        Kinder. `allOf`, weil die Go-Struktur `SyntaxonRef` einbettet und dessen
        Felder damit in dasselbe JSON-Objekt einwandern — ein verschachteltes
        Objekt hier und ein flaches auf der Leitung wären zwei Verträge, die
        sich als einer ausgeben. `life_form_group` steht nur in `SyntaxonRef`
        und wird hier nicht wiederholt; für jeden Rang außer der Formation ist
        der Wert aus der Formation des Ahnenpfads abgeleitet und deshalb
        nachvollziehbar, nicht behauptet.
      allOf:
        - $ref: "#/components/schemas/SyntaxonRef"
        - type: object
          required: [ancestors, children, direct_habitat_type_count]
          properties:
            ancestors:
              type: array
              description: >-
                Der Weg zur Wurzel, ÄUSSERSTE zuerst (Formation, dann Klasse,
                dann Ordnung). Für eine Formation leer — leer und vorhanden,
                nicht fehlend.
              items:
                $ref: "#/components/schemas/SyntaxonRef"
            children:
              type: array
              description: >-
                Die direkten Kinder, nach `id` sortiert. Für einen Verband leer;
                das ist die untere Grenze der freien Daten (Assoziationen sind
                pan-europäisch nicht verfügbar), kein Fehler. Auch hier ist die
                leere Liste vorhanden und nicht weggelassen.
              items:
                $ref: "#/components/schemas/SyntaxonRef"
            direct_habitat_type_count:
              type: integer
              description: >-
                Die Zahl der Habitattypen, die GENAU dieses Syntaxon verlinken —
                nicht die seiner Nachkommen. Für eine Klasse oder Formation
                praktisch immer 0, weil `habitat_type_syntaxon` Verbände
                verlinkt (und in einem Fall eine Ordnung). Der Name sagt das,
                damit die 0 nicht als „diese Klasse berührt keinen EUNIS-Typ“
                gelesen wird.
              example: 3
```

Dann die Byte-Gleichheit herstellen:

```bash
cp internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
diff internal/adapters/http/openapi.yaml api/openapi/openapi.yaml && echo "identisch"
```

- [ ] **Step 6: Vertragstests laufen lassen und grün sehen**

Run: `go test ./internal/adapters/http/ -run 'TestRoutesMatchOpenAPISpec|TestOpenAPICopiesAreIdentical|TestOpenAPIServerURLIsRelative' -v`
Expected: PASS. Meldet der erste Test „openapi.yaml documents … but no such route is mounted“, ist ein Pfadschlüssel falsch eingerückt (Pfade zwei, Methoden vier Leerzeichen).

- [ ] **Step 7: Das ganze Modul testen**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/http/ api/openapi/openapi.yaml
git commit -m "feat(http): Routen /v1/syntaxa und /v1/syntaxon/{id} samt OpenAPI-Vertrag"
```

---

### Task 5: Explorer-Panel „Syntaxa-Navigation“ — gerendert neben dem Roh-JSON

**Files:**
- Modify: `internal/adapters/http/explorer.html`
- Test: `internal/adapters/http/explorer_test.go`

**Der Mechanismus, ausdrücklich festgelegt.** `call()` überschreibt `out.textContent` **bedingungslos** und löscht `out.dataset.raw` vor jeder Anfrage; `toggleFormat()` rendert **ausschließlich** `out.dataset.raw` neu. Ein gerendertes Panel passt deshalb nicht in das `<pre>`: der nächste Aufruf oder der Format-Umschalter würde die Klickelemente wegwerfen. Also:

1. Das Panel behält `<div class="url">` und `<pre>` wie jedes andere — das Roh-JSON bleibt unverändert einsehbar, samt Format-Umschalter.
2. Die gerenderte Ansicht bekommt ein **zweites Zielelement**, `<div id="syn-nav-view">`, das nur `renderSyntaxon()` bzw. `renderFormations()` beschreibt.
3. `call()` gibt den geparsten Körper **zurück** (`return body;`). Das ist die einzige Änderung an der geteilten Funktion; jedes bestehende Panel ignoriert den Rückgabewert und verhält sich unverändert.

**Interfaces:**
- Consumes: `GET /v1/syntaxa`, `GET /v1/syntaxon/{id}` (Task 4); die bestehenden Helfer `call`, `withQuery`, `toggleFormat`, `runSynHabitat`.
- Produces: Panel `#syn-nav`, Zielelement `#syn-nav-view`, Funktionen `loadFormations()`, `openSyntaxon(id)`, `renderSyntaxon(s)`, `childList(items, caption)`, `syntaxonButton(s)`; `call()` liefert den geparsten Körper zurück.

- [ ] **Step 1: Die failing Tests schreiben**

In `internal/adapters/http/explorer_test.go` — im bestehenden Stil, auf Zeichenketten-Ebene (das Paket führt keine JS-Laufzeit; der Test prüft, dass die Seite die richtigen Aufrufe und Zielelemente überhaupt enthält):

```go
// Demonstrating the hierarchy without a click path would be the same problem
// as the API without a navigation route. The panel therefore starts at
// /v1/syntaxa and continues through /v1/syntaxon/{id}.
func TestExplorerPage_HatDasSyntaxaNavigationsPanel(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`id="syn-nav"`,
		`withQuery("/v1/syntaxa"`,
		"/v1/syntaxon/${encodeURIComponent(id)}",
		`id="syn-nav-group"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Seite enthaelt %q nicht; das Navigationspanel ist unvollstaendig", want)
		}
	}
}

// The rendered panel must not live in the <pre>: call() overwrites its
// textContent unconditionally and toggleFormat() re-renders dataset.raw alone.
// It needs a second target element — and the raw JSON stays next to it.
func TestExplorerPage_RendertDieNavigationNebenDemRohJSON(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`id="syn-nav-view"`,
		"function renderSyntaxon(",
		`toggleFormat('syn-nav')`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Seite enthaelt %q nicht", want)
		}
	}
	// The rendered panel needs the parsed body, so call() has to return it —
	// otherwise the panel could only render by hijacking the <pre>.
	if !strings.Contains(body, "return body;") {
		t.Error("call() gibt den geparsten Koerper nicht zurueck; das gerenderte Panel muesste sonst selbst fetchen")
	}
}

// The habitat-types button appears only when the response reports edges — and
// it reads the field under its correct name.
func TestExplorerPage_VerlinktHabitattypenNurBeiVorhandenenKanten(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rec.Body.String()
	if !strings.Contains(body, "s.direct_habitat_type_count > 0") {
		t.Error("die Seite prueft direct_habitat_type_count nicht, bevor sie den Habitattyp-Knopf zeigt")
	}
	if strings.Contains(body, "habitat_type_count >") && !strings.Contains(body, "direct_habitat_type_count >") {
		t.Error("die Seite liest habitat_type_count; das Feld heisst direct_habitat_type_count")
	}
}
```

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/adapters/http/ -run TestExplorerPage -v`
Expected: FAIL — `Seite enthaelt "id=\"syn-nav\"" nicht` und die weiteren fehlenden Zeichenketten. `TestExplorerPage_LoadsNoRemoteAssets` bleibt grün.

- [ ] **Step 3: Das Panel in die Seite schreiben**

In `internal/adapters/http/explorer.html`, im `<style>`-Block nach `.note`:

```css
  .crumbs { margin:0 0 .5rem; font-size:.9rem; }
  .crumbs button.hit { display:inline; width:auto; padding:0 .15rem; color:var(--accent); }
  #syn-nav-view { border-top:1px solid var(--line); margin-top:.75rem; padding-top:.75rem; }
```

Als neues Panel direkt **vor** `<section class="panel" id="syn-habitat">` (die Navigation kommt vor dem Ziel, zu dem sie führt):

```html
<section class="panel" id="syn-nav">
  <h2>Syntaxa-Navigation — GET /v1/syntaxa und GET /v1/syntaxon/{id}</h2>
  <div class="row">
    <div>
      <label for="syn-nav-group">life_form_group</label>
      <select id="syn-nav-group">
        <option value="">—</option>
        <option value="phanerogam">phanerogam</option>
        <option value="bryophyte_lichen">bryophyte_lichen</option>
        <option value="algae">algae</option>
      </select>
    </div>
    <button onclick="loadFormations()">Formationen laden</button>
    <button class="secondary" onclick="toggleFormat('syn-nav')">formatiert ⇄ JSON</button>
  </div>
  <p class="note">
    Ein Klick lädt das jeweilige Syntaxon und zeigt Brotkrumenzeile, aktuelle
    Einheit und Kinder. Die gerenderte Ansicht steht neben dem Roh-JSON, nicht
    an seiner Stelle — beides bleibt einsehbar.
  </p>
  <div class="url"></div>
  <div id="syn-nav-view"></div>
  <pre></pre>
</section>
```

Im `<script>`-Block `call()` um die Rückgabe erweitern (die einzige Änderung an der geteilten Funktion):

```js
    const res = await fetch(url, init);
    const body = await res.json();
    out.dataset.raw = JSON.stringify(body, null, 2);
    out.textContent = out.dataset.raw;
    if (!res.ok) { box.textContent += `  → HTTP ${res.status}`; }
    // Returned so a panel can additionally render the answer without fetching
    // it a second time. Every other panel ignores the value.
    return body;
```

Und am Ende des Skripts, vor den `loadTypologies()`/`loadAreas()`-Aufrufen:

```js
// The navigation panel is the first one with a RENDERED view. It needs a target
// element of its own: call() overwrites the <pre>'s textContent unconditionally
// and toggleFormat() re-renders out.dataset.raw alone — a click path inside the
// <pre> would be gone on the next request or format switch. The raw JSON
// therefore stays where every other panel keeps it, and the rendered view sits
// next to it in #syn-nav-view.
async function loadFormations() {
  const params = new URLSearchParams();
  const group = document.getElementById("syn-nav-group").value;
  if (group) { params.set("life_form_group", group); }
  const body = await call("syn-nav", withQuery("/v1/syntaxa", params));
  const view = document.getElementById("syn-nav-view");
  view.innerHTML = "";
  if (!Array.isArray(body)) { return; }
  view.appendChild(childList(body, "Formationen"));
}

// One navigation step. Nothing is computed that the response does not carry
// itself: ancestors arrives outermost-first and is printed in that order.
async function openSyntaxon(id) {
  const params = new URLSearchParams();
  const lang = document.getElementById("g-lang").value;
  if (lang) { params.set("lang", lang); }
  const body = await call("syn-nav", withQuery(`/v1/syntaxon/${encodeURIComponent(id)}`, params));
  renderSyntaxon(body);
}

function renderSyntaxon(s) {
  const view = document.getElementById("syn-nav-view");
  view.innerHTML = "";
  if (!s || !s.id) { return; }

  const crumbs = document.createElement("div");
  crumbs.className = "crumbs";
  for (const a of s.ancestors) {
    crumbs.appendChild(syntaxonButton(a));
    crumbs.appendChild(document.createTextNode(" › "));
  }
  const here = document.createElement("strong");
  here.textContent = `${s.id} — ${s.name}${s.author ? " " + s.author : ""} (${s.rank})`;
  crumbs.appendChild(here);
  view.appendChild(crumbs);

  if (s.life_form_group) {
    const g = document.createElement("p");
    g.className = "note";
    g.textContent = "Lebensform-Gruppe: " + s.life_form_group;
    view.appendChild(g);
  }
  if (s.direct_habitat_type_count > 0) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.textContent = `${s.direct_habitat_type_count} verknüpfte Habitattyp(en) abrufen`;
    // Fills the existing panel and fires its request, instead of presenting
    // the same answer a second time here.
    btn.onclick = () => {
      document.getElementById("syn-habitat-id").value = s.id;
      runSynHabitat();
    };
    view.appendChild(btn);
  }
  view.appendChild(childList(s.children, "Kinder"));
}

// The children (or the formations) as a clickable list. An empty list is SHOWN
// as empty rather than hidden: an alliance without children is the lower bound
// of the data, and a panel that renders nothing there looks broken.
function childList(items, caption) {
  const wrap = document.createElement("div");
  const head = document.createElement("p");
  head.className = "note";
  head.textContent = items.length ? `${caption} (${items.length})` : `${caption}: keine`;
  wrap.appendChild(head);
  const ul = document.createElement("ul");
  ul.className = "hits";
  for (const c of items) {
    const li = document.createElement("li");
    li.appendChild(syntaxonButton(c));
    ul.appendChild(li);
  }
  wrap.appendChild(ul);
  return wrap;
}

function syntaxonButton(s) {
  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = "hit";
  btn.textContent = `${s.id} — ${s.name}`;
  btn.onclick = () => openSyntaxon(s.id);
  return btn;
}
```

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./internal/adapters/http/ -run TestExplorer -v`
Expected: PASS, sieben Tests (die vier bestehenden und die drei neuen). Insbesondere `TestExplorerPage_LoadsNoRemoteAssets` bleibt grün — das Panel lädt nichts nach.

- [ ] **Step 5: Die Seite einmal von Hand bedienen**

Run:
```bash
go run ./cmd/situs serve --db situs.sqlite --addr :8080 &
sleep 2 && curl -s localhost:8080/v1/syntaxa | head -c 400 && echo
curl -s localhost:8080/v1/syntaxon/C | head -c 400 && echo
kill %1
```
Expected: die erste Antwort ist ein Array von Formationen (25 Einträge), die zweite ein Objekt mit `ancestors: []`, gefüllten `children` und `direct_habitat_type_count`. Danach die Seite unter `http://localhost:8080/` öffnen, eine Formation anklicken und bis zu einem Verband durchklicken: die Brotkrumenzeile muss bei jedem Schritt wachsen und jeder Krumen anklickbar zurückführen.

Ein Index, der noch aus einer Fassung vor Teilprojekt A stammt, lässt `serve` schon beim Start mit einer benannten fehlenden Spalte scheitern — dann zuerst neu ingestieren, nicht den Test anpassen.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/http/
git commit -m "feat(explorer): Panel Syntaxa-Navigation mit Brotkrumen und Kinderliste"
```

---

### Task 6: Gegen den echten Index messen, Gates fahren, dokumentieren

**Files:**
- Test: `internal/adapters/sqlite/read_syntaxon_test.go` (ein weiterer Integritätstest)
- Modify: `docs/reference/http-api.md`
- Modify: `docs/reference/measured-index.md`
- Modify: `CLAUDE.md`
- Modify: `.codecharta-ratchet.json` (nur, wenn eine Baseline gemessen sinkt)

**Interfaces:**
- Consumes: alles aus Task 1–5.
- Produces: die geprüfte Zusage „von jeder Zeile führt der Ahnenpfad in höchstens drei Schritten zu einer Formation“ an der Oberfläche; aktualisierte Referenzdokumentation.

- [ ] **Step 1: Den Integritätstest über die Leseoperationen schreiben**

In `internal/adapters/sqlite/read_syntaxon_test.go`:

```go
// The design's central promise, checked at the read rather than at the table:
// every row reaches a formation through SyntaxonAncestors, and the path is
// outermost-first. Subproject A checks the same thing at ingest time; here it
// is checked where a user experiences it.
func TestJedeZeileErreichtUeberDenAhnenpfadEineFormation(t *testing.T) {
	db := openHierarchyDB(t)

	all, err := db.AllSyntaxa(t.Context())
	if err != nil {
		t.Fatalf("AllSyntaxa: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("die Fixture ist leer; der Test wuerde nichts pruefen")
	}
	for _, s := range all {
		ancestors, err := db.SyntaxonAncestors(t.Context(), s.ID)
		if err != nil {
			t.Fatalf("SyntaxonAncestors(%s): %v", s.ID, err)
		}
		if s.Rank == domain.SyntaxonRankFormation {
			if len(ancestors) != 0 {
				t.Errorf("%s ist eine Formation, hat aber Ahnen %v", s.ID, syntaxonIDs(ancestors))
			}
			continue
		}
		if len(ancestors) == 0 || ancestors[0].Rank != domain.SyntaxonRankFormation {
			t.Errorf("%s: Ahnenpfad %v beginnt nicht bei einer Formation", s.ID, syntaxonIDs(ancestors))
		}
		if len(ancestors) > maxSyntaxonAncestors {
			t.Errorf("%s: Ahnenpfad hat %d Schritte, erlaubt sind %d",
				s.ID, len(ancestors), maxSyntaxonAncestors)
		}
	}
}
```

- [ ] **Step 2: Test laufen lassen und grün sehen**

Run: `go test ./internal/adapters/sqlite/ -run TestJedeZeileErreicht -v`
Expected: PASS. Schlägt er fehl, liegt ein echter Defekt in `SyntaxonAncestors` oder in der Fixture vor — melden, nicht den Test aufweichen.

- [ ] **Step 3: Gegen den echten Index messen**

Run:
```bash
go run ./cmd/situs serve --db situs.sqlite --addr :8080 &
sleep 2
python3 - <<'PY'
import json, urllib.request
def get(path):
    with urllib.request.urlopen("http://localhost:8080" + path) as r:
        return json.load(r)

roots = get("/v1/syntaxa")
print("Formationen:", len(roots))

# Pure following of children, without knowing a single id up front.
seen, alliances, queue = set(), 0, [r["id"] for r in roots]
while queue:
    sid = queue.pop()
    if sid in seen:
        continue
    seen.add(sid)
    d = get("/v1/syntaxon/" + sid)
    assert d["ancestors"] == [] or d["ancestors"][0]["rank"] == "formation", sid
    assert len(d["ancestors"]) <= 3, (sid, len(d["ancestors"]))
    if d["rank"] == "alliance":
        alliances += 1
        assert len(d["ancestors"]) == 3, (sid, len(d["ancestors"]))
    queue.extend(c["id"] for c in d["children"])
print("erreichte Zeilen:", len(seen), "davon Verbaende:", alliances)

print("Verbaende gesamt:", len(get("/v1/syntaxa?rank=alliance")))
print("Moos-/Flechtenverbaende:", len(get("/v1/syntaxa?rank=alliance&life_form_group=bryophyte_lichen")))
print("Klassen:", len(get("/v1/syntaxa?rank=class")))
print("Ordnungen:", len(get("/v1/syntaxa?rank=order")))
PY
kill %1
```
Expected: `Formationen: 25`; `erreichte Zeilen: 1882`, `davon Verbaende: 1326`; `Verbaende gesamt: 1326`; `Moos-/Flechtenverbaende: 137`; `Klassen: 150`; `Ordnungen: 381`. Keine Assertion schlägt an — jeder Verband erreicht seine Formation in genau drei Schritten.

Weicht eine Zahl ab: **anhalten und melden**. Die Erwartungen sind die gemessenen Werte aus Teilprojekt A, keine Schätzungen.

- [ ] **Step 4: Die Fehlerfälle gegen den echten Dienst prüfen**

Run:
```bash
go run ./cmd/situs serve --db situs.sqlite --addr :8080 &
sleep 2
for q in "?rank=association" "?life_form_group=pilze"; do
  echo "GET /v1/syntaxa$q"; curl -s -o /dev/null -w "%{http_code} " "localhost:8080/v1/syntaxa$q"
  curl -s "localhost:8080/v1/syntaxa$q"
done
echo; curl -s -o /dev/null -w "%{http_code}\n" localhost:8080/v1/syntaxon/GIBTSNICHT
curl -s -o /dev/null -w "%{http_code}\n" "localhost:8080/v1/syntaxon/%20"
kill %1
```
Expected: beide Filterfehler sind `400` mit `INVALID_QUERY` und nennen die erlaubten Werte (die Rangliste kommt aus dem Index: `alliance, class, formation, order`); die unbekannte ID und das Leerraumsegment sind je `404`.

- [ ] **Step 5: Dokumentation nachziehen**

In `docs/reference/http-api.md`:
- Die Routentabelle um zwei Zeilen ergänzen, direkt vor `GET /v1/syntaxon/{id}/habitat-types`:
  ```
  | `GET /v1/syntaxa?rank=&life_form_group=` | Einstieg in die Syntaxa-Hierarchie: ohne Parameter die 25 Formationen |
  | `GET /v1/syntaxon/{id}` | ein Syntaxon mit Ahnenpfad und direkten Kindern |
  ```
- Einen Abschnitt `## Syntaxa-Navigation: GET /v1/syntaxa und GET /v1/syntaxon/{id}` aufnehmen, der festhält: der Vorgabewert von `?rank=` ist `formation` (nicht „alle Zeilen“); die erlaubten Rang-Werte liest der Dienst aus dem Index, die von `life_form_group` stehen fest; `ancestors` ist äußerste-zuerst; `ancestors` und `children` sind immer vorhanden, auch leer; `direct_habitat_type_count` zählt nur die eigenen Kanten; eine baumelnde `parent_id` ist `INTERNAL_ERROR` und kein `404`; deutsche Syntaxa-Namen fehlen noch und `?lang=` antwortet bis dahin englisch.
- Im Abschnitt „Bekannte Grenze: Leseverhalten“ ergänzen, dass `GET /v1/syntaxon/{id}` bis zu vier Primärschlüsselabfragen für den Ahnenpfad braucht (eine je Ebene) und Kinder über `idx_syntaxon_parent` laufen — kein Full-Table-Scan.

In `docs/reference/measured-index.md` die in Step 3 gemessenen Zahlen mit ihrer Herkunft (der Abfrage bzw. der Route) aufnehmen: 25 Formationen, 150 Klassen, 381 Ordnungen, 1326 Verbände, 1882 über `children` erreichbare Zeilen, 137 Moos- und Flechtenverbände.

In `CLAUDE.md`:
- Im Abschnitt „Current State“ festhalten: die Syntaxa-Navigation ist umgesetzt, `GET /v1/syntaxa` und `GET /v1/syntaxon/{id}` sind die zwei neuen Routen, von den Formationen aus ist jeder Verband durch reines Verfolgen von `children` erreichbar, und der Explorer hat sein erstes gerendertes Panel.
- In der Tabelle der Design-Dokumente `docs/superpowers/specs/2026-09-21-syntaxa-navigation-design.md` und diesen Plan eintragen.
- Bei „Invariants that reviewers must check“ die neue Zusage aufnehmen: *`children` und `ancestors` sind immer im JSON, auch leer — nie `omitempty`, nie `nil`*, gleichrangig neben der Dreiwertigkeit von `in_area`.
- Im Abschnitt „Architecture“ die neue Datei `internal/application/syntaxon_nav.go` und `internal/adapters/http/syntaxa.go` sichtbar machen, falls die Paketliste Dateien nennt.

- [ ] **Step 6: Alle drei Gates fahren**

Run: `make verify && make mutation && make codecharta`
Expected: alle drei grün.

Zu erwartende Reibungspunkte, jeweils mit der vorgesehenen Reaktion:
- **Coverage.** `internal/application` steht auf 100 %: jeder Fehlerpfad in `syntaxon_nav.go` hat in Task 3 einen Test (`syntaxonChildrenErr`, `habitatTypeCountErr`, `syntaxonRanksErr`, `syntaxaByRankErr`, die baumelnde Referenz und die unbekannte ID). Fehlt einer, wird er ergänzt — der Floor wird **nicht** gesenkt. `internal/adapters/http` steht auf 88 %: die neuen Handler sind über die Tests aus Task 4 vollständig abgedeckt, inklusive des 500-Pfads.
- **Komplexitäts-Ratchet.** `query.go` wird durch die Umstellung von `syntaxaOf` auf `syntaxonRefs` **kleiner**. Den gemessenen neuen Wert in `.codecharta-ratchet.json` einsetzen statt 72 stehen zu lassen — Baselines senken ist Projektpraxis (`_query_note`). `syntaxon_nav.go` und `syntaxa.go` sind neue Dateien ohne Baseline und müssen unter `function_complexity.default_cap = 10` und `complexity.default_cap = 50` bleiben; `Syntaxon` hat fünf Fehlerzweige und liegt deutlich darunter. Keine Baseline wird **heraufgesetzt**.
- **Mutation.** `make mutation` läuft ein Paket je Aufruf über `scripts/mutation-gate.sh`; gremlins **niemals** mit `...` aufrufen (erzeugt still null Mutanten). Dieses Teilprojekt legt **kein neues Go-Paket** an — `internal/application`, `internal/adapters/sqlite`, `internal/adapters/http`, `internal/ports/input` und `internal/ports/output` sind alle schon in `.mutation-thresholds` geführt, die Datei bleibt also unverändert, solange die Schwellen halten. Überlebt ein Mutant, fehlt eine Zusicherung — der Test wird ergänzt, der Schwellwert nicht gesenkt. Steigt die Punktzahl, wird er angehoben (Raise-Only).
- **Hotspot.** Die Allowlist ist leer und bleibt leer. Keine der neuen Dateien kommt über `min_complexity: 40`.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/sqlite/read_syntaxon_test.go docs/ CLAUDE.md .codecharta-ratchet.json
git commit -m "test,docs: Navigationszusagen absichern und gemessene Werte festhalten"
```

---

## Self-Review

**Spec-Abdeckung, Abschnitt für Abschnitt.**

- **Abschnitt 1 (zwei Routen genügen).** Beide Routen in Task 4, Step 3, mit `.Methods("GET")`; die bestehende `/habitat-types`-Route bleibt unverändert. Keine `/children`- und keine `/ancestors`-Route — die Kinder- und Ahnenliste reisen in der Antwort mit (Task 3, Step 3). Die Kette selbst ist in Task 6, Step 3 durch reines Verfolgen von `children` nachgeprüft.
- **Abschnitt 2 (Antwortstrukturen).** `SyntaxonDetail` in Task 3, Step 3, mit eingebettetem `SyntaxonRef`, `Ancestors`/`Children` ohne `omitempty` und `DirectHabitatTypeCount` unter genau diesem Namen. Die Flachheit des JSON ist in Task 4, Step 1 festgenagelt (`TestSyntaxon_EingebetteteFelderStehenFlachImSelbenObjekt` prüft zwölf Schlüssel auf der ersten Ebene und die Abwesenheit eines verschachtelten `SyntaxonRef`), das OpenAPI-Gegenstück ist das `allOf` in Task 4, Step 5. `life_form_group` steht nur in `SyntaxonRef` und wird für Nicht-Formationen aus dem Ahnenpfad abgeleitet (`lifeFormGroupOf`, getestet in Task 3, Step 1). `GET /v1/syntaxa` liefert `[]SyntaxonRef`, nicht `SyntaxonDetail` — im Port-Kommentar begründet und in Task 4, Step 1 geprüft.
- **Abschnitt 3 (Filter).** `?rank=` und `?life_form_group=` in Task 4, Step 3; UND-Kombination in Task 3, Step 1 (`TestSyntaxaByRank_KombiniertRangUndGruppeAlsUnd`) und in Task 2, Step 1 auf SQL-Ebene. Vorgabewert `formation` **im Handler** (`defaultSyntaxonRank`), geprüft über `q.gotRank`. Erlaubte Rang-Werte aus dem Index (`SyntaxonRanks`, Task 2), erlaubte Gruppen fest verdrahtet (`validLifeFormGroup`). Der rekursive CTE aus Abschnitt 3 in Task 2, Step 3 — **ein** statisches Statement, kein Statement je Rang, keine aus `rank` gebaute Zeichenkette. Unbekannte Werte sind `INVALID_QUERY` mit den erlaubten Werten in der Nachricht (Tests in Task 3 und 4), niemals eine leere Liste.
- **Abschnitt 4 (Port und Repository).** Die vier Repository-Methoden aus dem Spec in Task 1 und 2, mit den dort genannten Signaturen. `maxSyntaxonAncestors = 3` als Schrittzahl, mit Kommentar zur Verwechslung mit der Knotenzahl 4. `AllSyntaxa` bleibt (Task 2, Step 5). **Eine Ergänzung gegenüber der Aufzählung des Specs:** `SyntaxonRanks` — Abschnitt 3 verlangt die Rangliste aus dem Index, Abschnitt 4 listet die dafür nötige Methode nicht auf. Sie ist hinzugefügt, nicht ersetzt. Beide Testdoubles werden mitgezogen: `fakeRepo` in Task 1, Step 10 und Task 2, Step 6, `fakeQueryService` in Task 3, Step 6.
- **Abschnitt 5 (Fehlerbehandlung).** Alle sechs Zeilen der Tabelle haben einen Test: unbekannte ID → 404 (Task 3 und 4), Leerraum-Pfadsegment → 404 ohne `INVALID_QUERY` (Task 4), unbekannter Filterwert → 400 mit erlaubten Werten (Task 3 und 4, plus curl-Prüfung in Task 6), weggelassenes `?rank=` → Formationen (Task 4), baumelnde `parent_id` → „index is inconsistent“ und **kein** 404 (Task 1, Step 5 auf Adapterebene, Task 3, Step 1 auf Use-Case-Ebene, jeweils mit `errors.Is`-Gegenprobe), Zykel → Fehler mit auslösender ID (Task 1). `children: []` und `direct_habitat_type_count: 0` sind als Normalfall getestet, nicht als Fehler.
- **Abschnitt 6 (Explorer).** Task 5, mit dem im Spec verlangten Ablauf (Start bei den Formationen, Klick lädt das Detail, drei Blöcke: Brotkrumen, aktuelle Einheit mit Rang und Autorschaft, Kinder), dem Auswahlfeld für `life_form_group`, dem Habitattyp-Knopf bei Kanten > 0 und dem bestehenden Format-Umschalter. Der Mechanismus ist konkret festgelegt statt angedeutet: zweites Zielelement `#syn-nav-view`, eigene `renderSyntaxon()`, und `call()` gibt den Körper zurück, damit das Roh-JSON im `<pre>` unberührt bleibt. Drei Tests im bestehenden Zeichenketten-Stil; der Remote-Asset-Test gilt unverändert weiter.
- **Abschnitt 7 (OpenAPI).** Task 4, Step 5: beide Pfade, `SyntaxonDetail` als `allOf`, `Rank` und `LifeFormGroup` als benannte `components/parameters`, `cp` für die Byte-Gleichheit, und die drei Vertragstests in Step 6. `docs/reference/http-api.md` in Task 6, Step 5.
- **Abschnitt 8 (Tests).** Adapterebene in Task 1 und 2 (Kinder sortiert, Ahnenpfad äußerste-zuerst, Formation leer, Zykel, `SyntaxaByRank` mit und ohne Gruppenfilter, Kantenzählung ohne Laden); Use-Case-Ebene in Task 3 (Formation, Klasse, Ordnung, Verband, unbekannte ID, baumelnde `parent_id`); HTTP-Ebene in Task 4 (beide Routen im Vertragstest, unbekannter Filterwert samt erlaubter Werte, `children: []` sichtbar im JSON); die Zusage „drei Schritte bis zur Formation“ in Task 6, Step 1 als Test und in Step 3 gegen den gebauten Index.
- **Abschnitt 9 (prüfbare Zusagen).** Alle fünf in Task 6, Step 3 und 4 gemessen bzw. in Task 1 und 6 als Test: Erreichbarkeit jedes Verbands ohne ID-Kenntnis, Ahnenpfad endet bei einer Formation für jede ID, `children`/`ancestors` immer vorhanden, kein stillschweigend ignorierter Filterwert, Indexnutzung ohne Full-Table-Scan (`idx_syntaxon_parent` für Kinder, Primärschlüssel für Ahnen).
- **Abschnitt 10 (bewusst außerhalb).** Nichts davon ist im Plan: keine deutschen Syntaxa-Namen (das `lang`-Argument wird angenommen und ausdrücklich nicht gelesen, mit Test auf den dokumentierten Zustand), keine Syntaxon-Namenssuche, keine Habitattyp→Formation-Aggregation, kein Verbreitungsfilter. Die Parameterprüfung liegt im Handler, damit Teilprojekt C `?area=` und `?include=` ergänzen kann, ohne die Route umzubauen.

**Zwei bewusste Abweichungen, beide verstärkend, keine abschwächend.**

1. **Der CTE bekommt eine Schrittgrenze.** Das Spec schreibt `WHERE s.parent_id <> ''` als einzige Abbruchbedingung. Bei einem Zykel im `parent_id`-Graphen — genau dem Fall, den Abschnitt 4 als Indexdefekt benennt — rekursiert dieses Statement unbegrenzt: kein Fehler, sondern ein hängender Prozess. `AND u.steps < ?` mit `maxSyntaxonAncestors` behebt das, bleibt **ein** statisches Literal, verändert das Ergebnis für jeden azyklischen Index nicht (die gemessene Maximaltiefe ist 3) und hat in Task 2, Step 1 einen eigenen Test. Ohne die Grenze wäre die Zykel-Behandlung im Ahnenpfad gehärtet und im Filter nicht.
2. **`SyntaxonRanks` ist eine fünfte Port-Methode.** Abschnitt 3 verlangt die Rangliste aus dem Index, Abschnitt 4 listet sie nicht. Sie ist zusätzlich, nicht anstelle einer der vier.

**Sprachtrennung.** Die Prosa des Plans ist deutsch, jeder Kommentar und Doc-Kommentar **innerhalb** eines Codeblocks ist englisch — Produktionscode, Testcode, das JS in `explorer.html` (wo der Bestand ebenfalls englisch ist) und auch der Kommentar im Messskript aus Task 6. Geprüft wurde durch einen Durchlauf über alle Zeilen der Codeblöcke, die mit `//` oder `#` beginnen. Teilprojekt A führt in seinen Codeblöcken deutsche Kommentare; das wäre ein Verstoß gegen CLAUDE.md („Code comments sparse, English“) und gegen den gesamten Bestand des Repos und ist hier bewusst nicht übernommen.

**Platzhalter-Scan.** Kein „TBD“, kein „TODO“, kein „angemessene Fehlerbehandlung“, kein „analog zu Task N“. Jeder Code-Step trägt den Code, den er meint, inklusive der Testfixtures; jeder Fehlerfall ist benannt, mit Meldung und Test. Die einzigen Stellen, die eine Entscheidung am Rechner verlangen, sind die gemessenen Werte in Task 6 (`.codecharta-ratchet.json`-Baseline von `query.go`, die Zahlen der Referenzdokumentation) — und dort ist das Vorgehen vorgeschrieben: einsetzen, was gemessen wurde, und bei einer Abweichung anhalten und melden.

**Typkonsistenz.**
- `SyntaxonChildren(ctx, parentID string) ([]domain.Syntaxon, error)`, `SyntaxonAncestors(ctx, id string) ([]domain.Syntaxon, error)`, `HabitatTypeCountForSyntaxon(ctx, syntaxonID string) (int, error)` (Task 1), `SyntaxaByRank(ctx, rank, lifeFormGroup string) ([]domain.Syntaxon, error)` und `SyntaxonRanks(ctx) ([]string, error)` (Task 2) werden in Task 3 mit exakt diesen Signaturen aufgerufen und in `fakeRepo` mit exakt diesen implementiert.
- `input.QueryService.Syntaxon(ctx, id, lang string) (SyntaxonDetail, error)` und `SyntaxaByRank(ctx, rank, lifeFormGroup string) ([]SyntaxonRef, error)` (Task 3) werden in Task 4 so gerufen und in `fakeQueryService` so implementiert. Die Implementierung in `syntaxon_nav.go` benennt den zweiten String-Parameter `_`, was die Interface-Erfüllung nicht berührt.
- `syntaxonRef`/`syntaxonRefs` liefern `input.SyntaxonRef` bzw. `[]input.SyntaxonRef` und werden von `Syntaxon`, `SyntaxaByRank` **und** dem umgestellten `syntaxaOf` benutzt — drei Aufrufer, ein Mapper, keine Möglichkeit für zwei Feldmengen.
- `scanSyntaxa(rows *sql.Rows) ([]domain.Syntaxon, error)` bedient `SyntaxonChildren` und `SyntaxaByRank` (beide Varianten) mit derselben neunspaltigen Reihenfolge; `SyntaxonAncestors` scannt nicht selbst, sondern ruft `d.Syntaxon`.
- `maxSyntaxonAncestors` ist an drei Stellen benutzt — im Ahnenpfad, als Bindungswert im CTE und im Integritätstest — und an keiner als Literal wiederholt; der `fakeRepo` spiegelt die 3 bewusst als eigene Zahl, damit ein Test nicht die Konstante der Produktion prüft.
- Jede neue Slice-Rückgabe ist non-nil initialisiert (`[]domain.Syntaxon{}`, `make([]input.SyntaxonRef, 0, len(in))`), wie `read_syntaxon.go` es schon tut und begründet; die Tests prüfen das ausdrücklich (`got == nil` ist ein Fehlschlag, nicht nur `len(got) != 0`).
