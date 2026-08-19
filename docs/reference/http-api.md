# HTTP-API

Die verbindliche Beschreibung ist die OpenAPI-Spezifikation. Sie ist in die
Binary eingebettet und wird unter `GET /openapi` ausgeliefert; eine
byte-identische Kopie liegt in `api/openapi/openapi.yaml` (ein Test erzwingt die
Gleichheit, ein zweiter, dass Routen und Spezifikation sich in beide Richtungen
decken).

Aktueller Stand (Gerüst):

| Route | Zweck |
|---|---|
| `GET /health/live` | Liveness-Probe |
| `GET /health/ready` | Readiness-Probe |
| `GET /metrics` | Prometheus-Metriken |
| `GET /openapi` | diese Spezifikation |
| `GET /v1/info` | Name und Version des Dienstes |

Die Lese-Endpunkte (`/v1/species/...`, `/v1/habitat-type/...`,
`/v1/syntaxon/...`) kommen mit Task 8.

Fehler kommen einheitlich als `{"error":{"code":"...","message":"..."}}` mit den
Codes `INVALID_QUERY`, `NOT_FOUND`, `UNRESOLVABLE`, `UPSTREAM_UNAVAILABLE`,
`INTERNAL_ERROR`.
