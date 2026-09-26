# HTTP-API

Die verbindliche Beschreibung ist die OpenAPI-Spezifikation. Sie ist in die
Binary eingebettet und wird unter `GET /openapi` ausgeliefert; eine
byte-identische Kopie liegt in `api/openapi/openapi.yaml` (ein Test erzwingt die
Gleichheit, ein zweiter, dass Routen und Spezifikation sich in beide Richtungen
decken).

| Route | Zweck |
|---|---|
| `GET /` | API-Explorer, selbst-enthalten |
| `GET /health/live` | Liveness-Probe |
| `GET /health/ready` | Readiness-Probe |
| `GET /metrics` | Prometheus-Metriken |
| `GET /openapi` | diese Spezifikation |
| `GET /docs` | Swagger-UI für diese Spezifikation, offline-fähig (Assets eingebettet) |
| `GET /v1/info` | Name, Version und Selbstauskunft des Index |
| `GET /v1/typologies` | alle geführten Typologien, nach `id` sortiert |
| `GET /v1/areas` | alle Gebiete mit Verbreitungsdaten (Code + Name), nach `code` sortiert |
| `GET /v1/habitat-type/{typology}/{code}` | Habitattyp mit Arten, Syntaxa und Crosswalks |
| `GET /v1/habitat-type/{typology}/{code}/species?role=` | Artenliste, optional nach Rolle gefiltert |
| `GET /v1/species/search?q=&limit=` | Namenssuche über die im Index geführten `verbatim_name` |
| `GET /v1/species/{conceptId}/habitat-types` | Habitattypen einer Art (mit Rolle) |
| `POST /v1/species/habitat-types` | Batch über Konzept-IDs (`concept_ids`) |
| `GET /v1/syntaxa?rank=&life_form_group=&area=&include=` | Wurzeln der Syntaxa-Hierarchie; ohne `?rank=` die 25 Formationen; `?area=` filtert auf Vorkommen |
| `GET /v1/syntaxon/{id}` | ein Syntaxon mit Ahnenpfad und direkten Kindern |
| `GET /v1/syntaxon/{id}/habitat-types` | Habitattypen einer Pflanzengesellschaft |
| `GET /v1/species/{conceptId}/traits?vocab=` | Zeigerwerte einer Art, optional nach Vokabular gefiltert |
| `POST /v1/species/traits/summary` | Zeigerwertanalyse über eine Artenliste |

## API-Explorer: `GET /`

Eine selbst-enthaltene Weboberfläche, die jeden Lese-Endpunkt dieses Dienstes
ausprobierbar macht und dabei die jeweils abgesetzte URL mitzeigt. Stylesheet
und Skript sind eingebettet, nichts wird aus dem Netz nachgeladen — situs läuft
lokal und im Feld ohne Netz. Die Seite rechnet **nichts selbst aus**: sie zeigt,
was der Dienst antwortet, keine eigene Aggregation, kein eigenes Ranking. Die
Route ist exakt gebunden (`GET /`, kein Prefix-Match), ein unbekannter Pfad
bleibt `404`.

Ein **Footer** schließt die Seite ab: die Verweise auf `/docs`, `/openapi` und
`/health/ready`, die Urheberangabe (`© 2026 Jo Brunner · Code unter MIT`), die
Herkunft **jeder** Quelle mit der Lizenz, die sie tatsächlich trägt, und die
gebaute Version. Ohne ihn war die Spezifikation zwar vorhanden, aber von der
Startseite aus nur zu erraten.

Die Vollständigkeit der Quellenangabe ist keine Höflichkeit: EUNIS-ESy, die
EVC-Verbreitungskarten, die Habitat-Factsheets und die drei Zeigerwert-Quellen
(EIVE, Tichý, Midolo) stehen unter CC BY 4.0 und verlangen die Namensnennung
der **Urheber** ausdrücklich auch für abgeleitete Daten — und die Zeigerwerte
gehen über `GET /v1/species/{conceptId}/traits` an die Nutzer. Die
EuroVegChecklist steht dabei **nicht** unter CC BY 4.0, sondern unter den
Nutzungsbedingungen von FloraVeg.EU.

Die Version wird **beim Bau der Seite einmal ersetzt** (Platzhalter
`<!--situs:version-->` in `explorer.html`, ersetzt in `NewServer`), nicht aus
`/v1/info` nachgeladen: ein Footer, der den Index fragen muss, wer er ist,
bleibt genau dann leer, wenn der Index das Kaputte ist. Ein ungestempelter Bau
zeigt `situs dev` — derselbe Platzhalter, den auch `situs version` ausgibt.
Weil die Ersetzung an einem fehlenden Platzhalter stillschweigend nichts tut,
prüft ein eigener Test das eingebettete Asset auf genau ein Vorkommen, und ein
zweiter, dass jedes `href` im Footer eine registrierte Route trifft. Die
Versionszeile trägt bewusst kein `opacity`: ein früheres `opacity:.75` drückte
ihren Kontrast auf gemessene 3,3:1 (hell) und 4,34:1 (dunkel), wo WCAG AA bei
12,8px 4,5:1 verlangt; ohne diese Deklaration misst dieselbe Zeile 5,64:1 und
6,74:1.

Die Seite ist **auf ein Telefon ausgelegt**, und das ist mehr als eine
Medienabfrage: ein `<select>` lässt sich nicht schmaler setzen als seine
längste `<option>`. Die Typologie-Auswahl trägt
`annex1 — Habitats Directive Annex I (2013…)` und maß damit 421 px, worauf die
Seite 462 px brauchte — ein Telefon beantwortet das mit einem geweiteten
Layout-Viewport und skaliert die ganze Seite auf 84 %, sodass aus 15 px 12,7 px
werden. Zwei Regeln halten das auf, und gemessen sind beide nötig:
`max-width:100%` auf den Formularelementen und `min-width:0` auf den
Flex-Kindern — ohne die zweite bleibt die erste wirkungslos, weil ein Flex-Item
mit `min-width:auto` gar nicht erst unter seine eigene min-content-Breite
schrumpft. Mit beiden bleibt der Layout-Viewport auf der Gerätebreite.

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

## Namenssuche: `GET /v1/species/search`

Sucht einen Teilstring (case-insensitiv) in den `verbatim_name`, die **dieser
Index selbst** trägt, und liefert die zugehörige Concept-ID mit:

```json
[
  {"verbatim_name": "Fagus orientalis", "concept_id": null},
  {"verbatim_name": "Fagus sylvatica", "concept_id": "wcvp:concept:83891"}
]
```

**Das ist keine Namensauflösung.** Kein Fuzzy-Matching, keine Synonyme, keine
Autorenvarianten, kein Backbone-Wissen — dafür ist hostus zuständig. Diese
Route beantwortet ausschließlich „welche Namen kennt *dieser* Index", nicht
„was heißt dieser Name". `concept_id: null` heißt, dass der Ingest den Namen
nicht auflösen konnte; solche Treffer werden **mitgeliefert und nicht
gefiltert**, weil sie zur Wahrheit über den Index gehören — eine gefilterte
Liste würde eine höhere Auflösungsquote behaupten, als der Index tatsächlich
hat. Gemessen am gepinnten Datenstand führt der **fertige Index** **3780**
distinkte `verbatim_name`, davon **3314** mit Concept-ID — das schließt die
Mitgliedsarten ein, die erst durch die Aggregat-Ableitung (`provenance =
'derived_from_aggregate'`) hinzukommen. Das ist eine andere Grundgesamtheit
als die **3587** Artennamen in [measured-index.md](measured-index.md), die die
**Eingabe** misst — die Namen aus `species_roles.csv` vor der
Aggregat-Ableitung.

`q` ist Pflicht: leer (oder nur Whitespace) ist `INVALID_QUERY`, nicht „liefere
alles". `limit` hat die Vorgabe **20** und das Maximum **100**; ein Wert
außerhalb davon oder ein nicht ganzzahliger Wert ist ebenfalls
`INVALID_QUERY` — ein verschluckter Tippfehler, der still auf einen anderen
Wert zurückfällt, wäre die Fehlerquelle, die diese Prüfung vermeidet. Die
Treffer sind nach Namen sortiert.

## Zeigerwertanalyse: `POST /v1/species/traits/summary`

Mittelt die Zeigerwerte (EIVE, Tichý, Midolo) einer Artenliste — der Body ist
derselbe Konzept-ID-Satz wie bei `POST /v1/species/habitat-types`:

```json
{"concept_ids": ["wcvp:concept:83891", "wcvp:concept:2692970"]}
```

Gemessen am gepinnten Datenstand: eine Abfrage über zwei Arten liefert für EIVE
5 von 5 Dimensionen mit gewichtetem Mittel, für Tichý 0 von 6 und für Midolo 0
von 5 — die Trennung nach Vokabular hält also im echten Betrieb, nicht nur auf
dem Papier.

Die Rechenregeln:

| Regel | Warum |
|---|---|
| Gemittelt wird **strikt je Vokabular und Dimension**, nie darüber hinweg | EIVE (0–10) und Tichý (1–12) sind verschiedene Skalen; ihr gemeinsames Mittel wäre eine erfundene Zahl |
| `mean_niche_weighted` gewichtet mit `1/Nischenbreite` | eine schmale Nische ist der präzisere Standortanzeiger |
| `mean_niche_weighted` **fehlt** bei Vokabularen ohne Nischenbreiten (Tichý, Midolo) | es wird nie mit dem ungewichteten Mittel gefüllt — sonst ließe sich Gewichtet nicht mehr von Ungewichtet unterscheiden |
| Arten mit Nischenbreite ≤ 0 fallen aus dem gewichteten Mittel und zählen in `n_excluded_weighted` | eine Division durch eine nicht positive Breite wäre kein Gewicht, sondern ein Datenfehler, der sichtbar bleiben muss |
| `sd` ist die Stichproben-Standardabweichung (Teiler n−1) und **fehlt bei n < 2** | aus einem Messwert lässt sich keine Streuung berechnen; eine `0` würde Übereinstimmung behaupten |
| `n` und `n_missing` stehen **je Dimension**, nicht global | EIVE deckt L/M/N/R/T ungleichmäßig ab — ein Mittel über 3 von 18 Arten ist eine andere Aussage als eines über 17 von 18 |

Unbekannte Concept-IDs sind kein Fehler, sondern kommen mit derselben
Begründung wie bei `POST /v1/species/habitat-types` zurück
(`unknown_backbone` bei unpassendem Präfix, `unknown_concept` bei passendem
Präfix ohne Daten). Eine Anfrage, in der kein einziges Konzept auflösbar ist,
ist ein normales **200** mit leeren `vocabularies`, kein `NOT_FOUND` — die
Analyse einer Liste ganz unbekannter Arten ist eine gültige, wenn auch leere
Antwort. Ein leeres `concept_ids`-Array bleibt dagegen `INVALID_QUERY`, wie
beim Batch-Endpunkt, mit denselben Grenzen (Body 1 MiB, höchstens 300
Einträge, siehe unten).

## Zeigerwerte: `GET /v1/species/{conceptId}/traits`

Pflanzenökologische Zeigerwerte (EIVE, Tichý, Midolo) zu einer Konzept-ID,
optional per `?vocab=` auf ein Vokabular gefiltert. Wie die Habitat-Routen
autark und mit demselben Leseverhalten: ein unbekanntes Vokabular ist
`400 INVALID_QUERY`, ein Konzept ohne Trait-Daten liefert ein leeres Array,
kein 404.

## Sprache

Jeder Endpunkt akzeptiert `?lang=de` (alternativ `Accept-Language`), Default ist
`en`. Lokalisierung ist **additiv**: `name_en` bleibt gesetzt und ist die
Identität, `name_de` kommt als **Objekt** hinzu und trägt seine Herkunft mit:

```json
"name_de": {
  "value": "Entkalkte Dünen mit Empetrum nigrum",
  "provenance": "derived",
  "source": "derived-annex1"
}
```

`provenance` ist eines von `official` | `curated` | `derived` | `situs`;
`situs` ist die schwächste Aussage — situs hat selbst übersetzt, keine externe
Quelle steht dahinter, und ein solches Label darf nie eine Ableitung speisen.
Ein etablierter deutscher Begriff steht, wo es ihn gibt, zusätzlich in
`vernacular`. Eine nicht unterstützte
Sprache ist kein Fehler, sondern fällt auf `en` zurück; ein ausdrückliches
`?lang=fr` fällt direkt auf `en` zurück und nicht auf ein mitgesendetes
`Accept-Language` — gefragt war weder Deutsch noch Englisch.

Listen werden immer als `[]` ausgeliefert, nie als `null`. Fehlende Einzelwerte
(`level`, `priority`, `concept_id`, `fidelity`, `constancy`) fehlen als Feld,
statt eine 0 oder ein `false` zu behaupten.

## Selbstauskunft: `GET /v1/info`

Neben `service` und `version` trägt die Antwort ein `index`-Objekt, dessen
Zahlen alle **am Index gemessen** sind — keine davon ist konfiguriert. Die
folgende Antwort ist echt: sie stammt vom Referenzlauf 2026-09-16 (dieselben
Zahlen wie in `measured-index.md`; `version` hängt naturgemäß am jeweiligen
Build):

```json
{
  "service": "situs",
  "version": "0.7.0",
  "index": {
    "concept_backbones": ["cdm", "eurosl", "wcvp"],
    "species_with_concept": 3323,
    "area_scheme": "wgsrpd_l3",
    "areas_with_data": 366,
    "syntaxon_area_scheme": "evc_territory",
    "syntaxa_with_distribution": 1114
  }
}
```

`concept_backbones` sind die im Index vorkommenden Konzept-ID-Präfixe, gemessen
über alle `concept_id`-Werte. `areas_with_data` ist die Zahl der verschiedenen
Gebietscodes in `species_distribution` — solange kein Verbreitungs-Ingest
gelaufen ist, steht dort `0`, und dann ist ein `?area=` mit **jedem** Code
`INVALID_QUERY`.

`area_scheme` meint weiterhin **ausschließlich** die Artverbreitung
(`wgsrpd_l3`, fest verdrahtet — der Index kennt kein zweites Artenschema).
Das zweite Schemapaar, `syntaxon_area_scheme` (immer `evc_territory`) und
`syntaxa_with_distribution` (die Zahl der Verbände mit mindestens einer
Coverage-Zeile, am Referenzlauf 2026-09-21 **1114** von 1326), gehört zur
Syntaxa-Verbreitung und wird **nicht** in `area_scheme`/`areas_with_data`
mitgezählt — beide Schemata bleiben getrennt gemessen, weil zwischen ihnen
keine Abbildung existiert (siehe unten, `GET /v1/areas`).

Wozu die Selbstauskunft taugt und wozu nicht: sie sagt, worauf dieser Index
gebaut ist — auf der **Batch**-Route beantwortbar sind aber nur `wcvp:`-IDs,
denn sie prüft gegen ein **fest einkompiliertes** Präfix (siehe unten).
Meldet `concept_backbones` mehr als `wcvp`, heißt das nicht, dass die Route
diese IDs nun beantwortet — und, wie der Referenzlauf oben zeigt, auch nicht
zwingend, dass Index und Binary nicht zusammenpassen: ein Index kann legitim
einzelne Konzepte aus anderen Backbones führen (siehe die Warnung unten).

Nicht enthalten ist die *Fassung* der Backbone (etwa `wcvp 2026-06-15`): der
Ingest schreibt sie heute nicht mit, und eine erfundene Fassung wäre schlimmer
als keine.

Die Zahlen werden **je Anfrage** ermittelt (zwei `SELECT DISTINCT`, siehe
„Bekannte Grenze: Leseverhalten"), nicht zwischengespeichert — eine gemessene
Zahl, die aus einem Cache stammt, wäre keine gemessene Zahl mehr.

Fällt eine der beiden Abfragen aus, antwortet die Route **500**
(`INTERNAL_ERROR`) und nicht ein mit Nullen gefülltes `index`-Objekt: das läse
sich für einen Client wie „leerer Index" oder „falsches Backbone".

## Typologien auflisten: `GET /v1/typologies`

Jede Habitattyp-Route adressiert über `(typology, code)` — aber ohne diese
Route kennt ein Client nur die Typologie-ID, die er schon geraten oder aus der
Doku abgeschrieben hat. `GET /v1/typologies` listet alle, **nach `id`
sortiert**, damit dieselbe Anfrage stabil dieselbe Reihenfolge liefert:

```json
[
  { "id": "annex1", "scheme": "annex1", "version": "92/43/EEC",
    "name": "Habitats Directive Annex I",
    "source_ref": "https://doi.org/10.2909/...", "habitat_types": 205 },
  { "id": "eunis@2012", "scheme": "eunis", "version": "2012",
    "name": "EUNIS 2012", "source_ref": "...", "habitat_types": 3783 },
  { "id": "eunis@2021", "scheme": "eunis", "version": "2021",
    "name": "EUNIS 2021", "source_ref": "...", "habitat_types": 3949 }
]
```

Die Zahlen sind der gepinnte Datenstand: 205 + 3783 + 3949 = 7937, die
Gesamtzahl der Habitattypen im Index.

`habitat_types` ist wie jede Zahl in `/v1/info` **am Index gemessen**, nie
konfiguriert. Eine `0` wäre eine ehrliche Aussage — die Typologie ist
registriert, aber (noch) nicht gefüllt —, kein Fehler. Die Antwort ist immer ein Array, auch wenn
der Index keine einzige Typologie führt (`[]`, nie `null`). Es gibt keinen
Filter und keine Parameter: wer eine einzelne Typologie will, kennt danach
ihre ID und fragt `GET /v1/habitat-type/{typology}/{code}`.

## Gebiete auflisten: `GET /v1/areas`

`?scheme=` wählt zwischen den **zwei** Gebietsschemata dieses Index — Vorgabe
`wgsrpd_l3` (die Artverbreitung, WGSRPD Level 3), daneben `evc_territory` (die
136 Territorien der Syntaxa-Verbreitung). Ein unbekannter Wert ist
`INVALID_QUERY`. Es gibt **keine Abbildung zwischen den Schemata**: ein
WGSRPD-Code lässt sich nicht in einen `evc_territory`-Code übersetzen und
umgekehrt — beide sind eigene, unverbundene Vokabulare, und die Route
beantwortet nur das eine, das gerade angefragt wurde.

Der Einstiegspunkt für `?area=`: die Gebiete, zu denen **dieser** Index
Verbreitungsdaten hat, jedes mit seinem englischen WGSRPD-Namen, **nach
`code` sortiert**:

```json
[
  { "scheme": "wgsrpd_l3", "code": "AUT", "name": "Austria" },
  { "scheme": "wgsrpd_l3", "code": "GER", "name": "Germany" }
]
```

Bewusst **nur Gebiete mit Daten**: ein Gebiet ohne Verbreitungszeilen wäre ein
Filter, der nur leer antworten kann, und genau diese Codes beantwortet `?area=`
mit `INVALID_QUERY`. Umgekehrt bleibt ein Code **mit** Daten in der Liste, auch
wenn zu ihm kein Name eingelesen wurde — dann ist `name` leer. Der Code ist die
Identität, der Name nur ein Overlay darauf; den Code als Ersatznamen
auszugeben, würde einen Namen behaupten, den niemand eingelesen hat.

Die Namen stammen aus der WGSRPD-Tabelle selbst (`pipelines/wgsrpd`, an einen
Commit gepinnt), nicht aus einem anderen Dienst: sie werden beim `ingest` aus
einer lokalen CSV gelesen. Ohne diese CSV läuft der Ingest normal durch, die
Codes bleiben dann nur namenlos.

Die Antwort ist immer ein Array, auch wenn der Index keine Verbreitungsdaten
führt (`[]`, nie `null`). Es gibt keinen Filter und keine Parameter.

## Beschreibung eines Habitattyps

`GET /v1/habitat-type/{typology}/{code}` trägt die Beschreibung des Typs, und
zwar **mit ihrer Herkunft**:

```json
"description": {
  "value": "Hay meadows of lowland and montane areas …",
  "provenance": "official",
  "source": "floraveg:eunis-habitat-factsheets:2021-06-01"
},
"description_de": {
  "value": "Mähwiesen der Tief- und mittleren Lagen …",
  "provenance": "situs",
  "source": "situs@0.7.0"
}
```

Zwei Herkünfte kommen vor, und sie sind nicht gleichwertig:

| `provenance` | Bedeutung |
|---|---|
| `official` | Der Wortlaut einer externen, zitierbaren Quelle, unverändert übernommen: die EUNIS-ESy-Factsheets |
| `situs` | situs hat den Text aus den unter `source` genannten Quellen **verfasst**; für diesen Wortlaut steht keine Behörde ein |

Die Anhang-I-Beschreibungen tragen `situs` mit
`source: situs:derived-from:eur28+eunis@2021`: sie entstehen aus dem amtlichen
Interpretationshandbuch EUR 28, der über den Crosswalk zugeordneten
EUNIS-Beschreibung und den Artendaten des Index. Der amtliche Wortlaut selbst
wird nicht ausgeliefert; er ist eine Abgrenzungsvorschrift für die
Rechtsanwendung und nicht die Beschreibung, die im Gelände weiterhilft.

`description_de` erscheint nur bei `?lang=de` und ist ein Overlay wie
`name_de`: der englische Text bleibt die Identität.

**Fehlt das Feld, gibt es keine Beschreibung.** Das ist der Normalfall: die
Factsheets beschreiben EUNIS-**Level 3**, der Index führt acht Level, und
beschrieben sind damit 264 von 7937 Typen. Eine Beschreibung wird **nicht**
nach unten vererbt: ein Untertyp ist nicht das, was sein Obertyp ist, und ein
geerbter Text würde genau das behaupten. Ein leeres Feld gibt es auch nicht —
absent heißt absent.

Einen eigenen Endpunkt dafür gibt es bewusst nicht: wer den Habitattyp
abfragt, will seine Beschreibung im selben Aufruf.

## Pflanzengesellschaften (`SyntaxonRef`)

**Der Identifikator hat mit der Quellenumkehr gewechselt.** `syntaxon.id`
folgt jetzt dem EVC-Primärcode (`AA01A`), nicht mehr dem EEA-EUNIS-Code
(`PAP-01A`). Rund 1310 vormals gültige EEA-Codes stehen nicht mehr in `id`,
sondern im Feld `eea_code`.

**Beide Syntaxon-Routen nehmen die alte ID trotzdem weiterhin an.** Findet
sich zum übergebenen Pfadsegment keine `id`, suchen sowohl
`GET /v1/syntaxon/{id}` als auch `GET /v1/syntaxon/{id}/habitat-types`
zusätzlich über `eea_code` — anders als zunächst dokumentiert bleibt eine
gespeicherte alte EEA-ID also nutzbar, ohne dass ein Client sie selbst
nachschlagen müsste. `GET /v1/syntaxon/{id}/habitat-types` sucht dabei die
Habitattyp-Kanten der über `eea_code` **aufgelösten** ID, nicht der alten —
wer die Kanten unter der ehemaligen `PAP-01A` abfragt, bekommt dieselbe
Antwort wie unter der aktuellen `AA01A`. Eine ID-Zeile hat immer Vorrang, ohne
dass es dafür eine Vorrangregel bräuchte. Der Fallback ist eindeutig (gemessen:
kein `eea_code` kollidiert mit einer fremden `id`, kein `eea_code` ist doppelt
vergeben) — und das bleibt er auch dann, wenn ein Index diese Eigenschaft
verletzt: Der Ingest bricht bei doppeltem `eea_code` ab, und trägt ein anders
gebauter oder älterer Index den Code doch doppelt, antwortet die Route
`INTERNAL_ERROR` statt eines geratenen Syntaxons. Der leere Code trifft nie.

Jede Syntaxon-Referenz — im `syntaxa`-Feld von `GET /v1/habitat-type/{typology}/{code}`
ebenso wie in der Antwort von `GET /v1/syntaxon/{id}/habitat-types` — trägt seit
der Syntaxa-Quellenumkehr (FloraVeg.EU als Primärquelle) vier zusätzliche
Felder:

```json
{
  "id": "CA01A",
  "rank": "alliance",
  "name": "Arrhenatherion",
  "author": "Koch 1926",
  "parent_id": "CA01",
  "eea_code": "TST-01A",
  "source": "evc",
  "parent_provenance": "official",
  "life_form_group": "phanerogam"
}
```

| Feld | Bedeutung |
|---|---|
| `eea_code` | der EEA-EUNIS-Code desselben Syntaxons; fehlt bei Formationen und bei Einheiten, die nur eine der beiden Quellen kennt |
| `source` | welche Quelle diese Zeile führt: `evc` (EuroVegChecklist) oder `eunis` (nur die EEA-EUNIS-Zuordnung kennt sie) |
| `parent_provenance` | ob `parent_id` aus der Quelle selbst stammt (`official`) oder aus dem Geschwisterkonsens abgeleitet wurde (`derived`) |
| `life_form_group` | die Lebensform-Gruppe (`phanerogam`, `bryophyte_lichen`, `algae`); nur auf Formationszeilen gesetzt |

`rank` führt jetzt vier statt zwei Werte: `formation` (die Wurzel — die 25
EuroVegChecklist-Sektionen A–Y) und `class` sind neu hinzugekommen, `order`
und `alliance` gab es schon. Jede Nicht-Formations-Zeile hat einen `parent_id`,
der in höchstens drei Schritten eine Formation erreicht — der Ingest bricht
ab, wenn eine Zeile das nicht schafft (siehe `CLAUDE.md`, Invariants).

### Die Hierarchie durchlaufen: `GET /v1/syntaxa` und `GET /v1/syntaxon/{id}`

`GET /v1/syntaxa?rank=&life_form_group=` ist der Einstiegspunkt: ohne
Parameter sind es die 25 Formationen, nicht alle 1882 Zeilen. `?rank=` fragt
einen anderen Rang ab (`alliance`, `class`, `formation`, `order` — die
tatsächlich erlaubten Werte liest der Dienst aus dem Index, nicht aus einer
fest verdrahteten Liste); `?life_form_group=` filtert zusätzlich über die
Formation, zu der ein Syntaxon gehört (feste Wertemenge, s.o.). Ein
unbekannter Wert bei beiden Parametern ist `INVALID_QUERY`, und die Meldung
nennt die erlaubten Werte. Die Antwort trägt `SyntaxonRef`.

`name_de` steht **nur in diesen beiden Navigationsrouten**, nicht an den
Syntaxon-Referenzen im `syntaxa`-Feld von `GET /v1/habitat-type/{typology}/{code}`
und nicht in den Artenantworten. Das ist ohne Wirkung: dort stehen ausschließlich
Verbände und eine Ordnung, und übersetzt sind nur Formationen (gemessen, siehe
`measured-index.md`).

`?lang=de` legt `name_de` additiv auf jede Referenz — Wert, Provenienz, Quelle
und, wo es einen gibt, den gebräuchlichen deutschen Ausdruck (`vernacular`).
`name` bleibt dabei die Identität und trägt weiter den wissenschaftlichen
Namen. Übersetzt sind die **25 Formationen**; Klassen, Ordnungen und Verbände
tragen `name_de` nicht. Das ist kein Übersetzungsrückstand, sondern eine
Entscheidung: ein Verband, der das Label seiner Formation erbte, hieße falsch,
statt hilfsweise zu heißen — dieselbe Haltung wie bei den abgeleiteten
deutschen Habitattyp-Namen, die nur aus `=` entstehen. Die Namen stammen von
situs selbst (`data/localizations-de-syntaxa.csv`), deshalb `provenance:
situs` — die schwächste Behauptung im Vokabular, und die einzige, die nie eine
Ableitung anstoßen darf.

`?area=` filtert zusätzlich auf Vorkommen in einem Territorium des Schemas
`evc_territory` (Liste: `GET /v1/areas?scheme=evc_territory`). `?include=`
steuert dabei, welche Vorkommens-Werte als Treffer zählen — eine kommagetrennte
Menge aus `verified` und `uncertain`, Vorgabe `verified`. Syntaxa, über die die
Quelle **nichts** sagt (kein Coverage-Eintrag), bleiben in der Liste — dieselbe
Regel wie bei `only_in_area` auf der Arten-Seite: eine Liste, die wegwirft, was
sie nicht beurteilen kann, ist unehrlich sauber. Ein solcher Treffer trägt
kein `occurrence`-Feld; ein Treffer, der auf `?include=` passt, trägt
`occurrence: verified` oder `occurrence: uncertain`. Ein Syntaxon, das die
Quelle geprüft hat, aber ohne die gefragte Ausprägung, fällt aus der Liste —
das ist eine definitive Aussage, keine Unwissenheit.

Sechs Kombinationen sind `INVALID_QUERY`: `?include=` ohne `?area=` (wäre
wirkungslos), ein unbekannter oder leerer Wert in `?include=`, ein doppelt
angegebener `?include=`-Parameter, `?area=` mit einem Rang ohne Verbreitungsdaten
— das sind `formation` (der Vorgabe-Rang), `class` und `order`, denn
`syntaxon_distribution` und `syntaxon_distribution_coverage` tragen
ausschließlich `alliance`-Zeilen — und ein `?area=`-Code, den das Schema
nicht kennt (nie eine leere Liste von „kommt nicht vor").

`GET /v1/syntaxon/{id}` liefert ein einzelnes Syntaxon als `SyntaxonDetail`
— `SyntaxonRef` eingebettet (die Felder liegen also flach im selben
JSON-Objekt) plus `ancestors` (Weg zur Wurzel, äußerste zuerst), `children`
(direkte Kinder, nach `id` sortiert) und `direct_habitat_type_count` (nur die
Habitattypen, die genau dieses Syntaxon verlinken, nicht seine Nachkommen).
`ancestors` und `children` stehen immer im JSON, auch leer — eine Formation
ohne Ahnen und ein Verband ohne Kinder sind der Normalfall, kein Fehler. Eine
unbekannte oder leere `{id}` ist `NOT_FOUND`, nie `INVALID_QUERY`: ein
Pfadsegment ist keine Query. Eine baumelnde `parent_id` — der Index verweist
auf eine Zeile, die es nicht gibt — ist `INTERNAL_ERROR`, kein `404`: das ist
ein Indexdefekt, nicht eine unbekannte Anfrage. `?lang=de` legt `name_de` auf
die Einheit selbst, auf **jeden Ahnen** und auf **jedes Kind**. Die Ahnen sind
der Grund dafür: sie sind die Brotkrumenzeile, und eine deutsche Zeile, die in
einer englischen Wurzel endet, wäre halb übersetzt. Das kostet eine einzige
zusätzliche Abfrage für die ganze Antwort, nicht eine je Referenz.

`SyntaxonDetail` trägt außerdem `distribution` — die Verbreitungsaussage der
Quelle (Preislerová et al., Schema `evc_territory`) mit sortierten
`verified`- und `uncertain`-Codelisten. Das Feld hat `omitempty` und **fehlt
ganz**, wenn für dieses Syntaxon keine Coverage-Zeile vorliegt: dann ist
nichts über sein Vorkommen bekannt. Zwei leere Listen dagegen bedeuten
„geprüft, kommt in keinem Territorium vor" — eine andere Aussage als das
fehlende Feld. Gemessen fehlt `distribution` bei 212 von 1326 Verbänden,
darunter jedem Moos-, Flechten- und Algenverband, weil die Quelle nur
Gefäßpflanzen-Vegetation abdeckt. Abwesenheit wird nie aufgezählt: der Client
kennt die 136 Codes des Schemas aus `GET /v1/areas?scheme=evc_territory`.

## Warum ein Habitattyp mehrfach in einer Liste steht

Die Listen der Endpunkte unten sind Listen von **(Habitattyp, Rolle)-Paaren**,
nicht von Habitattypen. Derselbe Habitattyp erscheint deshalb mehrfach, wenn
die Art in ihm mehrere Rollen spielt — und das ist der Normalfall, nicht die
Ausnahme: **3489 von 10470** (Habitattyp, Art)-Paaren tragen mehr als eine
Rolle, ein Drittel also.

Das sind **keine Duplikate**. Jede Rolle trägt ihre eigene Kennzahl, und
zusammen sind sie drei verschiedene Aussagen über dieselbe Art im selben
Habitattyp. Für *Fagus sylvatica* in `eunis@2021/T17` gemessen:

| Rolle | Kennzahl | Aussage |
|---|---|---|
| `diagnostic` | `fidelity` 31.0 | wie kennzeichnend die Art für den Typ ist |
| `constant` | `constancy` 99.0 | in wie vielen Aufnahmen sie vorkommt |
| `dominant` | `constancy` 98.0 | in wie vielen sie bestandsbildend ist |

Eine Zeile wegzulassen hieße, eine dieser Aussagen zu verlieren; sie zu einer
zusammenzufassen hieße, zwei verschiedene Kennzahlen in ein Feld zu zwingen.
`(typology, code, role)` ist eindeutig — echte Duplikate gibt es nicht.

**Betroffen sind die flachen Listen**, und zwar in beide Richtungen:

| Route | erscheint mehrfach | gemessen (Beispiel) |
|---|---|---|
| `GET /v1/species/{conceptId}/habitat-types` | der Habitattyp | 34 Einträge, 22 Habitattypen |
| `POST /v1/species/habitat-types` | der Habitattyp | ebenso |
| `GET /v1/habitat-type/{typology}/{code}/species` | die **Art** | 77 Einträge, 65 Arten |

**Nicht betroffen** ist `GET /v1/habitat-type/{typology}/{code}`: dort ist
`species` ein Objekt, das schon nach Rolle gruppiert
(`{"diagnostic": […], "constant": […], "dominant": […]}`). Ebenso wenig
`GET /v1/syntaxon/{id}/habitat-types` — dort spielen Rollen keine Rolle.

Dass derselbe Sachverhalt einmal gruppiert und einmal flach ausgeliefert wird,
ist eine bewusst stehengelassene Ungleichheit: die flache Form ist der
Vertrag, den die bestehenden Aufrufer kennen, und `?role=` filtert sie bereits
auf eine Rolle, wenn man genau eine will. Wer eine Liste **eindeutiger**
Habitattypen braucht, faltet clientseitig über `(typology, code)` — die
Antwort rechnet bewusst nichts vor.

Alle Zahlen mit ihrer Abfrage: `docs/reference/measured-index.md`.

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
Batch-Anfrage eine zusätzliche Abfrage kosten. Die Folge, die man kennen muss:
gegen einen Index, der **nicht** auf wcvp gebaut ist, meldet `/v1/info` treu
dessen Backbone, während jede seiner IDs hier `unknown_backbone` bekommt. Die
Selbstauskunft macht diesen Bruch sichtbar; sie verschiebt die Prüfung nicht.

!!! warning "Gemischte Backbones kommen real vor — und `unknown_backbone` ist für sie unwahr"

    Dieser Entwurf ging davon aus, dass ein Index genau einen Backbone führt.
    Der Ingest-Lauf vom 2026-09-16 widerlegt das: von 3323 Konzepten lauten
    **8 nicht auf `wcvp:`** (7 × `eurosl:`, 1 × `cdm:`) — Sammelarten und
    Sektionen, für die WCVP kein Konzept führt und die
    `hostus export-crosswalk` deshalb auf ein EuroSL-/CDM-Konzept abbildet.
    Der Ingest warnt darüber am Laufende.

    Für diese 8 IDs antwortet der Batch-Endpunkt `unknown_backbone`, obwohl der
    Index ihre Fakten besitzt und `GET /v1/species/{conceptId}/habitat-types`
    sie normal beantwortet. Die Schuldzuweisung „Fehler des Aufrufers" trifft
    hier nicht zu. Was daraus folgt, ist offen und wird in
    [#36](https://github.com/jobrunner/situs/issues/36) verhandelt; Zahlen und
    die vollständige Liste stehen in `measured-index.md`.

## Grenzen des Batch-Endpunkts

`POST /v1/species/habitat-types` ist zweifach begrenzt: der Body auf **1 MiB**
und das `concept_ids`-Array auf **300 Einträge** (`maxItems` in der
Spezifikation). Beides ergibt `INVALID_QUERY` (400). Die Array-Grenze ist nötig,
weil die Byte-Grenze sie nicht impliziert: 1 MiB kurzer IDs sind Zehntausende
Einträge, und jede verschiedene kostet eine Handvoll Index-Abfragen. 300 liegt
weit über jeder realistischen Geländeaufnahme. `POST /v1/species/traits/summary`
teilt dieselben beiden Grenzen — beide Routen dekodieren denselben Body und
denselben `batchRequest`.

Doppelte IDs sind erlaubt. Die Index-Arbeit wird intern dedupliziert, die Antwort
trägt aber **einen Eintrag je Eingabe-ID in Eingabereihenfolge**, sodass
`response[i]` zu `concept_ids[i]` gehört. Ein leerer bzw. nur aus Whitespace
bestehender Eintrag wird **abgewiesen**, nicht übersprungen: übersprungen würde
er die Liste eines vertrauenden Clients um eins verschieben, und
`unknown_backbone` wäre gelogen, weil ein leerer String keine andere Backbone
ist. Ein leeres `concept_ids` ist ebenfalls `INVALID_QUERY`. Für die
Zeigerwertanalyse gilt dieselbe Ablehnung leerer Einträge; die
Index-Reihenfolge-Garantie gilt dort nicht, weil ihre Antwort keine Liste je
Eingabe ist, sondern je Vokabular/Dimension aggregiert.

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

**Welche Codes gültig sind, sagt `GET /v1/areas`** (siehe oben).
`areas_with_data` in `/v1/info` nennt weiterhin nur ihre *Anzahl* — daraus
lässt sich ablesen, ob überhaupt ein Verbreitungs-Ingest gelaufen ist (`0`
heißt nein), nicht aber, ob `GER` dabei ist.

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

## CORS (optional, standardmäßig aus)

Ohne `SITUS_SERVER_CORS_ALLOWED_ORIGINS` sendet situs **keinerlei**
CORS-Header — der Dienst verhält sich byte-identisch wie ohne diese
Funktion. Das ist der korrekte Default: der eingebaute Explorer unter `/`
wird vom selben Origin ausgeliefert, den er abfragt, und braucht kein CORS.

Gesetzt wird eine kommagetrennte Liste erlaubter Origins:

```bash
export SITUS_SERVER_CORS_ALLOWED_ORIGINS='http://localhost:5173,https://*.fieldworksdiary.app'
```

Ein Eintrag ist entweder ein exakter Origin (`http://localhost:5173`) oder ein
Subdomain-Platzhalter (`https://*.fieldworksdiary.app`), bei dem nur das
führende Host-Label durch `*` ersetzt wird. **Schema und Port müssen auch beim
Platzhalter exakt passen** — `https://*.fieldworksdiary.app` deckt
`https://app.fieldworksdiary.app`, aber weder `http://app.fieldworksdiary.app`
(anderes Schema) noch `https://app.fieldworksdiary.app:8443` (anderer Port).
Leerraum um einen Eintrag ist erlaubt und wird vor dem Parsen entfernt —
`'http://localhost:5173, https://*.fieldworksdiary.app'` (Leerzeichen nach dem
Komma, die naheliegende Schreibweise) ergibt dieselben zwei Einträge wie ohne
Leerzeichen. Ein Eintrag, der nach dem Trimmen leer ist (ein doppeltes Komma,
ein Komma am Ende), gilt **nicht** als brauchbar und fällt unter dieselbe
Warnung wie jeder andere kaputte Eintrag — er verschwindet nicht kommentarlos.

Ein Eintrag ohne Schema, mit ungültigem Schema (z. B. `ht/tps://` oder ein
Schema mit eingebettetem Leerzeichen), mit Pfad, mit Query (`?...`) oder
Fragment (`#...`), mit einem Port außerhalb 1–65535 oder in nicht-kanonischer
Form (`:0443`, `:+443` — ein Browser schickt nie eine führende Null oder ein
`+`) oder ein bloßes `*` bricht den Start **nicht** ab. Er wird beim Start
verworfen, mit einer Warnung, die den fehlerhaften Eintrag und die nötige
Korrektur nennt (`ignoring unusable CORS origin pattern`) — der Dienst läuft
mit den übrigen, brauchbaren Einträgen weiter. Sind **alle** konfigurierten
Einträge unbrauchbar, kommt zusätzlich eine Fehlermeldung (`CORS was
configured but every allowed-origin entry was unusable — CORS stays
disabled`), weil CORS dann trotz Konfiguration vollständig wirkungslos
bleibt. Der Prozess startet in beiden Fällen.

Schema und Host werden beim Parsen auf Kleinschreibung normalisiert — beide
sind laut Spezifikation unabhängig von der Schreibweise gleich. Ein Eintrag
wie `HTTPS://Example.COM` trifft also dieselbe Herkunft wie
`https://example.com`. Anders als Schema und Host wird der Port nicht auf
Kleinschreibung normalisiert, da er ohnehin nur aus Ziffern besteht — das gilt
unabhängig von der Vorgabe-Port-Normalisierung im nächsten Absatz.

Ein Vorgabe-Port wird beim Parsen dagegen sehr wohl normalisiert — weg
genommen, nicht nur umgeschrieben: `https://example.com:443` und
`https://example.com` (ebenso `http://example.com:80` und
`http://example.com`) bezeichnen dieselbe Herkunft, weil ein Browser den
Vorgabe-Port im `Origin`-Header nie mitschickt. Das gilt **nur** für das
schema-eigene Vorgabe-Paar — `https://example.com:80` bleibt eine andere
Herkunft als `https://example.com`, weil `:80` der Vorgabe-Port des
`http`-Schemas ist, nicht des `https`-Schemas.

Es gibt **kein** `Access-Control-Allow-Credentials`: situs kennt keine
Anmeldung, also gibt es keine Sitzung, die mitgeschickt werden müsste.

`Access-Control-Allow-Methods` wird bei jedem Preflight aus der tatsächlichen
Routing-Tabelle beantwortet, nie aus einer festen Liste — ein neuer Endpunkt
ist damit automatisch abgedeckt.

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

`GET /v1/syntaxon/{id}` braucht bis zu vier Primärschlüsselabfragen für den
Ahnenpfad (eine je Ebene, maximal `maxSyntaxonAncestors` plus die Zeile selbst)
und liest die Kinder über `idx_syntaxon_parent` — kein Full-Table-Scan in
keinem der beiden Fälle.
