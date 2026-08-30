# Syntaxa-Hierarchie (Klasse/Ordnung) & Autorschafts-Trennung Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** situs führt die volle Syntaxa-Hierarchie (Klasse → Ordnung → Verband)
mit Codes, ergänzt um eine von der EUNIS-Quelle unabhängige Autorschafts-Spalte,
gespeist aus FloraVeg.EU.

**Architecture:** Eine neue Python-Pipeline (`pipelines/eurovegchecklist/`)
wandelt die FloraVeg-XLSX in `syntaxa_hierarchy.csv` (code|rank|name|author|
parent_code), wobei Rang und Elternteil ausschließlich aus dem Code-Muster
abgeleitet werden. Ein neuer Go-Ingest-Schritt (`IngestSyntaxaHierarchy`) läuft
nach `IngestCSV` (die EUNIS-Verbände müssen schon existieren): er schreibt
Klasse-/Ordnung-Zeilen neu und reichert jeden bereits ingestierten EUNIS-Verband
per längstem Präfix-Treffer gegen die FloraVeg-Verbandsnamen mit `Author` und
`ParentID` an. Kein Treffer bleibt ehrlich unangetastet, kein Treffer wird
geraten.

**Tech Stack:** Go 1.26 (domain/ports/application/sqlite/http), Python 3 stdlib
(`zipfile`, `xml.etree.ElementTree`, `csv`, `re`, `json`, `argparse`) für die
Pipeline — keine neue Abhängigkeit in keiner der beiden Sprachen.

**Spec:** `docs/superpowers/specs/2026-08-30-situs-syntaxa-hierarchie-design.md`

## Global Constraints

- Keine neue Bibliothek — weder Go (`.golangci.yml`-Allowlist bleibt
  unverändert) noch Python (nur Stdlib, kein `openpyxl`/`pandas`/`pyyaml`).
- `Author` wird **nie** heuristisch aus einem Kombi-String geschnitten — nur von
  FloraVegs eigener, bereits getrennter Spalte übernommen.
- `rank`/`parent_code` werden in der Pipeline **ausschließlich** aus dem
  Code-Muster (`AA`/`AA01`/`AA01A`) abgeleitet, nie aus dem Namen geraten.
- Ein EUNIS-Verband ohne FloraVeg-Treffer bleibt `Author=""`, `ParentID=""`,
  `Name` unverändert der volle historische Kombi-String — gezählt, kein
  Abbruch.
- Mehrdeutige, gleich lange Präfix-Treffer werden **nie** geraten — sie landen
  in `AmbiguousMatches`, kein Treffer wird übernommen.
- Fehlt `syntaxa_hierarchy.csv`, läuft der Ingest ohne Hierarchie-Anreicherung
  weiter (analog zu `IngestLocalizations`s fehlender `localizations.csv`).
- `author`/`parent_id` sind auf dem Wire `omitempty` — Rückwärtskompatibilität
  für bestehende Clients ist Pflicht.
- SQL bleibt statische `?`-Platzhalter-Syntax, nie String-Konkatenation
  (gosec G201/G202).
- `make verify` (fmt-check, vet, lint, test, arch, debt, build) muss vor jedem
  Commit grün sein; TDD (Test schreiben → rot sehen → implementieren → grün).

---

## File Structure

| Datei | Zweck |
|---|---|
| `pipelines/eurovegchecklist/manifest.yaml` | Pinning der FloraVeg-XLSX (URL, SHA-256, Lizenz) — Muster: `pipelines/eunis/manifest.yaml` |
| `pipelines/eurovegchecklist/xlsx_to_csv.py` | XLSX → `syntaxa_hierarchy.csv` + `report.json` |
| `pipelines/eurovegchecklist/test_xlsx_to_csv.py` | Unittest mit In-Memory-XLSX-Fixtures |
| `pipelines/eurovegchecklist/README.md` | Beschaffung, Ausführung, Eigenheiten — Muster: `pipelines/eunis/README.md` |
| `internal/domain/habitat.go` | `Syntaxon.Author` neu |
| `internal/ports/output/repository.go` | `IngestTx.UpsertSyntaxonAuthor`, `Repository.AllSyntaxa` neu |
| `internal/adapters/sqlite/schema.sql` | `syntaxon.author`-Spalte |
| `internal/adapters/sqlite/write.go` | `UpsertSyntaxon` schreibt `author`; `UpsertSyntaxonAuthor` neu |
| `internal/adapters/sqlite/read.go` | `Syntaxon`/`Syntaxa` lesen `author`; `AllSyntaxa` neu |
| `internal/adapters/sqlite/write_test.go`, `read_test.go` | Tests für alle drei |
| `internal/application/syntaxa_hierarchy_ingest.go` | `IngestSyntaxaHierarchy` — CSV lesen, Klasse/Ordnung schreiben, Verbände abgleichen |
| `internal/application/syntaxa_hierarchy_ingest_test.go` | Treffer/Nichttreffer/Mehrdeutigkeit/fehlende Datei |
| `internal/application/ingest_test.go` | `fakeRepo` um `AllSyntaxa`/`UpsertSyntaxonAuthor` erweitert |
| `cmd/situs/ingest.go` | Neuer Ingest-Schritt verdrahtet, `ingestOutput` erweitert |
| `cmd/situs/ingest_test.go` | `seedIngestDir` bzw. neuer Test mit `syntaxa_hierarchy.csv` |
| `internal/ports/input/services.go` | `SyntaxonRef` um `Author`/`ParentID` erweitert |
| `internal/application/query.go` | `syntaxaOf` füllt die neuen Felder |
| `internal/adapters/http/openapi.yaml`, `api/openapi/openapi.yaml` | `SyntaxonRef`-Schema erweitert (beide Kopien, byte-identisch) |
| `internal/adapters/http/habitat_test.go` | JSON-Test für `author`/`parent_id` |
| `docs/reference/measured-index.md` | Abschnitt „Syntaxa-Tiefe" mit echten neuen Zahlen |

---

### Task 1: Domäne, Ports und sqlite-Adapter um `Author`/`AllSyntaxa` erweitern

**Files:**
- Modify: `internal/domain/habitat.go`
- Modify: `internal/ports/output/repository.go`
- Modify: `internal/adapters/sqlite/schema.sql`
- Modify: `internal/adapters/sqlite/write.go`
- Modify: `internal/adapters/sqlite/read.go`
- Test: `internal/adapters/sqlite/write_test.go`
- Test: `internal/adapters/sqlite/read_test.go`

**Interfaces:**
- Produces: `domain.Syntaxon{ID, Rank, Name, Author, ParentID string}` (Author
  neu). `output.IngestTx.UpsertSyntaxonAuthor(id, author, parentID string) error`.
  `output.Repository.AllSyntaxa(ctx context.Context) ([]domain.Syntaxon, error)`.
- Consumes: nichts aus späteren Tasks.

- [ ] **Step 1: Fehlschlagenden Test für `Syntaxon.Author`-Rundtrip schreiben**

In `internal/adapters/sqlite/write_test.go`, im bestehenden
`TestIngestTx_UpsertsCrosswalkSyntaxonSpeciesRoleAndLocalization`-Umfeld einen
eigenständigen Test ergänzen:

```go
func TestIngestTx_UpsertSyntaxonRoundTripsAuthor(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	s := domain.Syntaxon{ID: "arrhenatherion", Rank: "alliance", Name: "Arrhenatherion", Author: "Koch 1926"}
	if err := tx.UpsertSyntaxon(s); err != nil {
		t.Fatalf("UpsertSyntaxon: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.Syntaxon(ctx, "arrhenatherion")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.Author != "Koch 1926" {
		t.Errorf("Author = %q, want %q", got.Author, "Koch 1926")
	}
}

func TestIngestTx_UpsertSyntaxonAuthor(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	seed := func() {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if err := tx.UpsertSyntaxon(domain.Syntaxon{ID: "arrhenatherion", Rank: "alliance", Name: "Arrhenatherion"}); err != nil {
			t.Fatalf("UpsertSyntaxon: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}
	seed()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertSyntaxonAuthor("arrhenatherion", "Koch 1926", "AA01"); err != nil {
		t.Fatalf("UpsertSyntaxonAuthor: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.Syntaxon(ctx, "arrhenatherion")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.Author != "Koch 1926" || got.ParentID != "AA01" {
		t.Errorf("got = %+v, want Author=%q ParentID=%q", got, "Koch 1926", "AA01")
	}
	if got.Rank != "alliance" || got.Name != "Arrhenatherion" {
		t.Errorf("got = %+v, want Rank/Name untouched", got)
	}

	// A second call with an empty parentID must not clear the one just set.
	tx2, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx2.UpsertSyntaxonAuthor("arrhenatherion", "Koch 1926 emend.", ""); err != nil {
		t.Fatalf("UpsertSyntaxonAuthor: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	got2, err := db.Syntaxon(ctx, "arrhenatherion")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got2.ParentID != "AA01" {
		t.Errorf("ParentID = %q after empty-parentID call, want it untouched (%q)", got2.ParentID, "AA01")
	}
}
```

In `internal/adapters/sqlite/read_test.go` `TestSyntaxonAndSyntaxa` um die
Author-Prüfung ergänzen: `openSeededDB` seedet `BRO-01A` bislang ohne Author
(siehe Zeile 66) — dort `Author: "Rivas-Martínez 1978"` ergänzen und danach
`if s.Author != "Rivas-Martínez 1978" { ... }` prüfen. Außerdem einen neuen Test:

```go
func TestAllSyntaxa(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.AllSyntaxa(t.Context())
	if err != nil {
		t.Fatalf("AllSyntaxa: %v", err)
	}
	if len(got) != 1 || got[0].ID != "BRO-01A" {
		t.Errorf("AllSyntaxa = %+v, want the one seeded syntaxon", got)
	}
}
```

Und `AllSyntaxa` den beiden Fehlerfall-Tabellen in `TestReads_QueryErrorsAreReturned`
(Zeile ~270) und `TestReads_RowsIterationAndScanErrorsAreReturned` (Zeile ~296)
hinzufügen:

```go
"AllSyntaxa": func() error { _, err := db.AllSyntaxa(ctx); return err },
```

```go
"AllSyntaxa": {
	call: func(db *DB) error { _, err := db.AllSyntaxa(ctx); return err },
	rows: "reading all syntaxa", scan: "scanning syntaxon",
},
```

- [ ] **Step 2: Tests laufen lassen, erwartet: Kompilierfehler / rot**

```bash
go test ./internal/adapters/sqlite/... -run 'Syntaxon|AllSyntaxa' -v
```

Erwartet: Compile-Fehler (`Author` unbekanntes Feld, `UpsertSyntaxonAuthor`/
`AllSyntaxa` nicht deklariert).

- [ ] **Step 3: Domäne, Ports, Schema, Adapter implementieren**

`internal/domain/habitat.go`, `Syntaxon` erweitern:

```go
type Syntaxon struct {
	ID       string
	Rank     string // "class" | "order" | "alliance"
	Name     string // reiner Syntaxon-Name, ohne Autorschaft
	Author   string // Autorschafts-Zitat; "" wenn kein sauberer Split bekannt
	ParentID string
}
```

`internal/ports/output/repository.go`, `IngestTx` erweitern (nach
`UpsertSyntaxon`):

```go
	UpsertSyntaxon(s domain.Syntaxon) error
	// UpsertSyntaxonAuthor sets Author on an already-upserted syntaxon and, if
	// parentID is non-empty, its ParentID — used ONLY by the hierarchy-matching
	// pass to enrich an existing EUNIS alliance row without re-declaring its
	// Rank/Name (die bleiben EUNIS-eigen bei fehlendem FloraVeg-Treffer).
	UpsertSyntaxonAuthor(id, author, parentID string) error
```

`Repository` erweitern (nach `Syntaxa`):

```go
	// AllSyntaxa returns every vegetation unit the index holds, in id order.
	// Used by the FloraVeg hierarchy-matching pass to find every
	// already-ingested EUNIS alliance to match its own names against.
	AllSyntaxa(ctx context.Context) ([]domain.Syntaxon, error)
```

`internal/adapters/sqlite/schema.sql`, `syntaxon`-Tabelle:

```sql
CREATE TABLE IF NOT EXISTS syntaxon (
  id        TEXT PRIMARY KEY,
  rank      TEXT NOT NULL,
  name      TEXT NOT NULL,
  author    TEXT NOT NULL DEFAULT '',
  parent_id TEXT NOT NULL DEFAULT ''
);
```

`internal/adapters/sqlite/write.go`, `UpsertSyntaxon` ersetzen und
`UpsertSyntaxonAuthor` ergänzen:

```go
func (t *ingestTx) UpsertSyntaxon(s domain.Syntaxon) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO syntaxon (id, rank, name, author, parent_id)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   rank=excluded.rank, name=excluded.name, author=excluded.author, parent_id=excluded.parent_id`,
		s.ID, s.Rank, s.Name, s.Author, s.ParentID)
	if err != nil {
		return fmt.Errorf("sqlite: upserting syntaxon %s: %w", s.ID, err)
	}
	return nil
}

// UpsertSyntaxonAuthor sets author on an already-upserted syntaxon. An empty
// parentID leaves the stored parent_id untouched — a repeated hierarchy-ingest
// pass without a fresh match must not erase a previous one.
func (t *ingestTx) UpsertSyntaxonAuthor(id, author, parentID string) error {
	if parentID == "" {
		_, err := t.tx.ExecContext(t.ctx, `UPDATE syntaxon SET author = ? WHERE id = ?`, author, id)
		if err != nil {
			return fmt.Errorf("sqlite: setting author of syntaxon %s: %w", id, err)
		}
		return nil
	}
	_, err := t.tx.ExecContext(t.ctx,
		`UPDATE syntaxon SET author = ?, parent_id = ? WHERE id = ?`, author, parentID, id)
	if err != nil {
		return fmt.Errorf("sqlite: setting author/parent of syntaxon %s: %w", id, err)
	}
	return nil
}
```

`internal/adapters/sqlite/read.go`, `Syntaxon` und `Syntaxa` um `author`
erweitern:

```go
func (d *DB) Syntaxon(ctx context.Context, id string) (domain.Syntaxon, error) {
	s := domain.Syntaxon{ID: id}
	row := d.QueryRowContext(ctx, `SELECT rank, name, author, parent_id FROM syntaxon WHERE id = ?`, id)
	if err := row.Scan(&s.Rank, &s.Name, &s.Author, &s.ParentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Syntaxon{}, fmt.Errorf("sqlite: syntaxon %q: %w", id, output.ErrNotFound)
		}
		return domain.Syntaxon{}, fmt.Errorf("sqlite: querying syntaxon %q: %w", id, err)
	}
	return s, nil
}

func (d *DB) Syntaxa(ctx context.Context, key domain.HabitatTypeKey) ([]domain.Syntaxon, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT s.id, s.rank, s.name, s.author, s.parent_id
		 FROM habitat_type_syntaxon l JOIN syntaxon s ON s.id = l.syntaxon_id
		 WHERE l.typology_id = ? AND l.code = ?
		 ORDER BY s.id`,
		string(key.Typology), key.Code)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying syntaxa of %s: %w", key, err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.Syntaxon{}
	for rows.Next() {
		var s domain.Syntaxon
		if err := rows.Scan(&s.ID, &s.Rank, &s.Name, &s.Author, &s.ParentID); err != nil {
			return nil, fmt.Errorf("sqlite: scanning syntaxa of %s: %w", key, err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading syntaxa of %s: %w", key, err)
	}
	return out, nil
}

// AllSyntaxa returns every vegetation unit the index holds, in id order. Used
// by the FloraVeg hierarchy-matching pass to find every already-ingested
// EUNIS alliance to match its own names against.
func (d *DB) AllSyntaxa(ctx context.Context) ([]domain.Syntaxon, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, rank, name, author, parent_id FROM syntaxon ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading all syntaxa: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.Syntaxon{}
	for rows.Next() {
		var s domain.Syntaxon
		if err := rows.Scan(&s.ID, &s.Rank, &s.Name, &s.Author, &s.ParentID); err != nil {
			return nil, fmt.Errorf("sqlite: scanning syntaxon: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading all syntaxa: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 4: Tests laufen lassen, erwartet: grün**

```bash
go test ./internal/adapters/sqlite/... -v
```

- [ ] **Step 5: Restlichen sqlite-Testsuite und Build prüfen**

```bash
go build ./... && go vet ./...
```

Zwei weitere Stellen implementieren `output.IngestTx`/`output.Repository`
vollständig und brauchen die neue Methode, sonst schlägt der Build fehl:
`internal/application/ingest_test.go`'s `fakeRepo` (Task 3) und keine weitere
— `internal/app/seam_test.go` und `internal/application/localize_test.go`
nutzen die echte sqlite-`DB`, nicht ein eigenes Double.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/habitat.go internal/ports/output/repository.go \
  internal/adapters/sqlite/schema.sql internal/adapters/sqlite/write.go \
  internal/adapters/sqlite/read.go internal/adapters/sqlite/write_test.go \
  internal/adapters/sqlite/read_test.go
git commit -m "feat(domain): add Syntaxon.Author and AllSyntaxa/UpsertSyntaxonAuthor"
```

---

### Task 2: `fakeRepo` in `internal/application/ingest_test.go` erweitern

Eigener Task, weil er reine Testinfrastruktur für Task 3 ist und ein Reviewer
ihn unabhängig von Task 3s eigentlicher Logik beurteilen können soll.

**Files:**
- Modify: `internal/application/ingest_test.go`

**Interfaces:**
- Consumes: `output.IngestTx.UpsertSyntaxonAuthor`, `output.Repository.AllSyntaxa`
  (Task 1).
- Produces: `fakeRepo.allSyntaxaErr error` (injizierbarer Fehler),
  `fakeRepo.authorUpdates []struct{ id, author, parentID string }` (für
  Assertions in Task 3), beide über den Export der bestehenden `fakeRepo`.

- [ ] **Step 1: `fakeRepo` um die neuen Felder/Methoden ergänzen**

In `internal/application/ingest_test.go`, im `fakeRepo`-Struct (nach
`syntaxa []domain.Syntaxon`):

```go
	authorUpdates []struct {
		id, author, parentID string
	}
	allSyntaxaErr error
```

Neue Methoden, direkt nach `func (r *fakeRepo) UpsertSyntaxon(...)`:

```go
func (r *fakeRepo) UpsertSyntaxonAuthor(id, author, parentID string) error {
	if err := r.failIfNamed("UpsertSyntaxonAuthor"); err != nil {
		return err
	}
	r.authorUpdates = append(r.authorUpdates, struct{ id, author, parentID string }{id, author, parentID})
	for i := range r.syntaxa {
		if r.syntaxa[i].ID == id {
			r.syntaxa[i].Author = author
			if parentID != "" {
				r.syntaxa[i].ParentID = parentID
			}
		}
	}
	return nil
}

func (r *fakeRepo) AllSyntaxa(_ context.Context) ([]domain.Syntaxon, error) {
	if r.allSyntaxaErr != nil {
		return nil, r.allSyntaxaErr
	}
	out := make([]domain.Syntaxon, len(r.syntaxa))
	copy(out, r.syntaxa)
	return out, nil
}
```

Und die Rollback-Fehlerinjektions-Tabelle (Zeile ~412,
`for _, failOn := range []string{"UpsertTypology", ...}`) um
`"UpsertSyntaxonAuthor"` ergänzen — nur, falls dieser Test generisch über alle
`IngestTx`-Methoden von `ingestAll` iteriert; `UpsertSyntaxonAuthor` wird von
`IngestCSV`/`ingestAll` nicht aufgerufen (das ist Task 3s eigener Ingest-Pfad),
deshalb bleibt diese Tabelle unverändert — nur zur Kenntnis, nicht ändern.

- [ ] **Step 2: Kompilieren und bestehende Tests laufen lassen**

```bash
go build ./... && go test ./internal/application/... -v
```

Erwartet: grün — diese Erweiterung fügt nur totes Zubehör hinzu, das Task 3
als Erstes benutzt.

- [ ] **Step 3: Commit**

```bash
git add internal/application/ingest_test.go
git commit -m "test(application): extend fakeRepo with AllSyntaxa/UpsertSyntaxonAuthor"
```

---

### Task 3: FloraVeg-Pipeline (`pipelines/eurovegchecklist/`)

**Files:**
- Create: `pipelines/eurovegchecklist/manifest.yaml`
- Create: `pipelines/eurovegchecklist/xlsx_to_csv.py`
- Create: `pipelines/eurovegchecklist/test_xlsx_to_csv.py`
- Create: `pipelines/eurovegchecklist/README.md`

**Interfaces:**
- Produces: `syntaxa_hierarchy.csv` mit Spalten
  `code,rank,name,author,parent_code` — `rank` ∈ `class|order|alliance`,
  `parent_code` leer für `class`.

- [ ] **Step 1: `manifest.yaml` anlegen**

```yaml
# Pinned source artifact for the EuroVegChecklist hierarchy-ingest pipeline.
#
# Plain text like pipelines/eunis/manifest.yaml — no YAML parser needed, no
# YAML parser used.
#
# Not committed: pipelines/eurovegchecklist/artifacts/ is gitignored. Run the
# curl command in README.md to re-download before running the pipeline.

sources:
  - id: eurovegchecklist-v4
    file: List_of_European_vegetation_units_version_4.xlsx
    description: >
      Mucina et al. 2016 EuroVegChecklist, version 4 (2025-06-17 update) —
      the full vegetation-unit hierarchy (class/order/alliance) with codes
      and authorship, published by FloraVeg.EU.
    dataset_record: https://floraveg.eu/download/
    url: "https://files.ibot.cas.cz/cevs/downloads/floraveg/List_of_European_vegetation_units_version_4.xlsx"
    license: "FloraVeg.EU terms of use — https://floraveg.eu/about/ (free download, no registration)"
    retrieved: "2026-08-30"
```

Ein `sha256`/`size_bytes`-Paar fehlt hier bewusst als Platzhalter — Step 5
misst beides gegen die echte Datei und trägt die realen Werte ein (Muster:
`pipelines/eunis/manifest.yaml`, „gemessen, nicht angenommen").

- [ ] **Step 2: Fehlschlagenden Test für die Code→Rang/Parent-Ableitung schreiben**

```python
# pipelines/eurovegchecklist/test_xlsx_to_csv.py
import io
import json
import os
import tempfile
import unittest
import zipfile

from xlsx_to_csv import CSV_HEADERS, HeaderError, convert, rank_and_parent


def _xml_escape(value):
    return value.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def _cell_xml(ci, value):
    ref = f"{chr(ord('A') + ci)}"
    return f'<c r="{ref}" t="inlineStr"><is><t>{_xml_escape(value)}</t></is></c>'


def make_workbook(rows, path, sheet_name="Vegetation units"):
    """Build a minimal single-sheet .xlsx on disk, same shape as
    pipelines/eunis/test_xlsx_to_csv.py's make_workbook."""
    workbook = (
        '<?xml version="1.0"?><workbook '
        'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
        'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
        f'<sheets><sheet name="{sheet_name}" sheetId="1" r:id="rId1"/></sheets></workbook>'
    )
    rels = (
        '<?xml version="1.0"?><Relationships '
        'xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" '
        'Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" '
        'Target="worksheets/sheet1.xml"/></Relationships>'
    )
    sheet_rows = "".join(
        "<row>" + "".join(_cell_xml(ci, v) for ci, v in enumerate(r)) + "</row>"
        for r in rows
    )
    sheet = (
        '<?xml version="1.0"?><worksheet '
        'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
        f"<sheetData>{sheet_rows}</sheetData></worksheet>"
    )
    with zipfile.ZipFile(path, "w") as z:
        z.writestr("xl/workbook.xml", workbook)
        z.writestr("xl/_rels/workbook.xml.rels", rels)
        z.writestr("xl/sharedStrings.xml", '<?xml version="1.0"?><sst></sst>')
        z.writestr("xl/worksheets/sheet1.xml", sheet)


class RankAndParentTest(unittest.TestCase):
    def test_class_code_has_no_parent(self):
        self.assertEqual(rank_and_parent("AA"), ("class", ""))

    def test_order_code_parent_is_the_class(self):
        self.assertEqual(rank_and_parent("AA01"), ("order", "AA"))

    def test_alliance_code_parent_is_the_order(self):
        self.assertEqual(rank_and_parent("AA01A"), ("alliance", "AA01"))

    def test_unrecognized_code_pattern_yields_empty_rank(self):
        self.assertEqual(rank_and_parent("not-a-code"), ("", ""))


class ConvertTest(unittest.TestCase):
    def test_writes_class_order_alliance_rows_with_derived_parents(self):
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "floraveg.xlsx")
            make_workbook(
                [
                    ["Code", "Name", "Author"],
                    ["AA", "Salicetea purpureae", "Moor 1958"],
                    ["AA01", "Salicetalia purpureae", "Moor 1958"],
                    ["AA01A", "Salicion albae", "Soó 1930"],
                ],
                xlsx,
            )
            out_dir = os.path.join(tmp, "out")
            os.makedirs(out_dir)
            report = convert(xlsx, out_dir)

            with open(os.path.join(out_dir, "syntaxa_hierarchy.csv"), encoding="utf-8") as f:
                content = f.read()
            self.assertIn("AA,class,Salicetea purpureae,Moor 1958,\n", content)
            self.assertIn("AA01,order,Salicetalia purpureae,Moor 1958,AA\n", content)
            self.assertIn("AA01A,alliance,Salicion albae,Soó 1930,AA01\n", content)
            self.assertEqual(report["classes"], 1)
            self.assertEqual(report["orders"], 1)
            self.assertEqual(report["alliances"], 1)
            self.assertEqual(report["skipped_rows"], 0)

    def test_skips_and_counts_a_code_matching_no_pattern(self):
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "floraveg.xlsx")
            make_workbook(
                [
                    ["Code", "Name", "Author"],
                    ["AA", "Salicetea purpureae", "Moor 1958"],
                    ["not-a-code", "Some legend row", ""],
                ],
                xlsx,
            )
            out_dir = os.path.join(tmp, "out")
            os.makedirs(out_dir)
            report = convert(xlsx, out_dir)
            self.assertEqual(report["classes"], 1)
            self.assertEqual(report["skipped_rows"], 1)

    def test_missing_required_header_raises_header_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "floraveg.xlsx")
            make_workbook([["Code", "Name"]], xlsx)  # no "Author" column
            out_dir = os.path.join(tmp, "out")
            os.makedirs(out_dir)
            with self.assertRaises(HeaderError):
                convert(xlsx, out_dir)


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 3: Test laufen lassen, erwartet: `ModuleNotFoundError`**

```bash
cd pipelines/eurovegchecklist && python3 -m unittest test_xlsx_to_csv -v
```

Erwartet: fehlschlagend, `xlsx_to_csv` existiert noch nicht.

- [ ] **Step 4: `xlsx_to_csv.py` implementieren**

```python
#!/usr/bin/env python3
"""Convert the pinned FloraVeg.EU EuroVegChecklist XLSX into
syntaxa_hierarchy.csv (code|rank|name|author|parent_code).

Same rationale as pipelines/eunis/xlsx_to_csv.py: an .xlsx is a zip of XML the
stdlib reads, so no spreadsheet library joins situs' dependency list.

rank and parent_code are derived EXCLUSIVELY from the code's own pattern
(class "AA" -> order "AA01" -> alliance "AA01A"), never guessed from the name
— author is a direct copy of FloraVeg's own, already-separated column, never a
heuristic split of a combined string.
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

CSV_HEADERS = {
    "syntaxa_hierarchy.csv": ["code", "rank", "name", "author", "parent_code"],
}

_NON_DATA_SHEETS = {"read me", "legend"}

_REQUIRED_HEADERS = ["Code", "Name", "Author"]
# The real FloraVeg export may spell "Author" differently — verified against
# the pinned file in Step 5 and extended here if so, never guessed silently.
_HEADER_ALIASES = {
    "Author": ["Author", "Authors", "Author(s)"],
}

_CLASS_RE = re.compile(r"^[A-Z]{2}$")
_ORDER_RE = re.compile(r"^[A-Z]{2}[0-9]{2}$")
_ALLIANCE_RE = re.compile(r"^[A-Z]{2}[0-9]{2}[A-Z]$")


class HeaderError(RuntimeError):
    """A data sheet is missing a column a parser needs — fail loudly instead
    of silently defaulting every cell to empty."""


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


def read_sheet(src, sheet_path):
    """Return the sheet as a list of equal-length string rows."""
    with zipfile.ZipFile(src) as zf:
        shared = _shared_strings(zf)
        root = ET.fromstring(zf.read(sheet_path))
    rows = []
    for row in root.iter(f"{{{NS['m']}}}row"):
        rows.append([_cell_text(c, shared) for c in row.findall("m:c", NS)])
    width = max((len(r) for r in rows), default=0)
    for r in rows:
        r.extend([""] * (width - len(r)))
    return rows


def _data_sheets(xlsx_path):
    """List (name, internal zip path) for every non-legend sheet, in workbook
    order — mirrors pipelines/eunis/xlsx_to_csv.py's _data_sheets."""
    with zipfile.ZipFile(xlsx_path) as zf:
        wb = ET.fromstring(zf.read("xl/workbook.xml"))
        rels = ET.fromstring(zf.read("xl/_rels/workbook.xml.rels"))
    relmap = {r.get("Id"): r.get("Target") for r in rels}
    sheets = []
    for sheet in wb.iter(f"{{{NS['m']}}}sheet"):
        name = (sheet.get("name") or "").strip()
        if name.lower() in _NON_DATA_SHEETS:
            continue
        target = relmap.get(sheet.get(_R_ID))
        if target:
            sheets.append((name, "xl/" + target))
    return sheets


def _row_index(header, aliases=None):
    idx = {h.strip(): i for i, h in enumerate(header)}
    for canonical, alternatives in (aliases or {}).items():
        if canonical in idx:
            continue
        for alt in alternatives:
            if alt in idx:
                idx[canonical] = idx[alt]
                break
    return idx


def _require_headers(idx, required, source, sheet):
    missing = [h for h in required if h not in idx]
    if missing:
        raise HeaderError(f"{source} [{sheet}]: missing required column(s) {missing}")


def _cell(row, idx, name, default=""):
    i = idx.get(name)
    return row[i].strip() if i is not None and i < len(row) else default


def rank_and_parent(code):
    """Derive (rank, parent_code) from the code's own pattern alone — never
    from the name. An unrecognized pattern yields ("", ""), so the caller can
    skip and count it instead of guessing."""
    code = code.strip()
    if _CLASS_RE.fullmatch(code):
        return "class", ""
    if _ORDER_RE.fullmatch(code):
        return "order", code[:2]
    if _ALLIANCE_RE.fullmatch(code):
        return "alliance", code[:-1]
    return "", ""


def convert(xlsx_path, out_dir):
    rows_out = []
    counts = {"classes": 0, "orders": 0, "alliances": 0}
    skipped = []

    for sheet_name, sheet_path in _data_sheets(xlsx_path):
        rows = read_sheet(xlsx_path, sheet_path)
        if not rows:
            continue
        idx = _row_index(rows[0], aliases=_HEADER_ALIASES)
        _require_headers(idx, _REQUIRED_HEADERS, xlsx_path, sheet_name)
        for row in rows[1:]:
            code = _cell(row, idx, "Code")
            if not code:
                continue
            rank, parent_code = rank_and_parent(code)
            if not rank:
                skipped.append((sheet_name, code))
                continue
            counts[rank + "es" if rank == "class" else rank + "s"] += 1
            rows_out.append({
                "code": code,
                "rank": rank,
                "name": _cell(row, idx, "Name"),
                "author": _cell(row, idx, "Author"),
                "parent_code": parent_code,
            })

    for sheet_name, code in skipped:
        print(f"skipped: {xlsx_path} [{sheet_name}]: code {code!r} matches no class/order/alliance pattern",
              file=sys.stderr)

    path = os.path.join(out_dir, "syntaxa_hierarchy.csv")
    with open(path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=CSV_HEADERS["syntaxa_hierarchy.csv"], lineterminator="\n")
        writer.writeheader()
        for row in rows_out:
            writer.writerow(row)

    report = {
        "classes": counts["classes"],
        "orders": counts["orders"],
        "alliances": counts["alliances"],
        "total_rows": len(rows_out),
        "skipped_rows": len(skipped),
    }
    with open(os.path.join(out_dir, "report.json"), "w", encoding="utf-8") as f:
        json.dump(report, f, indent=2, ensure_ascii=False, sort_keys=True)
        f.write("\n")
    return report


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--xlsx", required=True)
    parser.add_argument("--out-dir", required=True)
    args = parser.parse_args(argv)
    try:
        report = convert(args.xlsx, args.out_dir)
    except HeaderError as exc:
        print(f"error: {exc}", file=sys.stderr)
        sys.exit(1)
    print(json.dumps(report, indent=2, ensure_ascii=False, sort_keys=True))


if __name__ == "__main__":
    main()
```

Hinweis für den `rank + "es"`-Ausdruck: `"class" + "es"` = `"classes"`,
`"order" + "s"` = `"orders"`, `"alliance" + "s"` = `"alliances"` — trifft alle
drei Schlüssel korrekt; wer das für zu kryptisch hält, kann es stattdessen als
explizites `if/elif` schreiben, funktional identisch.

- [ ] **Step 5: Test laufen lassen, erwartet: grün**

```bash
cd pipelines/eurovegchecklist && python3 -m unittest test_xlsx_to_csv -v
```

- [ ] **Step 6: Echte Quelle beschaffen und Header/Muster gegenprüfen**

```bash
mkdir -p pipelines/eurovegchecklist/artifacts
curl -sSL -A "situs-ingest/0.1 (jo.brunner@mayflower.de)" \
  "https://files.ibot.cas.cz/cevs/downloads/floraveg/List_of_European_vegetation_units_version_4.xlsx" \
  -o pipelines/eurovegchecklist/artifacts/List_of_European_vegetation_units_version_4.xlsx
shasum -a 256 pipelines/eurovegchecklist/artifacts/List_of_European_vegetation_units_version_4.xlsx
```

Die reale Kopfzeile prüfen (Spaltennamen können von der Annahme in
`_REQUIRED_HEADERS` abweichen):

```bash
python3 - <<'EOF'
import sys
sys.path.insert(0, "pipelines/eurovegchecklist")
from xlsx_to_csv import _data_sheets, read_sheet
path = "pipelines/eurovegchecklist/artifacts/List_of_European_vegetation_units_version_4.xlsx"
for name, sheet_path in _data_sheets(path):
    rows = read_sheet(path, sheet_path)
    print(name, rows[0] if rows else "(empty)")
EOF
```

Weichen die realen Spaltennamen von `Code`/`Name`/`Author` ab: in
`_HEADER_ALIASES` (oder `_REQUIRED_HEADERS`, falls der kanonische Name selbst
anders heißt) die reale Schreibweise ergänzen — nie raten, immer gegen die
gedruckte Ausgabe oben abgleichen. Anschließend `manifest.yaml`s `sha256` und
`size_bytes` mit den echten, gerade gemessenen Werten befüllen.

- [ ] **Step 7: Pipeline gegen die echte Datei laufen lassen und gegen den Spike gegenprüfen**

```bash
cd pipelines/eurovegchecklist
python3 xlsx_to_csv.py --xlsx artifacts/List_of_European_vegetation_units_version_4.xlsx --out-dir out
cat out/report.json
```

Der Spike (Design-Spec, Zeile 20-21) maß **1841 Zeilen, 150 Klassen,
381 Ordnungen, 1310 Verbände, 0 übersprungene Zeilen**. Weicht `report.json`
davon ab, ist das ein echter Befund (neue Version der Quelle, andere
Spaltenbelegung) — dokumentieren, nicht stillschweigend anpassen.

- [ ] **Step 8: `README.md` schreiben**

```markdown
# pipelines/eurovegchecklist

Wandelt die gepinnte FloraVeg.EU-EuroVegChecklist-XLSX (Mucina et al. 2016 +
Updates, Version 4) in `syntaxa_hierarchy.csv` um, die `situs ingest` als
zweiten Syntaxa-Baustein neben den EUNIS-CSVs liest (siehe
`docs/superpowers/specs/2026-08-30-situs-syntaxa-hierarchie-design.md`).

**Nur Python-Stdlib**, wie `pipelines/eunis/`: `zipfile`,
`xml.etree.ElementTree`, `csv`, `re`, `json`, `argparse`. Python ≥ 3.9.

## Quelle beschaffen

Nicht eingecheckt (`artifacts/` ist gitignored). URL, SHA-256 und Lizenz stehen
in [`manifest.yaml`](manifest.yaml):

\`\`\`bash
mkdir -p artifacts
curl -sSL -A "situs-ingest/0.1 (<Kontakt-Mail>)" \
  "https://files.ibot.cas.cz/cevs/downloads/floraveg/List_of_European_vegetation_units_version_4.xlsx" \
  -o artifacts/List_of_European_vegetation_units_version_4.xlsx
shasum -a 256 artifacts/*.xlsx   # gegen manifest.yaml prüfen
\`\`\`

## Pipeline ausführen

\`\`\`bash
python3 xlsx_to_csv.py \
  --xlsx artifacts/List_of_European_vegetation_units_version_4.xlsx \
  --out-dir out
\`\`\`

Schreibt `out/syntaxa_hierarchy.csv` (`code,rank,name,author,parent_code`) und
`out/report.json` (Klassen/Ordnungen/Verbände/übersprungene Zeilen — gemessen,
siehe Design-Spec-Spike: 1841/150/381/1310/0).

`rank` und `parent_code` werden **ausschließlich** aus dem Code-Muster
abgeleitet (`AA` Klasse → `AA01` Ordnung → `AA01A` Verband, Elternteil durch
Suffix-Kürzung), nie aus dem Namen geraten. `author` ist eine direkte Kopie von
FloraVegs eigener Spalte, nie eine Textzerlegung eines Kombi-Strings.

## Tests

\`\`\`bash
python3 -m unittest discover -v
\`\`\`

Baut ihre XLSX-Fixtures im Speicher — kein Netzwerk, keine Binärdatei im Repo
nötig.
```

- [ ] **Step 9: `.gitignore` prüfen/ergänzen**

```bash
grep -q "pipelines/eurovegchecklist/artifacts/" .gitignore || \
  echo "pipelines/eurovegchecklist/artifacts/" >> .gitignore
grep -q "pipelines/eurovegchecklist/out/" .gitignore || \
  echo "pipelines/eurovegchecklist/out/" >> .gitignore
```

(Falls `pipelines/eunis/artifacts/` und `.../out/` bereits über ein
Wildcard-Muster wie `pipelines/*/artifacts/` erfasst sind, entfällt dieser
Schritt — vorher mit `git check-ignore -v pipelines/eurovegchecklist/artifacts/x`
prüfen.)

- [ ] **Step 10: Commit**

```bash
git add pipelines/eurovegchecklist/ .gitignore
git commit -m "feat(pipelines): FloraVeg EuroVegChecklist -> syntaxa_hierarchy.csv"
```

---

### Task 4: `IngestSyntaxaHierarchy` (Go-Applikationsschicht)

**Files:**
- Create: `internal/application/syntaxa_hierarchy_ingest.go`
- Create: `internal/application/syntaxa_hierarchy_ingest_test.go`

**Interfaces:**
- Consumes: `output.Repository.AllSyntaxa`, `output.IngestTx.UpsertSyntaxonAuthor`,
  `output.IngestTx.UpsertSyntaxon`, `output.Repository.Begin` (Task 1);
  `application.readAll`/`csvReader`/`newRowSkipper`/`headerIndex` (bereits in
  `internal/application/ingest.go`, gleiches Package — keine neue Datei nötig).
- Produces: `application.SyntaxaHierarchyReport{ClassesWritten, OrdersWritten,
  AlliancesMatched, AlliancesUnmatched int; AmbiguousMatches []string}`,
  `application.IngestSyntaxaHierarchy(ctx, repo, csvPath) (SyntaxaHierarchyReport, error)`.

- [ ] **Step 1: Fehlschlagenden Test schreiben**

```go
// internal/application/syntaxa_hierarchy_ingest_test.go
package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func writeHierarchyCSV(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "syntaxa_hierarchy.csv")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing syntaxa_hierarchy.csv: %v", err)
	}
	return path
}

func TestIngestSyntaxaHierarchy_WritesClassAndOrderRows(t *testing.T) {
	repo := newFakeRepo()
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir,
		"code,rank,name,author,parent_code\n"+
			"AA,class,Salicetea purpureae,Moor 1958,\n"+
			"AA01,order,Salicetalia purpureae,Moor 1958,AA\n")

	rep, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestSyntaxaHierarchy: %v", err)
	}
	if rep.ClassesWritten != 1 || rep.OrdersWritten != 1 {
		t.Errorf("ClassesWritten/OrdersWritten = %d/%d, want 1/1", rep.ClassesWritten, rep.OrdersWritten)
	}
	if len(repo.syntaxa) != 2 {
		t.Fatalf("syntaxa = %+v, want 2 rows written", repo.syntaxa)
	}
}

func TestIngestSyntaxaHierarchy_MatchesLongestAlliancePrefixAndSetsAuthorParent(t *testing.T) {
	repo := newFakeRepo()
	// An EUNIS alliance already in the index, as ingestSyntaxa would have left
	// it: full historical combi-string name, no author, no parent.
	repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{
		ID: "CAK-01C", Rank: "alliance", Name: "Cakilion edentulae Br.-Bl. 1931",
	})
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir,
		"code,rank,name,author,parent_code\n"+
			"AA01A,alliance,Cakilion edentulae,Br.-Bl. 1931,AA01\n")

	rep, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestSyntaxaHierarchy: %v", err)
	}
	if rep.AlliancesMatched != 1 || rep.AlliancesUnmatched != 0 {
		t.Errorf("AlliancesMatched/Unmatched = %d/%d, want 1/0", rep.AlliancesMatched, rep.AlliancesUnmatched)
	}
	if len(repo.authorUpdates) != 1 {
		t.Fatalf("authorUpdates = %+v, want exactly one", repo.authorUpdates)
	}
	got := repo.authorUpdates[0]
	if got.id != "CAK-01C" || got.author != "Br.-Bl. 1931" || got.parentID != "AA01" {
		t.Errorf("authorUpdate = %+v, want {CAK-01C, Br.-Bl. 1931, AA01}", got)
	}
}

func TestIngestSyntaxaHierarchy_UnmatchedAllianceIsCountedNotAborted(t *testing.T) {
	repo := newFakeRepo()
	repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{
		ID: "XYZ-01", Rank: "alliance", Name: "Nomatchion nowhereii",
	})
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir,
		"code,rank,name,author,parent_code\n"+
			"AA01A,alliance,Cakilion edentulae,Br.-Bl. 1931,AA01\n")

	rep, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestSyntaxaHierarchy: %v", err)
	}
	if rep.AlliancesMatched != 0 || rep.AlliancesUnmatched != 1 {
		t.Errorf("AlliancesMatched/Unmatched = %d/%d, want 0/1", rep.AlliancesMatched, rep.AlliancesUnmatched)
	}
	if len(repo.authorUpdates) != 0 {
		t.Errorf("authorUpdates = %+v, want none for an unmatched alliance", repo.authorUpdates)
	}
}

func TestIngestSyntaxaHierarchy_AmbiguousEqualLengthPrefixesAreNeverGuessed(t *testing.T) {
	repo := newFakeRepo()
	repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{
		ID: "AMB-01", Rank: "alliance", Name: "Salicion albae Soó 1930",
	})
	dir := t.TempDir()
	// Two FloraVeg alliance names share the same 11-character prefix
	// "Salicion al" with the EUNIS name above.
	path := writeHierarchyCSV(t, dir,
		"code,rank,name,author,parent_code\n"+
			"AA01A,alliance,Salicion albae,Soó 1930,AA01\n"+
			"AA02A,alliance,Salicion alberti,Nowak 1960,AA02\n")

	rep, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestSyntaxaHierarchy: %v", err)
	}
	if len(rep.AmbiguousMatches) != 1 || rep.AmbiguousMatches[0] != "AMB-01" {
		t.Errorf("AmbiguousMatches = %+v, want [AMB-01]", rep.AmbiguousMatches)
	}
	if len(repo.authorUpdates) != 0 {
		t.Errorf("authorUpdates = %+v, want none for an ambiguous match", repo.authorUpdates)
	}
}

func TestIngestSyntaxaHierarchy_MissingCSVIsNotAnError(t *testing.T) {
	repo := newFakeRepo()
	path := filepath.Join(t.TempDir(), "syntaxa_hierarchy.csv") // never written

	rep, err := IngestSyntaxaHierarchy(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestSyntaxaHierarchy: %v, want no error for a missing file", err)
	}
	if rep != (SyntaxaHierarchyReport{}) {
		t.Errorf("report = %+v, want the zero report", rep)
	}
}

func TestIngestSyntaxaHierarchy_BeginErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.beginErr = context.DeadlineExceeded
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir, "code,rank,name,author,parent_code\nAA,class,X,,\n")

	if _, err := IngestSyntaxaHierarchy(context.Background(), repo, path); err == nil {
		t.Fatal("IngestSyntaxaHierarchy = nil error, want the Begin error surfaced")
	}
}

func TestIngestSyntaxaHierarchy_AllSyntaxaErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.allSyntaxaErr = context.DeadlineExceeded
	dir := t.TempDir()
	path := writeHierarchyCSV(t, dir, "code,rank,name,author,parent_code\nAA,class,X,,\n")

	if _, err := IngestSyntaxaHierarchy(context.Background(), repo, path); err == nil {
		t.Fatal("IngestSyntaxaHierarchy = nil error, want the AllSyntaxa error surfaced")
	}
}
```

Testkommentar zur Ambiguitätsprobe: „längster Treffer gewinnt" heißt hier
konkret — der EUNIS-Name `"Salicion albae Soó 1930"` beginnt mit
`"Salicion albae"` (Präfix von `AA01A`, Länge 14) UND mit `"Salicion al"`
wäre auch ein Präfix von `"Salicion alberti"` (Länge 11, kürzer) — dieser Test
muss also so aufgebaut sein, dass **beide FloraVeg-Kandidaten denselben,
längsten Präfix** mit dem EUNIS-Namen teilen. Vor der Implementierung diesen
Fixture-Fall am echten Matching-Algorithmus (Step 3) verifizieren und bei
Bedarf die Testnamen anpassen, damit die Präfixlängen tatsächlich gleich sind
— nicht blind übernehmen.

- [ ] **Step 2: Test laufen lassen, erwartet: rot (Compile-Fehler)**

```bash
go test ./internal/application/... -run TestIngestSyntaxaHierarchy -v
```

- [ ] **Step 3: `syntaxa_hierarchy_ingest.go` implementieren**

```go
package application

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// SyntaxaHierarchyReport summarizes the FloraVeg hierarchy ingest + the
// EUNIS-alliance matching pass.
type SyntaxaHierarchyReport struct {
	ClassesWritten     int
	OrdersWritten      int
	AlliancesMatched   int // EUNIS-Verbände mit gefundenem FloraVeg-Elternteil
	AlliancesUnmatched int // EUNIS-Verbände ohne Treffer (ParentID bleibt leer)
	AmbiguousMatches   []string
}

// hierarchyRow is one parsed line of syntaxa_hierarchy.csv.
type hierarchyRow struct {
	code, rank, name, author, parentCode string
}

// IngestSyntaxaHierarchy loads csvPath (syntaxa_hierarchy.csv:
// code,rank,name,author,parent_code, produced by pipelines/eurovegchecklist)
// into repo. It writes every class/order row as a new syntaxon, then matches
// every EUNIS alliance already in the index against the FloraVeg alliance
// names by longest-prefix — a match lends Author and ParentID, a non-match
// stays untouched, an equal-length ambiguous match is never guessed.
//
// A missing csvPath is "no hierarchy data yet", not an error, mirroring
// IngestLocalizations: the caller (situs ingest) must keep working on an
// index that has no FloraVeg CSV pinned.
func IngestSyntaxaHierarchy(ctx context.Context, repo output.Repository, csvPath string) (SyntaxaHierarchyReport, error) {
	if _, err := os.Stat(csvPath); err != nil {
		if os.IsNotExist(err) {
			slog.InfoContext(ctx, "no syntaxa hierarchy file, skipping", "path", csvPath)
			return SyntaxaHierarchyReport{}, nil
		}
		return SyntaxaHierarchyReport{}, fmt.Errorf("statting %s: %w", csvPath, err)
	}

	rows, skipped, err := readHierarchyRows(csvPath)
	if err != nil {
		return SyntaxaHierarchyReport{}, err
	}

	existing, err := repo.AllSyntaxa(ctx)
	if err != nil {
		return SyntaxaHierarchyReport{}, fmt.Errorf("reading existing syntaxa: %w", err)
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return SyntaxaHierarchyReport{}, fmt.Errorf("beginning syntaxa hierarchy ingest transaction: %w", err)
	}

	rep, err := ingestHierarchyRows(tx, rows, existing)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return SyntaxaHierarchyReport{}, fmt.Errorf("%w (rollback also failed: %w)", err, rbErr)
		}
		return SyntaxaHierarchyReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyntaxaHierarchyReport{}, fmt.Errorf("committing syntaxa hierarchy ingest transaction: %w", err)
	}

	if skipped > 0 {
		slog.WarnContext(ctx, "skipped malformed rows in syntaxa_hierarchy.csv", "skipped", skipped)
	}
	return rep, nil
}

// readHierarchyRows parses csvPath's data rows, skipping (and counting) any
// row with the wrong field count or an unrecognized rank, same tolerance as
// every other ingest file in this package.
func readHierarchyRows(csvPath string) ([]hierarchyRow, int, error) {
	dir, file := splitDir(csvPath)
	var rows []hierarchyRow
	skipped := 0
	skip := newRowSkipper(&skipped, file, "syntaxon hierarchy")
	err := readAll(context.Background(), dir, file,
		[]string{"code", "rank", "name", "author", "parent_code"}, skip,
		func(idx map[string]int, row []string, line int) error {
			rank := row[idx["rank"]]
			switch rank {
			case "class", "order", "alliance":
			default:
				skip(line, fmt.Errorf("unknown rank %q", rank))
				return nil
			}
			rows = append(rows, hierarchyRow{
				code:       row[idx["code"]],
				rank:       rank,
				name:       row[idx["name"]],
				author:     row[idx["author"]],
				parentCode: row[idx["parent_code"]],
			})
			return nil
		})
	if err != nil {
		return nil, 0, err
	}
	return rows, skipped, nil
}

// ingestHierarchyRows writes every class/order row as a new syntaxon and
// matches every already-indexed EUNIS alliance against the FloraVeg alliance
// rows by longest-name-prefix.
func ingestHierarchyRows(tx output.IngestTx, rows []hierarchyRow, existing []domain.Syntaxon) (SyntaxaHierarchyReport, error) {
	var rep SyntaxaHierarchyReport
	var allianceRows []hierarchyRow

	for _, r := range rows {
		switch r.rank {
		case "class", "order":
			if err := tx.UpsertSyntaxon(domain.Syntaxon{
				ID: r.code, Rank: r.rank, Name: r.name, Author: r.author, ParentID: r.parentCode,
			}); err != nil {
				return SyntaxaHierarchyReport{}, fmt.Errorf("upserting %s %s: %w", r.rank, r.code, err)
			}
			if r.rank == "class" {
				rep.ClassesWritten++
			} else {
				rep.OrdersWritten++
			}
		case "alliance":
			allianceRows = append(allianceRows, r)
		}
	}

	for _, e := range existing {
		if e.Rank != "alliance" {
			continue
		}
		match, ambiguous := longestPrefixMatch(e.Name, allianceRows)
		if ambiguous {
			rep.AmbiguousMatches = append(rep.AmbiguousMatches, e.ID)
			continue
		}
		if match == nil {
			rep.AlliancesUnmatched++
			continue
		}
		if err := tx.UpsertSyntaxonAuthor(e.ID, match.author, match.parentCode); err != nil {
			return SyntaxaHierarchyReport{}, fmt.Errorf("setting author of %s: %w", e.ID, err)
		}
		rep.AlliancesMatched++
	}
	sort.Strings(rep.AmbiguousMatches)
	return rep, nil
}

// longestPrefixMatch finds the FloraVeg alliance whose name is the longest
// prefix of eunisName. Two candidates tied at the same longest length are
// reported as ambiguous (match == nil, ambiguous == true) — never guessed.
func longestPrefixMatch(eunisName string, candidates []hierarchyRow) (match *hierarchyRow, ambiguous bool) {
	bestLen := -1
	var best *hierarchyRow
	tie := false
	for i := range candidates {
		c := &candidates[i]
		if c.name == "" || !strings.HasPrefix(eunisName, c.name) {
			continue
		}
		l := len(c.name)
		switch {
		case l > bestLen:
			bestLen, best, tie = l, c, false
		case l == bestLen:
			tie = true
		}
	}
	if tie {
		return nil, true
	}
	return best, false
}
```

Für `splitDir` (Verzeichnis+Dateiname aus einem vollen Pfad, wie
`IngestLocalizations` es über `filepath.Split` schon tut) entweder
`filepath.Split` direkt inline verwenden — dann entfällt die eigene Funktion:

```go
	dir, file := filepath.Split(csvPath)
```

(`"path/filepath"` zu den Imports von `syntaxa_hierarchy_ingest.go`
hinzufügen, `splitDir` NICHT als eigene Funktion einführen — das obige
`readHierarchyRows` entsprechend mit `filepath.Split(csvPath)` statt
`splitDir(csvPath)` schreiben.)

- [ ] **Step 4: Tests laufen lassen, erwartet: grün**

```bash
go test ./internal/application/... -run TestIngestSyntaxaHierarchy -v
```

Schlägt der Ambiguitäts-Test fehl, weil die gewählten Testnamen doch keinen
gleich langen Präfix teilen: die Namen in
`TestIngestSyntaxaHierarchy_AmbiguousEqualLengthPrefixesAreNeverGuessed`
anpassen, bis `len(candidate.name)` für beide Kandidaten identisch ist und
beide echte Präfixe des EUNIS-Namens sind — z. B. beide FloraVeg-Namen auf
exakt dieselbe Zeichenkette kürzen, die dann als gemeinsamer Präfix dient.

- [ ] **Step 5: Ganzes Package testen**

```bash
go build ./... && go test ./internal/application/... -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/application/syntaxa_hierarchy_ingest.go \
  internal/application/syntaxa_hierarchy_ingest_test.go
git commit -m "feat(application): IngestSyntaxaHierarchy matches EUNIS alliances to FloraVeg"
```

---

### Task 5: In `situs ingest` verdrahten

**Files:**
- Modify: `cmd/situs/ingest.go`
- Modify: `cmd/situs/ingest_test.go`

**Interfaces:**
- Consumes: `application.IngestSyntaxaHierarchy`, `application.SyntaxaHierarchyReport`
  (Task 4).
- Produces: `ingestOutput.SyntaxaHierarchy application.SyntaxaHierarchyReport`
  im gedruckten JSON-Report.

- [ ] **Step 1: Fehlschlagenden Test schreiben**

In `cmd/situs/ingest_test.go`, neuer Test nach
`TestIngestCommandLoadsCSVsAndPrintsTheReport`:

```go
func TestIngestCommandRunsSyntaxaHierarchyIngestAfterEUNISSyntaxa(t *testing.T) {
	stubHostus(t)
	dir := seedIngestDir(t)
	// Overwrite syntaxa.csv/habitat_type_syntaxa.csv with one real EUNIS
	// alliance so the hierarchy step has something to match against.
	writeIngestCSV(t, dir, "syntaxa.csv", "id,rank,name,parent_id\nCAK-01C,alliance,Cakilion edentulae Br.-Bl. 1931,\n")
	writeIngestCSV(t, dir, "habitat_type_syntaxa.csv", "typology_id,code,syntaxon_id\neunis@2021,R22,CAK-01C\n")
	writeIngestCSV(t, dir, "syntaxa_hierarchy.csv",
		"code,rank,name,author,parent_code\n"+
			"AA01,order,Cakiletalia,Tüxen 1950,AA\n"+
			"AA01A,alliance,Cakilion edentulae,Br.-Bl. 1931,AA01\n")
	dbPath := filepath.Join(t.TempDir(), "situs.sqlite")

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"ingest", "--csv-dir", dir, "--db", dbPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("executing ingest: %v", err)
	}
	if !strings.Contains(out.String(), `"AlliancesMatched": 1`) {
		t.Errorf("output = %q, want the report to show one matched alliance", out.String())
	}
	if !strings.Contains(out.String(), `"OrdersWritten": 1`) {
		t.Errorf("output = %q, want the report to show one FloraVeg order written", out.String())
	}
}

func TestIngestCommandRunsWithoutASyntaxaHierarchyFile(t *testing.T) {
	stubHostus(t)
	dir := seedIngestDir(t) // no syntaxa_hierarchy.csv written
	dbPath := filepath.Join(t.TempDir(), "situs.sqlite")

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"ingest", "--csv-dir", dir, "--db", dbPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("executing ingest without syntaxa_hierarchy.csv: %v", err)
	}
	if !strings.Contains(out.String(), `"ClassesWritten": 0`) {
		t.Errorf("output = %q, want a present-but-zero SyntaxaHierarchy report", out.String())
	}
}
```

- [ ] **Step 2: Test laufen lassen, erwartet: rot**

```bash
go test ./cmd/situs/... -run TestIngestCommandRuns.*SyntaxaHierarchy -v
```

Erwartet: fehlschlagend — der Ingest-Schritt existiert noch nicht, die
Report-Felder erscheinen nicht im JSON.

- [ ] **Step 3: `cmd/situs/ingest.go` verdrahten**

`ingestOutput` erweitern (nach `Localizations int`):

```go
type ingestOutput struct {
	application.IngestReport
	Species            application.SpeciesReport
	ResolutionRate     float64
	Distribution       application.DistributionReport
	DistributionFailed int
	Localizations      int
	DerivedLabels      int
	SyntaxaHierarchy   application.SyntaxaHierarchyReport
}
```

In `runIngest`, direkt nach dem `IngestCSV`-Aufruf (vor
`IngestSpeciesRoles` — EUNIS-Verbände müssen existieren, bevor die
Hierarchie sie abgleicht, und diese Reihenfolge steht auch am wenigsten quer
zu den übrigen, unabhängigen Schritten):

```go
	report, err := application.IngestCSV(ctx, db, csvDir)
	if err != nil {
		return fmt.Errorf("ingesting %q: %w", csvDir, err)
	}

	// Runs right after the EUNIS alliances are indexed (IngestCSV, above) and
	// before species/localization/derivation, which do not depend on it and
	// which it does not depend on.
	hierarchyReport, err := application.IngestSyntaxaHierarchy(ctx, db, filepath.Join(csvDir, "syntaxa_hierarchy.csv"))
	if err != nil {
		return fmt.Errorf("ingesting syntaxa hierarchy from %q: %w", csvDir, err)
	}

	speciesReport, err := application.IngestSpeciesRoles(ctx, db, resolver, filepath.Join(csvDir, "species_roles.csv"))
```

Und `out := ingestOutput{...}` um `SyntaxaHierarchy: hierarchyReport,`
ergänzen.

Achtung: `resolver := hostus.NewClient(...)` steht im bestehenden Code
**zwischen** `IngestCSV` und `IngestSpeciesRoles` (siehe Read oben, Zeile
176-177) — beim Einfügen des neuen Schritts diese Zeile vor oder nach dem
neuen Aufruf stehen lassen, Hauptsache `resolver` ist definiert, bevor
`IngestSpeciesRoles` es braucht. Reihenfolge im fertigen Code:
`IngestCSV` → `IngestSyntaxaHierarchy` → `resolver := hostus.NewClient(...)` →
`IngestSpeciesRoles` → ... (unverändert ab hier).

- [ ] **Step 4: Tests laufen lassen, erwartet: grün**

```bash
go test ./cmd/situs/... -v
```

- [ ] **Step 5: `seedIngestDir` unverändert lassen — Rückwärtskompatibilität prüfen**

`seedIngestDir` (in `cmd/situs/ingest_test.go`) schreibt bewusst **keine**
`syntaxa_hierarchy.csv` — das ist genau der "Datei fehlt"-Pfad, den
`TestIngestCommandLoadsCSVsAndPrintsTheReport` (unverändert) weiter durchläuft.
Diesen bestehenden Test explizit erneut laufen lassen:

```bash
go test ./cmd/situs/... -run TestIngestCommandLoadsCSVsAndPrintsTheReport -v
```

- [ ] **Step 6: Ganzen Build/Test-Lauf prüfen**

```bash
go build ./... && go test ./... 
```

- [ ] **Step 7: Commit**

```bash
git add cmd/situs/ingest.go cmd/situs/ingest_test.go
git commit -m "feat(cmd): wire IngestSyntaxaHierarchy into situs ingest"
```

---

### Task 6: Read-API — `SyntaxonRef.Author`/`ParentID`

**Files:**
- Modify: `internal/ports/input/services.go`
- Modify: `internal/application/query.go`
- Modify: `internal/adapters/http/openapi.yaml`
- Modify: `api/openapi/openapi.yaml`
- Modify: `internal/adapters/http/habitat_test.go`

**Interfaces:**
- Consumes: `domain.Syntaxon.Author`/`ParentID` (Task 1).
- Produces: `input.SyntaxonRef{ID, Rank, Name, Author, ParentID string}` mit
  `Author`/`ParentID` als `omitempty` auf dem Wire.

- [ ] **Step 1: Fehlschlagenden HTTP-Test schreiben**

In `internal/adapters/http/habitat_test.go` einen Test suchen, der
`syntaxa` im JSON-Response prüft (z. B. den Test rund um
`GET /v1/habitat-type/{typology}/{code}`), und dort ergänzen bzw. einen
neuen Test hinzufügen, der prüft, dass ein Syntaxon mit gesetztem `Author`/
`ParentID` beide Felder im JSON zeigt, UND dass ein Syntaxon ohne beide
(EUNIS-Verband ohne FloraVeg-Treffer) sie **auslässt**:

```go
func TestHabitatType_SyntaxonAuthorAndParentIDAreOmittedWhenEmpty(t *testing.T) {
	// Wire the handler with a QueryService whose HabitatType returns one
	// syntaxon with Author/ParentID set and, if the existing fixture already
	// has a second unmatched syntaxon, verify that one omits both fields.
	// Follow this file's existing setup pattern (stub QueryService or seeded
	// sqlite DB — match whatever the surrounding tests in this file already
	// use) and assert on the raw response body:
	//   body must contain `"author":"Br.-Bl. 1931"` and `"parent_id":"AA01"`
	//   for the matched syntaxon entry, and must NOT contain `"author":`
	//   inside the unmatched syntaxon's JSON object.
}
```

Diesen Test **konkret** nach dem tatsächlichen Setup-Stil der Nachbartests in
`habitat_test.go` ausformulieren (stub `input.QueryService` vs. echte
sqlite-`DB` — im File nachsehen, welches Muster dort für
`GET /v1/habitat-type/...` bereits verwendet wird, und exakt diesem Muster
folgen statt einer neuen Teststrategie).

- [ ] **Step 2: Test laufen lassen, erwartet: rot**

```bash
go test ./internal/adapters/http/... -run TestHabitatType_SyntaxonAuthor -v
```

- [ ] **Step 3: `SyntaxonRef` und `query.go` erweitern**

`internal/ports/input/services.go`:

```go
// SyntaxonRef is a vegetation unit a habitat type is linked to.
type SyntaxonRef struct {
	ID     string `json:"id"`
	Rank   string `json:"rank"`
	Name   string `json:"name"`
	// Author is the authorship citation, taken verbatim from FloraVeg.EU's own
	// already-separated column — absent when no FloraVeg match was found, never
	// a guessed split of Name.
	Author string `json:"author,omitempty"`
	// ParentID references a FloraVeg order code (class -> order -> alliance
	// hierarchy) — absent when unknown. A plain string reference, not a
	// schema-bound foreign key: two id schemes (EUNIS alliance codes, FloraVeg
	// class/order codes) coexist here.
	ParentID string `json:"parent_id,omitempty"`
}
```

`internal/application/query.go`, `syntaxaOf`:

```go
func (q *QueryService) syntaxaOf(ctx context.Context, key domain.HabitatTypeKey) ([]input.SyntaxonRef, error) {
	syntaxa, err := q.repo.Syntaxa(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("fetching syntaxa of %s: %w", key, err)
	}
	out := make([]input.SyntaxonRef, 0, len(syntaxa))
	for _, s := range syntaxa {
		out = append(out, input.SyntaxonRef{ID: s.ID, Rank: s.Rank, Name: s.Name, Author: s.Author, ParentID: s.ParentID})
	}
	return out, nil
}
```

`internal/adapters/http/openapi.yaml` **und** `api/openapi/openapi.yaml`
(beide identisch ändern), `SyntaxonRef`-Schema:

```yaml
    SyntaxonRef:
      type: object
      required: [id, rank, name]
      properties:
        id:
          type: string
        rank:
          type: string
          enum: [class, order, alliance]
        name:
          type: string
        author:
          type: string
          description: >-
            Autorschafts-Zitat, direkt aus FloraVeg.EU übernommen; fehlt, wenn
            kein FloraVeg-Treffer gefunden wurde.
        parent_id:
          type: string
          description: >-
            Verweist auf einen FloraVeg-Klasse-/Ordnungscode (reine
            String-Referenz, kein Fremdschlüssel); fehlt, wenn unbekannt.
```

- [ ] **Step 4: Tests laufen lassen, erwartet: grün**

```bash
go test ./internal/adapters/http/... -v
go test ./internal/application/... -v
```

- [ ] **Step 5: OpenAPI-Kontrakttest explizit laufen lassen**

```bash
go test ./internal/adapters/http/... -run TestOpenAPI -v
```

(Name des Kontrakttests im Verzeichnis vorher mit
`grep -rn "func Test" internal/adapters/http/*openapi*_test.go` bestätigen,
falls er anders heißt — er hält die Byte-Gleichheit der zwei
`openapi.yaml`-Kopien und die Routen↔Spec-Deckung fest und muss grün
bleiben.)

- [ ] **Step 6: Ganzen Build/Test-Lauf prüfen**

```bash
make verify
```

- [ ] **Step 7: Commit**

```bash
git add internal/ports/input/services.go internal/application/query.go \
  internal/adapters/http/openapi.yaml api/openapi/openapi.yaml \
  internal/adapters/http/habitat_test.go
git commit -m "feat(http): expose SyntaxonRef.author/parent_id"
```

---

### Task 7: Echten Ingest fahren, Dokumentation nachziehen

**Files:**
- Modify: `docs/reference/measured-index.md`

**Interfaces:**
- Consumes: nichts Neues — nur die fertige Pipeline und den fertigen Ingest.

- [ ] **Step 1: Vollen `situs ingest` mit beiden Pipelines lokal fahren**

```bash
python3 pipelines/eunis/xlsx_to_csv.py \
  --eunis-xlsx pipelines/eunis/artifacts/eunis-2021-including-crosswalks.xlsx \
  --annex1-xlsx pipelines/eunis/artifacts/eunis-2021-annex1-separate-rows.xlsx \
  --esy-xlsx pipelines/eunis/artifacts/esy-characteristic-species-combinations.xlsx \
  --out-dir /tmp/situs-ingest
python3 pipelines/eurovegchecklist/xlsx_to_csv.py \
  --xlsx pipelines/eurovegchecklist/artifacts/List_of_European_vegetation_units_version_4.xlsx \
  --out-dir /tmp/situs-ingest
go run ./cmd/situs ingest --csv-dir /tmp/situs-ingest --db /tmp/situs.sqlite
```

Den gedruckten JSON-Report lesen: `ClassesWritten`, `OrdersWritten`,
`AlliancesMatched`, `AlliancesUnmatched`, `AmbiguousMatches` sind die real
gemessenen Zahlen für Schritt 2 — **nicht** die im Spike genannten
150/381/1310 blind übernehmen, sondern die tatsächliche `AlliancesMatched`-Quote
gegen die EUNIS-Verbände (aus `docs/reference/measured-index.md`s bestehendem
„Syntaxa: 1050"-Wert) ablesen.

- [ ] **Step 2: `docs/reference/measured-index.md` aktualisieren**

Den Abschnitt „## Syntaxa-Tiefe (offener Punkt 1)" um die neuen, echten Zahlen
ergänzen (Beispieltext — mit den in Step 1 tatsächlich gemessenen Werten
befüllen, nicht mit diesem Platzhaltertext selbst):

```markdown
## Syntaxa-Tiefe (offener Punkt 1)

Vorkommende Ränge: **`alliance`** (Verband) und **`order`** (Ordnung). Der
Verband ist praktisch durchgehend das Ende; gemessen gibt es genau **eine**
Ausnahme auf Ordnungs-Ebene (`Moltkeetalia petraeae`). **Assoziationen kommen
nicht vor** und sind in keiner freien paneuropäischen Quelle verfügbar — das ist
die dokumentierte Decke, kein Versäumnis.

### Volle Hierarchie via FloraVeg.EU (2026-08-30)

`pipelines/eurovegchecklist/` lädt die EuroVegChecklist (Mucina et al. 2016 +
Updates, Version 4) und liefert Klasse/Ordnung/Verband mit Codes und
Autorschaft. Gemessen gegen die reale Datei: **<N_ROWS>** Zeilen, **<N_CLASSES>**
Klassen, **<N_ORDERS>** Ordnungen, **<N_ALLIANCES>** Verbände (siehe
`pipelines/eurovegchecklist/out/report.json`).

Von den **1050** EUNIS-Verbänden im Index fanden **<ALLIANCES_MATCHED>** einen
FloraVeg-Namenstreffer (Author + ParentID übernommen), **<ALLIANCES_UNMATCHED>**
blieben ohne Treffer (Name bleibt der historische EUNIS-Kombi-String,
`Author=""`), **<N_AMBIGUOUS>** Mehrfachtreffer wurden nicht geraten
(`AmbiguousMatches`).
```

- [ ] **Step 3: Doc-Drift prüfen**

```bash
./scripts/check-doc-drift.sh
```

Datei existiert eventuell unter anderem Pfad — vorher mit
`find . -iname "*doc-drift*"` bestätigen und den tatsächlichen Pfad
verwenden.

- [ ] **Step 4: `make verify` als Abschluss**

```bash
make verify
```

- [ ] **Step 5: Commit**

```bash
git add docs/reference/measured-index.md
git commit -m "docs(reference): measure the FloraVeg syntaxa hierarchy against the real index"
```

---

## Self-Review (bereits durchgeführt)

- **Spec-Abdeckung:** Architektur/Datenfluss → Task 3+4; Domäne (`Author`) →
  Task 1; Ingest-Reihenfolge (nach `ingestSyntaxa`, vor Localization/Derivation)
  → Task 5 Step 3; Read-API (`SyntaxonRef`) → Task 6; alle vier
  Fehlerbehandlungs-Fälle der Tabelle → Task 4 Tests (kein Treffer, Muster
  passt nicht, Mehrdeutigkeit, fehlende Datei); alle vier „Prüfbare Zusagen" →
  jeweils durch einen konkreten Test in Task 4/6 abgesichert; Test-Dateien aus
  Abschnitt 5 der Spec → `internal/application/syntaxa_hierarchy_ingest_test.go`
  (Task 4) und `internal/adapters/http/habitat_test.go` (Task 6) — die Spec
  nennt zusätzlich `pipelines/eurovegchecklist/xlsx_to_csv_test.py`; dieser
  Plan folgt stattdessen der bereits im Repo etablierten Namenskonvention
  `test_xlsx_to_csv.py` (siehe `pipelines/eunis/`), inhaltlich identisch
  abgedeckt.
- **Platzhalter-Scan:** Task 6 Step 1 enthält bewusst keinen ausformulierten
  Testkörper, sondern eine Anweisung, dem Nachbar-Testmuster zu folgen — das
  ist eine reale Unsicherheit (welches Test-Setup `habitat_test.go` schon
  benutzt, unbekannt ohne den vollen Dateiinhalt zu lesen), keine Bequemlichkeit.
  Jeder andere Testkörper im Plan ist vollständig ausformuliert.
- **Typkonsistenz:** `domain.Syntaxon{ID,Rank,Name,Author,ParentID}` (Task 1)
  → `input.SyntaxonRef{ID,Rank,Name,Author,ParentID}` (Task 6) →
  `SyntaxaHierarchyReport{ClassesWritten,OrdersWritten,AlliancesMatched,
  AlliancesUnmatched,AmbiguousMatches}` (Task 4) → `ingestOutput.SyntaxaHierarchy`
  (Task 5) — Feldnamen sind über alle Tasks hinweg identisch verwendet.

## Execution Handoff

Plan gespeichert unter
`docs/superpowers/plans/2026-08-30-situs-syntaxa-hierarchie.md`. Zwei
Ausführungsoptionen:

1. **Subagent-getrieben (empfohlen)** — frischer Subagent pro Task, Review
   zwischen den Tasks, schnelle Iteration.
2. **Inline-Ausführung** — Tasks in dieser Sitzung mit Checkpoints zur Review.

Welcher Ansatz?
