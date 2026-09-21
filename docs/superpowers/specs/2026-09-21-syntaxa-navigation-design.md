# Syntaxa-Navigation: Formation → Klasse → Ordnung → Verband

Stand: 2026-09-21. Teilprojekt **B** von drei. Setzt Teilprojekt A
(`2026-09-21-syntaxa-quellenumkehr-design.md`) voraus: ohne die lückenlose
`parent_id`-Kette und die Formationsebene hat dieses Spec nichts zu
navigieren.

**Ziel:** situs ist ein Informations- und Orientierungsdienst. Die
Syntaxa-Hierarchie ist nach Teilprojekt A vollständig im Index, aber über die
HTTP-Oberfläche nicht erreichbar: es gibt genau eine syntaxon-adressierte
Route (`GET /v1/syntaxon/{id}/habitat-types`), keine Kinder-, keine
Elternteil-, keine Listenroute. Ein Client kann eine Gesellschaft nur
abrufen, wenn er ihre ID schon kennt — es gibt keinen Weg, sie zu finden.

## 1. Zwei neue Routen genügen

```
GET /v1/syntaxa            → die Wurzeln (25 Formationen), filterbar
GET /v1/syntaxon/{id}      → ein Syntaxon mit Ahnenpfad und direkten Kindern
GET /v1/syntaxon/{id}/habitat-types   (bestehend, unverändert)
```

Die Kette entsteht aus der zweiten Route allein, weil jede Antwort die
direkten Kinder mitführt:

```
/v1/syntaxa            → C  (Vegetation der nemoralen Waldzone)
/v1/syntaxon/C         → children: CA … CU        (21 Klassen)
/v1/syntaxon/CA        → children: CA01 … CA0n    (Ordnungen)
/v1/syntaxon/CA01      → children: CA01A …        (Verbände)
/v1/syntaxon/CA01A     → children: []             + habitat_types
```

Bewusst **keine** separate `/children`-Route: sie wäre ein zweiter Weg zu
denselben Daten, und die Kinderliste ist genau das, was ein Navigationsschritt
braucht — sie gehört in die Antwort, nicht hinter einen weiteren Abruf. Ebenso
bewusst **keine** `/ancestors`-Route: der Ahnenpfad ist mit höchstens drei
Elementen kurz genug, um immer mitzureisen, und ein Orientierungsdienst, der
für die Brotkrumenzeile drei Anfragen braucht, orientiert schlecht.

## 2. Antwortstrukturen

```go
// internal/ports/input/services.go

// SyntaxonDetail is a syntaxon with its surroundings: the path upward and
// the direct children. Both are part of the same question ("where am I
// and where can I go?") and therefore part of the same answer.
type SyntaxonDetail struct {
	SyntaxonRef

	// Ancestors is the path to the root, OUTERMOST first (formation,
	// then class, then order). Empty for a formation. The order is
	// fixed so a client can output it unchanged as a breadcrumb line.
	Ancestors []SyntaxonRef `json:"ancestors"`

	// Children are the direct children, sorted by id. Empty for an
	// alliance — that is the lower bound of the data, not an error.
	Children []SyntaxonRef `json:"children"`

	// DirectHabitatTypeCount is the number of habitat types that link
	// EXACTLY this syntaxon — not those of its descendants. For a class
	// or formation the value is therefore practically always 0, because
	// habitat_type_syntaxon links alliances (and in one case an order).
	// The name says so, so a client does not read the 0 as "this class
	// touches no EUNIS type"; aggregating over the descendants is a
	// separate question (section 10).
	DirectHabitatTypeCount int `json:"direct_habitat_type_count"`
}
```

`SyntaxonRef` wächst gegenüber Teilprojekt A nicht weiter — `alt_code`,
`source`, `parent_provenance` und `life_form_group` sind dort schon
eingezogen. `SyntaxonDetail` bettet `SyntaxonRef` ein, dessen Felder damit in
dasselbe JSON-Objekt einwandern (Go promoviert eingebettete Felder). Im
OpenAPI wird das als `allOf` aus `SyntaxonRef` und den Zusatzfeldern
beschrieben und durch einen Test festgenagelt, sonst behaupten Schema und
Leitung Verschiedenes.

`life_form_group` steht deshalb **nur** in `SyntaxonRef` und wird in
`SyntaxonDetail` nicht wiederholt — ein gleichnamiges Feld in der äußeren
Struktur würde das eingebettete überdecken und zwei Felder auf denselben
JSON-Schlüssel legen. Gespeichert ist der Wert laut Teilprojekt A nur auf
Formationszeilen; für jeden anderen Rang füllt ihn die Leseseite aus der
Formation, die der Ahnenpfad erreicht. Er ist dort also ein abgeleiteter, kein
gelesener Wert — und weil `ancestors` in derselben Antwort steht, ist die
Ableitung für den Client nachvollziehbar, nicht behauptet.

`GET /v1/syntaxa` liefert `[]SyntaxonRef`, kein `SyntaxonDetail`: die
Formationen brauchen weder Ahnenpfad (leer) noch Kinderliste (die holt der
nächste Schritt), und 25 Details mit je 150 Kindern wären eine
Antwort, die niemand angefordert hat. Die Lebensform-Gruppe steht in
`SyntaxonRef` und ist damit auch in der Liste sichtbar — nur deshalb ist der
Filter aus Abschnitt 3 nachvollziehbar.

## 3. Filter

`GET /v1/syntaxa` nimmt zwei optionale Parameter:

| Parameter | Werte | Wirkung |
|---|---|---|
| `life_form_group` | `phanerogam`, `bryophyte_lichen`, `algae` | Nur Formationen dieser Gruppe |
| `rank` | `formation`, `class`, `order`, `alliance` | Statt der Wurzeln: alle Syntaxa dieses Rangs |

`?rank=class` ist die flache Klassenliste, die die Navigation nicht braucht,
eine Suche aber schon. `?rank=alliance` liefert 1326 Zeilen — das ist die
größte Antwort dieses Specs und bleibt ohne Paginierung, weil `SyntaxonRef`
schmal ist und die Zahl fest (kein Wachstum mit Nutzung, nur mit einer neuen
Quellfassung). Kombiniert wirken beide Filter als UND:
`?rank=alliance&life_form_group=bryophyte_lichen` sind die 137 Moos- und
Flechtenverbände.

**Ein weggelassenes `?rank=` sind die 25 Formationen**, nicht alle 1882
Zeilen. Der Handler setzt den Vorgabewert `formation`, bevor er den Port ruft;
der Port kennt keinen Sonderfall für einen leeren Rang. Ein Vorgabewert, der
die ganze Tabelle ausgibt, wäre eine Antwort, die niemand angefordert hat.

Die erlaubten Rang-Werte werden **aus dem Index gelesen**
(`SELECT DISTINCT rank`), nicht als Liste verdrahtet. Teilprojekt A verzichtet
bewusst auf ein `CHECK` auf `rank`, damit ein weiterer Rang eine Datenzeile
sein kann; ein festes Enum an der Leseseite hätte diese Freiheit eine Ebene
höher wieder eingezogen und einen neuen Rang unauffindbar gemacht. Die
Wertemenge von `life_form_group` ist dagegen fest verdrahtet — sie steht als
`CHECK` im Schema und ist keine Erweiterungsstelle.

Ein unbekannter Wert ist `INVALID_QUERY` mit der Liste der erlaubten Werte —
niemals eine leere Liste, die wie „gibt es nicht“ aussieht, obwohl sie
„getippt hast du dich“ heißt. Das ist dieselbe Regel, die `?area=` und
`?vocab=` schon befolgen.

Weil `life_form_group` laut Teilprojekt A nur auf Formationszeilen gespeichert
ist, muss der Filter für `rank != formation` über `parent_id` nach oben joinen
— je Rang unterschiedlich tief (ein Verband drei Stufen, eine Klasse eine).
Die Join-Tiefe darf **nicht** aus dem `rank`-Wert in die SQL-Zeichenkette
gebaut werden: jedes Statement in diesem Projekt ist ein statisches Literal
mit `?`-Platzhaltern, und gosec G201 bricht den Build sonst zu Recht.

Gelöst wird es mit **einem** statischen Statement, das die Wurzel rekursiv
sucht:

```sql
WITH RECURSIVE up(id, root, steps) AS (
  SELECT id, id, 0 FROM syntaxon
  UNION ALL
  SELECT u.id, s.parent_id, u.steps + 1 FROM up u JOIN syntaxon s ON s.id = u.root
  WHERE s.parent_id <> '' AND u.steps < 3
)
SELECT s.id, s.rank, s.name, s.author, s.parent_id, s.alt_code, s.source,
       s.parent_provenance, s.life_form_group
FROM syntaxon s
JOIN up ON up.id = s.id
JOIN syntaxon f ON f.id = up.root AND f.rank = 'formation'
WHERE s.rank = ? AND f.life_form_group = ?
ORDER BY s.id
```

Die Schrittgrenze `u.steps < 3` ist nicht Zierde. Ein `parent_id`-Zykel —
genau der Indexdefekt, den Abschnitt 4 als `INTERNAL_ERROR` behandelt — würde
die Rekursion sonst unbegrenzt laufen lassen: die Abfrage **hängt**, statt zu
scheitern, und das ist das schlechtere Verhalten von beiden. Die Grenze ist
dieselbe gemessene Maximaltiefe wie `maxSyntaxonAncestors`, steht als
statisches Literal im Statement (kein Platzhalter, kein aus Go gebauter Wert)
und ändert für jeden azyklischen Index nichts am Ergebnis.

Ein rekursiver CTE ist neu für dieses Paket, aber die Alternative wäre ein
Statement je Rang — vier fast gleiche Literale, die bei einem weiteren Rang
unvollständig werden, also genau die Verdrahtung, die dieses Spec eine Zeile
weiter oben ablehnt. Ohne Gruppenfilter bleibt es beim einfachen
`WHERE rank = ?`; der CTE läuft nur, wenn wirklich gefiltert wird.

## 4. Port und Repository

```go
// internal/ports/input/services.go, QueryService-Port
Syntaxon(ctx context.Context, id, lang string) (input.SyntaxonDetail, error)
SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string) ([]input.SyntaxonRef, error)

// internal/ports/output/repository.go
// SyntaxonChildren returns the direct children, sorted by id.
SyntaxonChildren(ctx context.Context, parentID string) ([]domain.Syntaxon, error)
// SyntaxonAncestors walks parent_id to the root, OUTERMOST first. Fails
// after maxSyntaxonAncestors steps: a cycle in the parent_id graph is an
// index defect, not a reason for an infinite loop.
SyntaxonAncestors(ctx context.Context, id string) ([]domain.Syntaxon, error)
// SyntaxaByRank filters by rank and, when non-empty, by the life-form
// group of the reachable formation.
SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string) ([]domain.Syntaxon, error)
// HabitatTypeCountForSyntaxon counts the edges without loading them.
HabitatTypeCountForSyntaxon(ctx context.Context, syntaxonID string) (int, error)
// SyntaxonRanks returns the ranks the index actually carries
// (SELECT DISTINCT rank), sorted. The handler validates ?rank= against
// this instead of hard-wiring a list — see section 3.
SyntaxonRanks(ctx context.Context) ([]string, error)
```

`maxSyntaxonAncestors` ist eine Konstante mit Wert **3**: so viele Schritte
braucht ein Verband bis zur Formation (Verband → Ordnung → Klasse →
Formation), und das ist die gemessene Maximaltiefe, kein Vorsichtspuffer. Der
Name sagt „Ahnen“, nicht „Tiefe“, weil die Verwechslung von Knotenzahl (4)
und Schrittzahl (3) sonst vorprogrammiert ist. Ein vierter Schritt bedeutet
einen Zykel oder einen weiteren Rang und wird als `INTERNAL_ERROR` mit der
auslösenden ID gemeldet.

`AllSyntaxa` behält seinen Aufrufer: Teilprojekt A führt den Namensabgleich
für die 16 EEA-eigenen Einheiten weiter, und der liest weiterhin alle Syntaxa.
Die Methode bleibt unverändert bestehen, `SyntaxaByRank` tritt neben sie.

Die neuen Port-Methoden brechen die Testdoubles: `fakeQueryService` in
`internal/adapters/http/handlers_test.go` und `fakeRepo` in
`internal/application/ingest_test.go` müssen sie mitbekommen, sonst
kompiliert keines der beiden Pakete mehr.

## 5. Fehlerbehandlung

| Fall | Antwort |
|---|---|
| `GET /v1/syntaxon/{id}` mit unbekannter ID | `404 NOT_FOUND` |
| `{id}` mit leerem oder nur aus Leerzeichen bestehendem Pfadsegment | `404 NOT_FOUND` (die Route greift gar nicht; kein `INVALID_QUERY`, weil ein Pfadsegment keine Query ist) |
| `?rank=` / `?life_form_group=` mit unbekanntem Wert | `400 INVALID_QUERY`, Nachricht nennt die erlaubten Werte |
| `?rank=` leer weggelassen | Die 25 Formationen |
| Ahnenpfad trifft auf eine fehlende `parent_id`-Zielzeile | `500 INTERNAL_ERROR` „index is inconsistent“, wie schon bei `SyntaxonHabitatTypes` — eine baumelnde Referenz ist ein Indexdefekt, der gemeldet und nicht überbrückt wird |
| Zykel im `parent_id`-Graphen | `500 INTERNAL_ERROR` mit der auslösenden ID |

Ein Verband ohne Kinder und ein Syntaxon ohne Habitattypen sind **keine**
Fehler: `children: []` und `habitat_type_count: 0` sind gültige Antworten und
der Normalfall an der unteren Grenze der Daten.

## 6. Explorer

`internal/adapters/http/explorer.html` rendert heute ausschließlich rohes JSON
in ein `<pre>`; das einzige Syntaxa-Panel verlangt eine manuell eingetippte
ID. Dieses Spec fügt **ein** Panel hinzu, „Syntaxa-Navigation“:

- Startet mit den Formationen (`GET /v1/syntaxa`), als klickbare Liste.
- Ein Klick lädt `GET /v1/syntaxon/{id}` und zeigt drei Blöcke: den
  Ahnenpfad als klickbare Brotkrumenzeile, den aktuellen Namen mit Rang und
  Autorschaft, die Kinder als klickbare Liste.
- Ein Auswahlfeld für `life_form_group` auf der Formationsebene.
- Bei `habitat_type_count > 0` ein Knopf, der die bestehende
  Habitattyp-Route aufruft.

Das ist der erste Explorer-Bereich mit gerendertem statt rohem JSON. Das ist
beabsichtigt und der Grund, warum er als eigenes Panel kommt statt als
Erweiterung des bestehenden: eine Hierarchie ohne Klickpfad vorzuführen wäre
dasselbe Problem wie die API ohne Navigationsroute. Die rohe JSON-Ausgabe
bleibt für jedes andere Panel unverändert; das neue Panel bekommt zusätzlich
den bestehenden Format-Umschalter, damit die Antwort weiterhin im Original
einsehbar ist.

## 7. OpenAPI

Beide neuen Routen ziehen in `internal/adapters/http/openapi.yaml` und die
byte-identische Kopie unter `api/openapi/` ein, mit `SyntaxonDetail` als
neuem Schema und den zwei Query-Parametern als benannte
`components/parameters`-Einträge (`Rank`, `LifeFormGroup`) — der
Vertragstest prüft beide Richtungen und fällt sonst über die nicht
deklarierte Methode.

`GET /v1/syntaxa` braucht `.Methods("GET")` wie jeder andere Pfad, sonst
schlägt der Routen-Vertragstest an. Ein `/v1`-Pfad ohne Pfadparameter ist
dabei nichts Neues — `/v1/typologies` und `/v1/areas` sind es auch —, und
zwischen `/v1/syntaxon/{id}` und `/v1/syntaxon/{id}/habitat-types` gibt es in
mux keine Verdeckung.

Der Vertragstest ist `TestRoutesMatchOpenAPISpec` in
`internal/adapters/http/contract_test.go`; er liest die Spec als Zeilenstrom
und erwartet Pfadschlüssel mit zwei, Methodenschlüssel mit vier Leerzeichen
Einrückung. `TestOpenAPICopiesAreIdentical` prüft zusätzlich die
Byte-Gleichheit beider Kopien. Auch `docs/reference/http-api.md` beschreibt
die Routen und wächst mit.

## 8. Tests

- `internal/adapters/sqlite/read_syntaxon_test.go`: Kinder sortiert, Ahnenpfad
  äußerste-zuerst, Ahnenpfad einer Formation ist leer, Zykel läuft in den
  Tiefenfehler, `SyntaxaByRank` mit und ohne Gruppenfilter, Kantenzählung
  ohne Laden.
- `internal/application/query_test.go`: Detail einer Formation, einer Klasse,
  einer Ordnung, eines Verbands; unbekannte ID auf `NOT_FOUND`; baumelnde
  `parent_id` auf den Inkonsistenzfehler.
- `internal/adapters/http/`: die beiden Routen im Vertragstest, unbekannter
  Filterwert als `INVALID_QUERY` samt erlaubter Werte in der Nachricht,
  `children: []` bleibt im JSON sichtbar (kein `omitempty` — ein fehlendes
  Feld und eine leere Liste sind für einen Client nicht dasselbe).
- Ein Test über den gebauten Index: von jedem der 1326 Verbände führt der
  Ahnenpfad in genau drei Schritten zu einer Formation. Das ist die Zusage aus
  Teilprojekt A an der Oberfläche nachgeprüft, wo der Nutzer sie erlebt.

## 9. Prüfbare Zusagen

- Von `/v1/syntaxa` aus ist jeder der 1326 Verbände durch reines Verfolgen von
  `children` erreichbar, ohne eine ID zu kennen.
- Jede `/v1/syntaxon/{id}`-Antwort trägt einen Ahnenpfad, der bei einer
  Formation endet — für jede ID im Index, nicht nur für die gut gepflegten.
- `children` und `ancestors` sind immer im JSON vorhanden, auch leer.
- Kein Filterwert wird stillschweigend ignoriert: ein unbekannter Wert ist ein
  Fehler mit den erlaubten Werten in der Nachricht.
- Die Antwortzeit einer Navigationsstufe braucht keinen Full-Table-Scan:
  Kinder und Ahnen laufen über `idx_syntaxon_parent`.

## 10. Bewusst außerhalb dieses Specs

- **Deutsche Namen für Syntaxa** — wie in Teilprojekt A begründet eine eigene
  Runde. Die Routen nehmen `?lang=` durch (die bestehende `language(r)`-Logik),
  liefern aber bis dahin englische Namen; das ist keine Sonderregel, sondern
  der Zustand der Daten.
- **Suche nach Syntaxon-Namen.** `SearchSpeciesNames` hat ein Gegenstück für
  Syntaxa verdient, aber das ist eine Suchfunktion, nicht Navigation, und sie
  gehört mit derselben Sorgfalt gebaut (Teilstring, keine Fuzzy-Auflösung —
  Namensauflösung bleibt hostus' Aufgabe).
- **Habitattyp → Formation.** Die Aggregation „welche Formationen berührt
  dieser EUNIS-Typ“ ist über die Kanten und den Ahnenpfad berechenbar, aber
  eine eigene fachliche Frage mit eigener Antwortform.
- **Verbreitungsfilter auf den Navigationsrouten** — Teilprojekt C ergänzt
  `/v1/syntaxa` um `?area=` und `?include=` und beschreibt beides dort. Dieses
  Spec baut die Route so, dass ein weiterer Filter sie nicht umbaut: die
  Parameterprüfung liegt im Handler, nicht im Port.
