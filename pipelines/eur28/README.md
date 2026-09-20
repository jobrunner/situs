# `pipelines/eur28` — Arbeitsmaterial aus dem Interpretationshandbuch

Zerlegt das *Interpretation Manual of European Union Habitats, EUR 28* in eine
CSV je Anhang-I-Lebensraumtyp. Diese CSV wird **nicht ingestiert**: sie ist die
Grundlage, aus der die von situs verfassten Anhang-I-Beschreibungen entstehen,
und macht deren Ableitung nachprüfbar.

## Quelle

Europäische Kommission, GD Umwelt (April 2013), die amtliche Auslegung der
Lebensraumtypen des Anhangs I der FFH-Richtlinie. Nachnutzung nach Beschluss
2011/833/EU mit Quellenangabe.

Der historische Link der Kommission
(`ec.europa.eu/environment/nature/legislation/habitatsdirective/docs/`)
antwortet heute mit 404. `build.sh` lädt die Datei deshalb aus dem **Central
Data Repository der EEA** (`cdr.eionet.europa.eu`), also aus EU-Infrastruktur
statt von einem fremden Spiegel; die Kopie des spanischen Umweltministeriums
wurde als bitgleich geprüft (dieselbe SHA-256). Gepinnt wird über die
Prüfsumme, nicht über die URL allein.

## Warum am `PAL.CLASS.`-Anker geparst wird

Jeder Eintrag hat dieselbe Form:

```
CODE     Name des Lebensraumtyps (kann umbrechen)
PAL.CLASS.: 35.1, 36.31

1)  Definition
2)  Plants: … Animals: …
3)  Corresponding categories, Untertypen
```

Die Kopfzeile ist unbrauchbar als Anker: sie bricht um, ist je Eintrag anders
eingerückt, und ihre Code-Spalte lässt sich geometrisch nicht abtrennen. Die
Zeile `PAL.CLASS.` dagegen steht **genau einmal je Lebensraumtyp**. Von ihr aus
werden Kopfzeile (rückwärts) und Abschnitte (vorwärts) gelesen.

Zwei Eigenheiten des Dokuments behandelt der Konverter ausdrücklich:

- Die Seite **„Explanatory Notes"** zeigt einen vollständigen Musterteintrag
  (`2140`) mit Randbemerkungen. Er parst wie ein echter Eintrag und würde
  `2140` doppelt liefern; erkannt wird er an seinen Randbemerkungen.
- Die Codes enden **nicht immer auf einer Ziffer**: neben `1110` und `62C0`
  gibt es `91AA`, `91BA`, `91CA`.

## Lauf

```bash
bash pipelines/eur28/build.sh
```

Ergebnis: `output/annex1_source.csv`
(`typology_id,code,name_en,pal_class,definition_en,species_en,categories_en`)
und `output/report.json`. Am gepinnten Stand gemessen: **233** Einträge, alle
mit Definition, 224 mit Artenliste, 108 mit korrespondierenden Kategorien,
keine doppelten Codes. Das entspricht der Zahl der Anhang-I-Codes in EUR-Lex;
der Index führt davon 205.

## Was daraus wird

Die veröffentlichten Beschreibungen entstehen aus diesem Material **plus** der
über den Crosswalk zugeordneten EUNIS-Beschreibung und den Artenrollen und
Syntaxa des Index. Sie tragen `provenance: situs`: situs hat den Wortlaut
verfasst, keine Behörde steht dafür ein. Der amtliche Wortlaut selbst wird
nicht ausgeliefert, weil er eine Abgrenzungsvorschrift für die Rechtsanwendung
ist und nicht die Beschreibung, die im Gelände weiterhilft.

Die `PAL.CLASS.`-Codes stehen in der CSV, gehen aber **nicht** in den Index:
sie dienen beim Verfassen dazu, die regionalen Ausprägungen eines Typs zu
erkennen, und werden von keiner Route gebraucht.
