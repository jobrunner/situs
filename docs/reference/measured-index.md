# Gemessene Kennzahlen des Index

Alle Zahlen auf dieser Seite sind **am gepinnten Datenstand gemessen**, nicht
geschätzt und nicht aus der Spec übernommen. Sie beantworten die offenen Punkte
1, 2, 3 und 5 der Design-Spec
(`docs/superpowers/specs/2026-08-18-situs-foundation-design.md`).

!!! warning "Eine Ausnahme: die Crosswalk-Gesamtzahl ist vorhergesagt, nicht gemessen"

    Gemessen wurden **4305 ingestierte + 2 übersprungene** Crosswalk-Zeilen. Die
    2 übersprungenen waren die `≈`-Zeilen, die der Ingest damals verwarf. Seit
    `≈` ein vollwertiger Qualifier ist, sind **4307 ingestiert und 0
    übersprungen** zu erwarten — der reale Ingest wurde dafür aber **absichtlich
    nicht erneut ausgeführt**. Die 4307 unten sind also die erwartete, nicht die
    beobachtete Zahl; sie ist beim nächsten echten Lauf zu bestätigen. Ebenso
    kann die Zahl der Typen mit Anhang-I-Bezug dann um bis zu 2 auf höchstens 186
    steigen. Alle übrigen Zahlen auf dieser Seite sind unverändert gemessen; die
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
gitignoriert — deshalb diese Seite. Die Resolution Rates entstehen erst beim
Go-Ingest (`situs ingest`, siehe `../how-to/ingest.md`), weil sie hostus
brauchen.

## Umfang des Index

| Größe | Gemessen |
|---|---|
| Habitattypen (über `eunis@2021` + `eunis@2012` + `annex1`) | **7937** |
| Maximales Level der eunis@2021-Klassifikationshierarchie | **8** |
| Crosswalk-Zeilen (Versions- **und** Anhang-I-Crosswalk) | **4307** (vorhergesagt, siehe Warnung oben) |
| Syntaxa | **1050** |
| Habitattyp↔Syntaxon-Verknüpfungen | **1283** |
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

Vorkommende Ränge: **`alliance`** (Verband) und **`order`** (Ordnung). Der
Verband ist praktisch durchgehend das Ende; gemessen gibt es genau **eine**
Ausnahme auf Ordnungs-Ebene (`Moltkeetalia petraeae`). **Assoziationen kommen
nicht vor** und sind in keiner freien paneuropäischen Quelle verfügbar — das ist
die dokumentierte Decke, kein Versäumnis.

## Anhang-I-Abdeckung (offener Punkt 5)

| Größe | Gemessen |
|---|---|
| Habitattypen mit **irgendeiner** Anhang-I-Entsprechung | **184** |
| davon mit Qualifier `=` | **29** |

Die `=`-Quote ist die praktisch wichtige Zahl: nur `=` ist präzise genug, um
einen deutschen Namen zu leihen. Sobald die amtlichen Anhang-I-Bezeichnungen aus
EUR-Lex gepinnt sind, erben also **29** EUNIS-Typen ein abgeleitetes deutsches
Label — ohne Codeänderung. Aktuell gemessen: `Localizations: 0`,
`DerivedLabels: 0`, weil noch keine deutsche Namensquelle gepinnt ist (offener
Punkt 6).

Ein Habitattyp **ohne** Anhang-I-Entsprechung ist der Normalfall, nicht ein
Fehler: fehlende Daten sind Abwesenheit von Zeilen, nie ein Platzhalter-Code.

## Namensauflösung gegen hostus (offener Punkt 3)

Gemessen gegen einen vollen hostus-Index (WCVP-Backbone):

| Grundgesamtheit | Aufgelöst | Rate |
|---|---|---|
| Zeilen von `species_roles.csv` | 11559 / 13791 (2232 offen) | **83,82 %** |
| distinkte Artennamen | 3142 / 3587 (445 offen) | **87,59 %** |

Die distinktgewichtete Zahl ist die mit dem ESy-Spike vergleichbare
Grundgesamtheit: **87,59 %** liegen deutlich über der dort abgeschätzten
~57 %-Untergrenze (`../research/sp9-esy-spike.md`).

Nicht aufgelöste Namen werden **behalten**, nicht verworfen: `verbatim_name` ist
immer gesetzt, `concept_id` bleibt NULL.

## Verbreitung (`species_distribution`)

Gemessen am selben Lauf, gegen denselben hostus-Index. Der Schritt fragt hostus
einmal pro Konzept, gedrosselt auf 70 ms. Gemessen ist die Dauer des **ganzen**
`situs ingest`: **5:59,98**. Wie viel davon auf diesen Schritt entfällt, wurde
nicht einzeln gemessen; rechnerisch sind allein die Drosselpausen
3135 × 70 ms ≈ 3:40.

| Kennzahl | Gemessen |
|---|---|
| Konzepte im Index (`Concepts`) | 3135 |
| davon mit Verbreitungsdaten (`WithAreas`) | 3135 (100 %) |
| geschriebene Zeilen (`Rows`) | 104581 |
| unvollständige Gebiete (`Incomplete`) | 0 |
| übersprungene Konzepte (`DistributionFailed`) | 0 |
| verschiedene Gebietscodes (`areas_with_data` in `/v1/info`) | 366 |

Woher die **3135** kommen, weil die Zahl auf den ersten Blick niedrig aussieht:
`species_roles.csv` trägt **3587** verschiedene Artnamen, davon löste hostus
**3142** auf (siehe oben) — die restlichen **445** haben keine Konzept-ID und
zählen hier deshalb nicht mit. Von den 3142 aufgelösten Namen fallen nur **7**
paarweise auf dasselbe Konzept zusammen (3142 − 3135). Der Abstand zur Zahl der
Namen ist also fast ausschließlich die Auflösungslücke, nicht Synonymie.

Warum das den Aufwand wert war, an `eunis@2021/R15` mit `?area=GER` gemessen:

| Abfrage | Einträge |
|---|---|
| ohne Filter / `?area=GER` | 170 (`in_area: true` 78, `false` 83, Feld fehlt 9) |
| `?area=GER&only_in_area=true` | 87 (die 83 `false` entfernt, alle 9 unentscheidbaren behalten) |

**49 % der Arten dieses Habitattyps kommen in Deutschland nicht vor.** Eine
Artenliste ohne Gebietsfilter schickt einen Nutzer im Gelände also in etwa jedem
zweiten Fall hinter eine Pflanze, die dort nicht wachsen kann.

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

### Die 241 von situs verfassten Namen

Gemessen am 2026-08-24 gegen `data/localizations-de-situs.csv`:

| Kennzahl | Wert |
|---|---|
| Verfasste Typen (EUNIS L3 ohne `=`-Crosswalk) | **241** |
| Erzeugte Localization-Zeilen | **334** |
| davon `field = vernacular` | **93** (38 %) |
| Überschneidung mit den 29 Ableitungen | **0** |
| Localizations im Index gesamt | **567** (233 `official` + 334 `situs`) |

Die 38 % Vernakular-Abdeckung ist keine Lücke, sondern das Ergebnis der Regel:
ein etablierter deutscher Begriff existiert im Wesentlichen nur, wo der Typ in
Deutschland vorkommt. Für mediterrane, makaronesische und Schwarzmeer-Varianten
gibt es keinen — sie werden im Deutschen nie benannt, und ein konstruierter
Begriff wäre eine Erfindung mit dem Anschein von Geläufigkeit.

**Ein Fall, in dem die Regel gegen das Spec-Beispiel entschieden hat:** die Spec
zeigt `R22 Low and medium altitude hay meadow` → `Glatthaferwiese` als
Illustration. Streng angewandt entfällt das Vernakular dort: `R22` umfasst auch
die montanen Goldhaferwiesen, „Glatthaferwiese" ist also **enger** als der Typ.
Die Zeile trägt nur `name`.
