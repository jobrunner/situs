# Syntaxa-Quellenumkehr — Implementierungsplan (Teilprojekt A)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** FloraVeg wird die primäre Syntaxa-Quelle, sodass jeder Verband über eine lückenlose `parent_id`-Kette bis zu einer Formation navigierbar ist und die Moos-, Flechten- und Algengesellschaften überhaupt im Index erscheinen.

**Architecture:** Die Pipeline `pipelines/eurovegchecklist` gibt den bisher verworfenen EEA-Altcode aus der Klammer der FloraVeg-Code-Zelle mit aus. Alle Syntaxa-Ingest-Schritte wandern aus `IngestCSV` in eine neue `application.IngestSyntaxa`, die in einer Transaktion Formationen, EVC-Hierarchie, EEA-Restzeilen, Habitattyp-Kanten (per Altcode auf Primärcodes umgeschrieben) und die Restelternteile schreibt. Der Namens-Präfixabgleich bleibt nur noch für die 16 Einheiten ohne FloraVeg-Gegenstück.

**Tech Stack:** Go 1.26 (stdlib, `modernc.org/sqlite`, `gorilla/mux`, `cobra`, `viper`), Python 3 **nur Stdlib** für die Pipeline.

**Spec:** `docs/superpowers/specs/2026-09-21-syntaxa-quellenumkehr-design.md`

## Global Constraints

- `make verify` muss vor jedem Commit grün sein (fmt-check, vet, lint, test, arch, debt, build).
- Null `//nolint`, null `#nosec`, null `TODO`/`FIXME`/`HACK`/`XXX` in Go-Dateien. `.debt-budget` steht auf 0.
- Coverage-Floors sind ein Raise-Only-Ratchet (`.coverage-floors`). Aktuell: `internal/application` **100.0 %**, `internal/domain` 100.0 %, `internal/adapters/sqlite` 96 %, `internal/adapters/http` 88 %, `cmd/situs` 74 %, `internal/ports/output` 66 %. Neuer Code in `internal/application` und `internal/domain` muss vollständig getestet sein, sonst fällt das Gate.
- SQL-Statements sind **statische Strings** mit `?`-Platzhaltern. Niemals Werte konkatenieren (gosec G201/G202 bricht den Build). `PRAGMA table_info(...)` wird pro Tabelle wörtlich ausgeschrieben, nie aus einem Tabellennamen gebaut.
- Keine neue direkte Abhängigkeit. `gomodguard_v2` bricht den Build, bis sie in `.golangci.yml` bewusst eingetragen ist. Dieses Teilprojekt braucht keine.
- Pipeline-Python: **nur Stdlib**. Kein `openpyxl` (die Ausnahme gilt ausschließlich für die übernommenen Trait-Konverter in `pipelines/eive` und `pipelines/tichy`).
- **Komplexitäts-Ratchet (`.codecharta-ratchet.json`, `make codecharta`).** Der
  harte Deckel ist `function_complexity.default_cap = 10` — die komplexeste
  **Funktion** je Datei. `internal/application/ingest.go`, `query.go` und
  `localize.go` sitzen bereits bei **exakt 10**: jedes Wachstum ihrer
  schlimmsten Funktion bricht das Gate sofort. `cmd/situs/ingest.go` hat eine
  Baseline von **12** für `runIngest`. Der Deckel je Datei
  (`complexity.default_cap`) ist 50, Baselines: `ingest.go` 64, `query.go` 72.
  Baselines darf man **senken**, Heraufsetzen braucht eine schriftliche
  Begründung. Praktische Folge für diesen Plan: neue Funktionen werden von
  Anfang an klein geschnitten, und `runIngest` bekommt eine Phase nur über
  einen extrahierten Helfer (Muster: `ingestLocalOverlays`, `sealIndex`).
- OpenAPI liegt in zwei byte-identischen Kopien: `internal/adapters/http/openapi.yaml` und `api/openapi/openapi.yaml`. Der Vertragstest prüft Routen↔Spec in **beiden** Richtungen und schlägt bei einer Route ohne explizites `.Methods()` an.
- Fehlercodes sind genau drei: `INVALID_QUERY`, `NOT_FOUND`, `INTERNAL_ERROR`.
- Deutsch für `README.md` und Commit-Nachrichten; Code-Kommentare sparsam, englisch, und nur wo sie ein *Warum* erklären.
- Conventional Commits. `VERSION` und `CHANGELOG.md` gehören release-please — niemals händisch anfassen.

## Referenzmesswerte (gemessen 2026-09-21, gegen `situs.sqlite` und die gepinnten Artefakte)

Diese Zahlen sind die Sollwerte, gegen die die Tasks prüfen:

| Größe | Wert |
|---|---|
| FloraVeg-Zeilen gesamt | 1841 (150 Klassen, 381 Ordnungen, 1310 Verbände) |
| Zeilen mit Altcode in der Klammer | 1841 von 1841, eindeutig, 0 Kollisionen |
| Sektionsbuchstaben in den 150 Klassen | 25, lückenlos A–Y |
| EEA-Syntaxa gesamt | 1050 (1049 Verbände, 1 Ordnung `ASP-03`) |
| davon per Altcode auf FloraVeg joinbar | 1034 |
| EEA-Einheiten **ohne** FloraVeg-Gegenstück | 16 |
| davon mit Elternteil per Namensabgleich | 6 |
| davon nur per Geschwisterkonsens lösbar | 10 |
| Syntaxa nach dem Ingest | 25 + 150 + 381 + 1310 + 16 = **1882** |
| Verbände nach dem Ingest | 1310 + 16 = **1326** |
| Verbände in den Sektionen R–Y danach | 190 (heute 2) |
| Zeilen ohne Elternteil danach | **0** (außer den 25 Formationen) |

---

### Task 1: Pipeline gibt den Altcode aus und liest Zellen über ihren Bezug

**Files:**
- Modify: `pipelines/eurovegchecklist/xlsx_to_csv.py`
- Modify: `pipelines/eurovegchecklist/test_xlsx_to_csv.py`
- Modify: `pipelines/eurovegchecklist/README.md`

**Interfaces:**
- Consumes: nichts (erster Task).
- Produces: `out/syntaxa_hierarchy.csv` mit **sechs** Spalten `code,rank,name,author,parent_code,alt_code`; `out/report.json` mit den zusätzlichen Schlüsseln `alt_codes` (int) und `alt_code_collisions` (Liste von Strings). Python-Funktionen: `split_code(cell) -> (primary, alt)` ersetzt `primary_code(cell) -> str`; `col_index(ref) -> int`; `read_sheet` unverändert in der Signatur.

- [ ] **Step 1: Die failing Tests schreiben**

In `pipelines/eurovegchecklist/test_xlsx_to_csv.py` ergänzen (die Datei nutzt `unittest`; bestehende Importzeile um `split_code`, `col_index` erweitern):

```python
    def test_split_code_trennt_primaercode_und_altcode(self):
        self.assertEqual(xlsx_to_csv.split_code("AA01A (PAP-01A)"), ("AA01A", "PAP-01A"))
        self.assertEqual(xlsx_to_csv.split_code("  AA01 (PAP-01)  "), ("AA01", "PAP-01"))

    def test_split_code_ohne_klammer_liefert_leeren_altcode(self):
        self.assertEqual(xlsx_to_csv.split_code("AA01A"), ("AA01A", ""))

    def test_col_index_rechnet_zellbezug_in_spalte(self):
        self.assertEqual(xlsx_to_csv.col_index("A1"), 0)
        self.assertEqual(xlsx_to_csv.col_index("Z9"), 25)
        self.assertEqual(xlsx_to_csv.col_index("AA1"), 26)
        self.assertEqual(xlsx_to_csv.col_index("EF12"), 135)

    def test_read_sheet_haelt_luecken_offen(self):
        # A row where Excel omits the second cell: B is missing, C is
        # present. Positional reading would shift "x" into B's place.
        sheet = (
            '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
            '<sheetData>'
            '<row r="1"><c r="A1" t="inlineStr"><is><t>A</t></is></c>'
            '<c r="C1" t="inlineStr"><is><t>x</t></is></c></row>'
            '</sheetData></worksheet>'
        )
        path = self._write_xlsx_with_sheet(sheet)
        rows = xlsx_to_csv.read_sheet(path, "xl/worksheets/sheet1.xml")
        self.assertEqual(rows[0], ["A", "", "x"])

    def test_report_zaehlt_altcodes_und_kollisionen(self):
        rows = [
            {"Code": "AA (PAP)", "Name": "K", "Author": ""},
            {"Code": "AA01 (PAP-01)", "Name": "O", "Author": ""},
            {"Code": "AB01 (PAP-01)", "Name": "O2", "Author": ""},
        ]
        report = self._convert_rows(rows)
        self.assertEqual(report["alt_codes"], 3)
        self.assertEqual(report["alt_code_collisions"], ["PAP-01"])
```

Die Datei hat bereits Hilfsfunktionen zum Bauen einer XLSX aus Zeilen-Dicts. Falls `_write_xlsx_with_sheet` (rohes Sheet-XML) und `_convert_rows` (liefert das Report-Dict) fehlen, zuerst anlegen:

```python
    def _write_xlsx_with_sheet(self, sheet_xml):
        """Writes a minimal XLSX with exactly this sheet1.xml."""
        path = os.path.join(self.tmp, "sheet.xlsx")
        with zipfile.ZipFile(path, "w") as zf:
            zf.writestr(
                "xl/workbook.xml",
                '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"'
                ' xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
                '<sheets><sheet name="Data" sheetId="1" r:id="rId1"/></sheets></workbook>',
            )
            zf.writestr(
                "xl/_rels/workbook.xml.rels",
                '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
                '<Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>',
            )
            zf.writestr("xl/worksheets/sheet1.xml", sheet_xml)
        return path
```

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `python3 -m unittest discover -s pipelines/eurovegchecklist -v`
Expected: FAIL — `AttributeError: module has no attribute 'split_code'` bzw. `col_index`, und `test_read_sheet_haelt_luecken_offen` liefert `["A", "x"]` statt `["A", "", "x"]`.

- [ ] **Step 3: Implementieren**

In `pipelines/eurovegchecklist/xlsx_to_csv.py`:

```python
CSV_HEADERS = {
    "syntaxa_hierarchy.csv": ["code", "rank", "name", "author", "parent_code", "alt_code"],
}
```

`primary_code` durch `split_code` ersetzen (Docstring anpassen, `_CODE_CELL_RE` bekommt eine zweite Gruppe):

```python
# The code cell of the real FloraVeg output carries the historical EEA
# code in parentheses alongside the primary EVC code, e.g. "AA01A
# (PAP-01A)". That parenthesized part IS the code scheme of the EEA-EUNIS
# source and therefore the exact join key between the two sources — that
# is why it is emitted instead of discarded.
_CODE_CELL_RE = re.compile(r"^(\S+)\s*\(([^)]*)\)\s*$")


def split_code(cell):
    """Splits a code cell into (primary_code, alt_code). A cell without a
    parenthesized part yields an empty alt_code — not an error."""
    cell = cell.strip()
    m = _CODE_CELL_RE.match(cell)
    if m:
        return m.group(1), m.group(2).strip()
    return cell, ""
```

Zellbezug auswerten:

```python
def col_index(ref):
    """Converts the column part of a cell reference ("AB7") into a
    0-based column index. Excel omits empty cells from the XML; without
    this conversion, a gap would shift every following column."""
    n = 0
    for ch in ref:
        if not ch.isalpha():
            break
        n = n * 26 + (ord(ch.upper()) - 64)
    return n - 1
```

`read_sheet` auf den Bezug umstellen:

```python
def read_sheet(src, sheet_path):
    """Return the sheet as a list of equal-length string rows, each cell at
    the column its own r-attribute names."""
    with zipfile.ZipFile(src) as zf:
        shared = _shared_strings(zf)
        root = ET.fromstring(zf.read(sheet_path))
    rows = []
    for row in root.iter(f"{{{NS['m']}}}row"):
        cells = {}
        for c in row.findall("m:c", NS):
            cells[col_index(c.get("r") or "A")] = _cell_text(c, shared)
        width = max(cells) + 1 if cells else 0
        rows.append([cells.get(i, "") for i in range(width)])
    width = max((len(r) for r in rows), default=0)
    for r in rows:
        r.extend([""] * (width - len(r)))
    return rows
```

In `convert` den Altcode führen und Kollisionen zählen:

```python
    alt_seen = {}
    collisions = set()
    ...
            code, alt = split_code(raw_code)
            rank, parent_code = rank_and_parent(code)
            if not rank:
                skipped.append((sheet_name, code))
                continue
            if alt:
                if alt in alt_seen and alt_seen[alt] != code:
                    collisions.add(alt)
                alt_seen[alt] = code
            counts[_RANK_COUNTER_KEY[rank]] += 1
            rows_out.append({
                "code": code,
                "rank": rank,
                "name": _cell(row, idx, "Name"),
                "author": _cell(row, idx, "Author"),
                "parent_code": parent_code,
                "alt_code": alt,
            })
```

und im Report:

```python
        "alt_codes": sum(1 for r in rows_out if r["alt_code"]),
        "alt_code_collisions": sorted(collisions),
```

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `python3 -m unittest discover -s pipelines/eurovegchecklist -v`
Expected: PASS, alle Tests.

- [ ] **Step 5: Gegen das echte Artefakt messen**

Run:
```bash
cd pipelines/eurovegchecklist && python3 xlsx_to_csv.py \
  --xlsx artifacts/List_of_European_vegetation_units_version_4.xlsx --out-dir out && cat out/report.json
```
Expected: `classes: 150`, `orders: 381`, `alliances: 1310`, `total_rows: 1841`, `skipped_rows: 0`, **`alt_codes: 1841`**, **`alt_code_collisions: []`**.

Fehlt das Artefakt, zuerst laden (URL und SHA-256 in `pipelines/eurovegchecklist/manifest.yaml`, Befehl in dessen `README.md`). Weicht eine Zahl ab: **anhalten und melden**, nicht die Erwartung anpassen — die Zahlen sind gemessen, nicht geschätzt.

- [ ] **Step 6: README ergänzen**

In `pipelines/eurovegchecklist/README.md` den Abschnitt „Reale Spaltenbelegung" anpassen: der Klammercode wird jetzt als `alt_code` ausgegeben (bisherige Formulierung „wird verworfen" ist falsch geworden), und `read_sheet` wertet den Zellbezug aus. Die gemessenen Zahlen um `alt_codes: 1841` erweitern.

- [ ] **Step 7: Commit**

```bash
git add pipelines/eurovegchecklist/
git commit -m "feat(eurovegchecklist): Altcode ausgeben und Zellen ueber ihren Bezug lesen"
```

---

### Task 2: Formationen als Datendatei und im Sammelskript

**Files:**
- Create: `data/syntaxa_formations.csv`
- Modify: `scripts/collect-ingest-input.sh`
- Modify: `pipelines/eurovegchecklist/manifest.yaml`

**Interfaces:**
- Consumes: nichts.
- Produces: `data/syntaxa_formations.csv` mit Kopfzeile `letter,name_en,life_form_group` und 25 Datenzeilen; im Sammelziel liegt sie unter `syntaxa_formations.csv`.

- [ ] **Step 1: Die Datendatei anlegen**

`data/syntaxa_formations.csv` — die 25 Sektionen von https://floraveg.eu/vegetation/, vollständig:

```csv
letter,name_en,life_form_group
A,Vegetation of the arctic zone,phanerogam
B,Vegetation of the boreal zone,phanerogam
C,Vegetation of the nemoral forest zone,phanerogam
D,Vegetation of the steppe zone,phanerogam
E,Vegetation of the continental desert zone,phanerogam
F,Vegetation of the mediterranean zone,phanerogam
G,"Vegetation of the Canary Islands, Madeira and Azores",phanerogam
H,Alluvial forests and scrub,phanerogam
I,Swamp forests and scrub,phanerogam
J,Vegetation of coastal cliffs and dunes,phanerogam
K,Vegetation of rock crevices and screes,phanerogam
L,Arctic-alpine vegetation of snow-rich habitats,phanerogam
M,Vegetation of saline and brackish waters and swamps,phanerogam
N,Freshwater aquatic vegetation,phanerogam
O,"Vegetation of freshwater springs, shorelines and swamps",phanerogam
P,Vegetation of bogs and fens,phanerogam
Q,Anthropogenic vegetation,phanerogam
R,Epigaeic bryophyte and lichen vegetation,bryophyte_lichen
S,Epilithic bryophyte and lichen vegetation,bryophyte_lichen
T,"Bryophyte and lichen vegetation on bark, wood, leaves, and shaded boulders and base-rich cliff bases",bryophyte_lichen
U,Vegetation of freshwater algae,algae
V,Vegetation of soil algae,algae
W,Aerophytic algal vegetation,algae
X,Vegetation of snow and ice algae,algae
Y,Vegetation of marine algae,algae
```

- [ ] **Step 2: Prüfen, dass sie zu den Klassen im Index passt**

Run:
```bash
python3 -c "
import csv, string
rows=list(csv.DictReader(open('data/syntaxa_formations.csv')))
print('Zeilen:', len(rows))
letters=[r['letter'] for r in rows]
print('A-Y lueckenlos:', letters == list(string.ascii_uppercase[:25]))
print('Gruppen:', sorted({r['life_form_group'] for r in rows}))
"
sqlite3 situs.sqlite "SELECT DISTINCT substr(id,1,1) FROM syntaxon WHERE rank='class' ORDER BY 1;" | tr '\n' ' '
```
Expected: `Zeilen: 25`, `A-Y lueckenlos: True`, `Gruppen: ['algae', 'bryophyte_lichen', 'phanerogam']`, und die Buchstabenliste aus dem Index ist `A B C … Y` (25 Werte, deckungsgleich).

- [ ] **Step 3: Sammelskript umstellen**

In `scripts/collect-ingest-input.sh` die Zeile
`"pipelines/eurovegchecklist/out/syntaxa_hierarchy.csv:syntaxa_hierarchy.csv"`
aus dem `OPTIONAL`-Block in den `REQUIRED`-Block verschieben und dort ergänzen:

```bash
  "pipelines/eurovegchecklist/out/syntaxa_hierarchy.csv:syntaxa_hierarchy.csv"
  "data/syntaxa_formations.csv:syntaxa_formations.csv"
```

Beide sind nach diesem Teilprojekt Primärquellen: ohne sie entsteht ein Index ohne Hierarchie, und das darf nicht stillschweigend passieren.

- [ ] **Step 4: Sammelskript laufen lassen**

Run: `make ingest-input`
Expected: Erfolg, und `out/ingest-input/` enthält `syntaxa_hierarchy.csv` sowie `syntaxa_formations.csv`.

- [ ] **Step 5: Herkunft im Manifest festhalten**

In `pipelines/eurovegchecklist/manifest.yaml` unter `sources` einen zweiten Eintrag ergänzen, damit die Herkunft der Formationsnamen nachweisbar ist:

```yaml
  - id: evc-formations
    file: ../../data/syntaxa_formations.csv
    description: >
      Die 25 Sektionen A-Y oberhalb der EVC-Klassen (Formationen), mit
      Lebensform-Gruppe. Die gepinnte XLSX fuehrt die Sektionsnamen nicht
      mit; sie stammen von der Uebersichtsseite und sind deshalb als
      versionierte Datendatei im Repo geführt, nicht als Pipeline-Ausgabe.
    url: "https://floraveg.eu/vegetation/"
    license: "FloraVeg.EU terms of use — https://floraveg.eu/about/"
    retrieved: "2026-09-21"
```

- [ ] **Step 6: Commit**

```bash
git add data/syntaxa_formations.csv scripts/collect-ingest-input.sh pipelines/eurovegchecklist/manifest.yaml
git commit -m "feat(data): Formationsebene A-Y als Datendatei, Hierarchie-CSV wird Pflichtquelle"
```

---

### Task 3: Domäne, Schema und Schemaprüfung

**Files:**
- Modify: `internal/domain/habitat.go`
- Modify: `internal/adapters/sqlite/schema.sql`
- Modify: `internal/adapters/sqlite/db.go`
- Modify: `internal/adapters/sqlite/schema_check.go`
- Test: `internal/domain/habitat_test.go`, `internal/adapters/sqlite/db_test.go`, `internal/adapters/sqlite/schema_check_test.go`

**Interfaces:**
- Consumes: nichts.
- Produces: `domain.Syntaxon` mit den zusätzlichen Feldern `AltCode`, `Source`, `ParentProvenance`, `LifeFormGroup` (alle `string`); die Konstanten `domain.SyntaxonRankFormation = "formation"`, `domain.SyntaxonSourceEVC = "evc"`, `domain.SyntaxonSourceEUNIS = "eunis"`, `domain.ParentProvenanceOfficial = "official"`, `domain.ParentProvenanceDerived = "derived"`, `domain.LifeFormPhanerogam = "phanerogam"`, `domain.LifeFormBryophyteLichen = "bryophyte_lichen"`, `domain.LifeFormAlgae = "algae"`; die Spalten `syntaxon.alt_code`, `syntaxon.source`, `syntaxon.parent_provenance`, `syntaxon.life_form_group`.

- [ ] **Step 1: Den failing Test für die Migration schreiben**

In `internal/adapters/sqlite/db_test.go` (Muster: der bestehende Test für `species_role.provenance`):

```go
func TestMigrateFuegtSyntaxonSpaltenHinzu(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.sqlite")
	db, err := sqlite.OpenForIngest(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenForIngest: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// An index that does not yet have the four columns: create it fresh
	// without them.
	if _, err := db.ExecContext(context.Background(), `DROP TABLE syntaxon`); err != nil {
		t.Fatalf("DROP: %v", err)
	}
	if _, err := db.ExecContext(context.Background(),
		`CREATE TABLE syntaxon (id TEXT PRIMARY KEY, rank TEXT NOT NULL, name TEXT NOT NULL,
		 author TEXT NOT NULL DEFAULT '', parent_id TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("CREATE: %v", err)
	}

	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, col := range []string{"alt_code", "source", "parent_provenance", "life_form_group"} {
		var n int
		row := db.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM pragma_table_info('syntaxon') WHERE name = ?`, col)
		if err := row.Scan(&n); err != nil {
			t.Fatalf("pragma_table_info(%s): %v", col, err)
		}
		if n != 1 {
			t.Errorf("Spalte %s fehlt nach Migrate", col)
		}
	}
}

func TestMigrateIstIdempotentFuerSyntaxon(t *testing.T) {
	path := filepath.Join(t.TempDir(), "twice.sqlite")
	db, err := sqlite.OpenForIngest(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenForIngest: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for i := range 2 {
		if err := db.Migrate(context.Background()); err != nil {
			t.Fatalf("Migrate Durchlauf %d: %v", i+1, err)
		}
	}
}
```

- [ ] **Step 2: Test laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/adapters/sqlite/ -run 'TestMigrate.*Syntaxon|TestMigrateIstIdempotent' -v`
Expected: FAIL — „Spalte alt_code fehlt nach Migrate" (viermal).

- [ ] **Step 3: Schema und Migration implementieren**

In `internal/adapters/sqlite/schema.sql` die Tabelle `syntaxon` auf den Zielzustand bringen (für frisch angelegte Indizes) und die Indizes ergänzen:

```sql
CREATE TABLE IF NOT EXISTS syntaxon (
  id                TEXT PRIMARY KEY,
  rank              TEXT NOT NULL,
  name              TEXT NOT NULL,
  author            TEXT NOT NULL DEFAULT '',
  parent_id         TEXT NOT NULL DEFAULT '',
  -- The EEA-EUNIS code of the same syntaxon, from the parenthesized part
  -- of the FloraVeg code cell. It is the join key between the two sources
  -- and stays in the index so a client can resolve an old code.
  alt_code          TEXT NOT NULL DEFAULT '',
  source            TEXT NOT NULL DEFAULT 'evc'
                    CHECK (source IN ('evc', 'eunis')),
  parent_provenance TEXT NOT NULL DEFAULT 'official'
                    CHECK (parent_provenance IN ('official', 'derived')),
  -- Only set on formation rows: the group is a property of the formation,
  -- and a correction would otherwise have to be propagated across 1882
  -- rows. A filter joins at most three levels upward.
  life_form_group   TEXT NOT NULL DEFAULT ''
                    CHECK (life_form_group IN ('', 'phanerogam', 'bryophyte_lichen', 'algae'))
);
CREATE INDEX IF NOT EXISTS idx_syntaxon_parent ON syntaxon(parent_id);
CREATE INDEX IF NOT EXISTS idx_syntaxon_rank   ON syntaxon(rank);
```

**`idx_syntaxon_alt_code` darf hier NICHT stehen.** `OpenForIngest` wendet das eingebettete `schema.sql` an (`internal/adapters/sqlite/open.go`), und zwar **bevor** `cmd/situs/ingest.go` `Migrate` aufruft. Auf einem Index, der vor dieser Fassung gebaut wurde, gibt es die Spalte `alt_code` zu diesem Zeitpunkt noch nicht: das `CREATE INDEX` scheitert mit `no such column: alt_code`, `OpenForIngest` gibt „applying schema to …" zurück, und der ganze Ingest bricht ab, bevor die Migration die Spalte anlegen könnte. Der Index wird deshalb in `Migrate` erzeugt, direkt nach dem `ALTER TABLE` (Step 3 unten). `idx_syntaxon_parent` und `idx_syntaxon_rank` sind unbedenklich — beide Spalten gibt es seit immer.

Kein `CHECK` auf `rank`: die Tabelle hatte nie eines, und ein weiterer Rang soll eine Datenzeile sein, keine Schemaänderung. `source`, `parent_provenance` und `life_form_group` bekommen eines, weil ihre Wertemenge fachlich feststeht und ein Tippfehler dort eine stille Falschaussage wäre.

Beachte den Unterschied in den Vorgabewerten: in `schema.sql` (frisch angelegter Index, der sofort ingestiert wird) stehen `'evc'` und `'official'`; die **Migration** eines bestehenden Index setzt dagegen `''`, siehe Step 3.

In `internal/adapters/sqlite/db.go`, in `addMissingColumns`, nach dem `habitat_description`-Block:

```go
	syntaxa, err := tableColumns(ctx, db, `PRAGMA table_info(syntaxon)`)
	if err != nil {
		return fmt.Errorf("checking syntaxon columns: %w", err)
	}
	// Each ALTER statement carries its CHECK along: a migrated index must
	// not accept values that schema.sql forbids a new one — the mismatch
	// would only surface in production.
	// The migration defaults are EMPTY, not 'evc'/'official'. An existing
	// index carries 1049 rows from the EEA source and 1005 parents set by
	// name matching; defaulting to 'evc'/'official' would label exactly
	// those as EVC-sourced and source-backed, and the promise "a derived
	// parent is recognizable by derived" would be broken for every
	// migrated-but-not-freshly-ingested index. The CHECK therefore
	// explicitly allows ''; the ingest sets the real values.
	for _, c := range []struct{ name, ddl string }{
		{"alt_code", `ALTER TABLE syntaxon ADD COLUMN alt_code TEXT NOT NULL DEFAULT ''`},
		{"source", `ALTER TABLE syntaxon ADD COLUMN source TEXT NOT NULL DEFAULT ''
		            CHECK (source IN ('', 'evc', 'eunis'))`},
		{"parent_provenance", `ALTER TABLE syntaxon ADD COLUMN parent_provenance TEXT NOT NULL DEFAULT ''
		                       CHECK (parent_provenance IN ('', 'official', 'derived'))`},
		{"life_form_group", `ALTER TABLE syntaxon ADD COLUMN life_form_group TEXT NOT NULL DEFAULT ''
		                     CHECK (life_form_group IN ('', 'phanerogam', 'bryophyte_lichen', 'algae'))`},
	} {
		if syntaxa[c.name] {
			continue
		}
		if _, err := db.ExecContext(ctx, c.ddl); err != nil {
			return fmt.Errorf("adding syntaxon.%s: %w", c.name, err)
		}
	}
	// Only here, not in schema.sql: the schema is applied BEFORE this
	// migration, and alt_code does not exist yet on an old index at that
	// point.
	if _, err := db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_syntaxon_alt_code ON syntaxon(alt_code)`); err != nil {
		return fmt.Errorf("creating idx_syntaxon_alt_code: %w", err)
	}
```

Die `ddl`-Strings sind Literale im Slice — kein Wert wird interpoliert, `c.name` dient nur der Existenzprüfung und der Fehlermeldung.

Damit der `CHECK` in `schema.sql` und der in der Migration nicht auseinanderlaufen: `schema.sql` lässt `''` ebenfalls zu (die Spalte ist dort mit `'evc'` vorbelegt, aber ein migrierter Index bringt leere Werte mit, und beide Indizes müssen dieselbe Tabelle beschreiben).

- [ ] **Step 4: Test laufen lassen und grün sehen**

Run: `go test ./internal/adapters/sqlite/ -run 'TestMigrate' -v`
Expected: PASS.

- [ ] **Step 5: Den failing Test für die Schemaprüfung schreiben**

In `internal/adapters/sqlite/schema_check_test.go` (Muster: der bestehende Test für eine fehlende `habitat_description.provenance`):

```go
func TestOpenReadOnlyLehntIndexOhneSyntaxonSpaltenAb(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alt.sqlite")
	db, err := sqlite.OpenForIngest(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenForIngest: %v", err)
	}
	if _, err := db.ExecContext(context.Background(),
		`ALTER TABLE syntaxon DROP COLUMN parent_provenance`); err != nil {
		t.Fatalf("DROP COLUMN: %v", err)
	}
	if err := db.FinalizeForServing(context.Background()); err != nil {
		t.Fatalf("FinalizeForServing: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := sqlite.OpenReadOnly(context.Background(), path); err == nil {
		t.Fatal("OpenReadOnly hat einen Index ohne syntaxon.parent_provenance akzeptiert")
	} else if !strings.Contains(err.Error(), "parent_provenance") {
		t.Errorf("Fehler benennt die fehlende Spalte nicht: %v", err)
	}
}
```

- [ ] **Step 6: Test laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/adapters/sqlite/ -run TestOpenReadOnlyLehntIndexOhneSyntaxon -v`
Expected: FAIL — „OpenReadOnly hat einen Index ohne syntaxon.parent_provenance akzeptiert".

- [ ] **Step 7: Schemaprüfung erweitern**

In `internal/adapters/sqlite/schema_check.go`, in `migratedColumns`:

```go
	{"syntaxon", `PRAGMA table_info(syntaxon)`, []string{"alt_code", "source", "parent_provenance", "life_form_group"}},
```

Der Kommentar über `migratedColumns` erklärt bereits, warum: ein Index aus einer älteren Fassung trägt jede Tabelle, aber nicht diese Spalten, und ein neueres Binary würde ihn scheinbar bedienen und jede Syntaxa-Antwort mit `INTERNAL_ERROR` beantworten. Nach diesem Teilprojekt wäre die Folge schwerer: ein Dienst ohne Hierarchie bei grüner Gesundheitsprüfung.

- [ ] **Step 8: Domäne erweitern**

In `internal/domain/habitat.go`:

```go
// The ranks of the syntaxa hierarchy. "formation" is the root (the
// sections A-Y of the EuroVegChecklist) and the only rank with an empty
// ParentID.
const (
	SyntaxonRankFormation = "formation"
	SyntaxonRankClass     = "class"
	SyntaxonRankOrder     = "order"
	SyntaxonRankAlliance  = "alliance"
)

// The source of a syntaxon ROW, not of its factual data.
const (
	SyntaxonSourceEVC   = "evc"
	SyntaxonSourceEUNIS = "eunis"
)

const (
	ParentProvenanceOfficial = "official"
	ParentProvenanceDerived  = "derived"
)

const (
	LifeFormPhanerogam      = "phanerogam"
	LifeFormBryophyteLichen = "bryophyte_lichen"
	LifeFormAlgae           = "algae"
)

type Syntaxon struct {
	ID       string
	Rank     string
	Name     string
	Author   string
	ParentID string

	// AltCode is the EEA-EUNIS code of the same syntaxon. Empty for
	// formations and for units known to only one of the two sources.
	AltCode string

	// Source names the source of the row: SyntaxonSourceEVC or
	// SyntaxonSourceEUNIS.
	Source string

	// ParentProvenance distinguishes a parent taken from the source
	// (ParentProvenanceOfficial) from a derived one
	// (ParentProvenanceDerived). Always official when ParentID is empty.
	ParentProvenance string

	// LifeFormGroup is only set on formation rows.
	LifeFormGroup string
}
```

Den bisherigen Kommentar an `Name` („FloraVeg's clean name … else the historical EUNIS combi-string") beibehalten — er bleibt für die 16 EEA-eigenen Zeilen zutreffend.

- [ ] **Step 9: Domänen-Test schreiben und laufen lassen**

Ein Test, der die Konstantenwerte festnagelt (sie stehen im Schema-`CHECK` und im OpenAPI-Enum; eine Umbenennung darf nicht stillschweigend durchgehen):

```go
func TestSyntaxonKonstantenWerte(t *testing.T) {
	for got, want := range map[string]string{
		domain.SyntaxonRankFormation:    "formation",
		domain.SyntaxonSourceEVC:        "evc",
		domain.SyntaxonSourceEUNIS:      "eunis",
		domain.ParentProvenanceDerived:  "derived",
		domain.LifeFormBryophyteLichen:  "bryophyte_lichen",
	} {
		if got != want {
			t.Errorf("Konstante = %q, erwartet %q", got, want)
		}
	}
}
```

Run: `go test ./internal/domain/ ./internal/adapters/sqlite/ -v`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/domain/ internal/adapters/sqlite/
git commit -m "feat(domain,sqlite): Syntaxon um Altcode, Quelle, Elternteil-Provenienz und Lebensform"
```

---

### Task 4: Schreib- und Leseoperationen für den Syntaxa-Ingest

**Files:**
- Modify: `internal/ports/output/repository.go`
- Modify: `internal/adapters/sqlite/write.go` (dort liegt die `IngestTx`-Implementierung)
- Modify: `internal/adapters/sqlite/read_syntaxon.go`
- Test: `internal/adapters/sqlite/read_syntaxon_test.go`, `internal/adapters/sqlite/write_test.go`

**Interfaces:**
- Consumes: `domain.Syntaxon` (Task 3).
- Produces:
  - `IngestTx.UpsertSyntaxon(s domain.Syntaxon) error` — schreibt jetzt **alle** Felder.
  - `IngestTx.SetSyntaxonParent(id, parentID, provenance string) error` — setzt nur das Elternteil samt Provenienz.
  - `IngestTx.RelinkSyntaxon(from, to string) error` — schreibt `habitat_type_syntaxon.syntaxon_id` von `from` auf `to` um.
  - `Repository.SyntaxonIDsByAltCode(ctx) (map[string]string, error)` — Altcode → Primärcode.
  - `IngestTx.UpsertSyntaxonAuthor` **entfällt**.

- [ ] **Step 1: Die failing Tests schreiben**

In `internal/adapters/sqlite/write_test.go`:

```go
func TestUpsertSyntaxonSchreibtAlleFelder(t *testing.T) {
	db := openTestDB(t) // existing helper of the package
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	want := domain.Syntaxon{
		ID: "CA01A", Rank: domain.SyntaxonRankAlliance, Name: "Test-Verband",
		Author: "Autor 1970", ParentID: "CA01", AltCode: "TST-01A",
		Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial,
	}
	if err := tx.UpsertSyntaxon(want); err != nil {
		t.Fatalf("UpsertSyntaxon: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	got, err := db.Syntaxon(context.Background(), "CA01A")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got != want {
		t.Errorf("Syntaxon = %+v, erwartet %+v", got, want)
	}
}

func TestSetSyntaxonParentSetztNurElternteilUndProvenienz(t *testing.T) {
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	_ = tx.UpsertSyntaxon(domain.Syntaxon{
		ID: "NAR-01E", Rank: domain.SyntaxonRankAlliance, Name: "Campanulo-Nardion",
		Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceOfficial,
	})
	if err := tx.SetSyntaxonParent("NAR-01E", "CI01", domain.ParentProvenanceDerived); err != nil {
		t.Fatalf("SetSyntaxonParent: %v", err)
	}
	_ = tx.Commit()

	got, _ := db.Syntaxon(context.Background(), "NAR-01E")
	if got.ParentID != "CI01" || got.ParentProvenance != domain.ParentProvenanceDerived {
		t.Errorf("ParentID/Provenienz = %q/%q, erwartet CI01/derived", got.ParentID, got.ParentProvenance)
	}
	if got.Name != "Campanulo-Nardion" {
		t.Errorf("Name wurde angefasst: %q", got.Name)
	}
}

func TestRelinkSyntaxonSchreibtKanteUm(t *testing.T) {
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	key := domain.HabitatTypeKey{Typology: domain.TypologyEUNIS2021, Code: "U36"}
	_ = tx.LinkSyntaxon(key, "ASP-03")
	if err := tx.RelinkSyntaxon("ASP-03", "KC03"); err != nil {
		t.Fatalf("RelinkSyntaxon: %v", err)
	}
	_ = tx.Commit()

	keys, err := db.HabitatTypeKeysForSyntaxon(context.Background(), "KC03")
	if err != nil {
		t.Fatalf("HabitatTypeKeysForSyntaxon: %v", err)
	}
	if len(keys) != 1 || keys[0] != key {
		t.Errorf("KC03 traegt %v, erwartet genau %v", keys, key)
	}
	alt, _ := db.HabitatTypeKeysForSyntaxon(context.Background(), "ASP-03")
	if len(alt) != 0 {
		t.Errorf("ASP-03 traegt noch Kanten: %v", alt)
	}
}

func TestRelinkSyntaxonIstIdempotentBeiBestehenderZielkante(t *testing.T) {
	// Both edges already exist: U36 -> ASP-03 AND U36 -> KC03. The
	// rewrite must not fail on the primary key — it must simply remove
	// the source edge.
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	key := domain.HabitatTypeKey{Typology: domain.TypologyEUNIS2021, Code: "U36"}
	_ = tx.LinkSyntaxon(key, "ASP-03")
	_ = tx.LinkSyntaxon(key, "KC03")
	if err := tx.RelinkSyntaxon("ASP-03", "KC03"); err != nil {
		t.Fatalf("RelinkSyntaxon: %v", err)
	}
	_ = tx.Commit()

	keys, _ := db.HabitatTypeKeysForSyntaxon(context.Background(), "KC03")
	if len(keys) != 1 {
		t.Errorf("KC03 traegt %d Kanten, erwartet 1", len(keys))
	}
	alt, _ := db.HabitatTypeKeysForSyntaxon(context.Background(), "ASP-03")
	if len(alt) != 0 {
		t.Errorf("ASP-03 traegt noch Kanten: %v", alt)
	}
}
```

In `internal/adapters/sqlite/read_syntaxon_test.go`:

```go
func TestSyntaxonIDsByAltCode(t *testing.T) {
	db := openTestDB(t)
	tx, _ := db.Begin(context.Background())
	_ = tx.UpsertSyntaxon(domain.Syntaxon{ID: "AA01A", Rank: domain.SyntaxonRankAlliance,
		Name: "X", AltCode: "PAP-01A", Source: domain.SyntaxonSourceEVC,
		ParentProvenance: domain.ParentProvenanceOfficial})
	_ = tx.UpsertSyntaxon(domain.Syntaxon{ID: "AA01B", Rank: domain.SyntaxonRankAlliance,
		Name: "Y", Source: domain.SyntaxonSourceEVC,
		ParentProvenance: domain.ParentProvenanceOfficial})
	_ = tx.Commit()

	got, err := db.SyntaxonIDsByAltCode(context.Background())
	if err != nil {
		t.Fatalf("SyntaxonIDsByAltCode: %v", err)
	}
	if len(got) != 1 || got["PAP-01A"] != "AA01A" {
		t.Errorf("Karte = %v, erwartet genau PAP-01A->AA01A", got)
	}
}
```

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/adapters/sqlite/ -run 'Syntaxon' -v`
Expected: FAIL — die Methoden `SetSyntaxonParent`, `RelinkSyntaxon`, `SyntaxonIDsByAltCode` existieren nicht; `TestUpsertSyntaxonSchreibtAlleFelder` scheitert an den nicht geschriebenen Feldern.

- [ ] **Step 3: Port anpassen**

In `internal/ports/output/repository.go`, in `IngestTx`, `UpsertSyntaxonAuthor` ersetzen durch:

```go
	// SetSyntaxonParent sets the parent and its provenance on an already
	// written row. Separate from UpsertSyntaxon because the parent of the
	// 16 EEA-only units is only known once the FloraVeg rows and their
	// siblings are in the index.
	SetSyntaxonParent(id, parentID, provenance string) error
	// RelinkSyntaxon rewrites every habitat_type_syntaxon edge from
	// syntaxon id from to to. If a habitat type already carries both
	// edges, the from edge is removed instead of duplicating the target
	// edge.
	RelinkSyntaxon(from, to string) error
```

und in `Repository`:

```go
	// SyntaxonIDsByAltCode maps the EEA alt code to the syntaxon id. The
	// syntaxa ingest uses it to resolve the habitat-type edges of the EEA
	// source onto the FloraVeg primary codes. Rows without an alt code are
	// absent from the map.
	SyntaxonIDsByAltCode(ctx context.Context) (map[string]string, error)
```

- [ ] **Step 4: Implementieren**

In der `IngestTx`-Implementierung `UpsertSyntaxon` auf alle Felder erweitern:

```go
func (t *tx) UpsertSyntaxon(s domain.Syntaxon) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO syntaxon (id, rank, name, author, parent_id, alt_code, source,
		                       parent_provenance, life_form_group)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   rank = excluded.rank, name = excluded.name, author = excluded.author,
		   parent_id = excluded.parent_id, alt_code = excluded.alt_code,
		   source = excluded.source, parent_provenance = excluded.parent_provenance,
		   life_form_group = excluded.life_form_group`,
		s.ID, s.Rank, s.Name, s.Author, s.ParentID, s.AltCode, s.Source,
		s.ParentProvenance, s.LifeFormGroup)
	return err
}

func (t *tx) SetSyntaxonParent(id, parentID, provenance string) error {
	_, err := t.tx.ExecContext(t.ctx,
		`UPDATE syntaxon SET parent_id = ?, parent_provenance = ? WHERE id = ?`,
		parentID, provenance, id)
	return err
}

// Two statements, not one: an UPDATE would fail on the primary key if the
// habitat type already carries both edges (exactly the case for
// ASP-03/KC03). First add the target edges where they are missing, then
// remove the source edges.
func (t *tx) RelinkSyntaxon(from, to string) error {
	if _, err := t.tx.ExecContext(t.ctx,
		`INSERT OR IGNORE INTO habitat_type_syntaxon (typology_id, code, syntaxon_id)
		 SELECT typology_id, code, ? FROM habitat_type_syntaxon WHERE syntaxon_id = ?`,
		to, from); err != nil {
		return err
	}
	_, err := t.tx.ExecContext(t.ctx,
		`DELETE FROM habitat_type_syntaxon WHERE syntaxon_id = ?`, from)
	return err
}
```

`UpsertSyntaxonAuthor` und seine Testfälle entfernen.

In `internal/adapters/sqlite/read_syntaxon.go` das `SELECT` jeder Syntaxon-Leseoperation um die vier Spalten erweitern (`Syntaxon`, `Syntaxa`, `AllSyntaxa` — dem Scan-Muster der Datei folgen) und ergänzen:

```go
func (d *DB) SyntaxonIDsByAltCode(ctx context.Context) (map[string]string, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT alt_code, id FROM syntaxon WHERE alt_code <> ''`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var alt, id string
		if err := rows.Scan(&alt, &id); err != nil {
			return nil, err
		}
		out[alt] = id
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Tests laufen lassen und grün sehen**

Run: `go test ./internal/adapters/sqlite/ ./internal/ports/... -v`
Expected: PASS. Das Paket `internal/application` bricht jetzt die Kompilierung, weil `UpsertSyntaxonAuthor` weg ist — das ist erwartet und wird in Task 5 bis 7 behoben. Nur die beiden hier genannten Pakete testen.

- [ ] **Step 6: Commit**

```bash
git add internal/ports/output/repository.go internal/adapters/sqlite/
git commit -m "feat(sqlite): Syntaxon-Felder schreiben, Elternteil setzen, Kanten umschreiben"
```

---

### Task 5: IngestSyntaxa — Formationen und EVC-Hierarchie

**Files:**
- Create: `internal/application/syntaxa_ingest.go` (Einstieg, Report, Formationen, Hierarchie)
- Create: `internal/application/syntaxa_read.go` (die vier CSV-Leser)
- Delete: `internal/application/syntaxa_hierarchy_ingest.go`, `internal/application/syntaxa_hierarchy_ingest_test.go`
- Test: `internal/application/syntaxa_ingest_test.go`

**Dateiaufteilung ist hier keine Geschmacksfrage.** Der Ratchet deckelt die
komplexeste **Funktion** je Datei bei 10 und die Summe je Datei bei 50. Eine
`IngestSyntaxa`, die alle fünf Schritte selbst ausführt, reißt den
Funktionsdeckel sofort. Die Aufteilung ist deshalb von Anfang an vorgegeben:
`IngestSyntaxa` orchestriert nur (lesen, Transaktion, `writeSyntaxa`, Commit),
`writeSyntaxa` ruft je Schritt eine eigene Funktion, und die vier CSV-Leser
liegen in einer zweiten Datei. Task 7 legt aus demselben Grund eine dritte an.

**Interfaces:**
- Consumes: `IngestTx.UpsertSyntaxon`, `Repository.Begin` (Task 4); `domain`-Konstanten (Task 3); die CSV-Spalte `alt_code` (Task 1); `data/syntaxa_formations.csv` (Task 2).
- Produces:
  - `application.SyntaxaReport` mit den Feldern `FormationsWritten, ClassesWritten, OrdersWritten, AlliancesWritten, EunisOnly, LinksRemapped, LinksWritten, ParentsByName, ParentsDerived int`, `Orphans, AltCodeCollisions, AmbiguousMatches, UnknownLinkTargets []string`, `SkippedRows int`.
  - `application.IngestSyntaxa(ctx context.Context, repo output.Repository, dir string) (SyntaxaReport, error)` — `dir` ist das Sammelverzeichnis; die Funktion liest daraus `syntaxa_formations.csv`, `syntaxa_hierarchy.csv`, `syntaxa.csv` und `habitat_type_syntaxa.csv`.
  - intern: `formationOf(classCode string) string` (erster Buchstabe), `readFormations`, `readHierarchy`.

- [ ] **Step 1: Die failing Tests schreiben**

`internal/application/syntaxa_ingest_test.go` — Helfer, der die vier CSVs in ein Temp-Verzeichnis schreibt, und die ersten Fälle:

```go
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
	minimalHierarchy = "code,rank,name,author,parent_code,alt_code\n" +
		"CA,class,Testklasse,Autor 1950,,TST\n" +
		"CA01,order,Testordnung,Autor 1960,CA,TST-01\n" +
		"CA01A,alliance,Testverband,Autor 1970,CA01,TST-01A\n" +
		"RA,class,Moosklasse,Autor 1980,,MOO\n" +
		"RA01,order,Moosordnung,Autor 1981,RA,MOO-01\n" +
		"RA01A,alliance,Moosverband,Autor 1982,RA01,MOO-01A\n"
)

func TestIngestSyntaxaSchreibtFormationenAusDemBuchstaben(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})

	rep, err := application.IngestSyntaxa(context.Background(), repo, dir)
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
	rep, err := application.IngestSyntaxa(context.Background(), repo, dir)
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
	if _, err := application.IngestSyntaxa(context.Background(), repo, dir); err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	got := repo.syntaxonByID("CA01A")
	if got.Name != "Testverband" || got.Author != "Autor 1970" {
		t.Errorf("Name/Autor = %q/%q", got.Name, got.Author)
	}
}

func TestIngestSyntaxaBrichtOhneFormationsdateiAb(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		hierarchy: minimalHierarchy,
		eunis:     "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := application.IngestSyntaxa(context.Background(), repo, dir)
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
	_, err := application.IngestSyntaxa(context.Background(), repo, dir)
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
	if _, err := application.IngestSyntaxa(context.Background(), repo, dir); err == nil {
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
	rep, err := application.IngestSyntaxa(context.Background(), repo, dir)
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
```

Das Paket hat in `ingest_test.go` bereits einen `fakeRepo`. Er muss um `SetSyntaxonParent`, `RelinkSyntaxon`, `SyntaxonIDsByAltCode` erweitert und um `UpsertSyntaxonAuthor` gekürzt werden; dazu die Helfer `syntaxonByID(id) domain.Syntaxon` und `has(id) bool`. Ein Aufruf von `SetSyntaxonParent` auf einer unbekannten ID muss im Fake fehlschlagen, sonst prüft Task 7 ins Leere:

```go
func (r *fakeRepo) SetSyntaxonParent(id, parentID, provenance string) error {
	for i := range r.syntaxa {
		if r.syntaxa[i].ID == id {
			r.syntaxa[i].ParentID = parentID
			r.syntaxa[i].ParentProvenance = provenance
			return nil
		}
	}
	return fmt.Errorf("fakeRepo: kein Syntaxon %q", id)
}
```

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/application/ -run TestIngestSyntaxa -v`
Expected: FAIL — `undefined: application.IngestSyntaxa`.

- [ ] **Step 3: Implementieren**

`internal/application/syntaxa_ingest.go` anlegen. Die Datei ersetzt `syntaxa_hierarchy_ingest.go`; `longestPrefixMatch` wird aus ihr übernommen (Task 7 braucht es weiter), `ingestHierarchyRows` und `IngestSyntaxaHierarchy` entfallen.

```go
// syntaxa_ingest.go is the only place syntaxa enter the index. The steps
// used to live in two functions and two transactions, because name
// matching had to find the EUNIS rows first; now that FloraVeg is the
// primary source, the dependency is reversed, and the split was only an
// opportunity to get the ordering wrong.
package application

// SyntaxaReport counts what a syntaxa ingest wrote and what it could not
// decide. Every number is measured, none is estimated.
type SyntaxaReport struct {
	FormationsWritten int
	ClassesWritten    int
	OrdersWritten     int
	AlliancesWritten  int

	EunisOnly     int
	LinksWritten  int
	LinksRemapped int

	ParentsByName  int
	ParentsDerived int

	// Orphans are rows that, after every step, remained without a parent
	// and are not a formation. A non-empty value fails the ingest: a
	// chain that breaks at one point makes the orientation service
	// worthless at exactly that point.
	Orphans []string

	AltCodeCollisions  []string
	AmbiguousMatches   []string
	UnknownLinkTargets []string
	SkippedRows        int
}

const (
	fileFormations = "syntaxa_formations.csv"
	fileHierarchy  = "syntaxa_hierarchy.csv"
	fileEunisSyntaxa = "syntaxa.csv"
	fileSyntaxonLinks = "habitat_type_syntaxa.csv"
)

// formationOf returns a class's formation id: the first letter of its
// code. The level lives in the code itself (class "CA" belongs to
// section "C"), just like rank and parent — never guessed from the name.
func formationOf(classCode string) string {
	if classCode == "" {
		return ""
	}
	return classCode[:1]
}

func IngestSyntaxa(ctx context.Context, repo output.Repository, dir string) (SyntaxaReport, error) {
	var rep SyntaxaReport

	formations, err := readFormations(ctx, dir, &rep)
	if err != nil {
		return SyntaxaReport{}, err
	}
	rows, err := readHierarchy(ctx, dir, &rep)
	if err != nil {
		return SyntaxaReport{}, err
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return SyntaxaReport{}, fmt.Errorf("beginning syntaxa ingest transaction: %w", err)
	}
	if err := writeSyntaxa(ctx, tx, dir, formations, rows, &rep); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return SyntaxaReport{}, fmt.Errorf("%w (rollback also failed: %w)", err, rbErr)
		}
		return SyntaxaReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyntaxaReport{}, fmt.Errorf("committing syntaxa ingest transaction: %w", err)
	}
	return rep, nil
}
```

`readFormations` liest `letter,name_en,life_form_group` über den bestehenden `readAll`-Helfer (er fordert die Spalten ein und bricht bei einer fehlenden ab; eine fehlende Datei muss hier **Fehler** sein, nicht Überspringen — anders als bei `IngestLocalizations`) und liefert `map[string]domain.Syntaxon`.

**Pfade mit `splitCSVPath`, nicht `filepath.Split`.** `readAll` benutzt `os.OpenRoot(dir)`, und `filepath.Split` eines bloßen Dateinamens liefert `dir == ""`, was `os.OpenRoot` ablehnt. `internal/application/species_crosswalk.go` hält den Helfer `splitCSVPath` genau dafür bereit; `area_ingest.go` dokumentiert den Grund. Die gelöschte `syntaxa_hierarchy_ingest.go` benutzte noch das rohe `filepath.Split` — beim Umbau auf `splitCSVPath` wechseln. `readHierarchy` liest die sechs Spalten und liefert `[]hierarchyRow` (um `altCode` erweitert).

`writeSyntaxa` führt in dieser Reihenfolge aus, jeweils über `tx`:
1. Formationen schreiben (`Rank: domain.SyntaxonRankFormation`, `ParentID: ""`, `Source: domain.SyntaxonSourceEVC`, `LifeFormGroup` aus der Datei).
2. Hierarchiezeilen schreiben. Für eine Klasse ist `ParentID = formationOf(code)`; ist dieser Buchstabe **keine** bekannte Formation, wird die Zeile übersprungen und in `SkippedRows` gezählt — nie eine Formation erfinden. Ordnungen und Verbände übernehmen `parent_code` aus der CSV.

Die Schritte 3 bis 5 kommen in Task 6 und 7 dazu; `writeSyntaxa` bekommt sie dort angehängt.

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./internal/application/ -run TestIngestSyntaxa -v`
Expected: PASS für die acht Tests aus Step 1.

- [ ] **Step 5: Commit**

```bash
git add internal/application/
git commit -m "feat(ingest): Formationen und EVC-Hierarchie als primaere Syntaxa-Quelle"
```

---

### Task 6: IngestSyntaxa — EEA-Restzeilen und Kanten über den Altcode

**Files:**
- Modify: `internal/application/syntaxa_ingest.go`
- Test: `internal/application/syntaxa_ingest_test.go`

**Interfaces:**
- Consumes: `writeSyntaxa`, `SyntaxaReport`, `hierarchyRow` (Task 5); `IngestTx.LinkSyntaxon`, `IngestTx.RelinkSyntaxon` (Task 4).
- Produces: in `writeSyntaxa` die Schritte 3 und 4; intern `writeEunisOnly`, `writeLinks`.

- [ ] **Step 1: Die failing Tests schreiben**

```go
func TestIngestSyntaxaSchreibtNurEeaEinheitenOhneFloraVegGegenstueck(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		// TST-01A has a FloraVeg counterpart (CA01A), EIG-01A does not.
		eunis: "id,rank,name,parent_id\n" +
			"TST-01A,alliance,Testverband Autor 1970,\n" +
			"EIG-01A,alliance,Eigenverband Autor 1990,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := application.IngestSyntaxa(context.Background(), repo, dir)
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
	if eig.Source != domain.SyntaxonSourceEUNIS || eig.AltCode != "" {
		t.Errorf("EIG-01A = %+v, erwartet source=eunis und leeren AltCode", eig)
	}
	if eig.Name != "Eigenverband Autor 1990" {
		t.Errorf("Name = %q, erwartet den EUNIS-Kombistring unveraendert", eig.Name)
	}
}

func TestIngestSyntaxaLoestKantenUeberDenAltcodeAuf(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\nTST-01A,alliance,Testverband Autor 1970,\n",
		links: "typology_id,code,syntaxon_id\neunis@2021,T11,TST-01A\n",
	})
	rep, err := application.IngestSyntaxa(context.Background(), repo, dir)
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
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\nEIG-01A,alliance,Eigenverband Autor 1990,\n",
		links: "typology_id,code,syntaxon_id\neunis@2021,T11,EIG-01A\n",
	})
	rep, err := application.IngestSyntaxa(context.Background(), repo, dir)
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
	rep, err := application.IngestSyntaxa(context.Background(), repo, dir)
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
		eunis: "id,rank,name,parent_id\nTST-01,order,Testordnung Autor 1960,\n",
		links: "typology_id,code,syntaxon_id\neunis@2021,T11,TST-01\n",
	})
	rep, err := application.IngestSyntaxa(context.Background(), repo, dir)
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
```

`fakeRepo` um `linkTargets(typology, code string) []string` erweitern.

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/application/ -run TestIngestSyntaxa -v`
Expected: FAIL — `EunisOnly = 0, erwartet 1`, `LinksWritten = 0`, und die Kanten fehlen.

- [ ] **Step 3: Implementieren**

In `writeSyntaxa` nach dem Hierarchieschritt:

```go
	// Step 3: the EEA units that FloraVeg does not carry. Every other
	// unit is already represented by its FloraVeg row — writing it a
	// second time would put the same syntaxon into the index under two
	// ids.
	byAlt := map[string]string{}
	for _, r := range rows {
		if r.altCode != "" {
			byAlt[r.altCode] = r.code
		}
	}
	if err := writeEunisOnly(ctx, tx, dir, byAlt, rep); err != nil {
		return err
	}
	// Step 4: the edges. An EEA syntaxon id with a FloraVeg counterpart
	// gets resolved to the primary code; one without stays as it is.
	return writeLinks(ctx, tx, dir, byAlt, rep)
```

`writeEunisOnly` liest `syntaxa.csv` (`id,rank,name,parent_id`) und schreibt nur Zeilen, deren `id` **nicht** in `byAlt` steht — mit `Source: domain.SyntaxonSourceEUNIS`, `AltCode: ""`, `Author: ""` und `Name` unverändert (der historische Kombistring; nie heuristisch zerlegen). `ParentID` bleibt zunächst leer und wird in Task 7 gesetzt. Jede geschriebene Zeile zählt `EunisOnly`.

`writeLinks` liest `habitat_type_syntaxa.csv`, löst die `syntaxon_id` über `byAlt` auf (Treffer zählt `LinksRemapped`) und ruft `tx.LinkSyntaxon`. Ein Ziel, das weder als FloraVeg- noch als EEA-eigene Zeile existiert, wird nicht verlinkt, in `UnknownLinkTargets` vermerkt und gewarnt. Dafür braucht `writeSyntaxa` die Menge der geschriebenen IDs; sie entsteht ohnehin aus den Schritten 2 und 3 und wird als `map[string]bool` mitgeführt.

`RelinkSyntaxon` wird hier **nicht** gebraucht: die Kanten werden in derselben Transaktion erstmals geschrieben, also gleich auf den richtigen Code. Die Methode bleibt für einen Ingest auf einen bereits gefüllten Index (idempotenter Wiederholungslauf) nötig — der Aufruf steht in Task 7, Schritt 3.

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./internal/application/ -run TestIngestSyntaxa -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/application/
git commit -m "feat(ingest): EEA-eigene Syntaxa erhalten, Kanten ueber den Altcode aufloesen"
```

---

### Task 7: IngestSyntaxa — Restelternteile, Geschwisterkonsens und Waisen-Abbruch

**Files:**
- Create: `internal/application/syntaxa_parents.go` (Namensabgleich, Geschwisterkonsens, Waisenprüfung)
- Modify: `internal/application/syntaxa_ingest.go` (nur der Aufruf von `assignRemainingParents`)
- Test: `internal/application/syntaxa_parents_test.go`

Eigene Datei, weil `longestPrefixMatch`, `siblingConsensus` und `eeaGroup` eine
zusammengehörige Einheit sind und `syntaxa_ingest.go` sonst über den
Summendeckel von 50 läuft.

**Interfaces:**
- Consumes: alles aus Task 5 und 6; `IngestTx.SetSyntaxonParent`, `IngestTx.RelinkSyntaxon` (Task 4).
- Produces: in `writeSyntaxa` den Schritt 5; intern `assignRemainingParents`, `siblingConsensus(id string, parents map[string]string) string`, `longestPrefixMatch` (aus der gelöschten Datei übernommen).

- [ ] **Step 1: Die failing Tests schreiben**

```go
func TestIngestSyntaxaFindetElternteilPerNamensabgleich(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		// Name matches FloraVeg's "Testverband", alt code does not.
		eunis: "id,rank,name,parent_id\nAND-01A,alliance,Testverband Morariu 1957,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := application.IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if rep.ParentsByName != 1 || rep.ParentsDerived != 0 {
		t.Errorf("ParentsByName/Derived = %d/%d, erwartet 1/0",
			rep.ParentsByName, rep.ParentsDerived)
	}
	got := repo.syntaxonByID("AND-01A")
	if got.ParentID != "CA01" || got.ParentProvenance != domain.ParentProvenanceOfficial {
		t.Errorf("AND-01A = %q/%q, erwartet CA01/official", got.ParentID, got.ParentProvenance)
	}
}

func TestIngestSyntaxaLeitetElternteilAusGeschwisterkonsensAb(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: minimalHierarchy +
			"CA02A,alliance,Nachbarverband eins,A 1,CA02,NAR-01A\n" +
			"CA02B,alliance,Nachbarverband zwei,A 2,CA02,NAR-01B\n" +
			"CA02,order,Nachbarordnung,A 0,CA,NAR-01\n",
		// NAR-01E has no alt-code partner and no name match, but siblings
		// NAR-01A/B unanimously point at CA02.
		eunis: "id,rank,name,parent_id\n" +
			"NAR-01A,alliance,Nachbarverband eins A 1,\n" +
			"NAR-01B,alliance,Nachbarverband zwei A 2,\n" +
			"NAR-01E,alliance,Campanulo-Nardion Rivas-Mart. 1964,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := application.IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if rep.ParentsDerived != 1 {
		t.Errorf("ParentsDerived = %d, erwartet 1", rep.ParentsDerived)
	}
	got := repo.syntaxonByID("NAR-01E")
	if got.ParentID != "CA02" || got.ParentProvenance != domain.ParentProvenanceDerived {
		t.Errorf("NAR-01E = %q/%q, erwartet CA02/derived", got.ParentID, got.ParentProvenance)
	}
}

func TestIngestSyntaxaLeitetNichtsAusWiderspruechlicherGruppeAb(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: minimalHierarchy +
			"CA02,order,Nachbarordnung,A 0,CA,QUI-01\n" +
			"CA02A,alliance,Erster,A 1,CA02,QUI-01A\n" +
			"RA02,order,Andere Ordnung,A 9,RA,QUI-02\n" +
			"RA02A,alliance,Zweiter,A 2,RA02,QUI-01B\n",
		// QUI-01A -> CA02, QUI-01B -> RA02: group QUI-01 is
		// contradictory, QUI-01F must not inherit anything.
		eunis: "id,rank,name,parent_id\n" +
			"QUI-01A,alliance,Erster A 1,\n" +
			"QUI-01B,alliance,Zweiter A 2,\n" +
			"QUI-01F,alliance,Genisto pilosae-Pinion pinastri,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	_, err := application.IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz verbleibender Waise durch")
	}
	if !strings.Contains(err.Error(), "QUI-01F") {
		t.Errorf("Fehler benennt die Waise nicht: %v", err)
	}
}

func TestIngestSyntaxaScheitertAnVerbleibenderWaise(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		// No alt-code partner, no name match, no sibling.
		eunis: "id,rank,name,parent_id\nEIN-09Z,alliance,Voellig Unbekanntes 1900,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	_, err := application.IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz verbleibender Waise durch")
	}
	if !strings.Contains(err.Error(), "EIN-09Z") {
		t.Errorf("Fehler benennt die Waise nicht: %v", err)
	}
}

func TestIngestSyntaxaLaesstFormationenElternlos(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := application.IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if len(rep.Orphans) != 0 {
		t.Errorf("Orphans = %v, erwartet leer (Formationen zaehlen nicht)", rep.Orphans)
	}
}

func TestIngestSyntaxaMeldetMehrdeutigenNamensabgleich(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: minimalHierarchy +
			"CA02,order,Zweite Ordnung,A 0,CA,ZWE-01\n" +
			"CA02A,alliance,Testverband,Anderer Autor 1975,CA02,ZWE-01A\n",
		// "Testverband" occurs twice in FloraVeg, same length: no guessing.
		eunis: "id,rank,name,parent_id\nMEH-01A,alliance,Testverband Dritter 1980,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	_, err := application.IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz Waise durch")
	}
	if !strings.Contains(err.Error(), "MEH-01A") {
		t.Errorf("Fehler benennt die Waise nicht: %v", err)
	}
}
```

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/application/ -run TestIngestSyntaxa -v`
Expected: FAIL — `ParentsByName = 0`, `ParentsDerived = 0`, und die Waisen-Tests laufen durch, statt zu scheitern.

- [ ] **Step 3: Implementieren**

`longestPrefixMatch` unverändert aus der gelöschten `syntaxa_hierarchy_ingest.go` übernehmen (Wortgrenzen-Prüfung und Mehrdeutigkeits-Erkennung bleiben, wie sie sind).

```go
// siblingConsensus returns the parent that ALL siblings of id's EEA order
// group unanimously point to — or "", if the group is contradictory or
// empty. The group is the id without its last letter ("NAR-01E" ->
// "NAR-01").
//
// Measured over all 287 EEA order groups: 285 consistent, one
// contradictory (QUI-01, without an orphan), one with no parent set at
// all (THE-01, whose orphan the alt-code join resolves). The derivation
// is thus the tightest rule that closes the ten open cases without
// guessing.
func siblingConsensus(id string, parents map[string]string) string {
	group, ok := eeaGroup(id)
	if !ok {
		return ""
	}
	var found string
	for other, parent := range parents {
		if other == id || parent == "" {
			continue
		}
		if g, ok := eeaGroup(other); !ok || g != group {
			continue
		}
		if found == "" {
			found = parent
			continue
		}
		if found != parent {
			return ""
		}
	}
	return found
}

// eeaGroup strips the alliance letter off an EEA id. Only an id of the
// form XXX-NNL has a group; anything else has none, and then nothing
// gets derived.
func eeaGroup(id string) (string, bool) {
	if len(id) < 2 {
		return "", false
	}
	last, prev := id[len(id)-1], id[len(id)-2]
	if last < 'A' || last > 'Z' || prev < '0' || prev > '9' {
		return "", false
	}
	return id[:len(id)-1], true
}
```

`assignRemainingParents` läuft über die in Schritt 3 geschriebenen EEA-eigenen Zeilen:
1. `longestPrefixMatch(name, allianceRows)` — Treffer setzt `SetSyntaxonParent(id, match.parentCode, domain.ParentProvenanceOfficial)` und zählt `ParentsByName`. Mehrdeutigkeit wird in `AmbiguousMatches` vermerkt und **nicht** geraten.
2. Sonst `siblingConsensus` — Treffer setzt `SetSyntaxonParent(id, parent, domain.ParentProvenanceDerived)` und zählt `ParentsDerived`.
3. Sonst bleibt die Zeile Waise und wird in `Orphans` vermerkt.

Die Karte, die `siblingConsensus` bekommt, bildet **EEA-ID → Elternteil** und entsteht aus den Hierarchiezeilen mit Altcode (`altCode` → `parentCode`), ergänzt um die bereits in diesem Schritt gesetzten. Danach:

```go
	// Any non-formation row without a parent fails the ingest. An
	// information service whose navigation chain breaks at one point is
	// worthless at exactly that point — that must not be a warning one
	// skims past.
	if len(rep.Orphans) > 0 {
		sort.Strings(rep.Orphans)
		return fmt.Errorf("%d syntaxa have no parent after every step: %s",
			len(rep.Orphans), strings.Join(rep.Orphans, ", "))
	}
```

Am Ende von `writeSyntaxa` zusätzlich der idempotente Nachlauf: für jeden Altcode, der im Index noch als eigene Syntaxon-ID Kanten trägt (Wiederholungslauf auf einem gefüllten Index), `tx.RelinkSyntaxon(altCode, primaryCode)`. Dafür `repo.SyntaxonIDsByAltCode` vor `Begin` lesen und die IDs vergleichen; ein leerer Index liefert eine leere Karte und der Nachlauf tut nichts.

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./internal/application/ -v`
Expected: PASS für alle `TestIngestSyntaxa*`. Andere Tests des Pakets, die `IngestSyntaxaHierarchy` aufrufen, müssen jetzt entfernt oder umgestellt sein (die Datei wurde in Task 5 gelöscht).

- [ ] **Step 5: Commit**

```bash
git add internal/application/
git commit -m "feat(ingest): Restelternteile per Namensabgleich und Geschwisterkonsens, Waisen brechen ab"
```

---

### Task 8: Verdrahtung, Read-API-Felder und OpenAPI

**Files:**
- Modify: `internal/application/ingest.go` (Syntaxa-Schritte aus `ingestAll` entfernen, `IngestReport` kürzen)
- Modify: `cmd/situs/ingest.go` (Helfer extrahieren, dann Phase einhängen)
- Modify: `internal/ports/input/services.go`
- Modify: `internal/application/query.go` (`syntaxaOf`)
- Modify: `internal/adapters/sqlite/read_syntaxon.go` (falls in Task 4 noch nicht alle SELECTs erweitert)
- Modify: `internal/adapters/http/openapi.yaml`, `api/openapi/openapi.yaml`
- Test: `internal/application/ingest_test.go`, `internal/adapters/http/habitat_test.go`, `cmd/situs/ingest_test.go`

**Vier Stellen, die leicht übersehen werden:**

1. **`runIngest` darf nicht wachsen.** Seine Baseline im
   Funktionskomplexitäts-Ratchet ist 12; eine weitere Phase mit ihrem
   `if err != nil` schiebt sie auf 13 und das Gate bricht. Also **zuerst**
   einen Helfer extrahieren, dann die Phase einhängen. Muster sind
   `ingestLocalOverlays` und `sealIndex` in derselben Datei: eine Funktion, die
   mehrere zusammengehörige Phasen bündelt und einen Report zurückgibt. Hier
   passt `ingestSyntaxaPhase(ctx, db, csvDir) (application.SyntaxaReport, error)`
   zusammen mit dem Protokollieren der Warnungen — damit verliert `runIngest`
   die Verzweigungen für `AltCodeCollisions`, `AmbiguousMatches`,
   `UnknownLinkTargets` und `ParentsDerived`, statt sie zu gewinnen.
2. **`IngestReport` verliert zwei Felder.** Der Struct ist in
   `cmd/situs/ingest.go` in `ingestOutput` **eingebettet**, seine Felder sind
   also Schlüssel der ersten Ebene im ausgegebenen JSON. Mit `Syntaxa` und
   `SyntaxonLinks` verschwinden zwei Schlüssel; Tests, die auf `rep.Syntaxa`
   prüfen, wandern zu `SyntaxaReport`, und `docs/reference/measured-index.md`
   zeigt den Report wörtlich (Task 9).
3. **Alle Syntaxon-SELECTs brauchen die vier Spalten.**
   `internal/adapters/sqlite/read_syntaxon.go` führt drei explizite
   Spaltenlisten (`Syntaxon`, `Syntaxa`, `AllSyntaxa`), jede mit eigenem
   `Scan`. Fehlt eine, bleiben die Felder stumm leer und die API verschweigt,
   was der Index weiß.
4. **`AllSyntaxa` bleibt, `UpsertSyntaxonAuthor` geht.** Der Namensabgleich
   für die 16 EEA-eigenen Einheiten liest weiter alle Syntaxa, `AllSyntaxa`
   behält also seinen Aufrufer. `UpsertSyntaxonAuthor` dagegen entfällt (Task
   4): seine Zusage „leeres `parentID` lässt `parent_id` unberührt"
   widerspricht der lückenlosen Kette direkt.

**Interfaces:**
- Consumes: `application.IngestSyntaxa`, `application.SyntaxaReport` (Task 5–7).
- Produces: `input.SyntaxonRef` um `AltCode`, `Source`, `ParentProvenance` erweitert (alle `omitempty`); `IngestCSV` ohne Syntaxa-Schritte.

- [ ] **Step 1: Die failing Tests schreiben**

In `internal/application/ingest_test.go`:

```go
func TestIngestCSVSchreibtKeineSyntaxaMehr(t *testing.T) {
	repo := newFakeRepo()
	dir := t.TempDir()
	writeCSV(t, dir, "typologies.csv", "id,scheme,version,name,source_ref\neunis@2021,eunis,2021,EUNIS,\n")
	writeCSV(t, dir, "habitat_types.csv",
		"typology_id,code,level,name_en,parent_code,priority\neunis@2021,T1,1,Wald,,\n")
	writeCSV(t, dir, "crosswalks.csv",
		"from_typology,from_code,to_typology,to_code,qualifier\n")
	// The files are present but must no longer be read by IngestCSV.
	writeCSV(t, dir, "syntaxa.csv", "id,rank,name,parent_id\nX-01A,alliance,Darf nicht rein,\n")
	writeCSV(t, dir, "habitat_type_syntaxa.csv", "typology_id,code,syntaxon_id\neunis@2021,T1,X-01A\n")

	if _, err := application.IngestCSV(context.Background(), repo, dir); err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}
	if repo.has("X-01A") {
		t.Error("IngestCSV hat ein Syntaxon geschrieben")
	}
	if len(repo.links) != 0 {
		t.Errorf("IngestCSV hat %d Kanten geschrieben", len(repo.links))
	}
}
```

In `internal/adapters/http/habitat_test.go` (Muster: der bestehende Test für `author`/`parent_id` im JSON):

```go
func TestHabitatTypeJSONTraegtSyntaxonHerkunftsfelder(t *testing.T) {
	// Fixture syntaxon with alt code, source, and a derived parent.
	body := getJSON(t, srv, "/v1/habitat-type/eunis@2021/T11")
	syn := body["syntaxa"].([]any)[0].(map[string]any)
	for field, want := range map[string]any{
		"alt_code":          "TST-01A",
		"source":            "evc",
		"parent_provenance": "official",
	} {
		if got := syn[field]; got != want {
			t.Errorf("%s = %v, erwartet %v", field, got, want)
		}
	}
}

func TestHabitatTypeJSONLaesstLeereHerkunftsfelderWeg(t *testing.T) {
	body := getJSON(t, srv, "/v1/habitat-type/eunis@2021/T12")
	syn := body["syntaxa"].([]any)[0].(map[string]any)
	if _, ok := syn["alt_code"]; ok {
		t.Error("alt_code erscheint, obwohl leer")
	}
}
```

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/application/ -run TestIngestCSVSchreibtKeineSyntaxa -v && go test ./internal/adapters/http/ -run TestHabitatTypeJSONTraegtSyntaxon -v`
Expected: FAIL — `IngestCSV` schreibt das Syntaxon noch; die drei JSON-Felder fehlen.

- [ ] **Step 3: Implementieren**

In `internal/application/ingest.go`: aus `ingestAll` die Aufrufe von `ingestSyntaxa` und `ingestSyntaxonLinks` entfernen, die beiden Funktionen selbst löschen (sie leben jetzt als `writeEunisOnly`/`writeLinks` in `syntaxa_ingest.go`) und den Report-Struct um die Syntaxa-Zähler kürzen. Der Kommentar über `ingestAll` muss die neue Aufteilung nennen: Typologien, Habitattypen und Crosswalks hängen nicht an Syntaxa — es gibt keine Fremdschlüssel —, also ist die Trennung folgenlos für die Transaktionsgrenze.

In `cmd/situs/ingest.go` den Aufruf von `IngestSyntaxaHierarchy` ersetzen:

```go
	// Runs after IngestCSV (the habitat types must be in the index for
	// the edge check) and before the localization steps. Unlike before,
	// the hierarchy is not an optional addition: without it the index
	// ends up with no syntaxa hierarchy at all, so the ingest fails
	// instead of shipping it.
	syntaxa, err := application.IngestSyntaxa(ctx, db, csvDir)
	if err != nil {
		return fmt.Errorf("ingesting syntaxa from %q: %w", csvDir, err)
	}
```

und die Ausgabe des Reports entsprechend erweitern (dem bestehenden Protokollmuster der Funktion folgen, inklusive `AltCodeCollisions`, `AmbiguousMatches`, `UnknownLinkTargets` und `ParentsDerived` als Warnungen, wenn nichtleer).

In `internal/ports/input/services.go`:

```go
type SyntaxonRef struct {
	ID       string `json:"id"`
	Rank     string `json:"rank"`
	Name     string `json:"name"`
	Author   string `json:"author,omitempty"`
	ParentID string `json:"parent_id,omitempty"`

	// AltCode is the EEA-EUNIS code of the same syntaxon — a client
	// holding an old code can switch over with it, without guessing.
	AltCode string `json:"alt_code,omitempty"`
	// Source is "evc" or "eunis": which source this row carries.
	Source string `json:"source,omitempty"`
	// ParentProvenance is "official" or "derived". A derived parent is
	// never presented as a source-backed statement.
	ParentProvenance string `json:"parent_provenance,omitempty"`
	// LifeFormGroup is only filled on formation rows, so empty in every
	// response that exists today. The field is here already because
	// sub-project B delivers the formations as []SyntaxonRef and filters
	// on it.
	LifeFormGroup string `json:"life_form_group,omitempty"`
}
```

In `internal/application/query.go`, in `syntaxaOf`, die drei Felder mitmappen.

In **beiden** OpenAPI-Kopien das `SyntaxonRef`-Schema erweitern:

```yaml
    SyntaxonRef:
      type: object
      required: [id, rank, name]
      properties:
        id:
          type: string
        rank:
          type: string
          enum: [formation, class, order, alliance]
        name:
          type: string
        author:
          type: string
          description: >-
            Autorschafts-Zitat, direkt aus FloraVeg.EU übernommen; fehlt für
            die Einheiten, die nur die EEA-Quelle führt.
        parent_id:
          type: string
          description: >-
            Verweist auf den übergeordneten Syntaxon (reine String-Referenz,
            kein Fremdschlüssel). Fehlt nur bei einer Formation — sie ist die
            Wurzel der Hierarchie.
        alt_code:
          type: string
          description: >-
            Der EEA-EUNIS-Code desselben Syntaxons; fehlt bei Formationen und
            bei Einheiten, die nur eine der beiden Quellen kennt.
        source:
          type: string
          enum: [evc, eunis]
          description: >-
            Welche Quelle diese Zeile führt: die EuroVegChecklist (evc) oder
            allein die EEA-EUNIS-Zuordnung (eunis).
        parent_provenance:
          type: string
          enum: [official, derived]
          description: >-
            Ob parent_id aus der Quelle stammt (official) oder aus dem
            Geschwisterkonsens abgeleitet wurde (derived).
        life_form_group:
          type: string
          enum: [phanerogam, bryophyte_lichen, algae]
          description: >-
            Die Lebensform-Gruppe der Formation. Nur auf Formationszeilen
            gesetzt und deshalb in Antworten, die Verbände oder Ordnungen
            tragen, nicht vorhanden.
```

Die Beschreibung von `author` lautete „fehlt, wenn kein FloraVeg-Treffer
gefunden wurde" — das wird mit der Quellenumkehr falsch: es gibt keinen
Namensabgleich mehr, der scheitern könnte. Neue Fassung: „fehlt bei den
Einheiten, die nur die EEA-EUNIS-Quelle führt".

Danach die Byte-Gleichheit sicherstellen:

```bash
cp internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
diff internal/adapters/http/openapi.yaml api/openapi/openapi.yaml && echo "identisch"
```

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./... `
Expected: PASS für das ganze Modul.

- [ ] **Step 5: `make verify` laufen lassen**

Run: `make verify`
Expected: `verify passed.` Fällt ein Coverage-Floor (`internal/application` steht auf 100 %), fehlende Testfälle ergänzen — **nicht** den Floor senken.

- [ ] **Step 6: Commit**

```bash
git add internal/ cmd/ api/
git commit -m "feat(ingest,api): IngestSyntaxa verdrahten, Herkunftsfelder ausliefern"
```

---

### Task 9: Gegen den echten Index messen und dokumentieren

**Files:**
- Modify: `docs/reference/measured-index.md`
- Modify: `CLAUDE.md`
- Modify: `docs/reference/http-api.md`
- Test: `internal/adapters/sqlite/hierarchy_integrity_test.go` (neu)

**Interfaces:**
- Consumes: alles aus Task 1–8.
- Produces: ein Integritätstest über einen gebauten Index; aktualisierte Referenzdokumentation.

- [ ] **Step 1: Den Integritätstest schreiben**

`internal/adapters/sqlite/hierarchy_integrity_test.go` — er baut einen Index aus Fixtures (kein Verlass auf ein Artefakt im Arbeitsverzeichnis, sonst ist der Test in CI wertlos) und prüft die zentrale Zusage:

```go
// The promise from the spec as a test: no row except the formations is
// without a parent, and every one reaches a formation in at most three
// steps.
func TestHierarchieHatKeineWaisen(t *testing.T) {
	db := openTestDB(t)
	seedHierarchy(t, db) // Formation C, class CA, order CA01, alliance CA01A,
	                     // plus one EEA-only row with a derived parent

	rows, err := db.QueryContext(context.Background(),
		`SELECT id FROM syntaxon WHERE rank <> 'formation' AND parent_id = ''`)
	if err != nil {
		t.Fatalf("Abfrage: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var orphans []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		orphans = append(orphans, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("elternlose Nicht-Formationen: %v", orphans)
	}
}

func TestJedeZeileErreichtEineFormation(t *testing.T) {
	db := openTestDB(t)
	seedHierarchy(t, db)

	all, err := db.AllSyntaxa(context.Background())
	if err != nil {
		t.Fatalf("AllSyntaxa: %v", err)
	}
	byID := map[string]domain.Syntaxon{}
	for _, s := range all {
		byID[s.ID] = s
	}
	for _, s := range all {
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
```

- [ ] **Step 2: Test laufen lassen**

Run: `go test ./internal/adapters/sqlite/ -run 'TestHierarchie|TestJedeZeile' -v`
Expected: PASS (nach Task 3–7 ist die Logik da; schlägt er fehl, liegt ein echter Defekt vor — melden, nicht den Test aufweichen).

- [ ] **Step 3: Vollen Ingest fahren und messen**

Run:
```bash
make ingest-input
go run ./cmd/situs ingest --csv-dir out/ingest-input --db out/situs-neu.sqlite
sqlite3 out/situs-neu.sqlite "
SELECT rank, COUNT(*) FROM syntaxon GROUP BY 1 ORDER BY 1;
SELECT 'Waisen', COUNT(*) FROM syntaxon WHERE rank<>'formation' AND parent_id='';
SELECT 'derived', COUNT(*) FROM syntaxon WHERE parent_provenance='derived';
SELECT 'source=eunis', COUNT(*) FROM syntaxon WHERE source='eunis';
SELECT 'AMM-02B-Nachfolger', id, parent_id FROM syntaxon WHERE alt_code='AMM-02B';
SELECT 'Kryptogamen-Verbaende', COUNT(*) FROM syntaxon a
  JOIN syntaxon o ON o.id=a.parent_id JOIN syntaxon c ON c.id=o.parent_id
  WHERE a.rank='alliance' AND c.parent_id IN ('R','S','T','U','V','W','X','Y');
"
```
Expected: `formation 25`, `class 150`, `order 381`, `alliance 1326`; `Waisen 0`; `derived 10`; `source=eunis 16`; der `AMM-02B`-Nachfolger trägt `parent_id = JD02` (**nicht** `JE01`); `Kryptogamen-Verbaende 190`.

Weicht eine Zahl ab: anhalten und melden. Die Erwartungen stammen aus einer Messung, nicht aus einer Schätzung.

- [ ] **Step 4: Kantenzahl vor und nach der Umkehrung vergleichen**

Run:
```bash
sqlite3 situs.sqlite "SELECT COUNT(*) FROM habitat_type_syntaxon;"
sqlite3 out/situs-neu.sqlite "SELECT COUNT(*) FROM habitat_type_syntaxon;"
```
Expected: Beide Zahlen sind gleich. Ist die neue kleiner, ist eine EUNIS-Zuordnung verloren gegangen — das verletzt eine prüfbare Zusage des Specs und muss gemeldet werden. (Genau eine Ausnahme ist zulässig: die `U36`-Kante auf `ASP-03` und die auf `KC03` fallen zusammen, falls die Quelle beide führt. Prüfen mit `SELECT COUNT(*) FROM habitat_type_syntaxon WHERE code='U36';` — vorher 15, nachher 15, weil `ASP-03` auf `KC03` abgebildet wird und `KC03` vorher nicht verlinkt war.)

- [ ] **Step 5: Referenzdokumentation aktualisieren**

In `docs/reference/measured-index.md`:
- Die Syntaxa-Zahlen auf die neuen Messwerte bringen: 25 Formationen, 150
  Klassen, 381 Ordnungen, 1326 Verbände, 0 Waisen, 10 abgeleitete
  Elternteile, 16 EEA-eigene Zeilen, 190 Kryptogamen-Verbände. Die Datei
  führt heute eine Zeile `| Syntaxa | **1050** |` — sie wird ersetzt, nicht
  ergänzt.
- Den **wörtlich abgedruckten Ingest-Report** anpassen: mit dem Wegfall von
  `Syntaxa` und `SyntaxonLinks` aus `IngestReport` verschwinden zwei
  Schlüssel der ersten Ebene, und die Syntaxa-Zahlen stehen künftig im
  eigenen `SyntaxaReport`-Block.
- Die Herkunft jeder Zahl (die Abfrage) mitschreiben, wie es die Datei tut.

In `docs/reference/http-api.md` die drei neuen `SyntaxonRef`-Felder dokumentieren und das erweiterte `rank`-Enum.

In `CLAUDE.md` zusätzlich die Architektur-Liste der Pipelines um die neue
Datendatei ergänzen (`data/syntaxa_formations.csv` als Primärquelle neben
`data/localizations-de-situs.csv`) und im Abschnitt „Invariants that reviewers
must check" die neue Zusage aufnehmen: *jede Nicht-Formations-Zeile hat ein
Elternteil, sonst scheitert der Ingest* — sie steht gleichrangig neben der
Dreiwertigkeit von `in_area`.

In `CLAUDE.md` den Abschnitt „Current State" ergänzen: die Syntaxa-Quellenumkehr ist umgesetzt, FloraVeg ist Primärquelle, der Join läuft über den Altcode, die Formationsebene und die Lebensform-Gruppe sind ableitbar, und das Spec vom 2026-08-30 ist durch das vom 2026-09-21 revidiert. In der Tabelle der Design-Dokumente die drei neuen Specs eintragen. Den Satz „Assoziations-Ebene bleibt außerhalb" unverändert lassen — er gilt weiter.

- [ ] **Step 6: `make verify` und Mutationsgate**

Run: `make verify && make mutation && make codecharta`
Expected: alle drei grün.

**Zum Komplexitäts-Ratchet:** `internal/application/ingest.go` verliert mit
den beiden Syntaxa-Lesern Komplexität; seine Baseline von 64 ist ein Deckel,
das Gate bliebe also auch ohne Änderung grün. Die Projektkonvention ist
trotzdem, die Baseline zu **senken**, wenn der Code kleiner wird (der
`_query_note` in `.codecharta-ratchet.json` hält genau das als Praxis fest).
Also: den gemessenen neuen Wert einsetzen, nicht 64 stehen lassen. Steht
`cmd/situs/ingest.go` nach der Helfer-Extraktion unter 12, ebenso senken. `make mutation` läuft ein Paket je Aufruf über `scripts/mutation-gate.sh`; **niemals** gremlins mit `...` aufrufen (erzeugt still null Mutanten). Für ein neues Paket muss `.mutation-thresholds` einen Eintrag bekommen, sonst scheitert das Gate an „keine Mutanten" — dieses Teilprojekt legt kein neues Go-Paket an, also bleibt die Datei unverändert, solange die Schwellen halten. Steigt die Mutations-Punktzahl, den Schwellwert anheben (Raise-Only-Ratchet).

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/sqlite/hierarchy_integrity_test.go docs/ CLAUDE.md
git commit -m "test,docs: Hierarchie-Integritaet absichern und gemessene Werte festhalten"
```

---

## Self-Review

**Spec-Abdeckung.** Abschnitt 1 (Anlass) → Referenzmesswerte und Task 9, Step 3. Abschnitt 2 (Formation im Klassencode) → Task 2 und `formationOf` in Task 5. Abschnitt 3 (Datenfluss, Ingest-Umbau, Dateibereitstellung) → Task 2, Step 3 und Task 8, Step 3. Abschnitt 4 (Domäne) → Task 3, Step 8. Abschnitt 5 (Schema) → Task 3, Step 3 und 7. Abschnitt 6 (Pipeline, inkl. `read_sheet`-Härtung) → Task 1. Abschnitt 7 (Ingest, Geschwisterkonsens, ASP-03) → Task 5–7, ASP-03 als Test in Task 6, Step 1. Abschnitt 8 (Fehlerbehandlung) → alle sieben Zeilen der Tabelle haben einen Test: fehlende Formationsdatei und fehlende Hierarchiedatei (Task 5), fehlende `alt_code`-Spalte (Task 5), Altcode-Kollision (Task 1, Report), EEA-Einheit ohne Gegenstück (Task 6), unbekannter Buchstabe (Task 5), verbleibende Waise (Task 7). Abschnitt 9 (Read-API) → Task 8. Abschnitt 10 (Tests) → über alle Tasks verteilt, der Index-Test in Task 9. Abschnitt 11 (Zusagen) → Task 9, Step 3 und 4.

**Platzhalter.** Keine „TBD"/„TODO"; jeder Code-Schritt trägt den Code; die Fehlerbehandlung ist pro Fall benannt statt als „angemessene Fehlerbehandlung".

**Typkonsistenz.** `SyntaxaReport` (Task 5) trägt genau die Felder, die Task 6 (`EunisOnly`, `LinksWritten`, `LinksRemapped`, `UnknownLinkTargets`), Task 7 (`ParentsByName`, `ParentsDerived`, `Orphans`, `AmbiguousMatches`) und Task 8 (Protokollausgabe) verwenden. `SetSyntaxonParent(id, parentID, provenance string)` und `RelinkSyntaxon(from, to string)` sind in Task 4 definiert und in Task 6/7 mit derselben Signatur aufgerufen. `SyntaxonIDsByAltCode` liefert Altcode→ID und wird in Task 7, Step 3 so gelesen. Die Konstantennamen aus Task 3 werden in Task 5–8 unverändert benutzt.

**Ein bewusster Bruch in der Mitte.** Nach Task 4 kompiliert `internal/application` nicht, weil `UpsertSyntaxonAuthor` entfällt; Task 5 stellt den Zustand wieder her. Task 4, Step 5 sagt das ausdrücklich und begrenzt den Testlauf auf die zwei betroffenen Pakete. Die Alternative — Port und Anwendungsschicht in einem Task — wäre ein Task, den ein Prüfer nicht mehr in Teilen ablehnen kann.
