# Gemessene Kennzahlen des Index

Alle Zahlen auf dieser Seite sind **am gepinnten Datenstand gemessen**, nicht
geschätzt und nicht aus der Spec übernommen. Sie beantworten die offenen Punkte
1, 2, 3 und 5 der Design-Spec
(`docs/superpowers/specs/2026-08-18-situs-foundation-design.md`).

!!! info "Referenzlauf: 2026-09-16, situs 0.7.0"

    Die Zahlen zu Artenrollen, Verbreitung, Gebietsnamen, Zeigerwerten und
    Labels stammen aus **einem** vollständigen `situs ingest`-Lauf am
    2026-09-16 gegen situs 0.7.0 (Dauer **5:46**), mit allen Pipeline-CSVs im
    selben `--csv-dir` und einem lokalen hostus mit den Backbone-Fassungen
    **wcvp 2026-06-15**, cdm 2026-08-02, eurosl 2024-11-03, germansl 1.5.6 —
    denselben, die `https://hostus.fieldworksdiary.org` führt. Die
    Namensauflösung lief dabei erstmals über den
    `hostus export-crosswalk`-Export, nicht mehr über `/v1/match`; die früher
    hier stehenden Zahlen des August-Laufs sind damit überholt und ersetzt.

    Die **4307** Crosswalk-Zeilen sind mit diesem Lauf bestätigt (vorher
    vorhergesagt: 4305 ingestiert + 2 übersprungene `≈`-Zeilen). Die
    `=`-Quote bleibt 29, weil `≈` nach `IsSame()` nicht als volle Entsprechung
    zählt.

Reproduzierbar mit der Pipeline (Artefakte gepinnt in
`pipelines/eunis/manifest.yaml`):

```bash
cd pipelines/eunis
python3 xlsx_to_csv.py \
  --eunis-xlsx artifacts/eunis-2021-including-crosswalks.xlsx \
  --annex1-xlsx artifacts/eunis-2021-annex1-separate-rows.xlsx \
  --esy-xlsx artifacts/esy-characteristic-species-combinations.xlsx \
  --out-dir out
```

Das schreibt `out/report.json` mit den Messungen unten. `out/` ist
gitignoriert — deshalb diese Seite. Die Auflösungsquoten entstehen erst beim
Go-Ingest (`situs ingest`, siehe `../how-to/ingest.md`): die der Artenrollen
gegen die lokale `eurosl_crosswalk.csv`, die der Zeigerwerte und die
Verbreitung gegen einen laufenden hostus.

## Umfang des Index

| Größe | Gemessen |
|---|---|
| Habitattypen (über `eunis@2021` + `eunis@2012` + `annex1`) | **7937** |
| Maximales Level der eunis@2021-Klassifikationshierarchie | **8** |
| Crosswalk-Zeilen (Versions- **und** Anhang-I-Crosswalk) | **4307** |
| Syntaxa (`syntaxon`, nach Rang) | 25 Formationen, 150 Klassen, 381 Ordnungen, **1326** Verbände (1882 gesamt) |
| Habitattyp↔Syntaxon-Verknüpfungen | **1282** |
| Zeilen in `species_roles.csv` | **13791** |

Die Crosswalks sind ausschließlich auf **Level 3** befüllt, obwohl die
Klassifikationshierarchie bis Level 8 reicht. Das ist die bekannte, bewusst
akzeptierte Deckelung des Fundaments (siehe `CLAUDE.md`, „Known ceiling"), kein
Datenfehler.

## Qualifier (offener Punkt 2)

Tatsächlich vorkommende Symbole: **`=` `<` `>` `#` `≈`** — fünf, nicht vier.
Histogramm über die Anhang-I-Crosswalk-Zeilen (339 Zeilen):

| Qualifier | Zeilen |
|---|---|
| `#` | 137 |
| `>` | 110 |
| `<` | 61 |
| `=` | 29 |
| `≈` | 2 |

`≈` war in der Spec nicht vorgesehen, steht aber für echte
Naturschutz-Entsprechungen (`eunis@2021:R1S → annex1:6130`,
`eunis@2021:U28 → annex1:8130`) und ist deshalb als fünfter Qualifier
aufgenommen, statt seine Zeilen zu verwerfen.

## Syntaxa-Tiefe (offener Punkt 1)

!!! info "Referenzlauf: 2026-09-21, Syntaxa-Quellenumkehr"

    Die Zahlen dieses Abschnitts stammen aus einem vollständigen
    `situs ingest`-Lauf am 2026-09-21 gegen die realen EEA-/FloraVeg-Artefakte
    (Task 9 des Plans `2026-09-21-syntaxa-quellenumkehr`), **nicht** mehr aus
    dem Referenzlauf vom 2026-09-16. FloraVeg.EU (die EuroVegChecklist) ist
    seither die **Primärquelle** der Syntaxa-Hierarchie; die EEA-EUNIS-Zuordnung
    liefert nur noch die Habitattyp-Kanten und die Verbände, die FloraVeg nicht
    führt. Die Auflösungsquoten in „Namensauflösung", „Verbreitung" und
    „Zeigerwerte" unten sind von dieser Umkehrung unberührt und bleiben am
    2026-09-16-Lauf gemessen.

Vorkommende Ränge: **`formation`** (die Wurzel — die 25
EuroVegChecklist-Sektionen A–Y), **`class`**, **`order`**, **`alliance`**.
Jede Nicht-Formations-Zeile hat einen `parent_id`, der in höchstens drei
Schritten eine Formation erreicht — geprüft am realen Index (siehe unten) und
durch `internal/adapters/sqlite/hierarchy_integrity_test.go` als
Fixture-Integritätstest festgehalten. Der Verband ist praktisch durchgehend
das Ende der EUNIS-Seite; **Assoziationen kommen nicht vor** und sind in
keiner freien paneuropäischen Quelle verfügbar — das ist die dokumentierte
Decke, kein Versäumnis.

### Volle Hierarchie via FloraVeg.EU als Primärquelle

`pipelines/eurovegchecklist/` lädt die EuroVegChecklist (Mucina et al. 2016 +
Updates, Version 4) und liefert Klasse/Ordnung/Verband mit Codes, Autorschaft
und EEA-Code. `data/syntaxa_formations.csv` liefert die 25 Formationszeilen
(Sektionen A–Y mit Lebensform-Gruppe). Beide sind seit der Quellenumkehr
**Pflichtquellen** — ihr Fehlen bricht den Ingest ab, statt einen Index ohne
Hierarchie stillschweigend zu bauen.

Gemessen gegen `out/situs-neu.sqlite`, gebaut mit `go run ./cmd/situs ingest
--csv-dir out/ingest-input --db out/situs-neu.sqlite`:

```sql
SELECT rank, COUNT(*) FROM syntaxon GROUP BY 1 ORDER BY 1;
-- formation|25
-- class|150
-- order|381
-- alliance|1326

SELECT COUNT(*) FROM syntaxon WHERE rank<>'formation' AND parent_id='';
-- 0  (keine Waise)

SELECT COUNT(*) FROM syntaxon WHERE parent_provenance='derived';
-- 10  (Geschwisterkonsens)

SELECT COUNT(*) FROM syntaxon WHERE source='eunis';
-- 16  (EEA-eigene Zeilen ohne FloraVeg-Gegenstück)
```

Von den **1326** Verbänden im Index sind **1310** aus der FloraVeg-Hierarchie
direkt übernommen (Name, Author, ParentID stammen von FloraVeg) und **16** rein
EEA-eigen (`source='eunis'`, historischer EUNIS-Kombi-String, `Author=""`).
Von diesen 16 fanden **6** ihren Elternteil per Namensabgleich gegen die
FloraVeg-Verbandsnamen (`parent_provenance='official'`), **10** per
Geschwisterkonsens innerhalb ihrer EEA-Ordnungsgruppe
(`parent_provenance='derived'`) — keiner blieb Waise.

Der wörtliche `Syntaxa`-Block aus dem Ingest-Report (`SyntaxaReport`, ersetzt
seit dieser Umkehrung das frühere Feldpaar `Syntaxa`/`SyntaxonLinks` von
`IngestReport`):

```json
"Syntaxa": {
  "FormationsWritten": 25,
  "ClassesWritten": 150,
  "OrdersWritten": 381,
  "AlliancesWritten": 1310,
  "EunisOnly": 16,
  "LinksWritten": 1283,
  "LinksRemapped": 1265,
  "ParentsByName": 6,
  "ParentsDerived": 10,
  "Orphans": null,
  "AmbiguousMatches": null,
  "UnknownLinkTargets": null,
  "SkippedRows": 0
}
```

`SkippedUnknownSection` und `SkippedPattern` sind seit der letzten
Review-Runde entfernt: keines der beiden wurde je befüllt.

`EEACodeCollisions` fehlt im obigen Block, weil der gemessene Lauf älter ist
als das Feld; für diese Daten wäre es `null`.
`pipelines/eurovegchecklist/xlsx_to_csv.py` bricht die Konvertierung ab,
sobald zwei Primärcodes denselben EEA-Code beanspruchen — aber der Go-Ingest
liest CSV direkt, und eine handeditierte oder anders erzeugte Datei umgeht
diese Absicherung. Trägt die Hierarchie denselben EEA-Code doppelt, wird der
Code gemeldet und **ganz aus der Abbildung EEA-Code → Primärcode genommen**,
statt die zufällig letzte Zeile zum Gewinner zu küren; die betroffene
EEA-Einheit bleibt dann als eigene Zeile stehen, wie eine ohne Gegenstück.

`AlliancesWritten` (1310) zählt nur die FloraVeg-Hierarchiezeilen; zusammen mit
`EunisOnly` (16) ergibt das die 1326 Verbandszeilen der `syntaxon`-Tabelle.
`LinksWritten` (1283) zählt Schreibversuche, nicht die am Ende distinkten
Zeilen — der EEA-Code-Join löst eine EEA-Kante auf FloraVegs Primärcode auf und
kann sie damit auf dieselbe Zielkante schreiben wie eine bereits vorhandene
FloraVeg-Kante (siehe den `U36`/`ASP-03`/`KC03`-Sonderfall unten); die
`habitat_type_syntaxon`-Tabelle selbst trägt danach **1282** Zeilen.

### Der stille Namensabgleichsfehler: `AMM-02B` → `JD02`, nicht `JE01`

Der EEA-Code-Join löst EEA-Kanten über den EEA-Code auf FloraVegs Primärcode
auf. Gemessen am Nachfolger von `eea_code='AMM-02B'`:

```sql
SELECT id, parent_id FROM syntaxon WHERE eea_code='AMM-02B';
-- JD02B|JD02
```

`JD02` ist korrekt — der alte, auf reinem Namensvergleich beruhende Abgleich
hätte hier still `JE01` geliefert (ein Nachbarcode mit ähnlichem, aber
falschem Namen). Der EEA-Code-Join über die eindeutige EEA-ID vermeidet genau
diese Fehlklasse.

### Kryptogamen-Verbände (Sektionen R–Y)

Verbände, deren Formation (über Klasse → Ordnung → Verband) in den
EuroVegChecklist-Sektionen R–Y liegt (Kryptogamen- und Algenvegetation, nicht
Phanerogamen):

```sql
SELECT COUNT(*) FROM syntaxon a
  JOIN syntaxon o ON o.id=a.parent_id JOIN syntaxon c ON c.id=o.parent_id
  WHERE a.rank='alliance' AND c.parent_id IN ('R','S','T','U','V','W','X','Y');
-- 190
```

Vor der Quellenumkehr trug der Index hier **2** Verbände (die EEA-Zuordnung
kennt die Kryptogamen-Sektionen praktisch nicht); die FloraVeg-Hierarchie
liefert die vollen **190**.

### Kanten vor und nach der Umkehrung: keine verlorene EUNIS-Zuordnung

`habitat_type_syntaxon` trägt vor und nach der Umkehrung dieselbe Zeilenzahl:

| Index | `habitat_type_syntaxon` | `code='U36'` |
|---|---|---|
| `situs.sqlite` (vor der Umkehrung) | **1282** | **15** |
| `out/situs-neu.sqlite` (nach der Umkehrung) | **1282** | **15** |

Keine EUNIS-Zuordnung ging verloren. Die eine dokumentierte Ausnahme der
Zusage ist bereits in beiden Zahlen aufgegangen: `U36` verlinkte vor der
Umkehrung sowohl auf `ASP-03` als auch (separat) auf `KC03`; nach der
Umkehrung bildet der EEA-Code-Join `ASP-03` auf `KC03` ab, die beiden Kanten
fallen auf eine zusammen — `U36` trägt in beiden Ständen 15 Kanten, nicht 16
und nicht 14.

### Navigation über `GET /v1/syntaxa` und `GET /v1/syntaxon/{id}`

Gemessen am Referenzindex (`out/situs-neu.sqlite`, 2026-09-21), rein über die
HTTP-Routen der Syntaxa-Navigation — beginnend bei `GET /v1/syntaxa` (ohne
Parameter) und ausschließlich `children` aus `GET /v1/syntaxon/{id}`
verfolgend, ohne eine einzige ID im Voraus zu kennen:

| Größe | Route/Filter | Gemessen |
|---|---|---|
| Formationen | `GET /v1/syntaxa` | **25** |
| Klassen | `GET /v1/syntaxa?rank=class` | **150** |
| Ordnungen | `GET /v1/syntaxa?rank=order` | **381** |
| Verbände | `GET /v1/syntaxa?rank=alliance` | **1326** |
| Moos-/Flechtenverbände | `GET /v1/syntaxa?rank=alliance&life_form_group=bryophyte_lichen` | **137** |
| Algenverbände | `GET /v1/syntaxa?rank=alliance&life_form_group=algae` | **53** |
| über `children` erreichbare Zeilen (davon Verbände) | reines Verfolgen von `children` ab den 25 Formationen | **1882** (davon **1326** Verbände) |

Jeder Verband erreicht seine Formation in genau drei Schritten (`ancestors`
hat für jeden Verband die Länge 3, für jede andere Zeile höchstens 3); keine
Assertion des Messskripts schlägt an. Die Summe der über `children`
erreichbaren Zeilen (1882) deckt sich mit der SQL-gemessenen Gesamtzahl aus
der Tabelle „Umfang des Index" oben — die HTTP-Navigation lässt keine Zeile
aus.

## Verbreitung der Syntaxa (`syntaxon_distribution`)

!!! info "Referenzlauf: 2026-09-22, Syntaxa-Verbreitung (Task 12)"

    Die Zahlen dieses Abschnitts stammen aus einem vollständigen
    `situs ingest`-Lauf gegen `out/situs-neu.sqlite`, gebaut mit
    `bash pipelines/evc-distribution/build.sh` gefolgt von
    `go run ./cmd/situs ingest --csv-dir out/ingest-input --db out/situs-neu.sqlite`.
    Die Artenrollen- und Zeigerwert-Zahlen dieses Laufs sind **nicht**
    vergleichbar mit dem Referenzlauf oben: hostus war in dieser Umgebung nicht
    erreichbar, entsprechend blieben Artenauflösung und Zeigerwerte auf 0 —
    unerheblich für diesen Abschnitt, der ausschließlich die Syntaxa-Seite misst.

Preislerová et al. bilden 1115 europäische Vegetationsverbände über 136
Territorien ab (Zenodo Record
[11580949](https://zenodo.org/records/11580949), CC-BY 4.0). Zitiert werden
**beide** Veröffentlichungen, wie es die Lizenz verlangt:

- Preislerová Z. et al. (2022) Distribution maps of vegetation alliances in
  Europe. *Applied Vegetation Science* 25: e12642.
  <https://doi.org/10.1111/avsc.12642>
- Preislerová Z. et al. (2024) Structural, ecological and biogeographical
  attributes of European vegetation alliances. *Applied Vegetation Science*
  27: e12766. <https://doi.org/10.1111/avsc.12766>

### Die Pipeline, gemessen gegen das gepinnte Artefakt

```bash
bash pipelines/evc-distribution/build.sh
```

```json
{
  "alliances": 1115,
  "covered": 1115,
  "skipped_rows": 0,
  "slug_collisions": [],
  "summary_rows": 4,
  "territories": 136,
  "uncertain": 1920,
  "value_histogram": {"": 140112, "1": 9608, "U": 1920},
  "verified": 9608,
  "written": 11528
}
```

Die Wertemenge der Quellzellen ist bei diesem Lauf **erneut gemessen**, nicht
angenommen: genau drei Werte kommen vor (leer = Abwesenheit, `1` = `verified`,
`U` = `uncertain`) — die Zusage aus Abschnitt 9 der Design-Spec.

### Der Ingest-Report, wörtlich

```json
"Territories": {"Areas": 136, "SkippedRows": 0},
"SyntaxonDistribution": {
  "Written": 11525,
  "Verified": 9605,
  "Uncertain": 1920,
  "Covered": 1114,
  "SkippedRows": 0,
  "UnknownSyntaxa": ["CI01E"]
}
```

`Written` (11525) und `Verified` (9605) liegen **3 unter** den Pipeline-Zahlen
(11528, 9608): genau die drei Zeilen, die der Ingest für `CI01E` (die Pipeline
liest 3 Vorkommenszeilen für diesen Code) verwirft, weil die Hierarchie diesen
Code nicht führt — siehe `TestIngestSyntaxonDistributionVerwirftUnbekanntenCodeUndMeldetIhn`.
`SkippedRows` bleibt `0`: eine verworfene Zeile wegen unbekannten Syntaxons ist
kein *malformed row*, sondern eine benannte Fassungsdrift, und zählt deshalb in
`UnknownSyntaxa`, nicht in `SkippedRows`.

### Der Index, abgefragt

```sql
SELECT COUNT(*) FROM syntaxon_distribution;
-- 11525
SELECT occurrence, COUNT(*) FROM syntaxon_distribution GROUP BY 1 ORDER BY 1;
-- uncertain|1920
-- verified|9605
SELECT COUNT(*) FROM syntaxon_distribution_coverage;
-- 1114
SELECT COUNT(DISTINCT area_code) FROM syntaxon_distribution;
-- 136
SELECT COUNT(*) FROM area WHERE area_scheme='evc_territory';
-- 136
SELECT COUNT(*) FROM area WHERE area_scheme='wgsrpd_l3';
-- 369
SELECT COUNT(*) FROM syntaxon s WHERE s.rank='alliance' AND NOT EXISTS (
  SELECT 1 FROM syntaxon_distribution_coverage v WHERE v.syntaxon_id=s.id);
-- 212
SELECT COUNT(*) FROM syntaxon a
  JOIN syntaxon o ON o.id=a.parent_id JOIN syntaxon c ON c.id=o.parent_id
  JOIN syntaxon_distribution_coverage v ON v.syntaxon_id=a.id
  WHERE a.rank='alliance' AND c.parent_id IN ('R','S','T','U','V','W','X','Y');
-- 0
SELECT COUNT(*) FROM syntaxon_distribution d
  LEFT JOIN syntaxon s ON s.id=d.syntaxon_id WHERE s.id IS NULL;
-- 0
SELECT COUNT(DISTINCT d.syntaxon_id) FROM syntaxon_distribution d
  LEFT JOIN syntaxon_distribution_coverage v
    ON v.syntaxon_id=d.syntaxon_id AND v.area_scheme=d.area_scheme
  WHERE v.syntaxon_id IS NULL;
-- 0
```

Die beiden letzten Nullen sind die eigentlichen Zusagen dieses Teilprojekts:
kein Verweis der Verbreitungstabelle zeigt ins Leere, und keine
Verbreitungszeile existiert ohne ihre Coverage-Zeile — sonst wäre `absence` für
dieses Syntaxon wieder nicht von `unknown` zu unterscheiden.

### 212 Verbände ohne jede Aussage — 190 davon Kryptogamen

```sql
SELECT f.life_form_group, COUNT(*) FROM syntaxon a
  JOIN syntaxon o ON o.id=a.parent_id JOIN syntaxon c ON c.id=o.parent_id
  JOIN syntaxon f ON f.id=c.parent_id
  WHERE a.rank='alliance' AND NOT EXISTS (
    SELECT 1 FROM syntaxon_distribution_coverage v WHERE v.syntaxon_id=a.id)
  AND f.id IN ('R','S','T','U','V','W','X','Y')
  GROUP BY 1;
-- algae|53
-- bryophyte_lichen|137
```

**137** Moos-/Flechtenverbände und **53** Algenverbände tragen keine Aussage —
zusammen die **190** Kryptogamen-Verbände, für die die Quelle prinzipbedingt
nichts sagt (sie deckt nur Gefäßpflanzen-Vegetation ab). Die verbleibenden
**22** sind vaskuläre Verbände ohne Aussage:

```sql
SELECT s.id FROM syntaxon a
  JOIN syntaxon o ON o.id=a.parent_id JOIN syntaxon c ON c.id=o.parent_id
  JOIN syntaxon s ON s.id=a.id
  WHERE a.rank='alliance' AND NOT EXISTS (
    SELECT 1 FROM syntaxon_distribution_coverage v WHERE v.syntaxon_id=a.id)
  AND c.parent_id NOT IN ('R','S','T','U','V','W','X','Y')
  ORDER BY s.id;
-- AMM-01B, AMM-01C, CRU-01A, CRU-01B, CRU-01C, CRU-01D, CRU-02A, CRU-02B,
-- CRU-02C, CRU-03A, CRU-03B, CRU-03C, CT06A, DA12A, DA13A, DD01A, DD01B,
-- DD01C, GER-02E, NAR-01E, QUI-01F, TUB-03D
```

**Abweichung von der Aufgabenstellung:** die Vorgabe für diesen Task nannte
hier namentlich nur sechs Verbände (`CT06A`, `DA12A`, `DA13A`, `DD01A`,
`DD01B`, `DD01C`). Das reale, gemessene Bild ist größer: **22**, nicht 6. Die
16 zusätzlichen (`AMM-*`, `CRU-*`, `GER-02E`, `NAR-01E`, `QUI-01F`, `TUB-03D`)
tragen durchweg einen EEA-eigenen EEA-Code-Stil statt eines FloraVeg-Primärcodes
und sind plausibel Verbände, die die EVC-Verbreitungsquelle strukturell nicht
führen kann (kein FloraVeg-Primärcode, an dem die Pipeline andocken könnte),
nicht bloß zufällig ohne Daten geblieben. Die 6 genannten bleiben Teil der 22
und sind namentlich unverändert dieselben. Die Summe 190 + 22 = 212 ist
gemessen und schließt.

### Der Fassungsunterschied: EVC 3 gegen EVC 4, und `CI01E`

Die Verbreitungsdatei nennt sich selbst, laut ihrem "Read me"-Blatt, Version 2
(2024-06-12) und referenziert damit **EuroVegChecklist-Fassung 3**
(2024-06-12). Die Hierarchie-Datei dagegen heißt
`List_of_European_vegetation_units_version_4.xlsx` und trägt in ihren
Spaltenköpfen selbst den Stand `EVC, version 2025-06-12` — zwei gemessene
Angaben, die **Verschiedenes** meinen (Dateifassung gegen internen EVC-Stand)
und deshalb nebeneinander stehen, nicht zu einer Zahl verrechnet werden.

Die belastbare Aussage über die Überdeckung ist nicht die Differenz der
Versionsnummern, sondern die gemessene **1114 / 1115**: von den 1115
Verbänden, die die Verbreitungsquelle führt, kennt die (neuere) Hierarchie
1114 unter demselben Primärcode. Der eine fehlende ist `CI01E`
(`Campanulo-Nardion`, EEA-Code `NAR-01E`) — der Ingest verwirft seine drei
Vorkommenszeilen und meldet ihn namentlich in `UnknownSyntaxa`, statt ihn
stillschweigend zu verwerfen oder eine leere Liste zu melden, die die
Fassungsdrift verschwiegen hätte.

## Anhang-I-Abdeckung (offener Punkt 5)

| Größe | Gemessen |
|---|---|
| Habitattypen mit **irgendeiner** Anhang-I-Entsprechung | **184** |
| davon mit Qualifier `=` | **29** |

Beide Zahlen sind am Referenzlauf **nachgemessen**, jetzt mit beiden
`≈`-Zeilen im Index: 184 bleibt 184 — die zwei `≈`-Typen (`R1S`, `U28`) haben
ohnehin schon eine andere Anhang-I-Zeile, erweitern die Menge also nicht.

Die `=`-Quote ist die praktisch wichtige Zahl: nur `=` ist präzise genug, um
einen deutschen Namen zu leihen. Am Referenzlauf ist das eingetreten:
`Localizations: 567`, `DerivedLabels: 29` — die 29 EUNIS-Typen erben ihr
deutsches Label aus dem amtlichen Anhang-I-Namen, ohne Codeänderung. Offener
Punkt 6 ist damit geschlossen.

Ein Habitattyp **ohne** Anhang-I-Entsprechung ist der Normalfall, nicht ein
Fehler: fehlende Daten sind Abwesenheit von Zeilen, nie ein Platzhalter-Code.

## Namensauflösung (offener Punkt 3)

Gemessen am Referenzlauf. Aufgelöst wird seit der Aggregat-Erweiterung **nicht**
mehr über hostus' `/v1/match`, sondern gegen die lokale, deterministische
`eurosl_crosswalk.csv` aus `hostus export-crosswalk` (6,1 MB, aus derselben
hostus-Datenbank exportiert):

| Grundgesamtheit | Aufgelöst | Rate |
|---|---|---|
| Zeilen von `species_roles.csv` | 11618 / 13791 (2173 offen) | **84,24 %** |
| distinkte Artennamen | 3136 / 3587 (451 offen) | **87,43 %** |

Die distinktgewichtete Zahl ist die mit dem ESy-Spike vergleichbare
Grundgesamtheit: **87,43 %** liegen deutlich über der dort abgeschätzten
~57 %-Untergrenze (`../research/sp9-esy-spike.md`).

Gegenüber dem August-Lauf über `/v1/match` (83,82 % zeilen-, 87,59 %
namensgewichtet) liegt die Zeilenrate leicht höher, die Namensrate minimal
niedriger — der Dateiexport rät nicht: **63** Namen tragen in der
Crosswalk-Datei mehr als eine Konzept-ID (`AmbiguousCrosswalk`, durchweg
Sammelarten wie `Ranunculus acris aggr.` und `Taraxacum sect. Taraxacum`) und
bleiben deshalb bewusst unaufgelöst, statt auf eine der beiden geraten zu
werden.

Dazu kommen **905** abgeleitete Mitgliedsarten-Zeilen aus
`aggregate_members.csv` (`DerivedRows`), die **193** Konzepte in den Index
bringen, die sonst fehlten; bei **2** war der Schlüssel schon belegt
(`SuppressedByExplicit`). Ob dort eine explizite `species_roles.csv`-Zeile
gewonnen hat oder zwei Aggregate dieselbe Mitgliedsart nennen, unterscheidet
der Zähler nicht — beides ist derselbe `ON CONFLICT DO NOTHING`, und beides ist
kein Defekt.

Nicht aufgelöste Namen werden **behalten**, nicht verworfen: `verbatim_name` ist
immer gesetzt, `concept_id` bleibt NULL.

## Verbreitung (`species_distribution`)

Gemessen am Referenzlauf. Der Schritt fragt hostus einmal pro Konzept,
gedrosselt auf 70 ms. Gemessen ist die Dauer des **ganzen** `situs ingest`:
**5:46**. Wie viel davon auf diesen Schritt entfällt, wurde nicht einzeln
gemessen; rechnerisch sind allein die Drosselpausen 3323 × 70 ms ≈ 3:53.

| Kennzahl | Gemessen |
|---|---|
| Konzepte im Index (`Concepts`) | 3323 |
| davon mit Verbreitungsdaten (`WithAreas`) | 3315 (99,8 %) |
| geschriebene Zeilen (`Rows`) | 106068 |
| unvollständige Gebiete (`Incomplete`) | 0 |
| übersprungene Konzepte (`DistributionFailed`) | 0 |
| verschiedene Gebietscodes (`areas_with_data` in `/v1/info`) | 366 |

Woher die **3323** kommen, nachgerechnet am Index:

| | |
|---|---|
| Konzepte aus den Zeilen von `species_roles.csv` | 3130 |
| Konzepte, die **nur** über abgeleitete Mitgliedsarten dazukommen | 193 |
| **Summe** | **3323** |

`species_roles.csv` trägt 3587 verschiedene Artnamen, davon sind 3136
aufgelöst (siehe oben); diese 3136 Namen fallen auf **3130** Konzepte — nur
6 Namenspaare treffen dasselbe Konzept. Von den 200 Konzepten, die abgeleitete
Mitgliedsarten-Zeilen tragen, sind 7 bereits über eine eigene Artenzeile im
Index, sodass 193 neu sind. Der Abstand zur Zahl der Namen ist also fast
ausschließlich die Auflösungslücke, nicht Synonymie.

Die **8** Konzepte ohne Verbreitungsdaten sind exakt die 8, die nicht auf
`wcvp:` lauten (siehe „Ein gemischter Backbone" unten) — hostus führt für sie
keine Verbreitung, und der Ingest trägt das als Abwesenheit von Zeilen ein,
nicht als „kommt nirgends vor".

### Gebietsnamen (`area`)

Die Namen zu diesen Codes kommen aus `pipelines/wgsrpd` (TDWG-Tabelle
`tblLevel3.txt`, 2. Auflage, an einen Commit gepinnt) — eine lokale CSV, kein
Dienst. Gemessen am gepinnten Stand: **369** Gebiete, 0 übersprungene Zeilen
(`output/report.json` der Pipeline).

369 ist das ganze WGSRPD-Level-3-Vokabular, 366 ist die Zahl der Codes mit
Verbreitungsdaten in diesem Index. `GET /v1/areas` listet die zweite Menge:
nur Gebiete, nach denen sich auch filtern lässt.

**Am Referenzlauf gemessen: alle 366 Codes mit Daten haben einen Namen** — die
Namensmenge deckt die Datenmenge vollständig, `"name": ""` tritt an diesem
Datenstand nicht auf. Die Route trägt den Fall trotzdem (ein Code mit Daten,
aber ohne Namenszeile, bleibt mit leerem `name` in der Liste): 369 zu 366 ist
eine Eigenschaft *dieser* beiden Quellen, keine Garantie der Route.

Warum das den Aufwand wert war, an `eunis@2021/R15` mit `?area=GER` gemessen:

| Abfrage | Einträge |
|---|---|
| ohne Filter / `?area=GER` | 170 (`in_area: true` 78, `false` 82, Feld fehlt 10) |
| `?area=GER&only_in_area=true` | 88 (die 82 `false` entfernt, alle 10 unentscheidbaren behalten) |

**48 % der Arten dieses Habitattyps kommen in Deutschland nicht vor.** Eine
Artenliste ohne Gebietsfilter schickt einen Nutzer im Gelände also in etwa jedem
zweiten Fall hinter eine Pflanze, die dort nicht wachsen kann.

## Zeigerwerte (trait_value)

Drei Vokabulare (EIVE 1.0, Tichý 2023 v2.0, Midolo 2023 v3), ein
`hostus.Resolve()`-Aufruf über alle drei Dateien zusammen.

Gemessen am Referenzlauf, gegen den lokalen hostus:

| Vokabular | Zeilen | Aufgelöst | Nicht aufgelöst |
|---|---|---|---|
| eive | **71266** | 69094 (96,95 %) | 2172 |
| tichy2023 | **45592** | 45224 (99,19 %) | 368 |
| midolo2023 | **31910** | 31625 (99,11 %) | 285 |
| **gesamt** | **148768** | **145943 (98,10 %)** | **2825** |

Die Auflösungsquote liegt hier deutlich über der der Artenrollen (87,43 %
namensgewichtet), weil die drei Vokabulare überwiegend akzeptierte
Binomen führen, während `species_roles.csv` Sammelarten, Sektionen und
Aggregate trägt, die kein eindeutiges Konzept haben.

Geschriebene `trait_value`-Zeilen im Index: **138599** über **13824**
Konzepte — weniger als die 145943 aufgelösten Quellzeilen, weil mehrere
Quellzeilen desselben Vokabulars auf dasselbe (Konzept, Dimension) fallen
können und der Primärschlüssel sie zusammenführt.

Zeilenzahlen der kanonischen Pipeline-Ausgaben (2026-08-30, unverändert):
`pipelines/eive/output/eive-canonical.csv`,
`pipelines/tichy/output/tichy-canonical.csv`,
`pipelines/midolo/output/midolo-canonical.csv`. Nicht dasselbe wie „Zeilen der
Trait-CSVs im `--csv-dir`" — der Ingest liest die gleich benannten, aber ggf.
anders platzierten `eive_traits.csv`/`tichy_traits.csv`/`midolo_traits.csv`; an
den kanonischen Ausgaben ändert das nichts, sie sind identisch.

## Ein gemischter Backbone (2026-09-16, neu)

Der Referenzlauf hat einen Zustand erzeugt, den der Entwurf ausdrücklich nicht
vorsah. `/v1/info` meldet:

```json
"concept_backbones": ["cdm", "eurosl", "wcvp"]
```

Von **3323** Konzepten lauten **3315** auf `wcvp:`, **7** auf `eurosl:` und
**1** auf `cdm:`. Es sind durchweg Sammelarten und Sektionen, für die WCVP kein
Konzept führt:

| Konzept | Name |
|---|---|
| `cdm:concept:947bc56d…` | *Salix cinerea* subsp. *cinerea* |
| `eurosl:concept:18603a73…` | *Achillea millefolium* aggr. |
| `eurosl:concept:236666d3…` | *Taraxacum* sect. *Alpina* |
| `eurosl:concept:2e90cf36…` | *Salicornia europaea* aggr. |
| `eurosl:concept:3363d6dd…` | *Valeriana officinalis* aggr. |
| `eurosl:concept:4a8b5bf3…` | *Taraxacum* sect. *Palustria* |
| `eurosl:concept:6174d223…` | *Leucanthemum vulgare* aggr. |
| `eurosl:concept:fc7d5e20…` | *Polygonum aviculare* aggr. |

Sie stammen aus `hostus export-crosswalk`, das einen Namen auf das Konzept
abbildet, das ihn trägt — und das ist für ein Aggregat eben ein EuroSL- oder
CDM-Konzept. Der Ingest warnt darüber am Ende des Laufs
(`the index holds concept ids the batch route cannot answer`).

**Was daraus folgt.** Die Einzelrouten beantworten diese IDs normal; der
Batch-Endpunkt `POST /v1/species/habitat-types` prüft das Präfix dagegen gegen
eine **compile-time-Konstante** `wcvp` und antwortet für sie
`known: false, reason: unknown_backbone` — obwohl der Index ihre Fakten
besitzt. Dieselben 8 sind auch die 8 ohne Verbreitungsdaten.

Der Anteil ist mit 0,24 % klein, die Aussage aber falsch: `unknown_backbone`
heißt „der Fehler liegt beim Aufrufer", und hier liegt er nicht dort. Was daran
zu tun ist — Prüfung gegen die gemessenen Backbones, ein dritter `reason`, oder
eine Ingest-Regel, die Nicht-wcvp-Konzepte gar nicht erst schreibt — ist eine
offene Entwurfsfrage und wird in
[#36](https://github.com/jobrunner/situs/issues/36) verhandelt, nicht hier
entschieden.

## Beschreibungen der Habitattypen

Zwei Quellen, zwei Herkünfte, eine Tabelle:

| | Typen | `provenance` | Quelle |
|---|---|---|---|
| EUNIS 2021 | **264** | `official` | EUNIS-ESy-Factsheets, Wortlaut unverändert |
| Anhang I | **205** | `situs` | von situs verfasst aus EUR 28, dem EUNIS-Crosswalk und den Artendaten des Index |

Beide Textmengen sind **versioniert, nicht erzeugt**: sie entstanden mit
KI-Unterstützung und sind nicht reproduzierbar, ein erneuter Lauf ergäbe
andere Formulierungen. Sie liegen deshalb als CSV im Repo
(`data/annex1_descriptions.csv`, `data/localizations_descriptions.csv`), gehen
über `make ingest-input` denselben Weg wie jede Pipeline-Ausgabe und werden
von `cmd/situs/curated_test.go` geprüft, weil keine Pipeline das für sie tut.
Ein Ingest aus einem gegebenen Repo-Stand liefert damit immer denselben Index,
ohne dass jemand ein Modell braucht.

Die Anhang-I-Texte sind **keine** Übersetzung und **keine** Übernahme des
amtlichen Wortlauts. Das Interpretationshandbuch ist eine Abgrenzungsvorschrift
für die Rechtsanwendung; es beantwortet die Frage „zählt dieser Bestand als
6230", nicht die Frage „wovor stehe ich hier". Eine amtliche deutsche Fassung
existiert ohnehin nicht: weder die Kommission noch das BfN gibt EUR 28 auf
Deutsch heraus, und die deutschen wie österreichischen Handbücher sind
eigenständige Texte für ihr Land (93 bzw. 65 Typen), keine Übersetzungen.

Beide Sprachen sind gemessen vollständig: **205** englische Beschreibungen in
`habitat_description`, **205** deutsche als Overlay in `localization`
(`field = description`), dazu die 264 deutschen EUNIS-Beschreibungen, zusammen
**469** Localization-Zeilen dieses Feldes. Textlänge der Anhang-I-Beschreibungen
(englisch, gemessen): min 625, Median 964, max 1256 Zeichen.

Das Arbeitsmaterial dazu steht in `pipelines/eur28/output/annex1_source.csv`
und wird **nicht** ingestiert: **233** Einträge aus dem gepinnten Handbuch,
alle mit Definition, 224 mit Artenliste, 108 mit korrespondierenden Kategorien.
Der Index führt 205 davon; die übrigen 28 sind marine und Küstentypen, die die
EEA-Klassifikation dieses Index nicht kennt.

## Deutsche Labels (amtlich, aus EUR-Lex)

Gemessen am 2026-08-24 gegen CELEX `01992L0043-20130701` (deutsche
konsolidierte Fassung), bezogen über das Cellar-Repository des Amts für
Veröffentlichungen. Nachnutzung nach Beschluss 2011/833/EU mit Quellenangabe;
jede Zeile trägt `source = eur-lex:31992L0043`.

| Kennzahl | Wert |
|---|---|
| Codes in Anhang I laut EUR-Lex | **233** |
| Anhang-I-Typen im Index | **205** |
| In EUR-Lex, aber nicht im Index | **28** |
| Im Index, **ohne** amtlichen Namen | **0** |
| Abgeleitete EUNIS-Labels (`=`-Crosswalk) | **29** |

### Die Differenz 233 ↔ 205 ist geklärt

Der offene Punkt aus der Design-Spec — das BfN nennt 231 Anhang-I-Typen, der
Index führt 205 — löst sich in zwei Teile:

1. **EUR-Lex liefert 233, nicht 231.** Zwei mehr als die BfN-Angabe; die
   konsolidierte Fassung von 2013 ist neuer als die dort zitierte Zählung.
2. **Die 28 fehlenden sind überwiegend marine und Küstentypen**: `1110`
   (Sandbänke), `1120` (*Posidonia*-Bestände), `1130` (Estuarien), `1150`
   (Lagunen), `1170` (Riffe), `1320`, `1330`, `1410`, `1420`, `1630`, `1650` …
   Sie fehlen dem **Index**, nicht der Lokalisierung — eine Eigenschaft der
   EEA-Quelldaten, die situs einliest.

**Entscheidend ist die zweite Richtung: sie ist null.** Kein einziger Typ, den
der Index führt, bleibt ohne amtlichen deutschen Namen. Der Fall, der stillschweigend
schiefgehen könnte — jemandem fällt in der App ein englisches Label auf —
tritt nicht ein.

### Was die Ableitung erreicht

Von 339 Crosswalks nach `annex1` sind 29 ein `=`, und alle 29 zünden: der
amtliche Anhang-I-Name wird an den EUNIS-Typ verliehen, markiert als
`derived`. Beispiele:

| EUNIS | Abgeleiteter deutscher Name |
|---|---|
| `N18` | Entkalkte Dünen mit *Empetrum nigrum* |
| `R61` | Mediterrane Salzwiesen (*Limonietalia*) |
| `S65` | Iberische Gipssteppen (*Gypsophiletalia*) |

Die verbleibenden **241** EUNIS-Level-3-Typen ohne `=`-Crosswalk haben keine
amtliche Quelle und tragen `provenance = situs`.

Die Ableitung erreicht **Level 1 und 2 überhaupt nicht**: keiner der 49 Codes
der Level 1 und 2 trägt einen `=`-Crosswalk nach `annex1` (gemessen gegen
`crosswalks.csv`). Ohne verfasste Namen bliebe damit genau die Ebene englisch,
die in einer App als Gruppenknopf zuerst sichtbar ist — `R` und `R5` stünden
neben einem deutschen `R55`. Deshalb führt `data/localizations-de-situs.csv`
seit dem 2026-09-21 auch diese 49 Typen.

### Die 290 von situs verfassten Namen

Gemessen am 2026-09-21 mit `pipelines/eurlex/merge.py` gegen
`data/localizations-de-situs.csv`:

| Kennzahl | Wert |
|---|---|
| Verfasste Typen (EUNIS L1–L3 ohne `=`-Crosswalk) | **290** (241 L3 + 49 L1/L2) |
| Erzeugte Localization-Zeilen | **394** |
| davon `field = vernacular` | **104** (36 %) |
| Überschneidung mit den 29 Ableitungen | **0** |
| Localizations im Index gesamt | **627** (233 `official` + 394 `situs`) |
| Zeilen in `localization` nach dem Ingest | **656** (627 + 29 `derived`) |

Die beiden letzten Zeilen sind aus der gemessenen `merge.py`-Ausgabe
**gerechnet**, nicht aus einem Index gezählt: der Referenzlauf vom 2026-09-16
maß die alten Werte 567 und 596. Der nächste volle `situs ingest` hat sie zu
bestätigen.

Die 36 % Vernakular-Abdeckung ist keine Lücke, sondern das Ergebnis der Regel:
ein etablierter deutscher Begriff existiert im Wesentlichen nur, wo der Typ in
Deutschland vorkommt — und auf Level 1 und 2 zusätzlich nur dort, wo ein
Sammeltyp überhaupt genau einen Begriff hat. 'Laubwald' ist weiter als `T1`
(der immergrüne Laubwald `T2` steckt mit drin), 'Phrygana' enger als `S7`.
Beide tragen deshalb nur `name`. Für mediterrane, makaronesische und Schwarzmeer-Varianten
gibt es keinen — sie werden im Deutschen nie benannt, und ein konstruierter
Begriff wäre eine Erfindung mit dem Anschein von Geläufigkeit.

**Ein Fall, in dem die Regel gegen das Spec-Beispiel entschieden hat:** die Spec
zeigt `R22 Low and medium altitude hay meadow` → `Glatthaferwiese` als
Illustration. Streng angewandt entfällt das Vernakular dort: `R22` umfasst auch
die montanen Goldhaferwiesen, „Glatthaferwiese" ist also **enger** als der Typ.
Die Zeile trägt nur `name`.
