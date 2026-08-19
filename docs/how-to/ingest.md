# Index aufbauen (`situs ingest`)

```bash
situs ingest --csv-dir pipelines/eunis/out --db situs.db
```

Liest die von `pipelines/eunis/xlsx_to_csv.py` erzeugten CSVs
(`typologies.csv`, `habitat_types.csv`, `crosswalks.csv`, `syntaxa.csv`,
`habitat_type_syntaxa.csv`, `species_roles.csv`) aus `--csv-dir` und
schreibt in die SQLite-Datei `--db`. Die Ausgabe ist ein JSON-Report mit den
Zeilenzählern je Entität plus dem Artenrollen-Report (siehe unten).

## Zwei Transaktionen, nicht eine

`ingest` läuft in **zwei** Schritten, jeder mit eigener Transaktion:

1. `IngestCSV` — Typologien, Habitattypen, Crosswalks, Syntaxa.
2. `IngestSpeciesRoles` — Artenrollen, inklusive hostus-Namensauflösung.

Ist hostus beim zweiten Schritt nicht erreichbar, bricht `ingest` mit einem
Fehler ab — aber der **erste** Schritt ist zu diesem Zeitpunkt bereits
committed. Der Index enthält dann Typologien/Habitattypen/Crosswalks/Syntaxa,
aber keine Artenrollen. Das ist kein Datenverlust: jeder `Upsert*` ist
idempotent, ein erneuter `situs ingest`-Lauf gegen dieselbe `--db` holt den
fehlenden zweiten Schritt einfach nach. Ein Operator, der nach einem
fehlgeschlagenen Lauf den Index inspiziert, sollte diese Teilbefüllung aber
nicht als Bug lesen.

## Die gemeldete Resolution Rate

Der Artenrollen-Report (`SpeciesReport`) meldet `ResolutionRate()` als
**zeilengewichtet**: `Resolved / Rows`, über alle Zeilen von
`species_roles.csv`, nicht über die Menge distinkter Artennamen. Das ist eine
andere Grundgesamtheit als die ~57 %-Untergrenze aus dem ESy-Spike
(`docs/research/sp9-esy-spike.md`), die über distinkte Namen misst — beide
Zahlen sind nicht direkt vergleichbar. Wer die reale Messung nachträgt
(Design-Spec, offener Punkt 3), sollte die Anzahl distinkter Namen und ihre
eigene Resolution-Rate danebenstellen, nicht nur die zeilengewichtete Zahl.
