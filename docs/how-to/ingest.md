# Index aufbauen (`situs ingest`)

```bash
situs ingest --csv-dir pipelines/eunis/out            # nach index.path
situs ingest --csv-dir pipelines/eunis/out --db situs.sqlite
```

Liest die von `pipelines/eunis/xlsx_to_csv.py` erzeugten CSVs
(`typologies.csv`, `habitat_types.csv`, `crosswalks.csv`, `syntaxa.csv`,
`habitat_type_syntaxa.csv`, `species_roles.csv`, optional
`localizations.csv`) aus `--csv-dir` und schreibt in die SQLite-Datei
`--db`. Die Ausgabe ist ein JSON-Report mit den Zeilenzählern je Entität
plus dem Artenrollen-Report (siehe unten).

`--db` ist **optional** und fällt auf `index.path` (`SITUS_INDEX_PATH`,
Default `situs.sqlite`) zurück — dieselbe Datei, aus der `serve` liest. Das
ist Absicht: `serve` kennt kein `--db`, und ein Index, in den ingestiert
wurde, während ein anderer serviert wird, ergibt einen leeren, aber
`/health/ready`-grünen Dienst. Wird `--db` gesetzt, gewinnt das Flag über die
Konfiguration, wie überall in diesem Dienst.

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

## Zwei Transaktionen, nicht eine

`ingest` läuft in **zwei** Schritten, jeder mit eigener Transaktion:

1. `IngestCSV` — Typologien, Habitattypen, Crosswalks, Syntaxa.
2. `IngestSpeciesRoles` — Artenrollen, inklusive hostus-Namensauflösung.

Ist hostus beim zweiten Schritt nicht erreichbar, bricht `ingest` mit einem
Fehler ab — aber der **erste** Schritt ist zu diesem Zeitpunkt bereits
committed. Der Index enthält dann Typologien/Habitattypen/Crosswalks/Syntaxa,
aber keine Artenrollen. Das ist kein Datenverlust: jeder `Upsert*` ist
idempotent, ein erneuter `situs ingest`-Lauf gegen denselben Index holt den
fehlenden zweiten Schritt einfach nach. Ein Operator, der nach einem
fehlgeschlagenen Lauf den Index inspiziert, sollte diese Teilbefüllung aber
nicht als Bug lesen.

## Die gemeldete Resolution Rate

Der Artenrollen-Report (`SpeciesReport`) meldet `ResolutionRate()` als
**zeilengewichtet**: `Resolved / Rows`, über alle Zeilen von
`species_roles.csv`, nicht über die Menge distinkter Artennamen. Das ist eine
andere Grundgesamtheit als die ~57 %-Untergrenze aus dem ESy-Spike
(`../research/sp9-esy-spike.md`), die über distinkte Namen misst — beide
Zahlen sind nicht direkt vergleichbar.

Beide sind gegen den gepinnten Datenstand und einen vollen hostus-Index
**gemessen** (Design-Spec, offener Punkt 3):

| Grundgesamtheit | Aufgelöst | Rate |
|---|---|---|
| Zeilen von `species_roles.csv` | 11559 / 13791 (2232 offen) | **83,82 %** |
| distinkte Artennamen | 3142 / 3587 (445 offen) | **87,59 %** |

Die distinktgewichtete Zahl ist die mit dem ESy-Spike vergleichbare: 87,59 %
liegen deutlich über der dort abgeschätzten ~57 %-Untergrenze. `ResolutionRate()`
im Report meldet weiterhin die **zeilengewichtete** Zahl (`Resolved / Rows`).

Nicht aufgelöste Namen werden **nicht verworfen**: `verbatim_name` ist immer
gesetzt, `concept_id` bleibt NULL.
