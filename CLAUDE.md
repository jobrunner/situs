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
  `unknown_backbone` (the id prefix is not `wcvp:`) or `unknown_concept`
  (prefix `wcvp:`, no facts — the data's limit). The prefix is checked against
  a **compile-time constant**, deliberately not against what `/v1/info`
  measures: deriving it would cost a query per batch. The reference run showed
  that assumption has a hole — see the mixed-backbone note below and issue #36.
- `species_distribution` in the index, filled by `IngestDistribution` (one
  hostus request per concept, paced at 70 ms — measured **3323** concepts, and
  the whole `situs ingest` measured 5:46). A source outage does **not** abort
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

**German labels are in.** `pipelines/eurlex` pins the official Annex I names
(CELEX `01992L0043-20130701`) and merges them with the situs-authored EUNIS
names in `data/localizations-de-situs.csv`; the 2026-09-16 reference run
measured `Localizations: 567` and `DerivedLabels: 29`. That file covers EUNIS
**levels 1 to 3**: levels 1 and 2 carry no `=`-crosswalk at all (measured: none
of the 49 codes), so no derivation ever reaches them. Since 2026-09-21 it holds
290 authored types / 394 rows. Since 2026-09-23 the **25 syntaxa formations**
carry German names too (`data/localizations-de-syntaxa.csv`, 28 rows: 25 names
plus 3 `vernacular`s, all `provenance: situs`), served as `name_de` on every
`SyntaxonRef` under `?lang=de`. Deeper ranks are deliberately NOT translated:
from class down the names are nomenclatural Latin with an author citation, and
an alliance inheriting its formation's label would simply be named wrong. The
next full ingest measures `Localizations: 655`.

**The index really does carry mixed backbones.** That run measured
`concept_backbones: ["cdm", "eurosl", "wcvp"]` — 8 of 3323 concepts are
aggregates WCVP has no concept for, so the batch route answers
`unknown_backbone` for ids this very index issued. Open design question, see
issue #36. See `docs/reference/measured-index.md` for every measured figure.

- `third_party/claude-skills` (git submodule, SSH remote) + `.claude/skills/*`
  symlinks — the `new-go-service` skill resolves through them. The submodule is
  **not** at `vendor/`: a `vendor/` directory at the module root switches the Go
  toolchain into vendor mode and breaks every bare `go build`/`go test`.
- The design documents (see below).

**The syntaxa-quellenumkehr (Teilprojekt A) is done.** FloraVeg.EU (the
EuroVegChecklist) is now the **primary** source of the syntaxa hierarchy, not
an overlay on the EEA-EUNIS units. The join between the two sources runs over
the **eea code** (`syntaxon.eea_code`), never over a name comparison —
the earlier name-only join silently mismatched real cases (measured: the
`AMM-02B` successor is `JD02`, not the name-similar `JE01`). The formation
level (25 EuroVegChecklist sections A–Y, `data/syntaxa_formations.csv`) and
the life-form group (`phanerogam` / `bryophyte_lichen` / `algae`, set only on
formation rows) are both derivable from the data, not invented. A full
`situs ingest` against the real artifacts measures **25** formations, **150**
classes, **381** orders, **1326** alliances, **0** orphans, **10** derived
parents (sibling consensus), **16** EEA-only rows, and **190** cryptogam
alliances (sections R–Y) where the pre-reversal index carried 2. The
2026-08-30 spec (`2026-08-30-situs-syntaxa-hierarchie-design.md`) is revised
by the 2026-09-21 one; see `docs/reference/measured-index.md` for every
figure with its query. The association-level ceiling (see "Known ceiling"
below) is unchanged by this reversal and still applies.

**Teilprojekt B (Syntaxa-Navigation) is done.** `GET /v1/syntaxa` and
`GET /v1/syntaxon/{id}` are the two new routes over the hierarchy A builds:
the first lists syntaxa by rank/life-form-group filter (defaulting to the 25
formations), the second returns one syntaxon with its ancestor path
(outermost-first) and direct children. From the 25 formations, every one of
the **1326** alliances is reachable by following `children` alone, with no id
known in advance — measured end to end, see
`docs/reference/measured-index.md`. The explorer at `GET /` has its first
rendered panel for the hierarchy alongside the existing raw-JSON view.

**Teilprojekt C (Syntaxa-Verbreitung) is done — all 12 tasks.** The second
area scheme (`evc_territory`, 136 EVC territories, domain
`SchemeEVCTerritory`) sits alongside `wgsrpd_l3` with no mapping between the
two. `SyntaxonDetail.distribution` is present with data when the source has a
coverage row and absent when it does not — the fourth-valued distinction from
`in_area`'s three, see the invariant below. `GET /v1/syntaxa?area=&include=`
filters by occurrence while keeping the unjudgeable syntaxa unmarked, and
`GET /v1/info` now measures `syntaxon_area_scheme` and
`syntaxa_with_distribution` the same way it already measured the species side.
`pipelines/evc-distribution/` and `application.IngestSyntaxonDistribution` are
wired into `situs ingest` as local overlays, after `IngestSyntaxa` (the
syntaxon ids must exist first) and after the area-name overlay (which now
loads both `wgsrpd_areas.csv` and `evc_territories.csv` through one loader).
Task 12 measured the whole pipeline against the real pinned artifact: **1114**
of 1326 alliances carry a distribution statement, **none** of the 190
bryophyte/lichen/algae alliances (sections R–Y) does — held by
`internal/application/distribution_integrity_test.go` — and the one
alliance the distribution source names but the hierarchy does not,
`CI01E`, is reported by id in `UnknownSyntaxa` rather than silently dropped or
silently swallowed. See `docs/reference/measured-index.md` for every figure
and the query it came from. Deliberately out of scope: scoring/ranking, the
ESy rule engine, the EUNIS-2012 key, full plot classification, co-occurrence
ranking, an Article-17 filter, syntaxa distribution's inheritance upward
(class/order/formation), and any ISO↔WGSRPD mapping (the frontend derives the
area code from GPS).

## Design documents (read these before implementing)

| Document | What it settles |
|---|---|
| `docs/superpowers/specs/2026-08-18-situs-foundation-design.md` | The design: scope, data model, ingest, read API. **Authoritative.** |
| `docs/superpowers/plans/2026-08-19-situs-foundation.md` | The 8-task TDD implementation plan. |
| `docs/superpowers/specs/2026-08-20-autarke-laufzeit-und-verbreitung-design.md` | The autark read side and the area filter. **Authoritative** for both. |
| `docs/superpowers/plans/2026-08-20-autarke-laufzeit-und-verbreitung.md` | Its 7-task TDD plan. |
| `docs/research/situs-eea-eunis-2021-spike.md` | What the EEA data actually provides (measured, not assumed). |
| `docs/research/sp9-esy-spike.md` | The ESy rule set: obtainable, parsable, and its hard scope limit. |
| `docs/superpowers/specs/2026-09-21-syntaxa-quellenumkehr-design.md` | Teilprojekt A: FloraVeg.EU as the primary syntaxa source, full formation→class→order→alliance hierarchy. **Authoritative**, revises `2026-08-30-situs-syntaxa-hierarchie-design.md`. |
| `docs/superpowers/specs/2026-09-21-syntaxa-navigation-design.md` | Teilprojekt B: the HTTP navigation routes over the hierarchy A builds. **Authoritative.** |
| `docs/superpowers/plans/2026-09-21-syntaxa-navigation.md` | Its TDD implementation plan (6 tasks). |
| `docs/superpowers/specs/2026-09-21-syntaxa-verbreitung-design.md` | Teilprojekt C: syntaxa distribution, the second area scheme (`evc_territory`), join over the EVC primary codes A introduces. **Authoritative.** |
| `docs/superpowers/plans/2026-09-21-syntaxa-verbreitung.md` | Its TDD implementation plan (12 tasks); all 12 done. |
| `docs/superpowers/specs/2026-09-26-habitat-matching-design.md` | Artenliste → Rangliste der Habitattypen. **Revidiert** die „kein Scoring/Ranking"-Entscheidung des Fundaments, mit der Messung als Begründung. Umgesetzt. |

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
pipelines/eurovegchecklist/ # FloraVeg.EU XLSX -> syntaxa_hierarchy.csv, the
                    # PRIMARY syntaxa source (python3, stdlib only)
pipelines/evc-distribution/ # Zenodo alliance-distribution XLSX ->
                    # syntaxon_distribution.csv + coverage + territories
                    # (python3, stdlib only)
pipelines/wgsrpd/   # TDWG tblLevel3.txt -> wgsrpd_areas.csv (python3, stdlib only)
pipelines/floraveg-factsheets/ # EUNIS-ESy factsheet PDF -> habitat_descriptions.csv
                    # (python3 stdlib + poppler's pdftohtml, an external CLI tool)
data/               # curated, versioned (not pipeline-generated): localizations-de-situs.csv,
                    # annex1_descriptions.csv, localizations_descriptions.csv,
                    # syntaxa_formations.csv (the formation-level primary source, 25 rows A-Y)
                    # and localizations-de-syntaxa.csv (their German names, read
                    # straight by the ingest as localizations_syntaxa.csv)
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

`pipelines/eive` and `pipelines/tichy` are a **documented, deliberate
exception**: their `convert.py` imports `openpyxl`, carried over unchanged
from hostus (see
`docs/superpowers/specs/2026-08-29-situs-trait-modul-design.md`). Rewriting
them onto a stdlib-only XLSX reader was judged too risky for the trait-module
fix round given they are unmodified, working converters. `build.sh` in both
guards the `openpyxl` import up front with a clear error instead of a raw
traceback. `pipelines/midolo` (plain CSV, no XLSX) and `pipelines/eunis`
remain stdlib-only.

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
- **Serving opens the index read-only.** `internal/app` uses
  `sqlite.OpenReadOnly` (`file:<path>?mode=ro`) and nothing else; the allowlist
  in `internal/app/arch_test.go` enforces it. A read-write handle would create a
  missing index (green health, empty answers), force the file into WAL and so
  make even a pure reader need the `-wal`/`-shm` sidecars, and require a
  writable directory — all three break replacing the index underneath a running
  container. `situs ingest` ends with `FinalizeForServing` (checkpoint +
  `journal_mode=DELETE`) so the shipped index is a single file.
  `immutable=1` is deliberately NOT set: it would switch off SQLite's own change
  detection. Opening also verifies the schema (every table `schema.sql` creates
  plus the columns `Migrate` adds), so an empty file, an interrupted copy, a
  foreign database or an index from an older release fails at startup instead of
  serving `INTERNAL_ERROR` behind a green health check.
- **A failed read-only open is diagnosed from the FILE, never from the result
  code.** The same broken index answers `SQLITE_READONLY_DIRECTORY (1544)` from
  a chmod'ed directory and `SQLITE_CANTOPEN (14)` from a read-only bind mount —
  and 14 is also what a missing file returns. `indexFileHint` reads the header
  instead: byte 18 == 2 means WAL, which is what every index built before 0.11.0
  carries and the single most likely reason a container will not start after the
  upgrade. Mapping a code to a guess sent operators to check permissions that
  were fine.
- **Index paths are escaped into the SQLite URI**, `%` first, then `?` and `#`
  (`fileURI`), on BOTH openers. Measured against modernc.org/sqlite v1.56.0:
  unescaped, the driver truncates the DSN at the first `?` — an ingest to
  `/srv/with?chars/index.sqlite` silently built `/srv/with` and reported success
  — and SQLite percent-decodes the filename, so a CI-style `feature%2Fbranch`
  directory resolved to a path that does not exist.
- **`in_area` is three-valued.** `true`, `false`, or the field absent when it is
  unknowable (no concept id, or a concept with no distribution rows). Never
  collapse the third state into `false`, and `only_in_area` must keep the
  unknowables — a list that silently drops what it cannot judge is dishonestly
  clean.
- **Syntaxa-Verbreitung ist vierwertig.** `verified`, `uncertain`,
  `absence` (Coverage-Zeile, keine Verbreitungszeile) und `unknown`
  (keine Coverage-Zeile). `absence` und `unknown` dürfen an keiner Stelle
  zusammengeworfen werden: `SyntaxonDetail.distribution` **fehlt** bei
  `unknown` und trägt bei `absence` zwei leere Listen, und ein
  `?area=`-Filter behält die Unbeurteilbaren unmarkiert. Gemessen sind
  212 der 1326 Verbände `unknown`, darunter alle 190 Moos-, Flechten-
  und Algenverbände — die Quelle deckt nur *vascular-plant dominated
  vegetation* ab.
- **Es gibt zwei Gebietsschemata und keine Abbildung zwischen ihnen.**
  `wgsrpd_l3` für Arten, `evc_territory` für Syntaxa. Keine Antwort
  rechnet ein Territorium in einen WGSRPD-Code um — dieselbe Haltung wie
  bei ISO↔WGSRPD. Jede Abfrage, die Gebiete führt, ist
  schemaparametrisiert und liest die Abdeckung aus der Tabelle des
  jeweiligen Schemas.
- **Syntaxa-Labels gibt es nur auf Formationsebene, und sie werden nie
  vererbt.** `SyntaxonRef.name_de` ist mit `?lang=de` auf den 25 Formationen
  gesetzt und fehlt auf Klasse, Ordnung und Verband. Ein Fallback auf das
  Label der Formation wäre kein Notbehelf, sondern ein falscher Name; das
  Fehlen ist die richtige Aussage. `name` bleibt in jedem Fall die Identität.
  Die Label-Abfrage einer Liste ist **eine** Abfrage
  (`LocalizationsByEntityType`), nicht eine je Zeile — 1326 Verbände über 25
  Labels.
- **`children` and `ancestors` are always in the JSON, even empty — never
  `omitempty`, never `nil`.** A formation with no ancestors and an alliance
  with no children are the normal case for `GET /v1/syntaxon/{id}`, not an
  error; the field's absence would read as "unknown" where it means "none".
- **Every non-formation syntaxon row has a parent, or the ingest fails.** A
  `syntaxon` row with `rank <> 'formation'` and an empty `parent_id` is a
  broken hierarchy, not a partial one — the whole point of the syntaxa
  hierarchy is a chain that reaches a formation, and a chain that breaks at
  one point makes the orientation service worthless at exactly that point.
  `IngestSyntaxa` tries a FloraVeg name match, then unanimous sibling
  consensus within the EEA order group; a row neither resolves is reported
  as an orphan and the whole ingest fails, not just a warning.
  `internal/application/hierarchy_integrity_test.go` pins both halves of
  the promise against a fixture index: no orphans, and every row reaches a
  formation in at most three steps.
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

Also deliberately out of scope for this foundation: the ESy rule engine and the
EUNIS-2012 key (both need cover and region data), and full plot classification.

**Scoring/ranking was out of scope and no longer is.** `POST
/v1/habitat-types/match` ranks habitat types for an observed species list —
see `docs/superpowers/specs/2026-09-26-habitat-matching-design.md`, which
revises the foundation decision and carries the measurement that motivated it:
62 % of the 3561 species in the 198 level-3 types with a species list occur in
exactly one type, and 53 % of all type pairs share no species at all. The score
is a log-likelihood, never a probability — only the ordering is sound.

## Quality Gates

- `make verify` (fmt-check, vet, lint, test, arch, debt, build) must be green
  before every commit. `debt` is both ratchets: the suppression budget and the
  coverage floors.
- Zero `//nolint` / `#nosec` — the debt-guard baseline in `.debt-budget` is 0,
  and zero `TODO`/`FIXME`/`HACK`/`XXX` markers in Go files.
- `.coverage-floors` is a raise-only ratchet (`make debt-coverage`): lower a
  floor only with a written justification, and raise it when coverage improves.
- Mutation testing (gremlins v0.6.0) runs in CI **and locally, macOS included**
  (`make mutation`); the earlier "panics on macOS" note was wrong. Thresholds are
  per package in `.mutation-thresholds`, a raise-only ratchet enforced by
  `scripts/mutation-gate.sh`. **Never invoke gremlins with a `...` wildcard** —
  it does not expand it and silently generates zero mutants, which is how the
  gate stayed vacuous through the whole foundation. The script runs one package
  per invocation and fails on a package that yields no mutants unless
  `.mutation-thresholds` declares `NONE` for it.
  The vacuity was **not** a version problem: v0.5.1 and v0.6.0 produce identical
  mutant sets on a concrete package under Go 1.26 (measured). Don't chase the pin.
- CodeCharta is the third ratchet (`make codecharta`, wired as in ortus; needs
  node + java). `.codecharta-ratchet.json` holds three caps enforced by
  `scripts/codecharta-ratchet.py`: per-file complexity, per-**function**
  complexity, and the hotspot intersection (complex AND under-tested). The
  per-function cap is the one that matters — the per-file sum can be satisfied by
  splitting a file, since the sum moves with the code. Lower the baselines as code
  improves; raising one needs a written justification, same rule as the other two
  ratchets. The hotspot allowlist is empty and should stay that way.
- TDD: write the failing test, watch it fail, then implement.
- `golangci-lint` must be built with a Go ≥ the `go` directive in `go.mod`,
  otherwise it refuses to load the config. Install a matching one with
  `GOTOOLCHAIN=go1.26.6 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`.

## Git Workflow

1. Always create a feature branch (`feature/...`); never commit to `main`.
2. Conventional commits (`feat:`, `fix:`, `docs:`, `chore:` …).
3. `VERSION` and `CHANGELOG.md` are owned by **release-please** — never
   hand-edit them in a feature PR.
4. **A PR with open review threads is not done.** Every review comment —
   `copilot-pull-request-reviewer` included — is either fixed or declined with
   technical reasoning, answered in its thread, and the thread resolved. The
   Copilot review lands minutes *after* `gh pr create`, so look again rather
   than assuming silence means clean. The `Stop` hook in `.claude/settings.json`
   enforces this (`.claude/hooks/pr-review-threads-guard.sh`).

## Code Style

- `README.md` in German (hostus convention).
- Code comments sparse, English, and only where they explain *why*.
- OpenAPI is kept in two byte-identical copies (embedded + `api/openapi/`) and
  guarded by the routes↔spec contract test.
