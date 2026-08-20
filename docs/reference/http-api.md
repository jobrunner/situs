# HTTP-API

Die verbindliche Beschreibung ist die OpenAPI-Spezifikation. Sie ist in die
Binary eingebettet und wird unter `GET /openapi` ausgeliefert; eine
byte-identische Kopie liegt in `api/openapi/openapi.yaml` (ein Test erzwingt die
Gleichheit, ein zweiter, dass Routen und Spezifikation sich in beide Richtungen
decken).

| Route | Zweck |
|---|---|
| `GET /health/live` | Liveness-Probe |
| `GET /health/ready` | Readiness-Probe |
| `GET /metrics` | Prometheus-Metriken |
| `GET /openapi` | diese Spezifikation |
| `GET /docs` | Swagger-UI für diese Spezifikation, offline-fähig (Assets eingebettet) |
| `GET /v1/info` | Name, Version und Selbstauskunft des Index |
| `GET /v1/habitat-type/{typology}/{code}` | Habitattyp mit Arten, Syntaxa und Crosswalks |
| `GET /v1/habitat-type/{typology}/{code}/species?role=` | Artenliste, optional nach Rolle gefiltert |
| `GET /v1/species/{conceptId}/habitat-types` | Habitattypen einer Art (mit Rolle) |
| `POST /v1/species/habitat-types` | Batch über Konzept-IDs (`concept_ids`) |
| `GET /v1/syntaxon/{id}/habitat-types` | Habitattypen einer Pflanzengesellschaft |

**Die Leseseite läuft ohne hostus.** Jede Route hier wird allein aus der lokalen
SQLite-Datei beantwortet; kein Lesepfad ruft einen Upstream-Dienst. hostus wird
nur beim `ingest` gebraucht (Namen → Konzept-IDs, Konzept → Verbreitung). Ein
Architekturtest hält das fest: die Kompositionswurzel des Serve-Pfads
(`internal/app`) darf den hostus-Adapter nicht einmal importieren.

Ein Habitattyp wird immer über `(typology, code)` adressiert — dieselbe Route
trägt EUNIS und Anhang I (`annex1`), ein weiteres System braucht keinen neuen
Endpunkt. Ein **leeres** Segment fällt auf `eunis@2021` zurück — weglassen lässt
sich das Segment nicht, weil der Pfad es verlangt und der Router einen fehlenden
Pfadteil gar nicht auf die Route abbildet. Eine **unbekannte** Typologie ist
`INVALID_QUERY` (400), ein unbekannter Code innerhalb einer bekannten Typologie
`NOT_FOUND` (404).

**Ko-Kennarten** (welche weiteren Kennarten lohnen sich zu suchen, wenn eine
bekannt ist?) haben absichtlich **keinen** eigenen Endpunkt: die Frage ist mit
zwei Aufrufen der vorhandenen Routen beantwortet — erst
`GET /v1/species/{conceptId}/habitat-types`, dann je Treffer
`GET /v1/habitat-type/{typology}/{code}/species?role=diagnostic`. Ein
zusammenfassender Endpunkt wäre nur eine Bequemlichkeit und würde die Gewichtung
mehrerer Habitattypen vorwegnehmen — genau das, was dieses Fundament bewusst
offen lässt (kein Scoring/Ranking). Nicht vergessen, sondern entschieden.

Die Crosswalk-Liste trägt beides: andere EUNIS-Fassungen und
Anhang-I-Entsprechungen, in beiden Richtungen abfragbar. Sie ist leer, wenn es
keine gibt — bei Anhang I ist das der Normalfall. Der Qualifier liest sich immer
als „abgefragter Typ *qualifier* dieser Typ"; eine gespeicherte Zeile, die auf
den abgefragten Typ zeigt, wird dafür invertiert (`<` ↔ `>`).

## Sprache

Jeder Endpunkt akzeptiert `?lang=de` (alternativ `Accept-Language`), Default ist
`en`. Lokalisierung ist **additiv**: `name_en` bleibt gesetzt und ist die
Identität, `name_de` kommt hinzu und trägt mit `name_de_provenance`
(`official` | `curated` | `derived`) seine Herkunft. Eine nicht unterstützte
Sprache ist kein Fehler, sondern fällt auf `en` zurück; ein ausdrückliches
`?lang=fr` fällt direkt auf `en` zurück und nicht auf ein mitgesendetes
`Accept-Language` — gefragt war weder Deutsch noch Englisch.

Listen werden immer als `[]` ausgeliefert, nie als `null`. Fehlende Einzelwerte
(`level`, `priority`, `concept_id`, `fidelity`, `constancy`) fehlen als Feld,
statt eine 0 oder ein `false` zu behaupten.

## Selbstauskunft: `GET /v1/info`

Neben `service` und `version` trägt die Antwort ein `index`-Objekt, dessen
Zahlen alle **am Index gemessen** sind — keine davon ist konfiguriert. Die
folgende Antwort ist echt: sie stammt vom vollständigen Ingest des gepinnten
Datenstands (dieselben Zahlen wie in `measured-index.md`; `version` hängt
naturgemäß am jeweiligen Build):

```json
{
  "service": "situs",
  "version": "0.1.1",
  "index": {
    "concept_backbones": ["wcvp"],
    "species_with_concept": 3135,
    "area_scheme": "wgsrpd_l3",
    "areas_with_data": 366
  }
}
```

`concept_backbones` sind die im Index vorkommenden Konzept-ID-Präfixe, gemessen
über alle `concept_id`-Werte. `areas_with_data` ist die Zahl der verschiedenen
Gebietscodes in `species_distribution` — solange kein Verbreitungs-Ingest
gelaufen ist, steht dort `0`, und dann ist ein `?area=` mit **jedem** Code
`INVALID_QUERY`.

Wozu die Selbstauskunft taugt und wozu nicht: sie sagt, worauf dieser Index
gebaut ist, und macht damit einen Bruch sichtbar — beantwortbar sind aber nur
`wcvp:`-IDs, denn die Batch-Route prüft gegen ein **fest einkompiliertes**
Präfix (siehe unten). Meldet `concept_backbones` etwas anderes als `["wcvp"]`,
ist das kein Hinweis darauf, dass diese IDs nun beantwortet würden, sondern ein
Hinweis darauf, dass Index und Binary nicht zueinander passen.

Nicht enthalten ist die *Fassung* der Backbone (etwa `wcvp 2026-06-15`): der
Ingest schreibt sie heute nicht mit, und eine erfundene Fassung wäre schlimmer
als keine.

Die Zahlen werden **je Anfrage** ermittelt (zwei `SELECT DISTINCT`, siehe
„Bekannte Grenze: Leseverhalten"), nicht zwischengespeichert — eine gemessene
Zahl, die aus einem Cache stammt, wäre keine gemessene Zahl mehr.

Fällt eine der beiden Abfragen aus, antwortet die Route **500**
(`INTERNAL_ERROR`) und nicht ein mit Nullen gefülltes `index`-Objekt: das läse
sich für einen Client wie „leerer Index" oder „falsches Backbone".

## Die zwei Arten-Pfade

`GET /v1/species/{conceptId}/habitat-types` antwortet **404**, wenn der Index zu
dieser Konzept-ID keine Fakten kennt: eine Konzept-ID existiert hier nur, weil
eine Artenzeile sie trägt, „kein Treffer" heißt also „diese Art kommt in keinem
Habitattyp vor". Ein leeres oder nur aus Leerzeichen bestehendes Segment ist
`INVALID_QUERY` (400).

`POST /v1/species/habitat-types` beantwortet denselben Zustand dagegen mit
**200** und meldet ihn je Eingabe. Der Body ist ein Array von Konzept-IDs, die
der Aufrufer schon hat — aus hostus, aus einem eigenen Cache, egal woher; situs
löst zur Laufzeit keinen verbatim Namen mehr auf:

```json
{"concept_ids": ["wcvp:concept:2457314", "cdm:concept:x"]}
```

Ein anderes Feld als `concept_ids` (etwa das frühere `names`) ist
`INVALID_QUERY` — der Decoder verbietet unbekannte Felder, damit ein alter
Client eine klare 400 bekommt statt einer leeren Antwort.

Jeder Eintrag der Antwort trägt `known` und, wenn `known: false`, ein `reason`
mit genau zwei möglichen Werten:

| `reason` | Bedeutung | Wessen Fehler |
|---|---|---|
| `unknown_backbone` | Das ID-Präfix ist nicht `wcvp:` | der des Aufrufers |
| `unknown_concept` | Präfix `wcvp:`, aber der Index kennt zu dieser ID keine Fakten | die Grenze der Daten |

Zwei getrennte Werte, weil es zwei verschiedene Fehler sind: eine gemeinsame
Bezeichnung würde die Suche im falschen System beginnen lassen.

Geprüft wird gegen **`wcvp`, eine Konstante in der Binary** — nicht gegen das,
was `/v1/info` meldet. Das ist Absicht: eine Ableitung pro Anfrage würde jede
Batch-Anfrage eine zusätzliche Abfrage kosten, und ein Index mit gemischten
Backbones ist ohnehin kein Zustand, den dieser Entwurf trägt. Die Folge, die man
kennen muss: gegen einen Index, der **nicht** auf wcvp gebaut ist, meldet
`/v1/info` treu dessen Backbone, während jede seiner IDs hier
`unknown_backbone` bekommt. Die Selbstauskunft macht diesen Bruch sichtbar; sie
verschiebt die Prüfung nicht.

## Grenzen des Batch-Endpunkts

`POST /v1/species/habitat-types` ist zweifach begrenzt: der Body auf **1 MiB**
und das `concept_ids`-Array auf **300 Einträge** (`maxItems` in der
Spezifikation). Beides ergibt `INVALID_QUERY` (400). Die Array-Grenze ist nötig,
weil die Byte-Grenze sie nicht impliziert: 1 MiB kurzer IDs sind Zehntausende
Einträge, und jede verschiedene kostet eine Handvoll Index-Abfragen. 300 liegt
weit über jeder realistischen Geländeaufnahme.

Doppelte IDs sind erlaubt. Die Index-Arbeit wird intern dedupliziert, die Antwort
trägt aber **einen Eintrag je Eingabe-ID in Eingabereihenfolge**, sodass
`response[i]` zu `concept_ids[i]` gehört. Ein leerer bzw. nur aus Whitespace
bestehender Eintrag wird **abgewiesen**, nicht übersprungen: übersprungen würde
er die Liste eines vertrauenden Clients um eins verschieben, und
`unknown_backbone` wäre gelogen, weil ein leerer String keine andere Backbone
ist. Ein leeres `concept_ids` ist ebenfalls `INVALID_QUERY`.

## Gebietsfilter: `?area=` und `?only_in_area=`

`GET /v1/habitat-type/{typology}/{code}`,
`GET /v1/habitat-type/{typology}/{code}/species`,
`GET /v1/species/{conceptId}/habitat-types` und
`POST /v1/species/habitat-types` nehmen `?area=` und `?only_in_area=`.

`area` ist ein **WGSRPD-Level-3-Code** (`GER`, `AUT`, …) — das Frontend leitet
ihn aus der GPS-Position ab, situs braucht deshalb keine ISO-Abbildung. Ein Code,
für den der Index keine Daten hat, ist `INVALID_QUERY` (400) und **nicht** eine
Liste voller „kommt nicht vor": ein Tippfehler und eine echte Abwesenheit dürfen
nicht gleich aussehen.

**Welche Codes gültig sind, sagt keine Route.** `areas_with_data` in `/v1/info`
nennt nur ihre *Anzahl* — daraus lässt sich ablesen, ob überhaupt ein
Verbreitungs-Ingest gelaufen ist (`0` heißt nein), nicht aber, ob `GER` dabei
ist. Ein Client, der die Codes braucht, muss sie kennen (WGSRPD Level 3 ist ein
veröffentlichtes Vokabular) oder am 400 erkennen. Ein Endpunkt, der sie
auflistet, ist bewusst nicht gebaut, aber die naheliegende Ergänzung, sobald
jemand sie braucht.

Mit `?area=` trägt jeder Arteneintrag ein `in_area` mit **drei** Zuständen:

| `in_area` | Bedeutung |
|---|---|
| `true` | Die Art ist für dieses Gebiet verzeichnet |
| `false` | Verbreitungsdaten liegen vor und führen dieses Gebiet **nicht** |
| Feld fehlt | Nicht entscheidbar: keine Konzept-ID, oder das Konzept hat gar keine Verbreitungszeilen |

Ohne `?area=` fehlt `in_area` überall — ein `false` würde sonst „kommt hier nicht
vor" behaupten, obwohl niemand nach einem Ort gefragt hat.

`only_in_area=true` (nur zusammen mit `area` wirksam, sonst ein No-op) entfernt
die Einträge mit `in_area: false`. Die **unentscheidbaren bleiben**: eine Liste,
die stillschweigend verliert, was sie nicht beurteilen kann, wäre unehrlich
sauber. Ein `only_in_area`, das sich nicht als Boolean lesen lässt, ist
`INVALID_QUERY`.

Auf der Batch-Route sitzt `in_area` **am Eintrag** (es beschreibt das Konzept,
nicht den Habitattyp) und fehlt in den verschachtelten `habitat_types`. Und
`only_in_area` verwirft dort nie einen Eintrag: `response[i]` muss zu
`concept_ids[i]` gehören, also darf kein Eintrag verschwinden. Auf
`GET /v1/species/{conceptId}/habitat-types` dagegen wirkt es auf die ganze
Antwort: ist das Konzept für das Gebiet nachweislich nicht verzeichnet, ist die
Liste leer.

## Fehler

Fehler kommen einheitlich als `{"error":{"code":"...","message":"..."}}` mit den
Codes `INVALID_QUERY`, `NOT_FOUND` und `INTERNAL_ERROR`.

Es gibt **kein** `UPSTREAM_UNAVAILABLE`: der Lesepfad ist autark, zur Laufzeit
hängt kein Upstream-Dienst daran, der ausfallen könnte. Und es gibt kein
`UNRESOLVABLE`: eine Konzept-ID, die der Index nicht beantworten kann, ist kein
Fehlerfall, sondern eine normale 200-Antwort mit `known: false` und einem
`reason` — eine unbekannte ID darf nicht die ganze Anfrage verwerfen.

## Bekannte Grenze: Leseverhalten

Die Arten- und Syntaxon-Pfade lesen pro gefundenem Habitattyp nachträglich
dessen Typ-Zeile, Label und Syntaxa (N+1). Gegen eine lokale SQLite-Datei ist
das unkritisch — die reale Ajuga-reptans-Antwort sind rund 33 Zugriffe —, aber
bevor dieser Dienst nebenläufige Last bedient, sind die Nachlesen zu
Sammelabfragen zusammenzufassen.

`GET /v1/info` gehört in dieselbe Kategorie: es scannt je Anfrage
`species_role.concept_id` und `species_distribution.area_code` distinct, statt
die Zahlen zu cachen. Für eine Route, die ein Client einmal beim Start ruft, ist
das richtig — ein Cache würde aus einer gemessenen Zahl eine behauptete machen,
und ein Index kann sich unter dem laufenden Prozess ändern. Wer die Route in
eine Schleife legt, sollte wissen, was sie kostet.
