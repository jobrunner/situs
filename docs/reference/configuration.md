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
| `hostus.base_url` | `SITUS_HOSTUS_BASE_URL` | `http://localhost:8081` | Basis-URL des hostus-Namensauflösers (`POST /v1/match`); wird nur beim `ingest` der `species_roles.csv` aufgerufen, nie zur Laufzeit |
| `hostus.timeout` | `SITUS_HOSTUS_TIMEOUT` | `30s` | Timeout des hostus-HTTP-Clients |

Weitere Schlüssel (Index-Pfad) kommen mit den Tasks, die sie brauchen.
