# Konfiguration

Alle Werte kommen aus Umgebungsvariablen mit dem Prefix `SITUS_`; ein
Konfigurationsfile (`--config`) ist optional. Reihenfolge: Umgebung > Datei >
Defaults.

| Schlüssel | Umgebungsvariable | Default | Bedeutung |
|---|---|---|---|
| `server.host` | `SITUS_SERVER_HOST` | `127.0.0.1` | Listen-Adresse |
| `server.port` | `SITUS_SERVER_PORT` | `8070` | Listen-Port |
| `server.read_timeout` | `SITUS_SERVER_READ_TIMEOUT` | `30s` | `http.Server.ReadTimeout`: Frist für das Lesen eines ganzen Requests (`0` = keine, über den Header-Timeout von 10s hinaus) |
| `server.shutdown_timeout` | `SITUS_SERVER_SHUTDOWN_TIMEOUT` | `15s` | Frist für den geordneten Stop |
| `logging.level` | `SITUS_LOGGING_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `logging.format` | `SITUS_LOGGING_FORMAT` | `json` | `json` oder `text` |
| `index.path` | `SITUS_INDEX_PATH` | `situs.sqlite` | Pfad des lokalen SQLite-Index, aus dem die Lese-API antwortet. `situs ingest` schreibt ohne `--db` in genau diese Datei, damit Ingest und `serve` nicht auf verschiedene Indizes zeigen |
| `hostus.base_url` | `SITUS_HOSTUS_BASE_URL` | `http://localhost:8080` | **Nur beim `ingest`.** Basis-URL des hostus-Dienstes: `POST /v1/match` für die Namen der `species_roles.csv`, `GET /v1/concept/{id}` für die Verbreitung |
| `hostus.timeout` | `SITUS_HOSTUS_TIMEOUT` | `30s` | **Nur beim `ingest`.** Timeout des hostus-HTTP-Clients |
| `hostus.batch_size` | `SITUS_HOSTUS_BATCH_SIZE` | `50` | **Nur beim `ingest`.** Namen je `POST /v1/match`. hostus hat ein festes Zeitlimit pro Request, und die Kosten eines Batches hängen an seinem Inhalt: gemessen an den echten ESy-Namen scheiterten 500 Namen (HTTP 500 nach 30s), das teuerste 100er-Fenster brauchte 19,5s, das teuerste 50er-Fenster 16,3s. Wird ein Batch trotzdem nicht beantwortet, halbiert der Client ihn und versucht es erneut (bis zu einer Untergrenze) und protokolliert das |
| `hostus.entry_backbone` | `SITUS_HOSTUS_ENTRY_BACKBONE` | `wcvp` | **Nur beim `ingest`.** Taxonomisches Backbone, gegen das hostus matcht. Die gepinnten ESy-Namen sind Vaskularpflanzen, was `wcvp` abdeckt; eine hostus-Instanz auf einem anderen Backbone ist damit konfigurierbar statt neu zu kompilieren |

Die vier `SITUS_HOSTUS_*`-Schlüssel sind **ingest-only**. `situs serve` liest sie
nicht und braucht hostus nicht: die ganze Lese-API antwortet allein aus dem
lokalen SQLite-Index. Ein Architekturtest hält das fest — die Kompositionswurzel
des Serve-Pfads (`internal/app`) darf den hostus-Adapter nicht einmal
importieren. Ein hostus-Ausfall kann darum zur Laufzeit auch keinen Fehlercode
mehr erzeugen; `UPSTREAM_UNAVAILABLE` existiert nicht.

Auf welcher Backbone der Index tatsächlich steht, sagt `GET /v1/info` — gemessen
an den vorhandenen Konzept-IDs, nicht aus dieser Konfiguration abgelesen.

**Vorsicht bei `hostus.entry_backbone`:** das Präfix, das
`POST /v1/species/habitat-types` akzeptiert, ist fest einkompiliert (`wcvp:`).
Zeigt der Ingest auf ein anderes Backbone, antwortet die Batch-Route für **jede**
ID `unknown_backbone`, während `GET /v1/species/{conceptId}/habitat-types` weiter
funktioniert. Der Ingest vergleicht darum am Ende die Backbones im fertigen Index
mit diesem Präfix und **warnt** bei Abweichung (er bricht nicht ab — die
Umstellung kann gewollt sein).
