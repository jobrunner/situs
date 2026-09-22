# pipelines/evc-distribution — Verbreitung der Vegetationsverbände

Wandelt die gepinnte Zenodo-XLSX mit der Verbreitung der europäischen
Vegetationsverbände in drei CSVs um, die `situs ingest` liest (siehe
`docs/superpowers/specs/2026-09-21-syntaxa-verbreitung-design.md`).

**Nur Python-Stdlib**, wie `pipelines/eunis/` und
`pipelines/eurovegchecklist/`: `zipfile`, `xml.etree.ElementTree`, `csv`,
`json`, `re`, `argparse`. Python ≥ 3.9. Kein `openpyxl`.

## Quelle

[Zenodo Record 11580949](https://zenodo.org/records/11580949), Version 2.0
vom 2024-06-12, **CC-BY 4.0**. URL, SHA-256, Größe und **beide** zu
zitierenden Arbeiten stehen in [`manifest.yaml`](manifest.yaml):

- Preislerová et al. (2022) *Applied Vegetation Science* 25: e12642
- Preislerová et al. (2024) *Applied Vegetation Science* 27: e12766

Nicht eingecheckt (`artifacts/` ist gitignored).

## Lauf

```bash
bash pipelines/evc-distribution/build.sh
```

`build.sh` lädt bei Bedarf, prüft die SHA-256 **bei jedem Lauf** (ein
zwischengespeichertes Artefakt einer anderen Fassung würde sonst umgewandelt,
während der Lauf die gepinnte behauptet) und ruft die Umwandlung:

```
out/syntaxon_distribution.csv           syntaxon_id,area_scheme,area_code,occurrence
out/syntaxon_distribution_coverage.csv  syntaxon_id,area_scheme
out/evc_territories.csv                 area_scheme,area_code,name_en
out/report.json
```

Die vier Ausgaben werden **vor** Download und Prüfsummenprüfung gelöscht:
`collect-ingest-input.sh` geht nach Dateipräsenz, ein fehlgeschlagener Lauf
würde sonst die CSVs des vorigen Laufs still in den nächsten Ingest
veröffentlichen. Nach diesem Punkt enthält `out/` das Ergebnis dieses Laufs
oder gar nichts. `artifacts/` bleibt unangetastet — der Download-Cache ist
absichtlich dauerhaft.

Gemessen gegen die gepinnte Datei: **1115** Verbände, **136** Territorien,
**9608** `verified`, **1920** `uncertain`, **11528** Zeilen, 0 Slug-Kollisionen,
**4** übersprungene Summenzeilen, 0 sonstige übersprungene Zeilen, und das
`value_histogram` `{"": 140112, "1": 9608, "U": 1920}`.

## Vier Dinge, die gemessen und nicht angenommen sind

- **Die Zelle wird über ihr `r`-Attribut gelesen.** Excel lässt leere Zellen
  in der XML weg. Bei 136 überwiegend leeren Spalten je Zeile liefert eine
  positionelle Lesung gemessen die Wertemenge
  `{"", "1", "U", "0", "2", …, "86.111"}` statt der tatsächlichen drei Werte,
  weil Summenzellen in Gebietsspalten rutschen.
- **Das Blatt wird nach Namen gewählt.** `European alliances` ist das
  Datenblatt. `Borja tabulka` ist das **erste** Blatt der Mappe und ein
  Arbeitsentwurf (Ländercodes statt Territorien, dazu eine tschechische Notiz
  über noch fehlende Verbände); `Read me` trägt die Legende. Das erste
  Nicht-Legenden-Blatt zu nehmen würde den Entwurf einlesen.
- **Die vier Summen kommen zweimal vor**: als letzte vier Spalten *und* als
  letzte vier Zeilen. Beide werden an derselben Beschriftungsmenge erkannt.
  Ungeprüft wären die Zeilen vier Pseudo-Verbände mit Werten wie `133` und
  `36.666666666666664`.
- **Drei Verbandscodes tragen ein nachgestelltes Leerzeichen**: `JD02B `,
  `JD02C ` und `JE01B `. Ohne `strip()` vor der Musterprüfung erkennt die
  Pipeline nur **1112** statt der tatsächlichen **1115** gültigen Codes — die
  drei mit dem Leerzeichen fallen durch das Muster und verschwänden
  stillschweigend, ebenso aus dem Join gegen die FloraVeg-Hierarchie (1111
  statt 1114 Treffer). Eine Eigenschaft der Quelldatei, kein Programmierstil.

## Wann die Konvertierung abbricht

`xlsx_to_csv.py` scheitert, statt zu warnen, bei einem fehlenden Datenblatt
(`SheetError`), einer fehlenden Meta-Spalte oder einer unbekannten
Spaltenstruktur an der Summengrenze (`HeaderError`), einer Zelle, die weder
`1` noch `U` noch leer ist (`CellValueError`), zwei Spalten auf demselben
`area_code` (`SlugCollisionError`), zwei Zeilen auf demselben Verbandscode
(`AllianceCodeCollisionError`) — und **bei null gültigen Verbänden**
(`EmptyResultError`). Der doppelte Verbandscode ist keine bloße Dublette:
`Code 1` ist die ID, unter der beide Zeilen geschrieben würden, und weil
`syntaxon_distribution` auf `(syntaxon_id, area_scheme, area_code)` schlüsselt,
überlebt in jeder leeren Zelle der zweiten Zeile die Angabe der ersten — das
Ergebnis repräsentiert danach **keine** der beiden Quellzeilen. Welche Zeile
gemeint ist, entschiede sonst die Zeilenreihenfolge; dasselbe Urteil wie beim
doppelten Primärcode in `pipelines/eurovegchecklist/`. Der letzte Fall ist
kein Sonderfall der Sorgfalt:
`build.sh` räumt `out/` vor dem Lauf, und `IngestSyntaxonDistribution`
behandelt vorhandene Dateien als **Ersetzung**. Nur-Kopfzeilen-CSVs würden
also den gesamten Verbreitungs- und Coverage-Bestand des Index löschen und
leer committen. Die Prüfung läuft **vor** dem ersten Schreiben, `out/` bleibt
nach einem gescheiterten Lauf also leer — und der Skip-Pfad des Ingests
(Datei *fehlt* = „noch nichts gepinnt") greift wie bisher.

## Vier Zustände, nicht drei

| Zustand | In der Quelle | Im Index |
|---|---|---|
| `verified` | Zelle `1` | Zeile in `syntaxon_distribution` |
| `uncertain` | Zelle `U` | Zeile in `syntaxon_distribution` |
| `absence` | leere Zelle, **Verband steht in der Tabelle** | keine Verbreitungszeile, **aber** eine Coverage-Zeile |
| `unknown` | **Verband steht nicht in der Tabelle** | keine Coverage-Zeile |

Deshalb die zweite CSV. Die Quelle deckt ausdrücklich nur
*vascular-plant dominated vegetation* ab: gemessen tragen **196** der 1310
FloraVeg-Verbände keine Verbreitungszeile, darunter alle **190** Kryptogamen-
Verbände (137 Moos/Flechte, 53 Algen). Für die ist jede Aussage über jedes
Territorium `unknown` und darf **niemals** als `absence` erscheinen.

`CI01E` (`Campanulo-Nardion`) steht in der Verbreitungsdatei, aber nicht in
der gepinnten FloraVeg-Datei — echter Fassungsdrift zwischen EVC-Fassung 3
(2024-06-12) und der Hierarchiedatei `version_4`, kein Fehler dieser
Pipeline. Der Ingest meldet ihn namentlich.

## Bewusst nicht drin

Die Kartenbilder aus der 82-MB-ZIP (situs antwortet mit Daten, nicht mit
PDF-Karten), eine Abbildung der Territorien auf WGSRPD oder ISO (es gibt
keine geprüfte, also behauptet situs keine), und die vier Summenspalten
(jede ist aus den Zellen nachrechenbar; gemessen weichen 6 der 1115
Summenzellen von der Zellzahl ab, was sie als Quelle für Zahlen
disqualifiziert).

Die Bemerkung über die 6 abweichenden Summenzellen ist gemessen (`FA03A`,
`FA03B`, `FA03C`, `FA03F`, `FA03G` und eine weitere): die Spalte
`Verified occurrences` stimmt dort nicht mit der Zahl der `1`-Zellen
überein. Deshalb liest die Pipeline die Summen nicht, sondern zählt selbst.

## Tests

```bash
cd pipelines/evc-distribution && python3 -m unittest discover -v
```

Baut ihre XLSX-Fixtures selbst — kein Netzwerk, keine Binärdatei im Repo
nötig. Der wichtigste Fall ist `test_a_missing_cell_does_not_shift_the_
following_ones`: er ist der Test, der eine positionelle Lesung auffliegen
lässt.
