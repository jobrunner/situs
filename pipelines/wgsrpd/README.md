# pipelines/wgsrpd — WGSRPD-Gebietsnamen

Macht aus der TDWG-Gebietstabelle die CSV, aus der `situs ingest` die Namen der
Verbreitungsgebiete liest. Diese Namen sind das, was `GET /v1/areas` einem
Client neben dem Code liefert — ohne sie kennt der Index nur `GER`, `AUT`, `FOR`.

## Quelle

`tdwg/wgsrpd`, Datei `109-488-1-ED/2nd Edition/tblLevel3.txt` — die
*World Geographical Scheme for Recording Plant Distributions*, 2. Auflage
(Brummitt 2001). Das Verzeichnis `level3/` desselben Repositories enthält nur
Shapefiles; die Namen stehen ausschließlich in den Tabellen der 2. Auflage.

`build.sh` lädt **einen gepinnten Commit**, nie `master`: die Namen benennen
die Codes, die der Index bereits führt, und dürfen sich nicht unter einem
Neubau ändern, ohne dass jemand den Pin bewusst hochzieht.

Das Format ist ein Datenbank-Export: `*`-getrennt, CRLF, **cp1252**-kodiert
(`Føroyar`, `Galápagos`, `Québec` tragen die Nicht-ASCII-Bytes). `convert.py`
liest ihn mit der Python-Standardbibliothek — kein XLSX, keine
Fremdabhängigkeit — und schreibt UTF-8.

## Lauf

```bash
bash pipelines/wgsrpd/build.sh
```

Ergebnis: `output/wgsrpd_areas.csv` (`area_scheme,area_code,name_en`) und
`output/report.json` mit den **gezählten** Zahlen des Laufs. Gemessen am
gepinnten Stand: **369** Gebiete, 0 übersprungene Zeilen.

Für den Ingest gehört die CSV neben die übrigen in das `--csv-dir`:

```bash
cp pipelines/wgsrpd/output/wgsrpd_areas.csv "$CSV_DIR/"
situs ingest --csv-dir "$CSV_DIR"
```

Fehlt sie, läuft der Ingest normal durch (`"AreaNames": {"Areas": 0}` im
Report) und die Gebietscodes bleiben namenlos.

## Bewusst nicht drin

Level 1 (Kontinent) und Level 2 (Region) der Hierarchie, die ISO-Codes und die
Gazetteer-Tabelle. situs führt genau ein Gebietsschema (`wgsrpd_l3`), und für
eine Hierarchie oder eine ISO-Abbildung gibt es keinen Leser — das Frontend
leitet den Level-3-Code aus der GPS-Position ab.
