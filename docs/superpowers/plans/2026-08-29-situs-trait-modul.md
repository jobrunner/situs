# Trait-Modul Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** situs bekommt ein eigenständiges Lesemodul für pflanzenökologische
Zeigerwerte (EIVE, Tichý et al. 2023, Midolo et al. 2023): eine Konzept-ID,
ein Aufruf, die Werte aller drei Vokabulare — `GET
/v1/species/{conceptId}/traits[?vocab=eive]`.

**Architecture:** Erweiterung des bestehenden Ingest-/Read-Musters, keine neue
Mechanik. Drei Python-Pipelines (unverändert aus
`/tmp/hostus-traits-transfer/{eive,tichy,midolo}/` übernommen) erzeugen
kanonische, **pipe-getrennte** CSVs. Ein neuer Ingest-Schritt
(`application.IngestTraits`) sammelt alle distinkten Taxa über die drei
Dateien, löst sie in **einem** `hostus.Client.Resolve()`-Aufruf auf und
schreibt `trait_value`-Zeilen, geschlüsselt auf `concept_id`. Der Read-Pfad
bleibt autark: `QueryService.Traits` liest ausschließlich aus dem lokalen
Index.

**Tech Stack:** Go 1.26 (`encoding/csv` mit `Comma='|'`), `modernc.org/sqlite`,
`gorilla/mux`; Python-Pipelines unverändert aus dem hostus-Transfer.

**Spec:** `docs/superpowers/specs/2026-08-29-situs-trait-modul-design.md`
(Branch `feature/trait-modul`, Commit `e99424e`)

## Global Constraints

- Kanonisches CSV-Format aller drei Quellen (gemessen, nicht aus der Spec
  übernommen — die Spec nennt es "CSV", real ist es **pipe-getrennt**):
  `taxon|vocab|vocab_version|dim|value|niche_width|n_systems`. Kopfzeile
  identisch in allen drei Dateien.
- Zeilenzahlen (Kopf + Daten, gemessen via `wc -l`): `eive-canonical.csv`
  71267, `tichy-canonical.csv` 45593, `midolo-canonical.csv` 31911.
- `NicheWidth`/`NSystems` sind `nil`, nicht `0`/`0.0`, wenn ein Vokabular sie
  nicht liefert (Tichý, Midolo liefern nie; EIVE immer).
- Ein Konzept mit Trait-Daten in mehreren Vokabularen liefert sie **niemals**
  vermischt — jede `domain.TraitSet` trägt genau ein `Vocab`+`VocabVersion`.
- Fehlende CSV-Datei eines Vokabulars: überspringen, `TraitReport.Skipped`
  vermerkt, **kein** Abbruch (Trait-Daten sind Zusatzinformation, wie die
  Verbreitung).
- hostus beim `Resolve()`-Aufruf nicht erreichbar: harter Fehler, Ingest
  bricht ab (anders als eine fehlende Datei).
- Ein nicht auflösbares Taxon zählt als `Unresolved`, die Zeile wird
  verworfen (nicht wie bei `SpeciesRole` mit `ConceptID=nil` gespeichert —
  `trait_value`s Primärschlüssel verlangt `concept_id`).
- Serving bleibt autark: `IngestTraits`/`hostus.Client` laufen ausschließlich
  in `cmd/situs/ingest.go`, nie im Serve-Pfad. Der bestehende
  `internal/app/arch_test.go` deckt das bereits ab, ohne Anpassung.
- `?vocab=` ist optional, EIN Wert (kein Komma-getrennter Mehrfach-Filter).
  Unbekannter Wert → `400 INVALID_QUERY`. Ein Konzept ohne Trait-Daten → `200`
  mit leerem Array, kein 404.
- SQL-Statements bleiben statische Strings mit `?`-Platzhaltern (gosec
  G201/G202); ein `IN (...)`-Platzhalterstring wird wie in
  `appendAreasForChunk` (`internal/adapters/sqlite/read.go:152`) nur aus der
  Anzahl der Werte gebaut, nie aus den Werten selbst.
- Zwei neue Vokabulare (`trait_value`, `trait_vocabulary`) tragen **keine**
  `FOREIGN KEY`-Constraint — wie jede andere Tabelle im Schema (siehe
  `schema.sql`s Kopfkommentar): die Quelle ist ein rebuildbarer Ingest-Lauf,
  kein Constraint-geschütztes System.
- Ein Port-Methoden-Zusatz gegenüber der Spec (siehe Task 2): `IngestTx`
  bekommt `UpsertTraitVocabulary(vocab, version string) error` — die Spec
  definiert die Tabelle `trait_vocabulary`, aber keine Schreibmethode dafür.
  Ohne sie bliebe die Tabelle für immer leer.

---

### Task 1: Pipelines aus dem hostus-Transfer übernehmen

**Files:**
- Create: `pipelines/eive/build.sh`, `pipelines/eive/convert.py`
- Create: `pipelines/tichy/build.sh`, `pipelines/tichy/convert.py`
- Create: `pipelines/midolo/build.sh`, `pipelines/midolo/convert.py`
- Create: `pipelines/eive/README.md`, `pipelines/tichy/README.md`,
  `pipelines/midolo/README.md`
- Test: keine Go-Tests; Verifikation ist ein Probelauf jedes `build.sh`

**Interfaces:**
- Produces: drei kanonische CSVs unter `pipelines/{eive,tichy,midolo}/output/`
  im Format `taxon|vocab|vocab_version|dim|value|niche_width|n_systems`, die
  Task 5 (`IngestTraits`) als Eingabe braucht.

- [ ] **Step 1: Dateien kopieren**

```bash
mkdir -p pipelines/eive pipelines/tichy pipelines/midolo
cp /tmp/hostus-traits-transfer/eive/build.sh /tmp/hostus-traits-transfer/eive/convert.py pipelines/eive/
cp /tmp/hostus-traits-transfer/tichy/build.sh /tmp/hostus-traits-transfer/tichy/convert.py pipelines/tichy/
cp /tmp/hostus-traits-transfer/midolo/build.sh /tmp/hostus-traits-transfer/midolo/convert.py pipelines/midolo/
chmod +x pipelines/eive/build.sh pipelines/tichy/build.sh pipelines/midolo/build.sh
```

Gemessen (nicht angenommen): keines der drei `build.sh`/`convert.py`
referenziert `hostus` — `grep -rn hostus /tmp/hostus-traits-transfer/` liefert
keinen Treffer. `REPO_ROOT` wird nur für einen optionalen Cache-Reuse
(`${REPO_ROOT}/poc/data/...`) verwendet; existiert dieser Pfad in `situs`
nicht, fällt das Skript auf den regulären `curl`-Download zurück — kein
Umbau nötig.

- [ ] **Step 2: `situs`-Zielpfad statt `hostus` in Kommentaren prüfen**

Jedes `build.sh` leitet `REPO_ROOT` bereits relativ zu seinem eigenen
`SCRIPT_DIR` ab (`"${SCRIPT_DIR}/../.."`), nicht über einen hartcodierten
`hostus`-Pfad — nach dem Kopieren nach `situs/pipelines/{eive,tichy,midolo}/`
zeigt `REPO_ROOT` bereits korrekt auf den `situs`-Klon. Keine Änderung am
Skriptinhalt nötig. Verifiziere das per Blick in die Datei:

```bash
grep -n "REPO_ROOT" pipelines/eive/build.sh
```

Erwartet: `REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"` — ein reiner
Pfadausdruck, kein Repo-Name.

- [ ] **Step 3: README je Pipeline schreiben**

`pipelines/eive/README.md`:

```markdown
# eive

EIVE 1.0 (Dengler et al. 2023) — europaweite Zeigerwerte (M/N/R/L/T),
Neuberechnung der klassischen Ellenberg-Werte. Quelle: Zenodo-Record
10.5281/zenodo.7534792, Datei `EIVE_Paper_1.0_SM_08.xlsx`, Sheet
"mainTable". CC-BY-4.0 — jede Weiterverwendung muss Dengler et al. (2023),
Vegetation Classification and Survey 4: 7-29 (https://doi.org/10.3897/VCS.98324)
zitieren.

```bash
./build.sh
```

Lädt (oder nutzt einen Cache), konvertiert nach `output/eive-canonical.csv`
im kanonischen, **pipe-getrennten** Format
`taxon|vocab|vocab_version|dim|value|niche_width|n_systems`, das
`situs ingest` einliest. `niche_width`/`n_systems` sind bei EIVE immer
gefüllt.
```

`pipelines/tichy/README.md`:

```markdown
# tichy

Tichý et al. (2023) Indicator values, Vokabular `tichy2023`, Version 2.0.
Quelle: Zenodo-Record 10.5281/zenodo.7427088, Datei
`Indicator.values-tables-2022-11-07-Zenodo.v2.xlsx`.

```bash
./build.sh
```

Erzeugt `output/tichy-canonical.csv`, kanonisches Format wie bei `eive`
(siehe dessen README). `niche_width`/`n_systems` sind bei Tichý nie gefüllt.
```

`pipelines/midolo/README.md`:

```markdown
# midolo

Midolo et al. (2023) Disturbance indicator values, Vokabular `midolo2023`,
Version 3. Quelle: Zenodo-Record 10.5281/zenodo.7116957, Datei
`disturbance_indicator_values.csv`.

```bash
./build.sh
```

Erzeugt `output/midolo-canonical.csv`, kanonisches Format wie bei `eive`
(siehe dessen README). `niche_width`/`n_systems` sind bei Midolo nie
gefüllt.
```

- [ ] **Step 4: Probelauf und Format verifizieren**

```bash
cd pipelines/eive && ./build.sh && cd ../..
head -3 pipelines/eive/output/eive-canonical.csv
```

Erwartet: Kopfzeile
`taxon|vocab|vocab_version|dim|value|niche_width|n_systems`, danach
pipe-getrennte Datenzeilen. Ein Netzwerkfehler beim Download ist in einer
Sandbox ohne Internetzugang möglich — in dem Fall reicht die Verifikation
über die bereits vorhandene Kopie unter
`/tmp/hostus-traits-transfer/eive/output/eive-canonical.csv` (identischer
Inhalt, dasselbe `convert.py`).

- [ ] **Step 5: Commit**

```bash
git add pipelines/eive pipelines/tichy pipelines/midolo
git commit -m "feat(pipelines): eive/tichy/midolo trait pipelines from hostus transfer"
```

---

### Task 2: Domänenmodell + Port-Erweiterung + fakeRepo

**Files:**
- Create: `internal/domain/trait.go`
- Create: `internal/domain/trait_test.go`
- Modify: `internal/ports/output/repository.go`
- Modify: `internal/application/ingest_test.go` (erweitert `fakeRepo` um die
  vier neuen Methoden — sonst kompiliert **kein** Test im Paket
  `internal/application` mehr, sobald die Interfaces erweitert sind, denn
  `fakeRepo` implementiert dort sowohl `output.Repository` als auch
  `output.IngestTx`)

**Interfaces:**
- Produces: `domain.TraitDim`, `domain.ParseTraitDim`, `domain.TraitValue`,
  `domain.TraitSet`; `output.IngestTx.UpsertTraitValue(conceptID string, tv
  domain.TraitValue) error`; `output.IngestTx.UpsertTraitVocabulary(vocab,
  version string) error`; `output.Repository.Traits(ctx, conceptID string,
  vocabs []string) ([]domain.TraitSet, error)`;
  `output.Repository.KnownVocabs(ctx context.Context) ([]string, error)`.
  Task 3 (sqlite) implementiert alle vier; Task 5 (`IngestTraits`) ruft
  `UpsertTraitValue`/`UpsertTraitVocabulary`; Task 7 (Read-API) ruft
  `Traits`/`KnownVocabs`.

- [ ] **Step 1: Fehlschlagenden Domain-Test schreiben**

`internal/domain/trait_test.go`:

```go
package domain

import "testing"

func TestParseTraitDim_RejectsEmpty(t *testing.T) {
	if _, err := ParseTraitDim(""); err == nil {
		t.Fatal("ParseTraitDim(\"\") = nil error, want error")
	}
	if _, err := ParseTraitDim("   "); err == nil {
		t.Fatal("ParseTraitDim(\"   \") = nil error, want error")
	}
}

func TestParseTraitDim_TrimsAndAccepts(t *testing.T) {
	dim, err := ParseTraitDim("  M  ")
	if err != nil {
		t.Fatalf("ParseTraitDim: %v", err)
	}
	if dim != TraitDim("M") {
		t.Errorf("dim = %q, want %q", dim, "M")
	}
}
```

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/domain/... -run TestParseTraitDim -v`
Expected: FAIL (`ParseTraitDim`/`TraitDim` undefined)

- [ ] **Step 3: Domänenmodell implementieren**

`internal/domain/trait.go`:

```go
package domain

import (
	"fmt"
	"strings"
)

// TraitDim identifies one measured dimension within a trait vocabulary
// (e.g. "M" for EIVE's moisture axis, "disturbance_severity" for Midolo's
// overall disturbance intensity). Not a closed enum: each vocabulary
// defines its own set of dimensions, situs keeps no cross-vocabulary
// dimension registry.
type TraitDim string

// ParseTraitDim validates only that s is not empty — the spelling is a
// per-vocabulary convention, not a situs-wide register.
func ParseTraitDim(s string) (TraitDim, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("trait dimension is empty")
	}
	return TraitDim(s), nil
}

// TraitValue is one indicator value for one concept in one trait
// vocabulary. NicheWidth/NSystems are nil when the vocabulary does not
// provide them (Tichý/Midolo never do; EIVE always does) — never coerced to
// 0.0/0.
type TraitValue struct {
	Vocab        string
	VocabVersion string
	Dim          TraitDim
	Value        float64
	NicheWidth   *float64
	NSystems     *int
}

// TraitSet groups every TraitValue one vocabulary contributes for one
// concept — never merged across vocabularies (M in EIVE and M in Tichý are
// different measurements on different scales).
type TraitSet struct {
	Vocab        string
	VocabVersion string
	Values       []TraitValue
}
```

- [ ] **Step 4: Test ausführen, Erfolg bestätigen**

Run: `go test ./internal/domain/... -run TestParseTraitDim -v`
Expected: PASS

- [ ] **Step 5: Port-Interfaces erweitern**

In `internal/ports/output/repository.go`, `IngestTx` erweitern (nach
`UpsertDistribution`, vor `Commit`):

```go
	// UpsertTraitValue writes one trait_value row for conceptID. Idempotent
	// like every other Upsert here: a repinned vocabulary is simply
	// re-ingested.
	UpsertTraitValue(conceptID string, tv domain.TraitValue) error
	// UpsertTraitVocabulary records that vocab/version was (re-)ingested —
	// pure ingest metadata for later drift detection, no factual content for
	// the reader. Idempotent.
	UpsertTraitVocabulary(vocab, version string) error
```

`Repository` erweitern (nach `ConceptIDs`, letzte Methode im Interface):

```go
	// Traits returns every domain.TraitSet situs holds for conceptID,
	// grouped PER VOCABULARY, never mixed. vocabs empty means every
	// ingested vocabulary; otherwise filtered (serves GET .../traits?vocab=).
	Traits(ctx context.Context, conceptID string, vocabs []string) ([]domain.TraitSet, error)
	// KnownVocabs lists the trait vocabularies the index has data for. A
	// ?vocab= filter must be validated against this: an unknown value is an
	// error, not an empty answer that could be a typo.
	KnownVocabs(ctx context.Context) ([]string, error)
```

- [ ] **Step 6: `fakeRepo` erweitern (sonst kompiliert `internal/application`
  nicht mehr)**

In `internal/application/ingest_test.go`, im `fakeRepo`-Struct-Literal (nach
`distribution []fakeDistribution`) zwei Felder ergänzen:

```go
	distribution  []fakeDistribution
	traitValues   []fakeTraitValue
	traitVocabs   []fakeTraitVocab
```

Nach dem `fakeDistribution`-Typ (vor `func newFakeRepo`) zwei Hilfstypen
ergänzen:

```go
// fakeTraitValue is one recorded UpsertTraitValue call.
type fakeTraitValue struct {
	ConceptID string
	Value     domain.TraitValue
}

// fakeTraitVocab is one recorded UpsertTraitVocabulary call.
type fakeTraitVocab struct {
	Vocab, Version string
}
```

Nach `func (r *fakeRepo) UpsertDistribution(...)` (Zeile ~620) die vier neuen
Methoden ergänzen:

```go
func (r *fakeRepo) UpsertTraitValue(conceptID string, tv domain.TraitValue) error {
	if err := r.failIfNamed("UpsertTraitValue"); err != nil {
		return err
	}
	r.traitValues = append(r.traitValues, fakeTraitValue{ConceptID: conceptID, Value: tv})
	return nil
}

func (r *fakeRepo) UpsertTraitVocabulary(vocab, version string) error {
	if err := r.failIfNamed("UpsertTraitVocabulary"); err != nil {
		return err
	}
	r.traitVocabs = append(r.traitVocabs, fakeTraitVocab{Vocab: vocab, Version: version})
	return nil
}

func (r *fakeRepo) Traits(_ context.Context, conceptID string, vocabs []string) ([]domain.TraitSet, error) {
	sets := map[string]*domain.TraitSet{}
	var order []string
	for _, tv := range r.traitValues {
		if tv.ConceptID != conceptID {
			continue
		}
		if len(vocabs) > 0 && !slices.Contains(vocabs, tv.Value.Vocab) {
			continue
		}
		key := tv.Value.Vocab + "@" + tv.Value.VocabVersion
		set, ok := sets[key]
		if !ok {
			set = &domain.TraitSet{Vocab: tv.Value.Vocab, VocabVersion: tv.Value.VocabVersion}
			sets[key] = set
			order = append(order, key)
		}
		set.Values = append(set.Values, tv.Value)
	}
	out := make([]domain.TraitSet, 0, len(order))
	for _, key := range order {
		out = append(out, *sets[key])
	}
	return out, nil
}

func (r *fakeRepo) KnownVocabs(_ context.Context) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, tv := range r.traitVocabs {
		if !seen[tv.Vocab] {
			seen[tv.Vocab] = true
			out = append(out, tv.Vocab)
		}
	}
	return out, nil
}
```

`fakeRepo.Traits` braucht `"slices"` im Import-Block von `ingest_test.go` —
prüfe per `grep -n '"slices"' internal/application/ingest_test.go`, ob der
Import schon existiert; falls nicht, ergänzen.

- [ ] **Step 7: Build und volle Testsuite des Pakets prüfen**

Run: `go build ./... && go test ./internal/domain/... ./internal/application/... -v`
Expected: PASS, kein Kompilierfehler — `fakeRepo` erfüllt beide Interfaces
weiterhin vollständig.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/trait.go internal/domain/trait_test.go \
        internal/ports/output/repository.go internal/application/ingest_test.go
git commit -m "feat(domain): TraitDim/TraitValue/TraitSet + IngestTx/Repository trait ports"
```

---

### Task 3: sqlite-Adapter (Schema + Read/Write)

**Files:**
- Modify: `internal/adapters/sqlite/schema.sql`
- Create: `internal/adapters/sqlite/trait.go`
- Create: `internal/adapters/sqlite/trait_test.go`

**Interfaces:**
- Consumes: `domain.TraitValue`, `domain.TraitSet`, `domain.TraitDim` (Task 2)
- Produces: `(*DB).UpsertTraitValue`, `(*DB).UpsertTraitVocabulary` auf
  `ingestTx`; `(*DB).Traits`, `(*DB).KnownVocabs` auf `DB` — von Task 5
  (Ingest) und Task 7 (Read-API) verwendet.

- [ ] **Step 1: Schema erweitern**

In `internal/adapters/sqlite/schema.sql`, am Ende anfügen:

```sql
-- Pflanzenökologische Zeigerwerte (EIVE, Tichý, Midolo). Jede Zeile ist ein
-- Wert einer Dimension in einem Vokabular für ein Konzept; niche_width/
-- n_systems sind NULL, wenn das Vokabular sie nicht liefert (Tichý/Midolo
-- nie, EIVE immer) — nie 0/0.0.
CREATE TABLE IF NOT EXISTS trait_value (
  concept_id    TEXT NOT NULL,
  vocab         TEXT NOT NULL,
  vocab_version TEXT NOT NULL,
  dim           TEXT NOT NULL,
  value         REAL NOT NULL,
  niche_width   REAL,
  n_systems     INTEGER,
  PRIMARY KEY (concept_id, vocab, vocab_version, dim)
);
CREATE INDEX IF NOT EXISTS idx_trait_value_concept_id ON trait_value(concept_id);

-- Reines Ingest-Metadatum: wann wurde welche Vokabular-Version zuletzt
-- geschrieben. Kein fachlicher Inhalt für den Leser.
CREATE TABLE IF NOT EXISTS trait_vocabulary (
  vocab       TEXT NOT NULL,
  version     TEXT NOT NULL,
  ingested_at TEXT NOT NULL,
  PRIMARY KEY (vocab, version)
);
```

- [ ] **Step 2: Fehlschlagenden Test schreiben**

`internal/adapters/sqlite/trait_test.go`:

```go
package sqlite

import (
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func TestTraitValue_RoundTripsWithNicheWidthAndNSystems(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	nw, n := 2.47, 1
	tv := domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.22, NicheWidth: &nw, NSystems: &n}
	if err := tx.UpsertTraitValue("wcvp:1", tv); err != nil {
		t.Fatalf("UpsertTraitValue: %v", err)
	}
	if err := tx.UpsertTraitVocabulary("eive", "1.0"); err != nil {
		t.Fatalf("UpsertTraitVocabulary: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	sets, err := db.Traits(ctx, "wcvp:1", nil)
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("len(sets) = %d, want 1", len(sets))
	}
	if len(sets[0].Values) != 1 {
		t.Fatalf("len(Values) = %d, want 1", len(sets[0].Values))
	}
	got := sets[0].Values[0]
	if got.NicheWidth == nil || *got.NicheWidth != nw {
		t.Errorf("NicheWidth = %v, want %v", got.NicheWidth, nw)
	}
	if got.NSystems == nil || *got.NSystems != n {
		t.Errorf("NSystems = %v, want %v", got.NSystems, n)
	}
}

func TestTraitValue_NicheWidthAndNSystemsStayNilWhenAbsent(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	tv := domain.TraitValue{Vocab: "tichy2023", VocabVersion: "2.0", Dim: "T", Value: 4.7}
	if err := tx.UpsertTraitValue("wcvp:2", tv); err != nil {
		t.Fatalf("UpsertTraitValue: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	sets, err := db.Traits(ctx, "wcvp:2", nil)
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	got := sets[0].Values[0]
	if got.NicheWidth != nil {
		t.Errorf("NicheWidth = %v, want nil", *got.NicheWidth)
	}
	if got.NSystems != nil {
		t.Errorf("NSystems = %v, want nil", *got.NSystems)
	}
}

func TestTraits_GroupsPerVocabularyNeverMixed(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTraitValue("wcvp:3", domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.2}); err != nil {
		t.Fatalf("UpsertTraitValue eive: %v", err)
	}
	if err := tx.UpsertTraitValue("wcvp:3", domain.TraitValue{Vocab: "tichy2023", VocabVersion: "2.0", Dim: "M", Value: 5.1}); err != nil {
		t.Fatalf("UpsertTraitValue tichy2023: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	sets, err := db.Traits(ctx, "wcvp:3", nil)
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("len(sets) = %d, want 2 (never merged across vocabularies)", len(sets))
	}
	for _, s := range sets {
		if len(s.Values) != 1 {
			t.Errorf("vocab %s: len(Values) = %d, want 1", s.Vocab, len(s.Values))
		}
	}
}

func TestTraits_FiltersByVocab(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTraitValue("wcvp:4", domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.2}); err != nil {
		t.Fatalf("UpsertTraitValue eive: %v", err)
	}
	if err := tx.UpsertTraitValue("wcvp:4", domain.TraitValue{Vocab: "midolo2023", VocabVersion: "3", Dim: "disturbance_severity", Value: 0.7}); err != nil {
		t.Fatalf("UpsertTraitValue midolo2023: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	sets, err := db.Traits(ctx, "wcvp:4", []string{"midolo2023"})
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 1 || sets[0].Vocab != "midolo2023" {
		t.Fatalf("sets = %+v, want exactly one midolo2023 set", sets)
	}
}

func TestUpsertTraitValue_IsIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	upsert := func(value float64) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if err := tx.UpsertTraitValue("wcvp:5", domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: value}); err != nil {
			t.Fatalf("UpsertTraitValue: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}
	upsert(1.0)
	upsert(2.0) // repinned value overwrites, no duplicate row

	sets, err := db.Traits(ctx, "wcvp:5", nil)
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 1 || len(sets[0].Values) != 1 {
		t.Fatalf("sets = %+v, want exactly one set with one value", sets)
	}
	if sets[0].Values[0].Value != 2.0 {
		t.Errorf("Value = %v, want 2.0 (repinned overwrite, no duplicate)", sets[0].Values[0].Value)
	}
}

func TestKnownVocabs_ListsDistinctIngestedVocabularies(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTraitVocabulary("eive", "1.0"); err != nil {
		t.Fatalf("UpsertTraitVocabulary: %v", err)
	}
	if err := tx.UpsertTraitVocabulary("tichy2023", "2.0"); err != nil {
		t.Fatalf("UpsertTraitVocabulary: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.KnownVocabs(ctx)
	if err != nil {
		t.Fatalf("KnownVocabs: %v", err)
	}
	want := []string{"eive", "tichy2023"}
	if len(got) != len(want) {
		t.Fatalf("KnownVocabs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("KnownVocabs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTraits_UnknownConceptReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	sets, err := db.Traits(t.Context(), "wcvp:does-not-exist", nil)
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("sets = %+v, want empty", sets)
	}
}
```

- [ ] **Step 3: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/adapters/sqlite/... -run TestTraitValue -v`
Expected: FAIL (`UpsertTraitValue`/`Traits`/`KnownVocabs` undefined auf `*DB`
bzw. `*ingestTx`)

- [ ] **Step 4: Adapter implementieren**

`internal/adapters/sqlite/trait.go`:

```go
package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
)

// UpsertTraitValue writes one trait_value row for conceptID. Idempotent
// like every other Upsert here: a repinned vocabulary is simply
// re-ingested.
func (t *ingestTx) UpsertTraitValue(conceptID string, tv domain.TraitValue) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO trait_value (concept_id, vocab, vocab_version, dim, value, niche_width, n_systems)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(concept_id, vocab, vocab_version, dim) DO UPDATE SET
		   value=excluded.value, niche_width=excluded.niche_width, n_systems=excluded.n_systems`,
		conceptID, tv.Vocab, tv.VocabVersion, string(tv.Dim), tv.Value, tv.NicheWidth, tv.NSystems)
	if err != nil {
		return fmt.Errorf("sqlite: upserting trait value %s/%s/%s for %s: %w",
			tv.Vocab, tv.VocabVersion, tv.Dim, conceptID, err)
	}
	return nil
}

// UpsertTraitVocabulary records that vocab/version was (re-)ingested. Pure
// ingest metadata, no factual content for the reader.
func (t *ingestTx) UpsertTraitVocabulary(vocab, version string) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO trait_vocabulary (vocab, version, ingested_at)
		 VALUES (?, ?, datetime('now'))
		 ON CONFLICT(vocab, version) DO UPDATE SET ingested_at=excluded.ingested_at`,
		vocab, version)
	if err != nil {
		return fmt.Errorf("sqlite: upserting trait vocabulary %s %s: %w", vocab, version, err)
	}
	return nil
}

// Traits returns every domain.TraitSet situs holds for conceptID, grouped
// per vocabulary, never mixed. An empty vocabs slice means every ingested
// vocabulary.
func (d *DB) Traits(ctx context.Context, conceptID string, vocabs []string) ([]domain.TraitSet, error) {
	query := `SELECT vocab, vocab_version, dim, value, niche_width, n_systems
	          FROM trait_value WHERE concept_id = ?`
	args := []any{conceptID}
	if len(vocabs) > 0 {
		// Only placeholders are generated here, never values — the vocab
		// names stay arguments, so this is not SQL construction from input
		// (gosec G201/G202), same idiom as appendAreasForChunk.
		placeholders := strings.Repeat(",?", len(vocabs))[1:]
		query += ` AND vocab IN (` + placeholders + `)`
		for _, v := range vocabs {
			args = append(args, v)
		}
	}
	query += ` ORDER BY vocab, vocab_version, dim`

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading traits of %s: %w", conceptID, err)
	}
	defer func() { _ = rows.Close() }()

	sets := map[string]*domain.TraitSet{}
	var order []string
	for rows.Next() {
		var vocab, vocabVersion, dim string
		var tv domain.TraitValue
		if err := rows.Scan(&vocab, &vocabVersion, &dim, &tv.Value, &tv.NicheWidth, &tv.NSystems); err != nil {
			return nil, fmt.Errorf("sqlite: scanning trait value of %s: %w", conceptID, err)
		}
		tv.Vocab, tv.VocabVersion, tv.Dim = vocab, vocabVersion, domain.TraitDim(dim)
		key := vocab + "@" + vocabVersion
		set, ok := sets[key]
		if !ok {
			set = &domain.TraitSet{Vocab: vocab, VocabVersion: vocabVersion}
			sets[key] = set
			order = append(order, key)
		}
		set.Values = append(set.Values, tv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating traits of %s: %w", conceptID, err)
	}

	out := make([]domain.TraitSet, 0, len(order))
	for _, key := range order {
		out = append(out, *sets[key])
	}
	return out, nil
}

// KnownVocabs lists the distinct trait vocabularies the index has data for.
// A ?vocab= filter is validated against this, same role as KnownAreaCodes
// plays for ?area=.
func (d *DB) KnownVocabs(ctx context.Context) ([]string, error) {
	rows, err := d.QueryContext(ctx, `SELECT DISTINCT vocab FROM trait_vocabulary ORDER BY vocab`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading known vocabs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []string{}
	for rows.Next() {
		var vocab string
		if err := rows.Scan(&vocab); err != nil {
			return nil, fmt.Errorf("sqlite: scanning vocab: %w", err)
		}
		out = append(out, vocab)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating known vocabs: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 5: Test ausführen, Erfolg bestätigen**

Run: `go test ./internal/adapters/sqlite/... -v`
Expected: PASS, alle Tests inklusive der bereits bestehenden

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/sqlite/schema.sql internal/adapters/sqlite/trait.go internal/adapters/sqlite/trait_test.go
git commit -m "feat(sqlite): trait_value/trait_vocabulary tables + Traits/KnownVocabs"
```

---

### Task 4: `readAll`/`csvReader` bekommen ein konfigurierbares Trennzeichen

**Files:**
- Modify: `internal/application/ingest.go`
- Modify: `internal/application/localize.go`
- Modify: `internal/application/species_ingest.go`
- Modify: `internal/application/syntaxa_hierarchy_ingest.go`

**Interfaces:**
- Produces: `readAll(ctx, dir, name string, delim rune, required []string,
  skip rowSkipper, fn func(...) error) error` (Signaturänderung, ein Parameter
  mehr) — Task 5 (`IngestTraits`) ruft `readAll(..., '|', ...)` für die drei
  pipe-getrennten Trait-CSVs.

Kein neuer Test nötig: dieser Task ändert keine Logik für Komma-getrennte
Dateien (`delim=','` verhält sich exakt wie zuvor), die bestehende
Testsuite jedes betroffenen Pakets ist die Regression.

- [ ] **Step 1: `csvReader`/`readAll` erweitern**

In `internal/application/ingest.go`, `csvReader` ändern:

```go
func csvReader(dir, name string, delim rune) (*csv.Reader, *os.File, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("opening ingest directory %s: %w", dir, err)
	}
	defer func() { _ = root.Close() }() // the returned *os.File keeps its own fd

	f, err := root.Open(name)
	if err != nil {
		return nil, nil, fmt.Errorf("opening %s: %w", filepath.Join(dir, name), err)
	}
	r := csv.NewReader(f)
	r.Comma = delim
	return r, f, nil
}
```

`readAll`s Signatur und den internen Aufruf ändern:

```go
func readAll(ctx context.Context, dir, name string, delim rune, required []string, skip rowSkipper,
	fn func(idx map[string]int, row []string, line int) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("ingesting %s: %w", name, err)
	}

	r, f, err := csvReader(dir, name, delim)
```

(der Rest von `readAll` bleibt unverändert)

- [ ] **Step 2: Bestehende Aufrufer auf `,','` umstellen**

Fünf Stellen in `internal/application/ingest.go` (`ingestTypologies`,
`ingestHabitatTypes`, `ingestCrosswalks`, `ingestSyntaxa`,
`ingestSyntaxonLinks`): jeder `readAll(ctx, dir, file, ...)`-Aufruf bekommt
`','` als viertes Argument, unmittelbar nach `file`. Beispiel für
`ingestTypologies`:

```go
	err = readAll(ctx, dir, file, ',', []string{"id", "scheme", "version", colName, "source_ref"}, skip,
```

Dieselbe Änderung an den vier übrigen `readAll(ctx, dir, file, ...)`-Aufrufen
in derselben Datei.

In `internal/application/localize.go`, `IngestLocalizations`s `readAll`-Aufruf:

```go
	err = readAll(ctx, dir, file, ',',
		[]string{"entity_type", "entity_key", "lang", "field", "value", "source", "provenance"}, skip,
```

In `internal/application/species_ingest.go`, `readSpeciesRows`s
`readAll`-Aufruf:

```go
	err := readAll(ctx, dir, file, ',',
		[]string{colTypologyID, colCode, "verbatim_name", "role", "fidelity", "constancy"}, skip,
```

In `internal/application/syntaxa_hierarchy_ingest.go`, `readHierarchyRows`s
`readAll`-Aufruf:

```go
	err := readAll(ctx, dir, file, ',',
		[]string{"code", "rank", colName, "author", "parent_code"}, skip,
```

- [ ] **Step 3: Bestehende Testsuite als Regression laufen lassen**

Run: `go build ./... && go test ./internal/application/... -v`
Expected: PASS — jeder bereits bestehende Test (Typologien, Habitattypen,
Crosswalks, Syntaxa, Lokalisierungen, Artenrollen, Syntaxa-Hierarchie) läuft
unverändert weiter, da `delim=','` dieselbe Semantik hat wie zuvor der
`encoding/csv`-Standardwert.

- [ ] **Step 4: Commit**

```bash
git add internal/application/ingest.go internal/application/localize.go \
        internal/application/species_ingest.go internal/application/syntaxa_hierarchy_ingest.go
git commit -m "refactor(application): readAll/csvReader take a configurable delimiter"
```

---

### Task 5: `application.IngestTraits`

**Files:**
- Create: `internal/application/trait_ingest.go`
- Create: `internal/application/trait_ingest_test.go`

**Interfaces:**
- Consumes: `readAll(ctx, dir, name, delim, required, skip, fn)` (Task 4),
  `output.IngestTx.UpsertTraitValue`/`UpsertTraitVocabulary`,
  `output.NameResolver.Resolve` (bestehend), `fakeRepo`/`fakeResolver` (Task 2
  bzw. `species_ingest_test.go`)
- Produces: `TraitReport{Rows, Resolved, Unresolved, PerVocab
  map[string]VocabReport, Skipped []string}`, `VocabReport{Rows, Resolved,
  Unresolved}`, `IngestTraits(ctx, repo output.Repository, resolver
  output.NameResolver, csvPaths map[string]string) (TraitReport, error)` —
  Task 6 (`cmd/situs/ingest.go`) ruft `IngestTraits` mit den drei
  CSV-Pfaden.

- [ ] **Step 1: Fehlschlagende Tests schreiben**

`internal/application/trait_ingest_test.go`:

```go
package application

import (
	"context"
	"testing"
)

func seedTraitDir(t *testing.T, eive, tichy, midolo string) string {
	t.Helper()
	dir := t.TempDir()
	if eive != "" {
		writeCSV(t, dir, "eive_traits.csv", eive)
	}
	if tichy != "" {
		writeCSV(t, dir, "tichy_traits.csv", tichy)
	}
	if midolo != "" {
		writeCSV(t, dir, "midolo_traits.csv", midolo)
	}
	return dir
}

func traitCSVPaths(dir string) map[string]string {
	return map[string]string{
		"eive":       dir + "/eive_traits.csv",
		"tichy2023":  dir + "/tichy_traits.csv",
		"midolo2023": dir + "/midolo_traits.csv",
	}
}

func TestIngestTraits_ResolvesAndStoresByConceptID(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n",
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|tichy2023|2.0|T|4.7||\n",
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|midolo2023|3|disturbance_severity|0.7||\n")

	repo := newFakeRepo()
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}
	rep, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir))
	if err != nil {
		t.Fatalf("IngestTraits: %v", err)
	}
	if rep.Rows != 3 || rep.Resolved != 3 || rep.Unresolved != 0 {
		t.Errorf("Rows/Resolved/Unresolved = %d/%d/%d, want 3/3/0", rep.Rows, rep.Resolved, rep.Unresolved)
	}
	if len(repo.traitValues) != 3 {
		t.Fatalf("len(traitValues) = %d, want 3", len(repo.traitValues))
	}
	for _, tv := range repo.traitValues {
		if tv.ConceptID != "wcvp:1" {
			t.Errorf("ConceptID = %q, want wcvp:1", tv.ConceptID)
		}
	}
	if len(repo.traitVocabs) != 3 {
		t.Fatalf("len(traitVocabs) = %d, want 3 (one per vocab file actually read)", len(repo.traitVocabs))
	}
}

func TestIngestTraits_PerVocabBreakdown(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n"+
			"Unknown plant|eive|1.0|M|3.0|1.0|1\n",
		"", "")
	repo := newFakeRepo()
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}
	rep, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir))
	if err != nil {
		t.Fatalf("IngestTraits: %v", err)
	}
	vr, ok := rep.PerVocab["eive"]
	if !ok {
		t.Fatal("PerVocab has no \"eive\" entry")
	}
	if vr.Rows != 2 || vr.Resolved != 1 || vr.Unresolved != 1 {
		t.Errorf("eive VocabReport = %+v, want Rows=2 Resolved=1 Unresolved=1", vr)
	}
	if rep.Unresolved != 1 {
		t.Errorf("rep.Unresolved = %d, want 1", rep.Unresolved)
	}
	// An unresolved taxon's row is dropped, never stored with a nil concept
	// id — trait_value's primary key requires one, unlike species_role.
	if len(repo.traitValues) != 1 {
		t.Errorf("len(traitValues) = %d, want 1 (unresolved row discarded)", len(repo.traitValues))
	}
}

func TestIngestTraits_MissingFileIsSkippedNotAborted(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n",
		"", "")
	repo := newFakeRepo()
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}
	rep, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir))
	if err != nil {
		t.Fatalf("IngestTraits: %v", err)
	}
	want := map[string]bool{"tichy2023": true, "midolo2023": true}
	if len(rep.Skipped) != 2 {
		t.Fatalf("Skipped = %v, want 2 entries", rep.Skipped)
	}
	for _, s := range rep.Skipped {
		if !want[s] {
			t.Errorf("Skipped contains unexpected vocab %q", s)
		}
	}
	if _, ok := rep.PerVocab["tichy2023"]; ok {
		t.Error("PerVocab should not carry an entry for a skipped vocab")
	}
}

func TestIngestTraits_ResolverErrorAbortsTheIngest(t *testing.T) {
	dir := seedTraitDir(t,
		"taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
			"Inula hirta|eive|1.0|M|4.22|2.47|1\n",
		"", "")
	repo := newFakeRepo()
	_, err := IngestTraits(context.Background(), repo, erroringResolver{}, traitCSVPaths(dir))
	if err == nil {
		t.Fatal("IngestTraits with a failing resolver: want error, got nil")
	}
	if len(repo.traitValues) != 0 {
		t.Errorf("traitValues = %v, want none written on resolver failure", repo.traitValues)
	}
}

func TestIngestTraits_NicheWidthAndNSystemsStayNilWhenColumnsAreEmpty(t *testing.T) {
	dir := seedTraitDir(t, "", "taxon|vocab|vocab_version|dim|value|niche_width|n_systems\n"+
		"Inula hirta|tichy2023|2.0|T|4.7||\n", "")
	repo := newFakeRepo()
	resolver := fakeResolver{"Inula hirta": "wcvp:1"}
	if _, err := IngestTraits(context.Background(), repo, resolver, traitCSVPaths(dir)); err != nil {
		t.Fatalf("IngestTraits: %v", err)
	}
	if len(repo.traitValues) != 1 {
		t.Fatalf("len(traitValues) = %d, want 1", len(repo.traitValues))
	}
	tv := repo.traitValues[0].Value
	if tv.NicheWidth != nil {
		t.Errorf("NicheWidth = %v, want nil", *tv.NicheWidth)
	}
	if tv.NSystems != nil {
		t.Errorf("NSystems = %v, want nil", *tv.NSystems)
	}
}
```

- [ ] **Step 2: Tests ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/application/... -run TestIngestTraits -v`
Expected: FAIL (`IngestTraits`/`TraitReport`/`VocabReport` undefined)

- [ ] **Step 3: `IngestTraits` implementieren**

`internal/application/trait_ingest.go`:

```go
package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// pipeDelim is the field separator of the three canonical trait CSVs
// (taxon|vocab|vocab_version|dim|value|niche_width|n_systems) — set by the
// pipelines' own convert.py (measured, not assumed: the transferred files
// use "|" despite the .csv extension). A comma-delimited reader would
// silently misparse every row.
const pipeDelim = '|'

// VocabReport is one vocabulary's slice of TraitReport.
type VocabReport struct {
	Rows       int
	Resolved   int
	Unresolved int
}

// TraitReport summarizes one trait ingest run across all three canonical
// CSVs, resolved through ONE hostus.Resolve() call — the distinct taxa
// across all vocabularies are collected first, not resolved per file.
type TraitReport struct {
	Rows       int
	Resolved   int
	Unresolved int
	PerVocab   map[string]VocabReport
	// Skipped names the vocabularies whose CSV file was missing from
	// csvPaths — trait data is extra information, like distribution, so a
	// missing file does not abort the run.
	Skipped []string
}

type traitRow struct {
	taxon        string
	vocab        string
	vocabVersion string
	dim          string
	value        float64
	nicheWidth   *float64
	nSystems     *int
}

// parseFloat is the required-value counterpart to parseOptionalFloat: the
// "value" column is never absent in a well-formed trait row.
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// readTraitRows parses one canonical pipe-delimited trait CSV, skipping
// (and counting) any row with the wrong field count or an unparseable
// value — the same tolerance as every other ingest file in this package.
func readTraitRows(ctx context.Context, csvPath string, skip rowSkipper) ([]traitRow, error) {
	dir, file := filepath.Split(csvPath)
	var rows []traitRow
	err := readAll(ctx, dir, file, pipeDelim,
		[]string{"taxon", "vocab", "vocab_version", "dim", "value", "niche_width", "n_systems"}, skip,
		func(idx map[string]int, row []string, line int) error {
			value, perr := parseFloat(row[idx["value"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			nicheWidth, perr := parseOptionalFloat(row[idx["niche_width"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			nSystems, perr := parseOptionalInt(row[idx["n_systems"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			rows = append(rows, traitRow{
				taxon:        row[idx["taxon"]],
				vocab:        row[idx["vocab"]],
				vocabVersion: row[idx["vocab_version"]],
				dim:          row[idx["dim"]],
				value:        value,
				nicheWidth:   nicheWidth,
				nSystems:     nSystems,
			})
			return nil
		})
	return rows, err
}

// distinctTaxa returns the deduplicated taxon names across rows, sorted so
// batch composition is reproducible run to run — same rationale as
// species_ingest.go's distinctNames.
func distinctTaxa(rows []traitRow) []string {
	names := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		names[r.taxon] = struct{}{}
	}
	distinct := make([]string, 0, len(names))
	for n := range names {
		distinct = append(distinct, n)
	}
	sort.Strings(distinct)
	return distinct
}

// IngestTraits loads the three canonical trait CSVs named in csvPaths
// (keyed by vocabulary, e.g. {"eive": ".../eive_traits.csv", ...}) into
// repo, crosswalking every distinct taxon across ALL THREE files to a
// hostus concept ID in one Resolve call. A missing file is skipped and
// recorded in Skipped, never aborting the run. A resolver failure aborts
// the whole ingest — otherwise every row would be misrecorded as
// "unresolvable" instead of "hostus was down", the same distinction
// IngestSpeciesRoles already makes. An unresolved taxon's row is dropped
// (not stored with a nil concept id): trait_value's primary key requires
// one, unlike species_role's.
func IngestTraits(ctx context.Context, repo output.Repository, resolver output.NameResolver,
	csvPaths map[string]string) (TraitReport, error) {
	vocabKeys := make([]string, 0, len(csvPaths))
	for vocab := range csvPaths {
		vocabKeys = append(vocabKeys, vocab)
	}
	sort.Strings(vocabKeys) // reproducible order, not map iteration order

	rep := TraitReport{PerVocab: map[string]VocabReport{}, Skipped: []string{}}
	byVocab := map[string][]traitRow{}
	var all []traitRow
	skipped := 0
	for _, vocab := range vocabKeys {
		path := csvPaths[vocab]
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			rep.Skipped = append(rep.Skipped, vocab)
			continue
		}
		_, file := filepath.Split(path)
		skip := newRowSkipper(&skipped, file, "trait value")
		rows, err := readTraitRows(ctx, path, skip)
		if err != nil {
			return TraitReport{}, fmt.Errorf("reading %s trait CSV: %w", vocab, err)
		}
		byVocab[vocab] = rows
		all = append(all, rows...)
	}

	resolved, err := resolver.Resolve(ctx, distinctTaxa(all))
	if err != nil {
		return TraitReport{}, fmt.Errorf("resolving trait taxa via hostus: %w", err)
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return TraitReport{}, fmt.Errorf("beginning trait ingest transaction: %w", err)
	}

	for _, vocab := range vocabKeys {
		rows, ok := byVocab[vocab]
		if !ok {
			continue
		}
		vr, verr := upsertTraitRows(tx, vocab, rows, resolved)
		if verr != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return TraitReport{}, fmt.Errorf("%w (rollback also failed: %w)", verr, rbErr)
			}
			return TraitReport{}, verr
		}
		rep.PerVocab[vocab] = vr
		rep.Rows += vr.Rows
		rep.Resolved += vr.Resolved
		rep.Unresolved += vr.Unresolved
	}

	if err := tx.Commit(); err != nil {
		return TraitReport{}, fmt.Errorf("committing trait ingest transaction: %w", err)
	}
	return rep, nil
}

// upsertTraitRows writes vocab's rows via tx, tallying vr accordingly, and
// records the vocabulary once (with the version its own rows carry) so
// trait_vocabulary reflects a file that was processed even if every single
// taxon in it failed to resolve.
func upsertTraitRows(tx output.IngestTx, vocab string, rows []traitRow, resolved map[string]string) (VocabReport, error) {
	vr := VocabReport{Rows: len(rows)}
	var version string
	for _, r := range rows {
		version = r.vocabVersion
		conceptID, ok := resolved[r.taxon]
		if !ok {
			vr.Unresolved++
			continue
		}
		tv := domain.TraitValue{
			Vocab:        r.vocab,
			VocabVersion: r.vocabVersion,
			Dim:          domain.TraitDim(r.dim),
			Value:        r.value,
			NicheWidth:   r.nicheWidth,
			NSystems:     r.nSystems,
		}
		if err := tx.UpsertTraitValue(conceptID, tv); err != nil {
			return VocabReport{}, err
		}
		vr.Resolved++
	}
	if len(rows) > 0 {
		if err := tx.UpsertTraitVocabulary(vocab, version); err != nil {
			return VocabReport{}, err
		}
	}
	return vr, nil
}
```

- [ ] **Step 4: Tests ausführen, Erfolg bestätigen**

Run: `go test ./internal/application/... -run TestIngestTraits -v`
Expected: PASS

- [ ] **Step 5: Volle Paket-Testsuite als Regression**

Run: `go test ./internal/application/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/application/trait_ingest.go internal/application/trait_ingest_test.go
git commit -m "feat(application): IngestTraits — one resolver call across three vocabularies"
```

---

### Task 6: Einhängung in `situs ingest`

**Files:**
- Modify: `cmd/situs/ingest.go`
- Modify: `cmd/situs/ingest_test.go`

**Interfaces:**
- Consumes: `application.IngestTraits(ctx, repo, resolver, csvPaths)` (Task 5)
- Produces: `ingestOutput.Traits application.TraitReport` — im JSON-Report von
  `situs ingest` sichtbar.

- [ ] **Step 1: Bestehenden `ingest_test.go` auf die neue Erwartung
  erweitern**

In `cmd/situs/ingest_test.go` den Test, der die Feldnamen der JSON-Ausgabe
prüft (suche nach `"Localizations"` oder `"DerivedLabels"` im Test), um
`"Traits"` ergänzen. Ist kein solcher Assertion-Block vorhanden, folgenden
Test ergänzen — er nutzt dieselbe Testfixtur wie die bestehenden
`ingest`-Tests dieser Datei (CSV-Verzeichnis, `sqlite.Open` auf ein
`t.TempDir()`):

```go
func TestRunIngest_ReportIncludesTraits(t *testing.T) {
	dir := seedIngestDir(t) // die bestehende Helper-Funktion dieser Datei
	dbPath := filepath.Join(t.TempDir(), "test.sqlite")
	cmd := newIngestCmd()
	cmd.SetArgs([]string{"--csv-dir", dir, "--db", dbPath})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshaling report: %v", err)
	}
	if _, ok := parsed["Traits"]; !ok {
		t.Error("report has no \"Traits\" field")
	}
}
```

Prüfe vor dem Schreiben per `grep -n "func seedIngestDir\|func Test.*Ingest"
cmd/situs/ingest_test.go`, wie die bestehende Testfixtur genau heißt und
aufgerufen wird, und passe den Testnamen/Aufruf entsprechend an, statt ihn zu
duplizieren.

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./cmd/situs/... -run TestRunIngest_ReportIncludesTraits -v`
Expected: FAIL (`parsed["Traits"]` fehlt, da `ingestOutput` das Feld noch
nicht hat — oder der Build schlägt fehl, falls `hostus.NewClient` als
Resolver für `IngestTraits` noch nicht verdrahtet ist)

- [ ] **Step 3: `runIngest` erweitern**

In `cmd/situs/ingest.go`, `ingestOutput` um ein Feld ergänzen:

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
	Traits             application.TraitReport
}
```

In `runIngest`, nach dem `IngestDistribution`-Block (er braucht dieselbe
`resolver`-Variable, unabhängig von `speciesReport`/`distributionReport`) und
vor `IngestLocalizations` einfügen:

```go
	traitCSVPaths := map[string]string{
		"eive":       filepath.Join(csvDir, "eive_traits.csv"),
		"tichy2023":  filepath.Join(csvDir, "tichy_traits.csv"),
		"midolo2023": filepath.Join(csvDir, "midolo_traits.csv"),
	}
	traitReport, err := application.IngestTraits(ctx, db, resolver, traitCSVPaths)
	if err != nil {
		return fmt.Errorf("ingesting traits: %w", err)
	}
```

Im abschließenden `out := ingestOutput{...}`-Literal ergänzen:

```go
	out := ingestOutput{
		IngestReport:       report,
		Species:            speciesReport,
		ResolutionRate:     speciesReport.ResolutionRate(),
		Distribution:       distributionReport,
		DistributionFailed: distSrc.FailedConcepts(),
		Localizations:      localizations,
		DerivedLabels:      derivedLabels,
		SyntaxaHierarchy:   hierarchyReport,
		Traits:             traitReport,
	}
```

- [ ] **Step 4: Test ausführen, Erfolg bestätigen**

Run: `go test ./cmd/situs/... -v`
Expected: PASS, inklusive aller bereits bestehenden `ingest`-Tests dieser
Datei

- [ ] **Step 5: Commit**

```bash
git add cmd/situs/ingest.go cmd/situs/ingest_test.go
git commit -m "feat(cmd): wire IngestTraits into situs ingest"
```

---

### Task 7: Read-API — `GET /v1/species/{conceptId}/traits`

**Files:**
- Modify: `internal/ports/input/services.go`
- Modify: `internal/application/query.go`
- Create: `internal/application/query_traits_test.go`
- Modify: `internal/adapters/http/server.go`
- Create: `internal/adapters/http/trait.go`
- Create: `internal/adapters/http/trait_test.go`
- Modify: `internal/adapters/http/habitat.go` (nur `writeQueryError`)
- Modify: `internal/adapters/http/openapi.yaml`
- Modify: `api/openapi/openapi.yaml` (byte-identische Kopie)

**Interfaces:**
- Consumes: `output.Repository.Traits`/`KnownVocabs` (Task 3),
  `domain.TraitSet`/`TraitValue` (Task 2)
- Produces: `input.QueryService.Traits(ctx, conceptID, vocab string)
  ([]input.TraitSetView, error)`, `input.ErrUnknownVocab`, HTTP-Route `GET
  /v1/species/{conceptId}/traits`

- [ ] **Step 1: Fehlschlagenden Anwendungsfall-Test schreiben**

`internal/application/query_traits_test.go`:

```go
package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

func TestQueryService_Traits_ReturnsEveryVocabularyWhenNoneRequested(t *testing.T) {
	repo := newFakeRepo()
	repo.traitVocabs = []fakeTraitVocab{{Vocab: "eive", Version: "1.0"}}
	repo.traitValues = []fakeTraitValue{
		{ConceptID: "wcvp:1", Value: domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: 4.2}},
	}
	q := NewQueryService(repo)
	sets, err := q.Traits(context.Background(), "wcvp:1", "")
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 1 || sets[0].Vocab != "eive" {
		t.Fatalf("sets = %+v, want one eive set", sets)
	}
}

func TestQueryService_Traits_UnknownVocabIsInvalidQuery(t *testing.T) {
	repo := newFakeRepo()
	repo.traitVocabs = []fakeTraitVocab{{Vocab: "eive", Version: "1.0"}}
	q := NewQueryService(repo)
	_, err := q.Traits(context.Background(), "wcvp:1", "does-not-exist")
	if !errors.Is(err, input.ErrUnknownVocab) {
		t.Fatalf("err = %v, want input.ErrUnknownVocab", err)
	}
}

func TestQueryService_Traits_UnknownConceptReturnsEmptyNotError(t *testing.T) {
	repo := newFakeRepo()
	repo.traitVocabs = []fakeTraitVocab{{Vocab: "eive", Version: "1.0"}}
	q := NewQueryService(repo)
	sets, err := q.Traits(context.Background(), "wcvp:does-not-exist", "")
	if err != nil {
		t.Fatalf("Traits: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("sets = %+v, want empty (no data is a normal answer, not an error)", sets)
	}
}
```

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/application/... -run TestQueryService_Traits -v`
Expected: FAIL (`QueryService.Traits`/`input.ErrUnknownVocab` undefined)

- [ ] **Step 3: `input.ErrUnknownVocab` + DTO ergänzen**

In `internal/ports/input/services.go`, im `var (...)`-Block neben
`ErrUnknownArea` ergänzen:

```go
	// ErrUnknownVocab is a trait vocabulary the index has no data for. Same
	// treatment as ErrUnknownArea: a typo and a genuine absence must not
	// look the same -> INVALID_QUERY.
	ErrUnknownVocab = errors.New("unknown trait vocabulary")
```

Nach `ConceptResolution` (oder an beliebiger sinnvoller Stelle vor
`QueryService`) die View-Modelle ergänzen — eigene Wire-DTOs statt
`domain.TraitValue`/`TraitSet` direkt zu exportieren, damit die
JSON-Tags hier und nicht im domänennahen Typ leben (dasselbe Prinzip wie bei
`SyntaxonRef`):

```go
// TraitValueView is the wire shape of one domain.TraitValue.
type TraitValueView struct {
	Dim        string   `json:"dim"`
	Value      float64  `json:"value"`
	NicheWidth *float64 `json:"niche_width,omitempty"`
	NSystems   *int     `json:"n_systems,omitempty"`
}

// TraitSetView groups every TraitValueView one vocabulary contributes for
// one concept — never merged across vocabularies.
type TraitSetView struct {
	Vocab        string           `json:"vocab"`
	VocabVersion string           `json:"vocab_version"`
	Values       []TraitValueView `json:"values"`
}
```

`QueryService`-Interface um eine Methode erweitern:

```go
	// Traits returns a concept's indicator values, grouped per vocabulary,
	// never merged. vocab filters to one vocabulary; empty means every
	// ingested vocabulary. An unresolvable vocab is INVALID_QUERY; a
	// concept with no trait data is a normal empty answer, not NOT_FOUND.
	Traits(ctx context.Context, conceptID, vocab string) ([]TraitSetView, error)
```

- [ ] **Step 4: `QueryService.Traits` implementieren**

In `internal/application/query.go`, nach `SyntaxonHabitatTypes` ergänzen:

```go
// Traits returns a concept's indicator values, grouped per vocabulary,
// never merged. An empty vocab answers every ingested vocabulary; a
// non-empty one that the index has no data for is INVALID_QUERY — the same
// distinction areaLookup already makes for ?area=.
func (q *QueryService) Traits(ctx context.Context, conceptID, vocab string) ([]input.TraitSetView, error) {
	var vocabs []string
	if vocab != "" {
		known, err := q.repo.KnownVocabs(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing known trait vocabularies: %w", err)
		}
		if !slices.Contains(known, vocab) {
			return nil, fmt.Errorf("trait vocabulary %q: %w", vocab, input.ErrUnknownVocab)
		}
		vocabs = []string{vocab}
	}

	sets, err := q.repo.Traits(ctx, conceptID, vocabs)
	if err != nil {
		return nil, fmt.Errorf("fetching traits of %q: %w", conceptID, err)
	}
	out := make([]input.TraitSetView, 0, len(sets))
	for _, s := range sets {
		values := make([]input.TraitValueView, 0, len(s.Values))
		for _, v := range s.Values {
			values = append(values, input.TraitValueView{
				Dim: string(v.Dim), Value: v.Value, NicheWidth: v.NicheWidth, NSystems: v.NSystems,
			})
		}
		out = append(out, input.TraitSetView{Vocab: s.Vocab, VocabVersion: s.VocabVersion, Values: values})
	}
	return out, nil
}
```

`"slices"` ist in `query.go` bereits importiert (siehe `backbonesOf`) — kein
neuer Import nötig.

- [ ] **Step 5: Test ausführen, Erfolg bestätigen**

Run: `go test ./internal/application/... -run TestQueryService_Traits -v`
Expected: PASS

- [ ] **Step 6: HTTP-Handler-Test schreiben**

`internal/adapters/http/trait_test.go`:

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jobrunner/situs/internal/ports/input"
)

// fakeTraitQuery isolates the traits handler from the rest of QueryService —
// only Traits is exercised, every other method panics if called.
type fakeTraitQuery struct {
	input.QueryService
	sets []input.TraitSetView
	err  error
	// gotConceptID/gotVocab record the last call's arguments.
	gotConceptID, gotVocab string
}

func (f *fakeTraitQuery) Traits(_ context.Context, conceptID, vocab string) ([]input.TraitSetView, error) {
	f.gotConceptID, f.gotVocab = conceptID, vocab
	return f.sets, f.err
}
```

Prüfe vor dem Fortfahren per `grep -n "type fakeQuery\|func (f \*fakeQuery)"
internal/adapters/http/*_test.go`, ob dort bereits ein gemeinsamer
`fakeQuery`/Test-Server-Aufbau existiert, den dieser Handler-Test statt eines
eigenen Fakes wiederverwenden sollte — falls ja, `fakeTraitQuery` ersatzlos
streichen und stattdessen dessen Muster (z. B. ein Feld
`traitsFn func(...) (...)` auf dem bestehenden Fake) übernehmen. Ergänze
danach:

```go
func TestHandleSpeciesTraits_ReturnsSets(t *testing.T) {
	q := &fakeTraitQuery{sets: []input.TraitSetView{{Vocab: "eive", VocabVersion: "1.0",
		Values: []input.TraitValueView{{Dim: "M", Value: 4.2}}}}}
	s := NewServer(":0", Deps{Query: q}, testLogger(), Options{})
	req := httptest.NewRequest(http.MethodGet, "/v1/species/wcvp:1/traits", nil)
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got []input.TraitSetView
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling body: %v", err)
	}
	if len(got) != 1 || got[0].Vocab != "eive" {
		t.Fatalf("got = %+v, want one eive set", got)
	}
	if q.gotConceptID != "wcvp:1" {
		t.Errorf("gotConceptID = %q, want wcvp:1", q.gotConceptID)
	}
}

func TestHandleSpeciesTraits_PassesVocabQueryParam(t *testing.T) {
	q := &fakeTraitQuery{sets: []input.TraitSetView{}}
	s := NewServer(":0", Deps{Query: q}, testLogger(), Options{})
	req := httptest.NewRequest(http.MethodGet, "/v1/species/wcvp:1/traits?vocab=eive", nil)
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if q.gotVocab != "eive" {
		t.Errorf("gotVocab = %q, want eive", q.gotVocab)
	}
}

func TestHandleSpeciesTraits_UnknownVocabIsInvalidQuery(t *testing.T) {
	q := &fakeTraitQuery{err: fmt.Errorf("trait vocabulary %q: %w", "bogus", input.ErrUnknownVocab)}
	s := NewServer(":0", Deps{Query: q}, testLogger(), Options{})
	req := httptest.NewRequest(http.MethodGet, "/v1/species/wcvp:1/traits?vocab=bogus", nil)
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var body map[string]map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshaling body: %v", err)
	}
	if body["error"]["code"] != CodeInvalidQuery {
		t.Errorf("error.code = %q, want %q", body["error"]["code"], CodeInvalidQuery)
	}
}

func TestHandleSpeciesTraits_EmptyConceptIDIsInvalidQuery(t *testing.T) {
	q := &fakeTraitQuery{}
	s := NewServer(":0", Deps{Query: q}, testLogger(), Options{})
	req := httptest.NewRequest(http.MethodGet, "/v1/species/%20/traits", nil)
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
```

Prüfe per `grep -n "func testLogger" internal/adapters/http/*_test.go`, ob
dieser Helfer bereits existiert (er wird bereits von den bestehenden
Handler-Tests des Pakets verwendet); falls sein tatsächlicher Name
abweicht, den Test-Code entsprechend anpassen statt einen zweiten Helfer
anzulegen. `"context"` und `"fmt"` im Importblock von `trait_test.go`
ergänzen, sofern verwendet.

- [ ] **Step 7: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/adapters/http/... -run TestHandleSpeciesTraits -v`
Expected: FAIL (Route `/v1/species/{conceptId}/traits` nicht registriert →
404 statt der erwarteten Codes)

- [ ] **Step 8: Handler + Route implementieren**

`internal/adapters/http/trait.go`:

```go
package httpapi

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// handleSpeciesTraits answers GET /v1/species/{conceptId}/traits[?vocab=].
// Autark like every other species route: a concept id needs no upstream. A
// concept without trait data answers 200 with an empty array, never 404 —
// "no data" is a normal answer here, the same stance situs already takes
// for habitat assignments.
func (s *Server) handleSpeciesTraits(w http.ResponseWriter, r *http.Request) {
	conceptID := strings.TrimSpace(mux.Vars(r)["conceptId"])
	if conceptID == "" {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "conceptId is empty")
		return
	}
	vocab := strings.TrimSpace(r.URL.Query().Get("vocab"))
	sets, err := s.deps.Query.Traits(r.Context(), conceptID, vocab)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, sets)
}
```

In `internal/adapters/http/server.go`, `setupRoutes` um eine Zeile ergänzen
(direkt nach der `handleSpeciesHabitatTypes`-Route):

```go
	r.HandleFunc("/v1/species/{conceptId}/habitat-types", s.handleSpeciesHabitatTypes).Methods(http.MethodGet)
	r.HandleFunc("/v1/species/{conceptId}/traits", s.handleSpeciesTraits).Methods(http.MethodGet)
```

In `internal/adapters/http/habitat.go`, `writeQueryError`s erste `case`
erweitern:

```go
	case errors.Is(err, input.ErrUnknownTypology), errors.Is(err, input.ErrUnknownArea), errors.Is(err, input.ErrUnknownVocab):
```

- [ ] **Step 9: Tests ausführen, Erfolg bestätigen**

Run: `go test ./internal/adapters/http/... -v`
Expected: PASS, inklusive aller bereits bestehenden Handler-Tests (Kontrakttest
`TestRoutes...` muss die neue Route jetzt ebenfalls in `openapi.yaml` finden —
siehe nächster Schritt)

- [ ] **Step 10: OpenAPI-Route ergänzen (beide Kopien)**

In `internal/adapters/http/openapi.yaml`, nach der
`/v1/species/{conceptId}/habitat-types`-Definition (vor
`/v1/species/habitat-types:`) einfügen:

```yaml
  /v1/species/{conceptId}/traits:
    get:
      summary: Pflanzenökologische Zeigerwerte einer Art
      description: >-
        Zeigerwerte aller ingestierten Vokabulare (EIVE, Tichý, Midolo) zu
        einer Konzept-ID, optional auf ein Vokabular gefiltert. Autark: eine
        Konzept-ID braucht keinen Upstream-Dienst. Ein Konzept ohne
        Trait-Daten liefert ein leeres Array, kein 404 — "keine Daten" ist
        hier eine normale Antwort.
      operationId: speciesTraits
      parameters:
        - name: conceptId
          in: path
          required: true
          description: Konzept-ID aus hostus.
          schema:
            type: string
        - name: vocab
          in: query
          required: false
          description: >-
            Filtert auf ein Vokabular (z. B. "eive"). Unbekannter Wert (kein
            ingestiertes Vokabular) -> 400 INVALID_QUERY.
          schema:
            type: string
      responses:
        "200":
          description: Die Zeigerwerte, je Vokabular gruppiert, nie vermischt.
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/TraitSet"
        "400":
          $ref: "#/components/responses/InvalidQuery"
```

Im `components.schemas`-Block, nach `SyntaxonRef` einfügen:

```yaml
    TraitValue:
      type: object
      required: [dim, value]
      properties:
        dim:
          type: string
        value:
          type: number
        niche_width:
          type: number
          description: Fehlt, wenn das Vokabular keine Nischenbreite liefert (Tichý, Midolo).
        n_systems:
          type: integer
          description: Fehlt, wenn das Vokabular keine Systemanzahl liefert (Tichý, Midolo).
    TraitSet:
      type: object
      required: [vocab, vocab_version, values]
      properties:
        vocab:
          type: string
          example: eive
        vocab_version:
          type: string
          example: "1.0"
        values:
          type: array
          items:
            $ref: "#/components/schemas/TraitValue"
```

Dieselben beiden Ergänzungen (Route + Schemas) **identisch** in
`api/openapi/openapi.yaml` vornehmen — die beiden Dateien müssen
byte-identisch bleiben (Kontrakttest prüft das):

```bash
diff internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
```

Erwartet nach beiden Änderungen: keine Ausgabe (identisch).

- [ ] **Step 11: Kontrakttest und volle HTTP-Testsuite**

Run: `go test ./internal/adapters/http/... -v`
Expected: PASS — der Routen↔Spec-Kontrakttest findet die neue Route jetzt in
beiden Richtungen.

- [ ] **Step 12: Commit**

```bash
git add internal/ports/input/services.go internal/application/query.go \
        internal/application/query_traits_test.go internal/adapters/http/server.go \
        internal/adapters/http/trait.go internal/adapters/http/trait_test.go \
        internal/adapters/http/habitat.go internal/adapters/http/openapi.yaml api/openapi/openapi.yaml
git commit -m "feat(http): GET /v1/species/{conceptId}/traits"
```

---

### Task 8: Dokumentation

**Files:**
- Modify: `docs/reference/measured-index.md`
- Modify: `docs/reference/http-api.md`
- Modify: `docs/how-to/ingest.md`

**Interfaces:** keine (reine Dokumentation)

- [ ] **Step 1: `measured-index.md` — neuer Abschnitt**

Nach dem Abschnitt "Verbreitung (`species_distribution`)" einen neuen
Abschnitt `## Zeigerwerte (trait_value)` einfügen. Miss die tatsächlichen
Zahlen aus einem realen Ingest-Lauf (`situs ingest --csv-dir ... --db ...`,
das JSON-`Traits`-Feld der Ausgabe) statt sie zu erfinden — Platzhalter-Text
wie im folgenden Beispiel (`<gemessen>`) muss vor dem Commit durch die
echten Zahlen ersetzt sein:

```markdown
## Zeigerwerte (trait_value)

Drei Vokabulare (EIVE 1.0, Tichý 2023 v2.0, Midolo 2023 v3), ein
`hostus.Resolve()`-Aufruf über alle drei Dateien zusammen:

| Vokabular | Zeilen | Aufgelöst | Nicht aufgelöst |
|---|---|---|---|
| eive | <gemessen> | <gemessen> | <gemessen> |
| tichy2023 | <gemessen> | <gemessen> | <gemessen> |
| midolo2023 | <gemessen> | <gemessen> | <gemessen> |

Gemessen aus `situs ingest`s `Traits`-Feld am <Datum>.
```

- [ ] **Step 2: `http-api.md` — Route ergänzen**

Im Abschnitt "Die zwei Arten-Pfade" (oder direkt danach) einen kurzen
Absatz zur neuen Route ergänzen:

```markdown
## Zeigerwerte: `GET /v1/species/{conceptId}/traits`

Pflanzenökologische Zeigerwerte (EIVE, Tichý, Midolo) zu einer Konzept-ID,
optional per `?vocab=` auf ein Vokabular gefiltert. Wie die Habitat-Routen
autark und mit demselben Leseverhalten: ein unbekanntes Vokabular ist
`400 INVALID_QUERY`, ein Konzept ohne Trait-Daten liefert ein leeres Array,
kein 404.
```

- [ ] **Step 3: `how-to/ingest.md` — Schritt ergänzen**

Öffne `docs/how-to/ingest.md` und finde die nummerierte Schrittliste des
Ingest-Ablaufs (die im letzten Plan bereits von "zweiten" auf "dritten"
Schritt korrigiert wurde, weil hostus dort erstmals gebraucht wird). Ergänze
nach dem Verbreitungs-Schritt einen weiteren nummerierten Schritt für den
Trait-Ingest, mit korrekt fortlaufender Nummerierung — lies die Datei zuerst
komplett, um die exakte bestehende Nummerierung nicht zu brechen.

- [ ] **Step 4: Diff überfliegen, Commit**

```bash
git add docs/reference/measured-index.md docs/reference/http-api.md docs/how-to/ingest.md
git commit -m "docs: trait module — measured counts, HTTP route, ingest step"
```

---

## Self-Review

**1. Spec-Abdeckung** — jede Spec-Sektion hat eine Task:
- Datengrundlage/Pipelines → Task 1
- Domäne & Speicherung (`domain.TraitDim`/`TraitValue`/`TraitSet`,
  `IngestTx`/`Repository`-Erweiterung, Schema) → Task 2, 3
- Ingest (`IngestTraits`, Einhängung in `runIngest`) → Task 5, 6 (Task 4 ist
  eine von der Spec nicht genannte, aber notwendige Voraussetzung: die
  reale Pipe-Delimitierung, die die Spec nicht erwähnt)
- Read-API (`GET /v1/species/{conceptId}/traits?vocab=`) → Task 7
- Fehlerbehandlung & Tests (alle fünf Zeilen der Fehlertabelle) → Tasks 3, 5,
  7 decken je eine Zeile ab (fehlende Datei → Task 5; hostus down → Task 5;
  Taxon löst nicht auf → Task 5; `?vocab=` unbekannt → Task 7; Konzept ohne
  Daten → Task 7)
- Prüfbare Zusagen (nie vermischt, NicheWidth/NSystems nil, idempotent,
  keine neue hostus-Abhängigkeit in `internal/app`) → Tasks 3, 5, je mit
  einem dedizierten Test; die Architekturgrenze ist durch den bereits
  bestehenden `internal/app/arch_test.go` abgedeckt, ohne Anpassung — dieser
  Plan fügt `IngestTraits`/`hostus.Client` ausschließlich in
  `cmd/situs/ingest.go` ein, nie in `internal/app`.

**2. Platzhalter-Scan** — `docs/reference/measured-index.md`s
`<gemessen>`-Platzhalter in Task 8 ist eine bewusste Ausnahme: die Zahl kann
erst nach einem echten Ingest-Lauf gemessen werden (dieselbe Situation wie
bei jedem vorherigen Plan dieses Repos), der Task-Text sagt das explizit und
verlangt das Ersetzen vor dem Commit — kein unaufgelöster "TODO"/"fill in
later" ohne Anleitung.

**3. Typkonsistenz** — geprüft: `domain.TraitValue`/`TraitSet` (Task 2) →
`output.IngestTx.UpsertTraitValue`/`Repository.Traits` (Task 2) →
`sqlite.trait.go` (Task 3, exakt dieselben Feldnamen) →
`application.traitRow`/`upsertTraitRows` (Task 5, mappt auf
`domain.TraitValue`) → `input.TraitValueView`/`TraitSetView` (Task 7, eigene
Wire-DTOs mit JSON-Tags, nicht die Domänentypen direkt exportiert — konsistent
mit `SyntaxonRef` vs. `domain.Syntaxon`). `readAll`s Signatur
(`delim rune` als viertes Argument, Task 4) wird in Task 5
(`readTraitRows`) exakt in dieser Reihenfolge aufgerufen.

**4. Bekannte, bewusste Abweichung von der Spec** — die Spec nennt keine
`UpsertTraitVocabulary`-Methode, obwohl sie die Tabelle `trait_vocabulary`
definiert. Task 2/3/5 schließen diese Lücke: ohne Schreibmethode bliebe die
Tabelle für immer leer, was der Spec-Absicht ("Ingest-Metadatum für spätere
Drift-Erkennung") widerspricht.

## Execution Handoff

Plan complete and saved to
`docs/superpowers/plans/2026-08-29-situs-trait-modul.md`. Zwei
Ausführungsoptionen:

**1. Subagent-Driven (empfohlen)** — ich dispatche pro Task einen frischen
Subagenten, Review zwischen den Tasks, schnelle Iteration

**2. Inline Execution** — Ausführung in dieser Session via executing-plans,
Batch-Ausführung mit Checkpoints

Welcher Ansatz?
