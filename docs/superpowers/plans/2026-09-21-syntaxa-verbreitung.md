# Syntaxa-Verbreitung — Implementierungsplan (Teilprojekt C)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Zu jedem Verband im Index steht, in welchen von 136 europäischen Territorien er belegt (`verified`) oder möglicherweise (`uncertain`) vorkommt — und, streng davon getrennt, für welche Verbände die Quelle überhaupt eine Aussage macht. Eine Exkursions-App fragt nicht „kommt das irgendwo in Europa vor", sondern „kommt das *hier* vor".

**Architecture:** Eine neue Pipeline `pipelines/evc-distribution/` (Python, nur Stdlib) liest das Blatt „European alliances" der gepinnten Zenodo-XLSX **über das `r`-Attribut jeder Zelle**, übersetzt `1`/`U` in `verified`/`uncertain` und schreibt drei CSVs plus `report.json`. Die 136 Territorien ziehen als **zweites `area_scheme`** (`evc_territory`) in die bestehende `area`-Tabelle; dafür wird `IngestAreas` von „genau ein Schema" auf „die Menge der bekannten Schemata" umgestellt. Zwei neue Tabellen in `schema.sql` trennen Vorkommen (`syntaxon_distribution`) von Abdeckung (`syntaxon_distribution_coverage`) — ohne die zweite ist `absence` nicht von `unknown` zu unterscheiden. Die Leseseite ergänzt `SyntaxonDetail.distribution`, den Filter `?area=`/`?include=` auf `GET /v1/syntaxa`, den Parameter `?scheme=` auf `GET /v1/areas` und zwei gemessene Felder in `GET /v1/info`.

**Tech Stack:** Go 1.26 (stdlib, `modernc.org/sqlite`, `gorilla/mux`, `cobra`, `viper`), Python 3 **nur Stdlib** für die Pipeline.

**Spec:** `docs/superpowers/specs/2026-09-21-syntaxa-verbreitung-design.md`

**Setzt voraus:** Teilprojekt A (`2026-09-21-syntaxa-quellenumkehr.md`) — der Join läuft über die EVC-Primärcodes, die erst dort zur Syntaxon-ID werden. Und Teilprojekt B (`2026-09-21-syntaxa-navigation-design.md`) — `SyntaxonDetail`, `GET /v1/syntaxa` und `SyntaxaByRank` entstehen dort und werden hier erweitert, nicht angelegt.

## Global Constraints

- `make verify` muss vor jedem Commit grün sein (fmt-check, vet, lint, test, arch, debt, build).
- Null `//nolint`, null `#nosec`, null `TODO`/`FIXME`/`HACK`/`XXX` in Go-Dateien. `.debt-budget` steht auf 0.
- Coverage-Floors sind ein Raise-Only-Ratchet (`.coverage-floors`). Aktuell: `internal/application` **100.0 %**, `internal/domain` 100.0 %, `internal/adapters/sqlite` 96 %, `internal/adapters/http` 88 %, `cmd/situs` 74 %, `internal/ports/output` 66 %. Neuer Code in `internal/application` und `internal/domain` muss vollständig getestet sein, sonst fällt das Gate.
- SQL-Statements sind **statische Strings** mit `?`-Platzhaltern. Niemals Werte konkatenieren (gosec G201/G202 bricht den Build). Eine schemaabhängige Abfrage wird als **zwei Literale mit einer Verzweigung** geschrieben, nie als zusammengesetzter Tabellenname. `PRAGMA table_info(...)` wird pro Tabelle wörtlich ausgeschrieben.
- Keine neue direkte Abhängigkeit. `gomodguard_v2` bricht den Build, bis sie in `.golangci.yml` bewusst eingetragen ist. Dieses Teilprojekt braucht keine.
- Pipeline-Python: **nur Stdlib** (`zipfile`, `xml.etree.ElementTree`, `csv`, `json`, `re`, `argparse`, `os`, `sys`). **Kein `openpyxl`** — die dokumentierte Ausnahme gilt ausschließlich für die übernommenen Trait-Konverter in `pipelines/eive` und `pipelines/tichy`.
- **Komplexitäts-Ratchet (`.codecharta-ratchet.json`, `make codecharta`).** Der harte Deckel ist `function_complexity.default_cap = 10` — die komplexeste **Funktion** je Datei. `internal/application/ingest.go`, `query.go` und `localize.go` sitzen bereits bei **exakt 10**: jedes Wachstum ihrer schlimmsten Funktion bricht das Gate sofort. `cmd/situs/ingest.go` hat eine Baseline von **12** für `runIngest`. Der Deckel je Datei (`complexity.default_cap`) ist 50, Baselines: `internal/application/ingest.go` 64, `query.go` 72. Baselines darf man **senken**, Heraufsetzen braucht eine schriftliche Begründung. Praktische Folgen für diesen Plan: neue Funktionen werden von Anfang an klein geschnitten, die Leseseite bekommt eine **eigene Datei** statt `query.go` zu füllen, und `runIngest` gewinnt **keine** Verzweigung — die neue Phase hängt in dem bestehenden Helfer `ingestLocalOverlays`. Beachte: Teilprojekt A hängt dort schon eine Phase ein, C ist die zweite.
- OpenAPI liegt in zwei byte-identischen Kopien: `internal/adapters/http/openapi.yaml` und `api/openapi/openapi.yaml`. Der Vertragstest prüft Routen↔Spec in **beiden** Richtungen und schlägt bei einer Route ohne explizites `.Methods()` an.
- Fehlercodes sind genau drei: `INVALID_QUERY`, `NOT_FOUND`, `INTERNAL_ERROR`.
- Deutsch für `README.md` und Commit-Nachrichten; Code-Kommentare sparsam, englisch, und nur wo sie ein *Warum* erklären.
- Conventional Commits. `VERSION` und `CHANGELOG.md` gehören release-please — niemals händisch anfassen.

## Die Quelle (gepinnt)

```
Zenodo Record 11580949, Version 2.0 (2024-06-12), CC-BY 4.0
Distribution-maps-of-vegetation-alliances-in-Europe_ver-2_2024-06-12_database.xlsx
sha256 f395b2ce984148dd05d5ec661dfc616f668f6e20c43f3cfb0f1de0362b75665e
size  233454 Bytes
```

Zu zitieren sind **beide** Veröffentlichungen: Preislerová et al. (2022), *Applied Vegetation Science* 25: e12642 (die Originalveröffentlichung, die der Record nennt) und Preislerová et al. (2024), *Applied Vegetation Science* 27: e12766 (die das Blatt „Read me" der Fassung 2 als deren Bezug angibt). Bei CC-BY ist eine davon zu unterschlagen nicht bloß unhöflich.

Die 82-MB-ZIP mit den fertigen Kartenbildern braucht situs nicht.

## Referenzmesswerte (gemessen 2026-09-21 gegen die gepinnte Datei und `pipelines/eurovegchecklist/out/syntaxa_hierarchy.csv`)

Diese Zahlen sind die Sollwerte, gegen die die Tasks prüfen. Weicht eine ab: **anhalten und melden**, nicht die Erwartung anpassen.

| Größe | Wert |
|---|---|
| Blätter der Arbeitsmappe (in Mappenreihenfolge) | `Borja tabulka` (sheet1), **`European alliances`** (sheet2), `Read me` (sheet3) |
| Kopfzeilenbreite des Datenblatts | **144** Spalten = 4 Metaspalten + **136** Territorien + 4 Summenspalten |
| Metaspalten (Index 0–3) | `Code 1`, `Code 2`, `Name`, `Name with author citation` |
| Summenspalten (Index 140–143) | `Verified occurrences`, `Uncertain occurrences`, `All occurrences`, `% of uncertain occurrences` |
| Datenzeilen mit Verbandscode | **1115** (Zeilen `r="2"` bis `r="1116"`) |
| Trailing-**Summenzeilen** (Zeilen `r="1117"`–`r="1120"`) | **4**, erste Zelle genau die vier Summenspalten-Beschriftungen |
| Wertemenge aller 1115 × 136 Gebietszellen | **exakt** `{"", "1", "U"}` |
| belegte Zellen | **11528** = 9608 `verified` + 1920 `uncertain` |
| Zeilen ohne eine einzige belegte Zelle | **0** |
| Verbandscodes mit Rand-Leerzeichen in der Quelle | **3**: `'JD02B '`, `'JD02C '`, `'JE01B '` |
| Dubletten in `Code 1` bzw. `Code 2` | **0** / **0** |
| Territoriums-Slugs | 136 Spaltennamen → **136 eindeutige** Slugs, 0 Kollisionen |
| Join `Code 1` → FloraVeg-Verbandscode | **1114 / 1115**; genau `CI01E` fehlt (`Code 2` = `NAR-01E`, Name `Campanulo-Nardion`) |
| FloraVeg-Verbände **ohne** Verbreitungszeile | **196** von 1310 |
| davon Kryptogamen (Sektionen R–Y) | **190** = 137 Moos/Flechte (R 28, S 66, T 43) + **53** Algen (U 24, V 4, W 2, X 1, Y 22) |
| die übrigen 6 | `CT06A`, `DA12A`, `DA13A`, `DD01A`, `DD01B`, `DD01C` |
| Verbände im Index nach Teilprojekt A | **1326** → **212** ohne jede Verbreitungsaussage (196 + die 16 EEA-eigenen) |
| Fassungsangabe des Blatts „Read me" | „version 2 (2024-06-12), which matches the updated vegetation classification of EuroVegChecklist version 3 (2024-06-12)" |

**Zwei Korrekturen am Spec, gemessen:**

1. Spec Abschnitt 3 nennt „**51** der 53 Algenverbände" ohne Verbreitungszeile. Gemessen sind es **53 von 53** — kein einziger Algenverband trägt eine Zeile. Die Gesamtzahl 196 und die Kryptogamenzahl 190 stimmen dadurch weiterhin (137 + 53 + 6 = 196). Der Plan rechnet mit 53; `docs/reference/measured-index.md` hält die gemessene Zahl fest.
2. Spec Abschnitt 1 nennt nur die 136 Gebietsspalten und die 4 Summen**spalten**. Das Blatt trägt zusätzlich **4 Summen*zeilen*** am Ende. Positionell wie namentlich gelesen wären sie vier Pseudo-Verbände mit Zellwerten wie `133` und `36.666666666666664` — genau die Wertemenge, die Abschnitt 1 als Symptom einer Fehllesung beschreibt. Sie werden an derselben Beschriftungsmenge erkannt wie die Summenspalten.

---

### Task 1: Pipeline `pipelines/evc-distribution/` — Blattauswahl, Zellbezug, Slugs, Übersetzung

**Files:**
- Create: `pipelines/evc-distribution/xlsx_to_csv.py`
- Create: `pipelines/evc-distribution/test_xlsx_to_csv.py`

**Interfaces:**
- Consumes: nichts (erster Task). Die Pipeline ist eigenständig und liest keine andere Pipeline-Ausgabe.
- Produces (Python-Funktionen):
  - `col_index(ref: str) -> int` — Spaltenteil eines Zellbezugs (`"AB7"`) als 0-basierter Index.
  - `slugify(column_name: str) -> str` — Spaltenname → `area_code`.
  - `sheet_path(xlsx_path: str, sheet_name: str) -> str` — interner Zip-Pfad des Blatts mit genau diesem Namen; `SheetError`, wenn es fehlt.
  - `read_sheet(src: str, sheet_path: str) -> list[dict[int, str]]` — je Zeile eine Abbildung Spaltenindex → Zellinhalt, **keine** positionelle Liste.
  - `convert(xlsx_path: str, out_dir: str) -> dict` — schreibt drei CSVs und `report.json`, liefert das Report-Dict.
  - Ausnahmen `SheetError`, `HeaderError`, `CellValueError`, `SlugCollisionError`.
- Produces (Dateien):
  - `out/syntaxon_distribution.csv` — `syntaxon_id,area_scheme,area_code,occurrence`
  - `out/syntaxon_distribution_coverage.csv` — `syntaxon_id,area_scheme`
  - `out/evc_territories.csv` — `area_scheme,area_code,name_en`
  - `out/report.json`

**Die dritte CSV ist eine bewusste Abweichung vom Spec.** Abschnitt 4 nennt zwei Ausgaben, Abschnitt 3 verlangt aber eine Coverage-Tabelle, die „geprüft, kommt in keinem Territorium vor" trägt, und Abschnitt 7 nennt genau diesen Fall ausdrücklich gültig. Aus `syntaxon_distribution.csv` allein ist ein Verband mit 136 leeren Zellen nicht rekonstruierbar — er hat dort keine Zeile, genau wie ein Verband, der in der Quelle gar nicht steht. Heute ist die Menge leer (gemessen: 0 Zeilen ohne belegte Zelle), und genau deshalb darf die Unterscheidung nicht davon abhängen: Abschnitt 3 sagt das selbst. Die Coverage-Liste kommt also als eigene CSV aus der Pipeline, die sie messen kann, nicht aus einer Ableitung im Go-Ingest, die sie nicht messen kann.

- [ ] **Step 1: Die failing Tests schreiben**

`pipelines/evc-distribution/test_xlsx_to_csv.py` — baut ihre XLSX-Fixtures selbst, wie `pipelines/eurovegchecklist/test_xlsx_to_csv.py`. Die Fixture-Zellen sind **echte Musterzellen** aus der gepinnten Datei.

```python
# pipelines/evc-distribution/test_xlsx_to_csv.py
import csv
import json
import os
import tempfile
import unittest
import zipfile

from xlsx_to_csv import (
    CellValueError,
    HeaderError,
    SheetError,
    SlugCollisionError,
    col_index,
    convert,
    read_sheet,
    slugify,
)

SHEET = "European alliances"


def _esc(value):
    return value.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def _col_ref(ci):
    """0 -> A, 25 -> Z, 26 -> AA, 135 -> EF. The inverse of col_index, spelled
    out here so a fixture can name a column past Z at all."""
    ref = ""
    ci += 1
    while ci:
        ci, rem = divmod(ci - 1, 26)
        ref = chr(ord("A") + rem) + ref
    return ref


def make_workbook(path, rows, sheet_name=SHEET, extra_sheets=()):
    """Build a multi-sheet .xlsx on disk. rows is a list of dicts
    {column index: cell text} so a fixture can LEAVE OUT a cell the way Excel
    does — which is the whole point of the r-attribute reading."""
    sheets = [(sheet_name, rows)] + [(n, r) for n, r in extra_sheets]
    sheet_tags = "".join(
        f'<sheet name="{_esc(n)}" sheetId="{i + 1}" r:id="rId{i + 1}"/>'
        for i, (n, _) in enumerate(sheets)
    )
    workbook = (
        '<?xml version="1.0"?><workbook '
        'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
        'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
        f"<sheets>{sheet_tags}</sheets></workbook>"
    )
    rel_tags = "".join(
        f'<Relationship Id="rId{i + 1}" '
        'Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" '
        f'Target="worksheets/sheet{i + 1}.xml"/>'
        for i in range(len(sheets))
    )
    rels = (
        '<?xml version="1.0"?><Relationships '
        'xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        f"{rel_tags}</Relationships>"
    )
    with zipfile.ZipFile(path, "w") as z:
        z.writestr("xl/workbook.xml", workbook)
        z.writestr("xl/_rels/workbook.xml.rels", rels)
        z.writestr("xl/sharedStrings.xml", '<?xml version="1.0"?><sst></sst>')
        for i, (_, sheet_rows) in enumerate(sheets):
            body = ""
            for ri, cells in enumerate(sheet_rows, start=1):
                tags = "".join(
                    f'<c r="{_col_ref(ci)}{ri}" t="inlineStr"><is><t>{_esc(v)}</t></is></c>'
                    for ci, v in sorted(cells.items())
                )
                body += f'<row r="{ri}">{tags}</row>'
            z.writestr(
                f"xl/worksheets/sheet{i + 1}.xml",
                '<?xml version="1.0"?><worksheet '
                'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
                f"<sheetData>{body}</sheetData></worksheet>",
            )


def row(*pairs):
    return dict(pairs)


# The four meta columns and the four summary columns, spelled exactly as the
# pinned file spells them.
META = ["Code 1", "Code 2", "Name", "Name with author citation"]
SUMS = [
    "Verified occurrences",
    "Uncertain occurrences",
    "All occurrences",
    "% of uncertain occurrences",
]


def header(*territories):
    cells = {i: h for i, h in enumerate(META)}
    for i, t in enumerate(territories):
        cells[len(META) + i] = t
    for i, s in enumerate(SUMS):
        cells[len(META) + len(territories) + i] = s
    return cells


class ColIndexTest(unittest.TestCase):
    def test_single_letter_columns(self):
        self.assertEqual(col_index("A1"), 0)
        self.assertEqual(col_index("Z9"), 25)

    def test_two_letter_columns(self):
        self.assertEqual(col_index("AA1"), 26)
        self.assertEqual(col_index("AB7"), 27)

    def test_the_last_territory_column_of_the_pinned_file(self):
        # Column 140 (0-based 139) is "Ukraine Transcarpathian", the last
        # territory before the four summary columns.
        self.assertEqual(col_index("EJ1116"), 139)


class SlugifyTest(unittest.TestCase):
    def test_lowercases_and_hyphenates_a_space(self):
        self.assertEqual(slugify("Austria Alps"), "austria-alps")

    def test_hyphenates_the_coast_underscore(self):
        self.assertEqual(slugify("Albania_coast"), "albania-coast")

    def test_keeps_an_existing_hyphen(self):
        self.assertEqual(slugify("France Extra-Mediterranean"), "france-extra-mediterranean")

    def test_a_bare_country_is_its_own_slug(self):
        self.assertEqual(slugify("Armenia"), "armenia")


class ReadSheetTest(unittest.TestCase):
    def test_a_missing_cell_does_not_shift_the_following_ones(self):
        # THE test that makes positional reading fail. Excel omits the empty
        # cell at index 1; reading row.findall("c") in order would put "U" at
        # index 1 instead of 2.
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "gap.xlsx")
            make_workbook(xlsx, [row((0, "AB02C"), (2, "U"))])
            rows = read_sheet(xlsx, "xl/worksheets/sheet1.xml")
            self.assertEqual(rows[0], {0: "AB02C", 2: "U"})


class SheetSelectionTest(unittest.TestCase):
    def test_picks_the_named_sheet_not_the_first_one(self):
        # In the pinned file "Borja tabulka" is sheet1 and the data sheet is
        # sheet2. Taking the first non-legend sheet would parse the draft.
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "two.xlsx")
            make_workbook(
                xlsx,
                [header("Albania"), row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (4, "1"))],
                sheet_name="Borja tabulka",
                extra_sheets=[(SHEET, [header("Armenia"), row((0, "AB01A"), (1, "KOB-01A"), (2, "N"), (3, "N A"), (4, "1"))])],
            )
            self.assertEqual(sheet_path(xlsx, SHEET), "xl/worksheets/sheet2.xml")

    def test_a_missing_data_sheet_aborts_naming_it(self):
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "nodata.xlsx")
            make_workbook(xlsx, [header("Albania")], sheet_name="Read me")
            with self.assertRaises(SheetError) as ctx:
                convert(xlsx, tmp)
            self.assertIn(SHEET, str(ctx.exception))


class ConvertTest(unittest.TestCase):
    def _convert(self, rows, territories=("Albania", "Albania_coast", "Armenia")):
        tmp = tempfile.mkdtemp()
        self.addCleanup(lambda: None)
        xlsx = os.path.join(tmp, "dist.xlsx")
        make_workbook(xlsx, [header(*territories)] + list(rows))
        out_dir = os.path.join(tmp, "out")
        os.makedirs(out_dir)
        report = convert(xlsx, out_dir)
        return report, out_dir

    def _rows_of(self, out_dir, name):
        with open(os.path.join(out_dir, name), encoding="utf-8") as f:
            return list(csv.DictReader(f))

    def test_translates_the_cell_codes_into_the_long_form(self):
        report, out_dir = self._convert([
            row((0, "AB02C"), (1, "KOB-02C"), (2, "Festucion versicoloris"),
                (3, "Festucion versicoloris Krajina 1933"), (4, "1"), (6, "U")),
        ])
        got = self._rows_of(out_dir, "syntaxon_distribution.csv")
        self.assertEqual(got, [
            {"syntaxon_id": "AB02C", "area_scheme": "evc_territory",
             "area_code": "albania", "occurrence": "verified"},
            {"syntaxon_id": "AB02C", "area_scheme": "evc_territory",
             "area_code": "armenia", "occurrence": "uncertain"},
        ])
        self.assertEqual(report["verified"], 1)
        self.assertEqual(report["uncertain"], 1)

    def test_an_empty_cell_writes_no_row_at_all(self):
        _, out_dir = self._convert([
            row((0, "AA01A"), (1, "PAP-01A"), (2, "Papaverion dahliani"),
                (3, "Papaverion dahliani Hofmann ex Daniëls 2016"), (5, "1")),
        ])
        got = self._rows_of(out_dir, "syntaxon_distribution.csv")
        self.assertEqual([r["area_code"] for r in got], ["albania-coast"])

    def test_every_alliance_row_gets_a_coverage_row_even_with_no_occurrence(self):
        report, out_dir = self._convert([
            row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (4, "1")),
            row((0, "AB01A"), (1, "KOB-01A"), (2, "M"), (3, "M A")),
        ])
        got = self._rows_of(out_dir, "syntaxon_distribution_coverage.csv")
        self.assertEqual(got, [
            {"syntaxon_id": "AA01A", "area_scheme": "evc_territory"},
            {"syntaxon_id": "AB01A", "area_scheme": "evc_territory"},
        ])
        self.assertEqual(report["alliances"], 2)
        self.assertEqual(report["covered"], 2)

    def test_strips_a_trailing_blank_off_the_code(self):
        # Measured: 'JD02B ', 'JD02C ' and 'JE01B ' carry a trailing blank in
        # the pinned file. Unstripped they join with nothing.
        _, out_dir = self._convert([
            row((0, "JD02B "), (1, "AMM-02B"),
                (2, "Diantho attenuati-Scrophularion caninae"), (3, "x"), (4, "1")),
        ])
        got = self._rows_of(out_dir, "syntaxon_distribution.csv")
        self.assertEqual(got[0]["syntaxon_id"], "JD02B")

    def test_writes_one_territory_row_per_column_never_a_summary_column(self):
        report, out_dir = self._convert([
            row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"),
                (4, "1"), (7, "3"), (8, "0"), (9, "3"), (10, "0")),
        ])
        got = self._rows_of(out_dir, "evc_territories.csv")
        self.assertEqual([r["area_code"] for r in got], ["albania", "albania-coast", "armenia"])
        self.assertEqual([r["name_en"] for r in got],
                         ["Albania", "Albania_coast", "Armenia"])
        self.assertEqual(report["territories"], 3)
        # The four summary cells sit at 7..10 and must not become occurrences.
        self.assertEqual(report["verified"], 1)
        self.assertEqual(report["uncertain"], 0)

    def test_skips_the_four_trailing_summary_rows(self):
        report, out_dir = self._convert([
            row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (4, "1")),
            row((0, "Verified occurrences"), (4, "133")),
            row((0, "Uncertain occurrences"), (4, "77")),
            row((0, "All occurrences"), (4, "210")),
            row((0, "% of uncertain occurrences"), (4, "36.666666666666664")),
        ])
        self.assertEqual(report["alliances"], 1)
        self.assertEqual(report["summary_rows"], 4)
        self.assertEqual(report["skipped_rows"], 0)
        got = self._rows_of(out_dir, "syntaxon_distribution.csv")
        self.assertEqual([r["syntaxon_id"] for r in got], ["AA01A"])

    def test_an_unknown_cell_value_aborts_naming_value_row_and_column(self):
        with self.assertRaises(CellValueError) as ctx:
            self._convert([
                row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (5, "2")),
            ])
        message = str(ctx.exception)
        self.assertIn("'2'", message)
        self.assertIn("AA01A", message)
        self.assertIn("Albania_coast", message)

    def test_a_slug_collision_aborts_naming_both_columns(self):
        with self.assertRaises(SlugCollisionError) as ctx:
            self._convert(
                [row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"))],
                territories=("Austria Alps", "Austria_Alps"),
            )
        message = str(ctx.exception)
        self.assertIn("Austria Alps", message)
        self.assertIn("Austria_Alps", message)

    def test_a_missing_meta_column_raises_header_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "dist.xlsx")
            make_workbook(xlsx, [{0: "Code 1", 1: "Name"}])
            out_dir = os.path.join(tmp, "out")
            os.makedirs(out_dir)
            with self.assertRaises(HeaderError):
                convert(xlsx, out_dir)

    def test_the_report_carries_the_measured_value_histogram(self):
        report, _ = self._convert([
            row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (4, "1"), (5, "U")),
        ])
        self.assertEqual(report["value_histogram"], {"": 1, "1": 1, "U": 1})
        self.assertEqual(report["slug_collisions"], [])


if __name__ == "__main__":
    unittest.main()
```

Die Importzeile führt `sheet_path` noch nicht — `SheetSelectionTest` benutzt es. Ergänze es in der Importliste (alphabetisch nach `read_sheet`).

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `cd pipelines/evc-distribution && python3 -m unittest discover -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'xlsx_to_csv'`.

- [ ] **Step 3: Implementieren**

`pipelines/evc-distribution/xlsx_to_csv.py`:

```python
#!/usr/bin/env python3
"""Convert the pinned EVC alliance distribution XLSX into three CSVs.

Source: Zenodo record 11580949, version 2.0 (2024-06-12), CC-BY 4.0 —
"Distribution maps of vegetation alliances in Europe". Cite BOTH
Preislerová et al. (2022) Appl Veg Sci 25: e12642 and Preislerová et al.
(2024) Appl Veg Sci 27: e12766; see manifest.yaml.

Same rationale as pipelines/eunis/xlsx_to_csv.py: an .xlsx is a zip of XML the
stdlib reads, so no spreadsheet library joins situs' dependency list.

TWO things this parser must get right, both measured rather than assumed:

1. Cells are read by their OWN r-attribute, never by position. Excel omits
   empty cells, and with 136 mostly-empty territory columns per row a
   positional read is not merely imprecise: measured against the pinned file
   it yields the value set {"", "1", "U", "0", "2", ..., "86.111"} instead of
   the actual three values, because summary cells slide into territory
   columns.
2. The sheet is picked BY NAME. "Borja tabulka" is the workbook's FIRST sheet
   and is a working draft (country codes instead of territories, plus a Czech
   note about alliances still missing); taking the first non-legend sheet
   would parse it.

The 1 -> verified and U -> uncertain translation happens HERE, not in the Go
ingest: only the side that sees the raw cell can recognize an unknown value
and abort on it, and doing it twice would be two chances to disagree.
"""
import argparse
import csv
import json
import os
import re
import sys
import zipfile
import xml.etree.ElementTree as ET

NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}
_R_ID = "{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id"

SCHEME = "evc_territory"
DATA_SHEET = "European alliances"

CSV_HEADERS = {
    "syntaxon_distribution.csv": ["syntaxon_id", "area_scheme", "area_code", "occurrence"],
    "syntaxon_distribution_coverage.csv": ["syntaxon_id", "area_scheme"],
    "evc_territories.csv": ["area_scheme", "area_code", "name_en"],
}

# The four meta columns, spelled as the pinned file spells them. Code 2 is the
# EEA legacy code — kept in the required set so a format change that drops it
# fails loudly, even though the join runs on Code 1.
_META_HEADERS = ["Code 1", "Code 2", "Name", "Name with author citation"]

# The four summary labels. ONE constant, because the file uses the same four
# strings twice: as the last four column headers and as the first cell of the
# last four rows. Recognizing them in both places from one list is what keeps
# a renamed summary from being read as a territory in one place and a row in
# the other.
_SUMMARY_LABELS = [
    "Verified occurrences",
    "Uncertain occurrences",
    "All occurrences",
    "% of uncertain occurrences",
]

# The cell legend from the "Read me" sheet, verbatim: 1 = verified occurrence,
# U = uncertain occurrence, empty cell = absence. Absence is the ABSENCE of a
# row, never a row with an "absent" value.
_OCCURRENCE = {"1": "verified", "U": "uncertain"}

_ALLIANCE_RE = re.compile(r"^[A-Z]{2}[0-9]{2}[A-Z]$")


class SheetError(RuntimeError):
    """The named data sheet is not in the workbook. Never fall back to
    another one — the workbook's first sheet is a draft."""


class HeaderError(RuntimeError):
    """A meta column a parser needs is missing — fail loudly instead of
    silently defaulting every cell to empty."""


class CellValueError(RuntimeError):
    """A territory cell holds something other than "1", "U" or empty. Either
    the format changed or the cell reference is being read wrong, and both
    must stop the run rather than pass through as a silent mismapping."""


class SlugCollisionError(RuntimeError):
    """Two column names derive the same area_code. Picking a winner would
    silently merge two territories."""


def col_index(ref):
    """Turn the column part of a cell reference ("AB7") into a 0-based column
    index. Excel omits empty cells; without this a gap shifts every following
    column."""
    n = 0
    for ch in ref:
        if not ch.isalpha():
            break
        n = n * 26 + (ord(ch.upper()) - 64)
    return n - 1


def slugify(column_name):
    """Column name -> area_code: lowercase, spaces and underscores to
    hyphens, everything else unchanged. "Austria Alps" -> "austria-alps",
    "Albania_coast" -> "albania-coast". An existing hyphen
    ("France Extra-Mediterranean") stays put."""
    return column_name.strip().lower().replace(" ", "-").replace("_", "-")


def _shared_strings(zf):
    try:
        root = ET.fromstring(zf.read("xl/sharedStrings.xml"))
    except KeyError:
        return []
    return ["".join(t.text or "" for t in si.iter(f"{{{NS['m']}}}t"))
            for si in root.findall("m:si", NS)]


def _cell_text(c, shared):
    if c.get("t") == "inlineStr":
        return "".join(t.text or "" for t in c.iter(f"{{{NS['m']}}}t"))
    v = c.find("m:v", NS)
    if v is None or v.text is None:
        return ""
    if c.get("t") == "s":
        return shared[int(v.text)]
    return v.text


def sheet_path(xlsx_path, sheet_name):
    """Internal zip path of the sheet with exactly this name."""
    with zipfile.ZipFile(xlsx_path) as zf:
        wb = ET.fromstring(zf.read("xl/workbook.xml"))
        rels = ET.fromstring(zf.read("xl/_rels/workbook.xml.rels"))
    relmap = {r.get("Id"): r.get("Target") for r in rels}
    names = []
    for sheet in wb.iter(f"{{{NS['m']}}}sheet"):
        name = (sheet.get("name") or "").strip()
        names.append(name)
        if name == sheet_name:
            target = relmap.get(sheet.get(_R_ID))
            if target:
                return "xl/" + target
    raise SheetError(
        f"{xlsx_path}: no sheet named {sheet_name!r}; the workbook has {names!r}")


def read_sheet(src, path):
    """Return the sheet as a list of {column index: cell text} per row. NOT a
    list of positional lists: see the module docstring."""
    with zipfile.ZipFile(src) as zf:
        shared = _shared_strings(zf)
        root = ET.fromstring(zf.read(path))
    rows = []
    for row in root.iter(f"{{{NS['m']}}}row"):
        cells = {}
        for c in row.findall("m:c", NS):
            text = _cell_text(c, shared)
            if text != "":
                cells[col_index(c.get("r") or "A")] = text
        rows.append(cells)
    return rows


def _meta_index(head, xlsx_path):
    """Column index of each meta header, or HeaderError."""
    by_name = {name.strip(): ci for ci, name in head.items()}
    missing = [h for h in _META_HEADERS if h not in by_name]
    if missing:
        raise HeaderError(
            f"{xlsx_path} [{DATA_SHEET}]: missing required column(s) {missing}")
    return {h: by_name[h] for h in _META_HEADERS}


def _territories(head, meta, xlsx_path):
    """The territory columns: every header column that is neither a meta
    column nor a summary column, in column order. Derived rather than counted
    off a fixed offset, so a further territory needs no code change and a
    renamed summary column fails the slug check instead of being read as a
    territory."""
    meta_cols = set(meta.values())
    out = []
    seen = {}
    for ci in sorted(head):
        name = head[ci].strip()
        if ci in meta_cols or name in _SUMMARY_LABELS or name == "":
            continue
        code = slugify(name)
        if code in seen and seen[code] != name:
            raise SlugCollisionError(
                f"{xlsx_path} [{DATA_SHEET}]: columns {seen[code]!r} and "
                f"{name!r} both derive area_code {code!r}")
        seen[code] = name
        out.append((ci, code, name))
    return out


def _occurrence(raw, code, name, xlsx_path):
    try:
        return _OCCURRENCE[raw]
    except KeyError:
        raise CellValueError(
            f"{xlsx_path} [{DATA_SHEET}]: alliance {code!r}, territory "
            f"{name!r}: cell value {raw!r} is neither '1', 'U' nor empty"
        ) from None


def convert(xlsx_path, out_dir):
    rows = read_sheet(xlsx_path, sheet_path(xlsx_path, DATA_SHEET))
    if not rows:
        raise SheetError(f"{xlsx_path} [{DATA_SHEET}]: the sheet is empty")

    meta = _meta_index(rows[0], xlsx_path)
    territories = _territories(rows[0], meta, xlsx_path)
    code_col = meta["Code 1"]

    dist, coverage = [], []
    histogram = {}
    counts = {"verified": 0, "uncertain": 0}
    summary_rows, skipped = 0, []

    for row in rows[1:]:
        code = row.get(code_col, "").strip()
        if code == "":
            continue
        if code in _SUMMARY_LABELS:
            summary_rows += 1
            continue
        if not _ALLIANCE_RE.fullmatch(code):
            skipped.append(code)
            continue
        coverage.append({"syntaxon_id": code, "area_scheme": SCHEME})
        for ci, area_code, name in territories:
            raw = row.get(ci, "")
            histogram[raw] = histogram.get(raw, 0) + 1
            if raw == "":
                continue
            occurrence = _occurrence(raw, code, name, xlsx_path)
            counts[occurrence] += 1
            dist.append({"syntaxon_id": code, "area_scheme": SCHEME,
                         "area_code": area_code, "occurrence": occurrence})

    for code in skipped:
        print(f"skipped: {xlsx_path} [{DATA_SHEET}]: code {code!r} matches no "
              "alliance pattern", file=sys.stderr)

    _write(out_dir, "syntaxon_distribution.csv", dist)
    _write(out_dir, "syntaxon_distribution_coverage.csv", coverage)
    _write(out_dir, "evc_territories.csv",
           [{"area_scheme": SCHEME, "area_code": c, "name_en": n}
            for _, c, n in territories])

    report = {
        "alliances": len(coverage),
        "covered": len(coverage),
        "territories": len(territories),
        "verified": counts["verified"],
        "uncertain": counts["uncertain"],
        "written": len(dist),
        "slug_collisions": [],
        "summary_rows": summary_rows,
        "skipped_rows": len(skipped),
        "value_histogram": histogram,
    }
    with open(os.path.join(out_dir, "report.json"), "w", encoding="utf-8") as f:
        json.dump(report, f, indent=2, ensure_ascii=False, sort_keys=True)
        f.write("\n")
    return report


def _write(out_dir, name, rows):
    with open(os.path.join(out_dir, name), "w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=CSV_HEADERS[name], lineterminator="\n")
        w.writeheader()
        for r in rows:
            w.writerow(r)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--xlsx", required=True)
    parser.add_argument("--out-dir", required=True)
    args = parser.parse_args(argv)
    try:
        report = convert(args.xlsx, args.out_dir)
    except (SheetError, HeaderError, CellValueError, SlugCollisionError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        sys.exit(1)
    print(json.dumps(report, indent=2, ensure_ascii=False, sort_keys=True))


if __name__ == "__main__":
    main()
```

`slug_collisions` steht als leere Liste im Report, weil eine Kollision den Lauf abbricht — der Schlüssel bleibt trotzdem, damit der Report dieselbe Form hat, egal ob geprüft wurde. Das Spec führt ihn; ein fehlender Schlüssel wäre eine stillschweigende Auslassung.

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `cd pipelines/evc-distribution && python3 -m unittest discover -v`
Expected: PASS, alle Tests.

- [ ] **Step 5: Commit**

```bash
git add pipelines/evc-distribution/
git commit -m "feat(evc-distribution): Verbreitungstabelle ueber den Zellbezug in drei CSVs"
```

---

### Task 2: Gegen das echte Artefakt messen, Manifest, README, Verdrahtung

**Files:**
- Create: `pipelines/evc-distribution/manifest.yaml`
- Create: `pipelines/evc-distribution/README.md`
- Create: `pipelines/evc-distribution/build.sh`
- Modify: `Makefile` (`pipeline-test`)
- Modify: `scripts/collect-ingest-input.sh`

`.gitignore` braucht **keine** Änderung: die Muster `/pipelines/*/artifacts/` und `/pipelines/*/out/` (Zeile 26/27) greifen für das neue Verzeichnis schon.

**Interfaces:**
- Consumes: `convert` (Task 1).
- Produces: `out/syntaxon_distribution.csv`, `out/syntaxon_distribution_coverage.csv`, `out/evc_territories.csv` im Sammelverzeichnis unter denselben Namen; `make pipeline-test` deckt das neue Verzeichnis mit ab.

- [ ] **Step 1: Artefakt beschaffen und Prüfsumme vergleichen**

Run:
```bash
cd pipelines/evc-distribution && mkdir -p artifacts out && \
curl -sSL "https://zenodo.org/records/11580949/files/Distribution-maps-of-vegetation-alliances-in-Europe_ver-2_2024-06-12_database.xlsx?download=1" \
  -o artifacts/Distribution-maps-of-vegetation-alliances-in-Europe_ver-2_2024-06-12_database.xlsx && \
shasum -a 256 artifacts/*.xlsx && wc -c artifacts/*.xlsx
```
Expected: `f395b2ce984148dd05d5ec661dfc616f668f6e20c43f3cfb0f1de0362b75665e` und `233454`. Weicht eine der beiden ab, ist es **nicht** die gepinnte Fassung: anhalten und melden, nicht den Pin anpassen.

- [ ] **Step 2: Pipeline gegen das echte Artefakt laufen lassen**

Run:
```bash
cd pipelines/evc-distribution && python3 xlsx_to_csv.py \
  --xlsx artifacts/Distribution-maps-of-vegetation-alliances-in-Europe_ver-2_2024-06-12_database.xlsx \
  --out-dir out && cat out/report.json
```
Expected, jede Zahl:
```
alliances: 1115, covered: 1115, territories: 136,
verified: 9608, uncertain: 1920, written: 11528,
slug_collisions: [], summary_rows: 4, skipped_rows: 0,
value_histogram: {"": 140112, "1": 9608, "U": 1920}
```
Die drei Schlüssel des Histogramms sind der Kern der Zusage aus Abschnitt 9: die Wertemenge wird **gemessen**, nicht vorausgesetzt. Steht dort ein vierter Schlüssel, ist entweder das Format anders oder der Zellbezug wird falsch gelesen — anhalten und melden.

- [ ] **Step 3: Den Join gegen die FloraVeg-Codes messen**

Run:
```bash
python3 -c "
import csv
dist = {r['syntaxon_id'] for r in csv.DictReader(open('pipelines/evc-distribution/out/syntaxon_distribution_coverage.csv'))}
fv = {r['code'] for r in csv.DictReader(open('pipelines/eurovegchecklist/out/syntaxa_hierarchy.csv'))}
print('Verbreitungscodes:', len(dist))
print('ohne FloraVeg-Gegenstueck:', sorted(dist - fv))
print('Treffer:', len(dist & fv), '/', len(dist))
al = {r['code'] for r in csv.DictReader(open('pipelines/eurovegchecklist/out/syntaxa_hierarchy.csv')) if r['rank']=='alliance'}
ohne = al - dist
print('FloraVeg-Verbaende ohne Verbreitungszeile:', len(ohne))
import collections
print('davon je Sektion:', sorted(collections.Counter(c[0] for c in ohne).items()))
"
```
Expected: `Verbreitungscodes: 1115`, `ohne FloraVeg-Gegenstueck: ['CI01E']`, `Treffer: 1114 / 1115`, `FloraVeg-Verbaende ohne Verbreitungszeile: 196`, und je Sektion `[('C',1),('D',5),('R',28),('S',66),('T',43),('U',24),('V',4),('W',2),('X',1),('Y',22)]`.

`CI01E` ist **echter Fassungsdrift**: die Verbreitungsdatei nennt EVC-Fassung 3 (2024-06-12), die gepinnte Hierarchiedatei heißt `..._version_4.xlsx` und trägt in ihren Spaltenköpfen den Stand `EVC, version 2025-06-12`. Das sind zwei verschiedene Angaben — Dateifassung gegen EVC-Stand —, die nebeneinander genannt und nicht zu einer verrechnet werden. Der Verband heißt `Campanulo-Nardion` (`Code 2` = `NAR-01E`). Er wird in Task 6 namentlich gemeldet, nicht weggerundet.

- [ ] **Step 4: `build.sh` anlegen**

`pipelines/evc-distribution/build.sh` — Muster `pipelines/wgsrpd/build.sh`, mit gepinntem Dateinamen statt gepinntem Commit:

```bash
#!/usr/bin/env bash
# EVC alliance distribution -> syntaxon_distribution.csv + coverage + territories.
#
# Source: Zenodo record 11580949, version 2.0 (2024-06-12), CC-BY 4.0.
# Cite BOTH Preislerová et al. (2022) Appl Veg Sci 25: e12642 and
# Preislerová et al. (2024) Appl Veg Sci 27: e12766 — see manifest.yaml.
#
# Pinned to a VERSIONED record file, never to the record's "latest": the
# territory set and the alliance codes are the identity of what the index
# stores, and they must not change under a rebuild without somebody bumping
# this line.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

FILE="Distribution-maps-of-vegetation-alliances-in-Europe_ver-2_2024-06-12_database.xlsx"
SOURCE_URL="https://zenodo.org/records/11580949/files/${FILE}?download=1"
SHA256="f395b2ce984148dd05d5ec661dfc616f668f6e20c43f3cfb0f1de0362b75665e"

ART_DIR="${SCRIPT_DIR}/artifacts"
OUT_DIR="${SCRIPT_DIR}/out"
mkdir -p "${ART_DIR}" "${OUT_DIR}"
SRC_PATH="${ART_DIR}/${FILE}"

if [[ ! -f "${SRC_PATH}" ]]; then
  echo "evc-distribution: downloading ${SOURCE_URL}"
  curl -fsSL "${SOURCE_URL}" -o "${SRC_PATH}"
fi

# The checksum is verified on EVERY run, not only after a download: a cached
# artifact from a different record version would otherwise be converted while
# the run claims the pinned one.
if command -v shasum >/dev/null 2>&1; then
  echo "${SHA256}  ${SRC_PATH}" | shasum -a 256 -c -
else
  echo "${SHA256}  ${SRC_PATH}" | sha256sum -c -
fi

python3 "${SCRIPT_DIR}/xlsx_to_csv.py" --xlsx "${SRC_PATH}" --out-dir "${OUT_DIR}"

echo "evc-distribution: CSVs written to ${OUT_DIR}"
```

Run: `chmod +x pipelines/evc-distribution/build.sh && bash pipelines/evc-distribution/build.sh`
Expected: Erfolg, Prüfsumme bestätigt, derselbe Report wie in Step 2.

- [ ] **Step 5: `manifest.yaml` anlegen**

`pipelines/evc-distribution/manifest.yaml` — Klartext wie `pipelines/eurovegchecklist/manifest.yaml`, kein YAML-Parser nötig und keiner benutzt:

```yaml
# Pinned source artifact for the EVC alliance-distribution pipeline.
#
# Plain text like pipelines/eurovegchecklist/manifest.yaml — no YAML parser
# needed, no YAML parser used.
#
# Not committed: pipelines/evc-distribution/artifacts/ is gitignored. Run
# build.sh (or the curl command in README.md) to re-download.

sources:
  - id: evc-alliance-distribution-v2
    file: Distribution-maps-of-vegetation-alliances-in-Europe_ver-2_2024-06-12_database.xlsx
    description: >
      Occurrence of the 1115 European vegetation alliances across 136
      territories (states and biogeographical parts of them, most with their
      own coastal column). Cell values per the "Read me" sheet: 1 = verified
      occurrence, U = uncertain occurrence, empty cell = absence. Version 2
      matches, per that same sheet, EuroVegChecklist version 3 (2024-06-12).
      Only the database file is read; the 82 MB ZIP of rendered maps is not
      something situs needs.
    dataset_record: https://zenodo.org/records/11580949
    url: "https://zenodo.org/records/11580949/files/Distribution-maps-of-vegetation-alliances-in-Europe_ver-2_2024-06-12_database.xlsx?download=1"
    license: "CC-BY 4.0 — https://creativecommons.org/licenses/by/4.0/"
    # BOTH publications, deliberately. The record names the 2022 original; the
    # "Read me" sheet of version 2 names the 2024 paper as its reference.
    # Under CC-BY, dropping either one is not merely impolite.
    citation:
      - >
        Preislerová Z. et al. (2022) Distribution maps of vegetation
        alliances in Europe. Applied Vegetation Science 25: e12642.
        https://doi.org/10.1111/avsc.12642
      - >
        Preislerová Z. et al. (2024) Structural, ecological and
        biogeographical attributes of European vegetation alliances.
        Applied Vegetation Science 27: e12766.
        https://doi.org/10.1111/avsc.12766
    retrieved: "2026-09-21"
    sha256: f395b2ce984148dd05d5ec661dfc616f668f6e20c43f3cfb0f1de0362b75665e
    size_bytes: 233454
```

- [ ] **Step 6: `README.md` anlegen**

`pipelines/evc-distribution/README.md`, Muster `pipelines/eurovegchecklist/README.md`:

````markdown
# pipelines/evc-distribution — Verbreitung der Vegetationsverbände

Wandelt die gepinnte Zenodo-XLSX mit der Verbreitung der europäischen
Vegetationsverbände in drei CSVs um, die `situs ingest` liest (siehe
`docs/superpowers/specs/2026-09-21-syntaxa-verbreitung-design.md`).

**Nur Python-Stdlib**, wie `pipelines/eunis/` und
`pipelines/eurovegchecklist/`: `zipfile`, `xml.etree.ElementTree`, `csv`,
`json`, `re`, `argparse`. Python ≥ 3.9. Kein `openpyxl`.

## Quelle

[Zenodo Record 11580949](https://zenodo.org/records/11580949), Version 2.0
vom 2024-06-12, **CC-BY 4.0**. URL, SHA-256, Größe und **beide** zu
zitierenden Arbeiten stehen in [`manifest.yaml`](manifest.yaml):

- Preislerová et al. (2022) *Applied Vegetation Science* 25: e12642
- Preislerová et al. (2024) *Applied Vegetation Science* 27: e12766

Nicht eingecheckt (`artifacts/` ist gitignored).

## Lauf

```bash
bash pipelines/evc-distribution/build.sh
```

`build.sh` lädt bei Bedarf, prüft die SHA-256 **bei jedem Lauf** (ein
zwischengespeichertes Artefakt einer anderen Fassung würde sonst umgewandelt,
während der Lauf die gepinnte behauptet) und ruft die Umwandlung:

```
out/syntaxon_distribution.csv           syntaxon_id,area_scheme,area_code,occurrence
out/syntaxon_distribution_coverage.csv  syntaxon_id,area_scheme
out/evc_territories.csv                 area_scheme,area_code,name_en
out/report.json
```

Gemessen gegen die gepinnte Datei: **1115** Verbände, **136** Territorien,
**9608** `verified`, **1920** `uncertain`, **11528** Zeilen, 0 Slug-Kollisionen,
**4** übersprungene Summenzeilen, 0 sonstige übersprungene Zeilen, und das
`value_histogram` `{"": 140112, "1": 9608, "U": 1920}`.

## Drei Dinge, die gemessen und nicht angenommen sind

- **Die Zelle wird über ihr `r`-Attribut gelesen.** Excel lässt leere Zellen
  in der XML weg. Bei 136 überwiegend leeren Spalten je Zeile liefert eine
  positionelle Lesung gemessen die Wertemenge
  `{"", "1", "U", "0", "2", …, "86.111"}` statt der tatsächlichen drei Werte,
  weil Summenzellen in Gebietsspalten rutschen.
- **Das Blatt wird nach Namen gewählt.** `European alliances` ist das
  Datenblatt. `Borja tabulka` ist das **erste** Blatt der Mappe und ein
  Arbeitsentwurf (Ländercodes statt Territorien, dazu eine tschechische Notiz
  über noch fehlende Verbände); `Read me` trägt die Legende. Das erste
  Nicht-Legenden-Blatt zu nehmen würde den Entwurf einlesen.
- **Die vier Summen kommen zweimal vor**: als letzte vier Spalten *und* als
  letzte vier Zeilen. Beide werden an derselben Beschriftungsmenge erkannt.
  Ungeprüft wären die Zeilen vier Pseudo-Verbände mit Werten wie `133` und
  `36.666666666666664`.

## Vier Zustände, nicht drei

| Zustand | In der Quelle | Im Index |
|---|---|---|
| `verified` | Zelle `1` | Zeile in `syntaxon_distribution` |
| `uncertain` | Zelle `U` | Zeile in `syntaxon_distribution` |
| `absence` | leere Zelle, **Verband steht in der Tabelle** | keine Verbreitungszeile, **aber** eine Coverage-Zeile |
| `unknown` | **Verband steht nicht in der Tabelle** | keine Coverage-Zeile |

Deshalb die zweite CSV. Die Quelle deckt ausdrücklich nur
*vascular-plant dominated vegetation* ab: gemessen tragen **196** der 1310
FloraVeg-Verbände keine Verbreitungszeile, darunter alle **190** Kryptogamen-
Verbände (137 Moos/Flechte, 53 Algen). Für die ist jede Aussage über jedes
Territorium `unknown` und darf **niemals** als `absence` erscheinen.

`CI01E` (`Campanulo-Nardion`) steht in der Verbreitungsdatei, aber nicht in
der gepinnten FloraVeg-Datei — echter Fassungsdrift zwischen EVC-Fassung 3
(2024-06-12) und der Hierarchiedatei `version_4`, kein Fehler dieser
Pipeline. Der Ingest meldet ihn namentlich.

## Bewusst nicht drin

Die Kartenbilder aus der 82-MB-ZIP (situs antwortet mit Daten, nicht mit
PDF-Karten), eine Abbildung der Territorien auf WGSRPD oder ISO (es gibt
keine geprüfte, also behauptet situs keine), und die vier Summenspalten
(jede ist aus den Zellen nachrechenbar; gemessen weichen 6 der 1115
Summenzellen von der Zellzahl ab, was sie als Quelle für Zahlen
disqualifiziert).

## Tests

```bash
cd pipelines/evc-distribution && python3 -m unittest discover -v
```

Baut ihre XLSX-Fixtures selbst — kein Netzwerk, keine Binärdatei im Repo
nötig. Der wichtigste Fall ist `test_a_missing_cell_does_not_shift_the_
following_ones`: er ist der Test, der eine positionelle Lesung auffliegen
lässt.
````

Die Bemerkung über die 6 abweichenden Summenzellen ist gemessen (`FA03A`,
`FA03B`, `FA03C`, `FA03F`, `FA03G` und eine weitere): die Spalte
`Verified occurrences` stimmt dort nicht mit der Zahl der `1`-Zellen
überein. Deshalb liest die Pipeline die Summen nicht, sondern zählt selbst.

- [ ] **Step 7: `make pipeline-test` erweitern**

Im `Makefile`, im Ziel `pipeline-test`, nach der `eurovegchecklist`-Zeile:

```make
	cd pipelines/evc-distribution && python3 -m unittest discover
```

Run: `make pipeline-test`
Expected: alle sieben Pipeline-Testläufe grün. Die CI ruft dieses Ziel, braucht also keine eigene Änderung.

- [ ] **Step 8: Sammelskript erweitern**

In `scripts/collect-ingest-input.sh`, im `OPTIONAL`-Block. **Achtung:** Teilprojekt A hat `syntaxa_hierarchy.csv` aus `OPTIONAL` nach `REQUIRED` verschoben und `data/syntaxa_formations.csv` dort ergänzt — die drei neuen Zeilen gehören ans **Ende** des `OPTIONAL`-Blocks, nicht neben eine Zeile, die dort nicht mehr steht:

```bash
  "pipelines/evc-distribution/out/syntaxon_distribution.csv:syntaxon_distribution.csv"
  "pipelines/evc-distribution/out/syntaxon_distribution_coverage.csv:syntaxon_distribution_coverage.csv"
  "pipelines/evc-distribution/out/evc_territories.csv:evc_territories.csv"
```

**`OPTIONAL`, nicht `REQUIRED`.** Eine fehlende Verbreitung ist laut Spec Abschnitt 7 nur eine Warnung: sie ist Zusatzinformation wie `species_distribution`, nicht Primärquelle wie die Hierarchie. Ein Index ohne sie antwortet auf jede Frage, nur eben ohne `distribution` — und das ist dann die Wahrheit über diesen Index, keine Lücke, die verschwiegen wird.

Run: `make ingest-input`
Expected: Erfolg, und `out/ingest-input/` enthält alle drei neuen Dateien.

- [ ] **Step 9: Commit**

```bash
git add pipelines/evc-distribution/ Makefile scripts/collect-ingest-input.sh .gitignore
git commit -m "feat(evc-distribution): Quelle pinnen, Pipeline verdrahten, gemessene Zahlen dokumentieren"
```

---

### Task 3: Zweites Gebietsschema in der Domäne, zwei Tabellen in `schema.sql`

**Files:**
- Modify: `internal/domain/area.go`
- Modify: `internal/adapters/sqlite/schema.sql`
- Test: `internal/domain/area_test.go`, `internal/adapters/sqlite/schema_check_test.go`

**Interfaces:**
- Consumes: nichts.
- Produces:
  - `domain.SchemeEVCTerritory = "evc_territory"`
  - `domain.KnownAreaSchemes() []string` — die bekannten Schemata, sortiert, für Fehlermeldungen.
  - `domain.IsKnownAreaScheme(scheme string) bool`
  - `domain.SyntaxonDistribution` (Wertobjekt mit `Scheme string`, `Covered bool`, `Verified []string`, `Uncertain []string`)
  - `domain.OccurrenceVerified = "verified"`, `domain.OccurrenceUncertain = "uncertain"`
  - Tabellen `syntaxon_distribution` und `syntaxon_distribution_coverage` samt `idx_syntaxon_distribution_area`.

**Der Unterstrich ist keine Geschmacksfrage.** Die bestehende Konstante heißt `domain.SchemeWGSRPDL3 = "wgsrpd_l3"`. Ein einzelnes Schema mit Bindestrich wäre eine Sorte Uneinheitlichkeit, die man später nicht mehr los wird, weil sie in Antworten steht.

**Beide Tabellen gehören in `schema.sql`, nicht in `Migrate`.** `verifyTables` in `internal/adapters/sqlite/schema_check.go` liest die zu prüfende Tabellenliste per Regex (`CREATE TABLE IF NOT EXISTS ([a-z_]+)`) aus dem eingebetteten `schema.sql` — eine dort deklarierte Tabelle wird also beim schreibgeschützten Öffnen automatisch geprüft, eine nur in `Migrate` angelegte gar nicht. Läge sie in `Migrate`, würde ein Dienst mit einem Index von vor diesem Teilprojekt bei grüner Gesundheitsprüfung starten und für jedes Syntaxon `distribution` weglassen, also „niemand hat nachgesehen" behaupten, wo in Wahrheit die Tabelle fehlt. Genau diese Verwechslung soll das Spec verhindern.

Und umgekehrt: `migratedColumns` bekommt **keinen** Eintrag. Es sind neue Tabellen, keine neuen Spalten an bestehenden — `verifyTables` deckt sie vollständig ab. `Migrate` braucht auch kein `ALTER`: `OpenForIngest` wendet `schema.sql` an, und `CREATE TABLE IF NOT EXISTS` legt beide auf einem Altindex beim nächsten Ingest an.

- [ ] **Step 1: Die failing Tests schreiben**

In `internal/domain/area_test.go`:

```go
func TestSchemaKonstantenWerte(t *testing.T) {
	// The values sit in a schema CHECK, in query strings and in published
	// JSON: a rename must not pass silently.
	for got, want := range map[string]string{
		domain.SchemeWGSRPDL3:       "wgsrpd_l3",
		domain.SchemeEVCTerritory:   "evc_territory",
		domain.OccurrenceVerified:   "verified",
		domain.OccurrenceUncertain:  "uncertain",
	} {
		if got != want {
			t.Errorf("Konstante = %q, erwartet %q", got, want)
		}
	}
}

func TestIsKnownAreaSchemeKenntBeideUndSonstNichts(t *testing.T) {
	for _, scheme := range []string{domain.SchemeWGSRPDL3, domain.SchemeEVCTerritory} {
		if !domain.IsKnownAreaScheme(scheme) {
			t.Errorf("IsKnownAreaScheme(%q) = false", scheme)
		}
	}
	for _, scheme := range []string{"", "wgsrpd-l3", "evc-territory", "WGSRPD_L3", "iso3166"} {
		if domain.IsKnownAreaScheme(scheme) {
			t.Errorf("IsKnownAreaScheme(%q) = true, erwartet false", scheme)
		}
	}
}

func TestKnownAreaSchemesIstSortiertUndVollstaendig(t *testing.T) {
	got := domain.KnownAreaSchemes()
	want := []string{"evc_territory", "wgsrpd_l3"}
	if !slices.Equal(got, want) {
		t.Errorf("KnownAreaSchemes() = %v, erwartet %v", got, want)
	}
	// The caller puts this list into an INVALID_QUERY message; a caller
	// mutating it must not change what the next request is told.
	got[0] = "geaendert"
	if domain.KnownAreaSchemes()[0] != "evc_territory" {
		t.Error("KnownAreaSchemes() gibt den internen Slice heraus")
	}
}
```

In `internal/adapters/sqlite/schema_check_test.go` (Muster: der bestehende Test für eine fehlende Tabelle):

```go
func TestOpenReadOnlyLehntIndexOhneVerbreitungstabellenAb(t *testing.T) {
	for _, table := range []string{"syntaxon_distribution", "syntaxon_distribution_coverage"} {
		t.Run(table, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "alt.sqlite")
			db, err := sqlite.OpenForIngest(context.Background(), path)
			if err != nil {
				t.Fatalf("OpenForIngest: %v", err)
			}
			// An index built before this release: every other table, but not
			// this one. Dropping it is the only honest way to produce that
			// state from the current schema.
			if _, err := db.ExecContext(context.Background(), `DROP TABLE `+table); err != nil {
				t.Fatalf("DROP TABLE: %v", err)
			}
			if err := db.FinalizeForServing(context.Background()); err != nil {
				t.Fatalf("FinalizeForServing: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			_, err = sqlite.OpenReadOnly(context.Background(), path)
			if err == nil {
				t.Fatalf("OpenReadOnly hat einen Index ohne %s akzeptiert", table)
			}
			if !strings.Contains(err.Error(), table) {
				t.Errorf("Fehler benennt die fehlende Tabelle nicht: %v", err)
			}
		})
	}
}
```

Der `DROP TABLE `+table` ist eine Konkatenation im **Test**, nicht im Produktionscode; gosec G201/G202 prüft den Produktionspfad. Ist das Lint-Profil auch für Tests scharf, stattdessen zwei Unterfälle mit je einem Literal schreiben — nie ein `#nosec`.

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/domain/ -run 'TestSchemaKonstanten|TestIsKnownAreaScheme|TestKnownAreaSchemes' -v && go test ./internal/adapters/sqlite/ -run TestOpenReadOnlyLehntIndexOhneVerbreitung -v`
Expected: FAIL — `undefined: domain.SchemeEVCTerritory`, `undefined: domain.IsKnownAreaScheme`, `undefined: domain.KnownAreaSchemes`; und `no such table: syntaxon_distribution` beim `DROP TABLE`.

- [ ] **Step 3: Domäne erweitern**

In `internal/domain/area.go`, unter die bestehende Konstante:

```go
// SchemeEVCTerritory is the second area scheme: the 136 territory columns of
// the EVC alliance distribution table. They are NOT WGSRPD areas — some are
// states ("Albania"), some biogeographical parts of one ("Austria Alps",
// "France Mediterranean"), and most have a coastal counterpart
// ("Albania_coast").
//
// There is no mapping between the two schemes and there is not meant to be
// one, the same stance CLAUDE.md already records for ISO<->WGSRPD. Species
// distribution is filtered with WGSRPD codes, syntaxon distribution with
// territory codes, and no answer claims a conversion nobody has checked.
const SchemeEVCTerritory = "evc_territory"

// The occurrence states the source distinguishes: "1" and "U" in its cells.
// Absence is the ABSENCE of a row and therefore has no constant — giving it
// one would invite writing it.
const (
	OccurrenceVerified  = "verified"
	OccurrenceUncertain = "uncertain"
)

// knownAreaSchemes is checked against, rather than one scheme being hardcoded
// at each site: the ingest used to reject anything but wgsrpd_l3, which was
// right while there was one scheme and would have silently dropped all 136
// territories once there were two. Checking the SET keeps a typo failing.
var knownAreaSchemes = []string{SchemeEVCTerritory, SchemeWGSRPDL3}

// KnownAreaSchemes returns the area schemes situs stores, sorted. Callers put
// this into an INVALID_QUERY message, so it hands out a copy.
func KnownAreaSchemes() []string { return slices.Clone(knownAreaSchemes) }

// IsKnownAreaScheme reports whether scheme is one situs stores. Exact match,
// no case folding: the scheme id is an identifier, not free text.
func IsKnownAreaScheme(scheme string) bool { return slices.Contains(knownAreaSchemes, scheme) }

// SyntaxonDistribution is what the source says about one syntaxon in one area
// scheme.
//
// Covered is the field that keeps four states apart, and it is the whole
// reason this is a struct rather than two slices: with Covered false NOTHING
// is known about this syntaxon's occurrence, which is strictly different from
// "occurs nowhere" (Covered true, both slices empty). Measured: 212 of the
// index's 1326 alliances are the first case — every bryophyte, lichen and
// algal alliance among them — and reporting them as absences would be an
// invented claim about 136 territories each.
type SyntaxonDistribution struct {
	Scheme    string
	Covered   bool
	Verified  []string
	Uncertain []string
}
```

`slices` in die Importliste der Datei aufnehmen (heute importiert sie nichts).

- [ ] **Step 4: Tabellen in `schema.sql` anlegen**

In `internal/adapters/sqlite/schema.sql`, hinter `species_distribution` (damit die beiden Verbreitungstabellen beieinanderstehen):

```sql
-- Which territories a syntaxon occurs in. One row per OCCUPIED cell of the
-- source table: absence is the absence of a row, exactly as in
-- species_distribution.
CREATE TABLE IF NOT EXISTS syntaxon_distribution (
  syntaxon_id TEXT NOT NULL,
  area_scheme TEXT NOT NULL,
  area_code   TEXT NOT NULL,
  occurrence  TEXT NOT NULL CHECK (occurrence IN ('verified', 'uncertain')),
  PRIMARY KEY (syntaxon_id, area_scheme, area_code)
);
CREATE INDEX IF NOT EXISTS idx_syntaxon_distribution_area
  ON syntaxon_distribution(area_scheme, area_code);

-- Which syntaxa the source makes any statement about at all. WITHOUT this
-- table "does not occur" cannot be told from "nobody looked" — an alliance
-- with 136 empty cells looks exactly like one the source does not list. The
-- source covers vascular-plant dominated vegetation only: measured, 212 of
-- the index's 1326 alliances have no row here, including every bryophyte,
-- lichen and algal one.
CREATE TABLE IF NOT EXISTS syntaxon_distribution_coverage (
  syntaxon_id TEXT NOT NULL,
  area_scheme TEXT NOT NULL,
  PRIMARY KEY (syntaxon_id, area_scheme)
);
```

Der `CHECK` auf `occurrence` steht hier, weil die Wertemenge fachlich feststeht und ein Tippfehler dort eine stille Falschaussage wäre — dieselbe Begründung wie bei `species_role.provenance`. Kein `CHECK` auf `area_scheme`: ein weiteres Gebietsschema soll eine Datenzeile sein, keine Schemaänderung, und die Prüfung liegt im Ingest (Task 5), wo der Report sie zeigen kann.

- [ ] **Step 5: Tests laufen lassen und grün sehen**

Run: `go test ./internal/domain/ ./internal/adapters/sqlite/ -v`
Expected: PASS. Insbesondere greift `verifyTables` die beiden neuen Tabellen ohne eine Zeile Zusatzcode, weil es `schema.sql` selbst liest.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/area.go internal/domain/area_test.go internal/adapters/sqlite/
git commit -m "feat(domain,sqlite): zweites Gebietsschema und die zwei Verbreitungstabellen"
```

---

### Task 4: Schreib- und Leseoperationen, und die zwei schemaabhängigen Abfragen

**Files:**
- Modify: `internal/ports/output/repository.go`
- Modify: `internal/adapters/sqlite/ingest_tx.go` (die Datei, die `IngestTx` implementiert — Name gemäß Repo)
- Create: `internal/adapters/sqlite/syntaxon_distribution.go`
- Modify: `internal/adapters/sqlite/area.go` (`AreasWithData`)
- Modify: `internal/adapters/sqlite/read.go` (`KnownAreaCodes`)
- Test: `internal/adapters/sqlite/syntaxon_distribution_test.go`, `internal/adapters/sqlite/area_test.go`

**Interfaces:**
- Consumes: `domain.SchemeEVCTerritory`, `domain.SyntaxonDistribution`, `domain.Occurrence*` (Task 3).
- Produces:
  - `IngestTx.UpsertSyntaxonDistribution(syntaxonID, scheme, code, occurrence string) error`
  - `IngestTx.UpsertSyntaxonDistributionCoverage(syntaxonID, scheme string) error`
  - `Repository.SyntaxonDistribution(ctx context.Context, syntaxonID, scheme string) (domain.SyntaxonDistribution, error)`
  - `Repository.SyntaxonOccurrencesInArea(ctx context.Context, scheme, code string) (map[string]string, error)`
  - `Repository.SyntaxaWithCoverage(ctx context.Context, scheme string) (map[string]bool, error)`
  - `Repository.AreasWithData(ctx, scheme)` — zweiter Zweig über `syntaxon_distribution`
  - `Repository.KnownAreaCodes(ctx, scheme)` — derselbe zweite Zweig

**Blocker 2, und ein vierter Fund derselben Art.** `AreasWithData` in `internal/adapters/sqlite/area.go` leitet die Abdeckung aus `species_distribution` ab (`SELECT DISTINCT area_scheme, area_code FROM species_distribution WHERE area_scheme = ?`). Für `evc_territory` gibt es dort nie eine Zeile: die Territorien blieben unter jedem Schema-Argument unsichtbar, `GET /v1/areas?scheme=evc_territory` läge leer, und das Spec nennt genau das. **Dasselbe gilt für `KnownAreaCodes`** (`internal/adapters/sqlite/read.go`, Zeile 161) — das Spec führt es nicht auf, aber es ist die Methode, gegen die `areaLookup` einen `?area=`-Wert prüft. Ohne den zweiten Zweig dort wäre **jeder** Territoriumscode auf `GET /v1/syntaxa?area=` ein `INVALID_QUERY`, also der Filter aus Abschnitt 6 vollständig unbenutzbar, und zwar mit einer Fehlermeldung, die dem Client einen Tippfehler unterstellt. Beide bekommen denselben Zweig, und zwar als **zwei Literale mit einer Verzweigung** — ein aus dem Schema gebauter Tabellenname wäre G201.

- [ ] **Step 1: Die failing Tests schreiben**

`internal/adapters/sqlite/syntaxon_distribution_test.go`:

```go
package sqlite_test

func seedDistribution(t *testing.T, db *sqlite.DB) {
	t.Helper()
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// CA01A: two verified, one uncertain. CA01B: covered, no occurrence at all
	// ("checked, occurs in no territory"). CA01C: no coverage row — nothing is
	// known about it, which must never read as absence.
	for _, row := range []struct{ id, code, occ string }{
		{"CA01A", "austria-alps", domain.OccurrenceVerified},
		{"CA01A", "albania", domain.OccurrenceVerified},
		{"CA01A", "czech-republic", domain.OccurrenceUncertain},
	} {
		if err := tx.UpsertSyntaxonDistribution(row.id, domain.SchemeEVCTerritory, row.code, row.occ); err != nil {
			t.Fatalf("UpsertSyntaxonDistribution: %v", err)
		}
	}
	for _, id := range []string{"CA01A", "CA01B"} {
		if err := tx.UpsertSyntaxonDistributionCoverage(id, domain.SchemeEVCTerritory); err != nil {
			t.Fatalf("UpsertSyntaxonDistributionCoverage: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestSyntaxonDistributionLiefertSortierteListen(t *testing.T) {
	db := newTestDB(t) // the package's existing helper
	seedDistribution(t, db)

	got, err := db.SyntaxonDistribution(context.Background(), "CA01A", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if !got.Covered {
		t.Error("Covered = false, erwartet true")
	}
	if !slices.Equal(got.Verified, []string{"albania", "austria-alps"}) {
		t.Errorf("Verified = %v, erwartet [albania austria-alps]", got.Verified)
	}
	if !slices.Equal(got.Uncertain, []string{"czech-republic"}) {
		t.Errorf("Uncertain = %v, erwartet [czech-republic]", got.Uncertain)
	}
	if got.Scheme != domain.SchemeEVCTerritory {
		t.Errorf("Scheme = %q", got.Scheme)
	}
}

func TestSyntaxonDistributionTrenntAbsenceVonUnknown(t *testing.T) {
	db := newTestDB(t)
	seedDistribution(t, db)

	// Covered, but not a single occurrence: "checked, occurs nowhere".
	absent, err := db.SyntaxonDistribution(context.Background(), "CA01B", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if !absent.Covered {
		t.Error("CA01B: Covered = false, erwartet true — die Coverage-Zeile IST die Aussage")
	}
	if len(absent.Verified) != 0 || len(absent.Uncertain) != 0 {
		t.Errorf("CA01B traegt Vorkommen: %+v", absent)
	}

	// No coverage row: nothing is known.
	unknown, err := db.SyntaxonDistribution(context.Background(), "CA01C", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if unknown.Covered {
		t.Error("CA01C: Covered = true, erwartet false")
	}
}

func TestSyntaxonDistributionIstSchemabezogen(t *testing.T) {
	db := newTestDB(t)
	seedDistribution(t, db)

	got, err := db.SyntaxonDistribution(context.Background(), "CA01A", domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if got.Covered || len(got.Verified) != 0 {
		t.Errorf("CA01A unter wgsrpd_l3 = %+v, erwartet leer und uncovered", got)
	}
}

func TestSyntaxonOccurrencesInArea(t *testing.T) {
	db := newTestDB(t)
	seedDistribution(t, db)

	got, err := db.SyntaxonOccurrencesInArea(context.Background(), domain.SchemeEVCTerritory, "austria-alps")
	if err != nil {
		t.Fatalf("SyntaxonOccurrencesInArea: %v", err)
	}
	if len(got) != 1 || got["CA01A"] != domain.OccurrenceVerified {
		t.Errorf("Karte = %v, erwartet genau CA01A->verified", got)
	}

	// An area with no rows is an empty map and no error: "nobody occurs here"
	// is a valid answer for a code the scheme knows.
	empty, err := db.SyntaxonOccurrencesInArea(context.Background(), domain.SchemeEVCTerritory, "armenia")
	if err != nil {
		t.Fatalf("SyntaxonOccurrencesInArea: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("armenia = %v, erwartet leer", empty)
	}
}

func TestSyntaxaWithCoverage(t *testing.T) {
	db := newTestDB(t)
	seedDistribution(t, db)

	got, err := db.SyntaxaWithCoverage(context.Background(), domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxaWithCoverage: %v", err)
	}
	if len(got) != 2 || !got["CA01A"] || !got["CA01B"] || got["CA01C"] {
		t.Errorf("Menge = %v, erwartet genau CA01A und CA01B", got)
	}
}

func TestUpsertSyntaxonDistributionIstIdempotent(t *testing.T) {
	db := newTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// Twice the same cell, then the same cell with the other occurrence: a
	// repeated ingest must neither fail on the primary key nor keep the stale
	// value.
	for _, occ := range []string{domain.OccurrenceVerified, domain.OccurrenceVerified, domain.OccurrenceUncertain} {
		if err := tx.UpsertSyntaxonDistribution("CA01A", domain.SchemeEVCTerritory, "albania", occ); err != nil {
			t.Fatalf("UpsertSyntaxonDistribution(%s): %v", occ, err)
		}
	}
	if err := tx.UpsertSyntaxonDistributionCoverage("CA01A", domain.SchemeEVCTerritory); err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if err := tx.UpsertSyntaxonDistributionCoverage("CA01A", domain.SchemeEVCTerritory); err != nil {
		t.Fatalf("Coverage zweimal: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.SyntaxonDistribution(context.Background(), "CA01A", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if len(got.Verified) != 0 || !slices.Equal(got.Uncertain, []string{"albania"}) {
		t.Errorf("Verbreitung = %+v, erwartet nur uncertain=[albania]", got)
	}
}

func TestUpsertSyntaxonDistributionLehntFremdeAuspraegungAb(t *testing.T) {
	db := newTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// The CHECK is the last line of defence: a value the pipeline should have
	// rejected must not reach the index silently.
	if err := tx.UpsertSyntaxonDistribution("CA01A", domain.SchemeEVCTerritory, "albania", "1"); err == nil {
		t.Fatal("UpsertSyntaxonDistribution akzeptierte occurrence=\"1\"")
	}
	_ = tx.Rollback()
}
```

In `internal/adapters/sqlite/area_test.go`:

```go
func TestAreasWithDataListetTerritorienAusDerSyntaxonverbreitung(t *testing.T) {
	db := newTestDB(t)
	seedDistribution(t, db)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertArea(domain.NamedArea{
		Area:   domain.Area{Scheme: domain.SchemeEVCTerritory, Code: "austria-alps"},
		NameEN: "Austria Alps",
	}); err != nil {
		t.Fatalf("UpsertArea: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.AreasWithData(context.Background(), domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("AreasWithData: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Gebiete = %+v, erwartet 3 (albania, austria-alps, czech-republic)", got)
	}
	if got[0].Code != "albania" || got[0].NameEN != "" {
		t.Errorf("erstes Gebiet = %+v, erwartet albania mit leerem Namen", got[0])
	}
	if got[1].Code != "austria-alps" || got[1].NameEN != "Austria Alps" {
		t.Errorf("zweites Gebiet = %+v", got[1])
	}
}

func TestKnownAreaCodesLiestJeSchemaDieRichtigeTabelle(t *testing.T) {
	db := newTestDB(t)
	seedDistribution(t, db)

	terr, err := db.KnownAreaCodes(context.Background(), domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("KnownAreaCodes(evc_territory): %v", err)
	}
	if !slices.Equal(terr, []string{"albania", "austria-alps", "czech-republic"}) {
		t.Errorf("Territorien = %v", terr)
	}

	// The species table is untouched by the seed above, so wgsrpd_l3 must stay
	// empty — proof the branch reads the OTHER table, not just a filter.
	wg, err := db.KnownAreaCodes(context.Background(), domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("KnownAreaCodes(wgsrpd_l3): %v", err)
	}
	if len(wg) != 0 {
		t.Errorf("wgsrpd_l3 = %v, erwartet leer", wg)
	}
}
```

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/adapters/sqlite/ -run 'SyntaxonDistribution|SyntaxaWithCoverage|SyntaxonOccurrences|AreasWithDataListet|KnownAreaCodesLiest' -v`
Expected: FAIL — die Methoden `UpsertSyntaxonDistribution`, `UpsertSyntaxonDistributionCoverage`, `SyntaxonDistribution`, `SyntaxonOccurrencesInArea`, `SyntaxaWithCoverage` existieren nicht; `AreasWithData(evc_territory)` liefert 0 Gebiete; `KnownAreaCodes(evc_territory)` liefert nichts.

- [ ] **Step 3: Port erweitern**

In `internal/ports/output/repository.go`, in `IngestTx` neben `UpsertDistribution`:

```go
	// UpsertSyntaxonDistribution records that a syntaxon occurs in one area,
	// with occurrence being domain.OccurrenceVerified or
	// domain.OccurrenceUncertain. Idempotent, and a repeated ingest overwrites
	// the occurrence: the source is allowed to upgrade an uncertain record to
	// a verified one, and an index that kept the old value would answer from a
	// fassung nobody published.
	UpsertSyntaxonDistribution(syntaxonID, scheme, code, occurrence string) error
	// UpsertSyntaxonDistributionCoverage records that the source makes a
	// statement about this syntaxon at all. Without it "occurs in no
	// territory" is indistinguishable from "nobody looked".
	UpsertSyntaxonDistributionCoverage(syntaxonID, scheme string) error
```

und in `Repository`, neben `AreasForConcepts`:

```go
	// SyntaxonDistribution returns what the source says about one syntaxon in
	// one area scheme: the verified and uncertain codes, sorted, plus whether
	// there is any statement at all (Covered). A syntaxon with no coverage row
	// comes back Covered false and both lists empty — which the read side must
	// serve as "no distribution field", never as "occurs nowhere".
	SyntaxonDistribution(ctx context.Context, syntaxonID, scheme string) (domain.SyntaxonDistribution, error)
	// SyntaxonOccurrencesInArea maps syntaxon id -> occurrence for one area.
	// A syntaxon absent from the map has no row for that area, which is NOT
	// the same as one absent from SyntaxaWithCoverage.
	SyntaxonOccurrencesInArea(ctx context.Context, scheme, code string) (map[string]string, error)
	// SyntaxaWithCoverage is the set of syntaxa the source makes a statement
	// about. The read side needs it to keep the unjudgeable rows in an
	// ?area=-filtered list instead of dropping them.
	SyntaxaWithCoverage(ctx context.Context, scheme string) (map[string]bool, error)
```

Die Doc-Kommentare von `KnownAreaCodes` und `AreasWithData` werden angepasst: sie leiten die Abdeckung **je Schema** aus der jeweils zuständigen Tabelle ab, nicht pauschal aus `species_distribution`.

- [ ] **Step 4: Implementieren**

In der `IngestTx`-Implementierung:

```go
func (t *tx) UpsertSyntaxonDistribution(syntaxonID, scheme, code, occurrence string) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO syntaxon_distribution (syntaxon_id, area_scheme, area_code, occurrence)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(syntaxon_id, area_scheme, area_code) DO UPDATE SET
		   occurrence = excluded.occurrence`,
		syntaxonID, scheme, code, occurrence)
	return err
}

func (t *tx) UpsertSyntaxonDistributionCoverage(syntaxonID, scheme string) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT OR IGNORE INTO syntaxon_distribution_coverage (syntaxon_id, area_scheme)
		 VALUES (?, ?)`,
		syntaxonID, scheme)
	return err
}
```

`internal/adapters/sqlite/syntaxon_distribution.go` — eigene Datei, damit `read_syntaxon.go` nicht über den Summendeckel wächst:

```go
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
)

// SyntaxonDistribution reads the occurrence rows and the coverage row in two
// queries rather than one outer join: the coverage row is a different fact
// from the occurrence rows (it exists for a syntaxon with no occurrence at
// all), and folding both into one result set would make "no rows" ambiguous
// again — which is the exact confusion the second table exists to prevent.
func (d *DB) SyntaxonDistribution(ctx context.Context, syntaxonID, scheme string) (domain.SyntaxonDistribution, error) {
	out := domain.SyntaxonDistribution{
		Scheme:    scheme,
		Verified:  []string{},
		Uncertain: []string{},
	}

	var one int
	row := d.QueryRowContext(ctx,
		`SELECT 1 FROM syntaxon_distribution_coverage
		 WHERE syntaxon_id = ? AND area_scheme = ?`, syntaxonID, scheme)
	switch err := row.Scan(&one); {
	case err == nil:
		out.Covered = true
	case errors.Is(err, sql.ErrNoRows):
		out.Covered = false
	default:
		return domain.SyntaxonDistribution{}, fmt.Errorf("sqlite: reading syntaxon coverage: %w", err)
	}

	rows, err := d.QueryContext(ctx,
		`SELECT area_code, occurrence FROM syntaxon_distribution
		 WHERE syntaxon_id = ? AND area_scheme = ?
		 ORDER BY area_code`, syntaxonID, scheme)
	if err != nil {
		return domain.SyntaxonDistribution{}, fmt.Errorf("sqlite: reading syntaxon distribution: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var code, occurrence string
		if err := rows.Scan(&code, &occurrence); err != nil {
			return domain.SyntaxonDistribution{}, fmt.Errorf("sqlite: scanning syntaxon distribution: %w", err)
		}
		if occurrence == domain.OccurrenceUncertain {
			out.Uncertain = append(out.Uncertain, code)
			continue
		}
		out.Verified = append(out.Verified, code)
	}
	if err := rows.Err(); err != nil {
		return domain.SyntaxonDistribution{}, fmt.Errorf("sqlite: iterating syntaxon distribution: %w", err)
	}
	return out, nil
}

// SyntaxonOccurrencesInArea answers the ?area= filter with one query instead
// of one per syntaxon: the filtered list can hold 1326 entries.
func (d *DB) SyntaxonOccurrencesInArea(ctx context.Context, scheme, code string) (map[string]string, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT syntaxon_id, occurrence FROM syntaxon_distribution
		 WHERE area_scheme = ? AND area_code = ?`, scheme, code)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading occurrences in area: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]string{}
	for rows.Next() {
		var id, occurrence string
		if err := rows.Scan(&id, &occurrence); err != nil {
			return nil, fmt.Errorf("sqlite: scanning occurrence: %w", err)
		}
		out[id] = occurrence
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating occurrences in area: %w", err)
	}
	return out, nil
}

// SyntaxaWithCoverage is the set the read side needs to tell an absence from
// an unknown while filtering.
func (d *DB) SyntaxaWithCoverage(ctx context.Context, scheme string) (map[string]bool, error) {
	ids, err := d.queryStrings(ctx, "syntaxa with distribution coverage",
		`SELECT syntaxon_id FROM syntaxon_distribution_coverage
		 WHERE area_scheme = ?`, scheme)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}
```

`internal/adapters/sqlite/area.go`, `AreasWithData` — zwei Literale, eine Verzweigung:

```go
// AreasWithData lists the areas of one scheme the index has distribution data
// for, each with its ingested name, sorted by code.
//
// The distribution table decides membership and the area table only lends the
// name: an area nobody occurs in would be a filter that can only ever answer
// empty, and a code with data but no name row stays in the list with an empty
// name instead of being dropped.
//
// WHICH distribution table is the scheme's decision. wgsrpd_l3 codes come
// from species_distribution, evc_territory codes from syntaxon_distribution —
// and there is never a row of one in the other, so deriving the coverage from
// species_distribution alone made all 136 territories invisible under every
// scheme argument. Two literals, not one built from the scheme: a table name
// interpolated into SQL is gosec G201, and rightly so.
func (d *DB) AreasWithData(ctx context.Context, scheme string) ([]domain.NamedArea, error) {
	query := `SELECT d.area_code, COALESCE(a.name_en, '')
		 FROM (SELECT DISTINCT area_scheme, area_code FROM species_distribution
		       WHERE area_scheme = ?) d
		 LEFT JOIN area a ON a.area_scheme = d.area_scheme AND a.area_code = d.area_code
		 ORDER BY d.area_code`
	if scheme == domain.SchemeEVCTerritory {
		query = `SELECT d.area_code, COALESCE(a.name_en, '')
		 FROM (SELECT DISTINCT area_scheme, area_code FROM syntaxon_distribution
		       WHERE area_scheme = ?) d
		 LEFT JOIN area a ON a.area_scheme = d.area_scheme AND a.area_code = d.area_code
		 ORDER BY d.area_code`
	}

	rows, err := d.QueryContext(ctx, query, scheme)
	// ... the existing body from here on, unchanged
}
```

`internal/adapters/sqlite/read.go`, `KnownAreaCodes`:

```go
// KnownAreaCodes lists the distinct area codes the index has data for, in a
// given scheme. The read side validates an area filter against this: an
// unknown code becomes an error, not a silent "does not occur" answer.
//
// Per scheme from its own table, for the same reason AreasWithData branches:
// without this, every evc_territory code would be rejected as unknown and the
// ?area= filter on /v1/syntaxa could not answer a single valid request.
func (d *DB) KnownAreaCodes(ctx context.Context, scheme string) ([]string, error) {
	if scheme == domain.SchemeEVCTerritory {
		return d.queryStrings(ctx, "syntaxon area codes",
			`SELECT DISTINCT area_code FROM syntaxon_distribution
			 WHERE area_scheme = ? ORDER BY area_code`, scheme)
	}
	return d.queryStrings(ctx, "area codes",
		`SELECT DISTINCT area_code FROM species_distribution
		 WHERE area_scheme = ? ORDER BY area_code`, scheme)
}
```

- [ ] **Step 5: Tests laufen lassen und grün sehen**

Run: `go test ./internal/adapters/sqlite/ ./internal/ports/... -v`
Expected: PASS. Die Pakete `internal/application` und `internal/adapters/http` kompilieren jetzt nicht, weil `fakeRepo` und `fakeQueryService` die drei neuen Repository-Methoden nicht haben — das ist erwartet und wird in Task 6 bzw. 8 behoben. Nur die beiden hier genannten Pakete testen.

- [ ] **Step 6: Commit**

```bash
git add internal/ports/output/repository.go internal/adapters/sqlite/
git commit -m "feat(sqlite): Syntaxa-Verbreitung schreiben und lesen, Gebietsabfragen je Schema"
```

---

### Task 5: `IngestAreas` prüft gegen die Menge der bekannten Schemata (Blocker 1)

**Files:**
- Modify: `internal/application/area_ingest.go`
- Test: `internal/application/area_ingest_test.go`

**Interfaces:**
- Consumes: `domain.IsKnownAreaScheme`, `domain.KnownAreaSchemes` (Task 3).
- Produces: `IngestAreas` lädt jede CSV, deren `area_scheme` ein **bekanntes** ist — also auch `evc_territories.csv`. Signatur unverändert: `IngestAreas(ctx, repo, csvPath) (AreaReport, error)`.

**Blocker 1.** `internal/application/area_ingest.go` prüft heute `a.Scheme != domain.SchemeWGSRPDL3` und überspringt die Zeile samt Zählung. Alle 136 Territorien würden als übersprungen gemeldet, die `area`-Tabelle bliebe für `evc_territory` leer, und `GET /v1/areas?scheme=evc_territory` lieferte 136 namenlose Codes. Die Prüfung **war** richtig, solange es ein Schema gab: eine Zeile mit einem fremden Schema würde geschrieben und als Erfolg gezählt, joint aber auf der Leseseite mit nichts, und diese Stille ist der ganze Grund, sie hier abzulehnen. Sie wird deshalb nicht entfernt, sondern auf die **Menge** der bekannten Schemata umgestellt — damit fällt ein Tippfehler (`evc-territory`, `wgsrpd-l3`) weiterhin auf.

Damit lädt `IngestAreas` auch `evc_territories.csv`, und die Verbreitungs-Ingest-Funktion in Task 6 braucht **keinen zweiten Gebietslader**. Die Spec-Signatur in Abschnitt 5 (`IngestSyntaxonDistribution(..., csvPath, areaCSVPath)`) widerspricht dem: der zweite Pfad wäre genau dieser zweite Lader. Task 6 setzt an seine Stelle den Pfad der Coverage-CSV.

- [ ] **Step 1: Die failing Tests schreiben**

In `internal/application/area_ingest_test.go`:

```go
func TestIngestAreasSchreibtBeideSchemata(t *testing.T) {
	for _, tc := range []struct{ name, scheme, code, nameEN string }{
		{"wgsrpd", "wgsrpd_l3", "GER", "Germany"},
		{"evc", "evc_territory", "austria-alps", "Austria Alps"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			path := filepath.Join(t.TempDir(), "areas.csv")
			content := "area_scheme,area_code,name_en\n" + tc.scheme + "," + tc.code + "," + tc.nameEN + "\n"
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("Schreiben: %v", err)
			}

			rep, err := application.IngestAreas(context.Background(), repo, path)
			if err != nil {
				t.Fatalf("IngestAreas: %v", err)
			}
			if rep.Areas != 1 || rep.SkippedRows != 0 {
				t.Fatalf("Report = %+v, erwartet 1 Gebiet und 0 uebersprungene", rep)
			}
			got := repo.area(tc.scheme, tc.code)
			if got.NameEN != tc.nameEN {
				t.Errorf("Name = %q, erwartet %q", got.NameEN, tc.nameEN)
			}
		})
	}
}

func TestIngestAreasUeberspringtUnbekanntesSchema(t *testing.T) {
	// A typo must still be caught. "evc-territory" with a hyphen is the exact
	// mistake the underscore decision in the spec exists to keep visible.
	repo := newFakeRepo()
	path := filepath.Join(t.TempDir(), "areas.csv")
	content := "area_scheme,area_code,name_en\n" +
		"evc-territory,austria-alps,Austria Alps\n" +
		"iso3166,DE,Germany\n" +
		"evc_territory,albania,Albania\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("Schreiben: %v", err)
	}

	rep, err := application.IngestAreas(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestAreas: %v", err)
	}
	if rep.Areas != 1 || rep.SkippedRows != 2 {
		t.Errorf("Report = %+v, erwartet 1 Gebiet und 2 uebersprungene", rep)
	}
	if repo.hasArea("evc-territory", "austria-alps") {
		t.Error("das Bindestrich-Schema wurde geschrieben")
	}
}
```

`fakeRepo` im Paket um `area(scheme, code) domain.NamedArea` und `hasArea(scheme, code) bool` erweitern, falls noch nicht vorhanden.

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/application/ -run TestIngestAreas -v`
Expected: FAIL — der `evc`-Unterfall meldet `Report = {Areas:0 SkippedRows:1}`, weil die Zeile als fremdes Schema verworfen wird.

- [ ] **Step 3: Implementieren**

In `internal/application/area_ingest.go`, in `ingestAreaRows`, den Schema-Zweig ersetzen:

```go
			// situs stores a SET of area schemes (wgsrpd_l3 for species,
			// evc_territory for syntaxa). A row naming something else would be
			// written and counted as a success, yet join with nothing on the
			// read side — that silence is the whole reason to reject it here,
			// where the report can show it. Checking the set rather than one
			// constant is what keeps a typo like "evc-territory" failing while
			// letting the second real scheme through.
			if !domain.IsKnownAreaScheme(a.Scheme) {
				skip(line, fmt.Errorf("area scheme %q is not one of %v",
					a.Scheme, domain.KnownAreaSchemes()))
				return nil
			}
```

Und den Doc-Kommentar von `IngestAreas` anpassen: sie liest **eine** Gebietsnamen-CSV, und der Aufrufer ruft sie je Datei einmal (`wgsrpd_areas.csv`, `evc_territories.csv`). Die Kopfzeile ist bei beiden `area_scheme,area_code,name_en`, deshalb braucht es keinen zweiten Leser:

```go
// IngestAreas loads csvPath (area_scheme,area_code,name_en) into repo, in one
// transaction. Both area-name files have that header — pipelines/wgsrpd's
// wgsrpd_areas.csv and pipelines/evc-distribution's evc_territories.csv — so
// the caller calls this once per file rather than a second loader existing.
```

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./internal/application/ -run TestIngestAreas -v`
Expected: PASS für beide Unterfälle und den Tippfehler-Test.

- [ ] **Step 5: Commit**

```bash
git add internal/application/area_ingest.go internal/application/area_ingest_test.go
git commit -m "fix(ingest): Gebietsnamen gegen die Menge der bekannten Schemata pruefen"
```

---

### Task 6: `IngestSyntaxonDistribution`

**Files:**
- Create: `internal/application/syntaxon_distribution_ingest.go`
- Test: `internal/application/syntaxon_distribution_ingest_test.go`

**Interfaces:**
- Consumes: `IngestTx.UpsertSyntaxonDistribution`, `IngestTx.UpsertSyntaxonDistributionCoverage` (Task 4); `Repository.AllSyntaxa` (bestehend, liefert nach Teilprojekt A alle 1882 Zeilen); `readAll`, `newRowSkipper`, `splitCSVPath` (bestehende Helfer des Pakets).
- Produces:

```go
// SyntaxonDistributionReport counts what a distribution ingest wrote and what
// it could not place. Every number is counted, none is estimated.
type SyntaxonDistributionReport struct {
	Written        int      // rows in syntaxon_distribution
	Verified       int
	Uncertain      int
	Covered        int      // syntaxa with a statement (coverage rows)
	SkippedRows    int
	UnknownSyntaxa []string // source codes the index does not know, sorted
}

func IngestSyntaxonDistribution(ctx context.Context, repo output.Repository, csvPath, coveragePath string) (SyntaxonDistributionReport, error)
```

**Zwei Abweichungen von der Spec-Signatur, beide begründet.** Das Spec nennt `(ctx, repo, csvPath, areaCSVPath)`. Der zweite Parameter kann nicht der Gebietspfad sein: Abschnitt 6.1 desselben Specs legt fest, dass `IngestAreas` die Territorien lädt und dass es „keinen zweiten Gebietslader" gibt. An seine Stelle tritt der Pfad der Coverage-CSV aus Task 1 — die Datei, ohne die `absence` und `unknown` zusammenfallen. Und `Territories` fällt aus dem Report: die Zahl steht in `AreaReport` der Territorien-Datei, und sie zweimal zu melden wäre zwei Stellen, die sich widersprechen können.

**Läuft nach `IngestSyntaxa`** (Teilprojekt A) — die Syntaxon-IDs müssen im Index stehen, damit `UnknownSyntaxa` etwas bedeutet. Keine hostus-Beteiligung: eine reine CSV-Quelle, kein Namensauflösungsschritt, der Ingest bleibt offline.

- [ ] **Step 1: Die failing Tests schreiben**

`internal/application/syntaxon_distribution_ingest_test.go`:

```go
package application_test

const (
	minimalCoverage = "syntaxon_id,area_scheme\n" +
		"CA01A,evc_territory\n" +
		"CA01B,evc_territory\n"
	minimalDistribution = "syntaxon_id,area_scheme,area_code,occurrence\n" +
		"CA01A,evc_territory,austria-alps,verified\n" +
		"CA01A,evc_territory,czech-republic,uncertain\n"
)

// seedSyntaxa puts the ids the distribution rows refer to into the fake repo,
// the way IngestSyntaxa would have.
func seedSyntaxa(repo *fakeRepo, ids ...string) {
	for _, id := range ids {
		repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{
			ID: id, Rank: domain.SyntaxonRankAlliance, Name: id,
			Source: domain.SyntaxonSourceEVC,
		})
	}
}

func writeDistFiles(t *testing.T, dist, coverage string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	distPath := filepath.Join(dir, "syntaxon_distribution.csv")
	covPath := filepath.Join(dir, "syntaxon_distribution_coverage.csv")
	for path, content := range map[string]string{distPath: dist, covPath: coverage} {
		if content == "" {
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("Schreiben von %s: %v", path, err)
		}
	}
	return distPath, covPath
}

func TestIngestSyntaxonDistributionFuelltBeideTabellen(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A", "CA01B")
	dist, cov := writeDistFiles(t, minimalDistribution, minimalCoverage)

	rep, err := application.IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Written != 2 || rep.Verified != 1 || rep.Uncertain != 1 || rep.Covered != 2 {
		t.Errorf("Report = %+v, erwartet Written 2 / Verified 1 / Uncertain 1 / Covered 2", rep)
	}
	if got := repo.occurrence("CA01A", "evc_territory", "austria-alps"); got != domain.OccurrenceVerified {
		t.Errorf("austria-alps = %q, erwartet verified", got)
	}
	if got := repo.occurrence("CA01A", "evc_territory", "czech-republic"); got != domain.OccurrenceUncertain {
		t.Errorf("czech-republic = %q, erwartet uncertain", got)
	}
}

func TestIngestSyntaxonDistributionBehaeltCoverageOhneVorkommen(t *testing.T) {
	// CA01B has a coverage row and no occurrence row. That is a STATEMENT
	// ("checked, occurs in no territory") and the row must survive — it is the
	// only thing that keeps absence apart from unknown.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A", "CA01B")
	dist, cov := writeDistFiles(t, minimalDistribution, minimalCoverage)

	rep, err := application.IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Covered != 2 {
		t.Fatalf("Covered = %d, erwartet 2", rep.Covered)
	}
	if !repo.covered("CA01B", "evc_territory") {
		t.Error("CA01B hat keine Coverage-Zeile, obwohl die Quelle eine Aussage macht")
	}
	if n := repo.occurrenceCount("CA01B"); n != 0 {
		t.Errorf("CA01B traegt %d Vorkommen, erwartet 0", n)
	}
}

func TestIngestSyntaxonDistributionVerwirftUnbekanntenCodeUndMeldetIhn(t *testing.T) {
	// The measured case: CI01E is in the distribution file (EVC fassung 3) but
	// not in the pinned FloraVeg file. Real fassung drift between two sources,
	// not a defect of this pipeline — so it is named, not rounded away.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t,
		minimalDistribution+"CI01E,evc_territory,spain-atlantic,verified\n",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\nCI01E,evc_territory\n")

	rep, err := application.IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if len(rep.UnknownSyntaxa) != 1 || rep.UnknownSyntaxa[0] != "CI01E" {
		t.Errorf("UnknownSyntaxa = %v, erwartet [CI01E]", rep.UnknownSyntaxa)
	}
	if rep.Written != 2 {
		t.Errorf("Written = %d, erwartet 2 — die CI01E-Zeile darf nicht geschrieben werden", rep.Written)
	}
	if rep.Covered != 1 {
		t.Errorf("Covered = %d, erwartet 1 — auch die Coverage-Zeile faellt weg", rep.Covered)
	}
	if repo.covered("CI01E", "evc_territory") {
		t.Error("CI01E bekam eine Coverage-Zeile, obwohl der Index das Syntaxon nicht kennt")
	}
}

func TestIngestSyntaxonDistributionMeldetJedenUnbekanntenCodeGenauEinmal(t *testing.T) {
	// One unknown alliance has up to 136 rows. Reporting it 136 times would
	// turn one fassung drift into a wall of noise.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CI01E,evc_territory,spain-atlantic,verified\n"+
			"CI01E,evc_territory,portugal-mediterranean,verified\n"+
			"CI01E,evc_territory,spain-mediterranean,verified\n",
		"syntaxon_id,area_scheme\nCI01E,evc_territory\n")

	rep, err := application.IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if len(rep.UnknownSyntaxa) != 1 {
		t.Errorf("UnknownSyntaxa = %v, erwartet genau einen Eintrag", rep.UnknownSyntaxa)
	}
}

func TestIngestSyntaxonDistributionWarntNurBeiFehlenderDatei(t *testing.T) {
	// Distribution is extra information, exactly like IngestDistribution's —
	// unlike the hierarchy in Teilprojekt A, which is a primary source. A
	// missing file must not abort a five-minute ingest.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dir := t.TempDir()
	rep, err := application.IngestSyntaxonDistribution(context.Background(), repo,
		filepath.Join(dir, "syntaxon_distribution.csv"),
		filepath.Join(dir, "syntaxon_distribution_coverage.csv"))
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep != (application.SyntaxonDistributionReport{}) {
		t.Errorf("Report = %+v, erwartet den Nullwert", rep)
	}
}

func TestIngestSyntaxonDistributionBrichtBeiFehlenderCoverageDateiAb(t *testing.T) {
	// The distribution file WITHOUT the coverage file is the one combination
	// that must not pass: every row would be written and every alliance's
	// empty cell would then read as "nobody looked" instead of "does not
	// occur". Half the truth is worse here than none.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A", "CA01B")
	dist, cov := writeDistFiles(t, minimalDistribution, "")

	_, err := application.IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err == nil {
		t.Fatal("IngestSyntaxonDistribution lief ohne die Coverage-Datei durch")
	}
	if !strings.Contains(err.Error(), "syntaxon_distribution_coverage.csv") {
		t.Errorf("Fehler benennt die Datei nicht: %v", err)
	}
	_ = cov
}

func TestIngestSyntaxonDistributionUeberspringtFremdesSchema(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CA01A,evc-territory,austria-alps,verified\n"+
			"CA01A,evc_territory,albania,verified\n",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\n")

	rep, err := application.IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Written != 1 || rep.SkippedRows != 1 {
		t.Errorf("Report = %+v, erwartet Written 1 / SkippedRows 1", rep)
	}
}

func TestIngestSyntaxonDistributionUeberspringtUnbekannteAuspraegung(t *testing.T) {
	// The pipeline aborts on an unknown cell value, so this row should not
	// exist. It is skipped rather than written because the CHECK would reject
	// it anyway and a failed transaction would lose the other 11527 rows.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CA01A,evc_territory,austria-alps,1\n",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\n")

	rep, err := application.IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Written != 0 || rep.SkippedRows != 1 {
		t.Errorf("Report = %+v, erwartet Written 0 / SkippedRows 1", rep)
	}
}

func TestIngestSyntaxonDistributionUeberspringtUnvollstaendigeZeile(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			",evc_territory,austria-alps,verified\n"+
			"CA01A,evc_territory,,verified\n",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\n")

	rep, err := application.IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Written != 0 || rep.SkippedRows != 2 {
		t.Errorf("Report = %+v, erwartet Written 0 / SkippedRows 2", rep)
	}
}
```

`fakeRepo` um die Verbreitungsseite erweitern — die `IngestTx`-Methoden aus Task 4 und vier Prüfhelfer:

```go
type fakeOccurrence struct{ id, scheme, code string }

func (r *fakeRepo) UpsertSyntaxonDistribution(syntaxonID, scheme, code, occurrence string) error {
	if occurrence != domain.OccurrenceVerified && occurrence != domain.OccurrenceUncertain {
		// Mirrors the schema CHECK, so the application test cannot pass
		// something the real index would refuse.
		return fmt.Errorf("fakeRepo: occurrence %q", occurrence)
	}
	r.occurrences[fakeOccurrence{syntaxonID, scheme, code}] = occurrence
	return nil
}

func (r *fakeRepo) UpsertSyntaxonDistributionCoverage(syntaxonID, scheme string) error {
	r.coverage[syntaxonID+"|"+scheme] = true
	return nil
}

func (r *fakeRepo) occurrence(id, scheme, code string) string {
	return r.occurrences[fakeOccurrence{id, scheme, code}]
}

func (r *fakeRepo) covered(id, scheme string) bool { return r.coverage[id+"|"+scheme] }

func (r *fakeRepo) occurrenceCount(id string) int {
	n := 0
	for k := range r.occurrences {
		if k.id == id {
			n++
		}
	}
	return n
}
```

Die drei neuen `Repository`-Methoden aus Task 4 kommen ebenfalls an `fakeRepo`, sonst kompiliert das Paket nicht — `SyntaxonDistribution`, `SyntaxonOccurrencesInArea` und `SyntaxaWithCoverage`, jede aus `r.occurrences` und `r.coverage` beantwortet. `newFakeRepo` legt die beiden Karten an.

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/application/ -run TestIngestSyntaxonDistribution -v`
Expected: FAIL — `undefined: application.IngestSyntaxonDistribution`.

- [ ] **Step 3: Implementieren**

`internal/application/syntaxon_distribution_ingest.go`:

```go
package application

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

const (
	fileSyntaxonDistribution         = "syntaxon_distribution.csv"
	fileSyntaxonDistributionCoverage = "syntaxon_distribution_coverage.csv"
)

// SyntaxonDistributionReport counts what a distribution ingest wrote and what
// it could not place. Every number is counted, none is estimated.
type SyntaxonDistributionReport struct {
	Written   int
	Verified  int
	Uncertain int
	Covered   int

	SkippedRows int

	// UnknownSyntaxa are source codes the index does not know, each listed
	// once however many rows it had, sorted. Measured against the pinned
	// artifacts this is exactly one entry, CI01E: it is in the distribution
	// file (EVC fassung 3, 2024-06-12) but not in the pinned FloraVeg file.
	// Real fassung drift between two sources, not a defect of this ingest —
	// and naming it is the point.
	UnknownSyntaxa []string
}

// IngestSyntaxonDistribution loads the two CSVs pipelines/evc-distribution
// produces into repo, in one transaction.
//
// A missing csvPath is "no syntaxon distribution pinned yet", not an error:
// the distribution is extra information, exactly like IngestDistribution's,
// and a five-minute ingest must not die over an optional source. The coverage
// file is the one exception — see below.
//
// Runs AFTER IngestSyntaxa: the syntaxon ids have to be in the index for
// UnknownSyntaxa to mean anything. No hostus involvement — this is a pure CSV
// source, and the ingest stays offline for it.
func IngestSyntaxonDistribution(ctx context.Context, repo output.Repository, csvPath, coveragePath string) (SyntaxonDistributionReport, error) {
	if _, err := os.Stat(csvPath); err != nil {
		if os.IsNotExist(err) {
			slog.InfoContext(ctx, "no syntaxon distribution file, skipping", "path", csvPath)
			return SyntaxonDistributionReport{}, nil
		}
		return SyntaxonDistributionReport{}, fmt.Errorf("statting %s: %w", csvPath, err)
	}
	// The distribution WITHOUT the coverage is the one combination that must
	// not pass. Every occurrence row would be written, and every alliance's
	// empty cell would then read as "nobody looked" instead of "does not
	// occur" — the four states collapse to three, silently, for the whole
	// index. Half of this source is worse than none of it.
	if _, err := os.Stat(coveragePath); err != nil {
		return SyntaxonDistributionReport{}, fmt.Errorf(
			"%s is present but %s is not; without the coverage rows an absence cannot be told from an unknown: %w",
			fileSyntaxonDistribution, fileSyntaxonDistributionCoverage, err)
	}

	known, err := knownSyntaxonIDs(ctx, repo)
	if err != nil {
		return SyntaxonDistributionReport{}, err
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return SyntaxonDistributionReport{}, fmt.Errorf("beginning syntaxon distribution transaction: %w", err)
	}
	rep, err := writeSyntaxonDistribution(ctx, tx, csvPath, coveragePath, known)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return SyntaxonDistributionReport{}, fmt.Errorf("%w (rollback also failed: %w)", err, rbErr)
		}
		return SyntaxonDistributionReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyntaxonDistributionReport{}, fmt.Errorf("committing syntaxon distribution transaction: %w", err)
	}

	if len(rep.UnknownSyntaxa) > 0 {
		slog.WarnContext(ctx, "the distribution source names syntaxa the index does not carry",
			"count", len(rep.UnknownSyntaxa), "codes", rep.UnknownSyntaxa)
	}
	if rep.SkippedRows > 0 {
		slog.WarnContext(ctx, "skipped malformed rows in the syntaxon distribution files",
			"path", csvPath, "skipped", rep.SkippedRows)
	}
	return rep, nil
}

func knownSyntaxonIDs(ctx context.Context, repo output.Repository) (map[string]bool, error) {
	all, err := repo.AllSyntaxa(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing syntaxa: %w", err)
	}
	known := make(map[string]bool, len(all))
	for _, s := range all {
		known[s.ID] = true
	}
	return known, nil
}
```

Zweite Hälfte derselben Datei — die beiden Leser, klein geschnitten, damit keine Funktion über 10 kommt:

```go
// unknownTracker records an id once, however many rows it has. One unknown
// alliance can carry up to 136 rows, and reporting it 136 times would turn
// one fassung drift into a wall of noise.
type unknownTracker struct {
	seen map[string]bool
	ids  []string
}

func (u *unknownTracker) add(id string) {
	if u.seen[id] {
		return
	}
	u.seen[id] = true
	u.ids = append(u.ids, id)
}

func writeSyntaxonDistribution(ctx context.Context, tx output.IngestTx, csvPath, coveragePath string, known map[string]bool) (SyntaxonDistributionReport, error) {
	var rep SyntaxonDistributionReport
	unknown := &unknownTracker{seen: map[string]bool{}}

	if err := readOccurrenceRows(ctx, tx, csvPath, known, unknown, &rep); err != nil {
		return SyntaxonDistributionReport{}, err
	}
	if err := readCoverageRows(ctx, tx, coveragePath, known, unknown, &rep); err != nil {
		return SyntaxonDistributionReport{}, err
	}

	slices.Sort(unknown.ids)
	rep.UnknownSyntaxa = unknown.ids
	return rep, nil
}

func readOccurrenceRows(ctx context.Context, tx output.IngestTx, csvPath string,
	known map[string]bool, unknown *unknownTracker, rep *SyntaxonDistributionReport) error {
	// splitCSVPath, not filepath.Split: a bare relative filename splits to
	// dir="", which os.OpenRoot does not read as the current directory.
	dir, file := splitCSVPath(csvPath)
	skip := newRowSkipper(&rep.SkippedRows, file, "syntaxon distribution")

	return readAll(ctx, dir, file, ',',
		[]string{"syntaxon_id", "area_scheme", "area_code", "occurrence"}, skip,
		func(idx map[string]int, row []string, line int) error {
			id := row[idx["syntaxon_id"]]
			a := domain.Area{Scheme: row[idx["area_scheme"]], Code: row[idx["area_code"]]}
			occurrence := row[idx["occurrence"]]

			if id == "" || !a.IsComplete() {
				skip(line, fmt.Errorf("incomplete row: syntaxon %q, area %s", id, a))
				return nil
			}
			if !domain.IsKnownAreaScheme(a.Scheme) {
				skip(line, fmt.Errorf("area scheme %q is not one of %v", a.Scheme, domain.KnownAreaSchemes()))
				return nil
			}
			// The pipeline aborts on an unknown cell value, so this row should
			// not exist. Skipped rather than written because the CHECK would
			// reject it and a failed transaction would lose every other row.
			if occurrence != domain.OccurrenceVerified && occurrence != domain.OccurrenceUncertain {
				skip(line, fmt.Errorf("occurrence %q is neither %q nor %q",
					occurrence, domain.OccurrenceVerified, domain.OccurrenceUncertain))
				return nil
			}
			// A code the index does not know is NOT written: a distribution row
			// pointing at nothing would answer no question and would make
			// AreasWithData offer a territory nobody can reach.
			if !known[id] {
				unknown.add(id)
				return nil
			}
			if err := tx.UpsertSyntaxonDistribution(id, a.Scheme, a.Code, occurrence); err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
			}
			rep.Written++
			if occurrence == domain.OccurrenceUncertain {
				rep.Uncertain++
				return nil
			}
			rep.Verified++
			return nil
		})
}

func readCoverageRows(ctx context.Context, tx output.IngestTx, coveragePath string,
	known map[string]bool, unknown *unknownTracker, rep *SyntaxonDistributionReport) error {
	dir, file := splitCSVPath(coveragePath)
	skip := newRowSkipper(&rep.SkippedRows, file, "syntaxon distribution coverage")

	return readAll(ctx, dir, file, ',', []string{"syntaxon_id", "area_scheme"}, skip,
		func(idx map[string]int, row []string, line int) error {
			id := row[idx["syntaxon_id"]]
			scheme := row[idx["area_scheme"]]
			if id == "" || scheme == "" {
				skip(line, fmt.Errorf("incomplete coverage row: syntaxon %q, scheme %q", id, scheme))
				return nil
			}
			if !domain.IsKnownAreaScheme(scheme) {
				skip(line, fmt.Errorf("area scheme %q is not one of %v", scheme, domain.KnownAreaSchemes()))
				return nil
			}
			if !known[id] {
				unknown.add(id)
				return nil
			}
			if err := tx.UpsertSyntaxonDistributionCoverage(id, scheme); err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
			}
			rep.Covered++
			return nil
		})
}
```

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./internal/application/ -run TestIngestSyntaxonDistribution -v`
Expected: PASS für alle neun Tests.

- [ ] **Step 5: Coverage des Pakets prüfen**

Run: `go test ./internal/application/ -cover`
Expected: **100.0 %**. Der Floor ist ein Raise-Only-Ratchet; fehlt eine Zeile, fehlt ein Testfall — nicht den Floor senken. Erfahrungsgemäß offen bleiben die `Stat`-Fehlerzweige, die nicht `IsNotExist` sind: ein Verzeichnis anstelle der CSV oder eine Datei ohne Leserecht deckt sie ab.

- [ ] **Step 6: Commit**

```bash
git add internal/application/
git commit -m "feat(ingest): Syntaxa-Verbreitung und Abdeckung aus CSV einlesen"
```

---

### Task 7: `GET /v1/areas?scheme=` (Blocker 3)

**Files:**
- Modify: `internal/ports/input/services.go` (`QueryService.Areas`)
- Modify: `internal/application/area.go`
- Modify: `internal/adapters/http/area.go`
- Modify: `internal/adapters/http/openapi.yaml`, `api/openapi/openapi.yaml`
- Test: `internal/application/area_test.go`, `internal/adapters/http/handlers_test.go`

**Interfaces:**
- Consumes: `Repository.AreasWithData(ctx, scheme)` (Task 4), `domain.IsKnownAreaScheme` (Task 3).
- Produces: `input.QueryService.Areas(ctx context.Context, scheme string) ([]AreaView, error)`; `GET /v1/areas` nimmt `?scheme=` mit Vorgabewert `wgsrpd_l3`.

**Blocker 3.** `input.QueryService.Areas(ctx)` nimmt **keinen** Schema-Parameter, und `handleAreas` gibt keinen weiter — `internal/application/area.go` verdrahtet `domain.SchemeWGSRPDL3` fest. `/v1/areas` bekommt deshalb `?scheme=` mit Vorgabewert `wgsrpd_l3`: damit bleibt **jede heutige Anfrage unverändert beantwortet**, und die Antwort mischt nie Codes zweier Schemata in eine flache Liste, was sie mehrdeutig machen würde (`albania` und `GER` nebeneinander, ohne dass ein Feld sagt, welches Vokabular gilt — `AreaView.scheme` trägt es je Eintrag, aber ein Client, der die Liste als Auswahlfeld anzeigt, hätte zwei Vokabulare in einem Feld). Ein unbekanntes Schema ist `INVALID_QUERY`.

Die Änderung bricht `fakeQueryService` in `internal/adapters/http/handlers_test.go` — die Signatur muss dort mitgezogen werden, sonst kompiliert das Paket nicht.

- [ ] **Step 1: Die failing Tests schreiben**

In `internal/application/area_test.go`:

```go
func TestAreasReichtDasSchemaDurch(t *testing.T) {
	repo := newFakeRepo()
	repo.areasWithData = map[string][]domain.NamedArea{
		domain.SchemeWGSRPDL3: {{Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}, NameEN: "Germany"}},
		domain.SchemeEVCTerritory: {
			{Area: domain.Area{Scheme: domain.SchemeEVCTerritory, Code: "austria-alps"}, NameEN: "Austria Alps"},
		},
	}
	q := application.NewQueryService(repo)

	terr, err := q.Areas(context.Background(), domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if len(terr) != 1 || terr[0].Scheme != domain.SchemeEVCTerritory || terr[0].Code != "austria-alps" {
		t.Errorf("Territorien = %+v", terr)
	}

	wg, err := q.Areas(context.Background(), domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if len(wg) != 1 || wg[0].Code != "GER" {
		t.Errorf("WGSRPD = %+v", wg)
	}
}
```

In `internal/adapters/http/handlers_test.go`:

```go
func TestAreasOhneSchemaAntwortetWieBisher(t *testing.T) {
	// Every request made before this release must keep its answer. The default
	// is not cosmetic: it is the compatibility promise.
	srv, q := newTestServer(t) // the package's existing helper
	getJSON(t, srv, "/v1/areas")
	if q.areaScheme != "wgsrpd_l3" {
		t.Errorf("Schema = %q, erwartet wgsrpd_l3", q.areaScheme)
	}
}

func TestAreasMitTerritoriumsschema(t *testing.T) {
	srv, q := newTestServer(t)
	getJSON(t, srv, "/v1/areas?scheme=evc_territory")
	if q.areaScheme != "evc_territory" {
		t.Errorf("Schema = %q, erwartet evc_territory", q.areaScheme)
	}
}

func TestAreasMitUnbekanntemSchemaIstInvalidQuery(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, scheme := range []string{"evc-territory", "iso3166", "WGSRPD_L3"} {
		res := get(t, srv, "/v1/areas?scheme="+scheme)
		if res.Code != http.StatusBadRequest {
			t.Errorf("scheme=%q: Status %d, erwartet 400", scheme, res.Code)
		}
		body := res.Body.String()
		// The message lists the allowed values — never an empty list, which
		// would look like "there are none" while meaning "you mistyped".
		for _, want := range []string{"INVALID_QUERY", "evc_territory", "wgsrpd_l3"} {
			if !strings.Contains(body, want) {
				t.Errorf("scheme=%q: Antwort nennt %q nicht: %s", scheme, want, body)
			}
		}
	}
}
```

`fakeQueryService` bekommt das Feld `areaScheme string` und merkt sich das Argument.

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/application/ -run TestAreasReicht -v && go test ./internal/adapters/http/ -run TestAreas -v`
Expected: FAIL — `too many arguments in call to q.Areas`, und im HTTP-Paket gibt es kein Feld `areaScheme`; `?scheme=evc-territory` antwortet 200.

- [ ] **Step 3: Port, Anwendung und Handler umstellen**

In `internal/ports/input/services.go`, in `QueryService`:

```go
	// Areas lists the areas of one scheme the ?area= filter can answer, with
	// their names. The scheme is a parameter and not a constant because situs
	// stores two (domain.SchemeWGSRPDL3 for species, domain.SchemeEVCTerritory
	// for syntaxa) and a flat list mixing both would be ambiguous: the caller
	// could not tell which vocabulary a code belongs to without reading every
	// entry's scheme field.
	Areas(ctx context.Context, scheme string) ([]AreaView, error)
```

In `internal/application/area.go`:

```go
// Areas lists the areas of one scheme the ?area= filter can actually answer,
// each with its ingested name. It is the discovery entry point for that
// filter, the same role Typologies plays for (typology, code) addressing.
//
// The scheme is validated at the HTTP boundary, not here: it is a query
// parameter, and rejecting a typo with the list of allowed values is the
// handler's job — the same place ?rank= and ?include= are checked.
func (q *QueryService) Areas(ctx context.Context, scheme string) ([]input.AreaView, error) {
	areas, err := q.repo.AreasWithData(ctx, scheme)
	if err != nil {
		return nil, fmt.Errorf("listing areas of scheme %q: %w", scheme, err)
	}
	out := make([]input.AreaView, 0, len(areas))
	for _, a := range areas {
		out = append(out, input.AreaView{Scheme: a.Scheme, Code: a.Code, Name: a.NameEN})
	}
	return out, nil
}
```

`domain` bleibt in den Importen der Datei — `areaLookup` benutzt es weiter.

In `internal/adapters/http/area.go`:

```go
package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
)

// handleAreas answers GET /v1/areas: the distribution areas the index has
// data for, with their names, sorted by code. It is the discovery entry point
// for ?area= — without it a client has to know the code table by heart, and
// still could not tell which codes THIS index can answer.
//
// Deliberately only the areas with data: an area nobody occurs in would be a
// filter that can only ever answer empty.
//
// ?scheme= defaults to wgsrpd_l3, which is what this route answered before
// the second scheme existed — so every request made until now keeps its
// answer. One scheme per request, never both merged: a flat list of
// "albania" next to "GER" would be two vocabularies in one selection.
func (s *Server) handleAreas(w http.ResponseWriter, r *http.Request) {
	scheme := strings.TrimSpace(r.URL.Query().Get("scheme"))
	if scheme == "" {
		scheme = domain.SchemeWGSRPDL3
	}
	if !domain.IsKnownAreaScheme(scheme) {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			fmt.Sprintf("scheme %q is unknown; allowed: %s",
				scheme, strings.Join(domain.KnownAreaSchemes(), ", ")))
		return
	}

	areas, err := s.deps.Query.Areas(r.Context(), scheme)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "listing areas", "error", err, "scheme", scheme)
		s.writeError(w, http.StatusInternalServerError, CodeInternalError, "internal error")
		return
	}
	s.writeJSON(w, http.StatusOK, areas)
}
```

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./internal/application/ ./internal/adapters/http/ -v`
Expected: PASS. Schlägt ein anderer Test fehl, weil er `Areas(ctx)` aufruft, die Signatur dort mitziehen — nicht den Test löschen.

- [ ] **Step 5: OpenAPI in beiden Kopien**

In `internal/adapters/http/openapi.yaml` unter `components/parameters` neben `Area`:

```yaml
    AreaScheme:
      name: scheme
      in: query
      required: false
      description: >-
        Das Gebietsschema. `wgsrpd_l3` (Vorgabe) sind die
        WGSRPD-Level-3-Gebiete der **Artverbreitung**; `evc_territory` sind
        die 136 Territorien der **Syntaxa-Verbreitung** (Staaten und
        biogeografische Teile davon, meist mit eigener Küstenspalte). Eine
        Abbildung zwischen beiden gibt es nicht und soll es nicht geben —
        keine Antwort rechnet ein Territorium in einen WGSRPD-Code um. Ein
        unbekanntes Schema ist INVALID_QUERY.
      schema:
        type: string
        enum: [wgsrpd_l3, evc_territory]
        default: wgsrpd_l3
```

und am Pfad `/v1/areas`:

```yaml
      operationId: areas
      parameters:
        - $ref: "#/components/parameters/AreaScheme"
      responses:
        "200":
          description: Alle Gebiete des Schemas mit Daten, nach `code` sortiert.
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/AreaView"
        "400":
          $ref: "#/components/responses/InvalidQuery"
        "500":
          $ref: "#/components/responses/InternalError"
```

Die `description` des Pfads ergänzen: es sind die Gebiete **eines** Schemas, und welche Tabelle die Abdeckung entscheidet, hängt vom Schema ab (`species_distribution` bzw. `syntaxon_distribution`). Die `description` von `AreaView` ergänzen: `scheme` ist `wgsrpd_l3` **oder** `evc_territory`, und `example` bleibt `wgsrpd_l3`.

Byte-Gleichheit sicherstellen:

```bash
cp internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
diff internal/adapters/http/openapi.yaml api/openapi/openapi.yaml && echo "identisch"
```

- [ ] **Step 6: Vertragstest laufen lassen**

Run: `go test ./internal/adapters/http/ -run 'TestRoutesMatchOpenAPISpec|TestOpenAPICopiesAreIdentical' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ports/input/services.go internal/application/area.go internal/adapters/http/ api/openapi/
git commit -m "feat(api): /v1/areas nimmt scheme, Vorgabe wgsrpd_l3"
```

---

### Task 8: `SyntaxonDetail.distribution` — vorhanden bei Abdeckung, fehlend ohne

**Files:**
- Modify: `internal/ports/input/services.go` (`SyntaxonDetail`, neu: `SyntaxonDistribution`)
- Create: `internal/application/syntaxon_distribution.go`
- Modify: `internal/application/query_syntaxon.go` (die Datei, in der Teilprojekt B `Syntaxon` anlegt — Name gemäß Repo)
- Modify: `internal/adapters/http/openapi.yaml`, `api/openapi/openapi.yaml`
- Test: `internal/application/syntaxon_distribution_test.go`, `internal/adapters/http/syntaxon_test.go`

**Interfaces:**
- Consumes: `Repository.SyntaxonDistribution` (Task 4); `input.SyntaxonDetail` (Teilprojekt B).
- Produces:

```go
// SyntaxonDistribution is the source's statement about where a syntaxon
// occurs. verified and uncertain are sorted code lists.
//
// Absence is deliberately NOT enumerated: 136 minus the occupied codes would
// be an invented list, and the client knows the scheme from GET /v1/areas
// ?scheme=evc_territory.
type SyntaxonDistribution struct {
	AreaScheme string   `json:"area_scheme"`
	Verified   []string `json:"verified"`
	Uncertain  []string `json:"uncertain"`
}
```

und in `SyntaxonDetail`:

```go
	// Distribution is the source's statement about this syntaxon. Nil when
	// there is none (no coverage row) — then NOTHING is known about its
	// occurrence, which is strictly different from "occurs nowhere". Measured:
	// 212 of 1326 alliances are nil, including every bryophyte, lichen and
	// algal one, because the source covers vascular-plant dominated vegetation
	// only.
	Distribution *SyntaxonDistribution `json:"distribution,omitempty"`
```

**Eigene Anwendungs-Datei, kein Zuwachs in `query.go`.** `internal/application/query.go` sitzt beim Funktionsdeckel auf exakt 10 und beim Dateideckel auf der Baseline 72. Die Ableitung kommt deshalb als `syntaxonDistributionOf` in eine neue Datei; die Datei aus Teilprojekt B gewinnt nur einen Aufruf.

- [ ] **Step 1: Die failing Tests schreiben**

`internal/application/syntaxon_distribution_test.go`:

```go
func TestSyntaxonDetailTraegtVerbreitungBeiAbdeckung(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	repo.distributions["CA01A|evc_territory"] = domain.SyntaxonDistribution{
		Scheme: domain.SchemeEVCTerritory, Covered: true,
		Verified:  []string{"albania", "austria-alps"},
		Uncertain: []string{"czech-republic"},
	}
	q := application.NewQueryService(repo)

	got, err := q.Syntaxon(context.Background(), "CA01A", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.Distribution == nil {
		t.Fatal("Distribution ist nil, obwohl eine Coverage-Zeile vorliegt")
	}
	if got.Distribution.AreaScheme != domain.SchemeEVCTerritory {
		t.Errorf("AreaScheme = %q", got.Distribution.AreaScheme)
	}
	if !slices.Equal(got.Distribution.Verified, []string{"albania", "austria-alps"}) {
		t.Errorf("Verified = %v", got.Distribution.Verified)
	}
	if !slices.Equal(got.Distribution.Uncertain, []string{"czech-republic"}) {
		t.Errorf("Uncertain = %v", got.Distribution.Uncertain)
	}
}

func TestSyntaxonDetailLaesstVerbreitungOhneAbdeckungWeg(t *testing.T) {
	// The bryophyte case: no statement at all. Nil, never an empty list — an
	// empty list would read as "occurs nowhere", which the source never said.
	repo := newFakeRepo()
	seedSyntaxa(repo, "RA01A")
	q := application.NewQueryService(repo)

	got, err := q.Syntaxon(context.Background(), "RA01A", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.Distribution != nil {
		t.Errorf("Distribution = %+v, erwartet nil", got.Distribution)
	}
}

func TestSyntaxonDetailTraegtLeereListenBeiGeprueftemNichtvorkommen(t *testing.T) {
	// Covered, no occurrence: "checked, occurs in no territory". THIS is the
	// state that must arrive as a present object with two empty lists — the
	// only way a client can tell it from the nil case above.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01B")
	repo.distributions["CA01B|evc_territory"] = domain.SyntaxonDistribution{
		Scheme: domain.SchemeEVCTerritory, Covered: true,
		Verified: []string{}, Uncertain: []string{},
	}
	q := application.NewQueryService(repo)

	got, err := q.Syntaxon(context.Background(), "CA01B", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.Distribution == nil {
		t.Fatal("Distribution ist nil, obwohl die Quelle geprueft hat")
	}
	if len(got.Distribution.Verified) != 0 || len(got.Distribution.Uncertain) != 0 {
		t.Errorf("Distribution = %+v, erwartet zwei leere Listen", got.Distribution)
	}
}
```

`fakeRepo` bekommt `distributions map[string]domain.SyntaxonDistribution` und beantwortet `SyntaxonDistribution` daraus (fehlender Schlüssel ⇒ `Covered: false` und zwei leere Listen).

In `internal/adapters/http/syntaxon_test.go`:

```go
func TestSyntaxonJSONTraegtVerbreitungMitLeerenListen(t *testing.T) {
	// The JSON shape is the contract: "distribution" present with two empty
	// arrays means "checked, occurs nowhere"; absent means "nobody looked".
	srv, _ := newTestServer(t)
	body := getJSON(t, srv, "/v1/syntaxon/CA01B")
	dist, ok := body["distribution"].(map[string]any)
	if !ok {
		t.Fatalf("distribution fehlt oder ist kein Objekt: %v", body["distribution"])
	}
	if dist["area_scheme"] != "evc_territory" {
		t.Errorf("area_scheme = %v", dist["area_scheme"])
	}
	for _, field := range []string{"verified", "uncertain"} {
		list, ok := dist[field].([]any)
		if !ok {
			t.Errorf("%s ist kein Array: %v", field, dist[field])
			continue
		}
		if len(list) != 0 {
			t.Errorf("%s = %v, erwartet leer", field, list)
		}
	}
}

func TestSyntaxonJSONLaesstDistributionOhneAbdeckungWeg(t *testing.T) {
	srv, _ := newTestServer(t)
	body := getJSON(t, srv, "/v1/syntaxon/RA01A")
	if _, ok := body["distribution"]; ok {
		t.Errorf("distribution erscheint, obwohl keine Aussage vorliegt: %v", body["distribution"])
	}
}
```

Die Fixture des HTTP-Pakets braucht dafür zwei Syntaxa: `CA01B` mit Coverage und ohne Vorkommen, `RA01A` ohne Coverage.

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/application/ -run TestSyntaxonDetail -v && go test ./internal/adapters/http/ -run TestSyntaxonJSON -v`
Expected: FAIL — `got.Distribution undefined`, und im JSON fehlt der Schlüssel.

- [ ] **Step 3: Implementieren**

In `internal/ports/input/services.go` das Feld und den Typ aus dem Interfaces-Abschnitt oben anlegen. `SyntaxonDistribution` steht **nicht** in `SyntaxonRef`: `GET /v1/syntaxa` liefert 1326 Verbände, und je Eintrag eine Codeliste wäre eine Antwort, die niemand angefordert hat — dort trägt der Eintrag nur `occurrence` (Task 9).

`internal/application/syntaxon_distribution.go`:

```go
package application

import (
	"context"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// syntaxonDistributionOf turns the repository's four-state value into the
// read API's three fields plus presence.
//
// The pointer IS the fourth state. Covered false becomes nil, which the JSON
// omits, and a client then knows nothing was claimed. Returning an empty
// object instead would claim "occurs in none of the 136 territories" for
// every bryophyte, lichen and algal alliance — 212 of 1326 rows, none of
// which the source says anything about.
func syntaxonDistributionOf(ctx context.Context, repo interface {
	SyntaxonDistribution(ctx context.Context, syntaxonID, scheme string) (domain.SyntaxonDistribution, error)
}, syntaxonID string) (*input.SyntaxonDistribution, error) {
	d, err := repo.SyntaxonDistribution(ctx, syntaxonID, domain.SchemeEVCTerritory)
	if err != nil {
		return nil, fmt.Errorf("reading distribution of syntaxon %q: %w", syntaxonID, err)
	}
	if !d.Covered {
		return nil, nil
	}
	// Both lists are non-nil even when empty: an omitted array and an empty
	// array are not the same thing for a client, and "checked, occurs nowhere"
	// must arrive as two empty arrays rather than two missing fields.
	verified, uncertain := d.Verified, d.Uncertain
	if verified == nil {
		verified = []string{}
	}
	if uncertain == nil {
		uncertain = []string{}
	}
	return &input.SyntaxonDistribution{
		AreaScheme: d.Scheme,
		Verified:   verified,
		Uncertain:  uncertain,
	}, nil
}
```

Das anonyme Interface im Parameter hält die Funktion an der einen Methode fest, die sie braucht, statt die ganze `output.Repository` hereinzuziehen — `q.repo` erfüllt es. Ist das dem Paketstil fremd, stattdessen `output.Repository` nehmen; die Zusage bleibt dieselbe.

In der Datei aus Teilprojekt B, in `QueryService.Syntaxon`, direkt vor dem `return`:

```go
	dist, err := syntaxonDistributionOf(ctx, q.repo, id)
	if err != nil {
		return input.SyntaxonDetail{}, err
	}
	detail.Distribution = dist
```

Das ist **ein** `if err != nil` mehr in `Syntaxon`. Läuft die Funktion damit über 10, wandert der Aufruf mit in `syntaxonDistributionOf`, also in eine Funktion, die Detail und Verbreitung zusammensetzt — nicht die Baseline anheben.

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./internal/application/ ./internal/adapters/http/ -v`
Expected: PASS.

- [ ] **Step 5: OpenAPI in beiden Kopien**

Neues Schema, und `SyntaxonDetail` bekommt die Eigenschaft:

```yaml
    SyntaxonDistribution:
      description: >-
        Die Verbreitungsaussage der Quelle (Preislerová et al. 2022/2024,
        CC-BY 4.0) über ein Syntaxon. `verified` und `uncertain` sind
        sortierte Codelisten des Schemas `evc_territory` — die Codes selbst
        listet `GET /v1/areas?scheme=evc_territory`.

        **Abwesenheit wird nicht aufgezählt.** 136 minus die belegten Codes
        wäre eine erfundene Liste. Zwei leere Listen bedeuten „geprüft, kommt
        in keinem Territorium vor"; **fehlt das ganze Objekt**, liegt
        überhaupt keine Aussage vor. Das sind zwei verschiedene Dinge: die
        Quelle deckt nur *vascular-plant dominated vegetation* ab, weshalb
        gemessen 212 der 1326 Verbände — darunter alle 190 Moos-, Flechten-
        und Algenverbände — kein `distribution` tragen.
      type: object
      required: [area_scheme, verified, uncertain]
      properties:
        area_scheme:
          type: string
          enum: [evc_territory]
        verified:
          type: array
          description: Codes mit belegtem Vorkommen (Quellwert `1`).
          items:
            type: string
          example: [austria-alps, czech-republic]
        uncertain:
          type: array
          description: Codes mit unsicherem Vorkommen (Quellwert `U`).
          items:
            type: string
          example: [poland-sudetes]
```

und in `SyntaxonDetail` (dem `allOf`-Zweig mit den Zusatzfeldern):

```yaml
        distribution:
          allOf:
            - $ref: "#/components/schemas/SyntaxonDistribution"
          description: >-
            Fehlt, wenn die Quelle über dieses Syntaxon keine Aussage macht —
            dann ist NICHTS über sein Vorkommen bekannt, was von „kommt
            nirgends vor" streng zu trennen ist.
```

Byte-Gleichheit:

```bash
cp internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
diff internal/adapters/http/openapi.yaml api/openapi/openapi.yaml && echo "identisch"
```

- [ ] **Step 6: Commit**

```bash
git add internal/ports/input/services.go internal/application/ internal/adapters/http/ api/openapi/
git commit -m "feat(api): SyntaxonDetail traegt die Verbreitungsaussage, fehlt ohne Abdeckung"
```

---

### Task 9: `GET /v1/syntaxa?area=&include=`

**Files:**
- Modify: `internal/ports/input/services.go` (`SyntaxonRef.Occurrence`, `SyntaxonAreaFilter`, `SyntaxaByRank`)
- Modify: `internal/application/syntaxon_distribution.go`
- Modify: `internal/application/query_syntaxon.go` (`SyntaxaByRank`)
- Modify: `internal/adapters/http/syntaxon.go` (der Handler aus Teilprojekt B)
- Modify: `internal/adapters/http/openapi.yaml`, `api/openapi/openapi.yaml`
- Test: `internal/application/syntaxon_distribution_test.go`, `internal/adapters/http/syntaxon_test.go`

**Interfaces:**
- Consumes: `Repository.SyntaxonOccurrencesInArea`, `Repository.SyntaxaWithCoverage`, `Repository.KnownAreaCodes` (Task 4); `SyntaxaByRank` (Teilprojekt B).
- Produces:

```go
// SyntaxonAreaFilter is the ?area=/?include= pair on the syntaxa list.
//
// Include is the set of occurrence values that count as a hit, default
// {verified}. It is a SET rather than a single value because "verified or
// uncertain" is a real question and two requests plus a client-side merge
// would be a worse answer.
type SyntaxonAreaFilter struct {
	Code    string
	Include []string
}

func (f SyntaxonAreaFilter) Active() bool { return f.Code != "" }

// SyntaxaByRank filters by rank and, when non-empty, by the life-form group
// of the reachable formation. filter, when active, keeps only syntaxa with a
// matching occurrence PLUS those the source says nothing about.
SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string, filter SyntaxonAreaFilter) ([]SyntaxonRef, error)
```

und an `SyntaxonRef`:

```go
	// Occurrence is "verified" or "uncertain" and is set only in an
	// ?area=-filtered list. MISSING means no statement exists — the same
	// three-valuedness as in_area on the species side. Without it the carried
	// row would be indistinguishable from a confirmed one, and a list that
	// keeps everything but marks nothing is just as dishonest as one that
	// throws away.
	Occurrence string `json:"occurrence,omitempty"`
```

**Die Filterprüfung liegt im Handler**, wie Teilprojekt B es vorbereitet hat. Fünf Ablehnungen, alle `INVALID_QUERY`:

| Fall | Warum |
|---|---|
| `?include=` ohne `?area=` | Ohne Gebiet ist `include` wirkungslos; es stillschweigend zu ignorieren behauptet eine Wirkung |
| `?include=` mit unbekanntem Element | Ein ignorierter Filter ist schlimmer als ein abgelehnter |
| `?include=` mit leerem Element (`verified,,uncertain` oder `include=`) | Dasselbe; `include=` allein ist kein „Vorgabe" |
| `?include=` zweimal angegeben | Welche der beiden Angaben gälte, wäre geraten |
| `?area=` mit `rank=formation` (also auch ohne `?rank=`) | Formationen tragen keine Verbreitung; der Filter wäre wirkungslos. Die Meldung nennt, dass `?area=` einen Rang mit Verbreitungsdaten braucht — heute `alliance` |
| `?area=` mit unbekanntem Code | Nie eine Liste von „kommt nicht vor"; kommt als `ErrUnknownArea` aus der Anwendung |

- [ ] **Step 1: Die failing Tests schreiben**

In `internal/application/syntaxon_distribution_test.go`:

```go
func seedAreaFixture(repo *fakeRepo) {
	// CA01A verified in austria-alps, CA01B uncertain there, CA01C covered but
	// absent, RA01A not covered at all.
	seedSyntaxa(repo, "CA01A", "CA01B", "CA01C", "RA01A")
	repo.occurrences[fakeOccurrence{"CA01A", "evc_territory", "austria-alps"}] = domain.OccurrenceVerified
	repo.occurrences[fakeOccurrence{"CA01B", "evc_territory", "austria-alps"}] = domain.OccurrenceUncertain
	for _, id := range []string{"CA01A", "CA01B", "CA01C"} {
		repo.coverage[id+"|evc_territory"] = true
	}
	repo.knownAreaCodes["evc_territory"] = []string{"austria-alps", "albania"}
}

func TestSyntaxaByRankMitGebietBehaeltDieUnbeurteilbaren(t *testing.T) {
	repo := newFakeRepo()
	seedAreaFixture(repo)
	q := application.NewQueryService(repo)

	got, err := q.SyntaxaByRank(context.Background(), domain.SyntaxonRankAlliance, "",
		input.SyntaxonAreaFilter{Code: "austria-alps", Include: []string{domain.OccurrenceVerified}})
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	ids := map[string]string{}
	for _, r := range got {
		ids[r.ID] = r.Occurrence
	}
	// CA01A matches; RA01A has no statement and is CARRIED, unmarked; CA01B
	// (uncertain, not included) and CA01C (a definite absence) are dropped.
	if len(ids) != 2 {
		t.Fatalf("Ergebnis = %v, erwartet genau CA01A und RA01A", ids)
	}
	if ids["CA01A"] != domain.OccurrenceVerified {
		t.Errorf("CA01A.occurrence = %q, erwartet verified", ids["CA01A"])
	}
	if occ, ok := ids["RA01A"]; !ok || occ != "" {
		t.Errorf("RA01A = %q/%v, erwartet mitgefuehrt und unmarkiert", occ, ok)
	}
}

func TestSyntaxaByRankMitIncludeUncertain(t *testing.T) {
	repo := newFakeRepo()
	seedAreaFixture(repo)
	q := application.NewQueryService(repo)

	got, err := q.SyntaxaByRank(context.Background(), domain.SyntaxonRankAlliance, "",
		input.SyntaxonAreaFilter{Code: "austria-alps", Include: []string{domain.OccurrenceUncertain}})
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	for _, r := range got {
		if r.ID == "CA01A" {
			t.Error("CA01A erscheint bei include=uncertain")
		}
		if r.ID == "CA01B" && r.Occurrence != domain.OccurrenceUncertain {
			t.Errorf("CA01B.occurrence = %q", r.Occurrence)
		}
	}
}

func TestSyntaxaByRankMitBeidenAuspraegungen(t *testing.T) {
	repo := newFakeRepo()
	seedAreaFixture(repo)
	q := application.NewQueryService(repo)

	got, err := q.SyntaxaByRank(context.Background(), domain.SyntaxonRankAlliance, "",
		input.SyntaxonAreaFilter{Code: "austria-alps",
			Include: []string{domain.OccurrenceVerified, domain.OccurrenceUncertain}})
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("Ergebnis = %d Zeilen, erwartet 3 (CA01A, CA01B, RA01A)", len(got))
	}
}

func TestSyntaxaByRankOhneFilterMarkiertNichts(t *testing.T) {
	repo := newFakeRepo()
	seedAreaFixture(repo)
	q := application.NewQueryService(repo)

	got, err := q.SyntaxaByRank(context.Background(), domain.SyntaxonRankAlliance, "",
		input.SyntaxonAreaFilter{})
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("Ergebnis = %d Zeilen, erwartet alle 4", len(got))
	}
	for _, r := range got {
		if r.Occurrence != "" {
			t.Errorf("%s traegt occurrence %q ohne aktiven Filter", r.ID, r.Occurrence)
		}
	}
}

func TestSyntaxaByRankMitUnbekanntemGebietscode(t *testing.T) {
	repo := newFakeRepo()
	seedAreaFixture(repo)
	q := application.NewQueryService(repo)

	_, err := q.SyntaxaByRank(context.Background(), domain.SyntaxonRankAlliance, "",
		input.SyntaxonAreaFilter{Code: "gibtsnicht", Include: []string{domain.OccurrenceVerified}})
	if !errors.Is(err, input.ErrUnknownArea) {
		t.Errorf("Fehler = %v, erwartet ErrUnknownArea", err)
	}
}
```

In `internal/adapters/http/syntaxon_test.go`:

```go
func TestSyntaxaAreaFilterErreichtDenPort(t *testing.T) {
	srv, q := newTestServer(t)
	getJSON(t, srv, "/v1/syntaxa?rank=alliance&area=austria-alps&include=uncertain,verified")
	want := input.SyntaxonAreaFilter{Code: "austria-alps",
		Include: []string{"uncertain", "verified"}}
	if !reflect.DeepEqual(q.syntaxonAreaFilter, want) {
		t.Errorf("Filter = %+v, erwartet %+v", q.syntaxonAreaFilter, want)
	}
}

func TestSyntaxaAreaOhneIncludeIstVerified(t *testing.T) {
	srv, q := newTestServer(t)
	getJSON(t, srv, "/v1/syntaxa?rank=alliance&area=austria-alps")
	if !reflect.DeepEqual(q.syntaxonAreaFilter.Include, []string{"verified"}) {
		t.Errorf("Include = %v, erwartet [verified]", q.syntaxonAreaFilter.Include)
	}
}

func TestSyntaxaAbgelehnteFilterkombinationen(t *testing.T) {
	srv, _ := newTestServer(t)
	for name, tc := range map[string]struct{ path, mentions string }{
		"include ohne area":       {"/v1/syntaxa?rank=alliance&include=verified", "area"},
		"unbekanntes include":     {"/v1/syntaxa?rank=alliance&area=austria-alps&include=probable", "probable"},
		"leeres include":          {"/v1/syntaxa?rank=alliance&area=austria-alps&include=", "include"},
		"leeres Element":          {"/v1/syntaxa?rank=alliance&area=austria-alps&include=verified,,uncertain", "include"},
		"include zweimal":         {"/v1/syntaxa?rank=alliance&area=austria-alps&include=verified&include=uncertain", "include"},
		"area mit rank=formation": {"/v1/syntaxa?rank=formation&area=austria-alps", "rank"},
		"area ohne rank":          {"/v1/syntaxa?area=austria-alps", "rank"},
	} {
		t.Run(name, func(t *testing.T) {
			res := get(t, srv, tc.path)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("Status %d, erwartet 400", res.Code)
			}
			body := res.Body.String()
			if !strings.Contains(body, "INVALID_QUERY") {
				t.Errorf("Antwort ohne INVALID_QUERY: %s", body)
			}
			if !strings.Contains(body, tc.mentions) {
				t.Errorf("Antwort nennt %q nicht: %s", tc.mentions, body)
			}
		})
	}
}

func TestSyntaxaUnbekanntesGebietIstInvalidQuery(t *testing.T) {
	srv, q := newTestServer(t)
	q.err = fmt.Errorf("area %q: %w", "gibtsnicht", input.ErrUnknownArea)
	res := get(t, srv, "/v1/syntaxa?rank=alliance&area=gibtsnicht")
	if res.Code != http.StatusBadRequest {
		t.Errorf("Status %d, erwartet 400", res.Code)
	}
}

func TestSyntaxaJSONZeigtOccurrenceUndLaesstEsWeg(t *testing.T) {
	// The three-valuedness on the wire: a marked hit and a carried
	// unjudgeable row in the SAME list, distinguishable only by the field's
	// presence.
	srv, _ := newTestServer(t)
	body := getJSONArray(t, srv, "/v1/syntaxa?rank=alliance&area=austria-alps")
	byID := map[string]map[string]any{}
	for _, entry := range body {
		e := entry.(map[string]any)
		byID[e["id"].(string)] = e
	}
	if got := byID["CA01A"]["occurrence"]; got != "verified" {
		t.Errorf("CA01A.occurrence = %v, erwartet verified", got)
	}
	if _, ok := byID["RA01A"]["occurrence"]; ok {
		t.Error("RA01A traegt occurrence, obwohl keine Aussage vorliegt")
	}
}
```

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/application/ -run TestSyntaxaByRank -v && go test ./internal/adapters/http/ -run TestSyntaxa -v`
Expected: FAIL — `too many arguments in call to q.SyntaxaByRank`, `undefined: input.SyntaxonAreaFilter`, und die Filterkombinationen antworten 200.

- [ ] **Step 3: Port und Anwendung implementieren**

In `internal/ports/input/services.go` `SyntaxonAreaFilter`, das Feld `SyntaxonRef.Occurrence` und die neue `SyntaxaByRank`-Signatur aus dem Interfaces-Abschnitt anlegen.

In `internal/application/syntaxon_distribution.go`:

```go
// filterSyntaxaByArea marks the hits and drops the definite misses, keeping
// every row the source says nothing about.
//
// Three outcomes per row, and the third is the one that is easy to get wrong:
//   - an occurrence in Include        -> kept, marked
//   - covered, but no matching row    -> dropped; this is a DEFINITE statement
//   - not covered                     -> kept, UNMARKED; nothing was claimed
//
// Same rule only_in_area follows on the species side: a list that silently
// loses what it cannot judge is dishonestly clean.
func filterSyntaxaByArea(refs []input.SyntaxonRef, occurrences map[string]string,
	covered map[string]bool, include []string) []input.SyntaxonRef {
	out := make([]input.SyntaxonRef, 0, len(refs))
	for _, r := range refs {
		occ, hasRow := occurrences[r.ID]
		switch {
		case hasRow && slices.Contains(include, occ):
			r.Occurrence = occ
			out = append(out, r)
		case covered[r.ID]:
			// The source looked and did not record this occurrence here.
			continue
		default:
			out = append(out, r)
		}
	}
	return out
}

// syntaxonAreaLookup validates the code against the index and reads the two
// maps the filter needs. An unknown code is an error, never a list of "does
// not occur" — a typo and a real absence must not look the same.
func (q *QueryService) syntaxonAreaLookup(ctx context.Context, filter input.SyntaxonAreaFilter) (map[string]string, map[string]bool, error) {
	known, err := q.repo.KnownAreaCodes(ctx, domain.SchemeEVCTerritory)
	if err != nil {
		return nil, nil, err
	}
	if !slices.Contains(known, filter.Code) {
		return nil, nil, fmt.Errorf("area %q: %w", filter.Code, input.ErrUnknownArea)
	}
	occurrences, err := q.repo.SyntaxonOccurrencesInArea(ctx, domain.SchemeEVCTerritory, filter.Code)
	if err != nil {
		return nil, nil, err
	}
	covered, err := q.repo.SyntaxaWithCoverage(ctx, domain.SchemeEVCTerritory)
	if err != nil {
		return nil, nil, err
	}
	return occurrences, covered, nil
}
```

In `SyntaxaByRank` (Teilprojekt B), am Ende, statt des bisherigen `return`:

```go
	if !filter.Active() {
		return refs, nil
	}
	occurrences, covered, err := q.syntaxonAreaLookup(ctx, filter)
	if err != nil {
		return nil, err
	}
	return filterSyntaxaByArea(refs, occurrences, covered, filter.Include), nil
```

- [ ] **Step 4: Handler implementieren**

In `internal/adapters/http/syntaxon.go`, in `handleSyntaxa`, nach der bestehenden Rang- und Gruppenprüfung:

```go
	filter, ferr := syntaxonAreaFilter(r, rank)
	if ferr != nil {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, ferr.Error())
		return
	}
```

und als eigene Funktion in derselben Datei — klein geschnitten, weil sechs Ablehnungen in einer Funktion den Deckel von 10 reißen:

```go
// includeValues are the occurrence values ?include= accepts. Fixed on
// purpose, unlike ?rank=: this set is a schema CHECK, not an extension point.
var includeValues = []string{domain.OccurrenceUncertain, domain.OccurrenceVerified}

// syntaxonAreaFilter parses ?area= and ?include= for GET /v1/syntaxa.
//
// Nothing here is silently tolerated. An ignored filter parameter is worse
// than a rejected one: the client gets an answer that looks filtered and is
// not, and nothing in the response says so.
func syntaxonAreaFilter(r *http.Request, rank string) (input.SyntaxonAreaFilter, error) {
	q := r.URL.Query()
	code := strings.TrimSpace(q.Get("area"))
	raw, given := q["include"]

	if code == "" {
		if given {
			return input.SyntaxonAreaFilter{}, fmt.Errorf(
				"include needs an area; without one it would have no effect")
		}
		return input.SyntaxonAreaFilter{}, nil
	}
	// Formations carry no distribution, and rank defaults to formation — so a
	// bare ?area= would filter nothing at all. Naming the rank that does carry
	// data is the difference between a rejection and a riddle.
	if rank == domain.SyntaxonRankFormation {
		return input.SyntaxonAreaFilter{}, fmt.Errorf(
			"area needs a rank that carries distribution data (today: %s); rank=%s carries none",
			domain.SyntaxonRankAlliance, rank)
	}

	include, err := parseInclude(raw, given)
	if err != nil {
		return input.SyntaxonAreaFilter{}, err
	}
	return input.SyntaxonAreaFilter{Code: code, Include: include}, nil
}

// parseInclude turns the comma-separated set into a sorted slice. Sorted so
// two requests that differ only in order are the same request.
func parseInclude(raw []string, given bool) ([]string, error) {
	if !given {
		return []string{domain.OccurrenceVerified}, nil
	}
	if len(raw) > 1 {
		return nil, fmt.Errorf("include was given %d times; which one applies would be a guess", len(raw))
	}
	out := []string{}
	for _, part := range strings.Split(raw[0], ",") {
		value := strings.TrimSpace(part)
		if value == "" {
			return nil, fmt.Errorf("include has an empty element; allowed: %s",
				strings.Join(includeValues, ", "))
		}
		if !slices.Contains(includeValues, value) {
			return nil, fmt.Errorf("include value %q is unknown; allowed: %s",
				value, strings.Join(includeValues, ", "))
		}
		if !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	slices.Sort(out)
	return out, nil
}
```

`include=` allein liefert `raw[0] == ""`, `strings.Split` also `[""]` — und damit die Ablehnung „empty element". Das ist gewollt: ein leerer Parameter ist keine Vorgabeanforderung.

- [ ] **Step 5: Tests laufen lassen und grün sehen**

Run: `go test ./internal/application/ ./internal/adapters/http/ -v`
Expected: PASS, alle sieben Unterfälle von `TestSyntaxaAbgelehnteFilterkombinationen` inbegriffen.

- [ ] **Step 6: OpenAPI in beiden Kopien**

Zwei benannte Parameter neben `Rank` und `LifeFormGroup` aus Teilprojekt B:

```yaml
    SyntaxonArea:
      name: area
      in: query
      required: false
      description: >-
        Territoriumscode des Schemas `evc_territory` (Liste:
        `GET /v1/areas?scheme=evc_territory`). Gefiltert wird auf Syntaxa mit
        einer passenden Verbreitungszeile; Syntaxa **ohne** Aussage bleiben in
        der Liste und sind daran erkennbar, dass ihnen `occurrence` fehlt —
        dieselbe Dreiwertigkeit wie `in_area` bei den Arten.

        Braucht einen `rank`, der Verbreitungsdaten trägt (heute `alliance`);
        mit dem Vorgabewert `rank=formation` ist die Kombination
        INVALID_QUERY. Ein Code, den das Schema nicht kennt, ebenso — nie
        eine Liste von „kommt nicht vor".
      schema:
        type: string
        example: austria-alps
    SyntaxonInclude:
      name: include
      in: query
      required: false
      description: >-
        Kommagetrennte Menge aus `verified` und `uncertain`, in beliebiger
        Reihenfolge; Vorgabe ist `verified`. `include=uncertain` allein ist
        gültig. Ein unbekanntes Element, ein leerer Eintrag und ein doppelt
        angegebener Parameter sind INVALID_QUERY — eine stillschweigend
        ignorierte Filterangabe wäre schlimmer als eine abgelehnte. Ohne
        `area` ist der Parameter wirkungslos und deshalb ebenfalls
        INVALID_QUERY, statt vorzugeben, er hätte gewirkt.
      schema:
        type: string
        example: verified,uncertain
```

Am Pfad `/v1/syntaxa` beide unter `parameters` einhängen und `"400": $ref: "#/components/responses/InvalidQuery"` sicherstellen. In `SyntaxonRef` die Eigenschaft ergänzen:

```yaml
        occurrence:
          type: string
          enum: [verified, uncertain]
          description: >-
            Nur in einer mit `?area=` gefilterten Liste gesetzt. **Fehlt**,
            wenn die Quelle über dieses Syntaxon keine Aussage macht — die
            Zeile wird dann mitgeführt, aber nicht als Vorkommen behauptet.
```

Byte-Gleichheit:

```bash
cp internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
diff internal/adapters/http/openapi.yaml api/openapi/openapi.yaml && echo "identisch"
```

- [ ] **Step 7: Commit**

```bash
git add internal/ports/input/services.go internal/application/ internal/adapters/http/ api/openapi/
git commit -m "feat(api): /v1/syntaxa nimmt area und include, Unbeurteilbare bleiben unmarkiert stehen"
```

---

### Task 10: Zwei gemessene Felder in `GET /v1/info`

**Files:**
- Modify: `internal/ports/input/services.go` (`IndexInfo`)
- Modify: `internal/application/query.go` (`IndexInfo`)
- Modify: `internal/adapters/http/openapi.yaml`, `api/openapi/openapi.yaml`
- Test: `internal/application/query_test.go`, `internal/adapters/http/handlers_test.go`

**Interfaces:**
- Consumes: `Repository.SyntaxaWithCoverage` (Task 4).
- Produces: `IndexInfo.SyntaxonAreaScheme string` (`json:"syntaxon_area_scheme"`), `IndexInfo.SyntaxaWithDistribution int` (`json:"syntaxa_with_distribution"`).

**Das bestehende Feld heißt `area_scheme` (Einzahl) und meint das der Artverbreitung; es wird nicht umbenannt.** Ein Feldname in einer veröffentlichten Antwort ist keine Geschmacksfrage. Die beiden Namen stehen damit nebeneinander, und die Beschreibung im OpenAPI sagt bei **beiden** ausdrücklich, wessen Verbreitung sie betreffen — nebeneinanderstehende Namen ohne diese Unterscheidung wären schlimmer als der Umbau, den sie vermeiden.

`IndexInfo` in `query.go` gewinnt **ein** `if err != nil`. Die Funktion muss nach der Änderung unter 10 bleiben; reißt sie den Deckel, wandert die Syntaxa-Messung in eine kleine `syntaxonIndexInfo`-Funktion in `internal/application/syntaxon_distribution.go` — nicht die Baseline anheben.

- [ ] **Step 1: Die failing Tests schreiben**

In `internal/application/query_test.go`:

```go
func TestIndexInfoMisstSyntaxaVerbreitung(t *testing.T) {
	repo := newFakeRepo()
	repo.coverage["CA01A|evc_territory"] = true
	repo.coverage["CA01B|evc_territory"] = true
	q := application.NewQueryService(repo)

	info, err := q.IndexInfo(context.Background())
	if err != nil {
		t.Fatalf("IndexInfo: %v", err)
	}
	if info.SyntaxonAreaScheme != domain.SchemeEVCTerritory {
		t.Errorf("SyntaxonAreaScheme = %q, erwartet evc_territory", info.SyntaxonAreaScheme)
	}
	if info.SyntaxaWithDistribution != 2 {
		t.Errorf("SyntaxaWithDistribution = %d, erwartet 2", info.SyntaxaWithDistribution)
	}
	// The existing field keeps its meaning and its name.
	if info.AreaScheme != domain.SchemeWGSRPDL3 {
		t.Errorf("AreaScheme = %q, erwartet wgsrpd_l3", info.AreaScheme)
	}
}

func TestIndexInfoZaehltNullOhneVerbreitungsingest(t *testing.T) {
	// Zero is a true statement about this index, not a placeholder.
	repo := newFakeRepo()
	q := application.NewQueryService(repo)

	info, err := q.IndexInfo(context.Background())
	if err != nil {
		t.Fatalf("IndexInfo: %v", err)
	}
	if info.SyntaxaWithDistribution != 0 {
		t.Errorf("SyntaxaWithDistribution = %d, erwartet 0", info.SyntaxaWithDistribution)
	}
	if info.SyntaxonAreaScheme != domain.SchemeEVCTerritory {
		t.Errorf("SyntaxonAreaScheme = %q — das Schema ist auch ohne Daten benannt", info.SyntaxonAreaScheme)
	}
}
```

In `internal/adapters/http/handlers_test.go`:

```go
func TestInfoJSONTraegtBeideGebietsschemata(t *testing.T) {
	srv, _ := newTestServer(t)
	body := getJSON(t, srv, "/v1/info")
	index := body["index"].(map[string]any)
	for field, want := range map[string]any{
		"area_scheme":          "wgsrpd_l3",
		"syntaxon_area_scheme": "evc_territory",
	} {
		if got := index[field]; got != want {
			t.Errorf("%s = %v, erwartet %v", field, got, want)
		}
	}
	if _, ok := index["syntaxa_with_distribution"]; !ok {
		t.Error("syntaxa_with_distribution fehlt")
	}
}
```

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./internal/application/ -run TestIndexInfo -v && go test ./internal/adapters/http/ -run TestInfoJSON -v`
Expected: FAIL — `info.SyntaxonAreaScheme undefined`; im JSON fehlen beide Schlüssel.

- [ ] **Step 3: Implementieren**

In `internal/ports/input/services.go`, in `IndexInfo`:

```go
	// AreaScheme names the vocabulary ?area= codes come from ON THE SPECIES
	// ROUTES. It keeps its singular name although there are now two schemes:
	// a field name in a published answer is not a matter of taste, and
	// renaming it would break every client that reads it. What it means is
	// spelled out here and in the OpenAPI description instead.
	AreaScheme string `json:"area_scheme"`

	// (ConceptBackbones, SpeciesWithConcept and AreasWithData stay unchanged.)

	// SyntaxonAreaScheme names the vocabulary ?area= codes come from ON THE
	// SYNTAXA ROUTES. Named even when SyntaxaWithDistribution is zero: the
	// scheme is a property of this release, the count one of this index.
	SyntaxonAreaScheme string `json:"syntaxon_area_scheme"`
	// SyntaxaWithDistribution is the number of syntaxa the source makes any
	// statement about (the coverage rows). It is zero until a distribution
	// ingest has run — a true statement about the index, not a placeholder.
	// Measured against the pinned artifacts: 1114 of 1326 alliances.
	SyntaxaWithDistribution int `json:"syntaxa_with_distribution"`
```

In `internal/application/query.go`, in `IndexInfo`, vor dem `return`:

```go
	covered, err := q.repo.SyntaxaWithCoverage(ctx, domain.SchemeEVCTerritory)
	if err != nil {
		return input.IndexInfo{}, fmt.Errorf("counting syntaxa with distribution: %w", err)
	}
```

und im Rückgabewert:

```go
		SyntaxonAreaScheme:      domain.SchemeEVCTerritory,
		SyntaxaWithDistribution: len(covered),
```

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./internal/application/ ./internal/adapters/http/ -v`
Expected: PASS.

- [ ] **Step 5: OpenAPI in beiden Kopien**

Im `IndexInfo`-Schema die bestehende `area_scheme`-Beschreibung präzisieren und die zwei Felder ergänzen:

```yaml
        area_scheme:
          type: string
          enum: [wgsrpd_l3]
          description: >-
            Das Vokabular der `?area=`-Codes auf den **Arten**-Routen
            (WGSRPD Level 3). Der Name steht in der Einzahl, weil er vor dem
            zweiten Schema veröffentlicht wurde und ein Feldname in einer
            Antwort keine Geschmacksfrage ist — siehe
            `syntaxon_area_scheme` für das der Syntaxa.
        syntaxon_area_scheme:
          type: string
          enum: [evc_territory]
          description: >-
            Das Vokabular der `?area=`-Codes auf den **Syntaxa**-Routen (die
            136 EVC-Territorien). Steht auch dann, wenn
            `syntaxa_with_distribution` 0 ist: das Schema gehört zu dieser
            Fassung des Dienstes, die Zahl zu diesem Index. Zwischen den
            beiden Schemata gibt es keine Abbildung.
        syntaxa_with_distribution:
          type: integer
          description: >-
            Gemessene Zahl der Syntaxa, über die die Quelle **überhaupt** eine
            Aussage macht. 0 bis ein Verbreitungs-Ingest gelaufen ist — das
            ist eine wahre Aussage über den Index, kein Platzhalter. Gemessen
            gegen die gepinnten Artefakte: 1114.
```

Byte-Gleichheit:

```bash
cp internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
diff internal/adapters/http/openapi.yaml api/openapi/openapi.yaml && echo "identisch"
```

- [ ] **Step 6: Commit**

```bash
git add internal/ports/input/services.go internal/application/query.go internal/adapters/http/ api/openapi/
git commit -m "feat(api): /v1/info misst Syntaxa-Gebietsschema und Abdeckung"
```

---

### Task 11: Die Phase einhängen, ohne `runIngest` wachsen zu lassen

**Files:**
- Modify: `cmd/situs/ingest.go`
- Modify: `CLAUDE.md`
- Test: `cmd/situs/ingest_test.go`

**Interfaces:**
- Consumes: `application.IngestSyntaxonDistribution`, `application.SyntaxonDistributionReport` (Task 6); `application.IngestAreas` mit Schema-Menge (Task 5).
- Produces: `ingestOutput` um `Territories application.AreaReport` und `SyntaxonDistribution application.SyntaxonDistributionReport` erweitert; `localOverlays` um dieselben zwei Felder.

**`runIngest` darf nicht wachsen.** Seine Baseline im Funktionskomplexitäts-Ratchet ist **12**, und Teilprojekt A hängt dort schon eine Phase ein — C ist die zweite. Eine dritte Phase mit ihrem `if err != nil` reißt das Gate. Die neue Phase kommt deshalb **in den bestehenden Helfer `ingestLocalOverlays`**, und das ist nicht bloß eine Ausweichbewegung: dessen Doc-Kommentar beschreibt genau diese Sorte Schritt — „the two ingest steps that read nothing but a local CSV and ask no service at all". Die Territorien sind ein reines Namens-Overlay wie die WGSRPD-Namen, und die Verbreitung ist eine reine CSV-Quelle ohne hostus-Beteiligung. `runIngest` gewinnt damit **zwei Struktur-Zuweisungen und keine Verzweigung**.

Reihenfolge innerhalb von `ingestLocalOverlays`: Gebietsnamen (beide Dateien) vor der Verbreitung. Nicht weil es eine Abhängigkeit gäbe — es gibt keinen Fremdschlüssel —, sondern weil `AreasWithData` die Namen als Overlay über die Codes legt und ein Leser, der die Reihenfolge umdreht, nach dem Grund suchen würde.

- [ ] **Step 1: Die failing Tests schreiben**

In `cmd/situs/ingest_test.go` (Muster: der bestehende Test, der den Report-JSON prüft):

```go
func TestIngestBerichtetTerritorienUndSyntaxaVerbreitung(t *testing.T) {
	dir := t.TempDir()
	writeMinimalIngestInput(t, dir) // the package's existing helper
	writeCSV(t, dir, "evc_territories.csv",
		"area_scheme,area_code,name_en\n"+
			"evc_territory,austria-alps,Austria Alps\n"+
			"evc_territory,albania,Albania\n")
	writeCSV(t, dir, "syntaxon_distribution.csv",
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CA01A,evc_territory,austria-alps,verified\n")
	writeCSV(t, dir, "syntaxon_distribution_coverage.csv",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\n")

	out := runIngestForTest(t, dir) // parses the printed JSON
	terr := out["Territories"].(map[string]any)
	if terr["Areas"].(float64) != 2 {
		t.Errorf("Territories.Areas = %v, erwartet 2", terr["Areas"])
	}
	dist := out["SyntaxonDistribution"].(map[string]any)
	if dist["Written"].(float64) != 1 || dist["Covered"].(float64) != 1 {
		t.Errorf("SyntaxonDistribution = %v, erwartet Written 1 / Covered 1", dist)
	}
}

func TestIngestLaeuftOhneVerbreitungsdateienDurch(t *testing.T) {
	// The optional source is genuinely optional: no file, no failure, and the
	// report says zero rather than pretending.
	dir := t.TempDir()
	writeMinimalIngestInput(t, dir)

	out := runIngestForTest(t, dir)
	dist := out["SyntaxonDistribution"].(map[string]any)
	if dist["Written"].(float64) != 0 || dist["Covered"].(float64) != 0 {
		t.Errorf("SyntaxonDistribution = %v, erwartet Nullen", dist)
	}
	terr := out["Territories"].(map[string]any)
	if terr["Areas"].(float64) != 0 {
		t.Errorf("Territories.Areas = %v, erwartet 0", terr["Areas"])
	}
}
```

Hat das Paket keinen `runIngestForTest`-Helfer, dem bestehenden Muster folgen (Kommando mit `--csv-dir`/`--db` bauen, Ausgabe in einen Puffer, `json.Unmarshal`).

- [ ] **Step 2: Tests laufen lassen und Fehlschlag sehen**

Run: `go test ./cmd/situs/ -run TestIngest -v`
Expected: FAIL — `out["Territories"]` und `out["SyntaxonDistribution"]` sind `nil`, der Typcast paniert bzw. schlägt fehl.

- [ ] **Step 3: Implementieren**

In `cmd/situs/ingest.go`, `localOverlays` erweitern:

```go
// localOverlays bundles the ingest steps that read nothing but a local CSV
// and ask no service at all.
type localOverlays struct {
	areas        application.AreaReport
	territories  application.AreaReport
	descriptions application.DescriptionReport
	distribution application.SyntaxonDistributionReport
}
```

`ingestLocalOverlays` — der Doc-Kommentar wächst mit, und die zwei Gebietsdateien laufen über eine Schleife durch denselben Lader:

```go
// ingestLocalOverlays writes the area names, the habitat descriptions and the
// syntaxon distribution. None of them asks a service; all of them read one
// local CSV.
//
// Area names depend on nothing and nothing depends on them: they are a pure
// overlay on the area codes the distribution steps write. TWO files, one
// loader — wgsrpd_areas.csv and evc_territories.csv share the header
// area_scheme,area_code,name_en, and IngestAreas checks the scheme against
// the set of known ones, so a second loader would only be a second place for
// the check to drift.
//
// Descriptions must run after IngestCSV, because every row is checked against
// the habitat type it belongs to. The syntaxon distribution must run after
// IngestSyntaxa, because a distribution row naming a syntaxon the index does
// not carry is dropped and reported — which only means something once the
// syntaxa are there.
func ingestLocalOverlays(ctx context.Context, db *sqlite.DB, csvDir string) (localOverlays, error) {
	var out localOverlays

	for _, src := range []struct {
		file   string
		report *application.AreaReport
	}{
		{"wgsrpd_areas.csv", &out.areas},
		{"evc_territories.csv", &out.territories},
	} {
		path := filepath.Join(csvDir, src.file)
		report, err := application.IngestAreas(ctx, db, path)
		if err != nil {
			return localOverlays{}, fmt.Errorf("ingesting area names from %q: %w", path, err)
		}
		*src.report = report
	}

	// ... the existing description block, unchanged ...

	distCSV := filepath.Join(csvDir, "syntaxon_distribution.csv")
	coverageCSV := filepath.Join(csvDir, "syntaxon_distribution_coverage.csv")
	distribution, err := application.IngestSyntaxonDistribution(ctx, db, distCSV, coverageCSV)
	if err != nil {
		return localOverlays{}, fmt.Errorf("ingesting syntaxon distribution from %q: %w", distCSV, err)
	}
	out.distribution = distribution

	return out, nil
}
```

`ingestOutput` erweitern:

```go
	AreaNames          application.AreaReport
	// Territories is the second area-name file, reported separately rather
	// than summed into AreaNames: 369 WGSRPD areas and 136 territories added
	// up would be a figure that describes neither.
	Territories          application.AreaReport
	SyntaxonDistribution application.SyntaxonDistributionReport
```

und in `runIngest`, im `out := ingestOutput{...}`-Literal:

```go
		Territories:          overlays.territories,
		SyntaxonDistribution: overlays.distribution,
```

Das sind zwei Zuweisungen, keine Verzweigung: `runIngest` behält seine Komplexität.

`UnknownSyntaxa` und `SkippedRows` werden **nicht** in `runIngest` protokolliert — `IngestSyntaxonDistribution` warnt selbst (Task 6, Step 3), und eine zweite Warnung an derselben Sache wäre zwei Zeilen im Log, die dasselbe sagen und getrennt veralten. Der Report trägt sie ohnehin.

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./cmd/situs/ -v`
Expected: PASS.

- [ ] **Step 5: Komplexität prüfen**

Run: `make codecharta`
Expected: grün. Zu prüfen sind drei Zahlen:
- `cmd/situs/ingest.go`, Funktionsdeckel: `runIngest` muss **≤ 12** bleiben. `ingestLocalOverlays` ist jetzt die zweitkomplexeste Funktion der Datei und muss **≤ 10** bleiben — sie hat eine Schleife und drei Fehlerzweige dazugewonnen. Reißt sie den Deckel, wandert der Verbreitungsschritt in eine eigene Funktion `ingestSyntaxonDistributionPhase(ctx, db, csvDir) (application.SyntaxonDistributionReport, error)`, die `ingestLocalOverlays` aufruft — nicht die Baseline anheben.
- `cmd/situs/ingest.go`, Dateideckel: die Datei steht **nicht** in der `complexity`-Baseline, es gilt also `default_cap = 50`. Läuft sie darüber, kommen die Overlay-Helfer in eine neue Datei `cmd/situs/ingest_overlays.go`; der Deckel je Datei lässt sich durch Aufteilen befriedigen, weil die Summe mit dem Code wandert, und genau dafür ist er da.
- Sinkt eine Baseline messbar, **senken**. Die Projektkonvention ist, den gemessenen Wert einzusetzen, nicht den alten stehen zu lassen (`_query_note` in `.codecharta-ratchet.json` hält das als Praxis fest).

- [ ] **Step 6: `CLAUDE.md` nachziehen**

Drei Stellen:

1. Im Architektur-Abschnitt die Pipeline-Liste um eine Zeile:
   ```
   pipelines/evc-distribution/  # EVC-Verbreitungs-XLSX -> syntaxon_distribution.csv
                       # + coverage + evc_territories.csv (python3, stdlib only)
   ```
2. Unter „Invariants that reviewers must check" die neue Zusage, gleichrangig neben der Dreiwertigkeit von `in_area`:
   > - **Syntaxa-Verbreitung ist vierwertig.** `verified`, `uncertain`,
   >   `absence` (Coverage-Zeile, keine Verbreitungszeile) und `unknown`
   >   (keine Coverage-Zeile). `absence` und `unknown` dürfen an keiner Stelle
   >   zusammengeworfen werden: `SyntaxonDetail.distribution` **fehlt** bei
   >   `unknown` und trägt bei `absence` zwei leere Listen, und ein
   >   `?area=`-Filter behält die Unbeurteilbaren unmarkiert. Gemessen sind
   >   212 der 1326 Verbände `unknown`, darunter alle 190 Moos-, Flechten-
   >   und Algenverbände — die Quelle deckt nur *vascular-plant dominated
   >   vegetation* ab.
   > - **Es gibt zwei Gebietsschemata und keine Abbildung zwischen ihnen.**
   >   `wgsrpd_l3` für Arten, `evc_territory` für Syntaxa. Keine Antwort
   >   rechnet ein Territorium in einen WGSRPD-Code um — dieselbe Haltung wie
   >   bei ISO↔WGSRPD. Jede Abfrage, die Gebiete führt, ist
   >   schemaparametrisiert und liest die Abdeckung aus der Tabelle des
   >   jeweiligen Schemas.
3. Im Abschnitt „Current State" die Umsetzung eintragen und in der Tabelle der Design-Dokumente das Spec vom 2026-09-21 (Teilprojekt C) samt diesem Plan nennen. Den Satz, dass „syntaxa distribution" außerhalb liegt, **entfernen** — er ist mit diesem Teilprojekt falsch geworden. Der Satz über ISO↔WGSRPD bleibt unverändert und gilt für `evc_territory` genauso.

- [ ] **Step 7: Commit**

```bash
git add cmd/situs/ CLAUDE.md
git commit -m "feat(ingest): Territorien und Syntaxa-Verbreitung als lokale Overlays einhaengen"
```

---

### Task 12: Gegen den echten Index messen, absichern, dokumentieren

**Files:**
- Create: `internal/adapters/sqlite/distribution_integrity_test.go`
- Modify: `docs/reference/measured-index.md`
- Modify: `docs/reference/http-api.md`

**Interfaces:**
- Consumes: alles aus Task 1–11.
- Produces: ein Integritätstest über einen gebauten Index; aktualisierte Referenzdokumentation.

- [ ] **Step 1: Den Integritätstest schreiben**

`internal/adapters/sqlite/distribution_integrity_test.go` — er baut den Index aus Fixtures, nicht aus einem Artefakt im Arbeitsverzeichnis: ein Test, der eine 233-KB-XLSX braucht, ist in CI wertlos.

```go
// The spec's central promise as a test: no bryophyte, lichen or algal
// alliance carries a distribution statement, and none of them appears as
// "does not occur" either. The second half is the one that is easy to lose —
// a filter that drops the unjudgeable rows would satisfy the first half and
// break the promise.
func TestKeinKryptogamenverbandTraegtEineVerbreitungsaussage(t *testing.T) {
	db := newTestDB(t)
	seedCryptogamHierarchy(t, db) // formations C and R, classes CA/RA, orders
	                              // CA01/RA01, alliances CA01A/RA01A; only
	                              // CA01A gets distribution and coverage rows

	rows, err := db.QueryContext(context.Background(),
		`SELECT a.id FROM syntaxon a
		 JOIN syntaxon o ON o.id = a.parent_id
		 JOIN syntaxon c ON c.id = o.parent_id
		 JOIN syntaxon_distribution_coverage v ON v.syntaxon_id = a.id
		 WHERE a.rank = 'alliance' AND c.parent_id IN ('R','S','T','U','V','W','X','Y')`)
	if err != nil {
		t.Fatalf("Abfrage: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var wrong []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		wrong = append(wrong, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if len(wrong) != 0 {
		t.Errorf("Kryptogamen-Verbaende mit Verbreitungsaussage: %v", wrong)
	}
}

func TestKryptogamenverbandErscheintNichtAlsNichtvorkommen(t *testing.T) {
	db := newTestDB(t)
	seedCryptogamHierarchy(t, db)

	got, err := db.SyntaxonDistribution(context.Background(), "RA01A", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if got.Covered {
		t.Fatal("RA01A ist covered, obwohl die Quelle nichts ueber Moosverbaende sagt")
	}

	// And it stays in an ?area=-filtered list, unmarked: dropping it would be
	// the same false claim in the other direction.
	occurrences, err := db.SyntaxonOccurrencesInArea(context.Background(),
		domain.SchemeEVCTerritory, "austria-alps")
	if err != nil {
		t.Fatalf("SyntaxonOccurrencesInArea: %v", err)
	}
	if _, ok := occurrences["RA01A"]; ok {
		t.Error("RA01A traegt eine Vorkommenszeile")
	}
	covered, err := db.SyntaxaWithCoverage(context.Background(), domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxaWithCoverage: %v", err)
	}
	if covered["RA01A"] {
		t.Error("RA01A steht in der Coverage-Menge")
	}
}

func TestJedeVerbreitungszeileZeigtAufEinVorhandenesSyntaxon(t *testing.T) {
	// No foreign keys by design, so this is the check that replaces them.
	db := newTestDB(t)
	seedCryptogamHierarchy(t, db)

	for _, query := range []string{
		`SELECT d.syntaxon_id FROM syntaxon_distribution d
		 LEFT JOIN syntaxon s ON s.id = d.syntaxon_id WHERE s.id IS NULL`,
		`SELECT v.syntaxon_id FROM syntaxon_distribution_coverage v
		 LEFT JOIN syntaxon s ON s.id = v.syntaxon_id WHERE s.id IS NULL`,
	} {
		rows, err := db.QueryContext(context.Background(), query)
		if err != nil {
			t.Fatalf("Abfrage: %v", err)
		}
		var dangling []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatalf("Scan: %v", err)
			}
			dangling = append(dangling, id)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows.Err: %v", err)
		}
		_ = rows.Close()
		if len(dangling) != 0 {
			t.Errorf("baumelnde Verweise: %v", dangling)
		}
	}
}
```

- [ ] **Step 2: Test laufen lassen**

Run: `go test ./internal/adapters/sqlite/ -run 'Kryptogamen|TestJedeVerbreitungszeile' -v`
Expected: PASS. Schlägt einer fehl, liegt ein echter Defekt vor — melden, nicht den Test aufweichen.

- [ ] **Step 3: Vollen Ingest fahren und messen**

Run:
```bash
bash pipelines/evc-distribution/build.sh
make ingest-input
go run ./cmd/situs ingest --csv-dir out/ingest-input --db out/situs-neu.sqlite
```
Expected im Report: `Territories: {"Areas": 136, "SkippedRows": 0}` und
```json
"SyntaxonDistribution": {
  "Written": 11528, "Verified": 9608, "Uncertain": 1920,
  "Covered": 1114, "SkippedRows": 0, "UnknownSyntaxa": ["CI01E"]
}
```
`UnknownSyntaxa` muss **genau** `["CI01E"]` sein. Eine leere Liste hieße, dass die Fassungsdrift verschwiegen wird; mehr Einträge hießen, dass der Join nicht mehr trifft.

- [ ] **Step 4: Den Index abfragen**

Run:
```bash
sqlite3 out/situs-neu.sqlite "
SELECT 'Verbreitungszeilen', COUNT(*) FROM syntaxon_distribution;
SELECT occurrence, COUNT(*) FROM syntaxon_distribution GROUP BY 1 ORDER BY 1;
SELECT 'Coverage', COUNT(*) FROM syntaxon_distribution_coverage;
SELECT 'Territorien mit Daten', COUNT(DISTINCT area_code) FROM syntaxon_distribution;
SELECT 'Territoriumsnamen', COUNT(*) FROM area WHERE area_scheme='evc_territory';
SELECT 'WGSRPD-Namen', COUNT(*) FROM area WHERE area_scheme='wgsrpd_l3';
SELECT 'Verbaende ohne Aussage', COUNT(*) FROM syntaxon s
  WHERE s.rank='alliance' AND NOT EXISTS (
    SELECT 1 FROM syntaxon_distribution_coverage v WHERE v.syntaxon_id=s.id);
SELECT 'Kryptogamen mit Aussage', COUNT(*) FROM syntaxon a
  JOIN syntaxon o ON o.id=a.parent_id JOIN syntaxon c ON c.id=o.parent_id
  JOIN syntaxon_distribution_coverage v ON v.syntaxon_id=a.id
  WHERE a.rank='alliance' AND c.parent_id IN ('R','S','T','U','V','W','X','Y');
SELECT 'baumelnd', COUNT(*) FROM syntaxon_distribution d
  LEFT JOIN syntaxon s ON s.id=d.syntaxon_id WHERE s.id IS NULL;
SELECT 'Zeilen ohne Coverage', COUNT(DISTINCT d.syntaxon_id) FROM syntaxon_distribution d
  LEFT JOIN syntaxon_distribution_coverage v
    ON v.syntaxon_id=d.syntaxon_id AND v.area_scheme=d.area_scheme
  WHERE v.syntaxon_id IS NULL;
"
```
Expected: `Verbreitungszeilen 11528`; `uncertain 1920` und `verified 9608`; `Coverage 1114`; `Territorien mit Daten 136`; `Territoriumsnamen 136`; `WGSRPD-Namen 369`; `Verbaende ohne Aussage 212`; **`Kryptogamen mit Aussage 0`**; `baumelnd 0`; `Zeilen ohne Coverage 0`.

Die beiden Nullen am Schluss sind die eigentlichen Zusagen: kein Kryptogamen-Verband bekommt eine Aussage, die die Quelle nicht macht, und keine Verbreitungszeile existiert ohne ihre Coverage-Zeile (sonst wäre `absence` für dieses Syntaxon wieder nicht von `unknown` zu trennen). Weicht eine Zahl ab: anhalten und melden.

- [ ] **Step 5: Die Routen gegen den echten Index prüfen**

Run:
```bash
SITUS_INDEX_PATH=out/situs-neu.sqlite go run ./cmd/situs serve &
sleep 2
curl -s localhost:8080/v1/info | python3 -m json.tool | grep -A1 'area_scheme\|syntaxa_with'
curl -s 'localhost:8080/v1/areas?scheme=evc_territory' | python3 -c 'import json,sys; a=json.load(sys.stdin); print(len(a), a[0])'
curl -s 'localhost:8080/v1/areas' | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))'
curl -s -o /dev/null -w '%{http_code}\n' 'localhost:8080/v1/areas?scheme=evc-territory'
curl -s 'localhost:8080/v1/syntaxon/CA01A' | python3 -m json.tool | head -30
curl -s 'localhost:8080/v1/syntaxa?rank=alliance&area=austria-alps' | python3 -c '
import json,sys
a=json.load(sys.stdin)
marked=[x for x in a if "occurrence" in x]
carried=[x for x in a if "occurrence" not in x]
print("gesamt", len(a), "markiert", len(marked), "mitgefuehrt", len(carried))'
curl -s -o /dev/null -w '%{http_code}\n' 'localhost:8080/v1/syntaxa?rank=alliance&area=gibtsnicht'
curl -s -o /dev/null -w '%{http_code}\n' 'localhost:8080/v1/syntaxa?area=austria-alps'
kill %1
```
Expected: `/v1/info` nennt `"area_scheme": "wgsrpd_l3"`, `"syntaxon_area_scheme": "evc_territory"`, `"syntaxa_with_distribution": 1114`. `?scheme=evc_territory` liefert **136** Gebiete mit Namen, `/v1/areas` ohne Parameter unverändert **369**, `?scheme=evc-territory` **400**. `/v1/syntaxon/CA01A` trägt ein `distribution`-Objekt; ein Moosverband (etwa der erste aus `?rank=alliance&life_form_group=bryophyte_lichen`) trägt keines. Die gefilterte Liste zeigt „mitgefuehrt" = die Zahl der Verbände ohne Aussage (212, sofern der Filter über alle Verbände läuft). Beide Fehlerfälle **400**.

`?area=` ohne `rank` muss 400 sein: der Vorgabewert ist `formation`, und Formationen tragen keine Verbreitung.

- [ ] **Step 6: Referenzdokumentation aktualisieren**

In `docs/reference/measured-index.md`, mit der Abfrage je Zahl, wie es die Datei hält:
- Die Verbreitungszahlen: 11528 Zeilen (9608 `verified`, 1920 `uncertain`), 1114 Coverage-Zeilen, 136 Territorien mit Daten und 136 mit Namen, 212 Verbände ohne jede Aussage, davon 190 Kryptogamen-Verbände (137 Moos/Flechte, **53** Algen — nicht 51, siehe die Korrektur oben), dazu die 6 übrigen namentlich (`CT06A`, `DA12A`, `DA13A`, `DD01A`, `DD01B`, `DD01C`).
- Den **wörtlich abgedruckten Ingest-Report** um `Territories` und `SyntaxonDistribution` erweitern.
- Den **Fassungsunterschied** als eigenen Absatz: die Verbreitungsdatei nennt EVC-Fassung 3 (2024-06-12), die Hierarchie-Datei heißt `..._version_4.xlsx` und trägt in ihren Spaltenköpfen den Stand `EVC, version 2025-06-12`. Beide Angaben zur Hierarchie-Datei sind gemessen und meinen Verschiedenes — Dateifassung gegen EVC-Stand —, weshalb sie nebeneinander genannt und nicht zu einer verrechnet werden dürfen. Die belastbare Aussage über die Überdeckung ist die gemessene 1114/1115, nicht die Fassungsnummern; `CI01E` (`Campanulo-Nardion`, EEA-Code `NAR-01E`) wird namentlich genannt.
- Die **Attribution**: Zenodo Record 11580949, CC-BY 4.0, und **beide** Veröffentlichungen (Preislerová et al. 2022, Appl Veg Sci 25: e12642; Preislerová et al. 2024, Appl Veg Sci 27: e12766).
- Das gemessene `value_histogram` der Quellzellen `{"": 140112, "1": 9608, "U": 1920}` — die Zusage aus Abschnitt 9, dass die Wertemenge bei jedem Lauf gemessen und ausgewiesen wird.

In `docs/reference/http-api.md`:
- `GET /v1/areas`: der Parameter `?scheme=` mit Vorgabewert `wgsrpd_l3`, die erlaubten Werte, `INVALID_QUERY` bei einem unbekannten, und der Satz, dass es zwischen den Schemata keine Abbildung gibt.
- `GET /v1/syntaxon/{id}`: das Feld `distribution` und die vier Zustände — **fehlend** heißt „keine Aussage", zwei leere Listen heißen „geprüft, kommt nirgends vor".
- `GET /v1/syntaxa`: `?area=` und `?include=`, die Vorgabe `verified`, alle sechs Ablehnungsgründe, und das Feld `occurrence` samt seiner Dreiwertigkeit.
- `GET /v1/info`: `syntaxon_area_scheme` und `syntaxa_with_distribution`, und dass `area_scheme` weiter die Artverbreitung meint.

- [ ] **Step 7: Alle drei Gates**

Run: `make verify && make mutation && make codecharta`
Expected: alle drei grün.

**Zum Coverage-Ratchet:** `internal/application` steht auf 100 %. Fällt der Floor, fehlt ein Testfall — häufig ein Fehlerzweig in `readOccurrenceRows`/`readCoverageRows` oder der `Stat`-Zweig, der nicht `IsNotExist` ist. Nicht senken.

**Zum Mutationsgate:** `make mutation` läuft ein Paket je Aufruf über `scripts/mutation-gate.sh`; **niemals** gremlins mit `...` aufrufen (erzeugt still null Mutanten). Dieses Teilprojekt legt **kein** neues Go-Paket an, `.mutation-thresholds` bleibt also unverändert, solange die Schwellen halten. Die wahrscheinlichste überlebende Mutante ist der Vergleich `occurrence == domain.OccurrenceUncertain` in `SyntaxonDistribution` und in `readOccurrenceRows`: gegen sie hilft je ein Test, der **beide** Ausprägungen in derselben Antwort prüft — `TestSyntaxonDistributionLiefertSortierteListen` und `TestIngestSyntaxonDistributionFuelltBeideTabellen` tun genau das. Steigt die Punktzahl, den Schwellwert anheben (Raise-Only-Ratchet).

**Zum Komplexitäts-Ratchet:** sinkt eine Baseline messbar, senken. `internal/application/query.go` gewinnt in Task 10 eine Verzweigung und darf seine Baseline von 72 nicht überschreiten; tut es das, wandert die Syntaxa-Messung in `syntaxon_distribution.go` (Task 10, Vorbemerkung).

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/sqlite/distribution_integrity_test.go docs/
git commit -m "test,docs: Verbreitungs-Integritaet absichern und gemessene Werte festhalten"
```

---

## Self-Review

**Spec-Abdeckung, Abschnitt für Abschnitt.**

- **Abschnitt 1 (Die Quelle, gemessen).** Pin, SHA-256 und Größe in Task 2, Step 1 und im Manifest (Step 5); beide Zitate im Manifest und in `docs/reference/` (Task 12, Step 6). Die drei Blätter und die Auswahl nach Namen in Task 1 (`sheet_path`, `SheetSelectionTest`). Die Auswertung des Zellbezugs in Task 1 (`col_index`, `read_sheet`, `test_a_missing_cell_does_not_shift_the_following_ones`). Die Wertemenge `{"", "1", "U"}` als gemessenes `value_histogram` in Task 2, Step 2.
- **Abschnitt 2 (Territorien als eigenes Schema).** `domain.SchemeEVCTerritory` mit Unterstrich in Task 3, Step 3; die Slug-Ableitung samt `_coast` und Kollisionsabbruch in Task 1 (`slugify`, `SlugifyTest`, `test_a_slug_collision_aborts_naming_both_columns`); der ungekürzte Spaltenname als `name_en` in `evc_territories.csv`; „keine Abbildung zwischen den Schemata" als Doc-Kommentar (Task 3), als OpenAPI-Beschreibung (Task 7, Step 5) und als `CLAUDE.md`-Invariante (Task 11, Step 6).
- **Abschnitt 3 (Vier Zustände).** Beide Tabellen in `schema.sql`, nicht in `Migrate`, mit der `verifyTables`-Begründung und einem Test je Tabelle (Task 3). Die Unterscheidung `absence`/`unknown` wird an vier Stellen geprüft: im Repository (`TestSyntaxonDistributionTrenntAbsenceVonUnknown`, Task 4), im Ingest (`TestIngestSyntaxonDistributionBehaeltCoverageOhneVorkommen`, Task 6), in der Anwendung und im JSON (Task 8, alle drei Tests) und im Filter (Task 9, `TestSyntaxaByRankMitGebietBehaeltDieUnbeurteilbaren`). Die 212/1326 und die 190 Kryptogamen-Verbände in Task 12, Step 4.
- **Abschnitt 4 (Pipeline).** Task 1 und 2 vollständig, inklusive der drei Verdrahtungsstellen (`scripts/collect-ingest-input.sh` als `OPTIONAL`, `Makefile`-Ziel `pipeline-test`, `manifest.yaml` + `README.md`) und der Pipeline-Liste in `CLAUDE.md` (Task 11, Step 6). Die Übersetzung `1`→`verified`/`U`→`uncertain` macht die Pipeline (Task 1, `_OCCURRENCE`), die CSV trägt die Langform. **Eine Abweichung:** drei Ausgaben statt zwei, begründet in Task 1 (ohne die Coverage-CSV ist der in Abschnitt 7 ausdrücklich gültige Fall nicht darstellbar).
- **Abschnitt 5 (Ingest).** Task 6. **Eine Abweichung:** der zweite Parameter ist `coveragePath`, nicht `areaCSVPath` — Abschnitt 6.1 desselben Specs verbietet den zweiten Gebietslader, den `areaCSVPath` gewesen wäre. `Territories` fällt aus dem Report, weil die Zahl in `AreaReport` steht. `CI01E` namentlich: `TestIngestSyntaxonDistributionVerwirftUnbekanntenCodeUndMeldetIhn` (Task 6) und die Erwartung `UnknownSyntaxa == ["CI01E"]` im echten Lauf (Task 12, Step 3). Der Fassungsunterschied in `docs/reference/measured-index.md` (Task 12, Step 6).
- **Abschnitt 6 (Read-API).** `SyntaxonDetail.distribution` in Task 8; `?area=`/`?include=` samt aller sechs Ablehnungen in Task 9; die beiden `/v1/info`-Felder mit beibehaltenem `area_scheme` in Task 10. Die drei Stellen aus Abschnitt 6 sind Task 5 (`IngestAreas`, Blocker 1), Task 4 (`AreasWithData`, Blocker 2) und Task 7 (`QueryService.Areas` und `/v1/areas?scheme=`, Blocker 3). **Ein vierter Fund derselben Art** ist in Task 4 dokumentiert: `KnownAreaCodes` leitet die Abdeckung ebenso aus `species_distribution` ab, und ohne den zweiten Zweig dort wäre jeder Territoriumscode auf `?area=` ein `INVALID_QUERY` — der Filter aus Abschnitt 6 also vollständig unbenutzbar. Das Spec führt ihn nicht auf.
- **Abschnitt 7 (Fehlerbehandlung).** Alle sieben Zeilen der Tabelle haben einen Test: fehlende Datei nur gewarnt (Task 6, `TestIngestSyntaxonDistributionWarntNurBeiFehlenderDatei`); fehlendes Blatt bricht mit Blattnamen ab (Task 1, `test_a_missing_data_sheet_aborts_naming_it`); unbekannte Zellbelegung bricht mit Wert, Zeile und Spalte ab (Task 1, `test_an_unknown_cell_value_aborts_naming_value_row_and_column`); Slug-Kollision nennt beide Namen (Task 1); Verbandscode nicht im Index (Task 6); Coverage ohne Vorkommen gültig (Task 4 und 6 und 8); `?area=` mit unbekanntem Code `INVALID_QUERY` (Task 9, Anwendung und HTTP).
- **Abschnitt 8 (Tests).** Die vier Bündel liegen in Task 1 (Pipeline, alle sechs genannten Fälle), Task 6 (Ingest, alle vier genannten Fälle), Task 8 und 9 (HTTP, alle drei genannten Fälle) und Task 12 (der Test über den gebauten Index, „kein Moos-, Flechten- oder Algenverband trägt eine Verbreitungsaussage", in beiden Richtungen).
- **Abschnitt 9 (Prüfbare Zusagen).** Vier Zustände unterscheidbar → Task 3/4/6/8/9 wie oben. Kein Kryptogamen-Verband mit Aussage → Task 12, Step 2 und 4 (`Kryptogamen mit Aussage 0`). Wertemenge gemessen → Task 2, Step 2. Eigenes `area_scheme` ohne Umrechnung → Task 3, 7, 11. `CI01E` gemeldet → Task 6 und 12. Attribution beider Arbeiten → Manifest und `docs/reference/`.
- **Abschnitt 10 (Bewusst außerhalb).** ISO/GPS-Abbildung, Vererbung nach oben, Kartenbilder und verbreitungsgestützte Gewichtung kommen in keinem Task vor. Der `README.md`-Abschnitt „Bewusst nicht drin" (Task 2, Step 6) hält es fest, damit es nicht als Auslassung gelesen wird.

**Platzhalter-Scan.** Kein „TBD", kein „TODO", kein „analog zu Task N". Jeder Code-Step trägt den Code, den er meint: die Pipeline vollständig, die drei sqlite-Methoden vollständig, der Ingest in zwei Hälften vollständig, beide Handler-Funktionen vollständig. Die Fehlerbehandlung ist pro Fall benannt, nie als „angemessene Fehlerbehandlung". Wo ein Task auf bestehenden Code trifft, den Teilprojekt A oder B anlegt (`SyntaxaByRank`, `Syntaxon`, `handleSyntaxa`, `fakeRepo`, `newTestServer`), ist die einzufügende Stelle wörtlich zitiert und der Dateiname mit dem Vorbehalt „Name gemäß Repo" versehen — die Datei existiert zum Planungszeitpunkt noch nicht.

**Typkonsistenz.**
- `domain.SyntaxonDistribution` (Task 3) hat vier Felder; `Repository.SyntaxonDistribution` (Task 4) gibt genau diesen Typ zurück; `syntaxonDistributionOf` (Task 8) liest genau `Covered`, `Scheme`, `Verified`, `Uncertain`.
- `input.SyntaxonDistribution` (Task 8) hat drei Felder und wird als **Zeiger** in `SyntaxonDetail` geführt — der Zeiger ist der vierte Zustand.
- `IngestTx.UpsertSyntaxonDistribution(syntaxonID, scheme, code, occurrence string)` (Task 4) wird in Task 6 mit genau vier Strings in dieser Reihenfolge gerufen; `UpsertSyntaxonDistributionCoverage(syntaxonID, scheme string)` mit genau zwei.
- `SyntaxonDistributionReport` (Task 6) trägt genau die Felder, die Task 6 setzt (`Written`, `Verified`, `Uncertain`, `Covered`, `SkippedRows`, `UnknownSyntaxa`) und Task 11 im JSON ausgibt. `Territories` ist bewusst **nicht** darin.
- `input.SyntaxonAreaFilter` (Task 9) hat `Code` und `Include`; `SyntaxaByRank(ctx, rank, lifeFormGroup string, filter SyntaxonAreaFilter)` wird in Task 9 im Handler mit demselben Wert gerufen, den `syntaxonAreaFilter(r, rank)` liefert.
- `QueryService.Areas(ctx, scheme string)` (Task 7) wird in `handleAreas` mit dem validierten Schema gerufen; `Repository.AreasWithData(ctx, scheme)` und `KnownAreaCodes(ctx, scheme)` nehmen denselben String.
- `domain.OccurrenceVerified`/`OccurrenceUncertain` (Task 3) sind die einzigen Werte, die im Schema-`CHECK`, in `_OCCURRENCE` der Pipeline, in `includeValues` des Handlers und im OpenAPI-Enum stehen — vier Stellen, ein Wertepaar, und Task 3, Step 1 nagelt die Zeichenketten in einem Domänentest fest.

**Zwei bewusste Brüche in der Mitte.** Nach Task 4 kompilieren `internal/application` und `internal/adapters/http` nicht, weil `fakeRepo` und `fakeQueryService` die neuen Port-Methoden nicht haben; Task 5 bis 7 stellen den Zustand her, und Task 4, Step 5 sagt das ausdrücklich und begrenzt den Testlauf auf zwei Pakete. Nach Task 7 ist `Areas(ctx)` überall auf `Areas(ctx, scheme)` umgestellt — ein Bruch, der nur in einem Task steht. Die Alternative — Port, Anwendung, Adapter und Tests in einem Task — wäre ein Task, den ein Prüfer nicht mehr in Teilen ablehnen kann.

**Was dieser Plan nicht kann.** Die Reihenfolge setzt voraus, dass Teilprojekt A **und** B gemergt sind. Ohne A gibt es keine EVC-Primärcodes, gegen die `known[id]` prüft (jede der 1115 Zeilen landete in `UnknownSyntaxa`); ohne B gibt es weder `SyntaxonDetail` noch `GET /v1/syntaxa`, also die Objekte, die Task 8 und 9 erweitern. Task 1 bis 6 sind von B unabhängig und könnten parallel laufen; Task 7 ist von beiden unabhängig. Wer C vorzieht, fängt dort an und hält Task 8 bis 10 zurück.
