# situs

**situs** ist ein lokaler, read-only Dienst für **EUNIS-Habitattypen**.

Er beantwortet Fakten in beide Richtungen:

- **Art → Habitattypen**: In welchen Habitattypen ist diese Pflanze Kennart,
  konstante Art oder dominante Art?
- **Habitattyp → Arten**: Welche Kennarten, konstanten und dominanten Arten
  gehören zu diesem Habitattyp — und welche Pflanzengesellschaften (Syntaxa bis
  Verband)?
- **Crosswalks**: EUNIS 2012 ⇄ 2021 und EUNIS ⇄ **FFH-Lebensraumtyp**
  (Anhang I der FFH-Richtlinie), jeweils mit Abdeckungs-Qualifier
  (`=`, `<`, `>`, `#`, `≈`).

Habitattyp-Namen liefert der Index derzeit **nur auf Englisch** (`name_en`).
Der Mechanismus für deutsche Labels ist gebaut und getestet — Overlay statt
Ersetzung, `provenance` ∈ `official` | `curated` | `derived`, und Ableitung
ausschließlich über Qualifier `=` —, aber es ist noch **keine** deutsche
Namensquelle gepinnt: `localizations.csv` erzeugt bislang niemand, gemessen
sind entsprechend `Localizations: 0` und `DerivedLabels: 0`. Sobald die
amtlichen Anhang-I-Bezeichnungen aus EUR-Lex gepinnt sind, erben **29**
EUNIS-Typen mit `=`-Entsprechung ihr deutsches Label ohne Codeänderung.
`?lang=de` ist bereits bedienbar und antwortet heute ohne `name_de`.

## Wozu

Eine Exkursions-App nimmt im Gelände Pflanzen auf. Die Habitat-Schätzung über
die Koordinate ist unzuverlässig — ein kleiner Steppen-Fleck inmitten von
feuchtem Wald verliert gegen das große Polygon. Eine Artenliste grenzt die
möglichen Habitattypen **unabhängig davon** ein. Und ist eine Kennart bekannt,
nennt situs die weiteren Kennarten, nach denen sich gezielt suchen lässt.

## Verhältnis zu hostus

[hostus](https://github.com/jobrunner/hostus) löst **Namen** auf
(`verbatim → Konzept`). situs hält das **Habitat-/Vegetationswissen**. Beim
Ingest ruft situs hostus, um die Artnamen der Quellen auf stabile Konzept-IDs
zu normalisieren; zur Laufzeit ist situs für Abfragen per Konzept-ID autark.

## Stand

Im Aufbau. Das Gerüst steht (Go 1.26, hexagonal, Qualitäts-Gates, CI); Ingest
und Lese-API des Fundaments sind implementiert.

`make verify` braucht neben der Go-Toolchain ein **`python3`** im Pfad: die
Tests der XLSX→CSV-Pipeline gehören zum kanonischen Grün-Check. Zusätzliche
Python-Pakete sind nicht nötig — die Pipeline benutzt ausschließlich die
Standardbibliothek.

```bash
make verify      # fmt-check, vet, lint, test, pipeline-test, arch, debt, build
make build       # ./situs
./situs serve    # HTTP auf 127.0.0.1:8080
```

Erreichbar sind `GET /health/live`, `GET /health/ready`, `GET /metrics`,
`GET /openapi`, `GET /docs` (Swagger-UI, Assets eingebettet — funktioniert also
ohne Netz), `GET /v1/info` und die Lese-Endpunkte:

```bash
# --db entfällt, wenn index.path (Default: situs.sqlite) passt.
./situs ingest --csv-dir pipelines/eunis/out
./situs serve

curl -s 'localhost:8080/v1/habitat-type/eunis@2021/R22?lang=de'
curl -s 'localhost:8080/v1/habitat-type/annex1/6510'
curl -s 'localhost:8080/v1/species/<conceptId>/habitat-types'
curl -s 'localhost:8080/v1/syntaxon/<syntaxonId>/habitat-types'
```

### Fertige Artefakte

Jeder Release liefert Binaries für linux/darwin/windows × amd64/arm64 sowie ein
Multi-Arch-Container-Image. Der Index gehört **nicht** ins Image — er wird
eingehängt, damit dasselbe Image jeden Datenstand bedienen kann:

```bash
docker run --rm -p 8080:8080 \
  --user "$(id -u):$(id -g)" \
  -v "$PWD:/data" -e SITUS_INDEX_PATH=/data/situs.sqlite \
  ghcr.io/jobrunner/situs:latest
```

Zwei Eigenheiten, die beide gemessen und nicht vermutet sind:

- **Der Index muss schreibbar eingehängt werden**, obwohl der Dienst read-only
  ist: `Open` setzt `journal_mode=WAL`, und WAL verlangt Schreibrechte auf Datei
  und Verzeichnis. Ein `:ro`-Mount scheitert mit
  `attempt to write a readonly database (1544)`.
- Das Image läuft als `nonroot` (uid 65532), die Host-Datei gehört Dir — daher
  `--user`, alternativ `chown 65532` auf dem Index.

Das Image setzt `SITUS_SERVER_HOST=0.0.0.0` — der Config-Default `127.0.0.1` ist
für ein lokales Binary richtig, macht im Container aber jeden gemappten Port
unerreichbar. Die veröffentlichte Dokumentation liegt unter
<https://jobrunner.github.io/situs/> und wird je Release erneuert.

Details in `docs/reference/http-api.md`; die am gepinnten Datenstand
**gemessenen** Kennzahlen in `docs/reference/measured-index.md`.

Fundament-Spec und Implementierungsplan liegen unter `docs/`:

- `docs/superpowers/specs/2026-08-18-situs-foundation-design.md` — Design
- `docs/superpowers/plans/2026-08-19-situs-foundation.md` — Umsetzungsplan
- `docs/research/` — Sondierungen zur Datenlage (EEA/EUNIS 2021, ESy)

## Datenquellen

| Quelle | Liefert | Lizenz |
|---|---|---|
| EUNIS terrestrial habitat classification 2021_1 (EEA) | Habitattypen, Syntaxa-Crosswalk, Versions- und Anhang-I-Crosswalk | EEA-Datenpolitik |
| EUNIS-ESy `Characteristic-species-combinations` (Zenodo) | Kennarten / konstante / dominante Arten je Habitattyp | CC BY 4.0 |
| Euroveg Checklist 2016 | Syntaxonomie (Klasse, Ordnung, Verband) | — |
| FFH-Richtlinie Anhang I, deutsche Fassung (EUR-Lex) | amtliche deutsche LRT-Bezeichnungen — **noch nicht gepinnt**, siehe oben | EU-Recht |

Die Artefakte werden **gepinnt** (URL + Prüfsumme in
`pipelines/eunis/manifest.yaml`) und **nicht** ins Repo eingecheckt.
