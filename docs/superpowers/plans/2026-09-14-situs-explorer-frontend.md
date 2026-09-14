# Explorer-Frontend & Zeigerwertanalyse Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** situs bekommt eine eingebettete, abhängigkeitsfreie Weboberfläche unter `GET /` plus die zwei Routen, die sie braucht: eine Namenssuche über die Index-eigenen Namen und eine Zeigerwertanalyse über eine Artenliste.

**Architecture:** Hexagonal wie der Rest des Dienstes. Die Namenssuche ist ein reiner Read (sqlite-Adapter → Port → dünner Use-Case → Handler). Die Zeigerwertanalyse trennt strikt: eine **pure, I/O-freie Statistikfunktion** in `internal/application` (100 % Coverage-Floor) gegenüber dem Datenzugriff im sqlite-Adapter. Die Explorer-Seite folgt exakt dem Muster von `/docs`: eine `//go:embed`-Datei, inline ausgeliefert, ohne CDN.

**Tech Stack:** Go 1.26, `gorilla/mux`, `modernc.org/sqlite`. Frontend: reines HTML + Vanilla-JS + Inline-CSS, **keine** neue Abhängigkeit in `go.mod`, **keine** npm-Kette.

**Spec:** `docs/superpowers/specs/2026-09-14-situs-explorer-frontend-design.md`

## Global Constraints

Diese gelten für **jede** Task; sie sind Teil jeder Aufgabenstellung:

- **Keine neue Go-Abhängigkeit.** `gomodguard_v2` bricht den Build bei einer nicht in `.golangci.yml` erlaubten Bibliothek. Erlaubt sind nur: Go-Stdlib, `gorilla/mux`, `viper`, `cobra`, `modernc.org/sqlite`, OpenTelemetry-SDK (+`otelmux`), Prometheus-Client.
- **SQL-Statements sind statische Strings mit `?`-Platzhaltern.** Werte werden niemals in die Anweisung konkateniert (gosec G201/G202 brechen den Build). Nur Platzhalter dürfen generiert werden, nie Werte.
- **Jede gemountete Route muss in `internal/adapters/http/openapi.yaml` stehen** und umgekehrt — `TestRoutesMatchOpenAPISpec` prüft beide Richtungen und schlägt bei einer Route ohne explizites `.Methods()` fehl.
- **`openapi.yaml` existiert in zwei byte-identischen Kopien:** `internal/adapters/http/openapi.yaml` (eingebettet) und `api/openapi/openapi.yaml`. `TestOpenAPICopiesAreIdentical` hält das fest. Jede Änderung muss in **beide** Dateien.
- **Genau drei Fehlercodes:** `INVALID_QUERY`, `NOT_FOUND`, `INTERNAL_ERROR`. Es kommt keiner hinzu.
- **Coverage-Floors sind ein Ratchet** (`.coverage-floors`): `internal/application` **100 %**, `internal/ports/input` **100 %**, `internal/adapters/sqlite` 96 %, `internal/adapters/http` 88 %, TOTAL 95 %. Niemals senken.
- **Null `//nolint`, null `#nosec`, null `TODO`/`FIXME`/`HACK`/`XXX`** in Go-Dateien (`.debt-budget` steht auf 0).
- **`make verify` muss vor jedem Commit grün sein** (fmt-check, vet, lint, test, arch, debt, build).
- **Kommentare sind englisch und sparsam** — sie erklären *warum*, nicht *was*. Deutsch nur in `README.md` und `docs/`.
- **Der Serve-Pfad bleibt autark:** keine neue Route ruft hostus oder irgendeinen Upstream an. `internal/app/arch_test.go` verbietet `internal/app` sogar den Import des hostus-Adapters.
- **Fehlende Daten sind fehlende Felder, nie erfundene Nullen.** Ein `sd` von `0` bei einem einzigen Wert wäre eine Falschaussage.

---

## File Structure

| Datei | Verantwortung |
|---|---|
| `internal/domain/species_name.go` (neu) | `SpeciesName` — ein Index-eigener Name mit optionaler Concept-ID |
| `internal/ports/output/repository.go` (ändern) | `SearchSpeciesNames`, `TraitsForConcepts` ergänzen |
| `internal/ports/input/services.go` (ändern) | `SpeciesSearchHit`, `TraitSummary`, `VocabSummary`, `DimensionSummary`, `UnknownConcept`; `QueryService` erweitern |
| `internal/adapters/sqlite/search.go` (neu) | Namenssuche-Query |
| `internal/adapters/sqlite/trait.go` (ändern) | `TraitsForConcepts` — Zeigerwerte mehrerer Konzepte in einem Read |
| `internal/application/species_search.go` (neu) | Use-Case Namenssuche: Validierung + Mapping |
| `internal/application/trait_summary_stats.go` (neu) | **Pure Statistik**, keine I/O — Mittel, gewichtetes Mittel, SD |
| `internal/application/trait_summary.go` (neu) | Use-Case Zeigerwertanalyse: Konzepte auflösen, Statistik anwenden |
| `internal/adapters/http/search.go` (neu) | Handler `GET /v1/species/search` |
| `internal/adapters/http/trait_summary.go` (neu) | Handler `POST /v1/species/traits/summary` |
| `internal/adapters/http/explorer.go` (neu) | `//go:embed explorer.html`, Handler `GET /` |
| `internal/adapters/http/explorer.html` (neu) | Die Explorer-Seite (HTML + Vanilla-JS + Inline-CSS) |
| `internal/adapters/http/server.go` (ändern) | Drei Routen mounten |
| `internal/adapters/http/openapi.yaml` + `api/openapi/openapi.yaml` (ändern) | Drei Routen + Schemata dokumentieren |
| `docs/reference/http-api.md`, `README.md` (ändern) | Doku |

---

### Task 1: Namenssuche — Domäne, Port und sqlite-Adapter

**Files:**
- Create: `internal/domain/species_name.go`
- Create: `internal/adapters/sqlite/search.go`
- Create: `internal/adapters/sqlite/search_test.go`
- Modify: `internal/ports/output/repository.go`

**Interfaces:**
- Consumes: nichts aus früheren Tasks.
- Produces:
  - `domain.SpeciesName{VerbatimName string; ConceptID *string}`
  - `output.Repository.SearchSpeciesNames(ctx context.Context, q string, limit int) ([]domain.SpeciesName, error)`
  - implementiert von `*sqlite.DB`

**Kontext für die Implementierung:** Der Index führt 3780 distinkte `verbatim_name` in `species_role`, davon 3314 mit `concept_id`. Gemessen: **kein** Name trägt mehr als eine Concept-ID, und **kein** gespeicherter Name enthält `%` oder `_`. Die Nutzereingabe kann diese Zeichen aber sehr wohl enthalten — sie muss escaped werden, sonst matcht `q=%` den ganzen Index.

- [ ] **Step 1: Das Domänen-Value-Object anlegen**

`internal/domain/species_name.go`:

```go
package domain

// SpeciesName is one verbatim species name the index itself carries, with
// the concept id it resolved to — nil when the ingest could not resolve it.
//
// Deliberately nullable rather than dropped: a name the index knows but
// could not resolve is part of the truth about this index. Hiding those
// rows would suggest a higher resolution rate than the measured one.
type SpeciesName struct {
	VerbatimName string
	ConceptID    *string
}
```

- [ ] **Step 2: Den Port erweitern**

In `internal/ports/output/repository.go`, innerhalb des `Repository`-Interfaces (nach `ConceptIDs`):

```go
	// SearchSpeciesNames returns the index's own verbatim names containing
	// q (case-insensitive substring), at most limit of them, ordered by
	// name so the same query is stably reproducible.
	//
	// This is NOT name resolution: no fuzzy matching, no synonyms, no
	// author variants. It answers only "which names does THIS index carry,
	// and under which concept id" — real resolution is hostus' job.
	SearchSpeciesNames(ctx context.Context, q string, limit int) ([]domain.SpeciesName, error)
```

- [ ] **Step 3: Den fehlschlagenden Test schreiben**

`internal/adapters/sqlite/search_test.go`:

**Wichtig:** Die Tests dieses Pakets liegen in `package sqlite` (paketintern,
nicht `sqlite_test`) — so machen es `write_test.go` und `trait_test.go`
bereits, und nur so ist der vorhandene Helfer `openTestDB(t) *DB` aus
`write_test.go` sichtbar.

```go
package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// seedSearchDB builds an index with three names: two resolved, one not.
func seedSearchDB(t *testing.T) *DB {
	t.Helper()
	db := openTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTypology(domain.Typology{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}); err != nil {
		t.Fatalf("UpsertTypology: %v", err)
	}
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T17"}
	if err := tx.UpsertHabitatType(domain.HabitatType{Key: key, NameEN: "Beech forest"}); err != nil {
		t.Fatalf("UpsertHabitatType: %v", err)
	}
	fagus := "wcvp:concept:83891"
	abies := "wcvp:concept:381621"
	for _, r := range []domain.SpeciesRole{
		{Key: key, VerbatimName: "Fagus sylvatica", Role: "constant", ConceptID: &fagus},
		{Key: key, VerbatimName: "Abies alba", Role: "constant", ConceptID: &abies},
		{Key: key, VerbatimName: "Fagus orientalis", Role: "diagnostic"},
	} {
		if err := tx.UpsertSpeciesRole(r); err != nil {
			t.Fatalf("UpsertSpeciesRole %q: %v", r.VerbatimName, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return db
}

func TestSearchSpeciesNames_SubstringCaseInsensitiveOrdered(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "fagus", 20)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d hits, want 2 (both Fagus names, case-insensitively)", len(got))
	}
	// Ordered by name: "Fagus orientalis" sorts before "Fagus sylvatica".
	if got[0].VerbatimName != "Fagus orientalis" || got[1].VerbatimName != "Fagus sylvatica" {
		t.Errorf("order = %q, %q; want Fagus orientalis before Fagus sylvatica",
			got[0].VerbatimName, got[1].VerbatimName)
	}
}

// An unresolved name must come back WITH the others, carrying a nil concept
// id — never filtered out, or the search would overstate the index.
func TestSearchSpeciesNames_KeepsUnresolvedNamesWithNilConceptID(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "orientalis", 20)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d hits, want 1", len(got))
	}
	if got[0].ConceptID != nil {
		t.Errorf("ConceptID = %v, want nil for the unresolved name", *got[0].ConceptID)
	}
}

func TestSearchSpeciesNames_ResolvedNameCarriesItsConceptID(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "sylvatica", 20)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d hits, want 1", len(got))
	}
	if got[0].ConceptID == nil || *got[0].ConceptID != "wcvp:concept:83891" {
		t.Errorf("ConceptID = %v, want wcvp:concept:83891", got[0].ConceptID)
	}
}

func TestSearchSpeciesNames_LimitCapsTheResult(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "a", 1)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d hits, want exactly the 1 the limit allows", len(got))
	}
}

// A percent sign in the QUERY must be a literal, not a wildcard — otherwise
// "%" alone would dump the whole index.
func TestSearchSpeciesNames_EscapesLikeWildcardsInTheQuery(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "%", 20)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d hits for a literal %%, want 0 (no stored name contains one)", len(got))
	}
}

func TestSearchSpeciesNames_UnderscoreIsLiteralToo(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "_", 20)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d hits for a literal underscore, want 0", len(got))
	}
}

func TestSearchSpeciesNames_NoMatchIsEmptyNotError(t *testing.T) {
	db := seedSearchDB(t)

	got, err := db.SearchSpeciesNames(context.Background(), "Quercus", 20)
	if err != nil {
		t.Fatalf("SearchSpeciesNames: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d hits, want none", len(got))
	}
}
```

- [ ] **Step 4: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/adapters/sqlite/ -run TestSearchSpeciesNames -v`
Expected: FAIL — `db.SearchSpeciesNames undefined`.

`openTestDB` ist bereits in `internal/adapters/sqlite/write_test.go:25` definiert (`func openTestDB(t *testing.T) *DB`) — nicht neu anlegen.

- [ ] **Step 5: Den Adapter implementieren**

`internal/adapters/sqlite/search.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
)

// likeEscape makes q a literal for a LIKE pattern: the wildcards % and _
// and the escape character itself lose their special meaning. Without this,
// a query of "%" would match every name in the index instead of the (zero)
// names that actually contain a percent sign.
//
// The backslash is escaped FIRST — doing it last would also escape the
// backslashes this function just introduced.
func likeEscape(q string) string {
	q = strings.ReplaceAll(q, `\`, `\\`)
	q = strings.ReplaceAll(q, "%", `\%`)
	q = strings.ReplaceAll(q, "_", `\_`)
	return q
}

// SearchSpeciesNames implements output.Repository.
//
// DISTINCT over both columns, not just the name: measured on the pinned
// index no name carries more than one concept id, but if a future ingest
// produced one, showing both rows is the honest answer — silently keeping
// whichever row sqlite returned first would hide the ambiguity.
//
// sqlite's LIKE is case-insensitive for ASCII by default, which is what the
// binomial Latin names in this index are.
func (d *DB) SearchSpeciesNames(ctx context.Context, q string, limit int) ([]domain.SpeciesName, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT DISTINCT verbatim_name, concept_id FROM species_role
		 WHERE verbatim_name LIKE ? ESCAPE '\'
		 ORDER BY verbatim_name
		 LIMIT ?`,
		"%"+likeEscape(q)+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: searching species names for %q: %w", q, err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.SpeciesName{}
	for rows.Next() {
		var n domain.SpeciesName
		var conceptID sql.NullString
		if err := rows.Scan(&n.VerbatimName, &conceptID); err != nil {
			return nil, fmt.Errorf("sqlite: scanning species name for %q: %w", q, err)
		}
		if conceptID.Valid {
			id := conceptID.String
			n.ConceptID = &id
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading species names for %q: %w", q, err)
	}
	return out, nil
}
```

- [ ] **Step 6: Tests laufen lassen, grün bestätigen**

Run: `go test ./internal/adapters/sqlite/ -run TestSearchSpeciesNames -v`
Expected: PASS (7 Tests)

- [ ] **Step 7: Den fakeRepo der Application-Tests nachziehen**

Das Interface ist gewachsen, also erfüllt `fakeRepo` in `internal/application/ingest_test.go` es nicht mehr. Dort ergänzen (in der Nähe der anderen Read-Methoden):

```go
func (r *fakeRepo) SearchSpeciesNames(_ context.Context, q string, limit int) ([]domain.SpeciesName, error) {
	if r.searchErr != nil {
		return nil, r.searchErr
	}
	out := []domain.SpeciesName{}
	for _, n := range r.speciesNames {
		if strings.Contains(strings.ToLower(n.VerbatimName), strings.ToLower(q)) {
			out = append(out, n)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
```

Dazu die zwei Felder in der `fakeRepo`-Struktur ergänzen:

```go
	speciesNames []domain.SpeciesName
	searchErr    error
```

- [ ] **Step 8: Die ganze Suite laufen lassen**

Run: `go test ./...`
Expected: PASS — insbesondere darf kein Paket mehr „does not implement output.Repository" melden.

- [ ] **Step 9: `make verify` und committen**

```bash
make verify
git add internal/domain/species_name.go internal/adapters/sqlite/search.go \
        internal/adapters/sqlite/search_test.go internal/ports/output/repository.go \
        internal/application/ingest_test.go
git commit -m "feat(sqlite): search the index's own verbatim species names"
```

---

### Task 2: Namenssuche — Use-Case, Handler, Route, OpenAPI

**Files:**
- Create: `internal/application/species_search.go`
- Create: `internal/application/species_search_test.go`
- Create: `internal/adapters/http/search.go`
- Create: `internal/adapters/http/search_test.go`
- Modify: `internal/ports/input/services.go`
- Modify: `internal/adapters/http/server.go`
- Modify: `internal/adapters/http/openapi.yaml`, `api/openapi/openapi.yaml`

**Interfaces:**
- Consumes: `output.Repository.SearchSpeciesNames(ctx, q string, limit int) ([]domain.SpeciesName, error)` aus Task 1; `domain.SpeciesName{VerbatimName string; ConceptID *string}`.
- Produces:
  - `input.SpeciesSearchHit{VerbatimName string; ConceptID *string}`
  - `input.QueryService.SearchSpecies(ctx context.Context, q string, limit int) ([]input.SpeciesSearchHit, error)`
  - Route `GET /v1/species/search`
  - Konstanten `input.DefaultSearchLimit = 20`, `input.MaxSearchLimit = 100`

- [ ] **Step 1: Die DTOs und Konstanten ergänzen**

In `internal/ports/input/services.go`, bei den anderen View-Typen:

```go
// SpeciesSearchHit is one hit of the index-own name search.
//
// ConceptID has no omitempty on purpose: a name the index carries but could
// not resolve must arrive as an explicit null, so a client can tell "not
// resolvable" from "field forgotten". Filtering those hits out would state a
// higher resolution rate than the index actually has.
type SpeciesSearchHit struct {
	VerbatimName string  `json:"verbatim_name"`
	ConceptID    *string `json:"concept_id"`
}

// The search result bound. A default keeps a bare ?q= cheap; the maximum
// keeps one request from serving the whole index as an autocomplete answer.
const (
	DefaultSearchLimit = 20
	MaxSearchLimit     = 100
)
```

Im `QueryService`-Interface ergänzen:

```go
	// SearchSpecies finds index-own verbatim names containing q. It is not
	// name resolution (no fuzzy, no synonyms) — see the repository port.
	SearchSpecies(ctx context.Context, q string, limit int) ([]SpeciesSearchHit, error)
```

- [ ] **Step 2: Den fehlschlagenden Use-Case-Test schreiben**

`internal/application/species_search_test.go`:

```go
package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

func strp(s string) *string { return &s }

func TestSearchSpecies_MapsHitsIncludingUnresolvedOnes(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesNames = []domain.SpeciesName{
		{VerbatimName: "Fagus sylvatica", ConceptID: strp("wcvp:concept:83891")},
		{VerbatimName: "Fagus orientalis"},
	}

	got, err := NewQueryService(repo).SearchSpecies(context.Background(), "fagus", 20)
	if err != nil {
		t.Fatalf("SearchSpecies: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d hits, want 2", len(got))
	}
	if got[0].ConceptID == nil || *got[0].ConceptID != "wcvp:concept:83891" {
		t.Errorf("hit 0 ConceptID = %v, want wcvp:concept:83891", got[0].ConceptID)
	}
	if got[1].ConceptID != nil {
		t.Error("hit 1 must keep a nil ConceptID, not be dropped or filled in")
	}
}

func TestSearchSpecies_EmptyQueryIsInvalid(t *testing.T) {
	for _, q := range []string{"", "   "} {
		_, err := NewQueryService(newFakeRepo()).SearchSpecies(context.Background(), q, 20)
		if !errors.Is(err, input.ErrInvalidQuery) {
			t.Errorf("SearchSpecies(%q) error = %v, want ErrInvalidQuery", q, err)
		}
	}
}

// Zero means "unset" and takes the default; a negative or oversized value is
// a malformed request, not something to silently clamp.
func TestSearchSpecies_LimitZeroTakesTheDefault(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesNames = make([]domain.SpeciesName, 50)
	for i := range repo.speciesNames {
		repo.speciesNames[i] = domain.SpeciesName{VerbatimName: "Aaa" + strings.Repeat("a", i)}
	}

	got, err := NewQueryService(repo).SearchSpecies(context.Background(), "aaa", 0)
	if err != nil {
		t.Fatalf("SearchSpecies: %v", err)
	}
	if len(got) != input.DefaultSearchLimit {
		t.Errorf("got %d hits, want the default limit %d", len(got), input.DefaultSearchLimit)
	}
}

func TestSearchSpecies_RejectsNegativeAndOversizedLimit(t *testing.T) {
	for _, limit := range []int{-1, input.MaxSearchLimit + 1} {
		_, err := NewQueryService(newFakeRepo()).SearchSpecies(context.Background(), "fagus", limit)
		if !errors.Is(err, input.ErrInvalidQuery) {
			t.Errorf("SearchSpecies(limit=%d) error = %v, want ErrInvalidQuery", limit, err)
		}
	}
}

func TestSearchSpecies_RepositoryErrorIsWrapped(t *testing.T) {
	repo := newFakeRepo()
	repo.searchErr = errors.New("disk on fire")

	_, err := NewQueryService(repo).SearchSpecies(context.Background(), "fagus", 20)
	if err == nil {
		t.Fatal("SearchSpecies with a failing repository = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "searching species names") {
		t.Errorf("error = %q, want it to name the operation", err)
	}
}

func TestSearchSpecies_NoMatchIsAnEmptySliceNotNil(t *testing.T) {
	got, err := NewQueryService(newFakeRepo()).SearchSpecies(context.Background(), "quercus", 20)
	if err != nil {
		t.Fatalf("SearchSpecies: %v", err)
	}
	if got == nil {
		t.Error("result is nil; want an empty slice so it marshals to [] not null")
	}
}
```

- [ ] **Step 3: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/application/ -run TestSearchSpecies -v`
Expected: FAIL — `SearchSpecies undefined` und ggf. `input.ErrInvalidQuery undefined`.

- [ ] **Step 4: `ErrInvalidQuery` anlegen und im Fehler-Mapping ergänzen**

Geprüft: Ein solcher Sentinel existiert **noch nicht** — es gibt nur
`ErrUnknownTypology`, `ErrUnknownArea`, `ErrUnknownVocab`, `ErrNotFound`. In
`internal/ports/input/services.go` beim bestehenden `var (…)`-Block ergänzen:

```go
	// ErrInvalidQuery is a malformed request parameter -> INVALID_QUERY. The
	// existing ErrUnknown* sentinels say "this value does not exist here";
	// this one says "this value is not well-formed at all".
	ErrInvalidQuery = errors.New("invalid query")
```

Dann in `internal/adapters/http/habitat.go:170` den bestehenden `case` um den
neuen Sentinel erweitern:

```go
	case errors.Is(err, input.ErrUnknownTypology), errors.Is(err, input.ErrUnknownArea),
		errors.Is(err, input.ErrUnknownVocab), errors.Is(err, input.ErrInvalidQuery):
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, err.Error())
```

Ohne diese Zeile fiele jede Validierungsverletzung in den `default`-Zweig und
käme als **500** statt als 400 zurück.

- [ ] **Step 5: Den Use-Case implementieren**

`internal/application/species_search.go`:

```go
package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobrunner/situs/internal/ports/input"
)

// SearchSpecies finds the index's own verbatim names containing q.
//
// limit == 0 means "unset" and takes DefaultSearchLimit. A negative or
// oversized limit is rejected rather than clamped: silently answering a
// different question than the one asked is how a client ends up believing
// it saw every hit.
func (q *QueryService) SearchSpecies(ctx context.Context, query string, limit int) ([]input.SpeciesSearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("q must not be empty: %w", input.ErrInvalidQuery)
	}
	switch {
	case limit == 0:
		limit = input.DefaultSearchLimit
	case limit < 0 || limit > input.MaxSearchLimit:
		return nil, fmt.Errorf("limit must be between 1 and %d: %w", input.MaxSearchLimit, input.ErrInvalidQuery)
	}

	names, err := q.repo.SearchSpeciesNames(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("searching species names for %q: %w", query, err)
	}
	out := make([]input.SpeciesSearchHit, 0, len(names))
	for _, n := range names {
		out = append(out, input.SpeciesSearchHit{VerbatimName: n.VerbatimName, ConceptID: n.ConceptID})
	}
	return out, nil
}
```

- [ ] **Step 6: Use-Case-Tests grün bekommen**

Run: `go test ./internal/application/ -run TestSearchSpecies -v`
Expected: PASS (6 Tests)

- [ ] **Step 7: Den `fakeQueryService` um die neue Methode erweitern**

Das `QueryService`-Interface ist in Step 1 gewachsen — damit erfüllt der
`fakeQueryService` aus `internal/adapters/http/handlers_test.go` es nicht
mehr, und **das gesamte http-Testpaket kompiliert nicht**. Dort ergänzen:

```go
func (f *fakeQueryService) SearchSpecies(_ context.Context, q string, limit int) ([]input.SpeciesSearchHit, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	out := []input.SpeciesSearchHit{}
	for _, h := range f.searchHits {
		if strings.Contains(strings.ToLower(h.VerbatimName), strings.ToLower(q)) {
			out = append(out, h)
		}
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out, nil
}
```

Mit den Feldern in der `fakeQueryService`-Struktur:

```go
	searchHits []input.SpeciesSearchHit
	searchErr  error
```

Und in `seededQueryService()` zwei Treffer setzen — einen aufgelösten, einen
nicht, damit die Handler-Tests beide Fälle sehen:

```go
		searchHits: []input.SpeciesSearchHit{
			{VerbatimName: "Fagus sylvatica", ConceptID: strPtr("wcvp:concept:83891")},
			{VerbatimName: "Fagus orientalis"},
		},
```

Falls es im Paket noch keinen `strPtr`-Helfer gibt, den vorhandenen aus
`handlers_test.go` benutzen oder einen anlegen:
`func strPtr(s string) *string { return &s }`.

- [ ] **Step 8: Den fehlschlagenden Handler-Test schreiben**

`internal/adapters/http/search_test.go`. **Achtung auf die Signatur:**
`newTestServer` nimmt zwei Argumente (`contract_test.go:152`) —
`newTestServer(t, seededQueryService())`, nicht `newTestServer(t, seededQueryService())`:

```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchRoute_ReturnsHitsWithExplicitNullConceptID(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/search?q=fagus", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	// The null must survive as a key, not vanish: clients distinguish
	// "unresolvable" from "field absent".
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"concept_id":null`)) {
		t.Errorf("body = %s, want an explicit \"concept_id\":null for the unresolved hit", rec.Body)
	}

	var hits []struct {
		VerbatimName string  `json:"verbatim_name"`
		ConceptID    *string `json:"concept_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &hits); err != nil {
		t.Fatalf("unmarshaling: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("no hits; the test server's index must carry at least one Fagus name")
	}
}

func TestSearchRoute_MissingQueryIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/search", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a missing q", rec.Code)
	}
}

func TestSearchRoute_UnparsableLimitIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/search?q=fagus&limit=viele", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unparsable limit", rec.Code)
	}
}

func TestSearchRoute_OversizedLimitIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/search?q=fagus&limit=101", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a limit above the maximum", rec.Code)
	}
}

func TestSearchRoute_NoMatchIsEmptyArrayNotNull(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/search?q=zzzznothing", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("body = %q, want %q", got, "[]")
	}
}
```

Imports `bytes` und `strings` nicht vergessen. Den vorhandenen Helfer für den Testserver aus `internal/adapters/http/handlers_test.go` benutzen (dort heißt er ggf. anders als `newTestServer` — den bestehenden verwenden, keinen zweiten anlegen). Dessen Seed-Index muss mindestens einen aufgelösten und einen unaufgelösten `Fagus`-Namen enthalten; falls nicht vorhanden, dort ergänzen.

- [ ] **Step 9: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/adapters/http/ -run TestSearchRoute -v`
Expected: FAIL — 404, weil die Route noch nicht gemountet ist.

- [ ] **Step 10: Den Handler implementieren**

`internal/adapters/http/search.go`:

```go
package httpapi

import (
	"net/http"
	"strconv"
)

// handleSpeciesSearch answers the index-own name search.
//
// An absent limit is 0 ("unset") and lets the use case apply its default; an
// unparsable one is rejected rather than treated as absent, because a typo'd
// limit silently answering a different question is the failure mode this
// route would otherwise have.
func (s *Server) handleSpeciesSearch(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
				"limit must be a whole number")
			return
		}
		limit = parsed
	}

	hits, err := s.deps.Query.SearchSpecies(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, hits)
}
```

- [ ] **Step 11: Die Route mounten**

In `internal/adapters/http/server.go`, bei den `/v1`-Routen. **Vor** `/v1/species/{conceptId}/…` einfügen, damit `search` nicht als Concept-ID gelesen wird:

```go
	r.HandleFunc("/v1/species/search", s.handleSpeciesSearch).Methods(http.MethodGet)
```

- [ ] **Step 12: OpenAPI in BEIDEN Kopien dokumentieren**

In `internal/adapters/http/openapi.yaml` unter `paths:` (alphabetisch bei den anderen `/v1/species`-Einträgen):

```yaml
  /v1/species/search:
    get:
      summary: Namen suchen, die dieser Index selbst führt
      description: >-
        Sucht einen Teilstring (case-insensitiv) in den `verbatim_name`, die
        der Index selbst trägt, und liefert die zugehörige Concept-ID mit.
        **Das ist keine Namensauflösung:** kein Fuzzy-Matching, keine
        Synonyme, keine Autorenvarianten, kein Backbone-Wissen — dafür ist
        hostus zuständig. Diese Route beantwortet ausschließlich „welche
        Namen kennt *dieser* Index". `concept_id` ist `null`, wenn der Ingest
        den Namen nicht auflösen konnte; solche Treffer werden mitgeliefert
        und nicht gefiltert, weil sie zur Wahrheit über den Index gehören.
      operationId: speciesSearch
      parameters:
        - name: q
          in: query
          required: true
          description: Teilstring, mindestens ein Zeichen. Leer ist INVALID_QUERY, nicht „alles".
          schema:
            type: string
          example: fagus
        - name: limit
          in: query
          required: false
          description: Höchstzahl der Treffer. Fehlt der Parameter, gilt 20.
          schema:
            type: integer
            minimum: 1
            maximum: 100
            default: 20
      responses:
        "200":
          description: Die Treffer, nach Namen sortiert. Leer, wenn nichts passt.
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/SpeciesSearchHit"
        "400":
          $ref: "#/components/responses/InvalidQuery"
```

Und unter `components.schemas`:

```yaml
    SpeciesSearchHit:
      type: object
      required: [verbatim_name, concept_id]
      properties:
        verbatim_name:
          type: string
          example: Fagus sylvatica
        concept_id:
          type: string
          nullable: true
          description: >-
            `null`, wenn der Ingest diesen Namen nicht auflösen konnte. Das
            Feld ist immer vorhanden — ein fehlendes und ein leeres Feld
            wären für einen Client nicht unterscheidbar.
          example: wcvp:concept:83891
```

Dann byte-identisch kopieren:

```bash
cp internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
```

- [ ] **Step 13: Alle Tests grün bekommen**

Run: `go test ./...`
Expected: PASS — insbesondere `TestSearchRoute*`, `TestRoutesMatchOpenAPISpec` und `TestOpenAPICopiesAreIdentical`.

- [ ] **Step 14: `make verify` und committen**

```bash
make verify
git add internal/application/species_search.go internal/application/species_search_test.go \
        internal/adapters/http/search.go internal/adapters/http/search_test.go \
        internal/ports/input/services.go internal/adapters/http/server.go \
        internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
git commit -m "feat(http): GET /v1/species/search over the index's own names"
```

---

### Task 3: Zeigerwertanalyse — die pure Statistik

**Files:**
- Create: `internal/application/trait_summary_stats.go`
- Create: `internal/application/trait_summary_stats_test.go`
- Modify: `internal/ports/input/services.go`

**Interfaces:**
- Consumes: `domain.TraitValue{Vocab, VocabVersion string; Dim domain.TraitDim; Value float64; NicheWidth *float64; NSystems *int}`.
- Produces:
  - `input.DimensionSummary`, `input.VocabSummary`, `input.TraitSummary`, `input.UnknownConcept`
  - `summarizeDimension(values []domain.TraitValue, known int) input.DimensionSummary` — **paketintern, pure, keine I/O**

**Kontext:** Diese Task enthält **keinerlei** Datenbankzugriff. Sie ist reine Rechnung und muss auf 100 % Coverage kommen (`internal/application`-Floor). Gemessene Datenlage: EIVE (0–10) hat durchgängig Nischenbreiten zwischen 0,0798 und 10,0; Tichý (1–9 bzw. 1–12) und Midolo (0–2,63) haben keine.

- [ ] **Step 1: Die DTOs ergänzen**

In `internal/ports/input/services.go`:

```go
// DimensionSummary is one dimension's statistics over a species list.
//
// Absent fields are absent measurements, never zeroes: sd is omitted for a
// single value (a spread of zero is a claim; "cannot be computed" is a
// different one), and MeanNicheWeighted is omitted for vocabularies that
// carry no niche widths (Tichý, Midolo) so it can never be confused with
// the unweighted mean.
type DimensionSummary struct {
	Mean              float64  `json:"mean"`
	MeanNicheWeighted *float64 `json:"mean_niche_weighted,omitempty"`
	SD                *float64 `json:"sd,omitempty"`
	Min               float64  `json:"min"`
	Max               float64  `json:"max"`
	// N is how many species carried a value in this dimension, NMissing how
	// many of the known species did not. Per dimension, not global: EIVE
	// covers L/M/N/R/T unevenly, and a mean over 3 of 18 species is a
	// different statement than one over 17 of 18.
	N        int `json:"n"`
	NMissing int `json:"n_missing"`
	// NExcludedWeighted counts species left out of the weighted mean because
	// their niche width was not positive. Zero in the pinned data; reported
	// rather than silently repaired if a future ingest produces one.
	NExcludedWeighted int `json:"n_excluded_weighted,omitempty"`
}

// VocabSummary is one vocabulary's dimensions. Never merged across
// vocabularies: EIVE's 0–10 and Tichý's 1–12 are different scales, so their
// common mean would be an invented number.
type VocabSummary struct {
	VocabVersion string                      `json:"vocab_version"`
	Dimensions   map[string]DimensionSummary `json:"dimensions"`
}

// UnknownConcept is one concept id the index could not answer for, with the
// same two diagnoses the batch route uses.
type UnknownConcept struct {
	ConceptID string `json:"concept_id"`
	Reason    string `json:"reason"`
}

// TraitSummary is the indicator-value analysis over a species list.
type TraitSummary struct {
	Requested    int                     `json:"requested"`
	Known        int                     `json:"known"`
	Unknown      []UnknownConcept        `json:"unknown"`
	Vocabularies map[string]VocabSummary `json:"vocabularies"`
}
```

- [ ] **Step 2: Den fehlschlagenden Statistik-Test schreiben**

`internal/application/trait_summary_stats_test.go`:

```go
package application

import (
	"math"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func f64p(v float64) *float64 { return &v }

// tv builds one EIVE-shaped trait value; nw nil means "no niche width".
func tv(value float64, nw *float64) domain.TraitValue {
	return domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: value, NicheWidth: nw}
}

func TestSummarizeDimension_MeanMinMaxAndCounts(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{
		tv(4, nil), tv(6, nil), tv(8, nil),
	}, 5)

	if got.Mean != 6 {
		t.Errorf("Mean = %v, want 6", got.Mean)
	}
	if got.Min != 4 || got.Max != 8 {
		t.Errorf("Min/Max = %v/%v, want 4/8", got.Min, got.Max)
	}
	if got.N != 3 {
		t.Errorf("N = %d, want 3", got.N)
	}
	// 5 known species, 3 of them carried a value here.
	if got.NMissing != 2 {
		t.Errorf("NMissing = %d, want 2", got.NMissing)
	}
}

// Sample standard deviation (n-1), the right one for a vegetation record
// understood as a sample.
func TestSummarizeDimension_SampleStandardDeviation(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{tv(2, nil), tv(4, nil), tv(6, nil)}, 3)

	if got.SD == nil {
		t.Fatal("SD is nil, want a value for n=3")
	}
	if math.Abs(*got.SD-2) > 1e-9 {
		t.Errorf("SD = %v, want 2", *got.SD)
	}
}

// One value has no spread that can be computed. Reporting 0 would be a
// claim about the data that the data does not support.
func TestSummarizeDimension_SDAbsentForSingleValue(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{tv(5, nil)}, 1)

	if got.SD != nil {
		t.Errorf("SD = %v, want nil for n=1", *got.SD)
	}
	if got.Mean != 5 {
		t.Errorf("Mean = %v, want 5", got.Mean)
	}
}

// Weight is 1/niche_width: a narrow niche is the more precise indicator.
// Values 4 (width 1) and 8 (width 2) weigh 1 and 0.5, so the mean is
// (4*1 + 8*0.5) / 1.5 = 5.333...
func TestSummarizeDimension_NicheWeightedMean(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{
		tv(4, f64p(1)), tv(8, f64p(2)),
	}, 2)

	if got.MeanNicheWeighted == nil {
		t.Fatal("MeanNicheWeighted is nil, want a value when niche widths are present")
	}
	if math.Abs(*got.MeanNicheWeighted-16.0/3.0) > 1e-9 {
		t.Errorf("MeanNicheWeighted = %v, want %v", *got.MeanNicheWeighted, 16.0/3.0)
	}
	// The unweighted mean stays what it was.
	if got.Mean != 6 {
		t.Errorf("Mean = %v, want 6", got.Mean)
	}
}

// Tichý and Midolo carry no niche widths — the field must be ABSENT, never
// filled with the unweighted mean, or a client could not tell them apart.
func TestSummarizeDimension_NoWeightedMeanWithoutNicheWidths(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{tv(4, nil), tv(8, nil)}, 2)

	if got.MeanNicheWeighted != nil {
		t.Errorf("MeanNicheWeighted = %v, want nil when no value carries a niche width", *got.MeanNicheWeighted)
	}
}

// A non-positive niche width would divide by zero. Such a value is excluded
// and counted, never repaired with an invented substitute.
func TestSummarizeDimension_NonPositiveNicheWidthExcludedAndCounted(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{
		tv(4, f64p(1)), tv(8, f64p(0)), tv(6, f64p(-1)),
	}, 3)

	if got.NExcludedWeighted != 2 {
		t.Errorf("NExcludedWeighted = %d, want 2", got.NExcludedWeighted)
	}
	if got.MeanNicheWeighted == nil {
		t.Fatal("MeanNicheWeighted is nil; the one usable value must still produce a weighted mean")
	}
	if *got.MeanNicheWeighted != 4 {
		t.Errorf("MeanNicheWeighted = %v, want 4 (only the width-1 value counts)", *got.MeanNicheWeighted)
	}
	// All three still count for the unweighted statistics.
	if got.N != 3 {
		t.Errorf("N = %d, want 3", got.N)
	}
}

// Every niche width unusable: no weighted mean at all, but all of them
// counted as excluded.
func TestSummarizeDimension_AllNicheWidthsUnusable(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{tv(4, f64p(0)), tv(8, f64p(0))}, 2)

	if got.MeanNicheWeighted != nil {
		t.Errorf("MeanNicheWeighted = %v, want nil", *got.MeanNicheWeighted)
	}
	if got.NExcludedWeighted != 2 {
		t.Errorf("NExcludedWeighted = %d, want 2", got.NExcludedWeighted)
	}
}
```

- [ ] **Step 3: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/application/ -run TestSummarizeDimension -v`
Expected: FAIL — `summarizeDimension undefined`.

- [ ] **Step 4: Die Statistik implementieren**

`internal/application/trait_summary_stats.go`:

```go
package application

import (
	"math"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// summarizeDimension computes one dimension's statistics over the values of
// one vocabulary. Pure: no I/O, no repository, no context.
//
// known is how many of the requested concepts the index had ANY trait data
// for, so NMissing can say how many of them said nothing about THIS
// dimension. Callers pass it in rather than having it derived here, because
// only the caller knows the size of the species list.
//
// The caller guarantees values is non-empty and holds one vocabulary's
// values for one dimension.
func summarizeDimension(values []domain.TraitValue, known int) input.DimensionSummary {
	sum, min, max := 0.0, values[0].Value, values[0].Value
	for _, v := range values {
		sum += v.Value
		min = math.Min(min, v.Value)
		max = math.Max(max, v.Value)
	}
	n := len(values)
	mean := sum / float64(n)

	out := input.DimensionSummary{
		Mean: mean, Min: min, Max: max, N: n, NMissing: known - n,
	}
	if sd, ok := sampleSD(values, mean); ok {
		out.SD = &sd
	}
	weighted, excluded, ok := nicheWeightedMean(values)
	if ok {
		out.MeanNicheWeighted = &weighted
	}
	out.NExcludedWeighted = excluded
	return out
}

// sampleSD is the sample standard deviation (n-1). A single value has no
// computable spread — reporting 0 would state that the data agrees with
// itself, which is not what one measurement shows.
func sampleSD(values []domain.TraitValue, mean float64) (float64, bool) {
	if len(values) < 2 {
		return 0, false
	}
	sumSq := 0.0
	for _, v := range values {
		d := v.Value - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(values)-1)), true
}

// nicheWeightedMean weights each value by 1/niche_width: a narrow niche is a
// more precise site indicator than a generalist's broad one.
//
// Values without a niche width (Tichý, Midolo never carry one) contribute
// nothing and are NOT counted as excluded — the vocabulary simply does not
// offer the measure. A width that is present but not positive IS counted:
// that is a data defect worth seeing, and dividing by it would be a panic
// or an infinity. Measured on the pinned index this never occurs.
//
// ok is false when no value could be weighted at all; the caller then omits
// the field rather than substituting the unweighted mean.
func nicheWeightedMean(values []domain.TraitValue) (mean float64, excluded int, ok bool) {
	sumWeighted, sumWeights := 0.0, 0.0
	for _, v := range values {
		if v.NicheWidth == nil {
			continue
		}
		if *v.NicheWidth <= 0 {
			excluded++
			continue
		}
		w := 1 / *v.NicheWidth
		sumWeighted += v.Value * w
		sumWeights += w
	}
	if sumWeights == 0 {
		return 0, excluded, false
	}
	return sumWeighted / sumWeights, excluded, true
}
```

- [ ] **Step 5: Tests grün bekommen**

Run: `go test ./internal/application/ -run TestSummarizeDimension -v`
Expected: PASS (7 Tests)

- [ ] **Step 6: Coverage dieser Datei prüfen**

Run: `go test ./internal/application/ -coverprofile=/tmp/c.out && go tool cover -func=/tmp/c.out | grep trait_summary_stats`
Expected: jede Funktion 100.0 % — `internal/application` hat einen 100-%-Floor. Fehlt ein Zweig, fehlt ein Test.

- [ ] **Step 7: `make verify` und committen**

```bash
make verify
git add internal/application/trait_summary_stats.go \
        internal/application/trait_summary_stats_test.go \
        internal/ports/input/services.go
git commit -m "feat(application): pure indicator-value statistics over a species list"
```

---

### Task 4: Zeigerwertanalyse — Datenzugriff für mehrere Konzepte

**Files:**
- Modify: `internal/adapters/sqlite/trait.go`
- Modify: `internal/adapters/sqlite/trait_test.go`
- Modify: `internal/ports/output/repository.go`
- Modify: `internal/application/ingest_test.go` (fakeRepo)

**Interfaces:**
- Consumes: `domain.TraitValue` und die bestehende `trait_value`-Tabelle (Spalten `concept_id, vocab, vocab_version, dim, value, niche_width, n_systems`).
- Produces: `output.Repository.TraitsForConcepts(ctx context.Context, conceptIDs []string) (map[string][]domain.TraitValue, error)` — Schlüssel ist die Concept-ID; Konzepte ohne Zeigerwerte fehlen in der Map.

**Kontext:** Die bestehende `Traits(ctx, conceptID, vocabs)` liest **ein** Konzept. Eine Analyse über 18 Arten würde daraus 18 Einzelabfragen machen. `chunkSize` in `read.go` (`areaChunkSize = 500`) zeigt das etablierte Muster für gebundene `IN (…)`-Listen; sqlite begrenzt Platzhalter pro Statement.

- [ ] **Step 1: Den Port erweitern**

In `internal/ports/output/repository.go`, nach `Traits`:

```go
	// TraitsForConcepts returns every trait value the index holds for each
	// of conceptIDs, keyed by concept id. Concepts without any trait data
	// are absent from the map rather than present-but-empty: "no data" and
	// "an empty list of data" are the same fact, and one representation is
	// enough.
	//
	// One read for the whole list, not one per concept: an analysis over a
	// vegetation record asks about dozens of species at once.
	TraitsForConcepts(ctx context.Context, conceptIDs []string) (map[string][]domain.TraitValue, error)
```

- [ ] **Step 2: Den fehlschlagenden Test schreiben**

In `internal/adapters/sqlite/trait_test.go` ergänzen:

```go
func TestTraitsForConcepts_GroupsByConceptAndOmitsUnknownOnes(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTraitVocabulary(domain.TraitVocabulary{Vocab: "eive", VocabVersion: "1.0"}); err != nil {
		t.Fatalf("UpsertTraitVocabulary: %v", err)
	}
	nw := 2.5
	for conceptID, values := range map[string][]domain.TraitValue{
		"wcvp:concept:1": {
			{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.2, NicheWidth: &nw},
			{Vocab: "eive", VocabVersion: "1.0", Dim: "N", Value: 3.1, NicheWidth: &nw},
		},
		"wcvp:concept:2": {
			{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 6.8, NicheWidth: &nw},
		},
	} {
		for _, v := range values {
			if err := tx.UpsertTraitValue(conceptID, v); err != nil {
				t.Fatalf("UpsertTraitValue: %v", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.TraitsForConcepts(context.Background(),
		[]string{"wcvp:concept:1", "wcvp:concept:2", "wcvp:concept:999"})
	if err != nil {
		t.Fatalf("TraitsForConcepts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d concepts, want 2 (the unknown one must be absent, not empty)", len(got))
	}
	if len(got["wcvp:concept:1"]) != 2 {
		t.Errorf("concept 1 has %d values, want 2", len(got["wcvp:concept:1"]))
	}
	if _, present := got["wcvp:concept:999"]; present {
		t.Error("the unknown concept is present in the map; it must be absent")
	}
	if got["wcvp:concept:2"][0].NicheWidth == nil {
		t.Error("niche width was lost on the way out")
	}
}

func TestTraitsForConcepts_EmptyInputIsEmptyMapNotError(t *testing.T) {
	db := openTestDB(t)

	got, err := db.TraitsForConcepts(context.Background(), nil)
	if err != nil {
		t.Fatalf("TraitsForConcepts: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d entries, want none", len(got))
	}
}
```

Hinweis: Die Namen der Seed-Helfer (`openTestDB`, `UpsertTraitVocabulary`, `UpsertTraitValue`) aus der vorhandenen `trait_test.go` übernehmen, falls sie dort abweichen.

- [ ] **Step 3: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/adapters/sqlite/ -run TestTraitsForConcepts -v`
Expected: FAIL — `db.TraitsForConcepts undefined`.

- [ ] **Step 4: Implementieren**

In `internal/adapters/sqlite/trait.go` ergänzen:

```go
// traitChunkSize bounds how many concept ids go into one IN (...) list, for
// the same reason areaChunkSize does in read.go: sqlite caps bound
// parameters per statement, and an analysis may ask about hundreds of ids.
const traitChunkSize = 500

// TraitsForConcepts implements output.Repository.
func (d *DB) TraitsForConcepts(ctx context.Context, conceptIDs []string) (map[string][]domain.TraitValue, error) {
	out := map[string][]domain.TraitValue{}
	for start := 0; start < len(conceptIDs); start += traitChunkSize {
		end := start + traitChunkSize
		if end > len(conceptIDs) {
			end = len(conceptIDs)
		}
		if err := d.appendTraitsForChunk(ctx, conceptIDs[start:end], out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// appendTraitsForChunk reads one bounded chunk into out.
func (d *DB) appendTraitsForChunk(ctx context.Context, chunk []string, out map[string][]domain.TraitValue) error {
	// Only placeholders are generated, never values — the ids stay bound
	// arguments, so this is not SQL built from input (gosec G201/G202).
	placeholders := strings.Repeat(",?", len(chunk))[1:]
	args := make([]any, 0, len(chunk))
	for _, id := range chunk {
		args = append(args, id)
	}

	rows, err := d.QueryContext(ctx,
		`SELECT concept_id, vocab, vocab_version, dim, value, niche_width, n_systems
		 FROM trait_value WHERE concept_id IN (`+placeholders+`)
		 ORDER BY concept_id, vocab, dim`, args...)
	if err != nil {
		return fmt.Errorf("sqlite: reading traits for %d concepts: %w", len(chunk), err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var conceptID, vocab, vocabVersion, dim string
		var tv domain.TraitValue
		if err := rows.Scan(&conceptID, &vocab, &vocabVersion, &dim,
			&tv.Value, &tv.NicheWidth, &tv.NSystems); err != nil {
			return fmt.Errorf("sqlite: scanning trait value: %w", err)
		}
		tv.Vocab, tv.VocabVersion, tv.Dim = vocab, vocabVersion, domain.TraitDim(dim)
		out[conceptID] = append(out[conceptID], tv)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: iterating trait values: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Tests grün bekommen**

Run: `go test ./internal/adapters/sqlite/ -run TestTraitsForConcepts -v`
Expected: PASS (2 Tests)

- [ ] **Step 6: Den fakeRepo nachziehen**

In `internal/application/ingest_test.go`:

```go
func (r *fakeRepo) TraitsForConcepts(_ context.Context, conceptIDs []string) (map[string][]domain.TraitValue, error) {
	if r.traitsForConceptsErr != nil {
		return nil, r.traitsForConceptsErr
	}
	out := map[string][]domain.TraitValue{}
	for _, id := range conceptIDs {
		if vs, ok := r.conceptTraits[id]; ok {
			out[id] = vs
		}
	}
	return out, nil
}
```

Mit den Feldern in `fakeRepo`:

```go
	conceptTraits        map[string][]domain.TraitValue
	traitsForConceptsErr error
```

- [ ] **Step 7: Ganze Suite und `make verify`, dann committen**

```bash
go test ./...
make verify
git add internal/adapters/sqlite/trait.go internal/adapters/sqlite/trait_test.go \
        internal/ports/output/repository.go internal/application/ingest_test.go
git commit -m "feat(sqlite): TraitsForConcepts reads many concepts in one query"
```

---

### Task 5: Zeigerwertanalyse — Use-Case, Handler, Route, OpenAPI

**Files:**
- Create: `internal/application/trait_summary.go`
- Create: `internal/application/trait_summary_test.go`
- Create: `internal/adapters/http/trait_summary.go`
- Create: `internal/adapters/http/trait_summary_test.go`
- Modify: `internal/ports/input/services.go`
- Modify: `internal/adapters/http/server.go`
- Modify: `internal/adapters/http/openapi.yaml`, `api/openapi/openapi.yaml`

**Interfaces:**
- Consumes: `summarizeDimension(values []domain.TraitValue, known int) input.DimensionSummary` (Task 3); `output.Repository.TraitsForConcepts(ctx, conceptIDs) (map[string][]domain.TraitValue, error)` (Task 4); `input.TraitSummary`, `input.VocabSummary`, `input.DimensionSummary`, `input.UnknownConcept` (Task 3); `input.ReasonUnknownBackbone`, `input.ReasonUnknownConcept`; `indexBackbone` aus `internal/application/query.go`.
- Produces: `input.QueryService.SpeciesTraitSummary(ctx context.Context, conceptIDs []string) (input.TraitSummary, error)`; Route `POST /v1/species/traits/summary`.

- [ ] **Step 1: Das Interface erweitern**

In `internal/ports/input/services.go`, im `QueryService`:

```go
	// SpeciesTraitSummary aggregates the indicator values of a species list,
	// strictly per vocabulary and dimension — never across vocabularies,
	// whose scales differ.
	SpeciesTraitSummary(ctx context.Context, conceptIDs []string) (TraitSummary, error)
```

- [ ] **Step 2: Den fehlschlagenden Test schreiben**

`internal/application/trait_summary_test.go`:

```go
package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

func seedTraitSummaryRepo() *fakeRepo {
	repo := newFakeRepo()
	nw1, nw2 := 1.0, 2.0
	repo.conceptTraits = map[string][]domain.TraitValue{
		"wcvp:concept:1": {
			{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4, NicheWidth: &nw1},
			{Vocab: "tichy2023", VocabVersion: "2.0", Dim: "M", Value: 5},
		},
		"wcvp:concept:2": {
			{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 8, NicheWidth: &nw2},
		},
	}
	return repo
}

func TestSpeciesTraitSummary_GroupsPerVocabularyAndDimension(t *testing.T) {
	got, err := NewQueryService(seedTraitSummaryRepo()).SpeciesTraitSummary(
		context.Background(), []string{"wcvp:concept:1", "wcvp:concept:2"})
	if err != nil {
		t.Fatalf("SpeciesTraitSummary: %v", err)
	}
	if got.Requested != 2 || got.Known != 2 {
		t.Errorf("Requested/Known = %d/%d, want 2/2", got.Requested, got.Known)
	}
	eive, ok := got.Vocabularies["eive"]
	if !ok {
		t.Fatal("no eive entry")
	}
	if eive.VocabVersion != "1.0" {
		t.Errorf("VocabVersion = %q, want 1.0", eive.VocabVersion)
	}
	m := eive.Dimensions["M"]
	if m.Mean != 6 {
		t.Errorf("eive M mean = %v, want 6", m.Mean)
	}
	// Tichý's M is a different scale and must be its own entry.
	if got.Vocabularies["tichy2023"].Dimensions["M"].Mean != 5 {
		t.Errorf("tichy M mean = %v, want 5 (never merged with eive)",
			got.Vocabularies["tichy2023"].Dimensions["M"].Mean)
	}
}

// Only EIVE carries niche widths, so only EIVE may report a weighted mean.
func TestSpeciesTraitSummary_WeightedMeanOnlyWhereNicheWidthsExist(t *testing.T) {
	got, err := NewQueryService(seedTraitSummaryRepo()).SpeciesTraitSummary(
		context.Background(), []string{"wcvp:concept:1", "wcvp:concept:2"})
	if err != nil {
		t.Fatalf("SpeciesTraitSummary: %v", err)
	}
	if got.Vocabularies["eive"].Dimensions["M"].MeanNicheWeighted == nil {
		t.Error("eive M has no weighted mean, want one")
	}
	if got.Vocabularies["tichy2023"].Dimensions["M"].MeanNicheWeighted != nil {
		t.Error("tichy M reports a weighted mean; it has no niche widths at all")
	}
}

func TestSpeciesTraitSummary_ReportsUnknownConceptsWithReasons(t *testing.T) {
	got, err := NewQueryService(seedTraitSummaryRepo()).SpeciesTraitSummary(
		context.Background(), []string{"wcvp:concept:1", "wcvp:concept:999", "gbif:concept:7"})
	if err != nil {
		t.Fatalf("SpeciesTraitSummary: %v", err)
	}
	if got.Requested != 3 || got.Known != 1 {
		t.Errorf("Requested/Known = %d/%d, want 3/1", got.Requested, got.Known)
	}
	if len(got.Unknown) != 2 {
		t.Fatalf("got %d unknown entries, want 2", len(got.Unknown))
	}
	byID := map[string]string{}
	for _, u := range got.Unknown {
		byID[u.ConceptID] = u.Reason
	}
	if byID["wcvp:concept:999"] != input.ReasonUnknownConcept {
		t.Errorf("reason for the right-backbone id = %q, want %q",
			byID["wcvp:concept:999"], input.ReasonUnknownConcept)
	}
	if byID["gbif:concept:7"] != input.ReasonUnknownBackbone {
		t.Errorf("reason for the foreign-backbone id = %q, want %q",
			byID["gbif:concept:7"], input.ReasonUnknownBackbone)
	}
}

func TestSpeciesTraitSummary_EmptyListIsInvalidQuery(t *testing.T) {
	_, err := NewQueryService(seedTraitSummaryRepo()).SpeciesTraitSummary(context.Background(), nil)
	if !errors.Is(err, input.ErrInvalidQuery) {
		t.Errorf("error = %v, want ErrInvalidQuery", err)
	}
}

// Nothing resolvable is a normal answer, not a failure: empty vocabularies,
// every id reported back.
func TestSpeciesTraitSummary_AllUnknownIsEmptyNotError(t *testing.T) {
	got, err := NewQueryService(seedTraitSummaryRepo()).SpeciesTraitSummary(
		context.Background(), []string{"wcvp:concept:999"})
	if err != nil {
		t.Fatalf("SpeciesTraitSummary: %v", err)
	}
	if len(got.Vocabularies) != 0 {
		t.Errorf("got %d vocabularies, want none", len(got.Vocabularies))
	}
	if len(got.Unknown) != 1 {
		t.Errorf("got %d unknown entries, want 1", len(got.Unknown))
	}
}

func TestSpeciesTraitSummary_RepositoryErrorIsWrapped(t *testing.T) {
	repo := seedTraitSummaryRepo()
	repo.traitsForConceptsErr = errors.New("disk on fire")

	_, err := NewQueryService(repo).SpeciesTraitSummary(context.Background(), []string{"wcvp:concept:1"})
	if err == nil {
		t.Fatal("want an error when the repository fails")
	}
}

// A dimension where no species carried a value must be absent, not a row of
// zeroes.
func TestSpeciesTraitSummary_DimensionWithoutValuesIsAbsent(t *testing.T) {
	got, err := NewQueryService(seedTraitSummaryRepo()).SpeciesTraitSummary(
		context.Background(), []string{"wcvp:concept:2"})
	if err != nil {
		t.Fatalf("SpeciesTraitSummary: %v", err)
	}
	if _, present := got.Vocabularies["eive"].Dimensions["N"]; present {
		t.Error("dimension N is present although no species carried a value for it")
	}
}
```

- [ ] **Step 3: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/application/ -run TestSpeciesTraitSummary -v`
Expected: FAIL — `SpeciesTraitSummary undefined`.

- [ ] **Step 4: Den Use-Case implementieren**

`internal/application/trait_summary.go`:

```go
package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// SpeciesTraitSummary aggregates a species list's indicator values, strictly
// per vocabulary and dimension.
//
// Never across vocabularies: EIVE's 0–10 and Tichý's 1–12 measure different
// things on different scales, so a common mean would be an invented number.
func (q *QueryService) SpeciesTraitSummary(ctx context.Context, conceptIDs []string) (input.TraitSummary, error) {
	if len(conceptIDs) == 0 {
		return input.TraitSummary{}, fmt.Errorf("concept_ids must hold at least one id: %w", input.ErrInvalidQuery)
	}

	// A foreign backbone is the caller's fault and needs no index read; the
	// rest is asked about in one query.
	wanted := make([]string, 0, len(conceptIDs))
	unknown := []input.UnknownConcept{}
	for _, id := range conceptIDs {
		if !strings.HasPrefix(id, indexBackbone+":") {
			unknown = append(unknown, input.UnknownConcept{ConceptID: id, Reason: input.ReasonUnknownBackbone})
			continue
		}
		wanted = append(wanted, id)
	}

	traits := map[string][]domain.TraitValue{}
	if len(wanted) > 0 {
		var err error
		traits, err = q.repo.TraitsForConcepts(ctx, wanted)
		if err != nil {
			return input.TraitSummary{}, fmt.Errorf("reading traits of %d concepts: %w", len(wanted), err)
		}
	}
	for _, id := range wanted {
		if _, ok := traits[id]; !ok {
			unknown = append(unknown, input.UnknownConcept{ConceptID: id, Reason: input.ReasonUnknownConcept})
		}
	}

	return input.TraitSummary{
		Requested:    len(conceptIDs),
		Known:        len(traits),
		Unknown:      unknown,
		Vocabularies: summarizeVocabularies(traits),
	}, nil
}

// summarizeVocabularies buckets every value by (vocab, dim) and summarizes
// each bucket. A dimension nobody carried a value for never gets a bucket,
// so it is absent from the answer rather than a row of zeroes.
func summarizeVocabularies(traits map[string][]domain.TraitValue) map[string]input.VocabSummary {
	type bucket struct {
		version string
		byDim   map[string][]domain.TraitValue
	}
	buckets := map[string]*bucket{}
	for _, values := range traits {
		for _, v := range values {
			b, ok := buckets[v.Vocab]
			if !ok {
				b = &bucket{version: v.VocabVersion, byDim: map[string][]domain.TraitValue{}}
				buckets[v.Vocab] = b
			}
			b.byDim[string(v.Dim)] = append(b.byDim[string(v.Dim)], v)
		}
	}

	known := len(traits)
	out := make(map[string]input.VocabSummary, len(buckets))
	for vocab, b := range buckets {
		dims := make(map[string]input.DimensionSummary, len(b.byDim))
		for dim, values := range b.byDim {
			dims[dim] = summarizeDimension(values, known)
		}
		out[vocab] = input.VocabSummary{VocabVersion: b.version, Dimensions: dims}
	}
	return out
}
```

- [ ] **Step 5: Use-Case-Tests grün bekommen**

Run: `go test ./internal/application/ -run TestSpeciesTraitSummary -v`
Expected: PASS (7 Tests)

- [ ] **Step 6: Den `fakeQueryService` um die neue Methode erweitern**

Wie in Task 2: `QueryService` ist in Step 1 gewachsen, also erfüllt der
`fakeQueryService` in `internal/adapters/http/handlers_test.go` es nicht mehr
und **das ganze http-Testpaket kompiliert nicht**. Dort ergänzen:

```go
func (f *fakeQueryService) SpeciesTraitSummary(_ context.Context, conceptIDs []string) (input.TraitSummary, error) {
	if f.traitSummaryErr != nil {
		return input.TraitSummary{}, f.traitSummaryErr
	}
	if len(conceptIDs) == 0 {
		return input.TraitSummary{}, fmt.Errorf("concept_ids must hold at least one id: %w", input.ErrInvalidQuery)
	}
	summary := input.TraitSummary{
		Requested:    len(conceptIDs),
		Unknown:      []input.UnknownConcept{},
		Vocabularies: map[string]input.VocabSummary{},
	}
	for _, id := range conceptIDs {
		if values, ok := f.traitSummaries[id]; ok {
			summary.Known++
			summary.Vocabularies = values
			continue
		}
		summary.Unknown = append(summary.Unknown,
			input.UnknownConcept{ConceptID: id, Reason: input.ReasonUnknownConcept})
	}
	return summary, nil
}
```

Mit den Feldern in der Struktur:

```go
	traitSummaries  map[string]map[string]input.VocabSummary
	traitSummaryErr error
```

Und in `seededQueryService()` einen Eintrag für die Concept-ID setzen, die
der Handler-Test abfragt (`wcvp:concept:83891`), damit `"vocabularies"` in
der Antwort nicht leer ist.

- [ ] **Step 7: Den fehlschlagenden Handler-Test schreiben**

`internal/adapters/http/trait_summary_test.go`:

```go
package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTraitSummaryRoute_AnswersWithPerVocabularyStatistics(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	body := strings.NewReader(`{"concept_ids":["wcvp:concept:83891"]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/species/traits/summary", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"vocabularies"`) {
		t.Errorf("body = %s, want a vocabularies object", rec.Body)
	}
}

func TestTraitSummaryRoute_EmptyListIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	req := httptest.NewRequest(http.MethodPost, "/v1/species/traits/summary",
		strings.NewReader(`{"concept_ids":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestTraitSummaryRoute_MalformedBodyIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	req := httptest.NewRequest(http.MethodPost, "/v1/species/traits/summary",
		strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// Two concatenated objects must not decode as the first one silently.
func TestTraitSummaryRoute_TrailingJSONIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	req := httptest.NewRequest(http.MethodPost, "/v1/species/traits/summary",
		strings.NewReader(`{"concept_ids":["wcvp:concept:1"]}{"concept_ids":["wcvp:concept:2"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for trailing JSON", rec.Code)
	}
}

func TestTraitSummaryRoute_UnknownConceptIs200WithReason(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	req := httptest.NewRequest(http.MethodPost, "/v1/species/traits/summary",
		strings.NewReader(`{"concept_ids":["wcvp:concept:999999999"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — an unknown id is a normal answer", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unknown_concept") {
		t.Errorf("body = %s, want the unknown_concept reason", rec.Body)
	}
}
```

- [ ] **Step 8: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/adapters/http/ -run TestTraitSummaryRoute -v`
Expected: FAIL — 404/405, die Route fehlt noch.

- [ ] **Step 9: Den Handler implementieren**

`internal/adapters/http/trait_summary.go`:

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// handleSpeciesTraitSummary answers the indicator-value analysis.
//
// Body handling mirrors handleSpeciesBatch exactly — same shape, same
// limits, same refusal to silently swallow a second JSON object — because a
// caller that learned one of the two routes must not be surprised by the
// other.
func (s *Server) handleSpeciesTraitSummary(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBatchBodyBytes))
	dec.DisallowUnknownFields()
	var req batchRequest
	if err := dec.Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			"request body must be {\"concept_ids\":[...]}")
		return
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			"request body must hold exactly one JSON object")
		return
	}
	if len(req.ConceptIDs) > maxBatchConceptIDs {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			fmt.Sprintf("concept_ids holds %d entries, at most %d are accepted",
				len(req.ConceptIDs), maxBatchConceptIDs))
		return
	}

	asked := make([]string, 0, len(req.ConceptIDs))
	for i, id := range req.ConceptIDs {
		if id = strings.TrimSpace(id); id == "" {
			s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
				fmt.Sprintf("concept_ids[%d] is empty; every entry must be a concept id", i))
			return
		}
		asked = append(asked, id)
	}

	summary, err := s.deps.Query.SpeciesTraitSummary(r.Context(), asked)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, summary)
}
```

- [ ] **Step 10: Die Route mounten**

In `internal/adapters/http/server.go`, bei den `/v1`-Routen:

```go
	r.HandleFunc("/v1/species/traits/summary", s.handleSpeciesTraitSummary).Methods(http.MethodPost)
```

- [ ] **Step 11: OpenAPI in BEIDEN Kopien dokumentieren**

In `internal/adapters/http/openapi.yaml` unter `paths:`:

```yaml
  /v1/species/traits/summary:
    post:
      summary: Zeigerwertanalyse über eine Artenliste
      description: >-
        Mittelt die Zeigerwerte einer Artenliste — **strikt je Vokabular und
        Dimension**, niemals darüber hinweg: EIVE (0–10) und Tichý (1–12)
        sind verschiedene Skalen, ihr gemeinsames Mittel wäre eine erfundene
        Zahl. Neben dem arithmetischen Mittel wird, wo Nischenbreiten
        vorliegen (nur EIVE), ein mit `1/Nischenbreite` gewichtetes Mittel
        ausgewiesen: eine schmale Nische ist der präzisere Standortanzeiger.
        Beide stehen nebeneinander, damit die Methodenwahl beim Auswertenden
        bleibt. Unbekannte Concept-IDs sind ein normales 200 mit Begründung,
        kein Fehler.
      operationId: speciesTraitSummary
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
                  minItems: 1
                  maxItems: 300
                  items:
                    type: string
                  example: ["wcvp:concept:83891", "wcvp:concept:2692970"]
      responses:
        "200":
          description: Die Analyse.
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/TraitSummary"
        "400":
          $ref: "#/components/responses/InvalidQuery"
```

Unter `components.schemas`:

```yaml
    TraitSummary:
      type: object
      required: [requested, known, unknown, vocabularies]
      properties:
        requested:
          type: integer
          description: Zahl der angefragten Concept-IDs.
        known:
          type: integer
          description: Zahl der Konzepte, zu denen der Index Zeigerwerte führt.
        unknown:
          type: array
          items:
            $ref: "#/components/schemas/UnknownConcept"
        vocabularies:
          type: object
          description: Vokabular-ID -> Auswertung. Leer, wenn nichts auflösbar war.
          additionalProperties:
            $ref: "#/components/schemas/VocabSummary"
    UnknownConcept:
      type: object
      required: [concept_id, reason]
      properties:
        concept_id:
          type: string
        reason:
          type: string
          enum: [unknown_backbone, unknown_concept]
          description: >-
            `unknown_backbone`: das ID-Präfix passt nicht zum Index (Fehler
            des Aufrufers). `unknown_concept`: richtiges Präfix, aber keine
            Zeigerwerte vorhanden (Grenze der Daten).
    VocabSummary:
      type: object
      required: [vocab_version, dimensions]
      properties:
        vocab_version:
          type: string
          example: "1.0"
        dimensions:
          type: object
          description: Dimensions-Kürzel -> Statistik. Dimensionen ohne einen einzigen Wert fehlen.
          additionalProperties:
            $ref: "#/components/schemas/DimensionSummary"
    DimensionSummary:
      type: object
      required: [mean, min, max, n, n_missing]
      properties:
        mean:
          type: number
          description: Arithmetisches Mittel über alle Arten mit Wert in dieser Dimension.
        mean_niche_weighted:
          type: number
          description: >-
            Mit `1/Nischenbreite` gewichtetes Mittel. **Fehlt** bei
            Vokabularen ohne Nischenbreiten (Tichý, Midolo) — es wird nie mit
            dem ungewichteten Mittel gefüllt.
        sd:
          type: number
          description: Stichproben-Standardabweichung (n-1). **Fehlt** bei n < 2.
        min:
          type: number
        max:
          type: number
        n:
          type: integer
          description: Arten mit Wert in dieser Dimension.
        n_missing:
          type: integer
          description: Bekannte Arten ohne Wert in dieser Dimension.
        n_excluded_weighted:
          type: integer
          description: >-
            Arten, die aus dem gewichteten Mittel fielen, weil ihre
            Nischenbreite nicht positiv war. Fehlt, wenn keine betroffen war.
```

Dann kopieren:

```bash
cp internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
```

- [ ] **Step 12: Alle Tests grün bekommen**

Run: `go test ./...`
Expected: PASS, inklusive `TestRoutesMatchOpenAPISpec` und `TestOpenAPICopiesAreIdentical`.

- [ ] **Step 13: `make verify` und committen**

```bash
make verify
git add internal/application/trait_summary.go internal/application/trait_summary_test.go \
        internal/adapters/http/trait_summary.go internal/adapters/http/trait_summary_test.go \
        internal/ports/input/services.go internal/adapters/http/server.go \
        internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
git commit -m "feat(http): POST /v1/species/traits/summary — indicator-value analysis"
```

---

### Task 6: Die Explorer-Seite unter `GET /`

**Files:**
- Create: `internal/adapters/http/explorer.html`
- Create: `internal/adapters/http/explorer.go`
- Create: `internal/adapters/http/explorer_test.go`
- Modify: `internal/adapters/http/server.go`
- Modify: `internal/adapters/http/openapi.yaml`, `api/openapi/openapi.yaml`

**Interfaces:**
- Consumes: alle Routen aus Tasks 2 und 5 sowie die bestehenden Lese-Routen.
- Produces: Route `GET /`.

**Kontext:** `internal/adapters/http/docs.go` zeigt das Muster: `//go:embed` in ein `[]byte`, `Content-Type: text/html; charset=utf-8`, Schreibfehler werden geloggt, nicht ignoriert. Anders als `/docs` braucht die Explorer-Seite **keine** `fmt.Appendf`-Komposition — sie ist eine statische Datei und wird direkt ausgeliefert.

- [ ] **Step 1: Die HTML-Seite anlegen**

`internal/adapters/http/explorer.html` — eine vollständige, selbst-enthaltene Seite. Aufbau:

```html
<!doctype html>
<html lang="de">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>situs — Explorer</title>
<link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16'%3E%3Ctext y='13' font-size='13'%3E%F0%9F%8C%B1%3C/text%3E%3C/svg%3E">
<style>
  :root { --fg:#1a1a1a; --bg:#fdfdfc; --muted:#666; --line:#ddd; --accent:#2d6a4f; --warn:#9a3412; }
  @media (prefers-color-scheme: dark) {
    :root { --fg:#e8e8e6; --bg:#16181a; --muted:#9aa0a6; --line:#333; --accent:#74c69d; --warn:#fb923c; }
  }
  * { box-sizing: border-box; }
  body { margin:0; padding:1.5rem; font:15px/1.5 system-ui,-apple-system,sans-serif;
         color:var(--fg); background:var(--bg); max-width:1100px; margin-inline:auto; }
  h1 { font-size:1.4rem; margin:0 0 .25rem; }
  h2 { font-size:1rem; margin:0 0 .75rem; color:var(--accent); }
  .sub { color:var(--muted); margin:0 0 1.5rem; font-size:.9rem; }
  .globals { display:flex; gap:1rem; flex-wrap:wrap; align-items:end;
             padding:.75rem 1rem; border:1px solid var(--line); border-radius:6px; margin-bottom:1.5rem; }
  .panel { border:1px solid var(--line); border-radius:6px; padding:1rem; margin-bottom:1rem; }
  label { display:block; font-size:.8rem; color:var(--muted); margin-bottom:.2rem; }
  input, select, textarea, button { font:inherit; padding:.4rem .6rem; border:1px solid var(--line);
                                    border-radius:4px; background:var(--bg); color:var(--fg); }
  textarea { width:100%; min-height:5rem; font-family:ui-monospace,monospace; font-size:.85rem; }
  button { cursor:pointer; background:var(--accent); color:#fff; border-color:var(--accent); }
  button.secondary { background:transparent; color:var(--muted); border-color:var(--line); }
  .row { display:flex; gap:.5rem; align-items:end; flex-wrap:wrap; margin-bottom:.75rem; }
  .url { font-family:ui-monospace,monospace; font-size:.8rem; color:var(--muted);
         word-break:break-all; margin:.5rem 0; }
  pre { background:rgba(128,128,128,.08); padding:.75rem; border-radius:4px; overflow:auto;
        max-height:28rem; font-size:.82rem; margin:0; }
  table { border-collapse:collapse; width:100%; font-size:.85rem; }
  th, td { text-align:left; padding:.3rem .5rem; border-bottom:1px solid var(--line); }
  th { color:var(--muted); font-weight:600; }
  .hits { list-style:none; padding:0; margin:.5rem 0 0; max-height:12rem; overflow:auto; }
  .hits li { padding:.3rem .5rem; cursor:pointer; border-radius:3px; }
  .hits li:hover { background:rgba(128,128,128,.12); }
  .hits li.unresolved { color:var(--warn); cursor:not-allowed; }
  .note { font-size:.8rem; color:var(--muted); margin-top:.5rem; }
</style>
</head>
<body>
<h1>situs — Explorer</h1>
<p class="sub">
  Jedes Panel spricht genau einen Endpunkt an und zeigt die abgesetzte URL mit.
  Es wird nichts berechnet oder zusammengefasst, was der Dienst nicht selbst
  liefert.
</p>

<div class="globals">
  <div>
    <label for="g-lang">lang</label>
    <select id="g-lang"><option value="">—</option><option value="de">de</option></select>
  </div>
  <div>
    <label for="g-area">area (WGSRPD L3)</label>
    <input id="g-area" size="6" placeholder="GER">
  </div>
  <div>
    <label><input type="checkbox" id="g-onlyarea"> only_in_area</label>
  </div>
  <div id="info" class="url"></div>
</div>

<!-- Panels folgen: jeweils section.panel mit h2, Eingaben, .url, pre -->
</body>
</html>
```

Die acht Panels als `<section class="panel">` ergänzen. Jedes bekommt:
- eine `<h2>` mit dem Endpunktnamen,
- seine Eingabefelder,
- einen Knopf „Abfragen",
- ein `<div class="url">` für die abgesetzte URL,
- ein `<pre>` für die Antwort,
- einen Knopf „formatiert ⇄ JSON".

Das gemeinsame JS am Seitenende:

```html
<script>
// One helper for every panel: run the request, show the URL that was
// actually sent, render the response. No panel builds its own fetch logic.
async function call(panel, url, init) {
  const box = document.querySelector(`#${panel} .url`);
  const out = document.querySelector(`#${panel} pre`);
  box.textContent = (init && init.method === "POST" ? "POST " : "GET ") + url;
  out.textContent = "…";
  try {
    const res = await fetch(url, init);
    const body = await res.json();
    out.dataset.raw = JSON.stringify(body, null, 2);
    out.textContent = out.dataset.raw;
    if (!res.ok) { box.textContent += `  → HTTP ${res.status}`; }
  } catch (err) {
    out.textContent = "Anfrage fehlgeschlagen: " + err;
  }
}

// The global switches, appended by every panel that supports them.
function globals(params) {
  const lang = document.getElementById("g-lang").value;
  const area = document.getElementById("g-area").value.trim();
  if (lang) params.set("lang", lang);
  if (area) params.set("area", area);
  if (area && document.getElementById("g-onlyarea").checked) params.set("only_in_area", "true");
  return params;
}

// Name search, shared by every panel that takes a concept id. A hit without
// a concept id is shown but not selectable — the index knows the name, it
// just could not resolve it.
async function search(inputEl, listEl, targetEl) {
  const q = inputEl.value.trim();
  if (!q) { listEl.innerHTML = ""; return; }
  const res = await fetch(`/v1/species/search?q=${encodeURIComponent(q)}`);
  const hits = await res.json();
  listEl.innerHTML = "";
  for (const h of hits) {
    const li = document.createElement("li");
    if (h.concept_id) {
      li.textContent = `${h.verbatim_name} — ${h.concept_id}`;
      li.onclick = () => { targetEl.value = h.concept_id; listEl.innerHTML = ""; };
    } else {
      li.textContent = `${h.verbatim_name} — nicht aufgelöst`;
      li.className = "unresolved";
      li.title = "Der Index kennt diesen Namen, konnte ihn aber keiner Concept-ID zuordnen.";
    }
    listEl.appendChild(li);
  }
}

// Index self-description in the header, so one always sees what is queried.
fetch("/v1/info").then(r => r.json()).then(i => {
  document.getElementById("info").textContent =
    `${i.service} ${i.version} · ${i.index.species_with_concept} Konzepte · ` +
    `${i.index.areas_with_data} Gebiete · Backbones: ${i.index.concept_backbones.join(", ")}`;
});
</script>
```

**Zwingend:** kein `<script src="http…">`, kein `<link href="http…">`, keine Web-Font-URL. Die Seite muss ohne Netz vollständig funktionieren.

- [ ] **Step 2: Den fehlschlagenden Test schreiben**

`internal/adapters/http/explorer_test.go`:

```go
package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestExplorerRoute_ServesHTMLAtRoot(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Error("body does not look like an HTML document")
	}
}

// The page must work in the field, offline. A single remote asset would
// leave it blank exactly when it is needed.
func TestExplorerPage_LoadsNoRemoteAssets(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	remote := regexp.MustCompile(`(?i)(src|href)\s*=\s*["']https?://`)
	if loc := remote.FindString(rec.Body.String()); loc != "" {
		t.Errorf("page references a remote asset (%q); it must be fully self-contained", loc)
	}
}

// / must not swallow unknown paths — a typo'd API path has to stay a 404.
func TestExplorerRoute_DoesNotActAsCatchAll(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/nonexistent", nil))

	if rec.Code == http.StatusOK {
		t.Errorf("status = 200 for an unknown path; / is acting as a catch-all")
	}
}
```

- [ ] **Step 3: Test laufen lassen, Fehlschlag bestätigen**

Run: `go test ./internal/adapters/http/ -run TestExplorer -v`
Expected: FAIL — 404 für `/`.

- [ ] **Step 4: Den Handler implementieren**

`internal/adapters/http/explorer.go`:

```go
package httpapi

import (
	_ "embed"
	"net/http"
)

// The explorer page is embedded, not read from disk: situs ships as a single
// binary, and a page that needs a file next to the executable is a page that
// is missing wherever the binary was copied to.
//
// Embedded as []byte rather than an embed.FS, for the same reason docs.go
// does it: a single file resolved at compile time has no read that could
// fail at runtime, and therefore no error branch no test could reach.
//
//go:embed explorer.html
var explorerPage []byte

// handleExplorer serves the API explorer at the root.
//
// It lives at "/" because a locally started service whose bare address shows
// something usable needs no explanation. gorilla/mux matches this exactly,
// not as a prefix, so unknown paths stay 404 instead of silently returning
// the page.
func (s *Server) handleExplorer(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(explorerPage); err != nil {
		s.logger.Error("writing the explorer page", "error", err)
	}
}
```

- [ ] **Step 5: Die Route mounten**

In `internal/adapters/http/server.go`, bei den Betriebs-Routen (vor den `/v1`-Routen):

```go
	r.HandleFunc("/", s.handleExplorer).Methods(http.MethodGet)
```

- [ ] **Step 6: OpenAPI in BEIDEN Kopien dokumentieren**

In `internal/adapters/http/openapi.yaml`, bei `/docs`:

```yaml
  /:
    get:
      summary: API-Explorer
      description: >-
        Eine selbst-enthaltene Weboberfläche, die jeden Lese-Endpunkt dieses
        Dienstes ausprobierbar macht und dabei die jeweils abgesetzte URL
        mitzeigt. Stylesheet und Skript sind eingebettet, nichts wird aus dem
        Netz nachgeladen — situs läuft lokal und im Feld ohne Netz. Die Seite
        rechnet nichts selbst aus: sie zeigt, was der Dienst antwortet.
      operationId: explorer
      responses:
        "200":
          description: Die Explorer-Seite als HTML.
          content:
            text/html:
              schema:
                type: string
```

Dann kopieren:

```bash
cp internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
```

- [ ] **Step 7: Tests grün bekommen**

Run: `go test ./internal/adapters/http/ -v`
Expected: PASS, inklusive `TestRoutesMatchOpenAPISpec` und `TestOpenAPICopiesAreIdentical`.

- [ ] **Step 8: Die Seite von Hand ansehen**

```bash
go build -o /tmp/situs-explorer ./cmd/situs
SITUS_INDEX_PATH=out/situs-0.3.0.sqlite /tmp/situs-explorer serve
```

Dann `http://localhost:8070/` öffnen und prüfen: Kopfzeile zeigt die Index-Selbstauskunft, die Namenssuche findet „Fagus", ein Klick auf einen Treffer füllt das Concept-ID-Feld, jedes Panel liefert eine Antwort, die Zeigerwertanalyse zeigt für EIVE beide Mittelwerte und für Tichý nur einen.

- [ ] **Step 9: `make verify` und committen**

```bash
make verify
git add internal/adapters/http/explorer.html internal/adapters/http/explorer.go \
        internal/adapters/http/explorer_test.go internal/adapters/http/server.go \
        internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
git commit -m "feat(http): self-contained API explorer at /"
```

---

### Task 7: Dokumentation

**Files:**
- Modify: `docs/reference/http-api.md`
- Modify: `README.md`
- Modify: `docs/how-to/ingest.md` (nur falls dort Routen aufgezählt werden)

**Interfaces:**
- Consumes: alle drei neuen Routen aus den Tasks 2, 5 und 6.
- Produces: nichts, was Code konsumiert.

- [ ] **Step 1: `docs/reference/http-api.md` ergänzen**

Die drei Routen in der bestehenden Struktur und im bestehenden Ton (deutsch) dokumentieren. Inhaltlich zwingend:

- `GET /` — Explorer, selbst-enthalten, kein Netz nötig.
- `GET /v1/species/search` — **mit** der Abgrenzung: keine Fuzzy-Auflösung, keine Synonyme, kein hostus-Ersatz; `concept_id: null` wird mitgeliefert und nicht gefiltert.
- `POST /v1/species/traits/summary` — mit der Tabelle der Rechenregeln: pro Vokabular getrennt, `1/Nischenbreite` als Gewicht, `sd` fehlt bei n < 2, `mean_niche_weighted` fehlt ohne Nischenbreiten, `n`/`n_missing` je Dimension.

- [ ] **Step 2: `README.md` ergänzen**

Im Abschnitt zum Starten des Dienstes den Hinweis ergänzen, dass unter `http://localhost:8070/` der Explorer liegt — ein Satz, keine Abhandlung. Beispielaufruf für die Namenssuche und die Zeigerwertanalyse ergänzen, passend zu den vorhandenen curl-Beispielen.

- [ ] **Step 3: Die Doku bauen**

Run: `make docs`
Expected: `mkdocs build --strict` läuft durch — `--strict` bricht bei einem toten internen Link ab.

- [ ] **Step 4: `make verify` und committen**

```bash
make verify
git add docs/reference/http-api.md README.md
git commit -m "docs: explorer, name search and indicator-value analysis"
```

---

## Self-Review

**1. Spec-Abdeckung**

| Spec-Abschnitt | Task |
|---|---|
| `GET /` Explorer-Seite, eingebettet, offline | 6 |
| Acht Panels, URL sichtbar, formatiert ⇄ JSON | 6 |
| Keine Aggregation/Ranking im Frontend | 6 (Panels bilden nur Endpunkte ab) |
| `GET /v1/species/search`, `q` pflichtig, `limit` 20/100 | 2 |
| `concept_id: null` wird mitgeliefert | 1 (Adapter), 2 (DTO ohne `omitempty`) |
| LIKE-Wildcards escapen | 1 |
| `POST /v1/species/traits/summary` | 5 |
| Pro Vokabular/Dimension, nie darüber hinweg | 3 (Statistik), 5 (Bucketing) |
| `1/niche_width`, `niche_width <= 0` ausgeschlossen und gezählt | 3 |
| `mean_niche_weighted` fehlt ohne Nischenbreiten | 3 |
| `sd` fehlt bei n < 2 | 3 |
| `n`/`n_missing` je Dimension | 3 |
| Unbekannte IDs mit `unknown_backbone`/`unknown_concept` | 5 |
| Leere Liste → `INVALID_QUERY` | 5 |
| OpenAPI in beiden Kopien | 2, 5, 6 |
| Doku | 7 |

Keine Lücke gefunden.

**2. Platzhalter-Scan**

Kein „TBD"/„TODO"/„später". Jeder Code-Schritt enthält vollständigen Code. Einzige Ausnahme mit Absicht: die acht Panel-Blöcke in Task 6 sind als Muster plus Aufbau beschrieben statt achtmal ausgeschrieben — sie sind strukturell identisch, und der gemeinsame JS-Helfer (`call`, `globals`, `search`) ist vollständig angegeben.

**3. Typkonsistenz**

- `domain.SpeciesName{VerbatimName, ConceptID}` — Task 1 definiert, Tasks 1/2 nutzen.
- `SearchSpeciesNames(ctx, q string, limit int)` — Signatur in Tasks 1, 2 und im fakeRepo identisch.
- `TraitsForConcepts(ctx, conceptIDs []string) (map[string][]domain.TraitValue, error)` — Tasks 4 und 5 identisch.
- `summarizeDimension(values []domain.TraitValue, known int) input.DimensionSummary` — Task 3 definiert, Task 5 ruft mit genau dieser Signatur.
- `input.DimensionSummary`-Felder (`Mean`, `MeanNicheWeighted`, `SD`, `Min`, `Max`, `N`, `NMissing`, `NExcludedWeighted`) — in Task 3 definiert, in Tasks 3/5 und im OpenAPI-Schema gleich benannt.
- `input.ErrInvalidQuery` — existiert **nicht** (verifiziert), wird in Task 2 angelegt und in `writeQueryError` verdrahtet; Task 5 benutzt ihn. **Abhängigkeit: Task 5 setzt Task 2 voraus.**
- `batchRequest`, `maxBatchBodyBytes`, `maxBatchConceptIDs` — bestehende Typen aus `species.go`, in Task 5 wiederverwendet statt dupliziert.

**4. Gegen den echten Code verifiziert** (statt angenommen)

| Annahme | Befund | Konsequenz im Plan |
|---|---|---|
| `newTestServer(t)` | Nimmt **zwei** Argumente: `newTestServer(t, query input.QueryService)` (`contract_test.go:152`) | Alle 14 Aufrufe auf `newTestServer(t, seededQueryService())` korrigiert |
| sqlite-Tests sind `package sqlite_test` | Sie sind `package sqlite` (paketintern) | Task 1 nutzt `package sqlite`, kein Selbst-Import |
| `openTestDB` vorhanden | Ja, `write_test.go:25`, gibt `*DB` | Nicht neu anlegen |
| `input.ErrInvalidQuery` vorhanden | **Nein** | Task 2 legt ihn an *und* ergänzt `habitat.go:170`, sonst 500 statt 400 |
| HTTP-Tests laufen gegen einen echten Index | **Nein**, gegen `fakeQueryService` | Tasks 2 und 5 erweitern ihn explizit — sonst kompiliert das ganze http-Testpaket nicht mehr |

Der letzte Punkt ist der teuerste: Jede Erweiterung von `input.QueryService`
bricht `fakeQueryService`, und der Fehler erscheint nicht in der Task, die
das Interface ändert, sondern als Compile-Fehler im gesamten
`internal/adapters/http`-Testpaket.

**Reihenfolge:** 1 → 2 → 3 → 4 → 5 → 6 → 7. Tasks 3 und 4 sind voneinander unabhängig und könnten getauscht werden; Task 5 braucht beide plus `ErrInvalidQuery` aus Task 2.
