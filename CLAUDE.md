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

**The foundation plan (8 tasks) and the "autarke Laufzeit und Verbreitung" plan
(7 tasks) are both done.** The service is scaffolded with the full quality
harness (module `github.com/jobrunner/situs`, Go 1.26, hexagonal package layout,
ratchets, CI, release-please, MkDocs), and ingest plus read API are implemented:
domain value objects, the sqlite index, the hostus adapter, the CSV ingest, the
localization overlay with `=`-only German derivation, and the habitat-type /
species / syntaxon read endpoints.

The second plan made the read side **autark and area-aware**:

- **No hostus in the serve path.** The batch route takes `concept_ids`, not
  verbatim `names`; every read answers from the local index alone. Held by
  `internal/app/arch_test.go`, which forbids `internal/app` from even importing
  the hostus adapter. `SITUS_HOSTUS_*` is ingest-only configuration.
- Each batch entry reports `known` plus, when false, a `reason` of exactly
  `unknown_backbone` (the id prefix is not `wcvp:` — the caller's fault) or
  `unknown_concept` (prefix `wcvp:`, no facts — the data's limit). The prefix is
  checked against a **compile-time constant**, deliberately not against what
  `/v1/info` measures: deriving it would cost a query per batch, and a
  mixed-backbone index is not a state this design carries.
- `species_distribution` in the index, filled by `IngestDistribution` (one
  hostus request per concept, paced at 70 ms — measured **3135** concepts, and
  the whole `situs ingest` measured 6:00). A source outage does **not** abort
  the ingest: the distribution is extra information, so it is warned about and
  the run continues.
- `?area=` (WGSRPD level 3) and `?only_in_area=` on the species lists. `in_area`
  is three-valued (`true` / `false` / field absent when unknowable), and
  `only_in_area` drops only the definite `false`s. An unknown area code is
  `INVALID_QUERY`, never a list of "does not occur".
- `GET /v1/info` carries an `index` object (`concept_backbones`,
  `species_with_concept`, `area_scheme`, `areas_with_data`), every figure
  measured from the index. Deliberately not in it: the backbone's *fassung*
  (e.g. `wcvp 2026-06-15`) — the ingest does not record it, and it is a schema
  extension of its own.

Not in the index yet, deliberately: **no German labels.** The overlay mechanism
is built and tested, but no German name source is pinned (EUR-Lex is deferred),
so `Localizations` and `DerivedLabels` are measured 0. See
`docs/reference/measured-index.md` for every measured figure.

- `third_party/claude-skills` (git submodule, SSH remote) + `.claude/skills/*`
  symlinks — the `new-go-service` skill resolves through them. The submodule is
  **not** at `vendor/`: a `vendor/` directory at the module root switches the Go
  toolchain into vendor mode and breaks every bare `go build`/`go test`.
- The design documents (see below).

**Next action:** none in either plan — `feature/autark-runtime` is ready to
merge. Deliberately out of scope so far: scoring/ranking, the ESy rule engine,
the EUNIS-2012 key, full plot classification, co-occurrence ranking, an
Article-17 filter, syntaxa distribution, and any ISO↔WGSRPD mapping (the
frontend derives the area code from GPS).

## Design documents (read these before implementing)

| Document | What it settles |
|---|---|
| `docs/superpowers/specs/2026-08-18-situs-foundation-design.md` | The design: scope, data model, ingest, read API. **Authoritative.** |
| `docs/superpowers/plans/2026-08-19-situs-foundation.md` | The 8-task TDD implementation plan. |
| `docs/superpowers/specs/2026-08-20-autarke-laufzeit-und-verbreitung-design.md` | The autark read side and the area filter. **Authoritative** for both. |
| `docs/superpowers/plans/2026-08-20-autarke-laufzeit-und-verbreitung.md` | Its 7-task TDD plan. |
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
  ports/output/     # driven ports (Repository, IngestTx, NameResolver,
                    #               DistributionSource)
  application/      # use cases: ingest, localize, query
  adapters/
    sqlite/         # local index (modernc.org/sqlite)
    hostus/         # NameResolver (POST /v1/match) + DistributionSource
                    # (GET /v1/concept/{id}) — INGEST ONLY, never in serve
    http/           # gorilla/mux router + handlers + OpenAPI
  app/              # composition root
  config/           # SITUS_-prefixed config
pipelines/eunis/    # XLSX -> normalized CSV (python3, stdlib only)
```

Boundaries are enforced by depguard in the linter (`make arch`), not convention.
`gomodguard_v2` enforces the allowed-library list the same way: a new direct
dependency fails the build until it is added to `.golangci.yml` on purpose.

### HTTP conventions (hostus twin)

- Business routes live under **`/v1`** (not `/api/v1`); the spec is served at
  **`GET /openapi`** as the embedded YAML, and `/metrics`, `/health/live`,
  `/health/ready` are the operations surface.
- Every mounted route must declare `.Methods()` and must appear in
  `internal/adapters/http/openapi.yaml`; the contract test checks **both**
  directions and fails on a route without an explicit method.
- Error envelope: `{"error":{"code":"...","message":"..."}}` with exactly three
  codes: `INVALID_QUERY`, `NOT_FOUND`, `INTERNAL_ERROR`.
  **Two codes are decidedly not emitted.** `UPSTREAM_UNAVAILABLE` is gone with
  the runtime hostus dependency — no read path has an upstream that could fail.
  `UNRESOLVABLE` never existed: a concept id the index cannot answer is a normal
  200 carrying `known: false` and a `reason`; the input is reported back, never
  dropped, and one unknown id must not fail a batch of 300. Recorded in
  `openapi.yaml` and `docs/reference/http-api.md`.

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
- **Serving stays autark.** No read path may reach for hostus or any other
  upstream, and `internal/app` may not import the hostus adapter.
- **`in_area` is three-valued.** `true`, `false`, or the field absent when it is
  unknowable (no concept id, or a concept with no distribution rows). Never
  collapse the third state into `false`, and `only_in_area` must keep the
  unknowables — a list that silently drops what it cannot judge is dishonestly
  clean.
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

- `make verify` (fmt-check, vet, lint, test, arch, debt, build) must be green
  before every commit. `debt` is both ratchets: the suppression budget and the
  coverage floors.
- Zero `//nolint` / `#nosec` — the debt-guard baseline in `.debt-budget` is 0,
  and zero `TODO`/`FIXME`/`HACK`/`XXX` markers in Go files.
- `.coverage-floors` is a raise-only ratchet (`make debt-coverage`): lower a
  floor only with a written justification, and raise it when coverage improves.
- Mutation testing (gremlins) runs **in CI** — it panics on macOS.
- TDD: write the failing test, watch it fail, then implement.
- `golangci-lint` must be built with a Go ≥ the `go` directive in `go.mod`,
  otherwise it refuses to load the config. Install a matching one with
  `GOTOOLCHAIN=go1.26.6 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`.

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
