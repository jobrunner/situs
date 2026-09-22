# Index aufbauen (`situs ingest`)

```bash
make ingest-input CSV_DIR=out/ingest-input   # Eingabeverzeichnis füllen
situs ingest --csv-dir out/ingest-input      # nach index.path
situs ingest --csv-dir out/ingest-input --db situs.sqlite
```

Das Eingabeverzeichnis füllt **ein Befehl**, nicht eine Liste von
Kopierschritten:

`scripts/collect-ingest-input.sh` ist die eine Stelle, die weiß, was in das
Verzeichnis gehört: die Ausgaben aller Pipelines und die beiden kuratierten
Dateien aus `data/`. Fehlt eine **Pflichtquelle**, bricht es ab und nennt sie;
fehlt eine **optionale**, sagt es, was der Ingest deshalb überspringt. Vorher
stand diese Liste nur als Prosa hier, und eine übersprungene Zeile ergab
stillschweigend einen Index ohne die betroffenen Daten.

Drei Dateien erzeugt kein Lauf in diesem Repo: `eurosl_crosswalk.csv` und
`aggregate_members.csv` kommen aus `hostus export-crosswalk` (siehe unten),
`localizations.csv` aus `pipelines/eurlex`. Fehlt eine davon, **endet das
Skript mit einem Fehler** und nennt den Befehl, der sie erzeugt: ohne
`eurosl_crosswalk.csv` bricht der Ingest ab, ohne `localizations.csv` entsteht
lautlos ein Index ganz ohne deutsche Labels.

Das Zielverzeichnis gehört dem Skript: es entfernt die von ihm verwalteten
Dateien vor jedem Lauf, damit keine Ausgabe von gestern stehen bleibt und
ungeprüft mit ingestiert wird. Ein Pipeline- oder `data/`-Verzeichnis als Ziel
lehnt es ab, weil es dort Quelle auf Quelle kopieren würde.

### Warum zwei Dateien in `data/` versioniert sind

`data/annex1_descriptions.csv` und `data/localizations_descriptions.csv`
werden von **keiner** Pipeline erzeugt. Sie sind mit KI-Unterstützung verfasst
und damit **nicht reproduzierbar**: ein erneuter Lauf ergäbe andere Texte.
Deshalb sind sie versioniert statt erzeugt, und deshalb liefert ein Ingest
aus einem gegebenen Repo-Stand immer denselben Index, ohne dass jemand ein
Modell braucht. Geprüft werden sie von `cmd/situs/curated_test.go`, weil keine
Pipeline das für sie tut.

Liest die von `pipelines/eunis/xlsx_to_csv.py` erzeugten CSVs
(`typologies.csv`, `habitat_types.csv`, `crosswalks.csv`, `syntaxa.csv`,
`habitat_type_syntaxa.csv`, `species_roles.csv`, optional
`localizations.csv`), die zwei **Pflichtquellen** der Syntaxa-Hierarchie
`syntaxa_formations.csv` (aus `data/`) und `syntaxa_hierarchy.csv` (von
`pipelines/eurovegchecklist/xlsx_to_csv.py`) plus, optional,
`wgsrpd_areas.csv` von `pipelines/wgsrpd`, `evc_territories.csv` und
`syntaxon_distribution.csv`/`syntaxon_distribution_coverage.csv` von
`pipelines/evc-distribution`, `habitat_descriptions.csv` von
`pipelines/floraveg-factsheets`, `localizations_descriptions.csv` (aus
`data/`) und die drei Zeigerwert-CSVs
(`eive_traits.csv`, `tichy_traits.csv`, `midolo_traits.csv` — erzeugt von
`pipelines/{eive,tichy,midolo}`) — alle Pipelines schreiben in denselben
`--csv-dir` — aus `--csv-dir` und schreibt in die SQLite-Datei `--db`. Die
Ausgabe ist ein JSON-Report mit den Zeilenzählern je Entität plus dem
Artenrollen-Report und dem Trait-Report (siehe unten).

**Ein Feld im JSON wechselt seit der Quellenumkehr die Form.**
`ingestOutput` bettet den generischen `application.IngestReport` ein (der
seither kein eigenes `Syntaxa`- oder `SyntaxonLinks`-Zahlenfeld mehr führt)
**und** trägt ein eigenes Feld `Syntaxa` vom Typ `application.SyntaxaReport`.
Der JSON-Schlüssel `Syntaxa` ist damit seit dieser Quellenumkehr ein
**Objekt**, kein Zähler mehr — ein Skript, das `Syntaxa` als Zahl liest,
bricht. Der Nachfolger von `SyntaxonLinks` ist `Syntaxa.LinksWritten`.

`--crosswalk`/`--aggregate-members` sind optional und fallen auf
`<csv-dir>/eurosl_crosswalk.csv` bzw. `<csv-dir>/aggregate_members.csv`
zurück — nur bei abweichender Ablage nötig.

`--db` ist **optional** und fällt auf `index.path` (`SITUS_INDEX_PATH`,
Default `situs.sqlite`) zurück — dieselbe Datei, aus der `serve` liest. Das
ist Absicht: `serve` kennt kein `--db`. Wird `--db` gesetzt, gewinnt das Flag
über die Konfiguration, wie überall in diesem Dienst.

**In einen Index ingestieren, der gerade serviert wird, ist trotzdem falsch.**
`serve` liest read-only, kann also nichts kaputtschreiben — aber es liest dann
aus einer Datei, die sich unter ihm ändert, und sieht dabei einen halben Stand.
Der Weg für einen neuen Index steht in `deploy.md`: neu bauen, fertig an seinen
Platz ziehen, Container ersetzen.

Ein Index, der vor der Syntaxa-Hierarchie-Erweiterung gebaut wurde (ohne die
Spalte `syntaxon.author`), braucht die fehlenden Spalten aus `Migrate` — die
liefert `situs ingest` von selbst, ein Löschen ist dafür **nicht** nötig. Das
gilt seit dem Fix des gemessenen Doppel-Bestückungs-Befunds auch für einen
Altindex aus der Zeit vor der Syntaxa-Quellenumkehr: `IngestSyntaxa` leert
`syntaxon` und `habitat_type_syntaxon` vor jedem Schreiben (siehe Schritt 2
unten), also ersetzt ein erneuter Ingest die alten Zeilen statt sie neben den
neuen stehen zu lassen.

!!! warning "Vor dem Ausrollen ingestieren, nicht danach"

    `Migrate` läuft **nur** beim `ingest`, nie beim `serve` (siehe
    `../explanation/architecture.md`). Ein Index aus 0.8.0 trägt noch keine
    Spalte `habitat_description.provenance`; ein 0.9.0-Binary, das gegen ihn
    serviert, beantwortet deshalb **jedes**
    `GET /v1/habitat-type/{typology}/{code}` mit `INTERNAL_ERROR`, nicht nur
    das Beschreibungsfeld. Reihenfolge beim Upgrade ist also: neues Binary,
    `situs ingest`, dann `serve`.

Der Index gehört **nicht** ins Repo. `.gitignore` ignoriert `*.sqlite`
(plus `-wal`/`-shm`), aber keine anderen Endungen — eine Datei namens
`situs.db` wäre also versehentlich stagebar. Deshalb heißt der Default
`situs.sqlite`, und alle Beispiele hier benutzen genau diese Endung.

`localizations.csv` (`entity_type,entity_key,lang,field,value,source,provenance`)
ist optional: fehlt die Datei, ist das **keine** Fehlersituation, sondern
„keine Localizations" — der Ingest loggt das auf `info` und zählt 0. Erzeugt
wird sie von `pipelines/eurlex`: `extract.py` holt die amtlichen deutschen
Anhang-I-Bezeichnungen aus der gepinnten CELEX-Fassung, `merge.py` führt sie
mit den von situs verfassten EUNIS-Namen (`data/localizations-de-situs.csv`)
zu **einer** `localizations.csv` zusammen (siehe
`../../pipelines/eurlex/README.md`).

Zwei Report-Felder betreffen genau das:

- `Localizations` — Anzahl der aus `localizations.csv` gelesenen Overlay-Zeilen.
- `DerivedLabels` — Anzahl der daraus über Qualifier `=` **abgeleiteten**
  deutschen Labels (`provenance: derived`). Abgeleitet wird ausschließlich
  über `=`; `<`, `>`, `#` und `≈` sind zu unscharf, um einen Namen zu leihen,
  und ein vorhandenes `official`/`curated`-Label wird nie überschrieben.

Am Referenzlauf vom 2026-09-16 gemessen: `Localizations: 567`
(233 amtlich + 334 von situs verfasst), `DerivedLabels: 29`. Seitdem sind die
EUNIS-Level 1 und 2 dazugekommen (49 Typen, die nie einen `=`-Crosswalk
tragen): `merge.py` erzeugt am 2026-09-21 **394** verfasste Zeilen, der nächste
volle Lauf misst also `Localizations: 627`. Siehe
`../reference/measured-index.md`.

## Der fertige Index ist eine Datei

Ganz am Ende, nach allen Schreibschritten, checkpointet der Ingest die WAL in
die Datenbank und schaltet den Journal-Modus zurück auf `DELETE`. Das ist die
einzige Stelle, die das kann — der Wechsel aus WAL heraus verlangt, die einzige
Verbindung zu sein — und es ist die Voraussetzung dafür, dass `serve` den Index
read-only öffnen kann, ohne eine `-shm`-Beiwagendatei anzulegen. Ohne diesen
Schritt bräuchte schon ein reiner Leser ein beschreibbares Verzeichnis.

## Getrennte Transaktionen, nicht eine

`ingest` läuft in mehreren Schritten, jeder mit eigener Transaktion:

1. `IngestCSV` — Typologien, Habitattypen, Crosswalks. (Syntaxa sind seit der
   Quellenumkehr **nicht** mehr Teil dieses Schritts, siehe Schritt 2.)
2. `IngestSyntaxa` — liest **vier** Dateien: `syntaxa_formations.csv` (die 25
   EuroVegChecklist-Sektionen A–Y, aus `data/`) und `syntaxa_hierarchy.csv`
   (FloraVeg.EU EuroVegChecklist, aus `pipelines/eurovegchecklist`) sind seit
   der Syntaxa-Quellenumkehr die **primären** Quellen der Hierarchie — beide
   sind Pflichtdateien, ihr Fehlen bricht den Ingest ab, statt einen Index
   ohne Vegetationshierarchie stillschweigend zu bauen. `syntaxa.csv` und
   `habitat_type_syntaxa.csv` (aus `pipelines/eunis`) liefern noch die
   Verbände, die FloraVeg nicht führt (`source: eunis`), und die
   Habitattyp-Kanten, deren EEA-Code über den EEA-Code auf FloraVegs
   Primärcode aufgelöst wird. Details und gemessene Zahlen:
   `../reference/measured-index.md#syntaxa-tiefe-offener-punkt-1`. Läuft
   direkt nach `IngestCSV` und vor Artenrollen/Verbreitung/Zeigerwerten/
   Label-Overlay, von denen keiner davon abhängt.

   **`IngestSyntaxa` ersetzt die Hierarchie, statt sie zu ergänzen:** als
   allererster Schritt der Transaktion leert es `habitat_type_syntaxon` und
   danach `syntaxon` komplett, bevor es die aktuellen Dateien schreibt. Die
   beiden Quelldateien sind die vollständige Wahrheit über die Hierarchie —
   ein Upsert kann eine Zeile, die aus der Quelle verschwunden ist, nicht
   entfernen, und genau das ist seit der Quellenumkehr der Normalfall (rund
   1310 Identifikatoren haben dabei die Seite gewechselt). Ein Leser muss
   also wissen: ein zweiter Ingest **ersetzt** den Syntaxa-Teil des Index,
   er hängt nichts an. Da das Leeren und jedes folgende Schreiben in
   derselben Transaktion laufen, ist ein fehlschlagender Ingest (etwa eine
   verbleibende Waise) atomar — der Index bleibt unverändert, statt leer
   dazustehen.

   **Sieben Datenfehler lassen `IngestSyntaxa` absichtlich scheitern**, statt
   zu warnen und weiterzulaufen:

   - eine **Waise** — eine Zeile mit einem anderen Rang als `formation`, für
     die weder Namensabgleich noch Geschwisterkonsens einen Elternteil
     findet. Eine Kette, die an einer Stelle reißt, macht den
     Orientierungsdienst genau dort wertlos.
   - ein **Elternteil vom falschen Rang** — eine Kante, die eine Stufe von
     Formation → Klasse → Ordnung → Verband überspringt (ein Verband direkt
     unter einer Klasse), oder eine Zeile mit einem Rang, den die Leiter gar
     nicht führt, oder eine **Formation mit Elternteil**, obwohl über der
     Formation nichts steht. Alles drei erreicht eine Formation und ist
     trotzdem nicht navigierbar: `GET /v1/syntaxon/{id}` verspricht genau
     diese vier Stufen.
   - ein **doppelt vergebener Primärcode** — zwei Zeilen in
     `syntaxa_hierarchy.csv` mit demselben `code`. Der Primärcode ist die
     Identität des Syntaxons: er wird als `id` geschrieben, und ein Konflikt
     darauf wird per `ON CONFLICT(id) DO UPDATE` aufgelöst. Beide Zeilen zu
     schreiben verschmölze also zwei Vegetationseinheiten zu einer — die
     spätere gewinnt Rang, Name und Elternteil —, während der Report beide
     zählt; jede `parent_code`- und Habitattyp-Kante auf diesen Code meinte
     dann, wer zuletzt in der Datei stand. Wie beim EEA-Code nennt die Meldung
     **alle** kollidierenden Codes, und die Prüfung läuft vor der Transaktion.
     `pipelines/eurovegchecklist/xlsx_to_csv.py` bricht bereits bei der
     Konvertierung ab; der Go-Ingest liest die CSV aber direkt, eine
     handverlesene Datei erreicht ihn also ungeprüft.
   - eine **ID, die zwei Quellen beanspruchen**. In die Spalte `syntaxon.id`
     schreiben **drei** Quellen: die Formationen aus
     `data/syntaxa_formations.csv`, die Hierarchiezeilen aus
     `syntaxa_hierarchy.csv` und die EEA-eigenen Zeilen aus `syntaxa.csv` — in
     dieser Reihenfolge, jede per `ON CONFLICT(id) DO UPDATE`. Der doppelte
     Primärcode (voriger Spiegelstrich) prüft nur die Hierarchiezeilen
     untereinander; eine Hierarchie- oder EEA-Zeile mit dem Code einer
     Formation überschreibt dagegen die Formation selbst, und ist ihre eigene
     Elternkette rangtreu, laufen Waisen-, Zyklus- und Rangprüfung sauber
     durch — der Index committet mit einer Wurzel weniger. Geprüft wird über
     alle drei Quellen zugleich und vor der Transaktion; die Meldung nennt
     jede beanspruchte ID einmal, mit den beanspruchenden Dateien
     (`Z (syntaxa_formations.csv, syntaxa_hierarchy.csv)`). Abbruch statt
     Verwerfen-und-Melden, weil keine Seite verzichtbar ist: die Formationen
     sind die Wurzel der Navigation, und eine verworfene Hierarchiezeile
     nähme den ganzen Teilbaum unter sich mit. Geprüft werden nur die Zeilen,
     die tatsächlich geschrieben werden — eine EEA-Zeile mit FloraVeg-
     Gegenstück (ihre `id` ist ein `eea_code` der Hierarchie) erreicht die
     Tabelle nie und kann folglich mit nichts kollidieren.
   - ein **doppelt beanspruchter EEA-Code** — zwei Zeilen in
     `syntaxa_hierarchy.csv` mit demselben nichtleeren `eea_code`. Der
     EEA-Code ist der Migrationsschlüssel von den alten IDs
     (`GET /v1/syntaxon/PAP-01A`); ist er mehrdeutig, lässt sich nicht
     entscheiden, welches Syntaxon die alte ID meint. Die Meldung nennt
     **alle** kollidierenden Codes, damit die Quelle in einem Durchgang
     reparierbar ist. Diese Prüfung läuft vor der Transaktion, der Index
     wird also gar nicht erst angefasst.
   - ein **leerer oder doppelt vergebener Formationsbuchstabe** in
     `syntaxa_formations.csv`. Der Buchstabe ist der Schlüssel der Formation:
     leer ergäbe er eine Formation mit der ID `""`, auf die keine Klasse
     zeigen kann, doppelt überschriebe die spätere Zeile Namen und
     Lebensformgruppe der früheren, lautlos. Auch das wird beim Lesen der
     Datei geprüft, also vor der Transaktion. **Nicht** geprüft wird, ob die
     25 Buchstaben A–Y lückenlos beisammen sind: die Sektionen gehören der
     gepinnten EuroVegChecklist-Fassung, nicht diesem Code. Ein fehlender
     Buchstabe bleibt trotzdem nicht unbemerkt — jede Klasse darunter wird
     übersprungen, und deren Ordnungen und Verbände werden dadurch zu Waisen
     (erster Spiegelstrich).
   - eine **Formationsdatei ohne eine einzige Zeile**. Eine `syntaxa_formations.csv`
     mit bloßer Kopfzeile besteht die Pflichtquellen-Prüfung — die Datei ist ja
     da und trägt alle Spalten —, und seit das Leeren zum Ingest gehört, würde
     ein solcher Lauf den Syntaxa-Teil des Index löschen und mit null Wurzeln
     committen: eine abgeschnittene kuratierte Quelle nimmt lautlos die ganze
     Hierarchie mit. Auch das wird beim Lesen geprüft, also vor der
     Transaktion. Eine **leere `syntaxa_hierarchy.csv` bleibt erlaubt**: die
     Formationen allein sind eine karge, aber gültige Hierarchie.

   Eine Zeile in `syntaxa_hierarchy.csv` **ohne `code`** ist dagegen kein
   Abbruchgrund: der Code ist die Identität der Zeile, ohne ihn kann niemand
   auf sie zeigen, und geschrieben würde sie ein namenloses Syntaxon mit der
   ID `""` ergeben, das Waisen-, Zyklus- und Rangprüfung anstandslos passiert,
   sofern Rang und Elternteil stimmen. Sie wird übersprungen, in `SkippedRows`
   gezählt und mit Datei, Zeile und Grund als Warnung protokolliert. Dasselbe
   gilt für eine Zeile in `syntaxa.csv` **ohne `id`** — trifft ihr Name einen
   FloraVeg-Verband, löst sogar der Namensabgleich einen Elternteil auf, und
   der Lauf committet ein unerreichbares Syntaxon. Beide Schlüssel werden
   geprüft wie die Verbreitungsleser ihre prüfen. Ebenfalls verworfen und
   gemeldet wird eine Zeile in `syntaxa.csv` **mit dem Rang `formation`**:
   Formationen definiert allein `syntaxa_formations.csv`, und geschrieben
   bekäme die Zeile wie jede andere EEA-Zeile einen Elternteil abgeleitet —
   eine Formation *mit* Elternteil, obwohl die Formation die Wurzel ist. Die
   Rangprüfung meldet eine solche Formation seitdem auch, statt Formationen
   ungeprüft zu überspringen (zweiter Spiegelstrich oben).
3. `IngestAreas` — liest **zwei** Dateien über denselben Code-Pfad, je einmal
   aufgerufen: `wgsrpd_areas.csv` (`pipelines/wgsrpd`, aus der gepinnten
   TDWG-Tabelle, Report-Zweig `AreaNames`) und `evc_territories.csv`
   (`pipelines/evc-distribution`, Report-Zweig `Territories`, separat gezählt
   statt aufsummiert). Beide schreiben nur die Namen der Gebietscodes, die
   `GET /v1/areas` neben dem Code liefert. Rein lokal, kein Dienst wird
   gefragt; hängt von nichts ab und nichts hängt davon ab — die Namen sind ein
   Overlay auf Codes, die andere Schritte schreiben (Artenverbreitung bzw.
   Syntaxa-Verbreitung, Schritt 6). Fehlt eine der beiden Dateien, ist das
   **keine** Fehlersituation: der jeweilige Zweig zählt 0 Areas und die Codes
   bleiben namenlos. Eine Zeile ohne Code oder mit einem fremden
   Gebietsschema wird übersprungen und gezählt (`SkippedRows`) statt
   geschrieben: sie würde auf der Leseseite mit nichts zusammenfinden, und
   dieses Schweigen soll im Report stehen.
4. `IngestDescriptions` — liest optional `habitat_descriptions.csv`
   (`pipelines/floraveg-factsheets`, aus dem gepinnten Factsheet-PDF) und
   `annex1_descriptions.csv` (aus `data/`, von situs verfasst) und schreibt je
   Habitattyp seine englische Beschreibung. **Welche Datei eine Zeile trug,
   entscheidet ihre `provenance`** (`official` für die Factsheets, `situs` für
   Anhang I); nichts am Inhalt einer Zeile tut das. Läuft nach `IngestCSV`,
   weil jede Zeile gegen den Habitattyp geprüft wird, zu dem sie gehört: eine
   Beschreibung zu einem Code, den dieser Index nicht führt, wird verworfen und
   gezählt. `Descriptions.SkippedUnknownCode` ist die **Summe über beide
   Dateien** (am Referenzstand 13, sämtlich aus den Factsheets: die marinen
   `MA*`-Codes sowie `N23`/`N24`). Rein lokal, kein Dienst wird gefragt.
5. `IngestSyntaxonDistribution` — die Verbreitung der Syntaxa (nicht der
   Arten — das ist Schritt 7), gelesen aus `syntaxon_distribution.csv` und
   `syntaxon_distribution_coverage.csv` (beide von
   `pipelines/evc-distribution`, dem gepinnten EuroVegChecklist-Verbreitungs-
   export). Rein lokal, kein Dienst wird gefragt. Läuft nach `IngestSyntaxa`:
   eine Verbreitungszeile zu einer Syntaxon-ID, die der Index nicht führt,
   wird verworfen und gezählt (`UnknownSyntaxa`) — das bedeutet erst etwas,
   sobald die Syntaxa selbst im Index stehen. Ebenso verworfen und gemeldet
   (`NonAllianceSyntaxa`) wird eine Zeile zu einer ID, die der Index führt,
   aber **nicht als Verband**: die Quelle deckt nur Verbände ab, und eine
   Klassen- oder Ordnungszeile würde sonst als Verbreitungstatsache an
   `GET /v1/syntaxon/{id}` erscheinen. Beide Leser prüfen das, bevor eine der
   beiden Tabellen geschrieben wird. Die Coverage-Datei trennt eine
   echte „kommt hier nicht vor"-Aussage von einem bloß fehlenden Datenpunkt;
   siehe `../reference/http-api.md` für die Dreiwertigkeit auf der Leseseite.

   **Auch dieser Schritt ersetzt, statt zu ergänzen** (wie `IngestSyntaxa`):
   als erstes in derselben Transaktion leert er `syntaxon_distribution` und
   `syntaxon_distribution_coverage`, bevor er die aktuellen Dateien schreibt.
   Beide Tabellen hängen an keinem Fremdschlüssel, das Leeren der Syntaxa
   erreicht sie also nicht. Nötig ist es, weil hier die **Abwesenheit einer
   Zeile Bedeutung trägt**: eine Coverage-Zeile ohne Verbreitungszeile heißt
   „geprüft, kommt nirgends vor", gar keine Coverage-Zeile heißt „niemand hat
   nachgesehen". Eine stehengebliebene Coverage-Zeile veraltet also nicht bloß,
   sie verwandelt `unknown` in `absence` — die Umkehrung dessen, wofür die
   Tabelle existiert. Leeren und Schreiben laufen in derselben Transaktion:
   schlägt der Lauf fehl, bleibt der Altstand unverändert stehen.

   Fehlt `syntaxon_distribution.csv` ganz, wird der Schritt **übersprungen**,
   bevor überhaupt eine Transaktion aufgeht — „noch nichts gepinnt" löscht
   nichts.

   **Zwei Datenfehler lassen diesen Schritt absichtlich scheitern**, beide
   aus demselben Grund: sie würden die Vierwertigkeit der Syntaxa-Verbreitung
   stillschweigend zu einer Dreiwertigkeit zusammenfalten.

   - `syntaxon_distribution.csv` ist da, `syntaxon_distribution_coverage.csv`
     fehlt. Dann läse die leere Zelle jedes Verbands als „niemand hat
     nachgesehen", wo „kommt nicht vor" gemeint ist — und zwar für den ganzen
     Index. Geprüft, bevor eine Transaktion aufgeht.
   - beide Dateien sind da, aber die Coverage-Datei nennt ein Syntaxon nicht,
     für das die Verbreitungsdatei Zeilen trägt. Derselbe Widerspruch, nur je
     Syntaxon: `GET /v1/syntaxon/{id}` ließe die Verbreitung weg (`unknown`),
     während `GET /v1/info` und `GET /v1/areas` dessen Territorien weiter
     ausweisen. Die Meldung nennt **alle** betroffenen IDs, sortiert und je
     einmal. Anders als ein Syntaxon, das die Verbreitungsquelle führt und die
     Hierarchie nicht (`CI01E`, siehe `UnknownSyntaxa`), ist das keine
     Fassungsdrift zwischen zwei unabhängigen Quellen: beide Dateien stammen
     aus **einem** `pipelines/evc-distribution`-Lauf über **ein** Artefakt.
     Widersprechen sie einander, passen die Dateien nicht zusammen.
6. `IngestSpeciesRoles` — Artenrollen, aufgelöst gegen eine lokale
   Crosswalk-Datei (`eurosl_crosswalk.csv`), plus abgeleitete
   Mitgliedsarten-Zeilen für Sammelarten (`aggregate_members.csv`). Kein
   hostus-Aufruf mehr in diesem Schritt.
7. `IngestDistribution` — die Verbreitung der aufgelösten Arten-Konzepte
   (siehe unten; nicht zu verwechseln mit Schritt 5, der Syntaxa-Verbreitung).
8. `IngestTraits` — die Zeigerwerte (EIVE, Tichý, Midolo) der aufgelösten
   Konzepte, gelesen aus `eive_traits.csv`, `tichy_traits.csv` und
   `midolo_traits.csv` im `--csv-dir`, ebenfalls über hostus-Namensauflösung.
9. `IngestLocalizations` und `DeriveGermanLabels` — der Label-Overlay. Gelesen
   werden **zwei** Dateien über denselben Code-Pfad: `localizations.csv` (die
   Labels, aus `pipelines/eurlex`) und `localizations_descriptions.csv` (die
   deutschen Beschreibungen, gepflegt in `data/`). Beide sind optional, ihre
   Zeilen werden im Report zusammengezählt.

Fehlt `eurosl_crosswalk.csv`, bricht `ingest` beim **sechsten** Schritt
(`IngestSpeciesRoles`) mit einem Fehler ab — aber die **ersten fünf**
Schritte sind zu diesem Zeitpunkt bereits committed
(Typologien/Habitattypen/Crosswalks, Syntaxa inklusive der
FloraVeg-Hierarchie-Anreicherung, die Gebietsnamen, die Beschreibungen und
die Syntaxa-Verbreitung, aber keine Artenrollen). hostus wird dabei gar nicht
erst kontaktiert: die Namensauflösung ist seit der
Aggregat-Mitgliedsarten-Erweiterung rein dateibasiert, hostus kommt erst ab
Schritt 7 (`IngestDistribution`) ins Spiel. Ein fehlgeschlagener sechster
Schritt ist kein Datenverlust: jeder `Upsert*` ist idempotent, ein erneuter
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

**Woher die beiden Dateien kommen:** aus `hostus export-crosswalk`, seit
hostus 3.5 vorhanden:

```bash
hostus export-crosswalk --db <hostus-index.sqlite> --out-dir "$CSV_DIR"
```

Das schreibt `eurosl_crosswalk.csv` und `aggregate_members.csv` direkt in den
`--csv-dir` und meldet die Namenskollisionen, die es **nicht** rät. Am
Referenzlauf vom 2026-09-16 gemessen: 11618 / 13791 Zeilen aufgelöst
(84,24 %), 905 geschriebene Mitgliedsarten-Zeilen plus 2 weitere, deren
Schlüssel schon belegt war (`SuppressedByExplicit`; die 2 sind **nicht** in den
905 enthalten — es sind die beiden Zweige derselben Entscheidung). Ob dahinter
eine explizite `species_roles.csv`-Zeile stand oder zwei Aggregate dieselbe
Mitgliedsart nennen, unterscheidet der Zähler nicht — der Name des Feldes ist
enger als das, was es misst. 63 mehrdeutige Namen bleiben bewusst offen. Alle Zahlen in
`../reference/measured-index.md`.

Nicht aufgelöste Namen werden **nicht verworfen**: `verbatim_name` ist immer
gesetzt, `concept_id` bleibt NULL.

## Der Verbreitungsschritt

`IngestDistribution` läuft nach den Artenrollen (es braucht die schon
ingestierten Konzept-IDs) und füllt `species_distribution` — die Grundlage für
`?area=` und `?only_in_area=` auf der Leseseite. Gefragt wird hostus, **einmal
pro Konzept**: für die Verbreitung gibt es keine Batch-Route. Der Ingest-Pfad
drosselt deshalb selbst auf ein Konzept je 70 ms, weil hostus oberhalb von
20 req/s mit 429 antwortet. An den echten Daten sind das **3323** Konzepte
(gemessen am Referenzlauf 2026-09-16), allein die Drosselung also
3323 × 70 ms ≈ **3:53** — eine Untergrenze, die Antwortzeiten kommen obendrauf.
Gemessen ist der **ganze** `situs ingest` mit **5:46**; die Aufteilung auf die
Schritte wurde nicht einzeln gemessen und steht deshalb hier nicht.

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

Das gilt auch für **Zeitüberschreitungen einzelner Anfragen**: der
HTTP-Timeout einer Konzeptanfrage ist ein Quellenproblem und wird toleriert
(er zählt in `DistributionFailed`), auch wenn alle Anfragen daran scheitern —
dann greift der Gesamtausfall-Pfad mit Warnung und Nullen. Abgebrochen wird
der Lauf ausschließlich, wenn der **Ingest-Kontext selbst** beendet ist
(Ctrl-C, Deadline). Woran das erkannt wird, ist der Kontext, nicht der Fehler:
ein `http.Client.Timeout` liefert einen Fehler, der `context.DeadlineExceeded`
erfüllt, obwohl niemand den Lauf gestoppt hat.

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

Das ist eine bewusste Abweichung von der Syntaxa-Verbreitung (Schritt 5), die
ihre beiden Tabellen vorab leert. Die Artenverbreitung kommt **eine
hostus-Anfrage je Konzept**, und ein Ausfall mitten im Lauf bricht den Ingest
absichtlich nicht ab (siehe oben). Würde hier vorab geleert, machte genau
dieser tolerierte Ausfall aus „diesmal nicht aktualisiert" ein „sämtliche
Gebiete sämtlicher Arten sind weg". Eine Zeile nicht zu aktualisieren ist die
kleinere Unwahrheit als sie zu löschen.

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
