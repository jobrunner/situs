# `pipelines/eurlex` — amtliche deutsche Anhang-I-Namen

Normalisiert die deutsche Fassung der FFH-Richtlinie zu der
`localizations.csv`, die der Go-Ingest liest. Wie `pipelines/eunis` bleibt das
Parsen außerhalb des Binaries: bash + `python3`, **stdlib only**.

## Lauf

```bash
bash pipelines/eurlex/fetch.sh eurlex-de.xhtml
sqlite3 situs.sqlite "SELECT code FROM habitat_type WHERE typology_id='annex1'" > annex1-codes.txt
python3 pipelines/eurlex/extract.py eurlex-de.xhtml \
  --index-codes annex1-codes.txt -o localizations.csv -r report.json
```

## Zusammenführung mit den verfassten Übersetzungen

```bash
sqlite3 situs.sqlite "SELECT h.code FROM habitat_type h
  WHERE h.typology_id='eunis@2021' AND h.level IN (1,2,3)
  AND NOT EXISTS (SELECT 1 FROM habitat_type_crosswalk c
    WHERE c.from_code=h.code AND c.to_typology='annex1' AND c.qualifier='=')" > expected.txt
sqlite3 -separator '\t' situs.sqlite \
  "SELECT code,name_en FROM habitat_type WHERE typology_id='eunis@2021' AND level IN (1,2,3)" > index-names.tsv
python3 pipelines/eurlex/merge.py data/localizations-de-situs.csv \
  --version "$(cut -d' ' -f1 VERSION)" --official localizations.csv \
  --expected-codes expected.txt --index-names index-names.tsv -o localizations.csv
```

Die Level-Menge ist **1 bis 3**, nicht nur 3: Level 1 und 2 tragen keinen
`=`-Crosswalk nach `annex1` (gemessen: keiner der 49 Codes), die Ableitung
erreicht sie also nie, und ohne verfasste Zeilen bliebe die Ebene englisch, die
in der App als Gruppenknopf zuerst sichtbar ist. Gemessen am 2026-09-21: **290**
verfasste Typen, **394** Zeilen, davon 104 `vernacular`.

`--version` will **nur die Nummer**, nicht die ganze `VERSION`-Zeile: dahinter
steht der release-please-Marker `# x-release-please-version`, und `merge.py`
stempelt den Wert unverändert als `situs@<version>` in die `source`-Spalte. Der
Prod-Index vom 2026-08-31 trägt deshalb `situs@0.2.0 # x-release-please-version`
— hier stand früher `$(cat VERSION)`, und drei Wochen lang ist es niemandem
aufgefallen. `merge.py` weist so einen Wert inzwischen zurück, statt ihn zu
schreiben oder still zurechtzuschneiden: ein stillschweigend repariertes
Kommando bleibt für den nächsten Lauf kaputt.

`merge.py` schreibt **nichts**, wenn die verfasste Datei ihre Zusagen bricht:
eine fehlende oder überzählige Code-Menge, ein doppelter Code, ein
`vernacular_de` ohne `name_de`, oder ein `name_en`, das vom Index abweicht. Die
letzte Prüfung normalisiert geschützte Leerzeichen — die EEA-Quelldaten führen
in mindestens einem `name_en` ein `U+00A0` (`V31`), und ein Byte-Vergleich würde
dort einen Übersetzungsfehler melden, wo nur ein unsichtbares Zeichen von
upstream steht.

## Gepinnte Quelle

CELEX **`01992L0043-20130701`** — konsolidierte Fassung nach dem Beitritt
Kroatiens. Bezogen über das Cellar-Repository des Amts für Veröffentlichungen
(`publications.europa.eu/resource/celex/…` mit Content Negotiation); das Web-UI
von EUR-Lex antwortet einem einfachen `curl` mit HTTP 202 und leerem Rumpf.

Nachnutzung nach **Beschluss 2011/833/EU** mit Quellenangabe. Jede erzeugte
Zeile trägt `source = eur-lex:31992L0043`.

### Transport und Integrität

Das Schema auf `https` zu setzen genügt **nicht**: die CELEX-Ressource antwortet
303, und ihr `Location` zeigt auf plain **http** — `curl -L` stuft also
stillschweigend ab. `fetch.sh` löst die Umleitung deshalb in einem Schritt auf,
erzwingt `https` auf dem Ziel (dieselbe URL liefert byte-identischen Inhalt über
TLS, geprüft) und holt den Rumpf mit `--proto '=https'`.

Transportsicherheit allein sagt aber nicht, dass wir das *gepinnte* Dokument
haben. `EXPECT_SHA256` tut das: der Pin schlägt bei Manipulation **und** bei
stiller Neuveröffentlichung durch die EU an — genau der Sinn des Pinnens. Eine
Änderung soll ein sichtbares Ereignis sein, das jemand bewusst nachzieht.

## Gemessene Struktur

```html
<p class="dlist-term">9370</p>
<p class="dlist-definition">* Palmhaine von <span class="italics">Phönix</span></p>
```

Drei Dinge, die gemessen und nicht angenommen sind:

- **Codes sind vierstellig, aber nicht vierziffrig.** 44 der 233 Einträge sind
  alphanumerisch (`21A0`, `40A0`, `62A0`). Ein `\d{4}`-Filter verwirft sie
  stillschweigend.
- **Nur ein führender Stern** markiert prioritäre Typen und entfällt; die
  Priorität führt `habitat_type.priority` schon. Ein Stern im Text gehört zum
  Namen (`Machair (* in Irland)`).
- **Dieselbe `dlist`-Struktur tragen die späteren Anhänge** für Artenlisten. Die
  Extraktion wird deshalb auf den Bereich `ANHANG I` … `ANHANG II` begrenzt —
  sonst landet ein Wolf zwischen den Habitattypen.

## Bericht

`report.json` nennt beide Richtungen. Die zweite ist die wichtige: ein
indexierter Typ **ohne** amtlichen Namen darf nicht dadurch auffallen, dass
jemand in der App ein englisches Label sieht.
