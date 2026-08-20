# Autarke Laufzeit und Verbreitungsfilter: Design-Spec

Stand: 2026-08-20.

**Ziel:** situs braucht hostus **zur Laufzeit nicht mehr**. Gleichzeitig werden
die Artenlisten regional korrekt, damit eine Exkursions-App keine Arten
vorschlägt, die es am Fundort nicht gibt.

**Anlass.** Ein Frontend bestimmt aus GPS mehrere Ortsangaben (u. a. das
WGSRPD-Gebiet, über ortus oder lokale Logik) und löst die aufgenommenen
Pflanzennamen ohnehin über hostus auf — es braucht akzeptierte Namen für das
Exkursionstagebuch. Wenn es situs fragt, **hat es die Konzept-IDs schon**. Dass
situs sie erneut auflöst, ist überflüssige Komplexität und eine
Betriebsabhängigkeit ohne Gegenwert.

Grundlage der Messwerte: `docs/research/2026-08-20-verbreitungsfilter-spike.md`.

## Ausgangslage

Zur Laufzeit hängt **eine einzige Route** an hostus:

| Route | hostus zur Laufzeit |
|---|---|
| `GET /v1/habitat-type/{typology}/{code}` | nein |
| `GET /v1/habitat-type/{typology}/{code}/species` | nein |
| `GET /v1/species/{conceptId}/habitat-types` | nein |
| `GET /v1/syntaxon/{id}/habitat-types` | nein |
| **`POST /v1/species/habitat-types`** | **ja** — einziger Aufrufer |

Beim **Ingest** bleibt hostus nötig: dort entstehen die Konzept-IDs, und künftig
auch die Verbreitung. Das ist ein Datenaufbereitungsschritt, kein Betriebsdienst
— die Zusage lautet „der *Dienst* ist autark", nicht „hostus wird nie gebraucht".

## Teil 1 — Entkopplung

### Was entfällt

Der Umbau ist ein **Rückbau**: der Laufzeitpfad verliert eine Abhängigkeit *und*
eine Abstraktionsebene.

| Entfällt | Grund |
|---|---|
| `application.NameQueryService` | war nur die Hülle um den Resolver |
| Port `input.SpeciesNameQueryService` | der Batch ist „viele Konzept-IDs" und gehört auf `QueryService`, das `SpeciesHabitatTypes` für eine ID schon hat |
| `Deps.Names` samt Verdrahtung im Composition Root | entfällt mit dem `hostus.NewClient`-Aufruf für den Serve-Pfad |
| `input.ErrUpstreamUnavailable`, `CodeUpstreamUnavailable`, der zugehörige Handler-Zweig | zur Laufzeit unerreichbar — konsequent zu `UNRESOLVABLE`, das aus demselben Grund entfernt wurde |

**Bleibt:** `output.NameResolver`, der hostus-Adapter, `ErrResolverUnavailable`,
`ErrResolverRejected` und die vier `SITUS_HOSTUS_*`-Schlüssel. Der Ingest braucht
sie; ihre Dokumentation wird auf „nur beim Ingest" verschärft.

**Prüfbare Zusage:** nach dem Umbau importiert `internal/app` den hostus-Adapter
nicht mehr, nur `cmd/situs/ingest.go`. Depguard erlaubt das (die Regeln
beschränken `internal/*`, nicht `cmd/`), und ein Architektur-Test hält es fest —
diese Zusage darf nicht bloß im Kommentar stehen.

### Die Route

`POST /v1/species/habitat-types`, gleicher Pfad, neuer Rumpf:

```json
{ "concept_ids": ["wcvp:concept:2457314", "cdm:concept:3b97…"] }
```

**Ein Eintrag je Eingabe, in Eingabereihenfolge, Duplikate eingeschlossen** —
dieser Vertrag besteht und bleibt: `response[i]` gehört zu `concept_ids[i]`.
Dedupliziert wird nur intern für die Index-Abfrage.

```json
[
  { "concept_id": "wcvp:concept:2457314", "known": true,
    "in_area": true, "habitat_types": [ … ] },
  { "concept_id": "cdm:concept:3b97…", "known": false,
    "reason": "unknown_backbone", "in_area": null, "habitat_types": [] }
]
```

`verbatim` entfällt, `resolved` wird `known`. Kein Bestandsbruch — es gibt keinen
Konsumenten.

Grenzen wie heute: `maxItems: 300` auf der **rohen** Array-Länge.

### Zwei Gründe, nicht einer

| `reason` | Bedeutung |
|---|---|
| `unknown_backbone` | Das Präfix passt nicht zum Index — der Aufrufer hat gegen ein anderes Backbone aufgelöst |
| `unknown_concept` | Richtiges Backbone, aber der ESy-Datensatz kennt zu dieser Art keine Habitat-Fakten |

Ohne die Unterscheidung sucht man den Fehler an der falschen Stelle. situs'
Index enthält heute ausschließlich `wcvp:concept:…` (gemessen: 11559 von 11559).
Für ein deutsches Exkursionstagebuch wäre `cdm` (die deutsche Standardliste, in
hostus vorhanden) eine naheliegende Wahl — deren IDs kennt der Index nicht. Ein
stiller Fehlschlag ist hier der wahrscheinlichste Fehler, deshalb wird er
benannt.

### Selbstauskunft statt Raten

`GET /v1/info` nennt, worauf der Index gebaut ist:

```json
{ "service": "situs", "version": "0.2.0",
  "index": { "concept_backbones": ["wcvp"], "species_with_concept": 11559,
             "area_scheme": "wgsrpd_l3", "areas_with_data": <gemessen> } }
```

Aus dem Index abgefragt, nicht konfiguriert — `areas_with_data` ist die Zahl der
distinkten Gebietscodes in `species_distribution` und steht erst nach dem ersten
Verbreitungs-Ingest fest. Ein Client prüft damit **vorab**, ob sein Backbone
passt.

**Bekannte Lücke, bewusst offen:** situs speichert die hostus-*Fassung* nicht
(`wcvp 2026-06-15`). Echte Provenienz verlangt, dass der Ingest sie mitschreibt —
eine Schema-Erweiterung und ein eigener Schnitt.

## Teil 2 — Verbreitung

### Woher die Daten kommen

hostus führt sie bereits: `GET /v1/concept/{id}` liefert ein
`distribution`-Array im Schema **WGSRPD Level 3**. Es gibt **keinen Batch-Weg** —
`/v1/match` enthält keine Verbreitung, und eine Sammelroute existiert nicht. Der
Ingest fragt daher je distinktem Konzept einmal ab: bei 3587 Konzepten und dem
Ratenlimit von 20/s rund **3 Minuten**, einmalig und offline. Mit 0,07 s Taktung
fehlerfrei gemessen.

Lizenz: der Index führt WCVP als `CC-BY-4.0` mit Weitergabe `allowed` — das
Übernehmen in situs ist gedeckt, die Namensnennung bleibt Pflicht.

### Schema

```sql
CREATE TABLE species_distribution (
  concept_id  TEXT NOT NULL,
  area_scheme TEXT NOT NULL,   -- 'wgsrpd_l3'
  area_code   TEXT NOT NULL,   -- 'GER'
  PRIMARY KEY (concept_id, area_scheme, area_code)
);
CREATE INDEX idx_species_distribution_area
  ON species_distribution(area_scheme, area_code);
```

**Kein Area-Namensverzeichnis.** Die Namen holt das Frontend bei ortus oder
hostus; situs spiegelt sie nicht.

### Neuer Port

Getrennt vom Namensauflöser — andere Verantwortung, andere Fehlersemantik:

```go
// DistributionSource liefert die Gebiete, in denen ein Konzept vorkommt.
type DistributionSource interface {
    Areas(ctx context.Context, conceptIDs []string) (map[string][]domain.Area, error)
}
```

Der hostus-Adapter implementiert beide Ports.

### Ingest-Reihenfolge

`IngestCSV` → `IngestSpeciesRoles` → **`IngestDistribution`** → `IngestLocalizations`
→ `DeriveGermanLabels`. Die Verbreitung braucht die Konzept-IDs, muss also nach
den Artenrollen laufen.

**Ein Fehler der Verbreitungsquelle bricht den Ingest NICHT ab.** Das ist eine
bewusste Abweichung von der Artenrollen-Regel: dort bricht ein Resolver-Fehler
ab, damit nicht alle 13791 Namen als unauflösbar verbucht werden. Hier wäre das
falsch — ein Index ohne Verbreitung ist brauchbar, nur ungefiltert. Der Report
nennt, wie viele Konzepte Verbreitungsdaten haben; eine Null dort ist eine
sichtbare Aussage, kein stiller Ausfall.

### Filter-Semantik

Auf den Routen mit Artenlisten — `GET /v1/habitat-type/{t}/{c}` (die drei
Rollen-Arrays) und `GET /v1/habitat-type/{t}/{c}/species`:

```
?area=GER                    → jede Art trägt in_area: true|false|null
?area=GER&only_in_area=true  → false-Einträge entfallen, null bleibt
```

| `in_area` | Bedeutung |
|---|---|
| `true` | Konzept hat eine Verbreitungszeile für dieses Gebiet |
| `false` | Konzept hat Verbreitungsdaten, dieses Gebiet ist nicht dabei |
| `null` | **unbekannt** — kein Konzept (unauflösbarer Name) oder ein Konzept ohne Verbreitungszeilen |

Zwei Ursachen, ein Zustand: für die Feldfrage („lohnt es, danach zu suchen?")
sind beide dasselbe — nicht wissbar. Beide Ursachen sind dokumentiert.

**`null` wird niemals wegfiltert.** Eine Liste, die 8 % stillschweigend verliert,
wäre unehrlich sauber. Das ist dieselbe Regel, die situs bei unauflösbaren Namen
befolgt: behalten und kennzeichnen, nie verwerfen.

Ohne `?area=` erscheint `in_area` **nicht** — kein Feld, das immer `null` ist.

### Der Area-Code

`?area=` erwartet einen **WGSRPD-L3-Code** (`GER`, `FRA`, `BAL`). Das Frontend
bestimmt ihn ohnehin aus GPS, deshalb braucht situs **keine ISO-Abbildung** — und
das `CZE`-Problem (eine Area für Tschechien *und* die Slowakei) entsteht hier
nicht.

Ein Code, zu dem der Index keine Daten hat, ergibt **`INVALID_QUERY`** — nicht
eine Liste voller `false`. situs kann das prüfen, weil es seine Codes kennt.

### `in_area` auf der Batch-Route

`area` ist auf **allen** Routen ein Query-Parameter, auch auf der POST-Route —
`lang` wird dort heute schon so übergeben, und der Parameter beschreibt die Sicht
auf die Antwort, nicht die Nutzlast.

Jeder Batch-Eintrag trägt `in_area` für das **abgefragte** Konzept. Dieselbe
Abfrage, kein Zusatzaufwand — und sie beantwortet im Feld eine echte Frage: *„Die
Zeder, die ich hier notiert habe, kommt in Deutschland nicht vor — habe ich mich
verbestimmt?"* Ohne `?area=` ist das Feld nicht vorhanden.

## Warum die Verbreitung Artenlisten korrigiert, aber keine Habitattypen filtert

Gemessen (Details im Spike): der Anteil unmöglicher Arten schwankt je Typ
zwischen **0 % und 88 %**. Typen, die in Deutschland nicht vorkommen, rauschen
stark; Typen, die hier vorkommen, meist gar nicht. Das Problem sitzt bei **Typen
mit weitem europäischem Areal, die Deutschland am Rand erreichen** — R15
(kontinentaler Steppen-Trockenrasen, fränkisch/thüringisch) enthält **50 %**
Arten, die es hier nicht gibt, darunter *Peucedanum hispanicum* und *Cirsium
pyrenaicum*. Genau dort ist die Bestimmung ohnehin schwer.

Umgekehrt taugt die Artverbreitung **nicht**, um Habitattypen einzugrenzen: von
22 Habitattypen der Preiselbeere bleiben bei jeder sinnvollen Schwelle alle
stehen, obwohl es Palsamoore und Strauchtundra in Deutschland nicht gibt — ihre
*Arten* kommen hier vor, der *Typ* nicht. Habitattypen einzugrenzen ist Aufgabe
von Article 17 und der Syntaxa-Verbreitung; beides ist **nicht** Teil dieses
Schnitts.

## Fehlerbehandlung

| Fall | Antwort |
|---|---|
| Unbekannter `area`-Code | `400 INVALID_QUERY` |
| `concept_ids` leer oder > 300 | `400 INVALID_QUERY` |
| Konzept-ID mit fremdem Präfix | `200`, Eintrag mit `known: false, reason: "unknown_backbone"` |
| Konzept ohne Habitat-Fakten | `200`, Eintrag mit `known: false, reason: "unknown_concept"` |
| Rumpf mit mehr als einem JSON-Wert | `400 INVALID_QUERY` |

Der Fehlerumschlag bleibt `{"error":{"code","message"}}`. `UPSTREAM_UNAVAILABLE`
entfällt aus dem Code — es gibt zur Laufzeit keinen Upstream mehr.

## Tests

- **Architektur:** `internal/app` importiert den hostus-Adapter nicht mehr
  (Architektur-Test, nicht nur Kommentar).
- **Batch:** ein Eintrag je Eingabe in Eingabereihenfolge inkl. Duplikate;
  `unknown_backbone` gegen `unknown_concept`; `maxItems`-Grenze; Trailing-Daten.
- **`in_area`:** alle drei Zustände; `only_in_area=true` entfernt `false` und
  behält `null`; ohne `?area=` erscheint das Feld nicht; unbekannter Code → 400.
- **Ingest:** ein Stub-`DistributionSource`, der einen Fehler liefert, bricht den
  Ingest **nicht** ab und der Report weist null Verbreitungskonzepte aus.
- **Seam-Test** (sqlite → application → http) deckt Batch und `?area=` mit echter
  In-Memory-DB und echtem Router ab.
- Beide OpenAPI-Kopien byte-identisch, Contract-Test in beide Richtungen.

## Nicht in diesem Schnitt

Ko-Vorkommen-Rangliste (S2), Article-17-Filter für Anhang-I-Typen,
Syntaxa-Verbreitung, das Mitschreiben der hostus-Backbone-Fassung, eine
ISO→WGSRPD-Abbildung. Die Antwortformen lassen alles davon additiv zu.
