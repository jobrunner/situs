# Architektur

situs ist hexagonal aufgebaut (Ports & Adapters), in derselben Form wie hostus.
Abhängigkeiten zeigen nach innen:

```
domain      -> nichts Internes
ports       -> domain
application -> domain, ports
adapters    -> domain, ports        (nicht application, nicht Nachbar-Adapter)
app / cmd   -> alles                (Composition Root)
```

Die Grenzen sind kein Übereinkommen, sondern ein Lint-Gate: `depguard` in
`.golangci.yml` lässt einen verbotenen Import den Build brechen (`make arch`).
`gomodguard_v2` erzwingt zusätzlich die Liste erlaubter Bibliotheken.

| Paket | Rolle |
|---|---|
| `internal/domain` | Werte und Entitäten (`TypologyID`, `HabitatTypeKey`, `Qualifier`, …), kein I/O |
| `internal/ports/input` | treibende Ports: was der Dienst anbietet |
| `internal/ports/output` | getriebene Ports: was der Dienst braucht (`Repository`, `IngestTx`, `NameResolver`, `DistributionSource`, `Tracer`) |
| `internal/application` | Use Cases (Ingest, Query) |
| `internal/adapters/sqlite` | lokaler Index, `modernc.org/sqlite` (CGO-frei) |
| `internal/adapters/hostus` | `NameResolver` (`POST /v1/match`) und `DistributionSource` (`GET /v1/concept/{id}`) — **nur beim Ingest**; ein Test verbietet `internal/app` diesen Import |
| `internal/adapters/http` | HTTP-Adapter (gorilla/mux) + eingebettete OpenAPI |
| `internal/adapters/telemetry` | slog-Handler mit Trace-Korrelation |
| `internal/app` | Composition Root |
| `internal/config` | `SITUS_`-Konfiguration |

## Zwei Wege in denselben Index

Der SQLite-Adapter hat **zwei** Einstiege, und welcher benutzt wird, ist keine
Stilfrage:

| Einstieg | Wer | Was er tut |
|---|---|---|
| `OpenForIngest` | `cmd/situs ingest` | read-write, legt den Index bei Bedarf an, spielt `schema.sql` ein, schaltet WAL ein; dazu `Migrate` für Spalten, die ein älterer Index nicht hat |
| `OpenReadOnly` | `internal/app` (`serve`) | `file:<pfad>?mode=ro`, sonst nichts |

`serve` darf nur den zweiten benutzen. Ein schreibfähiges Handle würde einen
fehlenden Index **anlegen** (grüner Health-Check, `NOT_FOUND` auf alles), die
Datei in den WAL-Modus versetzen und damit selbst einem reinen Leser die
`-wal`/`-shm`-Beiwagen aufzwingen — und ein beschreibbares Verzeichnis
verlangen. Alle drei brechen das Verfahren aus `../how-to/deploy.md`, den Index
unter einem laufenden Container auszutauschen.

Als Importverbot ist diese Regel nicht formulierbar: `internal/app` **muss** den
SQLite-Adapter importieren. Sie wird deshalb an der Verwendung geprüft —
`internal/app/arch_test.go` parst die Kompositionswurzel und lässt nur eine
Allowlist von `sqlite.*`-Bezeichnern zu. Ein neuer schreibfähiger Einstieg muss
dort bewusst eingetragen werden, und genau in diesem Moment muss jemand
begründen, warum das Servieren schreiben können soll.

`FinalizeForServing` schließt den Ingest ab: WAL checkpointen, zurück auf
`journal_mode=DELETE`. Nur der Ingest kann das, weil der Wechsel aus WAL heraus
die einzige Verbindung zur Datenbank verlangt.

**Warum XLSX nicht in der Binary steckt:** eine `.xlsx` ist ein ZIP aus XML, das
die Python-Standardbibliothek liest. Die Konvertierung bleibt deshalb in
`pipelines/eunis/` (bash + `python3`, nur stdlib); der Go-Ingest liest
ausschließlich CSV. Das hält die Abhängigkeitsliste schmal — dieselbe Aufteilung
wie hostus' `pipelines/floraveg/`.
