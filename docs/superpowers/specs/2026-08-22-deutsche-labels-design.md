# Deutsche Labels: EUR-Lex und situs-Übersetzungen — Design-Spec

Stand: 2026-08-22.

**Ziel:** situs liefert deutsche Namen für Habitattypen — amtlich, wo eine
amtliche Quelle existiert, und selbst verfasst, wo keine existiert. Und zwar so,
dass ein Client die beiden Fälle **nie verwechseln kann**.

**Anlass.** Der Overlay-Mechanismus ist seit dem Fundament gebaut und getestet,
aber keine deutsche Quelle war gepinnt: `Localizations` und `DerivedLabels` sind
gemessen 0. Die Habitattyp-Routen liefern also ausschließlich englische Namen,
und eine Exkursions-App in Unterfranken zeigt „Low and medium altitude hay
meadow" statt „Glatthaferwiese".

## Ausgangslage, gemessen

Alle Zahlen aus dem echten Index (`situs.sqlite`, Stand 2026-08-20), nicht aus
der Spec übernommen:

| Gegenstand | Anzahl |
|---|---|
| Annex-I-Typen | 205 (davon 52 prioritär) |
| EUNIS-Typen gesamt | 7732 |
| **EUNIS-Typen auf Level 3** | **270** |
| Syntaxa | 1050 |
| Localizations | **0** |

Crosswalks nach `annex1`, nach Qualifier:

| Qualifier | Anzahl |
|---|---|
| `#` | 137 |
| `>` | 110 |
| `<` | 61 |
| **`=`** | **29** |
| `≈` | 2 |

184 distinkte EUNIS-Codes haben *irgendeinen* Crosswalk nach `annex1`, aber nur
**29** ein `=`. Da die Ableitungsregel ausschließlich `=` zulässt (bestehende
Invariante), deckt der vorhandene Mechanismus rund 11 % der Level-3-Typen ab.

### Was EUR-Lex tatsächlich hergibt

Geprüft an der deutschen konsolidierten Fassung, CELEX `01992L0043-20130701`:
**Anhang I ist vollständig übersetzt** — jeder Typ hat einen deutschen Namen,
auch solche, die es in Deutschland nicht gibt. Belegbeispiele:

| Code | Deutscher Name (amtlich) |
|---|---|
| 1150 | Lagunen des Küstenraumes (Strandseen) |
| 5330 | Thermo-mediterrane Gebüschformationen und Vorwüsten |
| 9370 | Palmhaine von Phönix |

Damit ist die Arbeitsannahme des Auftrags **widerlegt**: es ist nicht so, dass
nur die in Deutschland vorkommenden Typen amtlich übersetzt sind. Die Richtlinie
erscheint in allen Amtssprachen, der Anhang wird mitübersetzt.

**Wo die Annahme trifft:** national gibt es eine *zweite*, abweichende
Benennung. Das BfN führt für die **93 der 231** Anhang-I-Typen, die in
Deutschland vorkommen, eigene Bezeichnungen, die von der wörtlichen
Richtlinien-Übersetzung abweichen und das sind, was Praktiker kennen. Diese
Ebene bleibt **bewusst draußen** (siehe „Bewusst nicht enthalten").

## Umfang

| Ebene | Anzahl | Provenienz | Quelle |
|---|---|---|---|
| Annex I | 205 | `official` | `eur-lex:31992L0043` |
| EUNIS L3 **mit** `=`-Crosswalk | 29 | `derived` | vorhandener Mechanismus |
| EUNIS L3 **ohne** `=` | **241** | `situs` | von situs verfasst |

Die 241 sind genau 270 − 29: **wo eine Ableitung existiert, wird nichts
erfunden.** Damit gibt es je Typ nur einen Kandidaten und keinen Konflikt
zwischen Ableitung und Erfindung.

Nicht enthalten: EUNIS-Ebenen 1, 2 und 4+ (bleiben englisch), Syntaxa
(lateinische Namen werden nicht übersetzt), und Beschreibungen — `habitat_type`
hat **keine** Beschreibungsspalte, es gibt also nichts zu übersetzen. Deutsche
Beschreibungen wären zuerst eine Ingest-Erweiterung und ein eigener Schnitt.

## Teil 1 — Beschaffung

### Neue Pipeline `pipelines/eurlex/`

bash + `python3`, **stdlib only**, genau wie `pipelines/eunis/`. Sie holt die
CELEX-Fassung einmal, extrahiert Code → deutscher Name, und schreibt

- `localizations.csv` — der Ingest-Vertrag, wie er schon existiert
- `report.json` — die Messwerte (siehe „Was gemessen wird")

Der Go-Ingest liest **nur CSV**; die Extraktion bleibt außerhalb des Binaries.
Das hält die erlaubte Bibliotheksliste unverändert.

**Verworfene Alternativen.** Nur das extrahierte CSV committen, ohne Pipeline:
nicht reproduzierbar, und eine Änderung der Quelle wäre nicht nachvollziehbar.
Zur Ingest-Zeit bei EUR-Lex holen: macht den Ingest von der Verfügbarkeit eines
Rechtstext-Portals abhängig, und gepinnte Artefakte sind die Konvention dieses
Projekts.

### Der gepinnte Bezug

CELEX `01992L0043-20130701` (konsolidierte Fassung, nach dem Beitritt
Kroatiens). Die Kennung steht in der Pipeline als Konstante und wandert in jede
erzeugte Zeile als `source = eur-lex:31992L0043`. Ein Wechsel der Fassung ist
damit ein sichtbarer Diff, keine stille Verschiebung.

## Teil 2 — Die 241 verfassten Übersetzungen

### Versionierte Quelldatei

`data/localizations-de-situs.csv`, handgepflegt und in PRs zeilenweise
diffbar. Die Pipeline führt sie mit der EUR-Lex-Extraktion zu **einer**
`localizations.csv` zusammen: ein Ingest-Vertrag, und der verfasste Anteil
bleibt als eigene Datei sichtbar.

Spaltenschema wie bisher
(`entity_type,entity_key,lang,field,value,source,provenance`);
`entity_key` ist `HabitatTypeKey.String()`, also `eunis@2021:R22`.

### Zwei Felder: treu und geläufig

`field = name` trägt die **treue** Übersetzung des EUNIS-Namens, mit seinen
biogeografischen Zusätzen. `field = vernacular` trägt den **etablierten**
deutschen Begriff — aber nur, wo einer existiert *und* wirklich denselben Umfang
bezeichnet.

```csv
habitat_type,eunis@2021:R22,de,name,"Tief- und mittelmontane Mähwiese",situs@<version>,situs
habitat_type,eunis@2021:R22,de,vernacular,"Glatthaferwiese",situs@<version>,situs
```

**Wo das deutsche Konzept enger ist als der EUNIS-Typ, entfällt die
`vernacular`-Zeile** — sie wird nicht mit leerem Wert geschrieben. Beispiel:
`R12 Cryptogam- and annual-dominated vegetation on siliceous rock outcrops` hat
keinen etablierten Kurzbegriff, der denselben Umfang trägt; dort gibt es nur
`name`.

Das ist dieselbe Haltung wie beim dreiwertigen `in_area`: eine Liste, die
verschweigt, was sie nicht beurteilen kann, ist unehrlich sauber.

### Was „treu" heißt

Die `name`-Zeile ist am englischen Original nachprüfbar: wer beide nebeneinander
legt, muss die Zuordnung ohne Fachwissen bestätigen können. Biogeografische
Qualifikatoren („Temperate and submediterranean", „Atlantic and Baltic") werden
**mitübersetzt**, nicht weggelassen — sie sind Teil der Typdefinition.

### Wie die Richtigkeit gesichert wird

Ein falscher deutscher Name ist **schlimmer als ein englischer**: er sieht
autoritativ aus und wird nicht hinterfragt. Deshalb gehört zur verfassten Datei
eine Kontrollspalte, die nicht in den Index wandert:

- Die Datei trägt je Zeile den **englischen Ausgangsnamen** als Kommentarspalte.
  Ein Reviewer sieht Original und Übersetzung nebeneinander, ohne den Index
  abfragen zu müssen.
- Die 241 Zeilen landen in **thematischen Gruppen** (Grasland, Wald, Küste,
  Moor, Fels …), nicht in Code-Reihenfolge. Wer Grasland prüft, prüft alle
  Graslandtypen zusammen und sieht Inkonsistenzen in der Terminologie.
- Ein `vernacular`-Eintrag ist die riskante Zeile. Wo Zweifel besteht, ob der
  geläufige Begriff denselben Umfang trägt, **entfällt er** — Zweifel wird nicht
  zugunsten der Lesbarkeit aufgelöst.

Die fachliche Abnahme liegt beim Projekteigner; situs kann sie nicht ersetzen.
Der Plan sieht die Datei deshalb als eigenen, prüfbaren Schritt vor, getrennt
vom Mechanismus — der Mechanismus kann grün sein, bevor eine einzige Übersetzung
abgenommen ist.

## Teil 3 — Provenienz

### Vierter Wert

`provenance` wird von drei auf vier Werte erweitert:

| Wert | Bedeutung |
|---|---|
| `official` | Eine amtliche Quelle nennt diesen Namen |
| `curated` | Ein Mensch hat bewusst korrigiert |
| `derived` | Mechanisch aus einem amtlichen Namen über `=` gewonnen |
| **`situs`** | **situs hat übersetzt. Keine externe Quelle steht dahinter.** |

### Auflösungsreihenfolge

```
official  >  curated  >  derived  >  situs
```

`curated` steht über `derived`, weil dort ein Mensch bewusst eingegriffen hat.
`situs` ist der schwächste Anspruch und verliert gegen alles.

### Die harte Invariante

**`situs` darf niemals Ableitungssaat sein.** `officialOrCuratedName` wählt
heute den Wert, aus dem eine EUNIS-Bezeichnung abgeleitet wird, und lässt
ausschließlich `official` und `curated` zu — Abgeleitetes darf nicht erneut Saat
sein. `situs` tritt dieser Liste **nicht** bei.

Der Grund ist nicht formal: würde eine von situs erfundene Bezeichnung eine
Ableitung speisen, träge das Ergebnis `derived` und sähe damit
nachvollziehbar aus, obwohl am Anfang der Kette eine Erfindung steht. Das ist
Provenienz-Wäsche, und sie muss durch einen eigenen Test verhindert sein, nicht
durch einen Kommentar.

## Teil 4 — API

`name_de` wird von einem String zu einem Objekt:

```json
"name_de": {
  "value": "Tief- und mittelmontane Mähwiese",
  "vernacular": "Glatthaferwiese",
  "provenance": "situs",
  "source": "situs@0.3.0"
}
```

`vernacular` fehlt, wenn es keinen passenden gibt. Das ganze Objekt fehlt, wenn
es kein deutsches Label gibt.

**Das ist eine Breaking Change** an `HabitatTypeSummary`: `name_de` war ein
String, und das Geschwisterfeld `name_de_provenance` entfällt darin. Sie ist
vertretbar, weil beide Felder `omitempty` sind und bei 0 Localizations heute
*immer* fehlen — es gibt keinen Konsumenten, der brechen könnte.

Die Form ist der flachen Alternative (`name_de`, `name_de_provenance`,
`name_de_source`, `name_de_vernacular`) bewusst vorgezogen worden: im Objekt ist
die Zusammengehörigkeit strukturell erzwungen. Ein Client kann `value` nicht
lesen, ohne `provenance` daneben zu sehen — bei vier flachen Feldern kann er es.
Genau darum geht es bei diesem Schnitt.

Betrifft: `internal/ports/input/services.go`, beide byte-identischen
OpenAPI-Kopien und den Routen↔Spec-Contract-Test.

## Was gemessen wird

Die Pipeline **berichtet, statt anzunehmen** — `report.json`:

1. Wie viele Anhang-I-Codes EUR-Lex liefert.
2. **Die Differenz 205 ↔ 231.** Das BfN nennt für Anhang I 231 Typen, unser
   Index führt 205. Diese Lücke ist **ungeklärt** und muss als Zahl auf dem
   Tisch liegen: Codes aus EUR-Lex ohne `habitat_type`-Zeile, und Typen im Index
   ohne amtlichen Namen, jeweils mit Beispielen.
3. Wie viele der 29 Ableitungen tatsächlich zünden.
4. Wie viele der 241 verfassten Zeilen einen `vernacular`-Partner haben.

**Widerspricht die Messung diesem Design, wird angehalten und berichtet** — das
Design wird nicht still angepasst. Die 205/231-Frage ist ausdrücklich *nicht*
vorab entschieden; sie kann einen eigenen Folgeschnitt auslösen.

## Lizenz

EU-Rechtstexte sind nach Beschluss 2011/833/EU mit Quellenangabe nachnutzbar.
Die Angabe steht pro Zeile im `source`-Feld (`eur-lex:31992L0043`) und zusätzlich
in `docs/reference/`. Die verfassten Zeilen tragen `situs@<version>` — die
Quellenangabe ist dort situs selbst, und das ist der Punkt.

## Prüfbare Zusagen

Diese Sätze müssen als Test existieren, nicht als Kommentar:

1. Eine `situs`-Localization wird **nie** als Ableitungssaat verwendet.
2. Für die 29 EUNIS-Typen mit `=`-Crosswalk entsteht **keine** `situs`-Zeile.
3. `official` gewinnt gegen `curated` gegen `derived` gegen `situs`.
4. Eine `vernacular`-Zeile ohne zugehörige `name`-Zeile ist ein Fehler.
5. Fehlt ein deutsches Label, fehlt `name_de` ganz — kein leeres Objekt.
6. `name_en` bleibt unverändert die Identität; der Overlay ersetzt nie.

## Bewusst nicht enthalten

- **Die BfN-Benennung.** Nur als PDF verfügbar, Nachnutzungslizenz unklar. Weil
  der Primärschlüssel der `localization` `source` einschließt, lässt sich eine
  nationale Ebene später ohne Schemaänderung nachlegen — dann ist auch zu
  entscheiden, ob sie für die 93 deutschen Typen gegen die
  Richtlinien-Übersetzung gewinnen soll.
- **EUNIS-Ebenen 1, 2, 4+.** Über 7400 Namen; in dieser Menge ist keine Qualität
  zusicherbar, die einer Fachprüfung standhält.
- **Deutsche Beschreibungen.** Erst eine Ingest-Erweiterung
  (`habitat_type.description_en`), dann eine Übersetzungsfrage.
- **Andere Sprachen.** Das Schema trägt `lang`, aber gepinnt wird nur `de`.
- **Syntaxa-Namen.** Lateinische Nomenklatur wird nicht übersetzt.
