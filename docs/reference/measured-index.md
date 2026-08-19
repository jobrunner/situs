# Gemessene Kennzahlen des Index

Alle Zahlen auf dieser Seite sind **am gepinnten Datenstand gemessen**, nicht
geschätzt und nicht aus der Spec übernommen. Sie beantworten die offenen Punkte
1, 2, 3 und 5 der Design-Spec
(`docs/superpowers/specs/2026-08-18-situs-foundation-design.md`).

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
| Crosswalk-Zeilen (Versions- **und** Anhang-I-Crosswalk) | **4307** |
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
