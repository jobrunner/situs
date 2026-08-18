# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working in this repository.

## Project Overview

**situs** is a local, read-only service for **EUNIS habitat types**: it answers
*species → habitat types (with role)* and *habitat type → species / syntaxa /
crosswalks* from pinned EEA and ESy artifacts, with German labels and an
Annex I (FFH-LRT) crosswalk.

Purpose: an excursion app records plants in the field. A coordinate-derived
habitat guess is unreliable (a small steppe patch inside a large wet forest
loses to the big polygon). A plant list narrows the candidates independently —
and given one character species, situs names the other character species worth
looking for.

situs is a **sibling of hostus, not part of it**. hostus stays pure name
resolution (`verbatim → concept`); situs holds every habitat/vegetation fact.
situs calls hostus **at ingest** to crosswalk source species names to concept
IDs; at runtime it is autark for concept-ID queries.

## Current State — START HERE

The repo is **bootstrapped but not yet built**. What exists:

- `vendor/claude-skills` (git submodule, SSH remote) + `.claude/skills/*`
  symlinks — the `new-go-service` skill is available and resolves.
- The design documents (see below). **No Go code yet.**

**Next action:** implement `docs/superpowers/plans/2026-08-19-situs-foundation.md`
using the **superpowers:subagent-driven-development** skill.

Task 1 of that plan scaffolds the service via the `new-go-service` skill. Its
**Steps 1, 2 and 5 are already done** (repo init, submodule, symlinks,
`.gitignore`, docs copied) — start Task 1 at **Step 3** (read
`.claude/skills/new-go-service/SKILL.md` and execute its Steps 1–10).

## Design documents (read these before implementing)

| Document | What it settles |
|---|---|
| `docs/superpowers/specs/2026-08-18-situs-foundation-design.md` | The design: scope, data model, ingest, read API. **Authoritative.** |
| `docs/superpowers/plans/2026-08-19-situs-foundation.md` | The 8-task TDD implementation plan. |
| `docs/research/situs-eea-eunis-2021-spike.md` | What the EEA data actually provides (measured, not assumed). |
| `docs/research/sp9-esy-spike.md` | The ESy rule set: obtainable, parsable, and its hard scope limit. |

## Ubiquitous Language (do not deviate)

- **Habitat typology** (`habitat_typology`) — a classification system in a given
  fassung: `eunis@2021`, `eunis@2012`, `annex1`. A further system is **one row**,
  not a schema change.
- **Habitat type** (`habitat_type`) — an *abstract type* within a typology,
  identified **always** by `(typology, code)`. It is explicitly **not** a biotope
  in the landscape. Never name a table, type, or route just `habitat`.
- **One crosswalk mechanism** — the EUNIS version crosswalk and the
  EUNIS↔Annex I crosswalk are the same concept (both use `=`/`<`/`>`/`#`) and
  share **one** table and **one** route family.
- **Identifiers stay international**: the Annex I typology id is `annex1`.
  "FFH-LRT"/"Lebensraumtyp" is a German *label* and lives in `localization`.

## Architecture

Hexagonal (ports & adapters), same shape as hostus:

```
cmd/situs/          # thin entrypoint + cobra commands (serve, ingest, version)
internal/
  domain/           # TypologyID, HabitatTypeKey, Qualifier, entities — no I/O deps
  ports/input/      # driving ports (what the app offers)
  ports/output/     # driven ports (Repository, IngestTx, NameResolver)
  application/      # use cases: ingest, localize, query
  adapters/
    sqlite/         # local index (modernc.org/sqlite)
    hostus/         # NameResolver via hostus POST /v1/match
    http/           # gorilla/mux router + handlers + OpenAPI
  app/              # composition root
  config/           # SITUS_-prefixed config
pipelines/eunis/    # XLSX -> normalized CSV (python3, stdlib only)
```

Boundaries are enforced by depguard in the linter (`make arch`), not convention.

## Technical Constraints

### Allowed libraries only
Go stdlib, `github.com/gorilla/mux`, `github.com/spf13/viper`,
`github.com/spf13/cobra`, `modernc.org/sqlite` (pure-Go, CGO-free),
OpenTelemetry Go SDK (+`otelmux`), official Prometheus Go client.

**No** ORMs, no reflection-heavy dependencies, and **no XLSX library**.

### Why XLSX parsing is not in the binary
An `.xlsx` is a zip of XML that the Python stdlib reads. Keeping it in
`pipelines/eunis/` (bash + **stdlib-only** `python3`) keeps the Go dependency
list narrow. The Go ingest reads **only CSV**. This mirrors hostus'
`pipelines/floraveg/`.

### Invariants that reviewers must check
- **Localization is overlay, never replacement.** `habitat_type.name_en` stays
  the identity; `name_de` is additive. `provenance` ∈ `official` | `curated` |
  `derived`.
- **Derived German labels only from qualifier `=`.** Never from `<`, `>`, `#` —
  those correspondences are too imprecise to lend a name.
- **Missing data is absence of rows**, never a placeholder code. A habitat type
  without an Annex I correspondence is the normal case.
- **Unresolvable species names are kept**, not dropped: `verbatim_name` always
  set, `concept_id` NULL, and the resolution rate is measured and reported.
- **Measure, do not assume.** The pipeline emits a `report.json` (syntaxa depth,
  the qualifier symbols actually present, Annex I coverage). If the data
  contradicts the spec, stop and report — do not silently adapt.
- SQL statements are static strings with `?` placeholders — never concatenate
  values (gosec G201/G202 fail the build).

## Known ceiling (decided, not an oversight)

The free EEA/Euroveg data reaches **EUNIS level 3** and **alliance (Verband)**.
**Associations are not available** in any pan-European free source; they would
need EVA (European Vegetation Archive) access, which is only worth requesting
once a real scientific project justifies it. Do not design around associations.

Also deliberately out of scope for this foundation: scoring/ranking (plant set →
ranked habitats), the ESy rule engine and the EUNIS-2012 key (both need cover
and region data), and full plot classification.

## Quality Gates

- `make verify` (fmt-check, vet, lint, test, arch, debt) must be green before
  every commit.
- Zero `//nolint` / `#nosec` — the debt-guard baseline is 0.
- Mutation testing (gremlins) runs **in CI** — it panics on macOS.
- TDD: write the failing test, watch it fail, then implement.

## Git Workflow

1. Always create a feature branch (`feature/...`); never commit to `main`.
2. Conventional commits (`feat:`, `fix:`, `docs:`, `chore:` …).
3. `VERSION` and `CHANGELOG.md` are owned by **release-please** — never
   hand-edit them in a feature PR.

## Code Style

- `README.md` in German (hostus convention).
- Code comments sparse, English, and only where they explain *why*.
- OpenAPI is kept in two byte-identical copies (embedded + `api/openapi/`) and
  guarded by the routes↔spec contract test.
