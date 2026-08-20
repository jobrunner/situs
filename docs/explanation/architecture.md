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

**Warum XLSX nicht in der Binary steckt:** eine `.xlsx` ist ein ZIP aus XML, das
die Python-Standardbibliothek liest. Die Konvertierung bleibt deshalb in
`pipelines/eunis/` (bash + `python3`, nur stdlib); der Go-Ingest liest
ausschließlich CSV. Das hält die Abhängigkeitsliste schmal — dieselbe Aufteilung
wie hostus' `pipelines/floraveg/`.
