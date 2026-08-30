# Trait-Modul: Design-Spec

Stand: 2026-08-29.

**Ziel:** situs bekommt ein eigenständiges Lesemodul für pflanzenökologische
Zeigerwerte (EIVE, Tichý et al. 2023, Midolo et al. 2023) — eine Art, ein
Aufruf, die Werte aller drei Vokabulare. Die Exkursions-App kann so zu einer
aufgenommenen Art direkt die Zeigerwerte anzeigen, ohne einen zweiten Dienst
anzufragen.

**Anlass.** Dies ist "Teilprojekt 2" aus hostus' Namensraum-/Klassifikations-/
Aggregat-Redesign (`docs/superpowers/specs/2026-08-27-hostus-namensraum-redesign-design.md`
Abschnitt 8, hostus-Repo): hostus hat sein Traits-Subsystem entfernt (reine
Namensauflösung ist sein Zweck), die Zenodo/OSF-Rohdaten und die
Python-Konvertierungs-Pipelines wurden vorher gesichert
(`/tmp/hostus-traits-transfer/{eive,tichy,midolo}/`) und warten auf einen
neuen, situs-eigenen Träger. situs ist der richtige Ort: es hält bereits jede
andere Pflanzen-Fakten-Schicht (Habitat, Verbreitung, Vernakularnamen), traits
sind eine weitere.

**Bewusst kein Ellenberg.** Klassische Ellenberg-Zeigerwerte (1991/2001) werden
NICHT separat ingestiert: EIVE (Ecological Indicator Values for Europe) ist
bereits eine harmonisierte Neuberechnung im selben Werte-Schema (M/N/R/L/T) für
ganz Europa und deckt denselben Bedarf ab — ein zweites, überlappendes
Vokabular wäre Redundanz ohne Erkenntnisgewinn.

**Kein transplantierter Crosswalk-Mechanismus.** hostus' alte
`traitBearers`/`genuineBearerWinner`-Homonym-Tie-Break-Logik (die den
11–18-%-Mehrdeutigkeits-Bug behoben hatte, siehe hostus'
`internal/application/crosswalk.go`) muss NICHT nach situs portiert werden:
diese Logik lebt in Wahrheit in hostus' `/v1/match` selbst (`match.go`s
`genuineBearerWinner`, aus PR #76) — sie greift für JEDEN Aufrufer von
`/v1/match`, nicht nur für hostus' eigenen, inzwischen entfernten
Traits-Ingest. situs' bestehender `hostus.Client.Resolve()`
(`internal/adapters/hostus/client.go`) ruft bereits `/v1/match` auf, exakt wie
`species_ingest.go` es für Habitat-Artenlisten tut — ein mehrdeutiger Treffer
liefert dort `concept_id: ""`, und `resolveInto` behandelt das schon korrekt
als unaufgelöst. Das Trait-Modul braucht denselben Pfad, keinen eigenen.

## Datengrundlage — was gemessen wurde

Alle drei Quellen sind bereits durch hostus' Pipelines in eine identische
kanonische Form gebracht (`taxon|vocab|vocab_version|dim|value|niche_width|n_systems`,
pipe-getrennt) — dieselbe Form, die hostus vor der Traits-Entfernung nutzte.
Die Python-Konvertierungsskripte (`build.sh`+`convert.py` je Quelle) werden
unverändert nach `situs/pipelines/{eive,tichy,midolo}/` übernommen, nur der
`situs`-Klon statt `hostus` als Ziel-Repo.

| Vokabular | Version | Zeilen | Taxa | Dimensionen | Niche width / n_systems |
|---|---|---|---|---|---|
| `eive` | 1.0 | 71.266 | 14.835 | M, N, R, L, T | immer vorhanden |
| `tichy2023` | 2.0 | 45.592 | 8.908 | L, T, M, R, N, S | nie vorhanden |
| `midolo2023` | 3 | 31.910 | 6.382 | disturbance_severity, disturbance_frequency, mowing_frequency, grazing_pressure, soil_disturbance | nie vorhanden |

Die Dimensions-Namen sind je Vokabular verschieden buchstabiert (EIVE/Tichý
nutzen Einzelbuchstaben, Midolo ganze Wörter) und werden NICHT auf ein
gemeinsames Schema vereinheitlicht — sie sind fachlich unterschiedliche
Messgrößen, ein Vermischen wäre eine stille taxonomische/ökologische
Fehlaussage (dieselbe Haltung wie hostus' Namensräume: "eigenständige,
ehrliche Perspektiven", nicht verschmolzen).

## 1. Architektur & Datenfluss

Keine neue Mechanik — eine Erweiterung des bestehenden Ingest-/Read-Musters:

```
pipelines/{eive,tichy,midolo}/   (Python, aus hostus übernommen)
      ↓ kanonische CSV: taxon|vocab|vocab_version|dim|value|niche_width|n_systems
situs ingest                     (neuer Schritt, application.IngestTraits)
      → sammelt alle distinkten taxon-Werte über die drei CSVs ZUSAMMEN
      → EIN hostus.Client.Resolve()-Aufruf (bestehender Pfad, /v1/match,
        Tier-1/2-Homonymauflösung kommt automatisch mit)
      → schreibt trait_value-Zeilen, geschlüsselt auf concept_id (nicht taxon)
GET /v1/species/{conceptId}/traits[?vocab=eive]
```

Kein neuer Aufruf-Typ an hostus, keine transplantierte Crosswalk-Logik — nur
ein weiterer Ingest-Schritt, der den bereits vorhandenen `Resolve()`-Client
wiederverwendet.

## 2. Domäne & Speicherung

```go
// internal/domain/trait.go

// TraitDim identifies one measured dimension within a trait vocabulary
// (z.B. "M" für EIVEs Feuchte-Achse, "disturbance_severity" für Midolos
// gesamthaft gemessene Störungsintensität). Kein geschlossenes Enum: jedes
// Vokabular definiert seinen eigenen Dimensions-Satz, situs führt keine
// vokabular-übergreifende Dimensions-Registry.
type TraitDim string

// ParseTraitDim validiert nur, dass s nicht leer ist — die Schreibweise ist
// eine Pro-Vokabular-Konvention, kein situs-weites Register.
func ParseTraitDim(s string) (TraitDim, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("trait dimension is empty")
	}
	return TraitDim(s), nil
}

// TraitValue is one indicator value for one concept in one trait vocabulary.
// NicheWidth/NSystems are nil when the vocabulary does not provide them
// (Tichý/Midolo never do; EIVE always does) — never coerced to 0.0/0.
type TraitValue struct {
	Vocab        string
	VocabVersion string
	Dim          TraitDim
	Value        float64
	NicheWidth   *float64
	NSystems     *int
}

// TraitSet groups every TraitValue one vocabulary contributes for one
// concept — never merged across vocabularies (M in EIVE und M in Tichý sind
// unterschiedliche Messungen auf unterschiedlichen Skalen).
type TraitSet struct {
	Vocab        string
	VocabVersion string
	Values       []TraitValue
}
```

`IngestTx` (`internal/ports/output/repository.go`) bekommt:

```go
// UpsertTraitValue writes one trait_value row for conceptID. Idempotent wie
// jeder andere Upsert hier: ein repinntes Vokabular wird einfach neu
// ingestiert.
UpsertTraitValue(conceptID string, tv domain.TraitValue) error
```

`Repository` bekommt:

```go
// Traits returns every domain.TraitSet situs holds for conceptID — grouped
// PER VOCABULARY, nie vermischt. vocabs leer = alle ingestierten
// Vokabulare; sonst gefiltert (bedient GET .../traits?vocab=).
Traits(ctx context.Context, conceptID string, vocabs []string) ([]domain.TraitSet, error)
```

Schema (zwei neue Tabellen, analog zu hostus' altem Modell):

```sql
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

CREATE TABLE IF NOT EXISTS trait_vocabulary (
  vocab         TEXT NOT NULL,
  version       TEXT NOT NULL,
  ingested_at   TEXT NOT NULL,
  PRIMARY KEY (vocab, version)
);
```

`trait_vocabulary` trägt (wie bei hostus) keine fachliche Information für den
Leser — sie ist reines Ingest-Metadatum (wann wurde welche Version zuletzt
geschrieben), für spätere Drift-Erkennung.

## 3. Ingest

Neuer Anwendungsfall, analog zu `IngestSpeciesRoles`:

```go
// TraitReport summarizes one trait ingest run across all three canonical
// CSVs, resolved through ONE hostus.Resolve() call (not three — the
// distinct taxa across all vocabularies are collected first).
type TraitReport struct {
	Rows       int
	Resolved   int
	Unresolved int
	PerVocab   map[string]VocabReport // Rows/Resolved/Unresolved je Vokabular
	Skipped    []string               // Vokabulare, deren CSV-Datei fehlte
}

func IngestTraits(ctx context.Context, repo output.Repository, resolver output.NameResolver, csvPaths map[string]string) (TraitReport, error)
```

- `csvPaths` bildet Vokabular-Name auf CSV-Pfad ab (z.B. `{"eive": ".../eive_traits.csv", ...}`); eine fehlende Datei wird übersprungen und in `Skipped` vermerkt — KEIN Abbruch (Trait-Daten sind Zusatzinformation, wie die Verbreitung).
- hostus nicht erreichbar beim `Resolve()`-Aufruf: harter Fehler, bricht den Ingest ab — anders als bei fehlenden Dateien, weil sonst *jede* Zeile fälschlich als "unresolvable" statt "hostus war down" gebucht würde (dieselbe Unterscheidung, die `IngestSpeciesRoles` bereits trifft).
- Ein Taxon, das hostus nicht auflöst, zählt als `Unresolved`, bricht den Lauf nicht ab (dieselbe Toleranz wie bei Artenlisten).

Einhängung in `runIngest` (`cmd/situs/ingest.go`): nach `IngestSpeciesRoles`/
`IngestDistribution`, unabhängig von beiden — eine reine Ergänzung im
bestehenden Ablauf, kein Umbau der Reihenfolge nötig. Neue Flags/Config nicht
nötig: die drei CSV-Pfade leiten sich aus dem bestehenden `--csv-dir` ab
(`eive_traits.csv`, `tichy_traits.csv`, `midolo_traits.csv`).

`ingestOutput` (`cmd/situs/ingest.go`) bekommt ein Feld `Traits TraitReport`.

## 4. Read-API

```
GET /v1/species/{conceptId}/traits[?vocab=eive]
```

- `vocab` optional, EIN Wert (kein Komma-getrennter Mehrfach-Filter — YAGNI,
  kann bei Bedarf später ergänzt werden, ohne die Signatur zu brechen).
  Unbekannter Wert (kein ingestiertes Vokabular) → `400 INVALID_QUERY`
  (dasselbe Muster wie `role` bei `GET /v1/habitat-type/{typology}/{code}/species`).
- Antwort: `[]domain.TraitSet` — leeres Array bei unbekanntem/nicht-getraitetem
  Konzept, KEIN 404 (passt zum bestehenden Muster: "keine Daten" ist eine
  Antwort, kein Fehler — wie situs es für Habitat-Zuordnungen bereits hält).
- `QueryService.Traits(ctx, conceptID, vocab string) ([]domain.TraitSet, error)`
  (`internal/application/query.go`), ruft `Repository.Traits` durch.
- Route in `internal/adapters/http/server.go` neben den bestehenden
  `/v1/species/{conceptId}/...`-Routen; Handler in einer neuen
  `internal/adapters/http/trait.go`.

## 5. Fehlerbehandlung & Tests

| Fall | Verhalten |
|---|---|
| CSV-Datei für ein Vokabular fehlt im `--csv-dir` | überspringen, `TraitReport.Skipped` vermerkt — kein Abbruch |
| hostus beim `Resolve()`-Aufruf nicht erreichbar | harter Fehler, Ingest bricht ab |
| Taxon-Name löst nicht auf | `Unresolved` gezählt, Zeile verworfen, kein Abbruch |
| `?vocab=` unbekannt | `400 INVALID_QUERY` |
| Konzept ohne Trait-Daten | `200`, leeres Array |

Tests (TDD wie überall in situs):

- `internal/application/trait_ingest_test.go` — Resolve-Mock, Report-Zahlen
  (inkl. `PerVocab`), Skip-Verhalten bei fehlender Datei, harter Abbruch bei
  Resolver-Ausfall.
- `internal/adapters/sqlite/trait_test.go` — Upsert/Read-Rundreise,
  Vokabular-Filter, Idempotenz eines wiederholten Ingests.
- `internal/adapters/http/trait_test.go` — Query-Param-Validierung, leere vs.
  gefüllte Antwort, mehrere Vokabulare in einer Antwort korrekt gruppiert.
- `cmd/situs/ingest_test.go`-Erweiterung — `Traits`-Feld taucht im
  JSON-Report auf.

## Prüfbare Zusagen

- Ein Konzept mit Trait-Daten in mehreren Vokabularen liefert sie **niemals**
  vermischt (jede `TraitSet` trägt genau ein `Vocab`+`VocabVersion`).
- `NicheWidth`/`NSystems` sind `nil`, nicht `0`/`0.0`, wenn ein Vokabular sie
  nicht liefert (Tichý, Midolo) — ein Nulltest muss das unterscheiden können.
- Ein wiederholter Ingest überschreibt vorhandene `trait_value`-Zeilen
  (idempotent), erzeugt keine Duplikate.
- Der Trait-Ingest fügt `internal/app` keine neue hostus-Abhängigkeit hinzu:
  `Resolve()` läuft, wie bei Artenrollen und Verbreitung bereits etabliert,
  ausschließlich im Ingest-Pfad (`cmd/situs/ingest.go`), nie im Serve-Pfad —
  der bestehende `internal/app/arch_test.go`-Architekturtest deckt das bereits
  ab, ohne Anpassung.

## Offene Punkte (bewusst nicht in diesem Spec gelöst)

- **Aggregat-Mitgliedsarten-Erweiterung für Habitat-Artenlisten** (stumme
  Mitgliedseinträge für Sammelarten wie "Achillea millefolium agg." über
  hostus' neue `concept_aggregate`-Mitgliederliste) ist ein eigenständiges
  Thema, betrifft `species_ingest.go`/`SpeciesRole`, nicht dieses Trait-Modul
  — eigenes Brainstorming, eigene Spec.
- Eine spätere ESy-Regel-Engine könnte Trait-Werte als Eingabe brauchen
  (situs' eigene Offene-Punkte-Liste nennt ESy bereits als ausgeklammert) —
  dieses Modul ist bewusst nur als eigenständige Read-API entworfen, nicht
  als Baustein dafür; eine Integration wäre ein separater, späterer Schritt.
