# Syntaxa-Quellenumkehr: vollständige Hierarchie von der Formation bis zum Verband

Stand: 2026-09-21. Teilprojekt **A** von drei (B: Navigationsrouten,
C: Syntaxa-Verbreitung).

**Revidiert:** `2026-08-30-situs-syntaxa-hierarchie-design.md`. Jenes Spec
machte FloraVeg zu einem *Overlay*, das per Namens-Präfixabgleich Elternteile
an bereits ingestierte EUNIS-Verbände anheftet. Dieses Spec dreht die
Quellenrichtung: FloraVeg wird die **primäre** Syntaxa-Quelle, die
EEA-EUNIS-Quelle liefert nur noch die Kanten Habitattyp → Syntaxon. Der
Namensabgleich bleibt als schmaler Nachrang für die 16 Einheiten erhalten,
die FloraVeg nicht führt.

## 1. Anlass: drei Messungen

Alle Zahlen am gepinnten Artefakt und am gebauten Index gemessen
(2026-09-21), nicht geschätzt.

**Der Namensabgleich lässt Lücken und hat bereits einen stillen Fehler
produziert.** 38 der 1049 Verbände und 1 der 382 Ordnungen tragen
`parent_id = ''` und hängen damit an keiner Klasse; alle 38 sind an
EUNIS-Habitattypen verlinkt, erscheinen also in Antworten. Der Fehler:
`AMM-02B` (*Mertensio maritimae-Honckenyion diffusae*) trägt Elternteil
`JE01`, laut Quellcode gehört er an `JD02`. Von 1005 per Namensabgleich
gesetzten Elternteilen ist das der einzige Widerspruch zum Code — aber er
wäre unbemerkt geblieben.

**Die Quelle liefert den Schlüssel, den der Namensabgleich ersetzt.** Die
Code-Zelle der FloraVeg-XLSX lautet `AA01A (PAP-01A)`; der Klammerteil ist
genau das EEA-Codeschema. `pipelines/eurovegchecklist/xlsx_to_csv.py`
(`primary_code`) verwirft ihn seit Anlage der Pipeline. Gemessen: in
**1841 von 1841** Zeilen vorhanden, 1841 eindeutige Werte, keine Kollision,
kein Mehrfacheintrag. Damit sind 1034 der 1050 EEA-Syntaxa exakt joinbar.

**Die Kryptogamen-Vegetation fehlt vollständig.** Weil situs die Syntaxa aus
der EEA-Zuordnung zieht und diese fast ausschließlich Gefäßpflanzen-Vegetation
kennt, existieren ganze Vegetationsgruppen nicht im Index:

| Gruppe | Sektionen | Verbände in FloraVeg | im Index einer Sektion zugeordnet | fehlend |
|---|---|---|---|---|
| Phanerogamen | A–Q | 1120 | 1009 | 111 |
| Moose/Flechten | R–T | 137 | **0** | 137 |
| Algen | U–Y | 53 | 2 | 51 |
| **Summe** | A–Y | **1310** | **1011** | **299** |

299 Verbände fehlen, darunter jede epigäische, epilithische und
rinden-/holzbewohnende Moos- und Flechtengesellschaft.

Die dritte Spalte zählt, was im Index über `parent_id` einer Sektion
zuzuordnen ist — nicht die Zeilen der Tabelle `syntaxon`. Dort stehen 1049
Verbände; 1011 davon haben ein Elternteil und damit eine Sektion, die
übrigen 38 sind genau die elternlosen aus der ersten Messung
(1011 + 38 = 1049). Beide Zahlen sind richtig und zählen Verschiedenes, was
hier ausdrücklich steht, weil ein Leser sonst einen Widerspruch sieht.

## 2. Die Formation steckt im Klassencode

[floraveg.eu/vegetation](https://floraveg.eu/vegetation/) führt oberhalb der
Klasse eine Ebene, die der **erste Buchstabe des Klassencodes** ist: Klasse
`CA` gehört zu Sektion `C` (Vegetation der nemoralen Waldzone). Gemessen: alle
25 Buchstaben A–Y sind in den 150 Klassen belegt, keiner fehlt, keiner ist
überzählig. Die Ebene ist damit aus dem Code ableitbar — dieselbe Regel, mit
der die Pipeline schon heute `rank` und `parent_code` gewinnt, nur eine Stufe
höher.

Der Buchstabenbereich trägt zugleich die Lebensform-Gruppe: **A–Q**
Phanerogamen (Samenpflanzen-Gesellschaften), **R–T** Moos- und
Flechtengesellschaften, **U–Y** Algengesellschaften.

Die 25 Sektionsnamen sind die einzige Angabe dieses Specs, die nicht aus dem
gepinnten Artefakt ableitbar ist — die XLSX führt sie nicht mit. Sie werden
als versionierte Datendatei `data/syntaxa_formations.csv` im Repo geführt,
mit `pipelines/eurovegchecklist/manifest.yaml` als Herkunftsnachweis auf
floraveg.eu/vegetation. Sie sind eine Quellangabe, nicht situs-Erfindung —
abgetippt von der Übersichtsseite, nicht aus dem gepinnten Artefakt
gewonnen, weshalb die Datei ihre Herkunft im Manifest trägt. Eine
`provenance`-Spalte bekommt `syntaxon` deswegen **nicht**: die Tabelle
speichert Syntaxa, keine Labels, und die einzige Herkunftsfrage, die sich
hier stellt, betrifft das Elternteil (`parent_provenance`).

## 3. Datenfluss

```
FloraVeg.EU .xlsx (List_of_European_vegetation_units_version_4)
      ↓ pipelines/eurovegchecklist/xlsx_to_csv.py (erweitert)
      ↓ syntaxa_hierarchy.csv (code|rank|name|author|parent_code|eea_code)
      │   rank/parent_code/eea_code AUSSCHLIESSLICH aus dem Code-Muster bzw.
      │   der Code-Zelle — nie aus dem Namen geraten
data/syntaxa_formations.csv (letter|name_en|life_form_group)
      ↓
situs ingest → application.IngestSyntaxa (neu, EINE Transaktion)
  1. Formationen   → 25 Zeilen aus data/syntaxa_formations.csv
  2. EVC-Hierarchie → 150 Klassen, 381 Ordnungen, 1310 Verbände aus
                      syntaxa_hierarchy.csv; parent_id aus dem Codemuster
  3. EEA-Rest      → NUR Einheiten OHNE FloraVeg-Gegenstück (gemessen 16),
                      als eigene Zeilen mit source=eunis
  4. Kanten        → habitat_type_syntaxa.csv; Syntaxon-ID über eea_code auf
                      den FloraVeg-Primärcode aufgelöst
  5. Restelternteile → Namensabgleich, dann Geschwisterkonsens
                      (parent_provenance=derived) für die EEA-Waisen
```

### Umbau der Ingest-Verdrahtung

Heute liegen `ingestSyntaxa` und `ingestSyntaxonLinks` **innerhalb** von
`IngestCSV` (`internal/application/ingest.go`, `ingestAll`), und
`IngestSyntaxaHierarchy` läuft danach in eigener Transaktion — in dieser
Reihenfolge, weil der Namensabgleich die EUNIS-Zeilen vorfinden musste. Die
Umkehrung dreht diese Abhängigkeit, also wandern **alle** Syntaxa-Schritte aus
`IngestCSV` heraus in eine neue `application.IngestSyntaxa`, die die fünf
Schritte oben in einer Transaktion ausführt.

Das ist mehr als eine Reihenfolgenänderung und bewusst so: die Schritte 2 bis
5 sind voneinander abhängig (Schritt 4 braucht die EEA-Codes aus 2, Schritt 5
die Kanten aus 4 nicht, aber die Geschwister aus 2 und 3), und sie über zwei
Transaktionen und zwei Aufrufebenen zu verteilen hat schon einmal zu der
Reihenfolge-Kopplung geführt, die dieses Spec auflöst. `IngestCSV` behält
Typologien, Habitattypen und Crosswalks — keine davon hängt an Syntaxa, es
gibt keine Fremdschlüssel.

`cmd/situs/ingest.go` ruft danach `IngestSyntaxa` an der Stelle auf, an der
heute `IngestSyntaxaHierarchy` steht (nach `IngestCSV`, vor den
Localization-Schritten).

### Dateibereitstellung

`scripts/collect-ingest-input.sh` führt `syntaxa_hierarchy.csv` heute unter
`OPTIONAL`. Sie wird zu `REQUIRED`, zusammen mit der neuen
`data/syntaxa_formations.csv` — beide sind jetzt Primärquellen, und ihr Fehlen
darf nicht zu einem stillschweigend hierarchielosen Index führen. Das deckt
sich mit dem Ingest-Abbruch aus Abschnitt 8.

## 4. Domäne

```go
// internal/domain/habitat.go
type Syntaxon struct {
	ID       string
	Rank     string // "formation" | "class" | "order" | "alliance"
	Name     string
	Author   string
	ParentID string

	// EEACode is the EEA-EUNIS code of the same syntaxon, from the
	// parenthesized part of the FloraVeg code cell. Empty for formations
	// and for units known to only one of the two sources.
	EEACode string

	// Source names the source of the ROW: "evc" (FloraVeg/
	// EuroVegChecklist) or "eunis" (carried only by the EEA).
	Source string

	// ParentProvenance is "official" when ParentID comes from the code
	// pattern or from name matching, and "derived" when it was derived
	// from sibling consensus. Meaningless when ParentID is empty, and
	// then "official".
	ParentProvenance string

	// LifeFormGroup is "phanerogam" | "bryophyte_lichen" | "algae" and is
	// set EXCLUSIVELY on formation rows. Deliberately not denormalized
	// onto every row: the group is a property of the formation, and a
	// correction would otherwise have to be propagated across all 1882
	// rows.
	// A filter joins at most three levels upward.
	LifeFormGroup string
}
```

`Rank == "formation"` ist der einzige Rang mit leerem `ParentID` — die
Wurzel. Jede andere Zeile hat nach diesem Spec ein Elternteil; das ist die
zentrale prüfbare Zusage (Abschnitt 8).

## 5. Schema

```sql
-- syntaxon, extended (Migrate, not a fresh schema.sql creation)
ALTER TABLE syntaxon ADD COLUMN eea_code          TEXT NOT NULL DEFAULT '';
ALTER TABLE syntaxon ADD COLUMN source            TEXT NOT NULL DEFAULT 'evc';
ALTER TABLE syntaxon ADD COLUMN parent_provenance TEXT NOT NULL DEFAULT 'official';
ALTER TABLE syntaxon ADD COLUMN life_form_group   TEXT NOT NULL DEFAULT ''
  CHECK (life_form_group IN ('', 'phanerogam', 'bryophyte_lichen', 'algae'));
```

Jede `ALTER`-Anweisung trägt ihren `CHECK` mit, wie es
`habitat_description.provenance` schon vormacht: ein migrierter Index soll
keine Werte annehmen, die `schema.sql` einem neu angelegten verbietet — der
Unterschied fiele erst auf der Leitung auf. `source` bekommt deshalb
`CHECK (source IN ('evc', 'eunis'))` und `parent_provenance`
`CHECK (parent_provenance IN ('official', 'derived'))`.

Kein `CHECK` auf `rank`: die bestehende Tabelle hat keinen, und ein weiterer
Rang soll eine Datenzeile sein, keine Schemaänderung — dieselbe Haltung wie
bei `habitat_typology`. Das gilt für die **Speicherung**; die Leseseite in
Teilprojekt B leitet ihr `rank`-Enum aus dem Index ab, statt eine Liste fest
zu verdrahten, sonst wäre die Haltung eine Ebene höher wieder aufgegeben.

### Die Defaults der Migration sind leer, nicht plausibel

`source`, `parent_provenance` und `life_form_group` bekommen als
Migrationsvorgabe den **leeren String**, nicht `'evc'` bzw. `'official'`. Der
Grund ist unangenehm konkret: ein bestehender Index trägt 1049 Zeilen aus der
EEA-Quelle und 1005 per Namensabgleich gesetzte Elternteile. Eine Vorgabe
`'evc'`/`'official'` würde genau diese Zeilen als EVC-geführt und
quellenbelegt ausweisen — die Zusage „abgeleitete Elternteile sind an
`parent_provenance = derived` erkennbar" wäre für jeden migrierten, aber nicht
neu ingestierten Index gebrochen. Ein leerer Wert ist sichtbar unbefüllt; der
Ingest setzt ihn. Der `CHECK` lässt `''` deshalb ausdrücklich zu.

Für `schema.sql` (frisch angelegter Index, der sofort ingestiert wird) bleibt
`'evc'`/`'official'` als Vorgabe stehen: dort gibt es keine Altdaten, die
falsch beschriftet werden könnten.

### Zwei der drei Indizes gehören in schema.sql, der dritte nicht

`OpenForIngest` wendet das eingebettete `schema.sql` **vor** `Migrate` an
(`internal/adapters/sqlite/open.go`, dann `cmd/situs/ingest.go`). Ein
`CREATE INDEX ... ON syntaxon(eea_code)` in `schema.sql` scheitert deshalb auf
jedem Index, der vor dieser Fassung gebaut wurde, mit `no such column:
eea_code` — und zwar beim Öffnen, also bricht der ganze Ingest ab, bevor die
Migration die Spalte anlegen könnte.

Also: `idx_syntaxon_parent` und `idx_syntaxon_rank` nach `schema.sql` (beide
Spalten existieren seit immer), `idx_syntaxon_eea_code` in `Migrate`,
unmittelbar nach dem `ALTER TABLE`.

`idx_syntaxon_parent` ist nicht optional: die Navigationskette von Teilprojekt
B fragt pro Ebene „alle Kinder von X", ohne den Index ein Full-Table-Scan über
alle 1882 Syntaxa-Zeilen je Klick.

### Die Schemaprüfung beim Serven

`internal/adapters/sqlite/schema_check.go` führt in der Tabelle
`migratedColumns` die Spalten, die eine Migration nachträgt; sie ist
handgepflegt (im Unterschied zur Tabellenliste, die `verifyTables` per Regex
aus `schema.sql` liest). Dort kommt hinzu:

```go
{"syntaxon", `PRAGMA table_info(syntaxon)`, []string{"eea_code", "source", "parent_provenance", "life_form_group"}},
```

Ohne diesen Eintrag startet ein Dienst mit altem Index still und antwortet
ohne Hierarchie — genau der Fall, den die Prüfung verhindern soll.

## 6. Pipeline-Änderungen

`pipelines/eurovegchecklist/xlsx_to_csv.py`:

- `primary_code` wird zu `split_code(cell) -> (primary, alt)`. Der
  Klammerinhalt wird nicht mehr verworfen, sondern als `eea_code` geführt.
  Eine Zelle ohne Klammerteil liefert `alt = ""` — kein Fehler, die
  Testfixtures haben keinen.
- Neue CSV-Spalte `eea_code`. Die Kopfzeile wächst von fünf auf sechs
  Spalten; der Go-Ingest fordert sie über dieselbe `readAll`-Spaltenprüfung
  ein, sodass eine alte CSV laut scheitert statt still leere EEA-Codes zu
  liefern.
- `report.json` bekommt `eea_codes` (Anzahl nichtleerer) und
  `eea_code_collisions` (Anzahl EEA-Codes, die auf mehr als einen Primärcode
  zeigen). Gemessen sind das 1841 und 0; eine künftige Fassung mit
  Kollisionen muss auffallen, nicht stillschweigend einen Gewinner wählen.

- `read_sheet` liest die Zellen einer Zeile heute **positionell**
  (`row.findall("m:c")` in Reihenfolge). Excel lässt leere Zellen in der XML
  aber weg, sodass eine Lücke alle folgenden Spalten verschiebt. Für die
  gepinnte FloraVeg-Datei ist das nachgeprüft unschädlich (gemessen: 0 von
  1842 Zeilen haben eine Lücke), aber es ist eine unabgesicherte Annahme über
  fremde Daten, und Teilprojekt C bricht an genau dieser Stelle. Weil dieses
  Spec die Datei ohnehin anfasst, wird `read_sheet` hier auf die
  `r`-Attribut-basierte Lesung umgestellt — einmal, statt später zwei
  Varianten desselben Helfers im Repo zu haben. Ein Test mit einer künstlich
  lückenhaften Zeile hält die Umstellung fest.

Die Formationen erzeugt die Pipeline **nicht**. Sie stehen in
`data/syntaxa_formations.csv`, weil sie nicht aus der XLSX stammen; sie in
die Pipeline-Ausgabe zu mischen würde eine Quelle behaupten, die es dort
nicht gibt.

## 7. Ingest

```go
// internal/application/syntaxa_hierarchy_ingest.go (umgebaut)

type SyntaxaHierarchyReport struct {
	FormationsWritten int
	ClassesWritten    int
	OrdersWritten     int
	AlliancesWritten  int

	// EunisOnly are the EEA units without a FloraVeg counterpart, kept
	// as their own rows (measured: 16).
	EunisOnly int
	// LinksRemapped are habitat-type edges whose EEA syntaxon id was
	// rewritten to a FloraVeg primary code via eea_code.
	LinksRemapped int
	// ParentsByName are EEA orphans whose parent name matching found
	// (measured: 6).
	ParentsByName int
	// ParentsDerived are EEA orphans whose parent came from sibling
	// consensus (measured: 10).
	ParentsDerived int
	// Orphans are rows that, after every step, remained without a
	// parent and are not a formation. Must be 0 — see section 8.
	Orphans []string

	// SkippedUnknownSection counts class rows whose first letter is not
	// a known formation: skipped, never invented.
	SkippedUnknownSection int
	// SkippedPattern counts rows whose code fits no rank pattern
	// (measured 0 of 1841 — still no automatism that assumes it).
	SkippedPattern int

	EEACodeCollisions  []string
	AmbiguousMatches   []string
	UnknownLinkTargets []string
}
```

### Geschwisterkonsens

Die Regel gilt **nur für den Rang Verband**. Auf eine Ordnung angewandt
ergäbe das Abschneiden des letzten Zeichens Unsinn (`ASP-03` → `ASP-0`); die
einzige EEA-Ordnung wird ohnehin per EEA-Code auf `KC03` aufgelöst. Eine
EEA-Ordnung ohne Gegenstück bliebe also Waise und ließe den Ingest scheitern
— richtig so, denn geraten würde hier nichts.

Für einen EEA-Verband ohne FloraVeg-Gegenstück und ohne Namenstreffer wird die
**EEA-Ordnungsgruppe** gebildet (die ID ohne den letzten Buchstaben, etwa
`NAR-01E` → `NAR-01`). Zeigen *alle* Geschwister dieser Gruppe mit gesetztem
Elternteil auf dieselbe FloraVeg-Ordnung, erbt die Waise dieses Elternteil mit
`parent_provenance = derived`. Bei zwei verschiedenen Elternteilen oder keinem
einzigen gesetzten wird nichts abgeleitet.

Gemessen über alle 287 EEA-Ordnungsgruppen: 285 einheitlich, 1 mit zwei
verschiedenen Elternteilen (`QUI-01`, enthält keine Waise), 1 ohne jedes
gesetzte Elternteil (`THE-01`, deren einzige Waise `THE-01A` per EEA-Code
auflösbar ist). Kein Konflikt betrifft eine Waise. Die Ableitung greift damit
genau zehnmal, und der Report nennt jeden Fall einzeln.

### ASP-03 braucht keine Sonderregel

`ASP-03` (*Moltkeetalia petraeae* Lakušić 1968) ist die einzige EEA-Ordnung
und FloraVegs `KC03`; der EEA-Code-Join führt sie zusammen, die Kante
`eunis@2021/U36 → ASP-03` wird zu `→ KC03`. Die EEA nennt bei `U36` neben
elf Verbänden bewusst die Ordnung als Ganzes, weil `U36` Teile der Ordnung
umfasst, die keinem gelisteten Verband zuzuordnen sind (der dritte Verband
`ASP-03A` hängt an `U37`). Diese Aussage bleibt erhalten:
`habitat_type_syntaxon` darf auf jeden Rang zeigen, und das tut sie hier
absichtlich.

## 8. Fehlerbehandlung

| Fall | Verhalten |
|---|---|
| `syntaxa_hierarchy.csv` fehlt | Ingest bricht **ab**. Anders als bisher: die Datei ist jetzt die Primärquelle, ohne sie entsteht ein Index ohne jede Hierarchie. Der stille Weiterlauf war für ein Overlay richtig und ist für eine Primärquelle falsch. |
| `data/syntaxa_formations.csv` fehlt | Ingest bricht ab, gleiche Begründung — ohne Formationen hat die Kette keine Wurzel. |
| CSV ohne Spalte `eea_code` | Ingest bricht mit Spaltenfehler ab (alte Pipeline-Ausgabe). |
| EEA-Code zeigt auf mehrere Primärcodes | Ingest bricht **ab**, bevor die Transaktion überhaupt beginnt; alle kollidierenden Codes stehen in der Fehlermeldung und in `EEACodeCollisions` (jeder genau einmal, sortiert). Ursprünglich war hier ein Weiterlauf mit ausgelassenem Join vorgesehen — das ist revidiert: der EEA-Code ist der dokumentierte Migrationspfad von den alten IDs (`GET /v1/syntaxon/PAP-01A`), ein doppelt vergebener Code löst also eine alte ID auf ein beliebiges der beanspruchenden Syntaxa samt dessen Habitattyp-Kanten auf. Nie einen Gewinner raten — und ein Index, in dem geraten werden müsste, entsteht gar nicht erst. |
| EEA-Einheit ohne FloraVeg-Gegenstück | Eigene Zeile, `source=eunis`, `eea_code=""`; Elternteil per Namensabgleich, sonst Geschwisterkonsens. |
| Klassencode mit unbekanntem Anfangsbuchstaben | Zeile übersprungen und gezählt; die Formation wird **nie** aus einem unbekannten Buchstaben erfunden. |
| Zeile bleibt nach allen Schritten elternlos | In `Orphans` vermerkt und vom Ingest als Fehler gemeldet. |

Der letzte Punkt ist die zentrale Zusage: **`situs ingest` scheitert, wenn
eine Nicht-Formations-Zeile ohne Elternteil bleibt.** Ein Informationsdienst,
dessen Navigationskette an einer Stelle reißt, ist an dieser Stelle wertlos —
das darf kein Warnhinweis sein, den man überliest, sondern muss den Lauf
anhalten. Gemessen ist der Wert 0; die Zusage hält ihn dort.

## 9. Read-API: nur die Minimaländerung

Die Navigationsrouten sind Teilprojekt B. Dieses Spec darf aber keinen Index
hinterlassen, dessen Fakten die API verschweigt, und muss das `rank`-Enum
öffnen, weil sonst eine Formation die Vertragsprüfung bricht.

`input.SyntaxonRef` (`internal/ports/input/services.go`) wächst um drei
Felder, alle `omitempty`:

```go
type SyntaxonRef struct {
	ID       string `json:"id"`
	Rank     string `json:"rank"`
	Name     string `json:"name"`
	Author   string `json:"author,omitempty"`
	ParentID string `json:"parent_id,omitempty"`

	EEACode          string `json:"eea_code,omitempty"`
	Source           string `json:"source,omitempty"`
	ParentProvenance string `json:"parent_provenance,omitempty"`

	// LifeFormGroup is only filled on formation rows and therefore empty
	// in every response that exists today: habitat_type_syntaxon links
	// alliances, and in one case an order, never a formation. The field
	// is here already anyway, because sub-project B delivers the
	// formations as []SyntaxonRef and filters on it — a list that offers
	// a filter whose entries do not name the filtered value would not be
	// intelligible.
	LifeFormGroup string `json:"life_form_group,omitempty"`
}
```

In `internal/adapters/http/openapi.yaml` (und der byte-identischen Kopie unter
`api/openapi/`) wird das Enum von `SyntaxonRef.rank` auf
`[formation, class, order, alliance]` erweitert, die drei Felder werden
dokumentiert, und die Beschreibung von `parent_id` verliert den Zusatz „fehlt,
wenn unbekannt“ — nach diesem Spec fehlt er nur noch bei Formationen.

## 10. Tests

- `pipelines/eurovegchecklist/test_xlsx_to_csv.py`: `split_code` gegen echte
  Musterzellen (`AA01A (PAP-01A)`, Zelle ohne Klammer, Zelle mit
  Mehrfachinhalt), Kollisionszählung im Report.
- `internal/application/syntaxa_hierarchy_ingest_test.go`: Formationsableitung
  aus dem Buchstaben, EEA-Code-Join, die drei Elternteil-Wege mit ihrer
  Provenienz, Geschwisterkonsens bei einheitlicher und bei widersprüchlicher
  Gruppe, Kantenumschreibung, Abbruch bei fehlender Primärquelle, Abbruch bei
  verbleibender Waise.
- `internal/adapters/sqlite/`: Migration auf einem Index ohne die vier neuen
  Spalten, Schemaprüfung beim schreibgeschützten Öffnen.
- Ein Test über den **gebauten** Index: keine Zeile mit `rank != 'formation'`
  und leerem `parent_id`; jede Zeile erreicht über `parent_id` in höchstens
  drei Schritten eine Formation. Das ist die Zusage aus Abschnitt 8 als Test,
  nicht als Prosa.

## 11. Prüfbare Zusagen

- Kein Elternteil entsteht mehr aus einem Namensvergleich, wenn die Quelle
  einen Code-Join anbietet. Der Namensabgleich greift nur für Einheiten ohne
  FloraVeg-Gegenstück, und sein Ergebnis ist im Report von den Code-Joins
  getrennt ausgewiesen.
- `AMM-02B` hängt nach dem Ingest an `JD02`, nicht an `JE01`.
- Jede Nicht-Formations-Zeile hat ein Elternteil, sonst scheitert der Ingest.
- Moos-, Flechten- und Algengesellschaften sind im Index: die Sektionen R–Y
  tragen nach dem Ingest 190 Verbände, gegenüber 2 heute.
- Abgeleitete Elternteile sind an `parent_provenance = derived` erkennbar und
  im Report einzeln benannt — nie stillschweigend als Quellaussage ausgegeben.
- Keine EUNIS-Habitattyp-Kante geht verloren: die Kantenzahl vor und nach der
  Umkehrung ist gleich, nur die Syntaxon-IDs sind auf Primärcodes umgestellt.
  Das gilt, **weil** die EEA-Codes eineindeutig sind (gemessen: 1841 Codes,
  1841 verschiedene Werte). Fiele diese Eigenschaft in einer künftigen
  Fassung, kollabierten zwei EEA-Syntaxa auf einen Primärcode und der
  Primärschlüssel von `habitat_type_syntaxon` verschluckte eine Kante
  lautlos. Die Prämisse ist deshalb nicht nur zugesichert, sondern geprüft:
  Kollisionen landen in `EEACodeCollisions` und lassen den Ingest scheitern,
  und der Ingest vergleicht die Kantenzahl vor und nach dem Umschreiben und
  scheitert bei einer Abweichung.

## 12. Bewusst außerhalb dieses Specs

- **Read-API und Navigationsrouten** — Teilprojekt B. Dieses Spec ändert die
  bestehenden Syntaxa-Antworten nur um die neuen Felder; Formation → Klassen →
  Ordnungen → Verbände als Lesepfad ist dort beschrieben.
- **Syntaxa-Verbreitung** (Preislerová et al., Zenodo) — Teilprojekt C.
- **Assoziationsebene** — unverändert außerhalb des Scopes, keine freie
  pan-europäische Quelle (siehe „Known ceiling“ in `CLAUDE.md`).
- **Deutsche Namen für Syntaxa.** `domain.Localization.EntityType` erlaubt
  bereits `"syntaxon"` neben `"habitat_type"`; belegt ist bisher nur der
  zweite Wert. Syntaxa- und Formationsnamen ziehen dort ohne jede
  Schema- oder Domänenänderung ein. Das ist eine eigene Runde: die 25 Formationsnamen wären
  schnell übersetzt, die 1857 Syntaxa-Namen sind es nicht, und eine
  halbübersetzte Hierarchie ist schlechter als eine konsequent englische.
- **Nomenklatur-Drift** zwischen FloraVeg-Fassungen: ein erneuter Ingest
  überschreibt idempotent, ein Umbenennungs-Protokoll gibt es weiterhin nicht.
