# Syntaxa-Verbreitung: wo eine Pflanzengesellschaft vorkommt

Stand: 2026-09-21. Teilprojekt **C** von drei. Setzt Teilprojekt A
(`2026-09-21-syntaxa-quellenumkehr-design.md`) voraus, weil der Join über die
EVC-Primärcodes läuft, die erst dort zur Syntaxon-ID werden.

**Ziel:** Der syntaxonomische Teil von situs ist ohne Verbreitung nur
begrenzt brauchbar. Eine Exkursions-App in Oberbayern braucht nicht die
Auskunft, dass ein Verband irgendwo in Europa vorkommt, sondern ob er *hier*
vorkommt. Die Artverbreitung (`species_distribution`, WGSRPD) liegt schon im
Index; für Syntaxa gibt es bisher nichts.

## 1. Die Quelle (gemessen, nicht angenommen)

[Zenodo Record 11580949](https://zenodo.org/records/11580949), Version 2.0 vom
2024-06-12, CC-BY 4.0. Relevant ist allein die Datenbank-Datei
(233.454 Bytes); die ZIP mit 82 MB fertiger Kartenbilder braucht situs nicht.

```
Distribution-maps-of-vegetation-alliances-in-Europe_ver-2_2024-06-12_database.xlsx
sha256 f395b2ce984148dd05d5ec661dfc616f668f6e20c43f3cfb0f1de0362b75665e
```

Zu zitieren sind zwei Arbeiten: Preislerová et al. (2022), *Applied Vegetation
Science* 25: e12642 (die Originalveröffentlichung, die der Record nennt) und
Preislerová et al. (2024), *Applied Vegetation Science* 27: e12766 (die das
„Read me“-Blatt der Fassung 2 als deren Bezug angibt). Beide gehören in
`manifest.yaml`; eine davon zu unterschlagen wäre bei CC-BY nicht nur
unhöflich.

Die Datei trägt drei Blätter. Datenblatt ist **„European alliances“**:

| Spalte | Inhalt |
|---|---|
| `Code 1` | EVC-Primärcode, z. B. `AA01A` |
| `Code 2` | derselbe EEA-Altcode wie in der FloraVeg-XLSX, z. B. `PAP-01A` |
| `Name` | Verbandsname ohne Autorschaft |
| `Name with author citation` | Name mit Autorschaft |
| 136 weitere | je ein Territorium |
| 4 letzte | Summenspalten (`Verified occurrences`, `Uncertain occurrences`, `All occurrences`, `% of uncertain occurrences`) |

Gemessen: **1115 Datenzeilen** mit gültigem Verbandscode, keine Dublette.
Die Wertemenge aller 1115 × 136 Gebietszellen ist **exakt** `{"", "1", "U"}`
— nichts sonst. Das „Read me“-Blatt gibt die Legende: `1` = verified
occurrence, `U` = uncertain occurrence, leere Zelle = absence. Belegt sind
11.528 Zellen (9.608 verified, 1.920 uncertain).

**Blatt „Borja tabulka“ ist kein Datenblatt.** Es ist ein Arbeitsentwurf mit
Ländercodes statt Territorien und einer tschechischen Notiz („doplnit
chybějící svazy z publikovaného EVC“ — die fehlenden Verbände aus dem
publizierten EVC ergänzen). Es wird wie „Read me“ übersprungen; die Pipeline
liest **ausschließlich** das Blatt „European alliances“, nach Namen, nicht
nach Position.

### Der Zellbezug muss ausgewertet werden

Excel lässt leere Zellen in der XML einfach weg. Bei 136 überwiegend leeren
Spalten pro Zeile ist eine positionelle Lesung (`row.findall("c")` in
Reihenfolge) nicht bloß ungenau, sondern vollständig falsch: gemessen liefert
sie die Wertemenge `{"", "1", "U", "0", "2", "3", …, "86.111"}` statt der
tatsächlichen drei Werte, weil Summenzeilen und Gebietsspalten
durcheinanderrutschen. Die Pipeline **muss** das `r`-Attribut jeder Zelle
(`r="AB7"`) in einen Spaltenindex umrechnen.

Nebenbefund: `read_sheet` in `pipelines/eurovegchecklist/xlsx_to_csv.py` liest
heute positionell. Für die gepinnte FloraVeg-Datei ist das nachgeprüft
unschädlich (gemessen: 0 von 1842 Zeilen haben eine Lücke), aber es ist eine
unabgesicherte Annahme über fremde Daten. Teilprojekt A fasst diese Datei
ohnehin an und härtet `read_sheet` dabei auf die `r`-basierte Lesung, damit
nicht zwei Varianten desselben Helfers im Repo stehen.

## 2. Territorien sind ein eigenes Gebietsschema

Die 136 Spalten sind keine WGSRPD-Gebiete. Sie sind teils Staaten
(`Albania`), teils biogeografische Teile davon (`Austria Alps`,
`Austria Continental`, `France Mediterranean`), und für viele existiert eine
eigene Küstenspalte (`Albania_coast`) — die Publikation zählt 82
Territorialeinheiten, die Tabelle spaltet sie in 136 Spalten auf.

Sie ziehen deshalb als **zweites `area_scheme`** ein, `evc-territory`, neben
dem bestehenden `wgsrpd-l3`. Eine Abbildung zwischen beiden Schemata gibt es
nicht und soll es nicht geben: dieselbe Haltung, die `CLAUDE.md` schon für
ISO↔WGSRPD festhält. Wer nach Artverbreitung filtert, benutzt WGSRPD-Codes;
wer nach Syntaxa-Verbreitung filtert, Territoriumscodes. Die beiden Fragen
werden nicht vermischt, und keine Antwort behauptet eine Umrechnung, die
niemand geprüft hat.

`area_code` wird aus dem Spaltennamen als Slug abgeleitet
(`Austria Alps` → `austria-alps`, `Albania_coast` → `albania-coast`:
Kleinschreibung, Leerzeichen und `_` zu `-`, sonst unverändert). Der
ungekürzte Spaltenname zieht als `name_en` in die bestehende `area`-Tabelle
ein, sodass ein Client die Codes benennen kann, ohne die Ableitungsregel zu
kennen. Die Pipeline prüft die Slugs auf Eindeutigkeit und bricht bei einer
Kollision ab statt einen Gewinner zu wählen — gemessen sind alle 136
eindeutig.

## 3. Vier Zustände, nicht drei

Bei der Artverbreitung ist „keine Zeile“ gleich „unbekannt“. Hier nicht, und
das ist der eine Punkt dieses Specs, an dem man sich vertun kann:

| Zustand | In der Quelle | Bedeutung |
|---|---|---|
| verified | Zelle `1` | kommt vor, belegt |
| uncertain | Zelle `U` | kommt möglicherweise vor |
| absence | leere Zelle, **Verband steht in der Tabelle** | kommt nicht vor — eine Aussage |
| unknown | **Verband steht nicht in der Tabelle** | keine Aussage |

Die vierte Zeile ist nicht theoretisch: die Quelle deckt ausdrücklich nur
*vascular-plant dominated vegetation* ab. Von den 1310 EVC-Verbänden tragen
1114 eine Verbreitungszeile, **196 nicht** — darunter alle 137 Moos- und
Flechtenverbände und 51 der 53 Algenverbände, also genau die Vegetation, die
Teilprojekt A überhaupt erst in den Index holt. Für sie ist jede Aussage über
jedes Territorium `unknown`, und sie darf niemals als `absence` erscheinen.

Deshalb zwei Tabellen:

```sql
-- Eine Zeile je belegter Zelle. Absence ist die Abwesenheit einer Zeile,
-- genau wie bei species_distribution.
CREATE TABLE IF NOT EXISTS syntaxon_distribution (
  syntaxon_id TEXT NOT NULL,
  area_scheme TEXT NOT NULL,
  area_code   TEXT NOT NULL,
  occurrence  TEXT NOT NULL CHECK (occurrence IN ('verified', 'uncertain')),
  PRIMARY KEY (syntaxon_id, area_scheme, area_code)
);
CREATE INDEX IF NOT EXISTS idx_syntaxon_distribution_area
  ON syntaxon_distribution(area_scheme, area_code);

-- Für welche Syntaxa die Quelle überhaupt eine Aussage macht. OHNE diese
-- Tabelle ist "kommt nicht vor" nicht von "niemand hat nachgesehen" zu
-- unterscheiden — und ein Verband mit 136 leeren Zellen sieht genauso aus
-- wie einer, der in der Quelle fehlt.
CREATE TABLE IF NOT EXISTS syntaxon_distribution_coverage (
  syntaxon_id TEXT NOT NULL,
  area_scheme TEXT NOT NULL,
  PRIMARY KEY (syntaxon_id, area_scheme)
);
```

Gemessen tragen alle 1114 gejointen Verbände mindestens eine belegte Zelle,
die Coverage-Tabelle ist heute also für keine Zeile die einzige Information.
Sie bleibt trotzdem: die Unterscheidung darf nicht davon abhängen, dass eine
künftige Quellfassung keinen Verband mit ausschließlich leeren Zellen
enthält.

## 4. Pipeline

Neu: `pipelines/evc-distribution/` (Python, **nur Stdlib**, wie
`pipelines/eunis` und `pipelines/eurovegchecklist`; kein `openpyxl` — die
dokumentierte Ausnahme gilt nur für die übernommenen Trait-Konverter).

```
xlsx_to_csv.py --xlsx artifacts/…database.xlsx --out-dir out
  → out/syntaxon_distribution.csv  (syntaxon_id|area_scheme|area_code|occurrence)
  → out/evc_territories.csv        (area_scheme|area_code|name_en)
  → out/report.json
```

`report.json` führt: `alliances` (Zeilen mit gültigem Code), `territories`,
`verified`, `uncertain`, `slug_collisions`, `skipped_rows` sowie
`value_histogram` — die tatsächlich vorgefundene Wertemenge der Gebietszellen.
Der letzte Punkt ist die Lehre aus Abschnitt 1: taucht dort etwas anderes als
`1`, `U` und leer auf, hat sich entweder das Format geändert oder der
Zellbezug wird falsch gelesen, und beides muss auffallen, statt als stille
Fehlzuordnung durchzulaufen. Eine unbekannte Zellbelegung bricht den Lauf ab.

`manifest.yaml` pinnt URL, SHA-256, Größe, Lizenz (CC-BY 4.0) und beide
Zitate, wie die anderen Pipelines.

## 5. Ingest

```go
// internal/application/syntaxon_distribution_ingest.go (neu)

type SyntaxonDistributionReport struct {
	Written        int // Zeilen in syntaxon_distribution
	Verified       int
	Uncertain      int
	Covered        int      // Syntaxa mit Aussage (Coverage-Zeilen)
	Territories    int
	UnknownSyntaxa []string // Codes der Quelle, die der Index nicht kennt
}

func IngestSyntaxonDistribution(ctx context.Context, repo output.Repository, csvPath, areaCSVPath string) (SyntaxonDistributionReport, error)
```

Läuft nach `IngestSyntaxa` (Teilprojekt A) — die Syntaxon-IDs müssen im Index
stehen, damit `UnknownSyntaxa` etwas bedeutet. Keine hostus-Beteiligung: das
ist eine reine CSV-Quelle, kein Namensauflösungsschritt, und der Ingest bleibt
offline.

`IngestTx` bekommt `UpsertSyntaxonDistribution(syntaxonID string, scheme, code, occurrence string) error` und
`UpsertSyntaxonDistributionCoverage(syntaxonID, scheme string) error`.

Eine Zeile mit einem Code, den der Index nicht kennt, wird **nicht**
geschrieben, in `UnknownSyntaxa` vermerkt und gewarnt; der Lauf geht weiter.
Gemessen ist das genau ein Fall: `CI01E` steht in der Verbreitungsdatei
(EVC-Fassung 3), aber nicht in der gepinnten FloraVeg-Datei. Das ist echter
Fassungsdrift zwischen zwei Quellen, kein Fehler dieser Pipeline — und der
Report benennt ihn, statt ihn wegzurunden.

Der Fassungsunterschied gehört in `docs/reference/measured-index.md`: die
Verbreitungsdatei nennt EVC-Fassung 3 (2024-06-12), die Hierarchie-Datei
trägt Spaltenköpfe mit Stand 2025-06-12. Die gemessene Überdeckung von
1114/1115 ist die belastbare Aussage darüber; die Fassungsnummern allein sind
es nicht.

## 6. Read-API

`SyntaxonDetail` (Teilprojekt B) wächst um ein Feld:

```go
// Distribution ist die Verbreitungsaussage der Quelle. Nil, wenn für dieses
// Syntaxon keine Aussage vorliegt (Coverage fehlt) — dann ist NICHTS über
// sein Vorkommen bekannt, was von "kommt nirgends vor" streng zu trennen ist.
Distribution *SyntaxonDistribution `json:"distribution,omitempty"`

type SyntaxonDistribution struct {
	AreaScheme string   `json:"area_scheme"`
	Verified   []string `json:"verified"`
	Uncertain  []string `json:"uncertain"`
}
```

`verified` und `uncertain` sind Codelisten, sortiert. Absence wird **nicht**
aufgezählt: 136 minus die belegten Codes wäre eine erfundene Liste, und der
Client kennt das Schema über die bestehende Areas-Route.

Zusätzlich nimmt `GET /v1/syntaxa` aus Teilprojekt B einen Parameter
`?area=` (ein `evc-territory`-Code) plus `?include=` mit den Werten
`verified` (Vorgabe) und `verified,uncertain`. Gefiltert wird auf Syntaxa mit
einer entsprechenden Zeile; Syntaxa **ohne Coverage** werden dabei
mitgeführt, nicht entfernt — dieselbe Regel, die `only_in_area` bei den Arten
befolgt: eine Liste, die stillschweigend wegwirft, was sie nicht beurteilen
kann, ist unehrlich saubergemacht. Ein Code, den das Schema nicht kennt, ist
`INVALID_QUERY`.

`GET /v1/info` bekommt unter `index` zwei gemessene Felder:
`syntaxon_area_scheme` und `syntaxa_with_distribution`.

Die bestehende `AreasWithData`-Route und `area`-Tabelle sind schon
schemaparametrisiert und tragen die Territorien ohne Änderung mit; nur der
Aufruf bekommt das zweite Schema.

## 7. Fehlerbehandlung

| Fall | Verhalten |
|---|---|
| Datei fehlt | Ingest läuft weiter, Warnung. Die Verbreitung ist Zusatzinformation, genau wie bei `IngestDistribution` — anders als die Hierarchie in Teilprojekt A, die Primärquelle ist. |
| Blatt „European alliances“ fehlt | Abbruch mit Blattnamen. Nie auf ein anderes Blatt ausweichen. |
| Unbekannte Zellbelegung (nicht `1`/`U`/leer) | Abbruch in der Pipeline, mit Wert, Zeile und Spalte. |
| Slug-Kollision zweier Spaltennamen | Abbruch in der Pipeline, beide Namen genannt. |
| Verbandscode der Quelle nicht im Index | Zeile verworfen, in `UnknownSyntaxa`, Warnung, Lauf geht weiter. |
| Syntaxon mit Coverage, aber ohne belegte Zelle | Gültig: Coverage-Zeile ohne Verbreitungszeilen heißt „geprüft, kommt in keinem Territorium vor“. |
| `?area=` mit unbekanntem Code | `INVALID_QUERY`, nie eine Liste von „kommt nicht vor“. |

## 8. Tests

- `pipelines/evc-distribution/test_xlsx_to_csv.py`: `r`-basierte Lesung mit
  einer künstlich lückenhaften Zeile (der Test, der die positionelle Lesung
  auffliegen lässt), Blattauswahl nach Namen, Slug-Ableitung inklusive
  `_coast`, Kollisionsabbruch, Abbruch bei unbekannter Zellbelegung,
  Summenspalten werden nicht als Territorium gelesen.
- `internal/application/syntaxon_distribution_ingest_test.go`: beide Tabellen
  gefüllt, unbekannter Code verworfen und gemeldet, fehlende Datei nur
  gewarnt, Coverage ohne Vorkommen bleibt erhalten.
- `internal/adapters/http/`: `distribution` fehlt bei einem Syntaxon ohne
  Coverage und ist bei einem mit Coverage vorhanden; `?area=` filtert und
  behält die Unbeurteilbaren; unbekannter Code auf `INVALID_QUERY`.
- Ein Test über den gebauten Index: kein Moos-, Flechten- oder Algenverband
  trägt eine Verbreitungsaussage, und keiner von ihnen erscheint als
  „kommt nicht vor“.

## 9. Prüfbare Zusagen

- Die vier Zustände bleiben unterscheidbar; `absence` und `unknown` werden an
  keiner Stelle zusammengeworfen.
- Kein Moos-, Flechten- oder Algenverband bekommt eine Verbreitungsaussage,
  die die Quelle nicht macht.
- Die Wertemenge der Quellzellen wird bei jedem Pipeline-Lauf gemessen und
  ausgewiesen, nicht vorausgesetzt.
- Die 136 Territorien sind ein eigenes `area_scheme`; keine Antwort rechnet
  sie in WGSRPD um.
- `CI01E` wird namentlich als Fassungsdrift gemeldet, nicht verschwiegen.
- Die Attribution beider Veröffentlichungen steht im Manifest und in
  `docs/reference/`.

## 10. Bewusst außerhalb dieses Specs

- **ISO- oder GPS-Abbildung auf Territorien.** Die Frontend-Seite leitet den
  Gebietscode aus der Position ab; für WGSRPD ist das schon so entschieden
  und gilt hier genauso.
- **Vererbung der Verbreitung nach oben.** „In welchen Territorien kommt
  Klasse CA vor“ wäre die Vereinigung ihrer Verbände — fachlich plausibel,
  aber eine eigene Aussage mit eigener Unsicherheit, und nichts, was aus
  dieser Quelle unmittelbar folgt.
- **Die Kartenbilder** aus der ZIP. situs antwortet mit Daten, nicht mit
  PDF-Karten.
- **Verbreitungsgestützte Gewichtung** von Habitattyp-Kandidaten — das ist
  Scoring und bleibt außerhalb, wie in `CLAUDE.md` festgehalten.
