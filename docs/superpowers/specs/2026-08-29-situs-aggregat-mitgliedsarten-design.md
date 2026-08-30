# Aggregat-Mitgliedsarten & dateibasierter Species-Ingest: Design-Spec

Stand: 2026-08-29.

**Ziel:** `species_roles.csv` (EuroVeg/EUNIS-Referenzdaten: welche Art spielt
welche Rolle in welchem Habitattyp) führt Sammelarten (Sammelart-Namen,
z. B. "Salsola kali aggr.") als eigene Einträge, weil EuroVeg-Definitionen
taxonomisch oft auf dieser Ebene arbeiten. Ein Endnutzer-Konzept ist aber
meist eine Nominatart (z. B. "Salsola kali", "Pentanema hirtum" — aus WCVP,
über hostus' Autosuggest gewählt) und matcht ohne Weiteres nicht gegen die
Sammelart. situs soll beim Ingest die Mitgliederarten einer Sammelart
zusätzlich, aber sichtbar als abgeleitet, mit demselben Habitattyp
verknüpfen.

**Zweiter, damit verwobener Zweck:** der bestehende `species_ingest.go`-Pfad
ruft heute hostus' `/v1/match` live auf, um `verbatim_name` in eine
Concept-ID aufzulösen. Das ist für eine EuroVeg-kontrollierte Namensliste
unnötige Maschinerie (Fuzzy-/Homonym-Auflösung existiert für freien Nutzer-Text,
nicht für eine bereits kontrollierte Referenzliste) — UND macht den
situs-Ingest von einer live erreichbaren hostus-Instanz abhängig, was beide
Dienste in ihrem aktuellen Reifegrad nicht tragen sollen. Dieses Spec ersetzt
den Live-Aufruf durch eine dateibasierte 1:1-Übersetzungstabelle.

**Wichtige Abgrenzung — was NICHT betroffen ist:** der Laufzeit-Pfad
(`POST /v1/species/habitat-types`, nimmt Concept-IDs, kein hostus im
Serve-Pfad) ist bereits genau so gebaut, wie er sein soll — eine Mobile-App
löst Verbatim-Namen VOR dem Aufruf an situs über hostus auf; situs sieht nur
noch Concept-IDs. Daran ändert dieses Spec nichts. Ebenso unangetastet:
`IngestDistribution` (behält seinen Live-Aufruf an hostus — eine echte
Pro-Konzept-Abfrage, kein Namens-Matching, dafür bleibt die bestehende
Begründung gültig).

**hostus-seitige Voraussetzung (separates, kleines Spec/Feature in hostus):**
Dieses Design setzt voraus, dass hostus zwei neue kanonische CSV-Exporte
bereitstellt (`eurosl_crosswalk.csv`, `aggregate_members.csv`) — siehe
`docs/superpowers/specs/2026-08-29-hostus-eurosl-export-design.md`
(hostus-Repo). Bis dahin ist dieses Spec implementierbar, aber nicht
ingestierbar (die Dateien fehlen).

## 1. Architektur & Datenfluss

```
hostus (separater Export-Schritt, siehe hostus-Spec)
      → eurosl_crosswalk.csv     (name|concept_id — deterministische 1:1-Zuordnung)
      → aggregate_members.csv   (aggregate_concept_id|member_concept_id|member_name)
      ↓ (einmaliger Datei-Kopiervorgang, kein Netzwerkaufruf)
situs --csv-dir/
      ↓
IngestSpeciesRoles (umgebaut, kein hostus-Aufruf mehr)
      → lädt eurosl_crosswalk.csv als lokales Dictionary (name → concept_id)
      → schlägt jede species_roles.csv-Zeile darin nach (1:1, KEINE Fuzzy-/
        Ambiguitäts-Auflösung — ein Name mit zwei verschiedenen Concept-IDs
        in der Crosswalk-Tabelle ist ein Datenfund, kein Rateanlass)
      → schreibt die Zeile selbst als SpeciesRole{Provenance:"observed"}
      → lädt aggregate_members.csv, für jede Zeile, deren aufgelöstes Konzept
        selbst eine Sammelart ist (d. h. als aggregate_concept_id in dieser
        Datei vorkommt): schreibt je Mitglied eine zusätzliche
        SpeciesRole{Provenance:"derived_from_aggregate", DerivedFrom:&aggID},
        AUSSER eine explizite species_roles.csv-Zeile für (Key, Mitglied-
        Concept-ID) existiert bereits — die gewinnt
```

Kein Port für einen Namens-Resolver mehr in diesem Pfad — `output.NameResolver`
bleibt für `IngestDistribution` bestehen (unverändert), wird aber von
`IngestSpeciesRoles` nicht mehr injiziert.

## 2. Domäne

```go
// internal/domain/habitat.go, SpeciesRole erweitert

// SpeciesRole is a species' role in a habitat type. VerbatimName is always
// set (auch bei Provenance=derived_from_aggregate — dort trägt es den
// NAMEN DES MITGLIEDS, nicht der Sammelart, aus aggregate_members.csv);
// ConceptID is nil when the name did not resolve via the crosswalk table.
type SpeciesRole struct {
	Key          HabitatTypeKey
	ConceptID    *string
	VerbatimName string
	Role         string // "diagnostic" | "constant" | "dominant"
	Fidelity     *float64
	Constancy    *float64
	// Provenance distinguishes an observed (CSV-sourced) role from one
	// silently derived from a collective-species (aggregate) row. Never
	// merged into one bucket: an end user must be able to tell "this
	// species was actually assessed for this habitat" from "this species
	// is a member of an assessed aggregate, but was never itself assessed".
	Provenance string // "observed" | "derived_from_aggregate"
	// DerivedFrom names the aggregate concept id this row was derived
	// from. nil for Provenance == "observed".
	DerivedFrom *string
}
```

`Provenance`/`DerivedFrom` sind bewusst KEINE eigene Tabelle/kein eigener
Port-Typ — sie sind Teil derselben Zeile wie heute, nur um zwei Felder
erweitert.

## 3. Ingest

```go
// internal/application/species_ingest.go

// CrosswalkEntry is one row of eurosl_crosswalk.csv: a deterministic
// name -> concept id mapping. Anders als hostus' Resolve() KEINE Fuzzy-/
// Homonym-Logik — die Datei selbst muss bereits eindeutig sein.
type CrosswalkEntry struct {
	Name      string
	ConceptID string
}

// AggregateMemberEntry is one row of aggregate_members.csv.
type AggregateMemberEntry struct {
	AggregateConceptID string
	MemberConceptID    string
	MemberName         string
}

// IngestSpeciesRoles loads csvPath (species_roles.csv) plus the two
// crosswalk files, resolving every verbatim name against a LOCAL,
// deterministic dictionary — no network call. A name present more than
// once in the crosswalk with DIFFERENT concept ids is a data-quality
// finding (reported, not guessed) — see SpeciesReport.AmbiguousCrosswalk.
func IngestSpeciesRoles(ctx context.Context, repo output.Repository,
	csvPath, crosswalkPath, aggregateMembersPath string) (SpeciesReport, error)
```

`SpeciesReport` erweitert:

```go
type SpeciesReport struct {
	Rows                int
	Resolved            int
	Unresolved          int
	Skipped             int
	DerivedRows          int      // zusätzlich geschriebene Mitgliedsarten-Zeilen
	SuppressedByExplicit int      // abgeleitete Zeilen, die wegen einer expliziten
	                              // CSV-Zeile NICHT geschrieben wurden
	AmbiguousCrosswalk  int       // Zeilen mit >1 Concept-ID in der Crosswalk-Tabelle
}
```

Ein mehrdeutiger Crosswalk-Eintrag bricht den Ingest NICHT ab (dieselbe
Toleranz-Haltung wie überall sonst), wird aber sichtbar gezählt und mit
Beispiel gemeldet — nie stillschweigend eine der beiden Optionen gewählt.

## 4. Read-API

`input.SpeciesEntry` und `input.HabitatTypeRole` bekommen dieselben zwei
Felder wie die Domäne:

```go
type SpeciesEntry struct {
	ConceptID    string   `json:"concept_id,omitempty"`
	VerbatimName string   `json:"verbatim_name"`
	Role         string   `json:"role"`
	Fidelity     *float64 `json:"fidelity,omitempty"`
	Constancy    *float64 `json:"constancy,omitempty"`
	InArea       *bool    `json:"in_area,omitempty"`
	// Provenance is "observed" (default, omitted) or
	// "derived_from_aggregate" — present only in the derived case, so the
	// common (observed) response shape is unchanged for existing clients.
	Provenance  string           `json:"provenance,omitempty"`
	DerivedFrom *AggregateSource `json:"derived_from,omitempty"`
}

// AggregateSource lets a caller ask "what aggregate produced this match?".
type AggregateSource struct {
	ConceptID string `json:"concept_id"`
	Name      string `json:"name"`
}
```

`Provenance == ""` wird als `"observed"` interpretiert (Rückwärtskompatibilität:
ein bestehender Client, der das Feld ignoriert, sieht keine Änderung an
Zeilen, die er heute schon bekommt).

## 5. Fehlerbehandlung & Tests

| Fall | Verhalten |
|---|---|
| Name fehlt in `eurosl_crosswalk.csv` | `Unresolved` gezählt, Zeile mit `ConceptID=nil` gespeichert (wie heute) |
| Name mehrdeutig in `eurosl_crosswalk.csv` | `AmbiguousCrosswalk` vermerkt, Zeile bleibt `ConceptID=nil` (kein Raten) |
| `aggregate_members.csv` fehlt | Ingest läuft weiter OHNE Mitgliedsarten-Ableitung, `DerivedRows=0`, im Log vermerkt — Sammelarten-Zeilen selbst werden trotzdem korrekt eingetragen |
| Mitgliedsart bereits explizit in `species_roles.csv` | die explizite Zeile gewinnt, `SuppressedByExplicit` gezählt |

Tests: `internal/application/species_ingest_test.go` erweitert (Crosswalk-
Dictionary statt Resolver-Mock, Ambiguitäts-Fund, Ableitungs-/Unterdrückungs-
Fälle), `internal/adapters/http/habitat_test.go`/`species_test.go` erweitert
(Provenance/DerivedFrom im JSON), `cmd/situs/ingest_test.go` erweitert
(neue Report-Felder, neue CLI-Flags für die zwei Crosswalk-Dateipfade).

## Prüfbare Zusagen

- `species_ingest.go` importiert nach diesem Umbau `internal/adapters/hostus`
  nicht mehr — nur `IngestDistribution` (unverändert) tut das noch. Ein
  Architektur-Test (analog zu `internal/app/arch_test.go`) hält das fest.
- Eine abgeleitete Zeile (`Provenance == "derived_from_aggregate"`) hat
  IMMER `Fidelity == nil && Constancy == nil` — diese Werte wurden nie für
  das Mitglied selbst gemessen, sie zu erfinden wäre unehrlich.
- Eine explizite CSV-Zeile gewinnt IMMER gegen eine abgeleitete für denselben
  (Key, ConceptID) — nie umgekehrt, unabhängig von der Verarbeitungsreihenfolge
  der beiden Dateien.
- Ein bestehender API-Client, der `provenance`/`derived_from` ignoriert,
  sieht für jede schon heute existierende (beobachtete) Zeile keine
  Änderung.

## Offene Punkte (bewusst nicht in diesem Spec gelöst)

- **hostus-seitiger Export** (`eurosl_crosswalk.csv`, `aggregate_members.csv`)
  ist ein eigenes Spec/eigene Implementierung im hostus-Repo — Voraussetzung,
  aber nicht Teil dieses Plans.
- **Syntaxa-Hierarchie oberhalb Verband** (Ordnung, Klasse, Codes) ist ein
  eigenständiges Thema, betrifft `Syntaxon`/die EUNIS-Syntaxa-Pipeline, nicht
  dieses Spec — eigenes Brainstorming.
- **100-%-Konzeptabdeckung** für Habitat-/Syntaxa-relevante Arten ist eine
  Qualitätsanforderung an die hostus-seitige EuroSL-Ingest-Vollständigkeit,
  keine neue situs-Struktur — wird über die bestehende
  `SpeciesReport`-Berichterstattung beobachtet, nicht neu gelöst.
