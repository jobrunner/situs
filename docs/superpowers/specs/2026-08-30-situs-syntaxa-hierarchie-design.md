# Syntaxa-Hierarchie (Klasse/Ordnung) & Autorschafts-Trennung: Design-Spec

Stand: 2026-08-30.

**Ziel:** situs führt Pflanzengesellschaften (Syntaxa) bisher nur auf
Verbands-Ebene (`domain.Syntaxon.Rank == "alliance"`), aus dem
EEA-EUNIS-2021-Crosswalk geerbt. situs ist zunächst ein Info-System — bevor
spätere Experimente (ESy-Regelwerk, aus EIVE/Tichý/Midolo-Zeigerwerten
abgeleitete Standort-Schätzung) sinnvoll sind, muss die Grundlage stimmen:
die volle Syntaxa-Hierarchie (Klasse → Ordnung → Verband) mit Codes.

**Vorgeschaltete Recherche (Spike, 2026-08-30):** Die bestehende
EEA-EUNIS-Quelle führt Syntaxa fast ausschließlich auf Verbands-Ebene
(1049 von 1050 gemessenen Zeilen; eine einzige Ausnahme auf
Ordnungs-Ebene) — das ist keine Parsing-Lücke, die Quelle selbst liefert
keine Eltern-Hierarchie. Gefunden: **FloraVeg.EU** veröffentlicht die
vollständige EuroVegChecklist-Hierarchie (Mucina et al. 2016 + Updates,
Version 4, 2025-06-17) frei herunterladbar, ohne Registrierung:
`https://files.ibot.cas.cz/cevs/downloads/floraveg/List_of_European_vegetation_units_version_4.xlsx`.
Verifiziert (echte Datei geladen, nicht nur Ankündigung vertraut):
1841 Zeilen, 150 Klassen, 381 Ordnungen, 1310 Verbände. Der Code selbst
trägt die Hierarchie (`AA` Klasse → `AA01` Ordnung → `AA01A` Verband,
Elternteil durch Suffix-Kürzung ableitbar) — an allen 1841 Zeilen
automatisiert gegengeprüft, keine Ausnahme gefunden. Namensstamm-Überlappung
mit situs' bestehenden EUNIS-Verbänden stichprobenartig bestätigt (4/4
Treffer: "Agropyro-Rumicion", "Atriplicion littoralis", "Cakilion
edentulae", "Moltkeetalia petraeae").

**hostus/situs-Kopplung: keine.** Dieses Feature ist unabhängig von hostus
— FloraVeg liefert Syntaxa-Fachdaten, keine Pflanzenkonzepte. Kein
Zusammenhang mit den beiden anderen offenen situs-Specs
(Trait-Modul, Aggregat-Mitgliedsarten).

## 1. Architektur & Datenfluss

```
FloraVeg.EU .xlsx (List_of_European_vegetation_units_v4)
      ↓ pipelines/eurovegchecklist/ (neu, Python, analog zu pipelines/eunis)
      ↓ kanonische CSV: syntaxa_hierarchy.csv (code|rank|name|author|parent_code)
        — rank/parent_code werden AUSSCHLIESSLICH aus dem Code-Muster
        (AA/AA01/AA01A) abgeleitet, nie aus dem Namen geraten
situs ingest (erweitert, IngestCSV in internal/application/ingest.go)
      → schreibt Klasse-/Ordnung-Syntaxa als NEUE Zeilen (FloraVeg-Codes als ID)
      → für jeden BEREITS aus EUNIS ingestierten Verband: Präfix-Abgleich
        gegen FloraVeg-Verbandsnamen (längster Treffer gewinnt)
        → Treffer: Name/Author von FloraVeg übernommen, ParentID = FloraVeg-
          Ordnungscode
        → kein Treffer: Name bleibt der volle EUNIS-Kombi-String,
          Author="", ParentID="" — gezählt, gemeldet, kein Abbruch
```

Zwei ID-Schemata koexistieren unproblematisch: EUNIS-Verbandscodes (z. B.
`CAK-01C`) bleiben als `Syntaxon.ID` bestehen, `ParentID` verweist auf einen
FloraVeg-Ordnungscode (z. B. `AA01`) — `ParentID` ist eine reine
String-Referenz, kein schemagebundener Fremdschlüssel.

## 2. Domäne

```go
// internal/domain/habitat.go, Syntaxon erweitert
type Syntaxon struct {
	ID       string
	Rank     string // "class" | "order" | "alliance"
	Name     string // reiner Syntaxon-Name, ohne Autorschaft
	Author   string // Autorschafts-Zitat; "" wenn kein sauberer Split bekannt
	ParentID string
}
```

`Author` wird **nie** heuristisch aus einem Kombi-String geschnitten — nur
von FloraVegs eigener, bereits getrennter Spalte übernommen. Ein
EUNIS-Verband ohne FloraVeg-Treffer bleibt ehrlich `Author=""` (keine
geratene Trennung), `Name` unverändert der volle historische Kombi-String.

## 3. Ingest

```go
// internal/application/syntaxa_hierarchy_ingest.go (neu)

// SyntaxaHierarchyReport summarizes the FloraVeg hierarchy ingest + the
// EUNIS-alliance matching pass.
type SyntaxaHierarchyReport struct {
	ClassesWritten     int
	OrdersWritten      int
	AlliancesMatched   int // EUNIS-Verbände mit gefundenem FloraVeg-Elternteil
	AlliancesUnmatched int // EUNIS-Verbände ohne Treffer (ParentID bleibt leer)
	AmbiguousMatches   []string // gleich lange Mehrfachtreffer, nie geraten
}

func IngestSyntaxaHierarchy(ctx context.Context, repo output.Repository, csvPath string) (SyntaxaHierarchyReport, error)
```

Läuft NACH `ingestSyntaxa` (EUNIS-Verbände müssen bereits existieren, damit
der Abgleich sie finden kann) und VOR den Localization-/Derivation-Schritten
(unabhängig von beiden, reine Ergänzung).

`IngestTx` (`internal/ports/output/repository.go`) bekommt:

```go
// UpsertSyntaxonAuthor sets Name and Author on an already-upserted syntaxon
// and, if parentID is non-empty, its ParentID — used ONLY by the
// hierarchy-matching pass to enrich an existing EUNIS alliance row on a
// FloraVeg match. name is FloraVeg's own clean name, replacing the
// historical EUNIS combi-string; it is always set (a match always has a
// real FloraVeg name), unlike parentID, which can legitimately be empty.
//
// Implementation note (post-review): the port ended up taking name too
// (id, name, author, parentID) — the Rank/Name-stays-EUNIS-owned framing
// above was the spec's original intent, superseded once the implementation
// showed a FloraVeg match should also correct Name. See
// internal/ports/output/repository.go for the authoritative signature.
UpsertSyntaxonAuthor(id, name, author, parentID string) error
```

## 4. Read-API

`input.SyntaxonRef` (und jede Stelle, die einen Syntaxon rendert) bekommt:

```go
type SyntaxonRef struct {
	ID       string `json:"id"`
	Rank     string `json:"rank"`
	Name     string `json:"name"`
	Author   string `json:"author,omitempty"`
	ParentID string `json:"parent_id,omitempty"`
}
```

`author`/`parent_id` sind `omitempty` — ein bestehender Client, der beide
Felder ignoriert, sieht für bereits existierende Antworten keine Änderung.

## 5. Fehlerbehandlung & Tests

| Fall | Verhalten |
|---|---|
| EUNIS-Verband ohne FloraVeg-Treffer | `Author=""`, `ParentID=""`, gezählt (`AlliancesUnmatched`), kein Abbruch |
| FloraVeg-Code passt nicht ins `AA`/`AA01`/`AA01A`-Muster | Zeile übersprungen, gezählt, gemeldet (im Spike: 0/1841 betroffen — trotzdem kein Automatismus, der das voraussetzt) |
| Mehrere FloraVeg-Namen passen als Präfix gleich lang | `AmbiguousMatches` vermerkt, kein Treffer übernommen (kein Raten) |
| `syntaxa_hierarchy.csv` fehlt | Ingest läuft weiter ohne Hierarchie-Anreicherung, bestehende Verbands-Daten bleiben unverändert nutzbar |

Tests: `pipelines/eurovegchecklist/xlsx_to_csv_test.py` (Code→Rang/Parent-
Ableitung gegen echte Musterzeilen aus dem Spike), `internal/application/
syntaxa_hierarchy_ingest_test.go` (Treffer/Nichttreffer/Mehrdeutigkeit,
Name/Author-Übernahme), `internal/adapters/http/habitat_test.go` (neue
`author`/`parent_id`-Felder im JSON, Rückwärtskompatibilität bei leeren
Werten).

## Prüfbare Zusagen

- `Author` ist niemals das Ergebnis einer Textzerlegung eines Kombi-Strings
  — nur eine direkte Übernahme aus FloraVegs eigener Spalte.
- Jede Klasse-/Ordnung-Zeile stammt ausschließlich aus FloraVeg; jede
  Verbands-Zeile bleibt EUNIS-eigen (ID-Schema bleibt pro Rang erkennbar).
- Die Trefferquote wird im Ingest-Report gemessen und ausgegeben, nie als
  Zahl behauptet, die nicht nachgerechnet wurde.
- Ein bestehender API-Client, der `author`/`parent_id` ignoriert, sieht für
  jede schon heute existierende Verbands-Antwort keine Änderung.

## Offene Punkte (bewusst nicht in diesem Spec gelöst)

- Eine `--force`-Neu-Zuordnung für Verbände, deren FloraVeg-Gegenstück sich
  in einer künftigen Versionsaktualisierung umbenennt (Nomenklatur-Drift),
  ist nicht vorgesehen — ein erneuter Ingest überschreibt einfach die
  vorhandene Zuordnung (idempotent, wie jeder andere situs-Ingest-Schritt).
- Assoziations-Ebene (unterhalb Verband) bleibt außerhalb des Scopes — wie
  im ursprünglichen EEA-EUNIS-Spike bereits festgehalten, dort nicht robust
  abgedeckt.
