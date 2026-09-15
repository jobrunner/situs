# Index aufbauen (`situs ingest`)

```bash
situs ingest --csv-dir pipelines/eunis/out            # nach index.path
situs ingest --csv-dir pipelines/eunis/out --db situs.sqlite
```

Liest die von `pipelines/eunis/xlsx_to_csv.py` erzeugten CSVs
(`typologies.csv`, `habitat_types.csv`, `crosswalks.csv`, `syntaxa.csv`,
`habitat_type_syntaxa.csv`, `species_roles.csv`, optional
`localizations.csv`) plus, ebenfalls optional, `wgsrpd_areas.csv` von
`pipelines/wgsrpd` und `syntaxa_hierarchy.csv` von
`pipelines/eurovegchecklist/xlsx_to_csv.py` und die drei Zeigerwert-CSVs
(`eive_traits.csv`, `tichy_traits.csv`, `midolo_traits.csv` — erzeugt von
`pipelines/{eive,tichy,midolo}`) — alle Pipelines schreiben in denselben
`--csv-dir` — aus `--csv-dir` und schreibt in die SQLite-Datei `--db`. Die
Ausgabe ist ein JSON-Report mit den Zeilenzählern je Entität plus dem
Artenrollen-Report und dem Trait-Report (siehe unten).

`--crosswalk`/`--aggregate-members` sind optional und fallen auf
`<csv-dir>/eurosl_crosswalk.csv` bzw. `<csv-dir>/aggregate_members.csv`
zurück — nur bei abweichender Ablage nötig.

`--db` ist **optional** und fällt auf `index.path` (`SITUS_INDEX_PATH`,
Default `situs.sqlite`) zurück — dieselbe Datei, aus der `serve` liest. Das
ist Absicht: `serve` kennt kein `--db`, und ein Index, in den ingestiert
wurde, während ein anderer serviert wird, ergibt einen leeren, aber
`/health/ready`-grünen Dienst. Wird `--db` gesetzt, gewinnt das Flag über die
Konfiguration, wie überall in diesem Dienst.

Ein Index, der vor der Syntaxa-Hierarchie-Erweiterung gebaut wurde (ohne die
Spalte `syntaxon.author`), muss gelöscht und per frischem `situs ingest` neu
aufgebaut werden — ein erneuter Ingest in einen alten Index ist nicht sicher.

Der Index gehört **nicht** ins Repo. `.gitignore` ignoriert `*.sqlite`
(plus `-wal`/`-shm`), aber keine anderen Endungen — eine Datei namens
`situs.db` wäre also versehentlich stagebar. Deshalb heißt der Default
`situs.sqlite`, und alle Beispiele hier benutzen genau diese Endung.

`localizations.csv` (`entity_type,entity_key,lang,field,value,source,provenance`)
ist optional: fehlt die Datei, ist das **keine** Fehlersituation, sondern
„keine Localizations" — der Ingest loggt das auf `info` und zählt 0. Derzeit
erzeugt sie **niemand**; die amtlichen deutschen Anhang-I-Bezeichnungen aus
EUR-Lex sind noch nicht gepinnt (Design-Spec, offener Punkt 6).

Zwei Report-Felder betreffen genau das:

- `Localizations` — Anzahl der aus `localizations.csv` gelesenen Overlay-Zeilen.
- `DerivedLabels` — Anzahl der daraus über Qualifier `=` **abgeleiteten**
  deutschen Labels (`provenance: derived`). Abgeleitet wird ausschließlich
  über `=`; `<`, `>`, `#` und `≈` sind zu unscharf, um einen Namen zu leihen,
  und ein vorhandenes `official`/`curated`-Label wird nie überschrieben.

Am gepinnten Datenstand sind beide gemessen **0**. Siehe
`../reference/measured-index.md`.

## Getrennte Transaktionen, nicht eine

`ingest` läuft in mehreren Schritten, jeder mit eigener Transaktion:

1. `IngestCSV` — Typologien, Habitattypen, Crosswalks, Syntaxa.
2. `IngestSyntaxaHierarchy` — liest optional `syntaxa_hierarchy.csv` (FloraVeg.EU
   EuroVegChecklist, siehe `../reference/measured-index.md#syntaxa-tiefe-offener-punkt-1`)
   und reichert die gerade ingestierten EUNIS-Verbände um Klasse/Ordnung/Autorschaft
   an, soweit ein eindeutiger Namenstreffer existiert. Läuft direkt nach
   `IngestCSV` und vor Artenrollen/Verbreitung/Zeigerwerten/Label-Overlay, von
   denen keiner davon abhängt.
3. `IngestAreas` — liest optional `wgsrpd_areas.csv` (`pipelines/wgsrpd`, aus
   der gepinnten TDWG-Tabelle) und schreibt die Namen der Gebietscodes, die
   `GET /v1/areas` neben dem Code liefert. Rein lokal, kein Dienst wird
   gefragt; hängt von nichts ab und nichts hängt davon ab — die Namen sind ein
   Overlay auf die Codes, die Schritt 5 schreibt. Fehlt die Datei, ist das
   **keine** Fehlersituation: der Report zählt 0 (`AreaNames.Areas`) und die
   Codes bleiben namenlos. Eine Zeile ohne Code oder mit einem anderen
   Gebietsschema als `wgsrpd_l3` wird übersprungen und gezählt
   (`AreaNames.SkippedRows`) statt geschrieben: sie würde auf der Leseseite
   mit nichts zusammenfinden, und dieses Schweigen soll im Report stehen.
4. `IngestSpeciesRoles` — Artenrollen, aufgelöst gegen eine lokale
   Crosswalk-Datei (`eurosl_crosswalk.csv`), plus abgeleitete
   Mitgliedsarten-Zeilen für Sammelarten (`aggregate_members.csv`). Kein
   hostus-Aufruf mehr in diesem Schritt.
5. `IngestDistribution` — die Verbreitung der aufgelösten Konzepte (siehe unten).
6. `IngestTraits` — die Zeigerwerte (EIVE, Tichý, Midolo) der aufgelösten
   Konzepte, gelesen aus `eive_traits.csv`, `tichy_traits.csv` und
   `midolo_traits.csv` im `--csv-dir`, ebenfalls über hostus-Namensauflösung.
7. `IngestLocalizations` und `DeriveGermanLabels` — der Label-Overlay.

Fehlt `eurosl_crosswalk.csv`, bricht `ingest` beim **vierten** Schritt
(`IngestSpeciesRoles`) mit einem Fehler ab — aber die **ersten drei**
Schritte sind zu diesem Zeitpunkt bereits committed
(Typologien/Habitattypen/Crosswalks/Syntaxa inklusive der
FloraVeg-Hierarchie-Anreicherung und die Gebietsnamen, aber keine
Artenrollen). hostus wird dabei gar nicht erst kontaktiert: die
Namensauflösung ist seit der Aggregat-Mitgliedsarten-Erweiterung rein
dateibasiert, hostus kommt erst ab Schritt 5 (`IngestDistribution`) ins
Spiel. Ein fehlgeschlagener vierter Schritt ist kein Datenverlust: jeder
`Upsert*` ist idempotent, ein erneuter
`situs ingest`-Lauf gegen denselben Index holt ihn einfach nach. Ein
Operator, der nach einem fehlgeschlagenen Lauf den Index inspiziert, sollte
diese Teilbefüllung nicht als Bug lesen.

## Namensauflösung und Mitgliedsarten-Ableitung

`IngestSpeciesRoles` löst jeden `verbatim_name` gegen ein lokales,
deterministisches Wörterbuch auf — `eurosl_crosswalk.csv`
(`name,concept_id`), von hostus vorab exportiert, kein Netzwerkaufruf, keine
Fuzzy-/Homonym-Logik. Ein Name mit mehr als einer Concept-ID in dieser Datei
ist ein Datenfund, kein Rateanlass: er bleibt `concept_id: null`, wird
gezählt (`AmbiguousCrosswalk`) und geloggt.

Für jede Zeile, deren aufgelöste Concept-ID selbst eine in
`aggregate_members.csv` gelistete Sammelart ist, schreibt der Ingest je
Mitgliedsart eine zusätzliche Zeile mit `provenance: derived_from_aggregate`
— außer eine explizite `species_roles.csv`-Zeile für dasselbe Mitglied
existiert bereits; die gewinnt immer. Eine abgeleitete Zeile trägt nie
`fidelity`/`constancy` (nie für das Mitglied selbst gemessen).

`ResolutionRate()` bleibt **zeilengewichtet**: `Resolved / Rows`, über alle
Zeilen von `species_roles.csv`.

Die drei neuen Report-Felder:

| Feld | Bedeutung |
|---|---|
| `DerivedRows` | zusätzlich geschriebene Mitgliedsarten-Zeilen |
| `SuppressedByExplicit` | abgeleitete Zeilen, die wegen einer expliziten CSV-Zeile NICHT geschrieben wurden |
| `AmbiguousCrosswalk` | Namen mit mehr als einer Concept-ID in `eurosl_crosswalk.csv` |

**Noch nicht messbar:** `eurosl_crosswalk.csv`/`aggregate_members.csv`
kommen aus einem hostus-seitigen Export, der zum Zeitpunkt dieser Zeilen noch
nicht existiert (siehe
`../superpowers/specs/2026-08-29-situs-aggregat-mitgliedsarten-design.md`).
Bis dahin: `Resolved`, `DerivedRows`, `SuppressedByExplicit` und
`AmbiguousCrosswalk` sind an keinem echten Datenstand gemessen — genau wie
`Localizations`/`DerivedLabels` in `../reference/measured-index.md`, solange
die passende Quelle fehlt.

Nicht aufgelöste Namen werden **nicht verworfen**: `verbatim_name` ist immer
gesetzt, `concept_id` bleibt NULL.

## Der Verbreitungsschritt

`IngestDistribution` läuft nach den Artenrollen (es braucht die schon
ingestierten Konzept-IDs) und füllt `species_distribution` — die Grundlage für
`?area=` und `?only_in_area=` auf der Leseseite. Gefragt wird hostus, **einmal
pro Konzept**: für die Verbreitung gibt es keine Batch-Route. Der Ingest-Pfad
drosselt deshalb selbst auf ein Konzept je 70 ms, weil hostus oberhalb von
20 req/s mit 429 antwortet. An den echten Daten sind das **3135** Konzepte
(gemessen), allein die Drosselung also 3135 × 70 ms ≈ **3:40** — eine
Untergrenze, die Antwortzeiten kommen obendrauf. Gemessen ist der **ganze**
`situs ingest` mit **6:00**; die Aufteilung auf die Schritte wurde nicht
einzeln gemessen und steht deshalb hier nicht.

Das Report-Objekt `Distribution`:

| Feld | Bedeutung |
|---|---|
| `Concepts` | Konzept-IDs im Index, für die gefragt wurde |
| `WithAreas` | Konzepte, zu denen die Quelle mindestens ein Gebiet nannte |
| `Rows` | geschriebene Zeilen (Konzept × Gebiet) |
| `Incomplete` | Gebiete, die verworfen wurden, weil Schema oder Code fehlte |

Daneben, **eine Ebene höher** im gedruckten JSON und nicht in `Distribution`:

| Feld | Bedeutung |
|---|---|
| `DistributionFailed` | Konzepte, deren **einzelne** Anfrage fehlschlug und übersprungen wurde |

**Ein Ausfall der Quelle bricht den Ingest nicht ab.** Die Verbreitung ist
Zusatzwissen; ein Index ohne sie ist benutzbar, nur eben nicht filterbar. Fällt
hostus für diesen Schritt ganz aus, wird das als Warnung geloggt, der Report
zeigt Nullen, und der Lauf geht weiter — anders als bei der Namensauflösung, wo
ein Ausfall abbricht, damit nicht jeder Name als unauflösbar verbucht wird. Ein
späterer Lauf holt den Schritt nach: jeder `Upsert*` ist idempotent.

`DistributionFailed` ist ein **Teilausfall**-Signal, keine Gesamtzahl: es zählt
die Konzepte, die in einem ansonsten erfolgreichen Lauf übersprungen wurden. Es
steht auf `0`, wenn nichts fehlschlug — und ebenso, wenn **alles** fehlschlug,
denn dann ist der Lauf als ganzer Quellenausfall behandelt (Warnung, Report mit
Nullen). „Gab es überhaupt einen Fehler?" beantwortet also nicht
`DistributionFailed` allein, sondern es zusammen mit `WithAreas`/`Rows` und dem
Log.

**Stale-Zeilen.** `UpsertDistribution` fügt nur hinzu. Verliert ein Konzept in
hostus später ein Gebiet, bleibt die alte Zeile stehen und liest sich für immer
als `in_area: true` — hier ist eine veraltete Zeile eine **falsche Antwort**,
nicht bloß ein veraltetes Label. Eine **schrumpfende** Verbreitung braucht
deshalb einen frischen Index (neue Datei, voller Ingest), keinen Re-Ingest auf
den bestehenden.

Wie viele Gebiete am Ende tatsächlich im Index stehen, sagt `areas_with_data` in
`GET /v1/info`.

## Der Trait-Schritt

`IngestTraits` liest die drei Zeigerwert-CSVs (`eive_traits.csv`,
`tichy_traits.csv`, `midolo_traits.csv`) aus `--csv-dir` und löst ihre
Taxonnamen in **einem** gemeinsamen `hostus.Resolve()`-Aufruf über alle drei
Dateien auf, bevor die Werte geschrieben werden.

Die drei Pipelines (`pipelines/{eive,tichy,midolo}`) schreiben ihre Ausgabe
unter einem eigenen Dateinamen nach `output/`, nicht direkt unter dem Namen,
den `situs ingest` erwartet — dieser Kopierschritt ist manuell:

```bash
cp pipelines/eive/output/eive-canonical.csv "$CSV_DIR/eive_traits.csv"
cp pipelines/tichy/output/tichy-canonical.csv "$CSV_DIR/tichy_traits.csv"
cp pipelines/midolo/output/midolo-canonical.csv "$CSV_DIR/midolo_traits.csv"
```

Ein fehlendes Ziel-File wird von `IngestTraits` still als „übersprungen"
gezählt (siehe unten) statt als Fehler gemeldet — der Kopierschritt oben ist
also nötig, nicht nur bequem.

Jede der drei Dateien ist für sich **optional**: fehlt eine, wird ihr
Vokabular im Report als übersprungen (`Skipped`) gezählt statt den Ingest
abzubrechen — dieselbe Haltung wie bei `localizations.csv`. Ist hostus für
diesen Schritt dagegen nicht erreichbar, bricht `ingest` mit einem Fehler ab,
so wie beim Artenrollen-Schritt: eine fehlgeschlagene Namensauflösung darf
nicht stillschweigend als „keine Zeigerwerte" verbucht werden.

Das Report-Objekt `Traits` trägt je Vokabular Zeilen- und
Auflösungszahlen; siehe `../reference/measured-index.md` für die aus den
kanonischen Pipeline-CSVs zählbaren Zeilenzahlen. Ein Konzept ohne
Zeigerwerte ist der Normalfall, kein Fehler.
