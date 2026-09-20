# `pipelines/floraveg-factsheets` — Beschreibungen der EUNIS-Habitattypen

Zieht aus den *EUNIS Habitat Factsheets* die Beschreibung je Habitattyp und
schreibt sie als CSV, die `situs ingest` liest. Sie ist das, was
`GET /v1/habitat-type/{typology}/{code}` als `description` ausliefert.

## Quelle

`Habitat-factsheets-EUNIS-habitats-2021-06-01.pdf` von FloraVeg.EU, verteilt
mit dem Expertensystem EUNIS-ESy (Zenodo, doi:10.5281/zenodo.4812736,
CC-BY-4.0). Zitierpflicht: Chytrý et al. (2020), *Applied Vegetation Science*
23: 648–675, https://doi.org/10.1111/avsc.12519.

`build.sh` pinnt das Dokument über seine **SHA-256-Summe**, nicht nur über die
URL: der Dateiname trägt zwar eine Version, der Server kann den Inhalt aber
ersetzen, und ein stillschweigend geänderter Beschreibungssatz ist genau die
Drift, die diese Prüfung abfangen soll.

## Warum `pdftohtml` und nicht `pdftotext`

Bei **25** Factsheets bricht die Titelzeile um, und ihre Fortsetzung ist in
**Fließtextgröße** gesetzt (gemessen). Rein geometrisch zählt sie damit zur
Beschreibung — `MA223` begänne mit „and sedge bed …“. `pdftohtml -xml`
markiert Titel **und** Fortsetzung als `<b>`; damit ist die Trennung exakt.
Die Textknoten kommen unsortiert, deshalb werden sie zuerst in Leseordnung
gebracht: sonst sammelt die Beschreibung die Artentabelle derselben Seite ein.

`pdftohtml` gehört zu **poppler** und ist ein externes Kommandozeilenwerkzeug,
keine Python-Bibliothek — die Stdlib-only-Regel für Pipelines bleibt intakt.
`build.sh` prüft die Verfügbarkeit vorab und nennt die Installationsbefehle,
statt einen Traceback zu zeigen.

## Lauf

```bash
bash pipelines/floraveg-factsheets/build.sh
cp pipelines/floraveg-factsheets/output/habitat_descriptions.csv "$CSV_DIR/"
```

Ergebnis: `output/habitat_descriptions.csv`
(`typology_id,code,description_en`) und `output/report.json` mit den
**gezählten** Zahlen. Am gepinnten Stand: **277** Factsheets, alle mit
Beschreibung.

## Zwei Eigenheiten der Quelle, die der Konverter behandelt

- **Angeklebte Gruppen-Vorspanne.** Bei sechs Factsheets hängt der
  Einleitungstext der *folgenden* EUNIS-Obergruppe ohne Trennzeichen am Ende
  der Beschreibung („… before tree cover returns.Non-coastal habitats on
  substrates …“). Ein strukturelles Signal dafür gibt es nicht, deshalb stehen
  die Vorspanne wörtlich in `GROUP_INTROS` und werden dort abgeschnitten; der
  Report nennt die betroffenen Codes unter `group_intro_cut`.
- **Sonstige Klebungen** (Satzende direkt vor einem Großbuchstaben) werden im
  Report als `suspicious_joins` gemeldet, aber **nie** automatisch geschnitten:
  `U22` hat eine solche Stelle mitten im eigenen Text, und ein Schnitt dort
  würde echte Beschreibung verlieren.

## Nur Level 3

Das Dokument beschreibt ausschließlich EUNIS-**Level 3** (plus `Q3` auf
Level 2, dessen „Beschreibung“ der Hinweis ist, dass das Expertensystem `Q31`
und `Q32` nicht trennen konnte). Tiefere Level enthält es nicht — es gibt
dafür keine Quelle, und situs erbt eine Beschreibung **nicht** nach unten.

Von den 277 Factsheets führt der Index 264 als Habitattyp; die übrigen 13
(11 marine `MA*`-Codes, `N23`, `N24`) verwirft der Ingest und zählt sie.
