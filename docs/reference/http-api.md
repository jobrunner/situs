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
| `GET /v1/info` | Name und Version des Dienstes |
| `GET /v1/habitat-type/{typology}/{code}` | Habitattyp mit Arten, Syntaxa und Crosswalks |
| `GET /v1/habitat-type/{typology}/{code}/species?role=` | Artenliste, optional nach Rolle gefiltert |
| `GET /v1/species/{conceptId}/habitat-types` | Habitattypen einer Art (mit Rolle) |
| `POST /v1/species/habitat-types` | Batch über verbatim Namen (via hostus) |
| `GET /v1/syntaxon/{id}/habitat-types` | Habitattypen einer Pflanzengesellschaft |

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

## Die zwei Arten-Pfade

`GET /v1/species/{conceptId}/habitat-types` antwortet **404**, wenn der Index zu
dieser Konzept-ID keine Fakten kennt: eine Konzept-ID existiert hier nur, weil
eine Artenzeile sie trägt, „kein Treffer" heißt also „diese Art kommt in keinem
Habitattyp vor". Ein leeres oder nur aus Leerzeichen bestehendes Segment ist
`INVALID_QUERY` (400).

`POST /v1/species/habitat-types` antwortet für denselben Zustand dagegen mit
`resolved: true` und `habitat_types: []`. Das ist gewollt: dort hat hostus den
Namen gerade auf ein Konzept aufgelöst und bürgt damit für dessen Existenz — die
Antwort lautet „Konzept ja, Fakten nein". Ein nicht auflösbarer Name kommt mit
`resolved: false` zurück und wird nie verworfen, damit eine Teilauflösung nicht
die ganze Anfrage kostet.

## Grenzen des Batch-Endpunkts

`POST /v1/species/habitat-types` ist zweifach begrenzt: der Body auf **1 MiB**
und das `names`-Array auf **300 Einträge** (`maxItems` in der Spezifikation).
Beides ergibt `INVALID_QUERY` (400), ohne hostus überhaupt zu rufen. Die
Array-Grenze ist nötig, weil die Byte-Grenze sie nicht impliziert: 1 MiB kurzer
Namen sind rund 50 000 Einträge, und je 50 Namen kostet einen hostus-Roundtrip
von gemessen bis zu 16,3 s. 300 liegt weit über jeder realistischen
Geländeaufnahme und deckelt den schlechtesten Fall auf sechs Roundtrips.

Doppelte Namen sind erlaubt. Nach oben (hostus) wird dedupliziert — das begrenzt
die Roundtrips —, die Antwort trägt aber **einen Eintrag je Eingabename in
Eingabereihenfolge**, sodass `response[i]` zu `names[i]` gehört. Ein doppelt
genannter Name liefert also zweimal dieselbe Auflösung. Leere bzw. nur aus
Whitespace bestehende Einträge werden verworfen; enthält `names` danach keinen
Namen mehr, ist das `INVALID_QUERY`.

## Fehler

Fehler kommen einheitlich als `{"error":{"code":"...","message":"..."}}` mit den
Codes `INVALID_QUERY`, `NOT_FOUND`, `UPSTREAM_UNAVAILABLE` und
`INTERNAL_ERROR`.

Ein nicht auflösbarer verbatim Name ist ausdrücklich **kein** Fehlerfall (und
deshalb gibt es keinen Code `UNRESOLVABLE`): er wird je Eingabe mit
`resolved: false` gemeldet. `UPSTREAM_UNAVAILABLE` (502) bedeutet, dass hostus
nicht geantwortet hat (Transportfehler, Timeout, hostus-5xx). Antwortet hostus
mit einem 4xx, ist das ein Fehler auf situs-Seite und wird als
`INTERNAL_ERROR` gemeldet — sonst würde der Betrieb im falschen System suchen.

## Bekannte Grenze: Leseverhalten

Die Arten- und Syntaxon-Pfade lesen pro gefundenem Habitattyp nachträglich
dessen Typ-Zeile, Label und Syntaxa (N+1). Gegen eine lokale SQLite-Datei ist
das unkritisch — die reale Ajuga-reptans-Antwort sind rund 33 Zugriffe —, aber
bevor dieser Dienst nebenläufige Last bedient, sind die Nachlesen zu
Sammelabfragen zusammenzufassen.
