# situs Foundation Implementation Plan

> **Historischer Stand, umgesetzt 2026-08-19.** Eine Abweichung ist bewusst nicht
> nachträglich in den Text eingearbeitet: der Plan nennt an sechs Stellen
> `vendor/claude-skills` als Ort des Submoduls. Es liegt unter
> `third_party/claude-skills` — ein `vendor/` im Modul-Root schaltet die
> Go-Toolchain in Vendor-Mode und bricht jedes nackte `go build`/`go test`
> ("inconsistent vendoring"). Wer diesen Plan als Vorlage liest: `third_party/`
> verwenden.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build **situs**, a new read-only Go service that answers EUNIS
habitat-type questions — species → habitat types (with role), habitat type →
species/syntaxa/crosswalks — from pinned EEA/ESy artifacts, with German labels
and an Annex I (FFH-LRT) crosswalk.

**Architecture:** Hexagonal Go service (hostus twin): `domain` (typology/habitat
type/qualifier value objects, no I/O) → `ports` → `application` → `adapters`
(sqlite, http, hostus-client). Source XLSX files are converted to normalized
CSVs by a **stdlib-only Python pipeline** *outside* the binary (so the Go module
keeps its narrow dependency list); the Go ingest reads only CSV. Species names
are crosswalked to hostus concept IDs at ingest time; at runtime situs is
autark.

**Tech Stack:** Go 1.26, `gorilla/mux`, `spf13/cobra` + `spf13/viper`,
`modernc.org/sqlite` (pure-Go, CGO-free), OpenTelemetry Go SDK, Prometheus
client. Pipeline: `python3` **stdlib only** (`zipfile` + `xml.etree` read XLSX
directly — no third-party reader).

**Source spec:** `docs/superpowers/specs/2026-08-18-situs-foundation-design.md`
(in the hostus repo; Task 1 copies it into the new situs repo).
**Data feasibility:** `docs/research/situs-eea-eunis-2021-spike.md`,
`docs/research/sp9-esy-spike.md`.

## Global Constraints

- **Repo:** situs is its **own repository** at `~/work/projects/situs` (sibling
  of hostus). Never build it inside the hostus repo.
- **Go version:** 1.26 (via Nix flake, as hostus does).
- **Allowed libraries only:** Go stdlib, `github.com/gorilla/mux`,
  `github.com/spf13/viper`, `github.com/spf13/cobra`, `modernc.org/sqlite`,
  OpenTelemetry Go SDK (+`otelmux`), official Prometheus Go client. **No** ORMs,
  no reflection-heavy deps, **no XLSX library** — XLSX is handled by the Python
  pipeline, never by Go.
- **Pipeline scripts:** `bash` + `python3` **stdlib only** (matches the
  claude-skills repo convention).
- **Ubiquitous language (verbatim from the spec):** a **habitat typology** is a
  classification system in a fassung (`eunis@2021`, `eunis@2012`, `annex1`); a
  **habitat type** is an abstract type identified by `(typology, code)` — never
  a biotope in the landscape. Never name a table, type, or route just `habitat`.
- **One crosswalk mechanism:** the EUNIS version crosswalk and the EUNIS↔Annex I
  crosswalk are the same concept and share **one** table
  (`habitat_type_crosswalk`) and **one** route family.
- **Identifiers stay international:** the Annex I typology id is `annex1`.
  "FFH-LRT"/"Lebensraumtyp" is a German label and lives in `localization`.
- **Localization is overlay, never replacement:** `habitat_type.name_en` stays
  the identity; `name_de` is additive. `provenance` ∈
  `official` | `curated` | `derived`.
- **Derived German labels only from qualifier `=`.** Never from `<`, `>`, `#`.
- **Missing data is absence of rows**, never a placeholder code. A habitat type
  with no Annex I correspondence is the normal case.
- **Unresolvable species names are kept**, not dropped: `verbatim_name` always
  set, `concept_id` NULL, and the resolution rate is measured and logged.
- **Error envelope (identical to hostus):**
  `{"error":{"code":"...","message":"..."}}` with codes `INVALID_QUERY`,
  `NOT_FOUND`, `UNRESOLVABLE`, `UPSTREAM_UNAVAILABLE`, `INTERNAL_ERROR`.
- **Quality gates:** `make verify` (fmt-check, vet, lint, test, arch, debt) must
  be green before every commit; zero `//nolint` / `#nosec` (debt-guard baseline
  0); mutation testing runs in CI (gremlins panics on macOS).
- **Git:** feature branch per task group, conventional commits, never commit
  directly to the default branch. `CHANGELOG.md`/`VERSION` are owned by
  release-please — do not hand-edit.
- **Docs:** `README.md` in German (hostus convention); code comments sparse and
  English.

---

## File Structure

**New repo `~/work/projects/situs`:**

| Path | Responsibility |
|---|---|
| `vendor/claude-skills/` | git submodule, canonical skills (Task 1) |
| `.claude/skills/*` | symlinks into the submodule (Task 1) |
| `cmd/situs/main.go` | thin entrypoint |
| `cmd/situs/{serve,ingest,version}.go` | cobra commands |
| `internal/domain/typology.go` | `TypologyID`, `ParseTypologyID`, `HabitatTypeKey` |
| `internal/domain/qualifier.go` | `Qualifier`, `ParseQualifier` |
| `internal/domain/habitat.go` | `HabitatType`, `Syntaxon`, `SpeciesRole`, `Localization`, `Crosswalk` |
| `internal/ports/output/repository.go` | `Repository`, `IngestTx`, `NameResolver` |
| `internal/ports/input/services.go` | driving ports for the HTTP adapter |
| `internal/application/ingest.go` | ingest use case (CSV → repo, name crosswalk, derived labels) |
| `internal/application/query.go` | read use cases |
| `internal/adapters/sqlite/{schema.sql,db.go,write.go,read.go}` | SQLite adapter |
| `internal/adapters/hostus/client.go` | `NameResolver` via hostus `POST /v1/match` |
| `internal/adapters/http/{server.go,habitat.go,species.go,openapi.yaml}` | HTTP adapter + spec |
| `internal/app/app.go` | composition root |
| `internal/config/config.go` | `SITUS_`-prefixed config |
| `pipelines/eunis/xlsx_to_csv.py` | XLSX → normalized CSV + measurement report |
| `pipelines/eunis/manifest.yaml` | pinned source URLs/checksums |

---

### Task 1: Scaffold the situs repo with the full quality harness

**Files:**
- Create: `~/work/projects/situs/` (new git repo, everything below is relative to it)
- Create: `vendor/claude-skills` (submodule), `.claude/skills/*` (symlinks)
- Create: `go.mod`, `cmd/situs/main.go`, `internal/{domain,ports,application,adapters,app,config}/`
- Create: `Makefile`, `.golangci.yml`, `.debt-budget`, `.coverage-floors`, `gremlins.yaml`
- Create: `CLAUDE.md`, `README.md`, `docs/`, `.github/workflows/`
- Copy in: the spec + both spikes from the hostus repo

**Interfaces:**
- Consumes: nothing (first task).
- Produces: a green `make verify`, the module path
  `github.com/jobrunner/situs`, env prefix `SITUS_`, and the hexagonal package
  layout every later task fills in.

- [ ] **Step 1: Create the repo and vendor the skills**

```bash
mkdir -p ~/work/projects/situs && cd ~/work/projects/situs
git init -b main
git submodule add -b main https://github.com/jobrunner/claude-skills.git vendor/claude-skills
mkdir -p .claude/skills
for d in vendor/claude-skills/skills/*/; do
  s=$(basename "$d")
  ln -s "../../vendor/claude-skills/skills/$s" ".claude/skills/$s"
done
```

- [ ] **Step 2: Make the symlinks versioned**

Create `.gitignore` containing at least:

```gitignore
/situs
*.sqlite
*.sqlite-wal
*.sqlite-shm
.claude/*
!.claude/skills
!.claude/skills/**
```

- [ ] **Step 3: Verify the skill is discoverable, then follow it**

Run `ls -l .claude/skills/new-go-service` — it must resolve into
`vendor/claude-skills/skills/new-go-service`. Then **read
`.claude/skills/new-go-service/SKILL.md` and execute its Steps 1–10 in order**,
with these situs-specific substitutions:

| Skill placeholder | situs value |
|---|---|
| `<svc>` | `situs` |
| `<module>` | `github.com/jobrunner/situs` |
| `<PREFIX>` | `SITUS` |

Deviations from the skill's defaults, required by this plan:
- Route prefix is **`/v1`** (not `/api/v1`) and the spec is served at
  **`/openapi`** — hostus convention. Adjust `templates/openapi.yaml` and
  `templates/contract_test.go.tmpl` accordingly; the contract test checks both
  directions, so it stays green once both sides agree.
- Rename the template's demo `items` surface away entirely — situs adds its own
  routes in Task 8. Keep exactly one placeholder route so the contract test has
  something to assert until then.
- Add `modernc.org/sqlite` to `go.mod` now (Task 3 needs it) and add the
  allowed-library list from Global Constraints to the `gomodguard` section of
  `.golangci.yml`.

- [ ] **Step 4: Write CLAUDE.md**

Create `CLAUDE.md` stating: what situs is (one paragraph from the spec's Ziel),
the Ubiquitous Language block verbatim from Global Constraints above, the
allowed-library list, the gates (`make verify`, debt-guard 0, mutation in CI),
and the git workflow (feature branches, conventional commits, release-please
owns VERSION/CHANGELOG).

- [ ] **Step 5: Copy the design documents in**

```bash
mkdir -p docs/superpowers/specs docs/superpowers/plans docs/research
cp ~/work/projects/hostus/docs/superpowers/specs/2026-08-18-situs-foundation-design.md docs/superpowers/specs/
cp ~/work/projects/hostus/docs/superpowers/plans/2026-08-19-situs-foundation.md docs/superpowers/plans/
cp ~/work/projects/hostus/docs/research/situs-eea-eunis-2021-spike.md docs/research/
cp ~/work/projects/hostus/docs/research/sp9-esy-spike.md docs/research/
```

- [ ] **Step 6: Green the harness**

Run: `make verify`
Expected: PASS (fmt-check, vet, lint, test, arch, debt all green).
If `make arch` fails, fix the depguard `pkg:` paths to `github.com/jobrunner/situs/...`.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "chore: scaffold situs with vendored claude-skills and the quality harness"
```

---

### Task 2: Domain value objects — typology, qualifier, habitat type key

**Files:**
- Create: `internal/domain/typology.go`, `internal/domain/typology_test.go`
- Create: `internal/domain/qualifier.go`, `internal/domain/qualifier_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `type TypologyID string`
  - `func ParseTypologyID(s string) (TypologyID, error)`
  - `func (t TypologyID) Scheme() string` / `func (t TypologyID) Version() string`
  - `type HabitatTypeKey struct { Typology TypologyID; Code string }`
  - `func (k HabitatTypeKey) String() string` → `"eunis@2021:R22"`
  - `type Qualifier string` with `QualifierSame/Narrower/Broader/Partial`
  - `func ParseQualifier(s string) (Qualifier, error)`
  - `func (q Qualifier) IsSame() bool`

- [ ] **Step 1: Write the failing test for TypologyID**

Create `internal/domain/typology_test.go`:

```go
package domain

import "testing"

func TestParseTypologyID(t *testing.T) {
	tests := []struct {
		in            string
		wantScheme    string
		wantVersion   string
		wantErr       bool
	}{
		{in: "eunis@2021", wantScheme: "eunis", wantVersion: "2021"},
		{in: "eunis@2012", wantScheme: "eunis", wantVersion: "2012"},
		{in: "annex1", wantScheme: "annex1", wantVersion: ""},
		{in: "  eunis@2021  ", wantScheme: "eunis", wantVersion: "2021"},
		{in: "", wantErr: true},
		{in: "@2021", wantErr: true},
		{in: "eunis@", wantErr: true},
		{in: "eunis@2021@x", wantErr: true},
	}
	for _, tc := range tests {
		got, err := ParseTypologyID(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseTypologyID(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTypologyID(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got.Scheme() != tc.wantScheme || got.Version() != tc.wantVersion {
			t.Errorf("ParseTypologyID(%q) = scheme %q version %q, want %q/%q",
				tc.in, got.Scheme(), got.Version(), tc.wantScheme, tc.wantVersion)
		}
	}
}

func TestHabitatTypeKeyString(t *testing.T) {
	k := HabitatTypeKey{Typology: TypologyID("eunis@2021"), Code: "R22"}
	if got, want := k.String(), "eunis@2021:R22"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run 'TypologyID|HabitatTypeKey' -v`
Expected: FAIL — `undefined: ParseTypologyID`, `undefined: HabitatTypeKey`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/domain/typology.go`:

```go
package domain

import (
	"fmt"
	"strings"
)

// TypologyID identifies a habitat classification system in a given fassung,
// e.g. "eunis@2021" or "annex1" (which carries no version).
type TypologyID string

// ParseTypologyID validates "<scheme>" or "<scheme>@<version>".
func ParseTypologyID(s string) (TypologyID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("typology id is empty")
	}
	scheme, version, hasAt := strings.Cut(s, "@")
	if scheme == "" {
		return "", fmt.Errorf("typology id %q has no scheme", s)
	}
	if hasAt && version == "" {
		return "", fmt.Errorf("typology id %q has an empty version", s)
	}
	if strings.Contains(version, "@") {
		return "", fmt.Errorf("typology id %q has more than one %q", s, "@")
	}
	return TypologyID(s), nil
}

func (t TypologyID) Scheme() string {
	scheme, _, _ := strings.Cut(string(t), "@")
	return scheme
}

func (t TypologyID) Version() string {
	_, version, _ := strings.Cut(string(t), "@")
	return version
}

// HabitatTypeKey identifies an abstract habitat type. A type is never
// identified by its code alone — the same code means different things in
// different typologies.
type HabitatTypeKey struct {
	Typology TypologyID
	Code     string
}

func (k HabitatTypeKey) String() string {
	return string(k.Typology) + ":" + k.Code
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/ -run 'TypologyID|HabitatTypeKey' -v`
Expected: PASS.

- [ ] **Step 5: Write the failing test for Qualifier**

Create `internal/domain/qualifier_test.go`:

```go
package domain

import "testing"

func TestParseQualifier(t *testing.T) {
	tests := []struct {
		in      string
		want    Qualifier
		wantErr bool
	}{
		{in: "=", want: QualifierSame},
		{in: "<", want: QualifierNarrower},
		{in: ">", want: QualifierBroader},
		{in: "#", want: QualifierPartial},
		{in: " = ", want: QualifierSame},
		{in: "", wantErr: true},
		{in: "==", wantErr: true},
		{in: "~", wantErr: true},
	}
	for _, tc := range tests {
		got, err := ParseQualifier(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseQualifier(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseQualifier(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseQualifier(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Only '=' means full correspondence — the derived German label rule depends
// on exactly this distinction, so it is pinned here.
func TestQualifierIsSame(t *testing.T) {
	if !QualifierSame.IsSame() {
		t.Error("QualifierSame.IsSame() = false, want true")
	}
	for _, q := range []Qualifier{QualifierNarrower, QualifierBroader, QualifierPartial} {
		if q.IsSame() {
			t.Errorf("%q.IsSame() = true, want false", q)
		}
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./internal/domain/ -run Qualifier -v`
Expected: FAIL — `undefined: ParseQualifier`.

- [ ] **Step 7: Write minimal implementation**

Create `internal/domain/qualifier.go`:

```go
package domain

import (
	"fmt"
	"strings"
)

// Qualifier is the coverage of a crosswalk correspondence, verbatim from the
// EUNIS sources.
type Qualifier string

const (
	QualifierSame     Qualifier = "=" // full correspondence
	QualifierNarrower Qualifier = "<"
	QualifierBroader  Qualifier = ">"
	QualifierPartial  Qualifier = "#"
)

func ParseQualifier(s string) (Qualifier, error) {
	switch q := Qualifier(strings.TrimSpace(s)); q {
	case QualifierSame, QualifierNarrower, QualifierBroader, QualifierPartial:
		return q, nil
	default:
		return "", fmt.Errorf("unknown crosswalk qualifier %q", s)
	}
}

// IsSame reports full correspondence. Only these may seed a derived German
// label for a EUNIS type (see the spec).
func (q Qualifier) IsSame() bool { return q == QualifierSame }
```

- [ ] **Step 8: Run the full domain suite**

Run: `go test ./internal/domain/ -v`
Expected: PASS, output pristine.

- [ ] **Step 9: Commit**

```bash
git add internal/domain
git commit -m "feat(domain): typology id, habitat type key and crosswalk qualifier"
```

---

### Task 3: SQLite schema and the ingest write side

**Files:**
- Create: `internal/domain/habitat.go`
- Create: `internal/ports/output/repository.go`
- Create: `internal/adapters/sqlite/schema.sql`, `db.go`, `write.go`
- Create: `internal/adapters/sqlite/write_test.go`

**Interfaces:**
- Consumes: `domain.TypologyID`, `domain.HabitatTypeKey`, `domain.Qualifier` (Task 2).
- Produces:
  - `domain.HabitatType{Key HabitatTypeKey; Level *int; NameEN string; ParentCode string; Priority *bool}`
  - `domain.Crosswalk{From, To HabitatTypeKey; Qualifier Qualifier}`
  - `domain.Syntaxon{ID, Rank, Name, ParentID string}`
  - `domain.SpeciesRole{Key HabitatTypeKey; ConceptID *string; VerbatimName, Role string; Fidelity, Constancy *float64}`
  - `domain.Localization{EntityType, EntityKey, Lang, Field, Value, Source, Provenance, DerivedFrom string}`
  - `output.Repository` with `Begin(ctx) (IngestTx, error)`
  - `output.IngestTx` with `UpsertTypology`, `UpsertHabitatType`, `UpsertCrosswalk`,
    `UpsertSyntaxon`, `LinkSyntaxon`, `UpsertSpeciesRole`, `UpsertLocalization`,
    `Commit`, `Rollback`
  - `sqlite.Open(path string) (*DB, error)`

- [ ] **Step 1: Write the schema**

Create `internal/adapters/sqlite/schema.sql` (embedded and executed by `Open`):

```sql
CREATE TABLE IF NOT EXISTS habitat_typology (
  id         TEXT PRIMARY KEY,
  scheme     TEXT NOT NULL,
  version    TEXT NOT NULL DEFAULT '',
  name       TEXT NOT NULL DEFAULT '',
  source_ref TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS habitat_type (
  typology_id TEXT NOT NULL,
  code        TEXT NOT NULL,
  level       INTEGER,
  name_en     TEXT NOT NULL DEFAULT '',
  parent_code TEXT NOT NULL DEFAULT '',
  priority    INTEGER,
  PRIMARY KEY (typology_id, code)
);

CREATE TABLE IF NOT EXISTS habitat_type_crosswalk (
  from_typology TEXT NOT NULL,
  from_code     TEXT NOT NULL,
  to_typology   TEXT NOT NULL,
  to_code       TEXT NOT NULL,
  qualifier     TEXT NOT NULL,
  PRIMARY KEY (from_typology, from_code, to_typology, to_code)
);
CREATE INDEX IF NOT EXISTS idx_crosswalk_to
  ON habitat_type_crosswalk(to_typology, to_code);

CREATE TABLE IF NOT EXISTS syntaxon (
  id        TEXT PRIMARY KEY,
  rank      TEXT NOT NULL,
  name      TEXT NOT NULL,
  parent_id TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS habitat_type_syntaxon (
  typology_id TEXT NOT NULL,
  code        TEXT NOT NULL,
  syntaxon_id TEXT NOT NULL,
  PRIMARY KEY (typology_id, code, syntaxon_id)
);
CREATE INDEX IF NOT EXISTS idx_hts_syntaxon ON habitat_type_syntaxon(syntaxon_id);

CREATE TABLE IF NOT EXISTS species_role (
  typology_id   TEXT NOT NULL,
  code          TEXT NOT NULL,
  concept_id    TEXT,
  verbatim_name TEXT NOT NULL,
  role          TEXT NOT NULL,
  fidelity      REAL,
  constancy     REAL,
  PRIMARY KEY (typology_id, code, verbatim_name, role)
);
CREATE INDEX IF NOT EXISTS idx_species_role_concept ON species_role(concept_id);

CREATE TABLE IF NOT EXISTS localization (
  entity_type  TEXT NOT NULL,
  entity_key   TEXT NOT NULL,
  lang         TEXT NOT NULL,
  field        TEXT NOT NULL,
  value        TEXT NOT NULL,
  source       TEXT NOT NULL,
  provenance   TEXT NOT NULL,
  derived_from TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (entity_type, entity_key, lang, field, source)
);
CREATE INDEX IF NOT EXISTS idx_localization_lookup
  ON localization(entity_type, entity_key, lang);
```

- [ ] **Step 2: Write the failing test**

Create `internal/adapters/sqlite/write_test.go`:

```go
package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestIngestTx_RoundTripsAHabitatType(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	level := 3
	if err := tx.UpsertTypology(domain.Typology{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}); err != nil {
		t.Fatalf("UpsertTypology: %v", err)
	}
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	if err := tx.UpsertHabitatType(domain.HabitatType{Key: key, Level: &level, NameEN: "Low and medium altitude hay meadow"}); err != nil {
		t.Fatalf("UpsertHabitatType: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.HabitatType(ctx, key)
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	if got.NameEN != "Low and medium altitude hay meadow" {
		t.Errorf("NameEN = %q, want the ingested name", got.NameEN)
	}
	if got.Level == nil || *got.Level != 3 {
		t.Errorf("Level = %v, want 3", got.Level)
	}
}

// Re-ingesting the same source must not duplicate or fail — ingest is rerun
// whenever an artifact is repinned.
func TestIngestTx_UpsertIsIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}

	for i, name := range []string{"first", "second"} {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Begin %d: %v", i, err)
		}
		if err := tx.UpsertTypology(domain.Typology{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}); err != nil {
			t.Fatalf("UpsertTypology %d: %v", i, err)
		}
		if err := tx.UpsertHabitatType(domain.HabitatType{Key: key, NameEN: name}); err != nil {
			t.Fatalf("UpsertHabitatType %d: %v", i, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit %d: %v", i, err)
		}
	}

	got, err := db.HabitatType(ctx, key)
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	if got.NameEN != "second" {
		t.Errorf("NameEN = %q, want %q (the later ingest wins)", got.NameEN, "second")
	}
	n, err := db.countHabitatTypes(ctx)
	if err != nil {
		t.Fatalf("countHabitatTypes: %v", err)
	}
	if n != 1 {
		t.Errorf("habitat_type rows = %d, want 1 (upsert, not insert)", n)
	}
}

// Rollback must leave nothing behind — a failed ingest may not half-populate.
func TestIngestTx_RollbackDiscards(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTypology(domain.Typology{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}); err != nil {
		t.Fatalf("UpsertTypology: %v", err)
	}
	if err := tx.UpsertHabitatType(domain.HabitatType{
		Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, NameEN: "x",
	}); err != nil {
		t.Fatalf("UpsertHabitatType: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	n, err := db.countHabitatTypes(ctx)
	if err != nil {
		t.Fatalf("countHabitatTypes: %v", err)
	}
	if n != 0 {
		t.Errorf("habitat_type rows = %d after rollback, want 0", n)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapters/sqlite/ -v`
Expected: FAIL — `undefined: Open`, `undefined: domain.Typology`.

- [ ] **Step 4: Write the domain entities**

Create `internal/domain/habitat.go`:

```go
package domain

// Typology is a habitat classification system in a given fassung.
type Typology struct {
	ID        TypologyID
	Scheme    string
	Version   string
	Name      string
	SourceRef string
}

// HabitatType is an abstract type within a typology — not a biotope in the
// landscape. Level/ParentCode are nil/empty for typologies without hierarchy;
// Priority is set only for annex1 (priority habitat type).
type HabitatType struct {
	Key        HabitatTypeKey
	Level      *int
	NameEN     string
	ParentCode string
	Priority   *bool
}

// Crosswalk is a correspondence between two habitat types. The same shape
// carries both the EUNIS version crosswalk and the EUNIS->annex1 crosswalk.
type Crosswalk struct {
	From      HabitatTypeKey
	To        HabitatTypeKey
	Qualifier Qualifier
}

type Syntaxon struct {
	ID       string
	Rank     string // "class" | "order" | "alliance"
	Name     string
	ParentID string
}

// SpeciesRole is a species' role in a habitat type. VerbatimName is always
// set; ConceptID is nil when the name could not be resolved via hostus.
type SpeciesRole struct {
	Key          HabitatTypeKey
	ConceptID    *string
	VerbatimName string
	Role         string // "diagnostic" | "constant" | "dominant"
	Fidelity     *float64
	Constancy    *float64
}

// Localization is an additive label overlay. Provenance is "official",
// "curated" or "derived"; DerivedFrom records the origin of a derived value.
type Localization struct {
	EntityType  string // "habitat_type" | "syntaxon"
	EntityKey   string
	Lang        string
	Field       string // "name" | "description" | "key"
	Value       string
	Source      string
	Provenance  string
	DerivedFrom string
}
```

- [ ] **Step 5: Write the repository ports**

Create `internal/ports/output/repository.go`:

```go
package output

import (
	"context"

	"github.com/jobrunner/situs/internal/domain"
)

// IngestTx is one atomic ingest run. Every Upsert is idempotent so a repinned
// artifact can simply be re-ingested.
type IngestTx interface {
	UpsertTypology(t domain.Typology) error
	UpsertHabitatType(h domain.HabitatType) error
	UpsertCrosswalk(c domain.Crosswalk) error
	UpsertSyntaxon(s domain.Syntaxon) error
	LinkSyntaxon(key domain.HabitatTypeKey, syntaxonID string) error
	UpsertSpeciesRole(r domain.SpeciesRole) error
	UpsertLocalization(l domain.Localization) error
	Commit() error
	Rollback() error
}

type Repository interface {
	Begin(ctx context.Context) (IngestTx, error)
	HabitatType(ctx context.Context, key domain.HabitatTypeKey) (domain.HabitatType, error)
}
```

- [ ] **Step 6: Write the SQLite adapter**

Create `internal/adapters/sqlite/db.go` with `//go:embed schema.sql`, `Open`
(opening `modernc.org/sqlite`, setting `PRAGMA journal_mode=WAL` and
`foreign_keys=ON`, then executing the schema), `Close`, `Begin`, `HabitatType`
and the test helper `countHabitatTypes`. Create `internal/adapters/sqlite/write.go`
with the `ingestTx` type implementing every `IngestTx` method as
`INSERT ... ON CONFLICT(...) DO UPDATE SET ...`, e.g.:

```go
func (t *ingestTx) UpsertHabitatType(h domain.HabitatType) error {
	_, err := t.tx.Exec(
		`INSERT INTO habitat_type (typology_id, code, level, name_en, parent_code, priority)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(typology_id, code) DO UPDATE SET
		   level=excluded.level, name_en=excluded.name_en,
		   parent_code=excluded.parent_code, priority=excluded.priority`,
		string(h.Key.Typology), h.Key.Code, h.Level, h.NameEN, h.ParentCode, h.Priority)
	if err != nil {
		return fmt.Errorf("sqlite: upserting habitat type %s: %w", h.Key, err)
	}
	return nil
}
```

Every statement must be a **static string with `?` placeholders** — never build
SQL by concatenating values (gosec G201/G202 fail the build).

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/adapters/sqlite/ -v`
Expected: PASS (all three tests).

- [ ] **Step 8: Verify and commit**

Run: `make verify`
Expected: PASS.

```bash
git add internal/domain internal/ports internal/adapters/sqlite
git commit -m "feat(sqlite): habitat typology schema and idempotent ingest write side"
```

---

### Task 4: XLSX → CSV pipeline with a measurement report

**Files:**
- Create: `pipelines/eunis/xlsx_to_csv.py`
- Create: `pipelines/eunis/manifest.yaml`
- Create: `pipelines/eunis/test_xlsx_to_csv.py`
- Create: `pipelines/eunis/README.md`

**Interfaces:**
- Consumes: nothing from Go.
- Produces these CSVs (pipe-free, RFC4180, UTF-8, header row) that Task 5 reads:
  - `typologies.csv`: `id,scheme,version,name,source_ref`
  - `habitat_types.csv`: `typology_id,code,level,name_en,parent_code,priority`
  - `crosswalks.csv`: `from_typology,from_code,to_typology,to_code,qualifier`
  - `syntaxa.csv`: `id,rank,name,parent_id`
  - `habitat_type_syntaxa.csv`: `typology_id,code,syntaxon_id`
  - `report.json`: the measurements (see Step 5)

- [ ] **Step 1: Write the failing test**

Create `pipelines/eunis/test_xlsx_to_csv.py` (stdlib `unittest`; it builds a
minimal XLSX in-memory so the test needs no network and no fixture binary):

```python
import io
import unittest
import zipfile

from xlsx_to_csv import read_sheet


def make_xlsx(rows):
    """Build a minimal single-sheet .xlsx with inline strings."""
    def cell(ci, value):
        ref = f"{chr(ord('A') + ci)}"
        return (f'<c r="{ref}" t="inlineStr"><is><t>{value}</t></is></c>')

    sheet_rows = "".join(
        "<row>" + "".join(cell(ci, v) for ci, v in enumerate(r)) + "</row>"
        for r in rows
    )
    sheet = (
        '<?xml version="1.0"?><worksheet '
        'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
        f"<sheetData>{sheet_rows}</sheetData></worksheet>"
    )
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        z.writestr("xl/worksheets/sheet1.xml", sheet)
        z.writestr("xl/sharedStrings.xml", '<?xml version="1.0"?><sst></sst>')
    buf.seek(0)
    return buf


class ReadSheetTest(unittest.TestCase):
    def test_reads_rows_as_strings(self):
        src = make_xlsx([["Code", "Name"], ["R22", "Hay meadow"]])
        self.assertEqual(
            read_sheet(src, "xl/worksheets/sheet1.xml"),
            [["Code", "Name"], ["R22", "Hay meadow"]],
        )

    def test_pads_short_rows_so_columns_line_up(self):
        src = make_xlsx([["Code", "Name"], ["R22"]])
        rows = read_sheet(src, "xl/worksheets/sheet1.xml")
        self.assertEqual(rows[1], ["R22", ""])


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd pipelines/eunis && python3 -m unittest test_xlsx_to_csv -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'xlsx_to_csv'`.

- [ ] **Step 3: Write the XLSX reader**

Create `pipelines/eunis/xlsx_to_csv.py` starting with the reader (stdlib only):

```python
#!/usr/bin/env python3
"""Convert the pinned EUNIS/ESy XLSX artifacts into normalized CSVs.

XLSX parsing lives here, not in the Go binary: situs' dependency list has no
spreadsheet reader, and an .xlsx is just a zip of XML that the stdlib reads.
"""
import csv
import json
import zipfile
import xml.etree.ElementTree as ET

NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}


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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd pipelines/eunis && python3 -m unittest test_xlsx_to_csv -v`
Expected: PASS (both tests).

- [ ] **Step 5: Add the conversion + measurement CLI**

Append to `xlsx_to_csv.py` a `main()` that takes `--eunis-xlsx`,
`--annex1-xlsx`, `--esy-xlsx` and `--out-dir`, writes the six output files
listed under **Interfaces**, and writes `report.json` with exactly these
measurements (they are the spec's open points 1, 2 and 5, answered with data
instead of assumptions):

```python
report = {
    "syntaxa_ranks": sorted(set(ranks_seen)),          # does it stop at alliance?
    "max_habitat_level": max_level_seen,               # level 3 or deeper?
    "qualifier_values": sorted(set(qualifiers_seen)),  # exact symbols in the file
    "habitat_types": n_types,
    "annex1_crosswalks": n_annex1,
    "annex1_qualifier_histogram": dict(qualifier_counts),
    "types_with_annex1": n_types_with_annex1,
    "types_with_annex1_same": n_types_with_same,       # drives derivable de-labels
}
```

Unknown qualifier symbols must be **counted and reported**, never silently
dropped — a symbol the spec did not anticipate is a finding, not noise.

- [ ] **Step 6: Write the manifest and README**

Create `pipelines/eunis/manifest.yaml` pinning each source with `url`, `sha256`,
`license` and `retrieved` for: the EEA "EUNIS terrestrial habitat classification
2021_1 including crosswalks.xlsx", the "…with crosswalks to Annex I in separate
rows.xlsx" variant, and the ESy `Characteristic-species-combinations.xlsx`
(Zenodo DOI 10.5281/zenodo.3841729, CC BY 4.0). Create
`pipelines/eunis/README.md` (German) documenting how to fetch the artifacts and
run the pipeline. Do **not** check the XLSX files into git.

- [ ] **Step 7: Run against the real artifacts and record the report**

Download the pinned artifacts, run the pipeline, and paste `report.json` into
the commit message. If `syntaxa_ranks` contains a rank below `alliance`, or
`qualifier_values` contains a symbol outside `= < > #`, **stop and report it** —
the spec assumed otherwise and must be updated before Task 5.

- [ ] **Step 8: Commit**

```bash
git add pipelines/eunis
git commit -m "feat(pipeline): convert pinned EUNIS/ESy xlsx artifacts to normalized csv"
```

---

### Task 5: Ingest typologies, habitat types, crosswalks and syntaxa

**Files:**
- Create: `internal/application/ingest.go`, `internal/application/ingest_test.go`
- Create: `cmd/situs/ingest.go`

**Interfaces:**
- Consumes: `output.Repository`/`IngestTx` (Task 3); the CSVs of Task 4.
- Produces:
  - `application.IngestCSV(ctx context.Context, repo output.Repository, dir string) (IngestReport, error)`
  - `application.IngestReport{HabitatTypes, Crosswalks, Syntaxa, SyntaxonLinks, SkippedRows int}`

- [ ] **Step 1: Write the failing test**

Create `internal/application/ingest_test.go`:

```go
package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func writeCSV(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

// seedDir writes a minimal but complete CSV set: one EUNIS type, one annex1
// type, and a '=' crosswalk between them.
func seedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeCSV(t, dir, "typologies.csv",
		"id,scheme,version,name,source_ref\n"+
			"eunis@2021,eunis,2021,EUNIS 2021,https://example.org/eunis\n"+
			"annex1,annex1,,Habitats Directive Annex I,https://example.org/annex1\n")
	writeCSV(t, dir, "habitat_types.csv",
		"typology_id,code,level,name_en,parent_code,priority\n"+
			"eunis@2021,R22,3,Hay meadow,R2,\n"+
			"annex1,6510,,Lowland hay meadows,,0\n")
	writeCSV(t, dir, "crosswalks.csv",
		"from_typology,from_code,to_typology,to_code,qualifier\n"+
			"eunis@2021,R22,annex1,6510,=\n")
	writeCSV(t, dir, "syntaxa.csv",
		"id,rank,name,parent_id\nARR,alliance,Arrhenatherion elatioris,MOL\n")
	writeCSV(t, dir, "habitat_type_syntaxa.csv",
		"typology_id,code,syntaxon_id\neunis@2021,R22,ARR\n")
	return dir
}

func TestIngestCSV_LoadsEverySource(t *testing.T) {
	repo := newFakeRepo()
	rep, err := IngestCSV(context.Background(), repo, seedDir(t))
	if err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}
	if rep.HabitatTypes != 2 {
		t.Errorf("HabitatTypes = %d, want 2", rep.HabitatTypes)
	}
	if rep.Crosswalks != 1 || rep.SyntaxonLinks != 1 {
		t.Errorf("Crosswalks/SyntaxonLinks = %d/%d, want 1/1", rep.Crosswalks, rep.SyntaxonLinks)
	}
	if !repo.committed {
		t.Error("ingest did not commit")
	}
}

// The version crosswalk and the annex1 crosswalk share one table — an ingest
// that special-cases annex1 would break this.
func TestIngestCSV_AnnexOneUsesTheSameCrosswalkTable(t *testing.T) {
	repo := newFakeRepo()
	if _, err := IngestCSV(context.Background(), repo, seedDir(t)); err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}
	want := domain.Crosswalk{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}
	if len(repo.crosswalks) != 1 || repo.crosswalks[0] != want {
		t.Errorf("crosswalks = %+v, want exactly [%+v]", repo.crosswalks, want)
	}
}

// A malformed row must not abort the whole ingest, but it must be counted —
// silent skipping is how coverage gaps hide.
func TestIngestCSV_CountsSkippedRowsInsteadOfFailing(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "crosswalks.csv",
		"from_typology,from_code,to_typology,to_code,qualifier\n"+
			"eunis@2021,R22,annex1,6510,=\n"+
			"eunis@2021,R23,annex1,6520,~\n") // '~' is not a valid qualifier

	repo := newFakeRepo()
	rep, err := IngestCSV(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}
	if rep.Crosswalks != 1 {
		t.Errorf("Crosswalks = %d, want 1 (the valid row)", rep.Crosswalks)
	}
	if rep.SkippedRows != 1 {
		t.Errorf("SkippedRows = %d, want 1 (the bad qualifier)", rep.SkippedRows)
	}
}
```

Also create the fake in the same file:

```go
type fakeRepo struct {
	crosswalks []domain.Crosswalk
	types      []domain.HabitatType
	committed  bool
}

func newFakeRepo() *fakeRepo { return &fakeRepo{} }
```

with `Begin` returning the fake itself and each `Upsert*` appending to a slice,
`Commit` setting `committed = true`, and `Rollback` a no-op.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/ -v`
Expected: FAIL — `undefined: IngestCSV`.

- [ ] **Step 3: Write the implementation**

Create `internal/application/ingest.go` with `IngestCSV` opening each CSV via
`encoding/csv`, mapping header names to indices (never positional — the pipeline
may add columns), parsing values via `domain.ParseTypologyID` /
`domain.ParseQualifier`, calling the matching `Upsert*`, and counting a
`SkippedRows` for every row that fails to parse (logged with file and line).
On any repository error, `Rollback` and return the error.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/application/ -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Wire the cobra command**

Create `cmd/situs/ingest.go` adding `situs ingest --csv-dir <dir> --db <path>`
that opens the SQLite repo, calls `IngestCSV`, and prints the report as JSON.

- [ ] **Step 6: Verify and commit**

Run: `make verify`
Expected: PASS.

```bash
git add internal/application cmd/situs
git commit -m "feat(ingest): load typologies, habitat types, crosswalks and syntaxa from csv"
```

---

### Task 6: Species roles + hostus name crosswalk

**Files:**
- Create: `internal/adapters/hostus/client.go`, `client_test.go`
- Modify: `internal/ports/output/repository.go` (add `NameResolver`)
- Modify: `internal/application/ingest.go` (add `IngestSpeciesRoles`)
- Create: `internal/application/species_ingest_test.go`

**Interfaces:**
- Consumes: `output.IngestTx.UpsertSpeciesRole` (Task 3), `IngestCSV` (Task 5).
- Produces:
  - `output.NameResolver` with
    `Resolve(ctx context.Context, names []string) (map[string]string, error)`
    (verbatim → concept ID; absent key = unresolvable)
  - `hostus.NewClient(baseURL string, httpClient *http.Client) *Client`
  - `application.IngestSpeciesRoles(ctx, repo output.Repository, resolver output.NameResolver, csvPath string) (SpeciesReport, error)`
  - `application.SpeciesReport{Rows, Resolved, Unresolved int}` with
    `func (r SpeciesReport) ResolutionRate() float64`

- [ ] **Step 1: Write the failing client test**

Create `internal/adapters/hostus/client_test.go` using `httptest` — it asserts
the client speaks hostus' real batch contract:

```go
package hostus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ResolveMapsVerbatimToConceptID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/match" {
			t.Errorf("path = %q, want /v1/match", r.URL.Path)
		}
		var req struct {
			Names []struct {
				ID       string `json:"id"`
				Verbatim string `json:"verbatim"`
			} `json:"names"`
			EntryBackbone string `json:"entry_backbone"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if len(req.Names) != 2 {
			t.Errorf("names = %d, want 2 (batched in one call)", len(req.Names))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"0","concept_id":"wcvp:concept:1","match_type":"exact"},
			{"id":"1","match_type":"unresolvable"}]}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client()).Resolve(
		context.Background(), []string{"Inula hirta", "Nonexistent name"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["Inula hirta"] != "wcvp:concept:1" {
		t.Errorf("resolved = %q, want wcvp:concept:1", got["Inula hirta"])
	}
	if _, ok := got["Nonexistent name"]; ok {
		t.Error("unresolvable name must be absent from the map, not empty-valued")
	}
}

func TestClient_ResolveReturnsErrorOnUpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client()).Resolve(context.Background(), []string{"X"}); err == nil {
		t.Error("Resolve returned nil error on 503; ingest must not silently record every name as unresolvable")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/hostus/ -v`
Expected: FAIL — `undefined: NewClient`.

- [ ] **Step 3: Write the client**

Create `internal/adapters/hostus/client.go` posting
`{"names":[{"id":"<index>","verbatim":"<name>"}],"entry_backbone":"wcvp"}` to
`<baseURL>/v1/match`, chunking into batches of 500, mapping each result back by
index, and omitting results without a `concept_id`. Return an error for any
non-200 status.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/hostus/ -v`
Expected: PASS.

- [ ] **Step 5: Write the failing species-ingest test**

Create `internal/application/species_ingest_test.go` asserting that an
unresolvable name is stored with `ConceptID == nil` and a non-empty
`VerbatimName`, and that `SpeciesReport.ResolutionRate()` reports it:

```go
func TestIngestSpeciesRoles_KeepsUnresolvableNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "species_roles.csv")
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Inula hirta,diagnostic,0.8,\n"+
			"eunis@2021,R22,Nonexistent name,constant,,0.5\n")

	repo := newFakeRepo()
	resolver := fakeResolver{"Inula hirta": "wcvp:concept:1"}
	rep, err := IngestSpeciesRoles(context.Background(), repo, resolver, path)
	if err != nil {
		t.Fatalf("IngestSpeciesRoles: %v", err)
	}
	if rep.Rows != 2 || rep.Resolved != 1 || rep.Unresolved != 1 {
		t.Errorf("report = %+v, want Rows 2 / Resolved 1 / Unresolved 1", rep)
	}
	if len(repo.speciesRoles) != 2 {
		t.Fatalf("stored %d roles, want 2 (the unresolvable one is kept)", len(repo.speciesRoles))
	}
	for _, r := range repo.speciesRoles {
		if r.VerbatimName == "Nonexistent name" && r.ConceptID != nil {
			t.Error("unresolvable name stored with a concept id")
		}
		if r.VerbatimName == "" {
			t.Error("verbatim name must always be stored")
		}
	}
	if got := rep.ResolutionRate(); got != 0.5 {
		t.Errorf("ResolutionRate() = %v, want 0.5", got)
	}
}
```

- [ ] **Step 6: Run test to verify it fails, then implement**

Run: `go test ./internal/application/ -run SpeciesRoles -v`
Expected: FAIL — `undefined: IngestSpeciesRoles`.

Then add `IngestSpeciesRoles` to `internal/application/ingest.go`: read the CSV,
collect the distinct verbatim names, call `resolver.Resolve` **once** for all of
them, then upsert every row with `ConceptID` set only when the map has the name.

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/application/ ./internal/adapters/hostus/ -v`
Expected: PASS.

- [ ] **Step 8: Verify and commit**

Run: `make verify`
Expected: PASS.

```bash
git add internal/adapters/hostus internal/application internal/ports
git commit -m "feat(ingest): species roles with hostus name crosswalk and measured resolution rate"
```

---

### Task 7: German localization and derived entry labels

**Files:**
- Create: `internal/application/localize.go`, `internal/application/localize_test.go`
- Modify: `cmd/situs/ingest.go` (call the derivation after the other steps)

**Interfaces:**
- Consumes: `output.IngestTx.UpsertLocalization` (Task 3), crosswalks (Task 5).
- Produces:
  - `application.IngestLocalizations(ctx, repo output.Repository, csvPath string) (int, error)`
    reading `localizations.csv`
    (`entity_type,entity_key,lang,field,value,source,provenance`)
  - `application.DeriveGermanLabels(ctx, repo output.Repository) (int, error)`
  - `output.Repository` gains
    `CrosswalksTo(ctx, typology domain.TypologyID) ([]domain.Crosswalk, error)` and
    `Localization(ctx, entityType, entityKey, lang, field string) ([]domain.Localization, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/application/localize_test.go`:

```go
package application

import (
	"context"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// A '=' crosswalk lends the official German Annex I name to the EUNIS type as
// an entry-level label — clearly marked as derived.
func TestDeriveGermanLabels_CopiesOfficialNameAcrossSameQualifier(t *testing.T) {
	repo := newFakeRepo()
	repo.crosswalks = []domain.Crosswalk{{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}}
	repo.localizations = []domain.Localization{{
		EntityType: "habitat_type", EntityKey: "annex1:6510", Lang: "de",
		Field: "name", Value: "Magere Flachland-Mähwiesen",
		Source: "ffh-richtlinie-de", Provenance: "official",
	}}

	n, err := DeriveGermanLabels(context.Background(), repo)
	if err != nil {
		t.Fatalf("DeriveGermanLabels: %v", err)
	}
	if n != 1 {
		t.Fatalf("derived %d labels, want 1", n)
	}
	got := repo.derivedFor("habitat_type", "eunis@2021:R22")
	if got.Value != "Magere Flachland-Mähwiesen" {
		t.Errorf("Value = %q, want the official Annex I name", got.Value)
	}
	if got.Provenance != "derived" {
		t.Errorf("Provenance = %q, want %q — a derived label must never pose as official", got.Provenance, "derived")
	}
	if got.DerivedFrom != "annex1:6510 qualifier==" {
		t.Errorf("DerivedFrom = %q, want %q", got.DerivedFrom, "annex1:6510 qualifier==")
	}
}

// '<', '>' and '#' are too imprecise to lend a name.
func TestDeriveGermanLabels_IgnoresNonSameQualifiers(t *testing.T) {
	for _, q := range []domain.Qualifier{domain.QualifierNarrower, domain.QualifierBroader, domain.QualifierPartial} {
		repo := newFakeRepo()
		repo.crosswalks = []domain.Crosswalk{{
			From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
			To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
			Qualifier: q,
		}}
		repo.localizations = []domain.Localization{{
			EntityType: "habitat_type", EntityKey: "annex1:6510", Lang: "de",
			Field: "name", Value: "Magere Flachland-Mähwiesen",
			Source: "ffh-richtlinie-de", Provenance: "official",
		}}

		n, err := DeriveGermanLabels(context.Background(), repo)
		if err != nil {
			t.Fatalf("DeriveGermanLabels(%q): %v", q, err)
		}
		if n != 0 {
			t.Errorf("qualifier %q derived %d labels, want 0", q, n)
		}
	}
}

// An existing official/curated German name must never be overwritten by a
// derived one.
func TestDeriveGermanLabels_DoesNotOverrideAnExistingOfficialName(t *testing.T) {
	repo := newFakeRepo()
	repo.crosswalks = []domain.Crosswalk{{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}}
	repo.localizations = []domain.Localization{
		{EntityType: "habitat_type", EntityKey: "annex1:6510", Lang: "de", Field: "name",
			Value: "Magere Flachland-Mähwiesen", Source: "ffh-richtlinie-de", Provenance: "official"},
		{EntityType: "habitat_type", EntityKey: "eunis@2021:R22", Lang: "de", Field: "name",
			Value: "Kuratierter Name", Source: "curated", Provenance: "curated"},
	}

	if _, err := DeriveGermanLabels(context.Background(), repo); err != nil {
		t.Fatalf("DeriveGermanLabels: %v", err)
	}
	if got := repo.localizationFor("habitat_type", "eunis@2021:R22", "curated"); got.Value != "Kuratierter Name" {
		t.Errorf("curated value = %q, want it untouched", got.Value)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/ -run German -v`
Expected: FAIL — `undefined: DeriveGermanLabels`.

- [ ] **Step 3: Write the implementation**

Create `internal/application/localize.go`:

- `IngestLocalizations` reads the CSV and upserts each row verbatim
  (`provenance` comes from the file: `official` for the EUR-Lex directive names,
  `curated` for hand-maintained values).
- `DeriveGermanLabels` fetches crosswalks whose `To.Typology` is `annex1`, keeps
  only `Qualifier.IsSame()`, looks up the `de`/`name` localization of the target,
  skips when the source type already has an `official` or `curated` `de`/`name`,
  and upserts the rest with `Source: "derived-annex1"`, `Provenance: "derived"`,
  `DerivedFrom: fmt.Sprintf("%s qualifier==", c.To)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/application/ -run German -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Wire into the ingest command**

In `cmd/situs/ingest.go`, run the steps in this order and print each count:
`IngestCSV` → `IngestSpeciesRoles` → `IngestLocalizations` → `DeriveGermanLabels`.
Derivation must run **last** — it depends on both crosswalks and official labels.

- [ ] **Step 6: Verify and commit**

Run: `make verify`
Expected: PASS.

```bash
git add internal/application cmd/situs
git commit -m "feat(i18n): german labels plus derived entry labels from '=' annex I crosswalks"
```

---

### Task 8: Read API

**Files:**
- Create: `internal/ports/input/services.go`
- Create: `internal/application/query.go`, `query_test.go`
- Create: `internal/adapters/http/habitat.go`, `species.go`, `handlers_test.go`
- Modify: `internal/adapters/http/server.go` (routes), `openapi.yaml`
- Modify: `internal/adapters/sqlite/read.go` (query implementations)

**Interfaces:**
- Consumes: everything from Tasks 2–7.
- Produces the routes from the spec:
  - `GET /v1/species/{conceptId}/habitat-types`
  - `POST /v1/species/habitat-types`
  - `GET /v1/habitat-type/{typology}/{code}`
  - `GET /v1/habitat-type/{typology}/{code}/species?role=`
  - `GET /v1/syntaxon/{id}/habitat-types`

- [ ] **Step 1: Write the failing handler test**

Create `internal/adapters/http/handlers_test.go`. It pins the two rules a
reviewer must be able to check at a glance — additive localization and the empty
crosswalk list:

```go
func TestHabitatType_ReturnsGermanNameAdditively(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/R22?lang=de", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Typology       string `json:"typology"`
		Code           string `json:"code"`
		NameEN         string `json:"name_en"`
		NameDE         string `json:"name_de"`
		NameDEProvenance string `json:"name_de_provenance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if got.NameEN == "" {
		t.Error("name_en missing — the English name stays the identity even with lang=de")
	}
	if got.NameDE == "" || got.NameDEProvenance == "" {
		t.Errorf("name_de/%s missing, want the German overlay with its provenance", "name_de_provenance")
	}
}

func TestHabitatType_CrosswalksAreEmptyNotNullWhenNoneExist(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/R99", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a type without any annex I match is normal)", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"crosswalks":[]`)) {
		t.Errorf("body = %s, want an empty crosswalks array", rec.Body)
	}
}

func TestHabitatType_UnknownTypologyIsInvalidQuery(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/habitat-type/bogus@1/R22", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
		t.Errorf("body = %s, want the INVALID_QUERY error envelope", rec.Body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/http/ -v`
Expected: FAIL — the routes do not exist yet (404).

- [ ] **Step 3: Implement query service, handlers and routes**

Add `input.QueryService` (methods `HabitatType`, `SpeciesHabitatTypes`,
`HabitatTypeSpecies`, `SyntaxonHabitatTypes`), implement it in
`internal/application/query.go` over `output.Repository`, add the SQLite reads,
and mount the handlers. Rules the handlers must follow:
- `typology` defaults to `eunis@2021` when the caller omits it; an unparseable
  typology is `INVALID_QUERY` (400), an unknown-but-valid one is `NOT_FOUND` (404).
- `lang` (query param or `Accept-Language`) adds `*_de` fields; it never replaces
  `name_en`.
- Slices are always serialized as `[]`, never `null`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/http/ -v`
Expected: PASS.

- [ ] **Step 5: Update the OpenAPI spec and re-run the contract test**

Add every new route to `internal/adapters/http/openapi.yaml` and mirror the file
to `api/openapi/openapi.yaml` byte-identically.

Run: `go test ./internal/adapters/http/ -run TestRoutesMatchOpenAPISpec -v`
Expected: PASS (routes and spec agree in both directions).

- [ ] **Step 6: Full verify and commit**

Run: `make verify`
Expected: PASS.

```bash
git add internal api
git commit -m "feat(api): habitat-type, species and syntaxon read endpoints"
```

- [ ] **Step 7: End-to-end check against real data**

Start hostus locally, run the pipeline against the pinned artifacts, ingest, and
serve situs. Then confirm the main use case end to end:

```bash
curl -s 'localhost:8081/v1/habitat-type/eunis@2021/R22?lang=de' | python3 -m json.tool
curl -s 'localhost:8081/v1/species/<conceptId>/habitat-types' | python3 -m json.tool
```

Record the ingest report (habitat types, crosswalks, annex1 coverage, species
resolution rate) in the commit message of the final documentation commit.

---

## Self-Review

**Spec coverage:** Ziel/Abgrenzung → Tasks 2–8 (no scoring, no engines, no
association — none present). Architektur/Hostus-Grenze → Tasks 1, 6.
Ubiquitous Language → Global Constraints + Tasks 2, 3. Datenmodell (all eight
tables) → Task 3. Ingest steps 1–6 → Tasks 4, 5, 6, 7. Read-API + `lang` →
Task 8. Fehlerbehandlung → Global Constraints + Task 8 Step 3. Test-/
Qualitätsansatz → Task 1 (gates) + TDD throughout. Offene Punkte 1, 2, 5 →
Task 4 Step 5 `report.json`; open point 3 (resolution rate) → Task 6; open
point 4 + 6 (licenses/pinning) → Task 4 Step 6.

**Known gap, deliberately deferred:** the `situs bundle` export and MCP debug
adapter that hostus has are not part of this foundation — add them only when a
consumer needs them (YAGNI).

**Type consistency:** `HabitatTypeKey` is used identically in Tasks 2–8;
`Qualifier`/`IsSame()` in Tasks 2, 5, 7; `IngestTx` method names in Tasks 3, 5,
6, 7; `NameResolver.Resolve` in Task 6 only. `domain.Typology` (entity) and
`domain.TypologyID` (value) are distinct on purpose and used consistently.
