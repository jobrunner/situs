# Autarke Laufzeit und Verbreitungsfilter — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** situs braucht hostus zur Laufzeit nicht mehr, und die Artenlisten
tragen je Gebiet die Information, ob eine Art dort überhaupt vorkommt.

**Architecture:** Der Laufzeitpfad verliert eine Abhängigkeit *und* eine
Abstraktionsebene: der Verbatim-Namens-Batch wird zu einem Konzept-ID-Batch auf
dem bestehenden `QueryService`, und `NameQueryService` samt Port entfällt. Die
Verbreitung kommt über einen neuen getriebenen Port `DistributionSource` beim
**Ingest** in eine schmale Tabelle; die Leseseite markiert und filtert daraus.

**Tech Stack:** Go 1.26, `gorilla/mux`, `modernc.org/sqlite`, `spf13/cobra` +
`viper`. Keine neue Bibliothek.

**Spec:** `docs/superpowers/specs/2026-08-20-autarke-laufzeit-und-verbreitung-design.md`

## Global Constraints

- **Ubiquitous Language:** ein Habitattyp wird immer über `(typology, code)`
  adressiert. Niemals eine Tabelle, ein Typ oder eine Route nur `habitat`.
- **Fehlende Daten sind Abwesenheit von Zeilen**, nie ein Platzhalter-Code.
- **Nichts wird still verworfen.** `null` bei `in_area` bleibt in jeder Antwort
  enthalten, auch mit `only_in_area=true`.
- **SQL sind statische Strings mit `?`-Platzhaltern** — niemals Werte
  konkatenieren (gosec G201/G202 brechen den Build).
- **`internal/domain`** importiert nichts; **`internal/application`** keinen
  Adapter; **`internal/adapters/http`** weder den sqlite-Adapter noch
  `internal/application`. Depguard erzwingt das über `make arch`.
- **OpenAPI in zwei byte-identischen Kopien**
  (`internal/adapters/http/openapi.yaml`, `api/openapi/openapi.yaml`); der
  Contract-Test prüft Routen↔Spec in **beide** Richtungen und lässt keine Route
  ohne `.Methods()` durch.
- **Fehlerumschlag** `{"error":{"code","message"}}` mit `INVALID_QUERY`,
  `NOT_FOUND`, `INTERNAL_ERROR`. `UPSTREAM_UNAVAILABLE` **entfällt**.
- Null `//nolint`, null `#nosec`, keine `TODO`/`FIXME`/`HACK`/`XXX`-Marker.
- `.coverage-floors` ist ein **Nur-Anheben**-Ratchet (`make debt`, Teil von
  `make verify`). Niemals einen Floor senken.
- Kommentare sparsam, englisch, und nur wo sie ein *warum* erklären.
- TDD: den fehlschlagenden Test schreiben, ihn fehlschlagen sehen, dann
  implementieren.
- Feature-Branch `feature/autark-runtime`, Conventional Commits. `VERSION` und
  `CHANGELOG.md` gehören release-please — nie von Hand ändern.

---

## File Structure

| Pfad | Verantwortung |
|---|---|
| `internal/domain/area.go` | **neu** — `Area` als Wertobjekt (Schema + Code) |
| `internal/ports/output/repository.go` | `DistributionSource` ergänzen; `IngestTx` um `UpsertDistribution`; `Repository` um `AreasForConcepts` und `KnownAreaCodes` |
| `internal/ports/input/services.go` | `NameResolution` → `ConceptResolution`; `SpeciesNameQueryService` entfernen; `QueryService` um `SpeciesSetHabitatTypes`; `SpeciesEntry`/`HabitatTypeRole` um `InArea` |
| `internal/adapters/sqlite/schema.sql` | Tabelle `species_distribution` + Index |
| `internal/adapters/sqlite/write.go` | `UpsertDistribution` |
| `internal/adapters/sqlite/read.go` | `AreasForConcepts`, `KnownAreaCodes` |
| `internal/adapters/hostus/distribution.go` | **neu** — `Areas()` über `GET /v1/concept/{id}` |
| `internal/application/query.go` | `NameQueryService` entfernen; `SpeciesSetHabitatTypes`; `in_area`-Anreicherung |
| `internal/application/distribution_ingest.go` | **neu** — `IngestDistribution` |
| `internal/adapters/http/species.go` | Batch auf `concept_ids`; `area`/`only_in_area` parsen |
| `internal/adapters/http/habitat.go` | `area`/`only_in_area` auf den Artenlisten; `UPSTREAM_UNAVAILABLE`-Zweig entfernen |
| `internal/adapters/http/server.go` | `Deps.Names` entfernen; `CodeUpstreamUnavailable` entfernen |
| `internal/app/app.go` | hostus-Verdrahtung aus dem Serve-Pfad entfernen |
| `internal/app/arch_test.go` | **neu** — belegt, dass `internal/app` den hostus-Adapter nicht importiert |
| `cmd/situs/ingest.go` | `IngestDistribution` einhängen, Report erweitern |

`internal/application/query.go` ist heute 320 Zeilen; die Anreicherung kommt
hinzu, `NameQueryService` fällt weg. Bleibt unter 400 — kein Split nötig.

---

### Task 1: `Area` im Domain und die Ports

Legt die Namen fest, die alle folgenden Tasks benutzen. Kein I/O.

**Files:**
- Create: `internal/domain/area.go`, `internal/domain/area_test.go`
- Modify: `internal/ports/output/repository.go`

**Interfaces:**
- Consumes: nichts.
- Produces:
  - `domain.Area{Scheme, Code string}` mit `func (a Area) String() string` → `"wgsrpd_l3:GER"`
  - `domain.SchemeWGSRPDL3 = "wgsrpd_l3"`
  - `output.DistributionSource` mit `Areas(ctx, conceptIDs []string) (map[string][]domain.Area, error)`
  - `output.IngestTx` um `UpsertDistribution(conceptID string, a domain.Area) error`
  - `output.Repository` um
    `AreasForConcepts(ctx, conceptIDs []string, scheme string) (map[string][]string, error)`
    und `KnownAreaCodes(ctx, scheme string) ([]string, error)`

- [ ] **Step 1: Den fehlschlagenden Test schreiben**

Create `internal/domain/area_test.go`:

```go
package domain

import "testing"

func TestAreaString(t *testing.T) {
	a := Area{Scheme: SchemeWGSRPDL3, Code: "GER"}
	if got, want := a.String(), "wgsrpd_l3:GER"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// An area without a scheme is not addressable — the same code means different
// places in different schemes, exactly like a habitat type code.
func TestAreaIsComplete(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    Area
		want bool
	}{
		{name: "both set", a: Area{Scheme: SchemeWGSRPDL3, Code: "GER"}, want: true},
		{name: "no code", a: Area{Scheme: SchemeWGSRPDL3}, want: false},
		{name: "no scheme", a: Area{Code: "GER"}, want: false},
		{name: "empty", a: Area{}, want: false},
	} {
		if got := tc.a.IsComplete(); got != tc.want {
			t.Errorf("%s: IsComplete() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Test laufen lassen und fehlschlagen sehen**

Run: `go test ./internal/domain/ -run Area -v`
Expected: FAIL — `undefined: Area`, `undefined: SchemeWGSRPDL3`.

- [ ] **Step 3: Minimal implementieren**

Create `internal/domain/area.go`:

```go
package domain

// SchemeWGSRPDL3 is the only area scheme situs stores today: WGSRPD level 3
// ("botanical countries"), which is what hostus reports per concept.
const SchemeWGSRPDL3 = "wgsrpd_l3"

// Area is a distribution area. Scheme and Code together identify it — a bare
// code is ambiguous across schemes, the same way a habitat type code is
// ambiguous across typologies.
type Area struct {
	Scheme string
	Code   string
}

func (a Area) String() string { return a.Scheme + ":" + a.Code }

// IsComplete reports whether both halves are present. An incomplete area must
// never reach the index: it would silently match nothing.
func (a Area) IsComplete() bool { return a.Scheme != "" && a.Code != "" }
```

- [ ] **Step 4: Test laufen lassen und grün sehen**

Run: `go test ./internal/domain/ -run Area -v`
Expected: PASS.

- [ ] **Step 5: Die Ports erweitern**

In `internal/ports/output/repository.go`, `IngestTx` um diese Methode ergänzen
(neben `UpsertSpeciesRole`):

```go
	// UpsertDistribution records that a concept occurs in an area. Idempotent:
	// a repinned artifact is simply re-ingested.
	UpsertDistribution(conceptID string, a domain.Area) error
```

`Repository` um diese beiden ergänzen:

```go
	// AreasForConcepts maps each concept id to the area codes it occurs in,
	// within one scheme. A concept absent from the result has no distribution
	// data at all — that is "unknown", not "does not occur".
	AreasForConcepts(ctx context.Context, conceptIDs []string, scheme string) (map[string][]string, error)

	// KnownAreaCodes lists the area codes the index has data for. An area
	// filter must be validated against this: an unknown code has to be an
	// error, not a list of "does not occur".
	KnownAreaCodes(ctx context.Context, scheme string) ([]string, error)
```

Und den neuen getriebenen Port am Ende der Datei:

```go
// DistributionSource yields the areas a concept occurs in. Separate from
// NameResolver on purpose: different question (concept -> areas, not name ->
// concept) and different failure semantics — a distribution outage must not
// abort an ingest, an unresolvable name path must.
type DistributionSource interface {
	Areas(ctx context.Context, conceptIDs []string) (map[string][]domain.Area, error)
}
```

- [ ] **Step 6: Bauen — die Fakes werden noch nicht kompilieren**

Run: `go build ./... && go vet ./internal/ports/...`
Expected: `go build` PASS. `go test ./...` schlägt jetzt fehl, weil `fakeRepo`
in `internal/application` die neuen Methoden nicht hat — das ist erwartet und
wird in Task 2 behoben. Nicht hier reparieren.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/area.go internal/domain/area_test.go internal/ports/output/repository.go
git commit -m "feat(domain): area value object and the distribution ports"
```

---

### Task 2: Schema, Schreib- und Leseseite im sqlite-Adapter

**Files:**
- Modify: `internal/adapters/sqlite/schema.sql`, `write.go`, `read.go`
- Modify: `internal/adapters/sqlite/write_test.go` (neue Tests anhängen)
- Modify: `internal/application/ingest_test.go` (`fakeRepo` um die drei neuen
  Methoden ergänzen, damit das Paket wieder kompiliert)

**Interfaces:**
- Consumes: `domain.Area`, `output.IngestTx.UpsertDistribution`,
  `output.Repository.AreasForConcepts`/`KnownAreaCodes` (Task 1).
- Produces: dieselben Methoden, im sqlite-Adapter implementiert.

- [ ] **Step 1: Das Schema erweitern**

An `internal/adapters/sqlite/schema.sql` anhängen:

```sql
-- Which areas a species concept occurs in. Absence of rows for a concept means
-- "unknown", not "does not occur" — the read side must keep those apart.
CREATE TABLE IF NOT EXISTS species_distribution (
  concept_id  TEXT NOT NULL,
  area_scheme TEXT NOT NULL,
  area_code   TEXT NOT NULL,
  PRIMARY KEY (concept_id, area_scheme, area_code)
);
CREATE INDEX IF NOT EXISTS idx_species_distribution_area
  ON species_distribution(area_scheme, area_code);
```

- [ ] **Step 2: Die fehlschlagenden Tests schreiben**

An `internal/adapters/sqlite/write_test.go` anhängen:

```go
func TestIngestTx_UpsertDistributionIsIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	a := domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}

	for i := range 2 {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Begin %d: %v", i, err)
		}
		if err := tx.UpsertDistribution("wcvp:concept:1", a); err != nil {
			t.Fatalf("UpsertDistribution %d: %v", i, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit %d: %v", i, err)
		}
	}

	got, err := db.AreasForConcepts(ctx, []string{"wcvp:concept:1"}, domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("AreasForConcepts: %v", err)
	}
	if len(got["wcvp:concept:1"]) != 1 {
		t.Errorf("areas = %v, want exactly one (upsert, not insert)", got["wcvp:concept:1"])
	}
}

// A concept with no rows must be ABSENT from the map, not present-and-empty:
// the read side turns absence into "unknown" and an empty list would become
// "does not occur here", which is a different and wrong statement.
func TestAreasForConcepts_ConceptWithoutDataIsAbsent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertDistribution("wcvp:concept:1", domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}); err != nil {
		t.Fatalf("UpsertDistribution: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.AreasForConcepts(ctx, []string{"wcvp:concept:1", "wcvp:concept:2"}, domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("AreasForConcepts: %v", err)
	}
	if _, ok := got["wcvp:concept:2"]; ok {
		t.Error("a concept without distribution rows must be absent from the map, not empty-valued")
	}
}

func TestAreasForConcepts_EmptyInputNeedsNoQuery(t *testing.T) {
	db := openTestDB(t)
	got, err := db.AreasForConcepts(context.Background(), nil, domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("AreasForConcepts: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want an empty map", got)
	}
}

func TestKnownAreaCodes_ListsWhatTheIndexHas(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, code := range []string{"GER", "FRA", "GER"} {
		if err := tx.UpsertDistribution("wcvp:concept:1", domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: code}); err != nil {
			t.Fatalf("UpsertDistribution %s: %v", code, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.KnownAreaCodes(ctx, domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("KnownAreaCodes: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("codes = %v, want two distinct codes", got)
	}
}
```

- [ ] **Step 3: Tests laufen lassen und fehlschlagen sehen**

Run: `go test ./internal/adapters/sqlite/ -run 'Distribution|AreasFor|KnownArea' -v`
Expected: FAIL — `db.AreasForConcepts undefined`, `tx.UpsertDistribution undefined`.

- [ ] **Step 4: Die Schreibseite implementieren**

In `internal/adapters/sqlite/write.go`, im Stil der bestehenden Upserts:

```go
func (t *ingestTx) UpsertDistribution(conceptID string, a domain.Area) error {
	_, err := t.tx.Exec(
		`INSERT INTO species_distribution (concept_id, area_scheme, area_code)
		 VALUES (?, ?, ?)
		 ON CONFLICT(concept_id, area_scheme, area_code) DO NOTHING`,
		conceptID, a.Scheme, a.Code)
	if err != nil {
		return fmt.Errorf("sqlite: upserting distribution %s for %s: %w", a, conceptID, err)
	}
	return nil
}
```

- [ ] **Step 5: Die Leseseite implementieren**

In `internal/adapters/sqlite/read.go`. Die `IN`-Liste braucht so viele
Platzhalter wie IDs — das ist der einzige zulässige Weg, weil nur `?` erzeugt
wird, niemals ein Wert:

```go
func (db *DB) AreasForConcepts(ctx context.Context, conceptIDs []string, scheme string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(conceptIDs) == 0 {
		return out, nil
	}
	// Only placeholders are generated here, never values — the ids stay
	// arguments, so this is not SQL construction from input (gosec G201/G202).
	placeholders := strings.Repeat(",?", len(conceptIDs))[1:]
	args := make([]any, 0, len(conceptIDs)+1)
	for _, id := range conceptIDs {
		args = append(args, id)
	}
	args = append(args, scheme)

	rows, err := db.QueryContext(ctx,
		`SELECT concept_id, area_code FROM species_distribution
		 WHERE concept_id IN (`+placeholders+`) AND area_scheme = ?
		 ORDER BY concept_id, area_code`, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading distribution for %d concepts: %w", len(conceptIDs), err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id, code string
		if err := rows.Scan(&id, &code); err != nil {
			return nil, fmt.Errorf("sqlite: scanning distribution: %w", err)
		}
		out[id] = append(out[id], code)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating distribution: %w", err)
	}
	return out, nil
}

func (db *DB) KnownAreaCodes(ctx context.Context, scheme string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT area_code FROM species_distribution
		 WHERE area_scheme = ? ORDER BY area_code`, scheme)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading area codes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []string{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("sqlite: scanning area code: %w", err)
		}
		out = append(out, code)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating area codes: %w", err)
	}
	return out, nil
}
```

`strings` in die Imports von `read.go` aufnehmen, falls noch nicht vorhanden.

- [ ] **Step 6: Den Fake im application-Paket nachziehen**

`internal/application/ingest_test.go`: `fakeRepo` erfüllt `output.Repository`
und `output.IngestTx` und braucht die drei neuen Methoden, sonst kompiliert das
Paket nicht. Ergänzen:

```go
func (r *fakeRepo) UpsertDistribution(conceptID string, a domain.Area) error {
	if r.failOn == "UpsertDistribution" {
		return errors.New("boom")
	}
	r.distribution = append(r.distribution, fakeDistribution{ConceptID: conceptID, Area: a})
	return nil
}

func (r *fakeRepo) AreasForConcepts(_ context.Context, conceptIDs []string, scheme string) (map[string][]string, error) {
	if r.areasErr != nil {
		return nil, r.areasErr
	}
	out := map[string][]string{}
	for _, id := range conceptIDs {
		for _, d := range r.distribution {
			if d.ConceptID == id && d.Area.Scheme == scheme {
				out[id] = append(out[id], d.Area.Code)
			}
		}
	}
	return out, nil
}

func (r *fakeRepo) KnownAreaCodes(_ context.Context, scheme string) ([]string, error) {
	if r.areasErr != nil {
		return nil, r.areasErr
	}
	seen := map[string]bool{}
	out := []string{}
	for _, d := range r.distribution {
		if d.Area.Scheme == scheme && !seen[d.Area.Code] {
			seen[d.Area.Code] = true
			out = append(out, d.Area.Code)
		}
	}
	return out, nil
}
```

Und die Felder am `fakeRepo`-Struct sowie der Hilfstyp:

```go
type fakeDistribution struct {
	ConceptID string
	Area      domain.Area
}
```

plus `distribution []fakeDistribution` und `areasErr error` in `fakeRepo`.

- [ ] **Step 7: Tests laufen lassen und grün sehen**

Run: `go test ./internal/adapters/sqlite/ ./internal/application/ -count=1`
Expected: PASS in beiden Paketen.

- [ ] **Step 8: `make verify` und Commit**

Run: `make verify`
Expected: PASS. Falls ein Coverage-Floor greift, den betroffenen Floor
**anheben**, niemals senken.

```bash
git add internal/adapters/sqlite internal/application/ingest_test.go .coverage-floors
git commit -m "feat(sqlite): species distribution table with idempotent writes and area reads"
```

---

### Task 3: Der hostus-Adapter liefert Verbreitung

**Files:**
- Create: `internal/adapters/hostus/distribution.go`, `distribution_test.go`

**Interfaces:**
- Consumes: `output.DistributionSource`, `domain.Area` (Task 1).
- Produces: `(*Client).Areas(ctx, conceptIDs []string) (map[string][]domain.Area, error)`
  — derselbe `Client`, der schon `Resolve` hat.

- [ ] **Step 1: Den fehlschlagenden Test schreiben**

Create `internal/adapters/hostus/distribution_test.go`:

```go
package hostus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func TestClient_AreasReadsOneConceptPerRequest(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.HasPrefix(r.URL.Path, "/v1/concept/") {
			t.Errorf("path = %q, want /v1/concept/{id}", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"concept_id":"x","distribution":[
			{"area_scheme":"wgsrpd_l3","area_code":"GER"},
			{"area_scheme":"wgsrpd_l3","area_code":"FRA"}]}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client(), 50, "wcvp").
		Areas(context.Background(), []string{"wcvp:concept:1", "wcvp:concept:2"})
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("requests = %d, want 2 (hostus has no batch route for distribution)", calls.Load())
	}
	if len(got["wcvp:concept:1"]) != 2 {
		t.Errorf("areas = %v, want two", got["wcvp:concept:1"])
	}
	if got["wcvp:concept:1"][0] != (domain.Area{Scheme: "wgsrpd_l3", Code: "GER"}) {
		t.Errorf("first area = %v, want wgsrpd_l3:GER", got["wcvp:concept:1"][0])
	}
}

// A concept hostus does not know is not an error: it simply has no areas. The
// caller distinguishes "no data" from "does not occur" by absence.
func TestClient_AreasSkipsUnknownConcepts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client(), 50, "wcvp").
		Areas(context.Background(), []string{"wcvp:concept:404"})
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want an empty map", got)
	}
}

func TestClient_AreasReportsAnUnavailableUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client(), 50, "wcvp").
		Areas(context.Background(), []string{"wcvp:concept:1"}); err == nil {
		t.Error("Areas returned nil error on 503; the caller must be able to see the outage")
	}
}

func TestClient_AreasIgnoresOtherSchemes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"distribution":[
			{"area_scheme":"tdwg_l4","area_code":"GER-OO"},
			{"area_scheme":"wgsrpd_l3","area_code":"GER"}]}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client(), 50, "wcvp").
		Areas(context.Background(), []string{"wcvp:concept:1"})
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if len(got["wcvp:concept:1"]) != 1 {
		t.Errorf("areas = %v, want only the wgsrpd_l3 one", got["wcvp:concept:1"])
	}
}
```

- [ ] **Step 2: Test laufen lassen und fehlschlagen sehen**

Run: `go test ./internal/adapters/hostus/ -run Areas -v`
Expected: FAIL — `c.Areas undefined`.

- [ ] **Step 3: Implementieren**

Create `internal/adapters/hostus/distribution.go`:

```go
package hostus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// conceptResponse is the slice of GET /v1/concept/{id} this adapter needs.
type conceptResponse struct {
	Distribution []struct {
		AreaScheme string `json:"area_scheme"`
		AreaCode   string `json:"area_code"`
	} `json:"distribution"`
}

// Areas asks hostus once per concept: there is no batch route for distribution
// (/v1/match carries none), so the request count equals the concept count. The
// caller paces this — hostus rate-limits.
func (c *Client) Areas(ctx context.Context, conceptIDs []string) (map[string][]domain.Area, error) {
	out := map[string][]domain.Area{}
	for _, id := range conceptIDs {
		areas, err := c.areasOf(ctx, id)
		if err != nil {
			return nil, err
		}
		if len(areas) > 0 {
			out[id] = areas
		}
	}
	return out, nil
}

func (c *Client) areasOf(ctx context.Context, conceptID string) ([]domain.Area, error) {
	endpoint := c.baseURL + "/v1/concept/" + url.PathEscape(conceptID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("building the hostus concept request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling hostus %s: %w: %w", endpoint, output.ErrResolverUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// A concept hostus does not know has no areas — that is data, not failure.
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, errBodyLimit))
		return nil, fmt.Errorf("hostus answered %s for %s: %w: %s",
			resp.Status, conceptID, output.ErrResolverUnavailable, snippet)
	}

	var body conceptResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding the hostus concept response: %w", err)
	}
	areas := make([]domain.Area, 0, len(body.Distribution))
	for _, d := range body.Distribution {
		// Only the scheme situs stores; anything else would be a code in a
		// namespace the index cannot compare against.
		if d.AreaScheme != domain.SchemeWGSRPDL3 {
			continue
		}
		a := domain.Area{Scheme: d.AreaScheme, Code: d.AreaCode}
		if a.IsComplete() {
			areas = append(areas, a)
		}
	}
	return areas, nil
}
```

`errBodyLimit` und die Felder `baseURL`/`httpClient` existieren bereits in
`client.go` — nicht neu anlegen.

- [ ] **Step 4: Tests laufen lassen und grün sehen**

Run: `go test ./internal/adapters/hostus/ -count=1 -v`
Expected: PASS, Ausgabe rauschfrei.

- [ ] **Step 5: `make verify` und Commit**

```bash
git add internal/adapters/hostus
git commit -m "feat(hostus): read species distribution per concept"
```

---

### Task 4: `IngestDistribution` und die Einbindung in den Ingest

**Files:**
- Create: `internal/application/distribution_ingest.go`,
  `internal/application/distribution_ingest_test.go`
- Modify: `cmd/situs/ingest.go`

**Interfaces:**
- Consumes: `output.Repository`, `output.DistributionSource`,
  `output.IngestTx.UpsertDistribution` (Tasks 1–2), `Client.Areas` (Task 3).
- Produces:
  - `application.IngestDistribution(ctx context.Context, repo output.Repository, src output.DistributionSource) (DistributionReport, error)`
  - `application.DistributionReport{Concepts, WithAreas, Rows int}`

- [ ] **Step 1: Den fehlschlagenden Test schreiben**

Create `internal/application/distribution_ingest_test.go`:

```go
package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

type fakeDistSource struct {
	areas map[string][]domain.Area
	err   error
	asked [][]string
}

func (f *fakeDistSource) Areas(_ context.Context, ids []string) (map[string][]domain.Area, error) {
	f.asked = append(f.asked, ids)
	if f.err != nil {
		return nil, f.err
	}
	return f.areas, nil
}

func TestIngestDistribution_StoresAreasForTheIndexedConcepts(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "diagnostic"},
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R23"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "constant"},
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:2"), VerbatimName: "B", Role: "diagnostic"},
	}
	src := &fakeDistSource{areas: map[string][]domain.Area{
		"wcvp:concept:1": {{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}, {Scheme: domain.SchemeWGSRPDL3, Code: "FRA"}},
	}}

	rep, err := IngestDistribution(context.Background(), repo, src)
	if err != nil {
		t.Fatalf("IngestDistribution: %v", err)
	}
	if rep.Concepts != 2 {
		t.Errorf("Concepts = %d, want 2 distinct concept ids", rep.Concepts)
	}
	if rep.WithAreas != 1 || rep.Rows != 2 {
		t.Errorf("report = %+v, want WithAreas 1 / Rows 2", rep)
	}
	if len(src.asked) != 1 {
		t.Errorf("source was asked %d times, want once for all distinct ids", len(src.asked))
	}
	if len(src.asked[0]) != 2 {
		t.Errorf("asked for %v, want the two distinct ids (deduplicated)", src.asked[0])
	}
}

// The distribution is extra information, not a fact of the index. An outage
// must leave the index usable — unlike the species-role ingest, where a
// resolver failure aborts so that 13791 names are not all booked unresolvable.
func TestIngestDistribution_SourceFailureDoesNotAbort(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "diagnostic"},
	}
	src := &fakeDistSource{err: errors.New("upstream down")}

	rep, err := IngestDistribution(context.Background(), repo, src)
	if err != nil {
		t.Fatalf("IngestDistribution returned an error, want a usable report: %v", err)
	}
	if rep.WithAreas != 0 || rep.Rows != 0 {
		t.Errorf("report = %+v, want zeros — a visible statement, not a silent failure", rep)
	}
	if repo.committed {
		t.Error("nothing was written, so no transaction should have been committed")
	}
}

func TestIngestDistribution_NoConceptsIsNotAnError(t *testing.T) {
	repo := newFakeRepo()
	rep, err := IngestDistribution(context.Background(), repo, &fakeDistSource{})
	if err != nil {
		t.Fatalf("IngestDistribution: %v", err)
	}
	if rep.Concepts != 0 {
		t.Errorf("Concepts = %d, want 0", rep.Concepts)
	}
}
```

`strPtr` existiert im Paket (`species_ingest_test.go`). Falls nicht, dort
nachsehen und den vorhandenen Helfer benutzen, keinen zweiten anlegen.

- [ ] **Step 2: Test laufen lassen und fehlschlagen sehen**

Run: `go test ./internal/application/ -run IngestDistribution -v`
Expected: FAIL — `undefined: IngestDistribution`.

- [ ] **Step 3: Implementieren**

Create `internal/application/distribution_ingest.go`:

```go
package application

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/jobrunner/situs/internal/ports/output"
)

// DistributionReport is what an operator needs to judge the distribution step:
// how many concepts the index holds, how many of them the source knew, and how
// many rows that produced. WithAreas == 0 is a visible statement.
type DistributionReport struct {
	Concepts  int
	WithAreas int
	Rows      int
}

// IngestDistribution copies the areas of every indexed concept into the index.
//
// A source failure is reported, not returned: the distribution is extra
// information and an index without it is usable, only unfiltered. This is
// deliberately unlike IngestSpeciesRoles, where a resolver failure aborts so
// that every name is not booked as unresolvable.
func IngestDistribution(ctx context.Context, repo output.Repository, src output.DistributionSource) (DistributionReport, error) {
	ids, err := indexedConceptIDs(ctx, repo)
	if err != nil {
		return DistributionReport{}, err
	}
	rep := DistributionReport{Concepts: len(ids)}
	if len(ids) == 0 {
		return rep, nil
	}

	areas, err := src.Areas(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "distribution source unavailable, the index stays unfiltered",
			"concepts", len(ids), "error", err)
		return rep, nil
	}
	if len(areas) == 0 {
		return rep, nil
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return rep, fmt.Errorf("beginning the distribution ingest: %w", err)
	}
	for _, id := range ids {
		list := areas[id]
		if len(list) == 0 {
			continue
		}
		rep.WithAreas++
		for _, a := range list {
			if err := tx.UpsertDistribution(id, a); err != nil {
				return rep, fmt.Errorf("%w (rollback: %w)", err, tx.Rollback())
			}
			rep.Rows++
		}
	}
	if err := tx.Commit(); err != nil {
		return rep, fmt.Errorf("committing the distribution ingest: %w", err)
	}
	return rep, nil
}

// indexedConceptIDs returns the distinct concept ids the index holds, sorted so
// a run is reproducible.
func indexedConceptIDs(ctx context.Context, repo output.Repository) ([]string, error) {
	ids, err := repo.ConceptIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the indexed concept ids: %w", err)
	}
	sort.Strings(ids)
	return ids, nil
}
```

- [ ] **Step 4: `Repository.ConceptIDs` ergänzen**

Der vorige Schritt braucht eine Portmethode, die es noch nicht gibt. In
`internal/ports/output/repository.go` zu `Repository` hinzufügen:

```go
	// ConceptIDs lists the distinct concept ids the index holds, so the
	// distribution step knows what to ask for.
	ConceptIDs(ctx context.Context) ([]string, error)
```

In `internal/adapters/sqlite/read.go` implementieren:

```go
func (db *DB) ConceptIDs(ctx context.Context) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT concept_id FROM species_role
		 WHERE concept_id IS NOT NULL ORDER BY concept_id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading concept ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("sqlite: scanning concept id: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating concept ids: %w", err)
	}
	return out, nil
}
```

Und am `fakeRepo` in `internal/application/ingest_test.go`:

```go
func (r *fakeRepo) ConceptIDs(_ context.Context) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range r.speciesRoles {
		if s.ConceptID != nil && !seen[*s.ConceptID] {
			seen[*s.ConceptID] = true
			out = append(out, *s.ConceptID)
		}
	}
	return out, nil
}
```

- [ ] **Step 5: Einen sqlite-Test für `ConceptIDs` anhängen**

An `internal/adapters/sqlite/write_test.go`:

```go
func TestConceptIDs_DistinctAndWithoutNulls(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	id := "wcvp:concept:1"

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, r := range []domain.SpeciesRole{
		{Key: key, ConceptID: &id, VerbatimName: "A", Role: "diagnostic"},
		{Key: key, ConceptID: &id, VerbatimName: "A2", Role: "constant"},
		{Key: key, ConceptID: nil, VerbatimName: "Moss", Role: "diagnostic"},
	} {
		if err := tx.UpsertSpeciesRole(r); err != nil {
			t.Fatalf("UpsertSpeciesRole: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.ConceptIDs(ctx)
	if err != nil {
		t.Fatalf("ConceptIDs: %v", err)
	}
	if len(got) != 1 || got[0] != id {
		t.Errorf("ConceptIDs() = %v, want exactly [%s]", got, id)
	}
}
```

- [ ] **Step 6: Tests laufen lassen und grün sehen**

Run: `go test ./internal/application/ ./internal/adapters/sqlite/ -count=1`
Expected: PASS.

- [ ] **Step 7: In den Ingest-Befehl einhängen**

In `cmd/situs/ingest.go`: `IngestDistribution` **nach** `IngestSpeciesRoles`
und **vor** `IngestLocalizations` aufrufen (die Verbreitung braucht die
Konzept-IDs), denselben hostus-Client als `DistributionSource` übergeben, und
den Report um ein Feld erweitern:

```go
	Distribution application.DistributionReport
```

Die ausgegebene JSON-Struktur nennt damit `Concepts`, `WithAreas` und `Rows`.

- [ ] **Step 8: `make verify` und Commit**

```bash
git add internal/application internal/adapters/sqlite internal/ports/output cmd/situs
git commit -m "feat(ingest): copy species distribution into the index"
```

---

### Task 5: Die Leseseite — `in_area` und `only_in_area`

**Files:**
- Modify: `internal/ports/input/services.go`, `internal/application/query.go`
- Modify: `internal/adapters/http/habitat.go`
- Modify: `internal/application/query_test.go`,
  `internal/adapters/http/handlers_test.go`

**Interfaces:**
- Consumes: `Repository.AreasForConcepts`, `KnownAreaCodes` (Tasks 1–2).
- Produces:
  - `input.SpeciesEntry` und `input.HabitatTypeRole` je um
    `InArea *bool \`json:"in_area,omitempty"\``
  - `input.AreaFilter{Code string; OnlyInArea bool}` und
    `input.ErrUnknownArea`
  - `QueryService.HabitatType`/`HabitatTypeSpecies`/`SpeciesHabitatTypes` je um
    einen `filter input.AreaFilter`-Parameter

- [ ] **Step 1: Den fehlschlagenden Test schreiben**

An `internal/application/query_test.go` anhängen:

```go
func TestHabitatTypeSpecies_MarksAreaInThreeStates(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	here, elsewhere := "wcvp:concept:here", "wcvp:concept:elsewhere"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: key, ConceptID: &here, VerbatimName: "Here", Role: "diagnostic"},
		{Key: key, ConceptID: &elsewhere, VerbatimName: "Elsewhere", Role: "diagnostic"},
		{Key: key, ConceptID: nil, VerbatimName: "Moss", Role: "diagnostic"},
	}
	repo.distribution = []fakeDistribution{
		{ConceptID: here, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
		{ConceptID: elsewhere, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "FRA"}},
	}

	got, err := NewQueryService(repo).HabitatTypeSpecies(context.Background(), key, "",
		input.AreaFilter{Code: "GER"})
	if err != nil {
		t.Fatalf("HabitatTypeSpecies: %v", err)
	}
	byName := map[string]*bool{}
	for _, s := range got {
		byName[s.VerbatimName] = s.InArea
	}
	if byName["Here"] == nil || !*byName["Here"] {
		t.Errorf("Here: in_area = %v, want true", byName["Here"])
	}
	if byName["Elsewhere"] == nil || *byName["Elsewhere"] {
		t.Errorf("Elsewhere: in_area = %v, want false", byName["Elsewhere"])
	}
	if byName["Moss"] != nil {
		t.Errorf("Moss: in_area = %v, want nil (no concept, so not knowable)", byName["Moss"])
	}
}

// only_in_area removes the definite absences and keeps the unknowns: a list
// that silently loses what it cannot judge is dishonestly clean.
func TestHabitatTypeSpecies_OnlyInAreaKeepsTheUnknowns(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	here, elsewhere := "wcvp:concept:here", "wcvp:concept:elsewhere"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: key, ConceptID: &here, VerbatimName: "Here", Role: "diagnostic"},
		{Key: key, ConceptID: &elsewhere, VerbatimName: "Elsewhere", Role: "diagnostic"},
		{Key: key, ConceptID: nil, VerbatimName: "Moss", Role: "diagnostic"},
	}
	repo.distribution = []fakeDistribution{
		{ConceptID: here, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
		{ConceptID: elsewhere, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "FRA"}},
	}

	got, err := NewQueryService(repo).HabitatTypeSpecies(context.Background(), key, "",
		input.AreaFilter{Code: "GER", OnlyInArea: true})
	if err != nil {
		t.Fatalf("HabitatTypeSpecies: %v", err)
	}
	names := map[string]bool{}
	for _, s := range got {
		names[s.VerbatimName] = true
	}
	if !names["Here"] || !names["Moss"] {
		t.Errorf("names = %v, want Here and Moss (unknown stays)", names)
	}
	if names["Elsewhere"] {
		t.Error("Elsewhere survived only_in_area=true")
	}
}

func TestHabitatTypeSpecies_UnknownAreaIsAnError(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	repo.distribution = []fakeDistribution{
		{ConceptID: "wcvp:concept:1", Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
	}

	_, err := NewQueryService(repo).HabitatTypeSpecies(context.Background(), key, "",
		input.AreaFilter{Code: "NOPE"})
	if !errors.Is(err, input.ErrUnknownArea) {
		t.Errorf("err = %v, want ErrUnknownArea — a list of false would hide the typo", err)
	}
}

func TestHabitatTypeSpecies_WithoutAreaThereIsNoField(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	id := "wcvp:concept:1"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: key, ConceptID: &id, VerbatimName: "A", Role: "diagnostic"},
	}

	got, err := NewQueryService(repo).HabitatTypeSpecies(context.Background(), key, "",
		input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatTypeSpecies: %v", err)
	}
	if len(got) != 1 || got[0].InArea != nil {
		t.Errorf("in_area = %v, want nil without an area filter", got[0].InArea)
	}
}
```

- [ ] **Step 2: Test laufen lassen und fehlschlagen sehen**

Run: `go test ./internal/application/ -run 'InArea|OnlyInArea|UnknownArea|WithoutArea' -v`
Expected: FAIL — `undefined: input.AreaFilter`, zu viele Argumente für
`HabitatTypeSpecies`.

- [ ] **Step 3: Die Ports erweitern**

In `internal/ports/input/services.go`:

```go
// ErrUnknownArea is an area code the index has no data for. It must not be
// answered with a list of "does not occur": a typo and a genuine absence would
// look the same.
var ErrUnknownArea = errors.New("unknown area")

// AreaFilter is the caller's view on a species list. Code is a WGSRPD level 3
// code — the frontend derives it from GPS, so situs needs no ISO mapping (and
// the "CZE = Czechia-Slovakia" ambiguity never arises). OnlyInArea drops the
// definite absences; the unknowns always stay.
type AreaFilter struct {
	Code       string
	OnlyInArea bool
}

// Active reports whether a filter was asked for at all.
func (f AreaFilter) Active() bool { return f.Code != "" }
```

`SpeciesEntry` und `HabitatTypeRole` je um dieses Feld erweitern:

```go
	// InArea is nil when unknowable: no concept id, or a concept without
	// distribution rows. It is absent from the wire without an area filter.
	InArea *bool `json:"in_area,omitempty"`
```

Und die `QueryService`-Signaturen um `filter AreaFilter` erweitern
(`HabitatType`, `SpeciesHabitatTypes`, `HabitatTypeSpecies`).

- [ ] **Step 4: Die Anreicherung in `query.go` implementieren**

Ein Helfer, der von allen drei Methoden benutzt wird:

```go
// areaLookup resolves the filter once per request: it validates the code
// against the index and returns the areas of the concepts in play.
func (q *QueryService) areaLookup(ctx context.Context, filter input.AreaFilter, conceptIDs []string) (map[string][]string, error) {
	if !filter.Active() {
		return nil, nil
	}
	known, err := q.repo.KnownAreaCodes(ctx, domain.SchemeWGSRPDL3)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(known, filter.Code) {
		return nil, fmt.Errorf("area %q: %w", filter.Code, input.ErrUnknownArea)
	}
	return q.repo.AreasForConcepts(ctx, conceptIDs, domain.SchemeWGSRPDL3)
}

// inArea is the three-state answer: nil when not knowable.
func inArea(areas map[string][]string, conceptID, code string) *bool {
	if areas == nil || conceptID == "" {
		return nil
	}
	list, ok := areas[conceptID]
	if !ok {
		return nil // the concept has no distribution data at all
	}
	yes := slices.Contains(list, code)
	return &yes
}
```

In `HabitatTypeSpecies` die Konzept-IDs der Rollen sammeln, `areaLookup`
aufrufen, je Eintrag `InArea` setzen und bei `OnlyInArea` die Einträge
auslassen, deren `InArea` **nicht nil und false** ist. Dasselbe Muster in
`HabitatType` (über die drei Rollen-Buckets) und in `SpeciesHabitatTypes` (dort
für das abgefragte Konzept selbst).

`slices` in die Imports aufnehmen.

- [ ] **Step 5: Tests laufen lassen und grün sehen**

Run: `go test ./internal/application/ -count=1`
Expected: PASS.

- [ ] **Step 6: Die Handler anpassen**

In `internal/adapters/http/habitat.go`: `area` und `only_in_area` aus der Query
lesen, in `input.AreaFilter` verpacken, an den Service geben, und
`input.ErrUnknownArea` auf `400 INVALID_QUERY` abbilden. Den bisherigen
`input.ErrUpstreamUnavailable`-Zweig **entfernen**.

Ein Handler-Test dafür, an `internal/adapters/http/handlers_test.go`:

```go
func TestHabitatTypeSpecies_UnknownAreaIsInvalidQuery(t *testing.T) {
	srv := newTestServer(t, unknownAreaQueryService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/R22/species?area=NOPE", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
		t.Errorf("body = %s, want the INVALID_QUERY envelope", rec.Body)
	}
}
```

`unknownAreaQueryService()` ist ein Stub, dessen `HabitatTypeSpecies`
`input.ErrUnknownArea` zurückgibt — im Stil der vorhandenen Stubs in dieser
Datei anlegen.

- [ ] **Step 7: Tests, `make verify`, Commit**

Run: `go test ./internal/adapters/http/ ./internal/application/ -count=1 && make verify`
Expected: PASS.

```bash
git add internal/ports/input internal/application internal/adapters/http
git commit -m "feat(api): mark and optionally filter species by area"
```

---

### Task 6: Der Batch über Konzept-IDs, und hostus aus dem Serve-Pfad entfernen

Der Kern des Umbaus. Alles davor war Vorarbeit.

**Files:**
- Modify: `internal/ports/input/services.go` (`NameResolution` →
  `ConceptResolution`, `SpeciesNameQueryService` entfernen, `QueryService` um
  `SpeciesSetHabitatTypes`)
- Modify: `internal/application/query.go` (`NameQueryService` entfernen,
  `SpeciesSetHabitatTypes` ergänzen)
- Modify: `internal/adapters/http/species.go`, `server.go`
- Modify: `internal/app/app.go`
- Create: `internal/app/arch_test.go`
- Modify: `internal/adapters/http/handlers_test.go`, `openapi.yaml`,
  `api/openapi/openapi.yaml`

**Interfaces:**
- Consumes: `input.AreaFilter` (Task 5), `QueryService` (bestehend).
- Produces:
  - `input.ConceptResolution{ConceptID string; Known bool; Reason string; InArea *bool; HabitatTypes []HabitatTypeRole}`
  - `QueryService.SpeciesSetHabitatTypes(ctx, conceptIDs []string, lang string, filter AreaFilter) ([]ConceptResolution, error)`

- [ ] **Step 1: Den fehlschlagenden Test schreiben**

An `internal/application/query_test.go` anhängen:

```go
func TestSpeciesSetHabitatTypes_OneEntryPerInputInOrder(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	known := "wcvp:concept:known"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: key, ConceptID: &known, VerbatimName: "A", Role: "diagnostic"},
	}

	in := []string{known, "wcvp:concept:nofacts", "cdm:concept:other", known}
	got, err := NewQueryService(repo).SpeciesSetHabitatTypes(context.Background(), in, "", input.AreaFilter{})
	if err != nil {
		t.Fatalf("SpeciesSetHabitatTypes: %v", err)
	}
	if len(got) != len(in) {
		t.Fatalf("got %d entries, want %d — one per input, duplicates included", len(got), len(in))
	}
	for i, want := range in {
		if got[i].ConceptID != want {
			t.Errorf("entry %d = %q, want %q (input order)", i, got[i].ConceptID, want)
		}
	}
	if !got[0].Known || len(got[0].HabitatTypes) != 1 {
		t.Errorf("entry 0 = %+v, want known with one habitat type", got[0])
	}
	if got[3].ConceptID != known || !got[3].Known {
		t.Errorf("entry 3 = %+v, want the duplicate answered too", got[3])
	}
}

// The two reasons are different diagnoses: a wrong backbone is the caller's
// mistake, a concept without facts is the data's limit. One label for both
// sends people looking in the wrong place.
func TestSpeciesSetHabitatTypes_DistinguishesTheTwoReasons(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	known := "wcvp:concept:known"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: key, ConceptID: &known, VerbatimName: "A", Role: "diagnostic"},
	}

	got, err := NewQueryService(repo).SpeciesSetHabitatTypes(context.Background(),
		[]string{"cdm:concept:x", "wcvp:concept:nofacts"}, "", input.AreaFilter{})
	if err != nil {
		t.Fatalf("SpeciesSetHabitatTypes: %v", err)
	}
	if got[0].Known || got[0].Reason != "unknown_backbone" {
		t.Errorf("entry 0 = %+v, want unknown_backbone", got[0])
	}
	if got[1].Known || got[1].Reason != "unknown_concept" {
		t.Errorf("entry 1 = %+v, want unknown_concept", got[1])
	}
}
```

- [ ] **Step 2: Test laufen lassen und fehlschlagen sehen**

Run: `go test ./internal/application/ -run SpeciesSet -v`
Expected: FAIL — `SpeciesSetHabitatTypes undefined`.

- [ ] **Step 3: Ports umstellen**

In `internal/ports/input/services.go`: `NameResolution` durch

```go
// ConceptResolution is one entry of the batch answer. Known is false and
// HabitatTypes empty when the index cannot answer — the input is reported back
// either way, never dropped.
type ConceptResolution struct {
	ConceptID    string            `json:"concept_id"`
	Known        bool              `json:"known"`
	Reason       string            `json:"reason,omitempty"`
	InArea       *bool             `json:"in_area,omitempty"`
	HabitatTypes []HabitatTypeRole `json:"habitat_types"`
}
```

ersetzen, `SpeciesNameQueryService` **löschen**, `ErrUpstreamUnavailable`
**löschen**, und `QueryService` um

```go
	// SpeciesSetHabitatTypes answers a whole field record at once: one entry per
	// input concept id, in input order, duplicates included.
	SpeciesSetHabitatTypes(ctx context.Context, conceptIDs []string, lang string, filter AreaFilter) ([]ConceptResolution, error)
```

erweitern.

- [ ] **Step 4: Implementieren und `NameQueryService` löschen**

In `internal/application/query.go` den ganzen `NameQueryService`-Block löschen
und ergänzen:

```go
// indexBackbone is the concept-id prefix the index was built from. Anything
// else cannot match and is reported as such instead of answering empty.
const indexBackbone = "wcvp"

func (q *QueryService) SpeciesSetHabitatTypes(ctx context.Context, conceptIDs []string, lang string, filter input.AreaFilter) ([]input.ConceptResolution, error) {
	areas, err := q.areaLookup(ctx, filter, conceptIDs)
	if err != nil {
		return nil, err
	}

	// Deduplicated upstream work, one answer per input: the caller pairs
	// response[i] with conceptIDs[i], so duplicates must keep their positions.
	cache := map[string][]input.HabitatTypeRole{}
	out := make([]input.ConceptResolution, 0, len(conceptIDs))
	for _, id := range conceptIDs {
		entry := input.ConceptResolution{
			ConceptID:    id,
			HabitatTypes: []input.HabitatTypeRole{},
			InArea:       inArea(areas, id, filter.Code),
		}
		if !strings.HasPrefix(id, indexBackbone+":") {
			entry.Reason = "unknown_backbone"
			out = append(out, entry)
			continue
		}
		types, ok := cache[id]
		if !ok {
			types, err = q.SpeciesHabitatTypes(ctx, id, lang, input.AreaFilter{})
			if err != nil && !errors.Is(err, output.ErrNotFound) {
				return nil, err
			}
			if types == nil {
				types = []input.HabitatTypeRole{}
			}
			cache[id] = types
		}
		if len(types) == 0 {
			entry.Reason = "unknown_concept"
			out = append(out, entry)
			continue
		}
		entry.Known = true
		entry.HabitatTypes = types
		out = append(out, entry)
	}
	return out, nil
}
```

- [ ] **Step 5: Tests laufen lassen und grün sehen**

Run: `go test ./internal/application/ -count=1`
Expected: PASS.

- [ ] **Step 6: Den Handler umstellen**

In `internal/adapters/http/species.go`: `batchRequest` von `Names []string` auf
`ConceptIDs []string \`json:"concept_ids"\`` ändern, die Grenze `maxBatchNames`
zu `maxBatchConceptIDs` umbenennen (Wert 300 bleibt), Dedupe für den
Upstream-Aufruf entfernen (das macht jetzt der Service), `area` und
`only_in_area` lesen und `s.query.SpeciesSetHabitatTypes` aufrufen. Die
Trailing-Daten-Prüfung und die `maxItems`-Grenze auf der **rohen** Array-Länge
bleiben unverändert.

In `internal/adapters/http/server.go`: `Deps.Names` und
`CodeUpstreamUnavailable` löschen.

- [ ] **Step 7: Den Composition Root entlasten**

In `internal/app/app.go`: den `hostus.NewClient`-Aufruf und `Names:` aus
`httpapi.Deps` entfernen; der `hostus`- und der `net/http`-Import fallen weg,
sofern nicht anderweitig gebraucht.

- [ ] **Step 8: Die Zusage als Test festhalten**

Create `internal/app/arch_test.go`:

```go
package app_test

import (
	"go/build"
	"strings"
	"testing"
)

// The point of the autark runtime: serving needs no hostus. A stray import here
// would reintroduce the dependency without anyone noticing, so it is a test and
// not a comment.
func TestServePathDoesNotImportTheHostusAdapter(t *testing.T) {
	pkg, err := build.Import("github.com/jobrunner/situs/internal/app", "", 0)
	if err != nil {
		t.Fatalf("importing internal/app: %v", err)
	}
	for _, imp := range pkg.Imports {
		if strings.Contains(imp, "internal/adapters/hostus") {
			t.Errorf("internal/app imports %q — the serve path must stay free of hostus", imp)
		}
	}
}
```

- [ ] **Step 9: Die OpenAPI-Spezifikation nachziehen**

In `internal/adapters/http/openapi.yaml`:
- den Request-Body von `POST /v1/species/habitat-types` auf `concept_ids`
  umstellen (`minItems: 1`, `maxItems: 300`), die Beschreibung auf
  Konzept-IDs und die Eingabereihenfolge anpassen
- das Antwortschema `NameResolution` in `ConceptResolution` umbenennen, mit
  `known`, `reason` (`enum: [unknown_backbone, unknown_concept]`), `in_area`
- einen Parameter `Area` (`in: query`, WGSRPD-L3-Code) und `OnlyInArea`
  (`in: query`, boolean) definieren und auf den drei betroffenen Routen
  referenzieren
- `in_area` in `SpeciesEntry` und `HabitatTypeRole` aufnehmen
- `UPSTREAM_UNAVAILABLE` aus dem `code`-Enum des `Error`-Schemas entfernen
- die `502`-Antwort der Batch-Route entfernen

Dann spiegeln:

```bash
cp internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
cmp internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
```

- [ ] **Step 10: Handler-Tests umstellen**

In `internal/adapters/http/handlers_test.go`: alle Batch-Tests von `names` auf
`concept_ids` umstellen, `seededNameQueryService`/`newServerWithNames` entfernen
(der Stub `QueryService` deckt den Batch jetzt ab), und zwei Tests ergänzen:

```go
func TestSpeciesBatch_AnswersOneEntryPerConceptIDInOrder(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	body := `{"concept_ids":["wcvp:concept:1","cdm:concept:x","wcvp:concept:1"]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types", strings.NewReader(body))
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []struct {
		ConceptID string `json:"concept_id"`
		Known     bool   `json:"known"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3 (one per input, duplicate included)", len(got))
	}
	if got[1].Reason != "unknown_backbone" {
		t.Errorf("entry 1 reason = %q, want unknown_backbone", got[1].Reason)
	}
}

func TestSpeciesBatch_RejectsTheOldNamesField(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types",
		strings.NewReader(`{"names":["Bromus erectus"]}`))
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 — verbatim names are no longer accepted", rec.Code)
	}
}
```

Der zweite Test hängt daran, dass der Decoder `DisallowUnknownFields` benutzt —
das tut er heute schon.

- [ ] **Step 11: Alles laufen lassen**

Run: `go test ./... -count=1 && make verify`
Expected: PASS, inklusive Contract-Test und Byte-Identität der Spec-Kopien.

- [ ] **Step 12: Commit**

```bash
git add internal api
git commit -m "feat(api)!: batch habitat types by concept id, drop the runtime hostus dependency"
```

---

### Task 7: Selbstauskunft, Doku und der Nachweis am echten Index

**Files:**
- Modify: `internal/adapters/http/habitat.go` (oder wo `handleInfo` liegt),
  `internal/ports/input/services.go`, `internal/application/query.go`
- Modify: `internal/adapters/http/openapi.yaml`, `api/openapi/openapi.yaml`
- Modify: `docs/reference/http-api.md`, `docs/reference/configuration.md`,
  `docs/how-to/ingest.md`, `README.md`, `CLAUDE.md`

**Interfaces:**
- Consumes: `Repository.ConceptIDs`, `KnownAreaCodes` (Tasks 2, 4).
- Produces: `QueryService.IndexInfo(ctx) (input.IndexInfo, error)` mit
  `input.IndexInfo{ConceptBackbones []string; SpeciesWithConcept int; AreaScheme string; AreasWithData int}`

- [ ] **Step 1: Den fehlschlagenden Test schreiben**

An `internal/application/query_test.go`:

```go
func TestIndexInfo_ReportsTheBackboneTheIndexWasBuiltFrom(t *testing.T) {
	repo := newFakeRepo()
	id := "wcvp:concept:1"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: &id, VerbatimName: "A", Role: "diagnostic"},
	}
	repo.distribution = []fakeDistribution{
		{ConceptID: id, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
	}

	got, err := NewQueryService(repo).IndexInfo(context.Background())
	if err != nil {
		t.Fatalf("IndexInfo: %v", err)
	}
	if len(got.ConceptBackbones) != 1 || got.ConceptBackbones[0] != "wcvp" {
		t.Errorf("ConceptBackbones = %v, want [wcvp]", got.ConceptBackbones)
	}
	if got.SpeciesWithConcept != 1 || got.AreasWithData != 1 {
		t.Errorf("info = %+v, want one concept and one area", got)
	}
	if got.AreaScheme != domain.SchemeWGSRPDL3 {
		t.Errorf("AreaScheme = %q, want %q", got.AreaScheme, domain.SchemeWGSRPDL3)
	}
}
```

- [ ] **Step 2: Test laufen lassen und fehlschlagen sehen**

Run: `go test ./internal/application/ -run IndexInfo -v`
Expected: FAIL — `IndexInfo undefined`.

- [ ] **Step 3: Implementieren**

`input.IndexInfo` definieren, `IndexInfo(ctx)` auf `QueryService` implementieren
(Präfixe aus `ConceptIDs` ableiten, Gebietszahl aus `KnownAreaCodes`), und
`handleInfo` um das `index`-Objekt erweitern.

- [ ] **Step 4: Tests, Spec, Doku**

`/v1/info` in beiden OpenAPI-Kopien um `index` erweitern, spiegeln, `cmp`
prüfen. Dann die Prosa:

- `docs/reference/http-api.md`: `concept_ids` statt `names`, `known`/`reason`,
  `area`/`only_in_area` mit der Drei-Zustands-Tabelle, `UPSTREAM_UNAVAILABLE`
  aus der Codeliste streichen, den Satz ergänzen, dass die Leseseite ohne
  hostus läuft
- `docs/reference/configuration.md`: die vier `SITUS_HOSTUS_*`-Zeilen auf „nur
  beim `ingest`" verschärfen
- `docs/how-to/ingest.md`: den Verbreitungsschritt und seine Reportfelder
  dokumentieren, inklusive „ein Ausfall der Quelle bricht den Ingest nicht ab"
- `README.md`: die Beispiele auf `concept_ids` umstellen, einen Satz zur
  Autarkie
- `CLAUDE.md`: den Abschnitt „Current State" und die HTTP-Konventionen
  nachziehen (`UNRESOLVABLE` **und** `UPSTREAM_UNAVAILABLE` sind keine Codes
  mehr)

- [ ] **Step 5: `make verify` und `make docs`**

Run: `make verify && make docs`
Expected: PASS.

- [ ] **Step 6: Am echten Index nachweisen**

Der Nachweis, dass der Umbau trägt — nicht nur die Tests:

```bash
# 1. Ingest MIT hostus (die Verbreitung kommt von dort)
HOSTUS_SQLITE_PATH=/tmp/hostus-index.db /tmp/hostus-bin serve --port 8080 --ui=false &
./situs ingest --csv-dir pipelines/eunis/out --db /tmp/situs-real.sqlite

# 2. hostus beenden — ab hier darf es nicht mehr gebraucht werden
pkill -f hostus-bin

# 3. Serven und alle Routen prüfen
SITUS_INDEX_PATH=/tmp/situs-real.sqlite ./situs serve &
curl -s localhost:8070/v1/info
curl -s 'localhost:8070/v1/habitat-type/eunis@2021/R15/species?area=GER' | head -c 400
curl -s 'localhost:8070/v1/habitat-type/eunis@2021/R15/species?area=GER&only_in_area=true' | head -c 400
curl -s -X POST localhost:8070/v1/species/habitat-types -H 'Content-Type: application/json' \
  -d '{"concept_ids":["wcvp:concept:2457314","cdm:concept:x"]}'
curl -s 'localhost:8070/v1/habitat-type/eunis@2021/R22/species?area=NOPE'
```

Erwartet: alle Antworten kommen **ohne laufendes hostus**; `R15` zeigt
`in_area: false`-Einträge (gemessen 50 % der Arten kommen in DE nicht vor);
`only_in_area=true` liefert weniger Einträge, aber die `null`-Einträge bleiben;
`cdm:concept:x` ergibt `unknown_backbone`; `area=NOPE` ergibt 400.

Die Zahlen des Verbreitungs-Ingests (`Concepts`, `WithAreas`, `Rows`) in die
Commit-Nachricht aufnehmen. **Jeden gestarteten Prozess wieder beenden.**

- [ ] **Step 7: Commit**

```bash
git add internal api docs README.md CLAUDE.md
git commit -m "docs: autark read API, area filter and index self-description"
```

---

## Self-Review

**Spec-Abdeckung.** Teil 1 (Entkopplung) → Task 6; die entfallenden Symbole sind
dort einzeln benannt. Die prüfbare Zusage → Task 6 Step 8. Zwei `reason`-Werte →
Task 6 Steps 1/4. Selbstauskunft → Task 7. Teil 2: Datenquelle und Ingest →
Tasks 3–4; Schema → Task 2; Port → Task 1; Filter-Semantik → Task 5; Area-Code
und `INVALID_QUERY` → Task 5 Steps 3/6; `in_area` auf der Batch-Route → Task 6
Step 4. Fehlerbehandlungstabelle → Tasks 5–6. Tests → in jedem Task. Die
bewusst offene Backbone-Fassung bleibt offen, wie in der Spec vermerkt.

**Platzhalter.** Keine. Die Prosa-Schritte in Task 5 Step 4, Task 6 Steps 6/7/9
und Task 7 Steps 3/4 beschreiben Änderungen an vorhandenem Code, dessen
Zielzustand durch die Tests und die Portdefinitionen festgelegt ist — die
Signaturen und Feldnamen stehen alle ausgeschrieben.

**Typkonsistenz.** `domain.Area{Scheme, Code}` und `domain.SchemeWGSRPDL3`
durchgehend. `AreasForConcepts(ctx, []string, string) (map[string][]string, error)`
in Tasks 1, 2, 5. `input.AreaFilter{Code, OnlyInArea}` in Tasks 5, 6.
`ConceptResolution` mit `ConceptID/Known/Reason/InArea/HabitatTypes` in Task 6,
`IndexInfo` in Task 7. `ConceptIDs(ctx)` in Tasks 4 und 7. `InArea *bool` an
`SpeciesEntry`, `HabitatTypeRole` und `ConceptResolution` — überall Zeiger, damit
drei Zustände darstellbar bleiben.

**Reihenfolge-Abhängigkeit, bewusst:** Task 1 lässt `go test ./...` kurzzeitig
rot (der `fakeRepo` kennt die neuen Methoden noch nicht), Task 2 Step 6 macht es
wieder grün. Das ist in Task 1 Step 6 ausdrücklich vermerkt, damit niemand dort
zu reparieren beginnt.
