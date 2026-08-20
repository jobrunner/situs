# Machbarkeitsstudie: Verbreitungsfilter (LRT, Syntaxa, Pflanzen) pro Land

Stand: 2026-08-20. Ausgelöst von der Beobachtung, dass
`POST /v1/species/habitat-types` bei mehreren Arten unbrauchbar viele Treffer
liefert, und der Frage, ob Verbreitungsdaten pro EU-Land das eingrenzen.

Alle Zahlen unten sind **an der laufenden Installation gemessen** (situs mit dem
vollständigen Ingest, hostus mit seinem echten Index), nicht geschätzt. Wo
gerechnet statt gemessen wurde, steht es dabei.

## Ergebnis in drei Sätzen

Die Artverbreitung ist **schon da** (in hostus) und taugt **nicht**, um
Habitattypen einzugrenzen — wohl aber, und dort ist sie unverzichtbar, um die
*Artenlisten* regional korrekt zu machen: bis zu **50 % unmögliche Arten** in
Typen, die Deutschland am Rand ihres Areals erreichen. Habitattypen grenzen
Article 17 (Anhang I) und die Syntaxa-Verbreitung ein; beide Quellen sind frei
und pinnbar. Den größten Effekt auf die *Trefferzahl* liefert etwas ohne jede
neue Quelle: das **Ko-Vorkommen** mehrerer Feldarten (gemessen 60 → 7).

## Das Ausgangsproblem, quantifiziert

| Eingabe | Antwortgröße | Habitattyp-Einträge |
|---|---|---|
| 1 Art | 18,0 KiB | 34 |
| 3 Arten | 57,9 KiB | 115 |
| 6 Arten | 76,3 KiB | 157 |

Die Route liefert je Eingabename eine **eigene** Liste. Was ein Anwender im Feld
braucht, ist die Schnittmenge — und die ist drastisch kleiner (siehe unten).

## Achse 1: Anhang-I-Typen (LRT) pro Mitgliedsstaat

**Quelle:** [Article-17-Berichterstattung der EEA](https://www.eea.europa.eu/en/datahub/datahubitem-view/d8b47719-9213-485a-845b-db1bfe93598d)
(Habitatrichtlinie 92/43/EWG). Tabellarisch je Mitgliedsstaat **und
biogeografischer Region**: Fläche, Trends, Erhaltungszustand. Formate u. a. CSV;
zusätzlich ein räumlicher Datensatz mit 10-km-Rasterzellen.

**Verfügbarkeit:** frei, EEA-Datenpolitik wie die schon gepinnten EUNIS-Dateien.
Passt in `pipelines/eunis/manifest.yaml` ohne neue Mechanik.

**Erwartete Wirkung:** hoch und **direkt** — situs kennt 184 EUNIS-Typen mit
Anhang-I-Bezug. Ein Filter „in DE gemeldet" wirkt auf den Typ selbst, nicht auf
Umwege über Arten.

**Die Berichtseinheit richtig verstehen — hier ist ein Irrtum leicht:**
Article 17 berichtet je **Mitgliedsstaat × biogeografischer Region**. Die beiden
Achsen sind **nicht ineinander geschachtelt, sie kreuzen sich**:

- Es gibt **neun terrestrische Regionen für die gesamte EU** (alpin, atlantisch,
  Schwarzes Meer, boreal, kontinental, makaronesisch, mediterran, pannonisch,
  steppisch) bei 27 Mitgliedsstaaten. Eine Region ist damit im Regelfall **viel
  gröber als ein Land** — die kontinentale Region allein reicht über etwa ein
  Dutzend Staaten.
- Dass Deutschland in drei Regionen zerfällt, ist eine Eigenschaft Deutschlands,
  **keine Regel**. Aus einem Land folgt keine Region und aus einer Region kein
  Land.
- Nutzbar ist deshalb nur das **Paar** `(Staat, Region)` — und *das* ist feiner
  als das Land. Genau so berichtet Article 17.

**Folge für den Filter:** „Kommt LRT X in DE vor?" ist die Vereinigung über alle
DE-Zeilen (DE×atlantisch, DE×kontinental, DE×alpin). Feiner wird es nur, wenn
der Aufrufer die Region **kennt** — aus dem Land ableiten lässt sie sich nicht.
Wer Koordinaten hat, kann sie bestimmen: die EEA veröffentlicht die
[Biogeographical regions](https://www.eea.europa.eu/en/datahub/datahubitem-view/11db8d14-f167-4cd5-9205-95638dfd9618/view)
als Geodatensatz, und Punkt-in-Polygon ist genau das, was **ortus** tut. Für
situs heißt das: `?area=` sollte beides annehmen können — Land **oder** Region —
und nicht das eine aus dem anderen erraten.

**Grenze:** Article 17 deckt **nur Anhang-I-Typen** ab. Für die 7753 EUNIS-Typen
ohne Anhang-I-Bezug sagt es nichts.

## Achse 2: Syntaxa-Verbreitung

**Quelle:** [Distribution maps of vegetation alliances in Europe](https://zenodo.org/records/11580949)
(Zenodo, **CC BY 4.0**, XLSX, 233 kB) — **1115 Verbände × 82 europäische
Gebietseinheiten**. Dazu [FloraVeg.EU](https://floraveg.eu/) mit einem
Download-Bereich, auf dem EuroVegChecklist v3 aufsetzt.

**Verfügbarkeit:** frei und pinnbar, genau wie die ESy-Datei (auch Zenodo, auch
CC BY 4.0). Die Pipeline liest bereits XLSX.

**Erwartete Wirkung:** hoch. situs hat **1050 Syntaxa** und 1283 Verknüpfungen zu
Habitattypen; ein Verband, den es in DE nicht gibt, entwertet den Bezug direkt.

**Das messbare Risiko — und es ist das gleiche wie bei den Artnamen:** die
Verknüpfung läuft über **Verbandsnamen mit Autorenzitat**
(`Carpinion betuli Issler 1931`). 1050 EEA-Syntaxa gegen 1115 Zenodo-Verbände zu
joinen ist Namensabgleich, kein Schlüsselabgleich. Bei den Artnamen lag die
gemessene Quote bei 83,8 % — hier ist sie **unbekannt und muss zuerst gemessen
werden**. Ein Join, der 30 % verliert, filtert falsch statt gar nicht.

**Offen:** wie die „82 Gebietseinheiten" definiert sind (Länder? Regionen?) und
in welcher Spalte der Verband steht — beides ging aus der Zenodo-Seite nicht
hervor und ist die erste Frage eines Spikes.

## Achse 3: Pflanzenverbreitung — schon vorhanden, aber schwächer als erwartet

**Quelle: keine neue nötig.** hostus führt sie bereits:

- `GET /v1/concept/{id}` liefert ein `distribution`-Array
- Schema **WGSRPD Level 3**, 381 Gebiete, exponiert unter `GET /v1/areas`
- `GET /v1/suggest` hat schon einen `area`-Filter mit `in_area`-Flag

**Granularität, geprüft:** feiner als ISO-Länder — `GER`, `FRA`+`COR`,
`SPA`+`BAL`, `ITA`+`SAR`+`SIC`. **Eine Ausnahme:** `CZE` heißt
„Czechia-Slovakia" — Tschechien und Slowakei sind **nicht trennbar**. Ein
Länderfilter braucht also eine Abbildung WGSRPD L3 → ISO, meist n:1, in diesem
Fall 1:n.

**Gemessene Wirkung auf Artenlisten** (Anteil der Arten eines Typs, die es in DE
nicht gibt):

| Typ | Arten (Stichprobe) | nicht in DE |
|---|---|---|
| T38 Mediterraner Zedernwald | 60 | **82 %** |
| T31 Bergfichtenwald | 60 | 0 % |
| R22 Flachland-Mähwiese | 51 | 0 % |
| S42 Trockenheide | 20 | 0 % |

**Gemessene Wirkung auf Habitattypen — und hier kippt die Hypothese.** Für die
22 Habitattypen der Preiselbeere, gefiltert über den Anteil ihrer Arten in DE:

| Typ | Arten in DE | |
|---|---|---|
| Q3 Palsa mires | 69 % | **kommt in DE nicht vor** |
| S11 Shrub tundra | 71 % | **kommt in DE nicht vor** |
| T3H Larix light taiga | 82 % | **kommt in DE nicht vor** |
| S12 Moss and lichen tundra | 84 % | **kommt in DE nicht vor** |
| Q11 Raised bog | 100 % | kommt vor |

**Bei jeder sinnvollen Schwelle bleiben alle 22 Typen stehen.** Palsamoore und
Strauchtundra bestehen aus Arten, die es in Deutschland gibt (Preiselbeere,
Rauschbeere, Moor-Birke, Wollgras) — der *Typ* fehlt hier, seine *Arten* nicht.

**Schlussfolgerung:** Artverbreitung ist ein **untauglicher Stellvertreter** für
Habitattyp-Verbreitung, sobald sich die Artenpools überlappen (boreal ↔
temperat). Sie wirkt nur bei fernen Floren (mediterran: 82 %). Für die
Habitattyp-Eingrenzung braucht es Achse 1 und 2, nicht Achse 3.

### Der eigentliche Nutzen von Achse 3: die Artenliste selbst

Nicht als Filter über Habitattypen, sondern **innerhalb** der Artenliste, die
situs zurückgibt. Der Kern-Use-Case der Spec ist „eine Kennart → welche weiteren
Kennarten lohnt es zu suchen". Enthält diese Vorschlagsliste eine in der Region
unmögliche Art, ist sie schlimmer als unvollständig: der Anwender sucht im Feld
nach etwas, das dort nicht existieren kann.

**Gemessen, Anteil der Arten eines Typs, die es in DE nicht gibt** (vollständige
Listen, nicht Stichproben):

| Typ | Arten | nicht in DE | kommt der Typ in DE vor? |
|---|---|---|---|
| T23 Macaronesian laurophyllous forest | 88 | **88 %** | nein |
| T38 Mediterraner Zedernwald | 182 | **84 %** | nein |
| S72 Eastern Mediterranean phrygana | 85 | **61 %** | nein |
| **R15 Kontinentaler Steppen-Trockenrasen** | 109 | **50 %** | **ja** (fränkisch/thüringisch) |
| R17 Schwermetallrasen des Balkans | 116 | 45 % | nein |
| S34 Balkan-anatolisches Genistoid-Gebüsch | 107 | 40 % | nein |
| S24 Subalpines Genistoid-Gebüsch (Adria) | 86 | 29 % | nein |
| **R31 Kalk-Trockenrasen** | 36 | **22 %** | **ja** |
| T1G Alnus-cordata-Wald | 75 | 20 % | nein |
| T36 Montaner Kiefernwald | 82 | 6 % | ja |
| R23 Berg-Mähwiese | 78 | 1 % | ja |
| T1E, T12, T1F, R21, R22, S42, Q11 | 20–79 | **0 %** | ja |

**Das Muster:** Typen, die in Deutschland nicht vorkommen, haben viel Rauschen —
irrelevant, sobald Achse 1/2 sie ohnehin ausblendet. Typen, die hier vorkommen,
haben meist **0 %**. Das Problem sitzt genau in der Mitte: **Typen mit weitem
europäischem Areal, die Deutschland am Rand erreichen.** R15 mit 50 % ist der
fränkische Steppen-Trockenrasen — und seine Liste enthält iberische Arten wie
*Peucedanum hispanicum*, *Cirsium pyrenaicum*, *Hypericum caprifolium*,
*Carex mairei*. Genau dort ist die Bestimmung ohnehin schwer und die Hilfe am
wertvollsten. Das „Unterfranken sucht einen türkischen Endemiten"-Szenario ist
also nicht hypothetisch, sondern in den artenreichsten Trockenrasen der Regelfall.

### Der blinde Fleck — und warum „keine Verbreitung abrufbar" falsch wäre

Filtern lässt sich zunächst nur, was eine Konzept-ID hat: **11559 von 13791
Zeilen (83,8 %)**. Je Typ schwankt das erheblich — T3H 27 % ohne ID, R23 20 %,
T36 13 %, T12 13 %.

Diese 2232 Zeilen sind aber **kein einheitliches Problem**. Nachgemessen, indem
für jede der 259 betroffenen Gattungen geprüft wurde, ob WCVP sie überhaupt
führt:

| Gruppe | Namen | Zeilen | Ursache | beschaffbar? |
|---|---|---|---|---|
| **Vaskularpflanzen** | 133 Gattungen | **1130** (8,2 %) | Gattung **ist** in WCVP — die Auflösung scheitert an Mehrdeutigkeit, Synonymie und fehlendem Fuzzy-Matching | **ja, in hostus** |
| **Moose/Lebermoose** | ~180 Namen | ~828 (6,0 %) | außerhalb der taxonomischen Reichweite von WCVP (nur Vaskularpflanzen) | **ja, eigene Quelle** |
| **Flechten** | 79 Namen | 274 (2,0 %) | dito | **unklar** |

Die Aufteilung Moose/Flechten ist eine Heuristik über 56 bekannte
Flechtengattungen, keine taxonomische Prüfung.

**Gruppe 1 ist ein hostus-Thema, keine Datenlücke.** Die Ablehnungsgründe, die
hostus selbst nennt: *„Mehrdeutiger Treffer: mehrere Konzepte mit gleicher
Übereinstimmung"* und *„Kein eindeutiger Treffer, keine Fuzzy-Auflösung"*.
Betroffen sind unstrittige Arten wie **Alnus viridis**, *Beckmannia
eruciformis*, *Abies borisii-regis*, *Artemisia lerchiana* — alle in WCVP
vorhanden. `Bellidiastrum michelii` ist ein Synonym (*Aster bellidiastrum*),
`Arctostaphylos alpinus` eine Falschschreibung. Das sind 8,2 % der Fakten, die
an der Namensauflösung hängen, nicht an der Verbreitung. **Hebel liegt in
hostus**, nicht in situs.

**Gruppe 2 hat eine Quelle:** [Checklist and country status of European
bryophytes](https://www.npws.ie/sites/default/files/publications/pdf/IWM123.pdf)
(Hodgetts & Lockhart, aus dem
[annotierten Checklist-Vorhaben](https://www.tandfonline.com/doi/full/10.1080/03736687.2019.1694329)
für Europa, Makaronesien und Zypern) — 1892 Moosarten, Verbreitung **je Land**,
als Excel-Tabellen über NPWS und ECCB herunterladbar. Format und Granularität
passen also zu dem, was situs für Vaskularpflanzen aus hostus bekommt; die
Lizenz ist beim Pinnen zu prüfen.

**Gruppe 3 (Flechten, 2 %)** ist die einzige, für die hier keine
länderscharfe europäische Quelle belegt ist. Nicht recherchiert, nicht behauptet.

**Konsequenz für den Entwurf, unverändert:** drei Zustände, nicht zwei — *kommt
hier vor*, *kommt hier nicht vor*, *unbekannt*. Ein unbekannter Eintrag darf
nicht still durchfallen (dann ist die Liste unehrlich sauber) und nicht still
verschwinden (dann verliert man Fakten). Das ist dieselbe Regel, die situs bei
unauflösbaren Namen schon befolgt: behalten und kennzeichnen, nie verwerfen.
Der Unterschied ist nur, dass „unbekannt" jetzt nicht mehr 16 % dauerhaft
bedeutet, sondern 2 % nach zwei umsetzbaren Schritten.

**Architekturbefund:** hostus limitiert auf **20 Anfragen/s** (Token-Bucket,
`defaultRateLimitPerSecond`). Ein Lookup je Art zur Laufzeit ist damit
ausgeschlossen — die erste Messung lief in 276 × HTTP 429. Die Verbreitung muss
**beim Ingest** übernommen und in situs gespeichert werden, genau wie die
Konzept-IDs heute. Das erhält die Autarkie zur Laufzeit.

## Was tatsächlich am stärksten filtert — ohne jede neue Quelle

Sechs Feldarten (Preiselbeere, Heidekraut, Moor-Birke, Schmalblättriges
Wollgras, Rauschbeere, Sandsegge), ausgezählt gegen den bestehenden Index:

| Kriterium | Habitattypen |
|---|---|
| Vereinigung (**was die API heute liefert**) | **60** |
| ≥ 3 der 6 Arten belegt | 8 |
| ≥ 4 der 6 Arten belegt | **7** |
| ≥ 5 der 6 Arten belegt | **2** |

Die beiden Spitzentreffer: **Q11 Raised bog** und **T3J Pinus and Larix mire
forest**, je 5 von 6 Arten — für diese Artenkombination botanisch stimmig.

**60 → 7 durch reines Auszählen.** Das ist genau das „Scoring/Ranking", das die
Spec als Schnitt S2 vertagt hat, und es braucht nur Daten, die situs schon hat.

## Empfohlene Reihenfolge

1. **Artverbreitung beim Ingest** aus hostus übernehmen (eine Spalte je
   Art × Gebiet). Keine neue Quelle, keine neue Lizenz, kein Join-Risiko — die
   Konzept-IDs, über die es geht, hat situs schon. Damit werden die
   *Artenlisten* regional korrekt, und das ist die Voraussetzung dafür, dass
   Punkt 2 überhaupt etwas Brauchbares vorschlägt.
2. **Ko-Vorkommen-Ranking** (S2). Gemessen der stärkste Effekt auf die
   Trefferzahl (60 → 7), behebt die Beschwerde direkt. Erfordert eine
   Entscheidung über die Antwortform: Schnittmenge statt Liste-je-Art.
3. **Article 17** → Anhang-I-Typen pro Land/Region. Frei, tabellarisch, wirkt
   direkt auf den Typ. Blendet die Hochrausch-Typen aus, für die Punkt 1 sonst
   arbeiten müsste.
4. **Syntaxa-Verbreitung** (Zenodo). Gleiche Wirkungsklasse wie 3, aber **erst
   nach einem Join-Spike**: Trefferquote Verbandsname EEA ↔ Zenodo messen, bevor
   irgendetwas darauf aufbaut.

**Warum 1 vor 2:** das Ko-Vorkommen-Ranking erzeugt Artvorschläge *aus* diesen
Listen. Rangiert man zuerst und filtert später, schlägt man dem Anwender im
schlechtesten Fall genau die unmöglichen Arten am prominentesten vor.

## Offene Fragen für den Spike

- Nimmt `?area=` **Land und Region** an (empfohlen, siehe Achse 1) — und wie
  wird die Antwort gekennzeichnet, wenn nur die gröbere Achse bekannt ist? Ein
  Treffer „in DE gemeldet" ist eine schwächere Aussage als „in DE×kontinental
  gemeldet", und das darf ein Client nicht verwechseln.
- Trefferquote des Syntaxa-Namensjoins (EEA ↔ Zenodo). **Vor** dem Bau messen.
- Definition der 82 Gebietseinheiten im Zenodo-Datensatz.
- Abbildung WGSRPD L3 → ISO-3166, inklusive der Entscheidung zu `CZE`.
- Was gilt für die 7753 EUNIS-Typen **ohne** Anhang-I-Bezug? Article 17 schweigt
  dazu; die Syntaxa-Achse ist dort die einzige Verbreitungsinformation.
