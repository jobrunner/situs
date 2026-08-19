# Herkunft der Swagger-UI-Assets

Gepinnt wie jedes andere Fremdartefakt in diesem Repo — Version, Quelle und
Prüfsumme sind hier nachvollziehbar, nicht im Commit-Text.

| | |
|---|---|
| Paket | `swagger-ui-dist` |
| Version | **5.32.14** |
| Tarball | <https://registry.npmjs.org/swagger-ui-dist/-/swagger-ui-dist-5.32.14.tgz> |
| Integrity (sha512, base64) | `nOA2pSQhcmODMUQZpJHYKNuwniDUqcOWGNaSCOoZv12FdOSJ9JxV95HtyRGNMqEBj6h6lCNTy20TgZDYTSuUIg==` |
| Lizenz | Apache-2.0 (siehe `LICENSE`) |
| Übernommen | `swagger-ui.css`, `swagger-ui-bundle.js`, `LICENSE` |

Bewusst **nicht** übernommen: `swagger-ui-standalone-preset.js` (nur für die
URL-Explorer-Leiste, die hier nicht gebraucht wird), die `.map`-Dateien und die
ES-Bundles.

## Warum eingebettet statt CDN

situs ist ein lokaler Dienst; im Feld gibt es kein Netz. Eine `/docs`-Seite, die
ihr JavaScript von einem CDN zieht, ist genau dann leer, wenn sie gebraucht wird.
Der Preis sind ~1,7 MB im Repo und im Binary — bewusst bezahlt.
`TestDocs_IsSelfContained` hält die Eigenschaft fest: die Seite darf keine
externe URL enthalten.

## Aktualisieren

```bash
curl -sSO https://registry.npmjs.org/swagger-ui-dist/-/swagger-ui-dist-<version>.tgz
# Integrity gegen https://registry.npmjs.org/swagger-ui-dist prüfen, dann
tar xzf swagger-ui-dist-<version>.tgz
cp package/swagger-ui.css package/swagger-ui-bundle.js package/LICENSE .
```
Danach diese Datei aktualisieren und `go test ./internal/adapters/http/` laufen lassen.
