# Aggregat-Mitgliedsarten & dateibasierter Species-Ingest Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **Historical note (post-execution):** two details below were corrected
> during implementation and are left unedited here as the execution record
> — do not copy these snippets. `SpeciesReport.AmbiguousCrosswalk` is
> documented below as `[]string` (a list of ambiguous names); it shipped as
> an `int` counter instead (logged per occurrence via `slog.Warn`, not
> accumulated into a list). `AggregateSource.Name` is documented below as a
> required `json:"name"` field; the DTO ships it `omitempty` and the
> service currently never populates it (only the concept id is persisted).
> See `internal/application/species_ingest.go` and
> `internal/ports/input/services.go` for the authoritative shapes.

**Goal:** `IngestSpeciesRoles` löst Artnamen nicht mehr live gegen hostus auf,
sondern gegen eine lokale, deterministische Crosswalk-Datei
(`eurosl_crosswalk.csv`), und leitet für jede aufgelöste Sammelart
zusätzliche, sichtbar als abgeleitet markierte `SpeciesRole`-Zeilen für ihre
Mitgliedsarten ab (`aggregate_members.csv`).

**Architecture:** `species_ingest.go` verliert seinen `output.NameResolver`-
Parameter und bekommt stattdessen zwei CSV-Pfade. Auflösung wird ein reiner
Dictionary-Lookup (kein Netzwerk, keine Fuzzy-Logik) — ein Name mit mehr als
einer Concept-ID in der Crosswalk-Datei ist ein Datenfund (`AmbiguousCrosswalk`),
nie ein Rateanlass. Für jede Zeile, deren aufgelöstes Konzept selbst eine in
`aggregate_members.csv` gelistete Sammelart ist, schreibt der Ingest je
Mitglied eine zusätzliche `SpeciesRole{Provenance: "derived_from_aggregate"}` —
außer eine explizite Zeile für dasselbe `(Key, VerbatimName, Role)` existiert
bereits; die gewinnt immer, unabhängig von der Verarbeitungsreihenfolge.
`IngestDistribution` (Live-Aufruf an hostus, Konzept → Gebiete) bleibt
unverändert; nur die Namensauflösung wird dateibasiert.

**Tech Stack:** Go 1.26, keine neue Abhängigkeit. Kein neues Python-Pipeline-
Verzeichnis — `eurosl_crosswalk.csv`/`aggregate_members.csv` sind hostus-
seitige Exporte (siehe „Abhängigkeit" unten), situs liest sie nur.

**Spec:** `docs/superpowers/specs/2026-08-29-situs-aggregat-mitgliedsarten-design.md`
(liegt auf Branch `feature/aggregat-mitgliedsarten`; dieser Plan wurde gegen
den tatsächlichen Code auf `main`/`feature/syntaxa-hierarchie` verifiziert,
nicht blind aus der Spec übernommen — siehe die Abweichungen, die unten
explizit benannt sind).

## Abhängigkeit auf hostus — was das für diesen Plan bedeutet

Die Spec setzt zwei hostus-seitige CSV-Exporte voraus
(`docs/superpowers/specs/2026-08-29-hostus-eurosl-export-design.md`, hostus-
Repo), die **zum Zeitpunkt dieses Plans nicht existieren**. Dieser Plan ist
davon nur an einer Stelle betroffen: **Task 5** (Doku/Messung) kann keinen
echten `situs ingest`-Lauf mit echten Dateien fahren und daher keine echte
Trefferquote messen — er dokumentiert den Mechanismus und trägt `0`/„noch
nicht gepinnt" ein, exakt wie `docs/reference/measured-index.md` es heute
schon für die deutschen Labels tut (`Localizations: 0`, blockiert auf
EUR-Lex). Tasks 1-4 sind vollständig code- und testbar **ohne** die echten
Dateien — ihre Tests bauen eigene, kleine Fixture-CSVs, wie jeder andere
Ingest-Test in diesem Repo.

## Bewusste Abweichung von der Spec — `output.NameResolver` bleibt bestehen

Die Spec fordert unter „Prüfbare Zusagen" nur: „`species_ingest.go`
importiert nach diesem Umbau `internal/adapters/hostus` nicht mehr". Das ist
bereits heute wahr — `species_ingest.go` importierte hostus nie direkt, nur
über den injizierten Port `output.NameResolver` (`hostus.Client` wird
ausschließlich in `cmd/situs/ingest.go` konstruiert). Nach diesem Umbau wird
`output.NameResolver` und `hostus.Client.Resolve()` (inkl. der Batch-/
Downshift-Logik, `DefaultBatchSize`, `SITUS_HOSTUS_BATCH_SIZE`) toter, aber
weiterhin von zehn eigenen Tests abgedeckter Code — nichts ruft `Resolve`
mehr auf. Dieser Plan **löscht das bewusst nicht**: die Spec verlangt es
nicht explizit, und die Löschung würde in Config (`HostusConfig.BatchSize`)
und in `internal/adapters/hostus/client_test.go` kaskadieren, was ein
eigener, absichtlich gescopter Aufräum-Task wäre, keiner, den dieser Plan
nebenbei mitreißen sollte. Stattdessen hält Task 2 einen Architektur-Test
fest, der zusichert, dass `internal/application` `internal/adapters/hostus`
nicht importiert (siehe unten) — die Spec-Zusage bleibt geprüft, ohne den
toten Code anzufassen.

## Global Constraints

- Kein neuer direkter Dependency (Go stdlib + bestehende Allowlist reicht).
- SQL bleibt statische `?`-Platzhalter-Syntax, nie String-Konkatenation
  (gosec G201/G202).
- Eine abgeleitete Zeile (`Provenance == "derived_from_aggregate"`) hat
  **immer** `Fidelity == nil && Constancy == nil` — diese Werte wurden nie
  für das Mitglied selbst gemessen.
- Eine explizite `species_roles.csv`-Zeile gewinnt **immer** gegen eine
  abgeleitete für denselben `(Key, VerbatimName, Role)` — unabhängig von der
  Verarbeitungsreihenfolge der beiden Dateien innerhalb eines Laufs.
- Ein bestehender API-Client, der `provenance`/`derived_from` ignoriert, sieht
  für jede schon heute existierende (beobachtete) Zeile keine Änderung
  (`provenance` ist `omitempty` und wird auf dem Wire nur bei
  `derived_from_aggregate` gesetzt).
- Ein mehrdeutiger Crosswalk-Eintrag bricht den Ingest **nicht** ab —
  gezählt, gemeldet, nie geraten.
- Fehlt `aggregate_members.csv`, läuft der Ingest ohne Mitgliedsarten-
  Ableitung weiter (`DerivedRows=0`, geloggt) — die Sammelarten-Zeilen selbst
  werden trotzdem korrekt eingetragen. Fehlt `eurosl_crosswalk.csv`
  **dagegen**, bricht der Ingest ab (siehe Task 3 Step 3 für die Begründung —
  ohne sie kann keine einzige Zeile auflösen, dieselbe Schwere wie der
  frühere Resolver-Ausfall).
- `make verify` muss vor jedem Commit grün sein. TDD: Test schreiben → rot
  sehen → implementieren → grün.

---

## File Structure

| Datei | Zweck |
|---|---|
| `internal/domain/habitat.go` | `SpeciesRole.Provenance`/`DerivedFrom` neu |
| `internal/adapters/sqlite/schema.sql` | `species_role.provenance`/`derived_from`-Spalten |
| `internal/adapters/sqlite/write.go` | `UpsertSpeciesRole` schreibt beide Felder; `UpsertDerivedSpeciesRole` neu |
| `internal/adapters/sqlite/read.go` | `SpeciesRoles`/`SpeciesRolesByConcept` lesen beide Felder |
| `internal/adapters/sqlite/write_test.go`, `read_test.go` | Tests dafür |
| `internal/ports/output/repository.go` | `IngestTx.UpsertDerivedSpeciesRole` neu |
| `internal/application/species_ingest.go` | Kernumbau: Crosswalk-Dictionary statt Resolver, Aggregat-Ableitung |
| `internal/application/species_ingest_test.go` | Vollständig auf das neue Dateimodell umgestellt |
| `internal/application/arch_test.go` | **neu** — hält fest, dass `internal/application` `internal/adapters/hostus` nicht importiert |
| `internal/application/ingest_test.go` | `fakeRepo` um `UpsertDerivedSpeciesRole` erweitert |
| `cmd/situs/ingest.go` | Neue Flags `--crosswalk`/`--aggregate-members`, Resolver-Aufruf entfernt |
| `cmd/situs/ingest_test.go` | Wiring-Tests auf das neue Dateimodell umgestellt |
| `internal/ports/input/services.go` | `SpeciesEntry`/`HabitatTypeRole` um `Provenance`/`DerivedFrom`/`AggregateSource` erweitert |
| `internal/application/query.go` | `speciesEntry`/`SpeciesHabitatTypes` füllen die neuen Felder |
| `internal/adapters/http/openapi.yaml`, `api/openapi/openapi.yaml` | `SpeciesEntry`/`HabitatTypeRole`-Schema erweitert |
| `internal/adapters/http/handlers_test.go` | JSON-Test für `provenance`/`derived_from` |
| `docs/how-to/ingest.md` | Abschnitt „Namensauflösung" neu beschrieben, „Resolution Rate" korrigiert |

---

### Task 1: Domäne und sqlite-Adapter um `Provenance`/`DerivedFrom` erweitern

**Files:**
- Modify: `internal/domain/habitat.go`
- Modify: `internal/ports/output/repository.go`
- Modify: `internal/adapters/sqlite/schema.sql`
- Modify: `internal/adapters/sqlite/write.go`
- Modify: `internal/adapters/sqlite/read.go`
- Test: `internal/adapters/sqlite/write_test.go`
- Test: `internal/adapters/sqlite/read_test.go`

**Interfaces:**
- Produces: `domain.SpeciesRole{..., Provenance string, DerivedFrom *string}`.
  `output.IngestTx.UpsertDerivedSpeciesRole(r domain.SpeciesRole) (suppressed bool, err error)`
  — schreibt nur, wenn noch keine Zeile für `(typology_id, code, verbatim_name,
  role)` existiert; `suppressed == true`, wenn eine bereits existierte (dann
  bleibt die bestehende Zeile unverändert).
- Consumes: nichts aus späteren Tasks.

- [ ] **Step 1: Fehlschlagende Tests schreiben**

In `internal/adapters/sqlite/write_test.go`, neuer Test neben den
bestehenden `UpsertSpeciesRole`-Prüfungen:

```go
func TestIngestTx_UpsertSpeciesRoleRoundTripsProvenance(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	concept := "wcvp:concept:1"
	r := domain.SpeciesRole{
		Key: r22, ConceptID: &concept, VerbatimName: "Bromus erectus", Role: "diagnostic",
		Provenance: "observed",
	}
	if err := tx.UpsertSpeciesRole(r); err != nil {
		t.Fatalf("UpsertSpeciesRole: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.SpeciesRoles(ctx, r22, "")
	if err != nil {
		t.Fatalf("SpeciesRoles: %v", err)
	}
	if len(got) != 1 || got[0].Provenance != "observed" || got[0].DerivedFrom != nil {
		t.Errorf("SpeciesRoles = %+v, want one observed row with DerivedFrom nil", got)
	}
}

func TestIngestTx_UpsertDerivedSpeciesRole_WritesWhenAbsent(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	member := "wcvp:concept:2"
	aggID := "wcvp:concept:99"
	r := domain.SpeciesRole{
		Key: r22, ConceptID: &member, VerbatimName: "Rubus caesius", Role: "diagnostic",
		Provenance: "derived_from_aggregate", DerivedFrom: &aggID,
	}
	suppressed, err := tx.UpsertDerivedSpeciesRole(r)
	if err != nil {
		t.Fatalf("UpsertDerivedSpeciesRole: %v", err)
	}
	if suppressed {
		t.Error("suppressed = true, want false for a fresh row")
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.SpeciesRoles(ctx, r22, "")
	if err != nil {
		t.Fatalf("SpeciesRoles: %v", err)
	}
	if len(got) != 1 || got[0].Provenance != "derived_from_aggregate" {
		t.Fatalf("SpeciesRoles = %+v, want one derived_from_aggregate row", got)
	}
	if got[0].DerivedFrom == nil || *got[0].DerivedFrom != aggID {
		t.Errorf("DerivedFrom = %v, want a pointer to %q", got[0].DerivedFrom, aggID)
	}
}

func TestIngestTx_UpsertDerivedSpeciesRole_NeverOverwritesAnExplicitRow(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	explicitConcept := "wcvp:concept:3"
	seed := func() {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		explicit := domain.SpeciesRole{
			Key: r22, ConceptID: &explicitConcept, VerbatimName: "Rubus caesius", Role: "diagnostic",
			Provenance: "observed",
		}
		if err := tx.UpsertSpeciesRole(explicit); err != nil {
			t.Fatalf("UpsertSpeciesRole: %v", err)
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
	member := "wcvp:concept:2"
	aggID := "wcvp:concept:99"
	derived := domain.SpeciesRole{
		Key: r22, ConceptID: &member, VerbatimName: "Rubus caesius", Role: "diagnostic", // same (key, name, role)
		Provenance: "derived_from_aggregate", DerivedFrom: &aggID,
	}
	suppressed, err := tx.UpsertDerivedSpeciesRole(derived)
	if err != nil {
		t.Fatalf("UpsertDerivedSpeciesRole: %v", err)
	}
	if !suppressed {
		t.Error("suppressed = false, want true — an explicit row already exists for this key")
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.SpeciesRoles(ctx, r22, "")
	if err != nil {
		t.Fatalf("SpeciesRoles: %v", err)
	}
	if len(got) != 1 || got[0].Provenance != "observed" || got[0].ConceptID == nil || *got[0].ConceptID != explicitConcept {
		t.Errorf("SpeciesRoles = %+v, want the untouched explicit row to survive", got)
	}
}
```

In `internal/adapters/sqlite/read_test.go`, `seedReadFixture`s
`UpsertSpeciesRole`-Zeilen um `Provenance: "observed"` ergänzen (sonst wäre
die Fixture ab diesem Task inkonsistent mit dem neuen NOT-NULL-Feld — leer
bliebe technisch gültig, würde aber `"observed"` nicht wirklich zusichern),
und `TestSyntaxonAndSyntaxa`-Nachbartests unverändert lassen (die prüfen kein
`Provenance`).

Auch die geschlossene Fehlerfall-Tabelle in
`TestIngestTx_MethodsWrapErrorsOnAClosedTransaction`
(`internal/adapters/sqlite/write_test.go`) um den neuen Fall ergänzen:

```go
"UpsertDerivedSpeciesRole": func() error {
	_, err := tx.UpsertDerivedSpeciesRole(domain.SpeciesRole{Key: key, VerbatimName: "x", Role: "diagnostic", Provenance: "derived_from_aggregate"})
	return err
},
```

- [ ] **Step 2: Tests laufen lassen, erwartet: Compile-Fehler / rot**

```bash
go test ./internal/adapters/sqlite/... -run 'Provenance|Derived' -v
```

- [ ] **Step 3: Domäne, Port, Schema, Adapter implementieren**

`internal/domain/habitat.go`, `SpeciesRole` erweitern:

```go
// SpeciesRole is a species' role in a habitat type. VerbatimName is always
// set (also when Provenance is derived_from_aggregate — there it carries the
// MEMBER's name, taken from aggregate_members.csv, not the aggregate's
// name); ConceptID is nil when the name did not resolve via the crosswalk
// table.
type SpeciesRole struct {
	Key          HabitatTypeKey
	ConceptID    *string
	VerbatimName string
	Role         string // "diagnostic" | "constant" | "dominant"
	Fidelity     *float64
	Constancy    *float64
	// Provenance distinguishes an observed (CSV-sourced) role from one
	// derived from a collective-species (aggregate) row: "observed" |
	// "derived_from_aggregate". Never merged into one bucket — a caller must
	// be able to tell "this species was actually assessed for this habitat"
	// from "this species is a member of an assessed aggregate, but was
	// never itself assessed".
	Provenance string
	// DerivedFrom names the aggregate concept id this row was derived from.
	// nil when Provenance is "observed".
	DerivedFrom *string
}
```

`internal/ports/output/repository.go`, `IngestTx` erweitern (nach
`UpsertSpeciesRole`):

```go
	UpsertSpeciesRole(r domain.SpeciesRole) error
	// UpsertDerivedSpeciesRole inserts a species role derived from an
	// aggregate member, UNLESS an explicit row already exists for the same
	// (typology, code, verbatim_name, role) — an explicit row always wins,
	// regardless of write order within the same ingest run. suppressed is
	// true when an existing row blocked the insert; the existing row is left
	// untouched either way.
	UpsertDerivedSpeciesRole(r domain.SpeciesRole) (suppressed bool, err error)
```

`internal/adapters/sqlite/schema.sql`, `species_role`-Tabelle:

```sql
CREATE TABLE IF NOT EXISTS species_role (
  typology_id   TEXT NOT NULL,
  code          TEXT NOT NULL,
  concept_id    TEXT,
  verbatim_name TEXT NOT NULL,
  role          TEXT NOT NULL,
  fidelity      REAL,
  constancy     REAL,
  provenance    TEXT NOT NULL DEFAULT 'observed',
  derived_from  TEXT,
  PRIMARY KEY (typology_id, code, verbatim_name, role)
);
```

`internal/adapters/sqlite/write.go`, `UpsertSpeciesRole` ersetzen und
`UpsertDerivedSpeciesRole` ergänzen:

```go
func (t *ingestTx) UpsertSpeciesRole(r domain.SpeciesRole) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO species_role (typology_id, code, concept_id, verbatim_name, role, fidelity, constancy, provenance, derived_from)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(typology_id, code, verbatim_name, role) DO UPDATE SET
		   concept_id=excluded.concept_id, fidelity=excluded.fidelity, constancy=excluded.constancy,
		   provenance=excluded.provenance, derived_from=excluded.derived_from`,
		string(r.Key.Typology), r.Key.Code, r.ConceptID, r.VerbatimName, r.Role, r.Fidelity, r.Constancy,
		r.Provenance, r.DerivedFrom)
	if err != nil {
		return fmt.Errorf("sqlite: upserting species role %q in %s: %w", r.VerbatimName, r.Key, err)
	}
	return nil
}

// UpsertDerivedSpeciesRole inserts unless an explicit row already occupies
// the same natural key — the mechanism that makes "explicit always wins"
// independent of write order: a later explicit UpsertSpeciesRole call would
// still overwrite a derived row (DO UPDATE), but a derived call arriving
// after an explicit row is a silent DO NOTHING.
func (t *ingestTx) UpsertDerivedSpeciesRole(r domain.SpeciesRole) (bool, error) {
	res, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO species_role (typology_id, code, concept_id, verbatim_name, role, fidelity, constancy, provenance, derived_from)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(typology_id, code, verbatim_name, role) DO NOTHING`,
		string(r.Key.Typology), r.Key.Code, r.ConceptID, r.VerbatimName, r.Role, r.Fidelity, r.Constancy,
		r.Provenance, r.DerivedFrom)
	if err != nil {
		return false, fmt.Errorf("sqlite: upserting derived species role %q in %s: %w", r.VerbatimName, r.Key, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: reading rows affected for derived species role %q in %s: %w", r.VerbatimName, r.Key, err)
	}
	return n == 0, nil
}
```

`internal/adapters/sqlite/read.go`, `SpeciesRoles` und
`SpeciesRolesByConcept` erweitern (beide nutzen `scanSpeciesRole`, siehe
unten für die eine Stelle, die für beide reicht):

```go
func (d *DB) SpeciesRoles(ctx context.Context, key domain.HabitatTypeKey, role string) ([]domain.SpeciesRole, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT concept_id, verbatim_name, role, fidelity, constancy, provenance, derived_from FROM species_role
		 WHERE typology_id = ? AND code = ? AND (? = '' OR role = ?)
		 ORDER BY role, verbatim_name`,
		string(key.Typology), key.Code, role, role)
	// ... unchanged wrapping/loop, scanSpeciesRole below carries the new columns
}
```

`scanSpeciesRole` (used by `SpeciesRoles`) und die inline `Scan` in
`SpeciesRolesByConcept` beide erweitern:

```go
func scanSpeciesRole(rows *sql.Rows, r *domain.SpeciesRole) error {
	var concept, derivedFrom sql.NullString
	var fidelity, constancy sql.NullFloat64
	if err := rows.Scan(&concept, &r.VerbatimName, &r.Role, &fidelity, &constancy, &r.Provenance, &derivedFrom); err != nil {
		return err
	}
	applySpeciesNullables(r, concept, fidelity, constancy)
	if derivedFrom.Valid {
		v := derivedFrom.String
		r.DerivedFrom = &v
	}
	return nil
}
```

`SpeciesRolesByConcept`'s eigenes `SELECT`/`Scan` (es nutzt `scanSpeciesRole`
nicht, siehe der bestehende Code) entsprechend um `provenance, derived_from`
erweitern — Spaltenreihenfolge exakt wie oben, mit einem eigenen
`sql.NullString` für `derived_from` und direkter Zuweisung an `r.Provenance`
(nicht nullable, da `NOT NULL DEFAULT 'observed'`).

- [ ] **Step 4: Tests laufen lassen, erwartet: grün**

```bash
go test ./internal/adapters/sqlite/... -v
```

- [ ] **Step 5: Ganzes Modul bauen**

```bash
go build ./... && go vet ./...
```

`internal/application/ingest_test.go`s `fakeRepo` implementiert
`output.IngestTx` vollständig und braucht `UpsertDerivedSpeciesRole` —
dieser Build-Schritt zeigt das als Compile-Fehler; die Ergänzung ist Task 2
Step 1 (dort, weil Task 2 sie zuerst braucht — siehe dortige Begründung).
Für **diesen** Task genügt: bestätigen, dass `go build ./...` außerhalb von
`internal/application` sauber ist, und den `internal/application`-Fehler im
Report vermerken statt zu fixen.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/habitat.go internal/ports/output/repository.go \
  internal/adapters/sqlite/schema.sql internal/adapters/sqlite/write.go \
  internal/adapters/sqlite/read.go internal/adapters/sqlite/write_test.go \
  internal/adapters/sqlite/read_test.go
git commit -m "feat(domain): add SpeciesRole.Provenance/DerivedFrom and UpsertDerivedSpeciesRole"
```

---

### Task 2: `species_ingest.go` auf dateibasierte Crosswalk-Auflösung umbauen

**Files:**
- Modify: `internal/application/species_ingest.go`
- Modify: `internal/application/species_ingest_test.go`
- Modify: `internal/application/ingest_test.go` (nur `fakeRepo` erweitern)
- Create: `internal/application/arch_test.go`

**Interfaces:**
- Consumes: `output.IngestTx.UpsertDerivedSpeciesRole` (Task 1).
- Produces:
  `IngestSpeciesRoles(ctx, repo, csvPath, crosswalkPath, aggregateMembersPath string) (SpeciesReport, error)`
  (Resolver-Parameter entfernt), `SpeciesReport` erweitert um `DerivedRows`,
  `SuppressedByExplicit int` und `AmbiguousCrosswalk []string`.

- [ ] **Step 1: `fakeRepo` um `UpsertDerivedSpeciesRole` erweitern**

In `internal/application/ingest_test.go`, `fakeRepo`-Methode direkt nach
`UpsertSpeciesRole`:

```go
func (r *fakeRepo) UpsertDerivedSpeciesRole(role domain.SpeciesRole) (bool, error) {
	if err := r.failIfNamed("UpsertDerivedSpeciesRole"); err != nil {
		return false, err
	}
	for _, existing := range r.speciesRoles {
		if existing.Key == role.Key && existing.VerbatimName == role.VerbatimName && existing.Role == role.Role {
			return true, nil
		}
	}
	r.speciesRoles = append(r.speciesRoles, role)
	return false, nil
}
```

```bash
go build ./... && go test ./internal/application/... -v
```

Erwartet: grün — dieser Schritt fügt nur totes Zubehör hinzu, das die
nächsten Schritte benutzen.

- [ ] **Step 2: Fehlschlagende Tests für den neuen Ingest-Pfad schreiben**

`internal/application/species_ingest_test.go` wird komplett neu geschrieben
(die alten resolver-basierten Tests entfallen — es gibt keinen Resolver mehr
in diesem Pfad):

```go
package application

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func seedSpeciesRolesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Rubus fruticosus aggr.,diagnostic,0.8,\n"+
			"eunis@2021,R22,Nonexistent name,constant,,0.5\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv",
		"name,concept_id\n"+
			"Rubus fruticosus aggr.,wcvp:concept:99\n")
	writeCSV(t, dir, "aggregate_members.csv",
		"aggregate_concept_id,member_concept_id,member_name\n"+
			"wcvp:concept:99,wcvp:concept:1,Rubus caesius\n"+
			"wcvp:concept:99,wcvp:concept:2,Rubus plicatus\n")
	return dir
}

func speciesRolesPaths(dir string) (csvPath, crosswalkPath, aggPath string) {
	return filepath.Join(dir, "species_roles.csv"),
		filepath.Join(dir, "eurosl_crosswalk.csv"),
		filepath.Join(dir, "aggregate_members.csv")
}

func TestIngestSpeciesRoles_ResolvesViaLocalCrosswalkFile(t *testing.T) {
	dir := seedSpeciesRolesDir(t)
	csvPath, crosswalkPath, aggPath := speciesRolesPaths(dir)

	repo := newFakeRepo()
	rep, err := IngestSpeciesRoles(context.Background(), repo, csvPath, crosswalkPath, aggPath)
	if err != nil {
		t.Fatalf("IngestSpeciesRoles: %v", err)
	}
	if rep.Rows != 2 || rep.Resolved != 1 || rep.Unresolved != 1 {
		t.Errorf("report = %+v, want Rows 2 / Resolved 1 / Unresolved 1", rep)
	}
	var sawResolved bool
	for _, r := range repo.speciesRoles {
		if r.VerbatimName == "Rubus fruticosus aggr." {
			sawResolved = true
			if r.ConceptID == nil || *r.ConceptID != "wcvp:concept:99" {
				t.Errorf("ConceptID = %v, want a pointer to wcvp:concept:99", r.ConceptID)
			}
			if r.Provenance != "observed" {
				t.Errorf("Provenance = %q, want observed", r.Provenance)
			}
		}
	}
	if !sawResolved {
		t.Fatal("resolved row not found among stored roles")
	}
}

func TestIngestSpeciesRoles_DerivesMemberRowsFromAResolvedAggregate(t *testing.T) {
	dir := seedSpeciesRolesDir(t)
	csvPath, crosswalkPath, aggPath := speciesRolesPaths(dir)

	repo := newFakeRepo()
	rep, err := IngestSpeciesRoles(context.Background(), repo, csvPath, crosswalkPath, aggPath)
	if err != nil {
		t.Fatalf("IngestSpeciesRoles: %v", err)
	}
	if rep.DerivedRows != 2 {
		t.Errorf("DerivedRows = %d, want 2 (two members of the one resolved aggregate)", rep.DerivedRows)
	}
	byName := map[string]bool{}
	for _, r := range repo.speciesRoles {
		if r.Provenance != "derived_from_aggregate" {
			continue
		}
		byName[r.VerbatimName] = true
		if r.DerivedFrom == nil || *r.DerivedFrom != "wcvp:concept:99" {
			t.Errorf("row %q: DerivedFrom = %v, want a pointer to wcvp:concept:99", r.VerbatimName, r.DerivedFrom)
		}
		if r.Fidelity != nil || r.Constancy != nil {
			t.Errorf("row %q: Fidelity/Constancy = %v/%v, want both nil — never measured for the member itself",
				r.VerbatimName, r.Fidelity, r.Constancy)
		}
		if r.Role != "diagnostic" {
			t.Errorf("row %q: Role = %q, want the aggregate row's own role (diagnostic)", r.VerbatimName, r.Role)
		}
	}
	if !byName["Rubus caesius"] || !byName["Rubus plicatus"] {
		t.Errorf("stored derived rows = %+v, want both Rubus caesius and Rubus plicatus", byName)
	}
}

func TestIngestSpeciesRoles_ExplicitMemberRowWinsOverDerivation(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Rubus fruticosus aggr.,diagnostic,0.8,\n"+
			// The member is ALSO explicitly assessed in its own right, with its
			// own fidelity — this must win, regardless of row order.
			"eunis@2021,R22,Rubus caesius,diagnostic,0.95,\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv",
		"name,concept_id\n"+
			"Rubus fruticosus aggr.,wcvp:concept:99\n"+
			"Rubus caesius,wcvp:concept:1\n")
	writeCSV(t, dir, "aggregate_members.csv",
		"aggregate_concept_id,member_concept_id,member_name\n"+
			"wcvp:concept:99,wcvp:concept:1,Rubus caesius\n")
	csvPath, crosswalkPath, aggPath := speciesRolesPaths(dir)

	repo := newFakeRepo()
	rep, err := IngestSpeciesRoles(context.Background(), repo, csvPath, crosswalkPath, aggPath)
	if err != nil {
		t.Fatalf("IngestSpeciesRoles: %v", err)
	}
	if rep.SuppressedByExplicit != 1 {
		t.Errorf("SuppressedByExplicit = %d, want 1", rep.SuppressedByExplicit)
	}
	if rep.DerivedRows != 0 {
		t.Errorf("DerivedRows = %d, want 0 — the only member row was suppressed, not written", rep.DerivedRows)
	}
	var caesiusRows int
	for _, r := range repo.speciesRoles {
		if r.VerbatimName != "Rubus caesius" {
			continue
		}
		caesiusRows++
		if r.Provenance != "observed" || r.Fidelity == nil || *r.Fidelity != 0.95 {
			t.Errorf("Rubus caesius row = %+v, want the explicit observed row (Fidelity 0.95) to survive untouched", r)
		}
	}
	if caesiusRows != 1 {
		t.Fatalf("Rubus caesius rows = %d, want exactly 1 (no duplicate from the suppressed derivation)", caesiusRows)
	}
}

func TestIngestSpeciesRoles_AmbiguousCrosswalkNameStaysUnresolvedAndIsReported(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Salsola kali,diagnostic,0.8,\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv",
		"name,concept_id\n"+
			"Salsola kali,wcvp:concept:1\n"+
			"Salsola kali,wcvp:concept:2\n") // two different concepts for the same name
	writeCSV(t, dir, "aggregate_members.csv", "aggregate_concept_id,member_concept_id,member_name\n")
	csvPath, crosswalkPath, aggPath := speciesRolesPaths(dir)

	repo := newFakeRepo()
	rep, err := IngestSpeciesRoles(context.Background(), repo, csvPath, crosswalkPath, aggPath)
	if err != nil {
		t.Fatalf("IngestSpeciesRoles: %v", err)
	}
	if len(rep.AmbiguousCrosswalk) != 1 || rep.AmbiguousCrosswalk[0] != "Salsola kali" {
		t.Errorf("AmbiguousCrosswalk = %+v, want [Salsola kali]", rep.AmbiguousCrosswalk)
	}
	if rep.Resolved != 0 || rep.Unresolved != 1 {
		t.Errorf("Resolved/Unresolved = %d/%d, want 0/1 — an ambiguous name is never guessed", rep.Resolved, rep.Unresolved)
	}
	if len(repo.speciesRoles) != 1 || repo.speciesRoles[0].ConceptID != nil {
		t.Errorf("stored row = %+v, want it kept with ConceptID nil", repo.speciesRoles)
	}
}

func TestIngestSpeciesRoles_MissingAggregateMembersFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Rubus fruticosus aggr.,diagnostic,0.8,\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv", "name,concept_id\nRubus fruticosus aggr.,wcvp:concept:99\n")
	csvPath, crosswalkPath, _ := speciesRolesPaths(dir)
	missingAggPath := filepath.Join(dir, "aggregate_members.csv") // never written

	repo := newFakeRepo()
	rep, err := IngestSpeciesRoles(context.Background(), repo, csvPath, crosswalkPath, missingAggPath)
	if err != nil {
		t.Fatalf("IngestSpeciesRoles with a missing aggregate_members.csv: %v", err)
	}
	if rep.DerivedRows != 0 || rep.Resolved != 1 {
		t.Errorf("report = %+v, want the aggregate row itself still resolved, DerivedRows 0", rep)
	}
}

// A missing crosswalk file is a hard error — without it nothing can resolve
// at all, the same severity the old live-resolver failure had.
func TestIngestSpeciesRoles_MissingCrosswalkFileFails(t *testing.T) {
	dir := seedSpeciesRolesDir(t)
	csvPath, _, aggPath := speciesRolesPaths(dir)
	missingCrosswalkPath := filepath.Join(dir, "does-not-exist.csv")

	repo := newFakeRepo()
	if _, err := IngestSpeciesRoles(context.Background(), repo, csvPath, missingCrosswalkPath, aggPath); err == nil {
		t.Fatal("IngestSpeciesRoles with a missing crosswalk file = nil error, want an error")
	}
	if repo.committed {
		t.Error("ingest committed despite a missing crosswalk file")
	}
}

func TestIngestSpeciesRoles_MissingSpeciesRolesFileFails(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "eurosl_crosswalk.csv", "name,concept_id\n")
	csvPath, crosswalkPath, aggPath := speciesRolesPaths(dir)

	repo := newFakeRepo()
	if _, err := IngestSpeciesRoles(context.Background(), repo, csvPath, crosswalkPath, aggPath); err == nil {
		t.Fatal("IngestSpeciesRoles with a missing species_roles.csv = nil error, want an error")
	}
}

func TestIngestSpeciesRoles_SkipsMalformedRows(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Inula hirta,diagnostic,0.8,\n"+
			"not valid @@ id,R22,Bad typology,diagnostic,0.8,\n"+
			"eunis@2021,R22,Bad fidelity,diagnostic,not-a-number,\n"+
			"eunis@2021,R22,Bad constancy,constant,,not-a-number\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv", "name,concept_id\n")
	writeCSV(t, dir, "aggregate_members.csv", "aggregate_concept_id,member_concept_id,member_name\n")
	csvPath, crosswalkPath, aggPath := speciesRolesPaths(dir)

	repo := newFakeRepo()
	rep, err := IngestSpeciesRoles(context.Background(), repo, csvPath, crosswalkPath, aggPath)
	if err != nil {
		t.Fatalf("IngestSpeciesRoles: %v", err)
	}
	if rep.Rows != 1 {
		t.Errorf("Rows = %d, want 1 (three malformed rows skipped)", rep.Rows)
	}
	if rep.Skipped != 3 {
		t.Errorf("Skipped = %d, want 3", rep.Skipped)
	}
}

func TestIngestSpeciesRoles_BeginErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.beginErr = fmt.Errorf("boom")
	dir := seedSpeciesRolesDir(t)
	csvPath, crosswalkPath, aggPath := speciesRolesPaths(dir)

	if _, err := IngestSpeciesRoles(context.Background(), repo, csvPath, crosswalkPath, aggPath); err == nil {
		t.Fatal("IngestSpeciesRoles with a Begin error = nil error, want an error")
	}
}

func TestIngestSpeciesRoles_RepositoryErrorRollsBackAndReturnsTheError(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertSpeciesRole"
	dir := seedSpeciesRolesDir(t)
	csvPath, crosswalkPath, aggPath := speciesRolesPaths(dir)

	if _, err := IngestSpeciesRoles(context.Background(), repo, csvPath, crosswalkPath, aggPath); err == nil {
		t.Fatal("IngestSpeciesRoles with a repository error = nil error, want an error")
	}
	if repo.committed {
		t.Error("ingest committed despite a repository error")
	}
	if !repo.rolledBack {
		t.Error("ingest did not roll back after a repository error")
	}
}

func TestIngestSpeciesRoles_RollbackErrorIsWrappedWithTheOriginal(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertSpeciesRole"
	repo.rollbackErr = fmt.Errorf("connection lost")
	dir := seedSpeciesRolesDir(t)
	csvPath, crosswalkPath, aggPath := speciesRolesPaths(dir)

	_, err := IngestSpeciesRoles(context.Background(), repo, csvPath, crosswalkPath, aggPath)
	if err == nil {
		t.Fatal("IngestSpeciesRoles = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "connection lost") {
		t.Errorf("error = %q, want it to mention the rollback failure too", err)
	}
}

func TestIngestSpeciesRoles_CommitErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.commitErr = fmt.Errorf("disk full")
	dir := seedSpeciesRolesDir(t)
	csvPath, crosswalkPath, aggPath := speciesRolesPaths(dir)

	if _, err := IngestSpeciesRoles(context.Background(), repo, csvPath, crosswalkPath, aggPath); err == nil {
		t.Fatal("IngestSpeciesRoles with a Commit error = nil error, want an error")
	}
}

func TestSpeciesReport_ResolutionRateOfNoRowsIsZero(t *testing.T) {
	if got := (SpeciesReport{}).ResolutionRate(); got != 0 {
		t.Errorf("ResolutionRate() of an empty report = %v, want 0", got)
	}
}
```

Der bisherige `strPtr`-Helfer, `fakeResolver` und `erroringResolver` entfallen
komplett (es gibt keinen Resolver mehr) — `strPtr` nur dann behalten, wenn
ein anderer Test in diesem Package ihn noch nutzt (mit
`grep -rn "strPtr(" internal/application/*_test.go` vor dem Löschen
prüfen).

- [ ] **Step 3: Tests laufen lassen, erwartet: Compile-Fehler**

```bash
go test ./internal/application/... -run TestIngestSpeciesRoles -v
```

- [ ] **Step 4: `species_ingest.go` implementieren**

Ersetzt die bisherige Datei komplett:

```go
package application

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// SpeciesReport summarizes one species-role ingest run. Skipped mirrors
// IngestReport.SkippedRows.
type SpeciesReport struct {
	Rows       int
	Resolved   int
	Unresolved int
	Skipped    int
	// DerivedRows is how many additional member rows an aggregate match
	// produced. SuppressedByExplicit is how many of those were NOT written
	// because an explicit row already covered the same (key, name, role).
	DerivedRows          int
	SuppressedByExplicit int
	// AmbiguousCrosswalk lists (sorted, deduplicated) the verbatim names that
	// carried more than one distinct concept id in eurosl_crosswalk.csv — a
	// data-quality finding, never guessed at, so such a row's ConceptID stays
	// nil.
	AmbiguousCrosswalk []string
}

// ResolutionRate is the fraction of rows whose verbatim name resolved,
// row-weighted (not distinct-name-weighted) — see docs/how-to/ingest.md.
func (r SpeciesReport) ResolutionRate() float64 {
	if r.Rows == 0 {
		return 0
	}
	return float64(r.Resolved) / float64(r.Rows)
}

// The two provenance values a stored SpeciesRole carries. Unexported package
// constants, same pattern as localize.go's provenanceOfficial etc.
const (
	speciesProvenanceObserved             = "observed"
	speciesProvenanceDerivedFromAggregate = "derived_from_aggregate"
)

// loadCrosswalk reads eurosl_crosswalk.csv (name,concept_id) into a
// name -> []concept_id multimap: a name with more than one distinct concept
// id is a data-quality finding the caller must detect, not something this
// reader collapses. A missing file is a hard error — without a crosswalk NO
// row can resolve, the same severity a live resolver outage had.
func loadCrosswalk(csvPath string) (map[string][]string, int, error) {
	dir, file := filepath.Split(csvPath)
	out := map[string][]string{}
	skipped := 0
	skip := newRowSkipper(&skipped, file, "crosswalk entry")
	err := readAll(context.Background(), dir, file, []string{"name", "concept_id"}, skip,
		func(idx map[string]int, row []string, _ int) error {
			name := row[idx["name"]]
			id := row[idx["concept_id"]]
			for _, existing := range out[name] {
				if existing == id {
					return nil // exact duplicate row, not a new candidate
				}
			}
			out[name] = append(out[name], id)
			return nil
		})
	if err != nil {
		return nil, 0, fmt.Errorf("loading crosswalk %s: %w", csvPath, err)
	}
	return out, skipped, nil
}

// aggregateMember is one row of aggregate_members.csv.
type aggregateMember struct {
	memberConceptID, memberName string
}

// loadAggregateMembers reads aggregate_members.csv
// (aggregate_concept_id,member_concept_id,member_name) into a
// aggregate_concept_id -> []aggregateMember map. A missing file is NOT an
// error — the ingest runs without member-derivation, the aggregate rows
// themselves are still stored correctly.
func loadAggregateMembers(csvPath string) (map[string][]aggregateMember, int, error) {
	if _, err := os.Stat(csvPath); err != nil {
		if os.IsNotExist(err) {
			slog.Info("no aggregate members file, skipping member derivation", "path", csvPath)
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("statting %s: %w", csvPath, err)
	}
	dir, file := filepath.Split(csvPath)
	out := map[string][]aggregateMember{}
	skipped := 0
	skip := newRowSkipper(&skipped, file, "aggregate member")
	err := readAll(context.Background(), dir, file,
		[]string{"aggregate_concept_id", "member_concept_id", "member_name"}, skip,
		func(idx map[string]int, row []string, _ int) error {
			aggID := row[idx["aggregate_concept_id"]]
			out[aggID] = append(out[aggID], aggregateMember{
				memberConceptID: row[idx["member_concept_id"]],
				memberName:      row[idx["member_name"]],
			})
			return nil
		})
	if err != nil {
		return nil, 0, fmt.Errorf("loading aggregate members %s: %w", csvPath, err)
	}
	return out, skipped, nil
}

// resolveRow looks up r.verbatim in crosswalk. Exactly one candidate
// resolves it; zero means unresolved; two or more is ambiguous and is never
// guessed at (nil concept id, name reported to ambiguous).
func resolveRow(verbatim string, crosswalk map[string][]string) (conceptID string, ambiguous bool) {
	candidates := crosswalk[verbatim]
	if len(candidates) == 1 {
		return candidates[0], false
	}
	if len(candidates) > 1 {
		return "", true
	}
	return "", false
}

// upsertSpeciesRows writes every explicit (CSV-sourced) row via tx, then —
// for every row whose resolved concept is itself a known aggregate — derives
// one additional row per member. The two passes are sequential ON PURPOSE:
// every explicit row is written before any derivation is attempted, so an
// explicit row for the SAME member elsewhere in species_roles.csv always
// exists (and therefore always wins) by the time UpsertDerivedSpeciesRole
// runs for it — independent of the two files' row order.
func upsertSpeciesRows(tx output.IngestTx, rows []speciesRow, crosswalk map[string][]string,
	aggregates map[string][]aggregateMember) (SpeciesReport, error) {
	rep := SpeciesReport{Rows: len(rows)}
	ambiguousSeen := map[string]bool{}
	resolvedConcepts := make([]string, len(rows))

	for i, r := range rows {
		conceptID, ambiguous := resolveRow(r.verbatim, crosswalk)
		var conceptPtr *string
		switch {
		case ambiguous:
			if !ambiguousSeen[r.verbatim] {
				ambiguousSeen[r.verbatim] = true
				rep.AmbiguousCrosswalk = append(rep.AmbiguousCrosswalk, r.verbatim)
			}
			rep.Unresolved++
		case conceptID != "":
			conceptPtr = &conceptID
			resolvedConcepts[i] = conceptID
			rep.Resolved++
		default:
			rep.Unresolved++
		}
		sr := domain.SpeciesRole{
			Key: r.key, ConceptID: conceptPtr, VerbatimName: r.verbatim, Role: r.role,
			Fidelity: r.fidelity, Constancy: r.constancy, Provenance: speciesProvenanceObserved,
		}
		if err := tx.UpsertSpeciesRole(sr); err != nil {
			return SpeciesReport{}, err
		}
	}
	sort.Strings(rep.AmbiguousCrosswalk)

	for i, r := range rows {
		aggID := resolvedConcepts[i]
		if aggID == "" {
			continue
		}
		members, ok := aggregates[aggID]
		if !ok {
			continue
		}
		aggIDCopy := aggID
		for _, m := range members {
			memberID := m.memberConceptID
			derived := domain.SpeciesRole{
				Key: r.key, ConceptID: &memberID, VerbatimName: m.memberName, Role: r.role,
				Provenance: speciesProvenanceDerivedFromAggregate, DerivedFrom: &aggIDCopy,
			}
			suppressed, err := tx.UpsertDerivedSpeciesRole(derived)
			if err != nil {
				return SpeciesReport{}, err
			}
			if suppressed {
				rep.SuppressedByExplicit++
			} else {
				rep.DerivedRows++
			}
		}
	}
	return rep, nil
}

// IngestSpeciesRoles loads csvPath (species_roles.csv) into repo, resolving
// every verbatim name against a LOCAL, deterministic dictionary
// (crosswalkPath: name,concept_id) — no network call, no fuzzy/homonym
// logic. For every row whose resolved concept is itself a collective species
// (aggregate), it additionally derives one row per member listed in
// aggregateMembersPath — unless an explicit row already covers that member,
// which always wins.
//
// A missing crosswalkPath aborts the ingest (nothing could resolve without
// it — the same severity the earlier live-resolver outage had). A missing
// aggregateMembersPath does not: the ingest runs without member-derivation.
func IngestSpeciesRoles(ctx context.Context, repo output.Repository,
	csvPath, crosswalkPath, aggregateMembersPath string) (SpeciesReport, error) {
	dir, file := filepath.Split(csvPath)

	skipped := 0
	rows, err := readSpeciesRows(ctx, dir, file, newRowSkipper(&skipped, file, "species role"))
	if err != nil {
		return SpeciesReport{}, err
	}

	crosswalk, crosswalkSkipped, err := loadCrosswalk(crosswalkPath)
	if err != nil {
		return SpeciesReport{}, err
	}
	aggregates, aggSkipped, err := loadAggregateMembers(aggregateMembersPath)
	if err != nil {
		return SpeciesReport{}, err
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return SpeciesReport{}, fmt.Errorf("beginning species-role ingest transaction: %w", err)
	}

	rep, err := upsertSpeciesRows(tx, rows, crosswalk, aggregates)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return SpeciesReport{}, fmt.Errorf("%w (rollback also failed: %w)", err, rbErr)
		}
		return SpeciesReport{}, err
	}
	rep.Skipped = skipped + crosswalkSkipped + aggSkipped

	if err := tx.Commit(); err != nil {
		return SpeciesReport{}, fmt.Errorf("committing species-role ingest transaction: %w", err)
	}
	return rep, nil
}
```

`readSpeciesRows`/`speciesRow` bleiben **unverändert** in `ingest.go` (Task
1's Nachbarcode, siehe `internal/application/ingest.go`) — dieser Task
importiert sie nur weiter, ändert nichts an ihnen.

- [ ] **Step 5: Tests laufen lassen, erwartet: grün**

```bash
go test ./internal/application/... -v
```

- [ ] **Step 6: Architektur-Test schreiben**

Neue Datei `internal/application/arch_test.go` (mirrors
`internal/app/arch_test.go`'s exact rationale, one package earlier in the
call chain than the runtime-serve guard):

```go
package application_test

import (
	"go/build"
	"strings"
	"testing"
)

// species_ingest.go used to take an output.NameResolver whose only
// implementation lived in internal/adapters/hostus; it now resolves entirely
// from local files. Nothing in internal/application should import the hostus
// adapter — a stray import here would silently reintroduce the live
// dependency this task removed.
func TestApplicationDoesNotImportTheHostusAdapter(t *testing.T) {
	pkg, err := build.Import("github.com/jobrunner/situs/internal/application", "", 0)
	if err != nil {
		t.Fatalf("importing internal/application: %v", err)
	}
	if len(pkg.Imports) == 0 {
		t.Fatal("build.Import reported no imports for internal/application — the assertion below would be vacuous")
	}
	for _, imp := range pkg.Imports {
		if strings.Contains(imp, "internal/adapters/hostus") {
			t.Errorf("internal/application imports %q — species ingest must stay free of a live hostus dependency", imp)
		}
	}
}
```

```bash
go test ./internal/application/... -run TestApplicationDoesNotImportTheHostusAdapter -v
```

- [ ] **Step 7: Ganzen Build/Test-Lauf prüfen**

```bash
go build ./... && go vet ./...
```

Erwartet: weiterhin Compile-Fehler in `cmd/situs/ingest.go` (ruft
`IngestSpeciesRoles` noch mit der alten Signatur auf) — das ist Task 3,
hier nur bestätigen und im Report vermerken.

- [ ] **Step 8: Commit**

```bash
git add internal/application/species_ingest.go internal/application/species_ingest_test.go \
  internal/application/ingest_test.go internal/application/arch_test.go
git commit -m "feat(application): resolve species roles from a local crosswalk file, derive aggregate members"
```

---

### Task 3: In `situs ingest` verdrahten

**Files:**
- Modify: `cmd/situs/ingest.go`
- Modify: `cmd/situs/ingest_test.go`

**Interfaces:**
- Consumes: `application.IngestSpeciesRoles` mit der neuen Signatur (Task 2).
- Produces: zwei neue Flags `--crosswalk`/`--aggregate-members`, `ingestOutput`
  mit den drei neuen `Species`-Report-Feldern (implizit, da `SpeciesReport`
  selbst schon erweitert ist — `cmd/situs/ingest.go` muss dafür nichts extra
  tun, nur die Aufrufstelle anpassen).

- [ ] **Step 1: Fehlschlagende Tests schreiben**

In `cmd/situs/ingest_test.go`, `seedIngestDir` um die zwei neuen Dateien
ergänzen (sonst schlägt **jeder** bestehende Test dieser Datei fehl, weil
`eurosl_crosswalk.csv` jetzt Pflicht ist):

```go
func seedIngestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeIngestCSV(t, dir, "typologies.csv",
		"id,scheme,version,name,source_ref\neunis@2021,eunis,2021,EUNIS 2021,https://example.org\n")
	writeIngestCSV(t, dir, "habitat_types.csv",
		"typology_id,code,level,name_en,parent_code,priority\neunis@2021,R22,3,Hay meadow,R2,\n")
	writeIngestCSV(t, dir, "crosswalks.csv",
		"from_typology,from_code,to_typology,to_code,qualifier\n")
	writeIngestCSV(t, dir, "syntaxa.csv", "id,rank,name,parent_id\n")
	writeIngestCSV(t, dir, "habitat_type_syntaxa.csv", "typology_id,code,syntaxon_id\n")
	writeIngestCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\neunis@2021,R22,Inula hirta,diagnostic,0.8,\n")
	writeIngestCSV(t, dir, "eurosl_crosswalk.csv",
		"name,concept_id\nInula hirta,wcvp:concept:1\n")
	writeIngestCSV(t, dir, "aggregate_members.csv",
		"aggregate_concept_id,member_concept_id,member_name\n")
	return dir
}
```

(`stubHostus` bleibt unverändert und wird weiterhin gebraucht — für
`IngestDistribution`, nicht mehr für die Namensauflösung.)

Neuer Test:

```go
func TestIngestCommandFailsOnAMissingCrosswalkFile(t *testing.T) {
	stubHostus(t)
	dir := seedIngestDir(t)
	if err := os.Remove(filepath.Join(dir, "eurosl_crosswalk.csv")); err != nil {
		t.Fatalf("removing eurosl_crosswalk.csv: %v", err)
	}
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"ingest", "--csv-dir", dir, "--db", filepath.Join(t.TempDir(), "situs.sqlite")})
	if err := root.Execute(); err == nil {
		t.Fatal("executing ingest with a missing eurosl_crosswalk.csv = nil error, want an error")
	}
}

func TestIngestCommandCrosswalkFlagOverridesTheDefaultPath(t *testing.T) {
	stubHostus(t)
	dir := seedIngestDir(t)
	// Move the crosswalk file out of csv-dir entirely — only the flag finds it.
	elsewhere := filepath.Join(t.TempDir(), "custom-crosswalk.csv")
	if err := os.Rename(filepath.Join(dir, "eurosl_crosswalk.csv"), elsewhere); err != nil {
		t.Fatalf("moving crosswalk file: %v", err)
	}
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"ingest", "--csv-dir", dir, "--db", filepath.Join(t.TempDir(), "situs.sqlite"),
		"--crosswalk", elsewhere})
	if err := root.Execute(); err != nil {
		t.Fatalf("executing ingest with --crosswalk: %v", err)
	}
	if !strings.Contains(out.String(), `"Resolved": 1`) {
		t.Errorf("output = %q, want the row resolved via the flagged crosswalk path", out.String())
	}
}
```

- [ ] **Step 2: Test laufen lassen, erwartet: rot**

```bash
go test ./cmd/situs/... -run 'TestIngestCommand.*Crosswalk' -v
```

- [ ] **Step 3: `cmd/situs/ingest.go` verdrahten**

`newIngestCmd()` um zwei Flags erweitern:

```go
func newIngestCmd() *cobra.Command {
	var csvDir, dbPath, crosswalkPath, aggregateMembersPath string

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Typologien, Habitattypen, Crosswalks, Syntaxa und Artenrollen aus CSVs laden",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if csvDir == "" {
				return fmt.Errorf("--csv-dir is required")
			}
			cfg, err := config.Load(configFile)
			if err != nil {
				return err
			}
			if dbPath == "" {
				dbPath = cfg.Index.Path
			}
			if dbPath == "" {
				return fmt.Errorf("no index path: pass --db or set index.path (SITUS_INDEX_PATH)")
			}
			if crosswalkPath == "" {
				crosswalkPath = filepath.Join(csvDir, "eurosl_crosswalk.csv")
			}
			if aggregateMembersPath == "" {
				aggregateMembersPath = filepath.Join(csvDir, "aggregate_members.csv")
			}
			return runIngest(cmd, cfg, csvDir, dbPath, crosswalkPath, aggregateMembersPath)
		},
	}
	cmd.Flags().StringVar(&csvDir, "csv-dir", "", "directory holding the pipeline CSVs (required)")
	cmd.Flags().StringVar(&dbPath, "db", "",
		"path to the sqlite index file (default: index.path / SITUS_INDEX_PATH)")
	cmd.Flags().StringVar(&crosswalkPath, "crosswalk", "",
		"path to eurosl_crosswalk.csv (default: <csv-dir>/eurosl_crosswalk.csv)")
	cmd.Flags().StringVar(&aggregateMembersPath, "aggregate-members", "",
		"path to aggregate_members.csv (default: <csv-dir>/aggregate_members.csv)")
	return cmd
}
```

`runIngest`'s Signatur und der `IngestSpeciesRoles`-Aufruf:

```go
func runIngest(cmd *cobra.Command, cfg *config.Config, csvDir, dbPath, crosswalkPath, aggregateMembersPath string) error {
	ctx := cmd.Context()

	installLogger(cfg.Logging, os.Stdout)

	db, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("opening sqlite index %q: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	report, err := application.IngestCSV(ctx, db, csvDir)
	if err != nil {
		return fmt.Errorf("ingesting %q: %w", csvDir, err)
	}

	speciesReport, err := application.IngestSpeciesRoles(ctx, db,
		filepath.Join(csvDir, "species_roles.csv"), crosswalkPath, aggregateMembersPath)
	if err != nil {
		return fmt.Errorf("ingesting species roles from %q: %w", csvDir, err)
	}

	// resolver stays needed for the distribution step below — Areas(), not
	// Resolve(); species-name resolution no longer calls hostus at all.
	resolver := hostus.NewClient(cfg.Hostus.BaseURL, &http.Client{Timeout: cfg.Hostus.Timeout}, cfg.Hostus.BatchSize, cfg.Hostus.EntryBackbone)
	distSrc := &pacedDistributionSource{src: resolver, pause: hostusDistributionPause}
	distributionReport, err := application.IngestDistribution(ctx, db, distSrc)
	if err != nil {
		return fmt.Errorf("ingesting species distribution: %w", err)
	}

	// ... everything from here on (localizations, DeriveGermanLabels, info,
	// WarnOnForeignBackbones, ingestOutput{...}) stays EXACTLY as it is today —
	// unaffected by this task.
}
```

Die restlichen Zeilen von `runIngest` (`IngestLocalizations`,
`DeriveGermanLabels`, `NewQueryService(db).IndexInfo`,
`WarnOnForeignBackbones`, die `ingestOutput{...}`-Konstruktion, `enc.Encode`)
**unverändert übernehmen** — nur die Reihenfolge `resolver := ...` /
`IngestSpeciesRoles(...)` hat sich vertauscht (Resolver wird jetzt erst nach
der Namensauflösung gebraucht, für die Verbreitung).

- [ ] **Step 4: Tests laufen lassen, erwartet: grün**

```bash
go test ./cmd/situs/... -v
```

Jeder bestehende Test in `cmd/situs/ingest_test.go`, der `seedIngestDir`
nutzt, muss weiterhin grün sein — das war der Zweck, `seedIngestDir` in
Step 1 um die zwei neuen Dateien zu ergänzen, statt einen Parallel-Helper zu
bauen.

- [ ] **Step 5: `make verify`**

```bash
make verify
```

- [ ] **Step 6: Commit**

```bash
git add cmd/situs/ingest.go cmd/situs/ingest_test.go
git commit -m "feat(cmd): wire the crosswalk/aggregate-members flags into situs ingest"
```

---

### Task 4: Read-API — `provenance`/`derived_from`

**Files:**
- Modify: `internal/ports/input/services.go`
- Modify: `internal/application/query.go`
- Modify: `internal/adapters/http/openapi.yaml`
- Modify: `api/openapi/openapi.yaml`
- Modify: `internal/adapters/http/handlers_test.go`

**Interfaces:**
- Consumes: `domain.SpeciesRole.Provenance`/`DerivedFrom` (Task 1).
- Produces: `input.SpeciesEntry`/`input.HabitatTypeRole` mit
  `Provenance string` (`omitempty`, nur bei `derived_from_aggregate` gesetzt)
  und `DerivedFrom *input.AggregateSource` (`omitempty`).

- [ ] **Step 1: Fehlschlagenden HTTP-Test schreiben**

In `internal/adapters/http/handlers_test.go`, neuer Test neben
`TestHabitatType_SpeciesCarriesEveryRoleBucket`:

```go
func TestHabitatTypeSpecies_ProvenanceAndDerivedFromAreOmittedForObservedRows(t *testing.T) {
	q := seededQueryService()
	detail := q.types["eunis@2021:R22"]
	detail.Species[input.RoleDiagnostic] = []input.SpeciesEntry{
		{ConceptID: "wcvp-1", VerbatimName: "Bromus erectus", Role: input.RoleDiagnostic},
		{
			ConceptID: "wcvp-2", VerbatimName: "Rubus caesius", Role: input.RoleDiagnostic,
			Provenance: "derived_from_aggregate",
			DerivedFrom: &input.AggregateSource{ConceptID: "wcvp-99", Name: "Rubus fruticosus aggr."},
		},
	}
	q.types["eunis@2021:R22"] = detail
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/R22", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, `"verbatim_name":"Bromus erectus","role":"diagnostic","fidelity"`) {
		// (only reachable if fidelity/constancy happen to be set — the real
		// assertion is the negative one below; keep both branches honest.)
	}
	if !strings.Contains(body, `"concept_id":"wcvp-2","verbatim_name":"Rubus caesius","role":"diagnostic","provenance":"derived_from_aggregate","derived_from":{"concept_id":"wcvp-99","name":"Rubus fruticosus aggr."}`) {
		t.Errorf("body = %s, want the derived entry's provenance/derived_from present in this exact shape", body)
	}
	var got struct {
		Species map[string][]struct {
			VerbatimName string `json:"verbatim_name"`
			Provenance   string `json:"provenance"`
		} `json:"species"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	for _, e := range got.Species[input.RoleDiagnostic] {
		if e.VerbatimName == "Bromus erectus" && e.Provenance != "" {
			t.Errorf("observed entry Provenance = %q, want empty (omitted)", e.Provenance)
		}
	}
}
```

Vor dem Implementieren prüfen, ob die exakte Feldreihenfolge im erwarteten
Teilstring zur tatsächlichen `input.SpeciesEntry`-Struct-Definition aus
Step 3 passt (`concept_id, verbatim_name, role, fidelity, constancy,
in_area, provenance, derived_from` — Reihenfolge unten festgelegt); bei
Abweichung den Teilstring anpassen, nicht die Feldreihenfolge nachträglich
verbiegen.

- [ ] **Step 2: Test laufen lassen, erwartet: rot**

```bash
go test ./internal/adapters/http/... -run TestHabitatTypeSpecies_Provenance -v
```

- [ ] **Step 3: DTOs und `query.go` erweitern**

`internal/ports/input/services.go`:

```go
// AggregateSource lets a caller ask "what aggregate produced this match?".
type AggregateSource struct {
	ConceptID string `json:"concept_id"`
	Name      string `json:"name"`
}

// SpeciesEntry is one species in its role. VerbatimName is always set;
// ConceptID is absent when the name did not resolve.
type SpeciesEntry struct {
	ConceptID    string   `json:"concept_id,omitempty"`
	VerbatimName string   `json:"verbatim_name"`
	Role         string   `json:"role"`
	Fidelity     *float64 `json:"fidelity,omitempty"`
	Constancy    *float64 `json:"constancy,omitempty"`
	InArea       *bool    `json:"in_area,omitempty"`
	// Provenance is "observed" (default, omitted) or "derived_from_aggregate"
	// — present only in the derived case, so the common (observed) response
	// shape is unchanged for existing clients.
	Provenance  string           `json:"provenance,omitempty"`
	DerivedFrom *AggregateSource `json:"derived_from,omitempty"`
}
```

`HabitatTypeRole` identisch um dieselben zwei Felder erweitern (gleiche
JSON-Tags, gleiche Semantik — ein Species-Habitat-Treffer, der über eine
Aggregat-Ableitung zustande kam, muss das genauso zeigen können wie die
Artenliste).

`internal/application/query.go`, `speciesEntry` erweitern:

```go
func speciesEntry(r domain.SpeciesRole) input.SpeciesEntry {
	e := input.SpeciesEntry{
		VerbatimName: r.VerbatimName,
		Role:         r.Role,
		Fidelity:     r.Fidelity,
		Constancy:    r.Constancy,
	}
	if r.ConceptID != nil {
		e.ConceptID = *r.ConceptID
	}
	if r.Provenance == "derived_from_aggregate" {
		e.Provenance = r.Provenance
		if r.DerivedFrom != nil {
			e.DerivedFrom = &input.AggregateSource{ConceptID: *r.DerivedFrom}
		}
	}
	return e
}
```

`AggregateSource.Name` bleibt hier absichtlich leer — der Domäne fehlt der
Name der Aggregat-Art selbst an dieser Stelle (`domain.SpeciesRole` trägt nur
die `DerivedFrom`-Concept-ID, nicht deren Namen). Diesen Namen aufzulösen
bräuchte einen zusätzlichen Repository-Lookup pro derived-Zeile — außerhalb
des Scopes dieses Plans, als **Offener Punkt** unten vermerkt. Den Test aus
Step 1 entsprechend anpassen: `Name` im erwarteten JSON-Teilstring weglassen
bzw. `""` erwarten, **nicht** den befüllten Beispielwert aus Step 1 blind
übernehmen (der stammt aus der stub-`fakeQueryService`-Fixture, die den Namen
frei erfinden darf, weil sie keinen echten Repository-Lookup macht — die
echte `query.go`-Implementierung darf das nicht).

`SpeciesHabitatTypes` (in `query.go`) baut `input.HabitatTypeRole` direkt,
nicht über `speciesEntry` — dort dieselbe Provenance/DerivedFrom-Übernahme
inline ergänzen, an der Stelle, wo `out = append(out, input.HabitatTypeRole{...})`
steht.

`internal/adapters/http/openapi.yaml` **und** `api/openapi/openapi.yaml`
(beide identisch), `SpeciesEntry`-Schema erweitern:

```yaml
    AggregateSource:
      type: object
      required: [concept_id, name]
      properties:
        concept_id:
          type: string
        name:
          type: string
    SpeciesEntry:
      description: >-
        verbatim_name ist immer gesetzt; concept_id fehlt, wenn der Name nicht
        auf ein Konzept auflösbar war — solche Zeilen werden behalten.
        provenance/derived_from erscheinen nur bei einer aus einer Sammelart
        abgeleiteten Zeile.
      type: object
      required: [verbatim_name, role]
      properties:
        concept_id:
          type: string
        verbatim_name:
          type: string
        role:
          $ref: "#/components/schemas/Role"
        fidelity:
          type: number
        constancy:
          type: number
        in_area:
          $ref: "#/components/schemas/InArea"
        provenance:
          type: string
          enum: [derived_from_aggregate]
        derived_from:
          $ref: "#/components/schemas/AggregateSource"
```

`HabitatTypeRole`-Schema um dieselben zwei Properties (`provenance`,
`derived_from`) erweitern, gleiche Beschreibung.

- [ ] **Step 4: Tests laufen lassen, erwartet: grün**

```bash
go test ./internal/adapters/http/... -v
go test ./internal/application/... -v
```

- [ ] **Step 5: OpenAPI-Kontrakttest**

```bash
go test ./internal/adapters/http/... -run TestOpenAPI -v
go test ./internal/adapters/http/... -run TestRoutesMatchOpenAPISpec -v
```

- [ ] **Step 6: `make verify`**

```bash
make verify
```

- [ ] **Step 7: Commit**

```bash
git add internal/ports/input/services.go internal/application/query.go \
  internal/adapters/http/openapi.yaml api/openapi/openapi.yaml \
  internal/adapters/http/handlers_test.go
git commit -m "feat(http): expose SpeciesEntry/HabitatTypeRole provenance/derived_from"
```

---

### Task 5: Dokumentation nachziehen

**Files:**
- Modify: `docs/how-to/ingest.md`

**Interfaces:** keine — reine Doku.

- [ ] **Step 1: Abschnitt „Getrennte Transaktionen" korrigieren**

`docs/how-to/ingest.md` Zeile 49 (`2. IngestSpeciesRoles — Artenrollen,
inklusive hostus-Namensauflösung.`) ersetzen durch:

```markdown
2. `IngestSpeciesRoles` — Artenrollen, aufgelöst gegen eine lokale
   Crosswalk-Datei (`eurosl_crosswalk.csv`), plus abgeleitete
   Mitgliedsarten-Zeilen für Sammelarten (`aggregate_members.csv`). Kein
   hostus-Aufruf mehr in diesem Schritt.
```

Zeile 53-54 (die den hostus-Ausfall dieses Schritts beschreiben) ersetzen:

```markdown
Fehlt `eurosl_crosswalk.csv`, bricht `ingest` beim zweiten Schritt mit einem
Fehler ab — aber der **erste** Schritt ist zu diesem Zeitpunkt bereits
committed (Typologien/Habitattypen/Crosswalks/Syntaxa, aber keine
Artenrollen). Das ist kein Datenverlust: jeder `Upsert*` ist idempotent, ein
erneuter Lauf holt den fehlenden Schritt nach.
```

- [ ] **Step 2: Abschnitt „Die gemeldete Resolution Rate" neu fassen**

Den ganzen Abschnitt (Zeilen 62-84) ersetzen:

```markdown
## Namensauflösung und Mitgliedsarten-Ableitung

`IngestSpeciesRoles` löst jeden `verbatim_name` gegen ein lokales,
deterministisches Wörterbuch auf — `eurosl_crosswalk.csv`
(`name,concept_id`), von hostus vorab exportiert, kein Netzwerkaufruf, keine
Fuzzy-/Homonym-Logik. Ein Name mit mehr als einer Concept-ID in dieser Datei
ist ein Datenfund, kein Rateanlass: er bleibt `concept_id: null`, wird
gezählt (`AmbiguousCrosswalk`) und geloggt.

Für jede Zeile, deren aufgelöste Concept-ID selbst eine in
`aggregate_members.csv` gelistete Sammelart ist, schreibt der Ingest je
Mitgliedsart eine zusätzliche Zeile mit `provenance: derived_from_aggregate`
— außer eine explizite `species_roles.csv`-Zeile für dasselbe Mitglied
existiert bereits; die gewinnt immer. Eine abgeleitete Zeile trägt nie
`fidelity`/`constancy` (nie für das Mitglied selbst gemessen).

`ResolutionRate()` bleibt **zeilengewichtet**: `Resolved / Rows`, über alle
Zeilen von `species_roles.csv`.

Die zwei neuen Report-Felder:

| Feld | Bedeutung |
|---|---|
| `DerivedRows` | zusätzlich geschriebene Mitgliedsarten-Zeilen |
| `SuppressedByExplicit` | abgeleitete Zeilen, die wegen einer expliziten CSV-Zeile NICHT geschrieben wurden |
| `AmbiguousCrosswalk` | Namen mit mehr als einer Concept-ID in `eurosl_crosswalk.csv` |

**Noch nicht messbar:** `eurosl_crosswalk.csv`/`aggregate_members.csv`
kommen aus einem hostus-seitigen Export, der zum Zeitpunkt dieser Zeilen noch
nicht existiert (siehe `docs/superpowers/specs/
2026-08-29-situs-aggregat-mitgliedsarten-design.md`). Bis dahin: `Resolved`,
`DerivedRows`, `SuppressedByExplicit` und `AmbiguousCrosswalk` sind an keinem
echten Datenstand gemessen — genau wie `Localizations`/`DerivedLabels` in
`../reference/measured-index.md`, solange die passende Quelle fehlt.

Nicht aufgelöste Namen werden **nicht verworfen**: `verbatim_name` ist immer
gesetzt, `concept_id` bleibt NULL.
```

- [ ] **Step 3: `--crosswalk`/`--aggregate-members` im Kopf-Beispiel erwähnen**

Nach dem bestehenden Codeblock (Zeilen 3-6) einen Satz ergänzen:

```markdown
`--crosswalk`/`--aggregate-members` sind optional und fallen auf
`<csv-dir>/eurosl_crosswalk.csv` bzw. `<csv-dir>/aggregate_members.csv`
zurück — nur bei abweichender Ablage nötig.
```

- [ ] **Step 4: Doc-Drift-Gate**

```bash
find . -iname "*doc-drift*" -not -path "*/node_modules/*"
```

Den gefundenen Pfad ausführen (z. B. `./scripts/check-doc-drift.sh`) — muss
grün bleiben.

- [ ] **Step 5: `make verify`**

```bash
make verify
```

- [ ] **Step 6: Commit**

```bash
git add docs/how-to/ingest.md
git commit -m "docs(how-to): describe the crosswalk-file species resolution and aggregate derivation"
```

---

## Offene Punkte (bewusst nicht in diesem Plan gelöst)

- **`AggregateSource.Name` bleibt leer.** `domain.SpeciesRole.DerivedFrom`
  trägt nur die Aggregat-Concept-ID, nicht deren Namen; einen Namen
  aufzulösen bräuchte einen zusätzlichen Lookup pro derived-Zeile. Eigener,
  kleiner Folge-Task, falls ein Client den Namen tatsächlich braucht (die
  Concept-ID allein reicht, um bei Bedarf selbst nachzuschlagen).
- **`output.NameResolver`/`hostus.Client.Resolve` bleiben toter Code.** Siehe
  „Bewusste Abweichung von der Spec" oben — Löschung ist ein eigener,
  bewusst gescopter Aufräum-Task.
- **hostus-seitiger Export** (`eurosl_crosswalk.csv`, `aggregate_members.csv`)
  ist ein eigenes Spec/eigene Implementierung im hostus-Repo — Voraussetzung
  für eine echte Messung, aber außerhalb dieses Plans.
- **Keine Abhängigkeit zum Syntaxa-Hierarchie-Plan** (`docs/superpowers/plans/
  2026-08-30-situs-syntaxa-hierarchie.md`): beide Pläne ändern
  `cmd/situs/ingest.go` (unterschiedliche Stellen: dort wird ein zusätzlicher
  Ingest-Schritt zwischen `IngestCSV` und `IngestSpeciesRoles` eingefügt,
  hier ändert sich nur `IngestSpeciesRoles`s Aufruf selbst) und
  `internal/ports/input/services.go`/beide `openapi.yaml`-Kopien
  (unterschiedliche Typen: dort `SyntaxonRef`, hier `SpeciesEntry`/
  `HabitatTypeRole`). Wer beide Pläne nacheinander auf denselben Branch
  ausführt, bekommt in `ingest.go` und `openapi.yaml` reale, aber triviale
  Merge-Overlaps (Nachbarzeilen, keine widersprüchliche Logik) — kein
  Blocker, nur beim Zusammenführen beachten.

## Self-Review (bereits durchgeführt)

- **Spec-Abdeckung:** Architektur/Datenfluss → Task 2+3; Domäne
  (`Provenance`/`DerivedFrom`) → Task 1; alle vier Fehlerbehandlungs-Fälle
  der Spec-Tabelle → Task 2 Tests (Name fehlt, Name mehrdeutig,
  `aggregate_members.csv` fehlt, Mitgliedsart bereits explizit vorhanden);
  alle vier „Prüfbare Zusagen" → je ein konkreter Test in Task 1/2 (Author-
  Analogie: hier Fidelity/Constancy-nil-Test, explizit-gewinnt-Test,
  Architektur-Test, Rückwärtskompatibilitäts-Test in Task 4); Read-API → Task
  4; Doku → Task 5. Abweichung von der Spec explizit benannt (NameResolver
  bleibt bestehen) statt stillschweigend zu ergänzen oder zu löschen.
- **Platzhalter-Scan:** keine offenen Code-Stellen — jeder Testkörper ist
  vollständig ausformuliert; die einzige verbleibende Unschärfe
  (`AggregateSource.Name` leer) ist eine benannte Design-Entscheidung mit
  Begründung, kein TODO.
- **Typkonsistenz:** `domain.SpeciesRole{...,Provenance,DerivedFrom}` (Task 1)
  → `output.IngestTx.UpsertDerivedSpeciesRole` (Task 1) →
  `application.SpeciesReport{...,DerivedRows,SuppressedByExplicit,
  AmbiguousCrosswalk}` (Task 2) → `input.SpeciesEntry`/`HabitatTypeRole`
  (Task 4) — Feldnamen und die beiden Provenance-String-Werte
  (`"observed"`/`"derived_from_aggregate"`) sind über alle Tasks identisch.

## Execution Handoff

Plan gespeichert unter
`docs/superpowers/plans/2026-08-29-situs-aggregat-mitgliedsarten.md`. Zwei
Ausführungsoptionen:

1. **Subagent-getrieben (empfohlen)** — frischer Subagent pro Task, Review
   zwischen den Tasks, schnelle Iteration.
2. **Inline-Ausführung** — Tasks in dieser Sitzung mit Checkpoints zur Review.

Welcher Ansatz?
