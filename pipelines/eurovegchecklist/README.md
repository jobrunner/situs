# pipelines/eurovegchecklist

Wandelt die gepinnte FloraVeg.EU-EuroVegChecklist-XLSX (Mucina et al. 2016 +
Updates, Version 4) in `syntaxa_hierarchy.csv` um, die `situs ingest` als
zweiten Syntaxa-Baustein neben den EUNIS-CSVs liest (siehe
`docs/superpowers/specs/2026-08-30-situs-syntaxa-hierarchie-design.md`).

**Nur Python-Stdlib**, wie `pipelines/eunis/`: `zipfile`,
`xml.etree.ElementTree`, `csv`, `re`, `json`, `argparse`. Python ≥ 3.9.

## Quelle beschaffen

Nicht eingecheckt (`artifacts/` ist gitignored). URL, SHA-256 und Lizenz stehen
in [`manifest.yaml`](manifest.yaml):

```bash
mkdir -p artifacts
curl -sSL -A "situs-ingest/0.1 (<Kontakt-Mail>)" \
  "https://files.ibot.cas.cz/cevs/downloads/floraveg/List_of_European_vegetation_units_version_4.xlsx" \
  -o artifacts/List_of_European_vegetation_units_version_4.xlsx
shasum -a 256 artifacts/*.xlsx   # gegen manifest.yaml prüfen
```

## Pipeline ausführen

```bash
mkdir -p out
python3 xlsx_to_csv.py \
  --xlsx artifacts/List_of_European_vegetation_units_version_4.xlsx \
  --out-dir out
```

Schreibt `out/syntaxa_hierarchy.csv` (`code,rank,name,author,parent_code`) und
`out/report.json` (Klassen/Ordnungen/Verbände/übersprungene Zeilen). Gemessen
gegen die reale, am 2026-08-30 gepinnte Datei (siehe `manifest.yaml`): 150
Klassen, 381 Ordnungen, 1310 Verbände, 1841 Zeilen insgesamt, 0 übersprungene
Zeilen — deckt sich exakt mit dem Design-Spec-Spike (Zeile 20-21).

`rank` und `parent_code` werden **ausschließlich** aus dem Code-Muster
abgeleitet (`AA` Klasse → `AA01` Ordnung → `AA01A` Verband, Elternteil durch
Suffix-Kürzung), nie aus dem Namen geraten. `author` ist eine direkte Kopie von
FloraVegs eigener Spalte, nie eine Textzerlegung eines Kombi-Strings.

### Reale Spaltenbelegung (gemessen, nicht angenommen)

Die echte Kopfzeile weicht von einem naiven `Code/Name/Author`-Schema ab:

- `Code` trägt zusätzlich einen alternativen/historischen Code in Klammern,
  z. B. `AA01A (KOB-01A)`. Der Parser extrahiert davon nur den führenden,
  primären Code (`AA01A`) — der Klammerteil hat in den fünf CSV-Spalten keinen
  Platz und wird verworfen, nie erraten.
- Name und Autor liegen doppelt vor: als ursprüngliche EVC-Fassung (Mucina et
  al. 2016) und als aktualisierte Fassung — in der gepinnten Datei tragen
  deren Spaltenköpfe selbst das Datum `EVC, version 2025-06-12`.
  `_HEADER_ALIASES` in `xlsx_to_csv.py` bevorzugt diese aktualisierten
  Spalten und fällt nur auf die Original-Spalten zurück, falls ein älterer
  Export sie nicht mitführt.

## Tests

```bash
python3 -m unittest discover -v
```

Baut ihre XLSX-Fixtures im Speicher — kein Netzwerk, keine Binärdatei im Repo
nötig.
