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

// SyntaxonDetail ist ein Syntaxon mit seiner Umgebung: der Weg nach oben und
// die direkten Kinder. Beides ist Teil derselben Frage ("wo bin ich und wohin
// kann ich?") und deshalb Teil derselben Antwort.
type SyntaxonDetail struct {
	SyntaxonRef

	// LifeFormGroup ist "phanerogam" | "bryophyte_lichen" | "algae". Gesetzt
	// ist es nur auf Formationszeilen; auf jeder anderen Ebene trägt es die
	// Gruppe der Formation, die der Ahnenpfad erreicht — dort also ein
	// abgeleiteter Wert, kein gespeicherter.
	LifeFormGroup string `json:"life_form_group,omitempty"`

	// Ancestors ist der Weg zur Wurzel, ÄUSSERSTE zuerst (Formation, dann
	// Klasse, dann Ordnung). Für eine Formation leer. Die Reihenfolge ist
	// festgelegt, damit ein Client sie unverändert als Brotkrumenzeile
	// ausgeben kann.
	Ancestors []SyntaxonRef `json:"ancestors"`

	// Children sind die direkten Kinder, nach ID sortiert. Für einen Verband
	// leer — das ist die untere Grenze der Daten, nicht ein Fehler.
	Children []SyntaxonRef `json:"children"`

	// HabitatTypeCount ist die Anzahl verknüpfter Habitattypen, gemessen.
	// Die Liste selbst holt /v1/syntaxon/{id}/habitat-types; hier steht nur
	// die Zahl, damit ein Client weiß, ob der Abruf sich lohnt.
	HabitatTypeCount int `json:"habitat_type_count"`
}
```

`SyntaxonRef` wächst gegenüber Teilprojekt A nicht weiter — `alt_code`,
`source` und `parent_provenance` sind dort schon eingezogen.

`GET /v1/syntaxa` liefert `[]SyntaxonRef`, kein `SyntaxonDetail`: die
Formationen brauchen weder Ahnenpfad (leer) noch Kinderliste (die holt der
nächste Schritt), und 25 Details mit je 150 Kindern wären eine
Antwort, die niemand angefordert hat.

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

Ein unbekannter Wert ist `INVALID_QUERY` mit der Liste der erlaubten Werte —
niemals eine leere Liste, die wie „gibt es nicht“ aussieht, obwohl sie
„getippt hast du dich“ heißt. Das ist dieselbe Regel, die `?area=` und
`?vocab=` schon befolgen.

Weil `life_form_group` laut Teilprojekt A nur auf Formationszeilen gespeichert
ist, joint der Filter für `rank != formation` über `parent_id` nach oben. Bei
höchstens drei Ebenen ist das ein fester, kurzer Join — und
`idx_syntaxon_parent` aus Teilprojekt A ist genau dafür da.

## 4. Port und Repository

```go
// internal/ports/input/services.go, QueryService-Port
Syntaxon(ctx context.Context, id, lang string) (input.SyntaxonDetail, error)
SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string) ([]input.SyntaxonRef, error)

// internal/ports/output/repository.go
// SyntaxonChildren liefert die direkten Kinder, nach ID sortiert.
SyntaxonChildren(ctx context.Context, parentID string) ([]domain.Syntaxon, error)
// SyntaxonAncestors läuft parent_id bis zur Wurzel, ÄUSSERSTE zuerst. Bricht
// nach maxSyntaxonDepth Schritten mit einem Fehler ab: eine Zykel im
// parent_id-Graphen ist ein Indexdefekt, kein Grund für eine Endlosschleife.
SyntaxonAncestors(ctx context.Context, id string) ([]domain.Syntaxon, error)
// SyntaxaByRank filtert nach Rang und, wenn nichtleer, nach der
// Lebensform-Gruppe der erreichbaren Formation.
SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string) ([]domain.Syntaxon, error)
// HabitatTypeCountForSyntaxon zählt die Kanten, ohne sie zu laden.
HabitatTypeCountForSyntaxon(ctx context.Context, syntaxonID string) (int, error)
```

`maxSyntaxonDepth` ist eine Konstante mit Wert 4 (Formation, Klasse, Ordnung,
Verband). Sie ist kein Vorsichtspuffer, sondern die gemessene Tiefe der
Hierarchie; ein fünfter Schritt bedeutet einen Zykel und wird als
`INTERNAL_ERROR` mit der ID gemeldet, die ihn auslöste.

Die bestehende `AllSyntaxa` verliert ihren einzigen Aufrufer, wenn Teilprojekt
A den Namensabgleich umbaut; sie bleibt als Port-Methode, weil die neue
`SyntaxaByRank` sie mit leerem Rang nicht ersetzt (unterschiedliche
Sortierzusage). Wird sie nach A wirklich nirgends mehr gebraucht, fällt sie
raus — der Debt-Ratchet würde eine ungenutzte exportierte Methode sonst
mitschleppen.

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

`GET /v1/syntaxa` ist der erste `/v1`-Pfad ohne Pfadparameter neben den
Info-Routen; er braucht `.Methods("GET")` wie jeder andere, sonst schlägt der
Routen-Vertragstest an.

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
- **Verbreitungsfilter auf den Navigationsrouten** — Teilprojekt C. Ob
  `/v1/syntaxa?area=` sinnvoll ist, entscheidet sich erst, wenn die
  Verbreitungsdaten im Index liegen.
