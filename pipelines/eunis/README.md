# pipelines/eunis

Wandelt die gepinnten EUNIS-/ESy-XLSX-Artefakte in normalisierte CSVs um, die
der Go-Ingest (`situs ingest`, Task 5) liest. Die XLSX-Verarbeitung lebt
bewusst hier, nicht im Go-Binary: eine `.xlsx` ist ein ZIP aus XML, das die
Python-Stdlib liest — situs' Abhängigkeitsliste bekommt dafür keinen
Spreadsheet-Reader (siehe `CLAUDE.md`).

**Nur Python-Stdlib.** Kein `openpyxl`, kein `pandas`, kein `pyyaml` — nur
`zipfile`, `xml.etree.ElementTree`, `csv`, `json`, `argparse`. Python ≥ 3.9.

## Quellen beschaffen

Die XLSX-Dateien werden **nicht** ins Repository eingecheckt
(`pipelines/eunis/artifacts/` ist gitignored). Die exakten URLs, SHA-256-
Prüfsummen und Lizenzen stehen in [`manifest.yaml`](manifest.yaml). Laden:

```bash
mkdir -p artifacts
curl -sSL -A "situs-ingest/0.1 (<Kontakt-Mail>)" \
  "https://sdi.eea.europa.eu/webdav/datastore/public/eea_t_eunis-hab-t_p_2004-on-going_v01_r00/EUNIS%20terrestrial%20habitat%20classification%202021_1%20including%20crosswalks.xlsx" \
  -o artifacts/eunis-2021-including-crosswalks.xlsx

curl -sSL -A "situs-ingest/0.1 (<Kontakt-Mail>)" \
  "https://sdi.eea.europa.eu/webdav/datastore/public/eea_t_eunis-hab-t_p_2004-on-going_v01_r00/EUNIS%20terrestrial%20habitat%20classification%202021_1%20with%20crosswalks%20to%20Annex%20I%20in%20separate%20rows.xlsx" \
  -o artifacts/eunis-2021-annex1-separate-rows.xlsx

curl -sSL -A "situs-ingest/0.1 (<Kontakt-Mail>)" \
  "https://zenodo.org/records/3841729/files/Characteristic-species-combinations.xlsx" \
  -o artifacts/esy-characteristic-species-combinations.xlsx

shasum -a 256 artifacts/*.xlsx   # gegen manifest.yaml prüfen
```

Die EEA-Downloadseite ist eine JS-Single-Page-App ohne stabile Direktlinks;
der obige WebDAV-Pfad ist der von der Datensatz-Metadatenseite (DOI
`10.2909/bfe4c237-e378-4a83-ab21-b3807f96c2e2`) verlinkte
`EEA:FOLDERPATH`-Transfer und war am Pinning-Datum (siehe `manifest.yaml`)
erreichbar. Ändert sich der Pfad, ist die Metadatenseite unter der DOI der
verlässliche Ausgangspunkt.

## Pipeline ausführen

```bash
python3 xlsx_to_csv.py \
  --eunis-xlsx artifacts/eunis-2021-including-crosswalks.xlsx \
  --annex1-xlsx artifacts/eunis-2021-annex1-separate-rows.xlsx \
  --esy-xlsx artifacts/esy-characteristic-species-combinations.xlsx \
  --out-dir out
```

Schreibt nach `out/`: `typologies.csv`, `habitat_types.csv`,
`crosswalks.csv`, `syntaxa.csv`, `habitat_type_syntaxa.csv`,
`species_roles.csv` und `report.json` (die Messungen — Syntaxa-Tiefe,
Qualifier-Werte, Annex-I-Abdeckung; siehe Design-Spec, offene Punkte 1, 2, 5).
Übersprungene, nicht parsbare Zeilen werden gezählt und mit Grund auf
`stderr` protokolliert, nie kommentarlos verworfen. Fehlt einer Quelle eine
Spalte, die ein Parser braucht (umbenannt/verschoben), bricht der Lauf mit
einer klaren Fehlermeldung und Exit-Code 1 ab — ein Schema-Wechsel in der
Quelle darf nie zu leise falschen CSVs führen.

### `report.json` — was jeder Schlüssel zählt

Die acht Schlüssel zählen **zwei unterschiedliche Populationen**; sie sind
keine Ausschnitte derselben Grundgesamtheit:

| Schlüssel | Zählt |
|---|---|
| `habitat_types` | **alle** Zeilen von `habitat_types.csv`, über **alle drei** Typologien (`eunis@2021` + `eunis@2012` + `annex1`) summiert. |
| `max_habitat_level` | das höchste `Level` der eunis@2021-Klassifikationshierarchie selbst (nicht der Crosswalks). |
| `syntaxa_ranks` | die Menge der tatsächlich vorkommenden Syntaxa-Ränge (gemessen pro Namenssuffix). |
| `qualifier_values` | die Menge der tatsächlich vorkommenden Qualifier-Symbole, über Versions- **und** Annex-I-Crosswalk zusammen. |
| `annex1_crosswalks` | die Anzahl der eunis@2021→annex1-Crosswalk-**Zeilen** (nicht Typen). |
| `annex1_qualifier_histogram` | dieselben Zeilen wie `annex1_crosswalks`, aufgeschlüsselt nach Qualifier. |
| `types_with_annex1` | die Anzahl **distinkter eunis@2021-Codes** (nur diese Typologie, i.d.R. Level 3) mit ≥1 Annex-I-Crosswalk. |
| `types_with_annex1_same` | dieselbe Population wie `types_with_annex1`, aber nur Codes mit ≥1 Zeile mit Qualifier `=`. |

`habitat_types` ist also kein Nenner für `types_with_annex1` — Ersteres
läuft über drei Typologien, Letzteres nur über eunis@2021.

## Tests

```bash
python3 -m unittest discover -v
```

Die Tests bauen ihre XLSX-Fixtures **im Speicher** (kleine ZIP/XML-Konstrukte,
siehe `test_xlsx_to_csv.py`) — kein Netzwerk, keine Binärdatei im Repo nötig.

## Bekannte Eigenheiten der Rohdaten (gemessen, nicht angenommen)

- Das Tabellenblatt **"Man-made"** benennt seine Code-/Name-/
  Beschreibungs-Spalten `Code 2018`/`Name 2018`/`Description 2018` statt
  der schlichten Form, die jedes andere Blatt benutzt. Ohne Alias-Auflösung
  fällt der komplette `V`-Zweig (vegetated man-made habitats) unter den
  Tisch — die Pipeline löst `Code`/`Name` explizit über eine Alias-Liste
  auf und deckt das mit einem eigenen Test ab.
- Die EUNIS-Klassifikationshierarchie selbst reicht bis **Level 8**; die
  Crosswalks (Annex I, Syntaxa, EUNIS-2012-Version) sind aber ausschließlich
  auf **Level 3** befüllt — das deckt sich mit der bekannten Deckelung des
  Fundaments.
- Der Annex-I-Qualifier kennt neben `= < > #` auch **`≈`** (2 von 341
  Zeilen in der realen Datei) — ein von der Design-Spec nicht vorgesehenes
  Symbol. Die Pipeline gibt es **verbatim** weiter (kein Silent-Drop, keine
  erfundene Zuordnung); `report.json` zählt es in `qualifier_values`.
- Der Syntaxa-Bezug ist praktisch durchgehend auf **Verband**-Ebene (Suffix
  `-ion`), mit einer einzigen gemessenen Ausnahme auf **Ordnung**-Ebene
  (`Moltkeetalia petraeae`, Suffix `-etalia`). Der Rang wird pro Syntaxon aus
  dem Namenssuffix bestimmt, nicht pauschal auf `alliance` gesetzt.
- Der Code `x` in der Annex-I-Spalte bedeutet „kein Annex-I-Typ" — ein
  Sentinel, keine echte Kennung. Solche Zeilen erzeugen **keine**
  Crosswalk-Zeile (Abwesenheit von Zeilen, nie ein Platzhalter-Code).
- Ein führendes `*` im amtlichen Annex-I-Namen markiert einen **prioritären**
  Lebensraumtyp; die Pipeline liest das in die `priority`-Spalte und
  entfernt das `*` aus dem Namen.
- Einzelne Zeilen bündeln mehrere Entsprechungen in einer Zelle (z. B. zwei
  Annex-I-Codes durch `;` getrennt), obwohl die "separate rows"-Variante das
  eigentlich vermeiden soll. Wo Qualifier-/Code-/Namens-Listen in einer
  Zeile nicht dieselbe Länge haben, wird die Zeile übersprungen und gezählt
  statt geraten zusammengesetzt.
