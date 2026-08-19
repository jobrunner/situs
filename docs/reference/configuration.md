# Konfiguration

Alle Werte kommen aus Umgebungsvariablen mit dem Prefix `SITUS_`; ein
Konfigurationsfile (`--config`) ist optional. Reihenfolge: Umgebung > Datei >
Defaults.

| Schlüssel | Umgebungsvariable | Default | Bedeutung |
|---|---|---|---|
| `server.host` | `SITUS_SERVER_HOST` | `127.0.0.1` | Listen-Adresse |
| `server.port` | `SITUS_SERVER_PORT` | `8080` | Listen-Port |
| `server.read_timeout` | `SITUS_SERVER_READ_TIMEOUT` | `30s` | `http.Server.ReadTimeout`: Frist für das Lesen eines ganzen Requests (`0` = keine, über den Header-Timeout von 10s hinaus) |
| `server.shutdown_timeout` | `SITUS_SERVER_SHUTDOWN_TIMEOUT` | `15s` | Frist für den geordneten Stop |
| `logging.level` | `SITUS_LOGGING_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `logging.format` | `SITUS_LOGGING_FORMAT` | `json` | `json` oder `text` |
| `index.path` | `SITUS_INDEX_PATH` | `situs.sqlite` | Pfad des lokalen SQLite-Index, aus dem die Lese-API antwortet (erzeugt von `situs ingest --db`) |
| `hostus.base_url` | `SITUS_HOSTUS_BASE_URL` | `http://localhost:8081` | Basis-URL des hostus-Namensauflösers (`POST /v1/match`); beim `ingest` der `species_roles.csv` und zur Laufzeit ausschließlich für `POST /v1/species/habitat-types` (verbatim Namen) |
| `hostus.timeout` | `SITUS_HOSTUS_TIMEOUT` | `30s` | Timeout des hostus-HTTP-Clients |
| `hostus.batch_size` | `SITUS_HOSTUS_BATCH_SIZE` | `50` | Namen je `POST /v1/match`. hostus hat ein festes Zeitlimit pro Request, und die Kosten eines Batches hängen an seinem Inhalt: gemessen an den echten ESy-Namen scheiterten 500 Namen (HTTP 500 nach 30s), das teuerste 100er-Fenster brauchte 19,5s, das teuerste 50er-Fenster 16,3s. Wird ein Batch trotzdem nicht beantwortet, halbiert der Client ihn und versucht es erneut (bis zu einer Untergrenze) und protokolliert das |

Abfragen über Konzept-IDs sind autark: sie brauchen hostus nicht. Nur der
Batch-Endpunkt für verbatim Namen ruft hostus zur Laufzeit auf und antwortet mit
`UPSTREAM_UNAVAILABLE`, wenn er nicht erreichbar ist.
