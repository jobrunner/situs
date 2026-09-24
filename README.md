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

Habitattyp-Namen liefert der Index auf Englisch (`name_en`) und **auf Deutsch**
(`name_de`, abrufbar über `?lang=de`). Deutsch ist dabei Overlay, nie Ersatz:
`name_en` bleibt die Identität, jede deutsche Zeile trägt ihre
`provenance` ∈ `official` | `curated` | `derived` | `situs` (am aktuellen
Datenstand kommen davon `official`, `situs` und `derived` vor), und abgeleitet
wird
ausschließlich über Qualifier `=`. Am Referenzlauf vom 2026-09-16 gemessen:
**567** Localizations (233 amtlich aus EUR-Lex, 334 von situs verfasst) plus
**29** über `=` abgeleitete Labels.

## Wozu

Eine Exkursions-App nimmt im Gelände Pflanzen auf. Die Habitat-Schätzung über
die Koordinate ist unzuverlässig — ein kleiner Steppen-Fleck inmitten von
feuchtem Wald verliert gegen das große Polygon. Eine Artenliste grenzt die
möglichen Habitattypen **unabhängig davon** ein. Und ist eine Kennart bekannt,
nennt situs die weiteren Kennarten, nach denen sich gezielt suchen lässt.

## Verhältnis zu hostus

[hostus](https://github.com/jobrunner/hostus) löst **Namen** auf
(`verbatim → Konzept`). situs hält das **Habitat-/Vegetationswissen**. Beim
Ingest ruft situs hostus — für die Artnamen der Quellen (→ stabile Konzept-IDs)
und für die Verbreitung dieser Konzepte.

**Die Leseseite ist autark:** `situs serve` braucht hostus nicht. Jede Route
antwortet allein aus dem lokalen SQLite-Index, und die Batch-Route nimmt darum
Konzept-IDs statt verbatim Namen — wer Namen auflösen will, ruft hostus selbst.
Ein Architekturtest hält das fest: die Kompositionswurzel des Serve-Pfads darf
den hostus-Adapter nicht einmal importieren.

## Stand

Im Aufbau. Das Gerüst steht (Go 1.26, hexagonal, Qualitäts-Gates, CI); Ingest
und Lese-API des Fundaments sind implementiert. Darüber hinaus fertig: die
**autarke Leseseite** (Batch über Konzept-IDs, kein hostus zur Laufzeit), der
**Gebietsfilter** (`?area=`, `?only_in_area=`) samt Verbreitungs-Ingest, und die
**Selbstauskunft** `GET /v1/info`. Noch nicht gebaut: Scoring/Ranking, die
ESy-Regel-Engine, der EUNIS-2012-Schlüssel und deutsche Labels (siehe oben).

`make verify` braucht neben der Go-Toolchain ein **`python3`** im Pfad: die
Tests der XLSX→CSV-Pipeline gehören zum kanonischen Grün-Check. Zusätzliche
Python-Pakete sind nicht nötig — die Pipeline benutzt ausschließlich die
Standardbibliothek.

```bash
make verify      # fmt-check, vet, lint, test, pipeline-test, arch, debt, build
make build       # ./situs
./situs serve    # HTTP auf 127.0.0.1:8070
```

Unter `http://localhost:8070/` liegt der API-Explorer — eine selbst-enthaltene
Seite, die jeden Lese-Endpunkt ausprobierbar macht, ohne Netz. Ihr Footer führt
zu `/docs` und `/openapi` und nennt die gebaute Version.

Erreichbar sind außerdem `GET /health/live`, `GET /health/ready`,
`GET /metrics`, `GET /openapi`, `GET /docs` (Swagger-UI, Assets eingebettet —
funktioniert also ohne Netz), `GET /v1/info` und die Lese-Endpunkte:

```bash
# Das Eingabeverzeichnis füllt ein Befehl (siehe docs/how-to/ingest.md);
# --db entfällt, wenn index.path (Default: situs.sqlite) passt.
make ingest-input CSV_DIR=out/ingest-input
./situs ingest --csv-dir out/ingest-input
./situs serve

curl -s 'localhost:8070/v1/info'                        # worauf der Index gebaut ist
curl -s 'localhost:8070/v1/typologies'                  # welche (typology, code) es überhaupt gibt
curl -s 'localhost:8070/v1/areas'                       # nach welchen Gebieten gefiltert werden kann
curl -s 'localhost:8070/v1/habitat-type/eunis@2021/R22?lang=de'
curl -s 'localhost:8070/v1/habitat-type/annex1/6510'
curl -s 'localhost:8070/v1/species/<conceptId>/habitat-types'
curl -s 'localhost:8070/v1/syntaxon/<syntaxonId>/habitat-types'

# Eine ganze Geländeaufnahme in einem Aufruf — Konzept-IDs, keine Namen.
curl -s -X POST localhost:8070/v1/species/habitat-types \
  -H 'Content-Type: application/json' \
  -d '{"concept_ids":["wcvp:concept:2457314","wcvp:concept:2606633"]}'

# Namen, die der Index selbst führt — keine Namensauflösung, dafür ist hostus da.
curl -s 'localhost:8070/v1/species/search?q=fagus&limit=5'

# Zeigerwertanalyse über eine Artenliste, je Vokabular und Dimension getrennt.
curl -s -X POST localhost:8070/v1/species/traits/summary \
  -H 'Content-Type: application/json' \
  -d '{"concept_ids":["wcvp:concept:83891","wcvp:concept:2692970"]}'

# Nur was im Gebiet vorkommt (WGSRPD-Level-3-Code, aus GPS abgeleitet).
# Setzt einen abgeschlossenen Verbreitungs-Ingest voraus — siehe unten.
curl -s 'localhost:8070/v1/habitat-type/eunis@2021/R15/species?area=GER&only_in_area=true'
```

Mit `?area=` trägt jeder Arteneintrag ein dreiwertiges `in_area`: `true`,
`false`, oder das Feld fehlt, wenn es nicht entscheidbar ist (keine Konzept-ID
oder keine Verbreitungsdaten). `only_in_area=true` entfernt nur die
`false`-Einträge — die unentscheidbaren bleiben.

**`?area=` braucht Verbreitungsdaten im Index.** Die holt der Ingest von hostus
(ein Request je Konzept, gedrosselt); war hostus dabei nicht erreichbar, läuft
der Ingest trotzdem durch — der Index ist dann nur nicht filterbar, und **jeder**
Gebietscode ergibt `INVALID_QUERY` (400). Ob Daten da sind, sagt
`areas_with_data` in `GET /v1/info`: `0` heißt nein.

### Fertige Artefakte

Jeder Release liefert Binaries für linux/darwin/windows × amd64/arm64 sowie ein
Multi-Arch-Container-Image. Der Index gehört **nicht** ins Image — er wird
eingehängt, damit dasselbe Image jeden Datenstand bedienen kann:

```bash
docker run --rm -p 8070:8070 \
  -v "$PWD:/data:ro" -e SITUS_INDEX_PATH=/data/situs.sqlite \
  ghcr.io/jobrunner/situs:latest
```

Beides gemessen und nicht vermutet:

- **Der Mount darf `:ro` sein.** `serve` öffnet den Index read-only
  (`file:<pfad>?mode=ro`) und legt keine `-wal`/`-shm`-Beiwagen an — der Ingest
  hinterlässt den Index ohne WAL. Ein fehlender Index bricht den Start ab,
  statt einen leeren anzulegen.
- Das Image läuft als `nonroot` (uid 65532). Für einen `:ro`-Mount genügt, dass
  die Host-Datei für diese uid **lesbar** ist — und jedes Verzeichnis auf dem
  Weg dorthin sein `x`-Bit für sie trägt, sonst scheitert schon das Traversieren
  (ein Index unter einem `0700`-Home reicht nicht). `--user "$(id -u):$(id -g)"`
  braucht es nur noch, wenn im Container geschrieben werden soll (etwa ein
  `situs ingest`).

Das Verzeichnis einhängen, nicht die Datei: ein Bind-Mount auf eine Datei bindet
deren Inode, und ein Index-Tausch auf dem Host käme im Container nie an. Das
Verfahren dafür steht in [`docs/how-to/deploy.md`](docs/how-to/deploy.md).

### CORS für einen Browser-Client

Standardmäßig sendet situs kein CORS — nur der eingebaute Explorer unter `/`
ruft den Dienst same-origin auf. Eine externe Browser-Anwendung (z. B. eine
Exkursions-App) braucht eine explizite Freigabe:

```bash
export SITUS_SERVER_CORS_ALLOWED_ORIGINS='http://localhost:5173,https://*.fieldworksdiary.app'
```

Details (Platzhalter-Regeln, Fehlerverhalten) in
`docs/reference/http-api.md#cors-optional-standardmäßig-aus`.

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
| FFH-Richtlinie Anhang I, deutsche Fassung (EUR-Lex) | amtliche deutsche LRT-Bezeichnungen (CELEX `01992L0043-20130701`, `pipelines/eurlex`) | EU-Recht (Beschluss 2011/833/EU) |
| WGSRPD Level 3, 2. Auflage (TDWG) | Namen der Verbreitungsgebiete für `GET /v1/areas` (`pipelines/wgsrpd`) | TDWG-Standard |
| EUNIS Habitat Factsheets 2021-06-01 (FloraVeg.EU / EUNIS-ESy) | Beschreibung je Habitattyp (`pipelines/floraveg-factsheets`) | CC BY 4.0 |
| Verbreitungskarten der Vegetationsverbände Europas, v2 2024-06-12 (Zenodo) | Syntaxa-Verbreitung je EVC-Territorium (`pipelines/evc-distribution`) | CC BY 4.0 |
| EIVE 1.0 (Zenodo `10.5281/zenodo.7534792`) | Zeigerwerte (`pipelines/eive`) | CC BY 4.0 |
| Tichý et al. 2023, Indicator values v2.0 (Zenodo `10.5281/zenodo.7427088`) | Zeigerwerte (`pipelines/tichy`) | CC BY 4.0 |
| Midolo et al. 2023, v3 (Zenodo `10.5281/zenodo.7116957`) | Zeigerwerte (`pipelines/midolo`) | CC BY 4.0 |

Die vier CC-BY-Zeilen verlangen die Namensnennung ausdrücklich auch für
abgeleitete Daten; der Footer des Explorers nennt sie deshalb alle.

Die Artefakte werden **gepinnt** und **nicht** ins Repo eingecheckt — je
Pipeline dort, wo es zur Quelle passt: URL + Prüfsumme in
`pipelines/eunis/manifest.yaml` und `pipelines/eurovegchecklist/manifest.yaml`,
die CELEX-Kennung `01992L0043-20130701` in `pipelines/eurlex/fetch.sh`
(Prüfsumme wird beim Laden verifiziert), der Commit-SHA in
`pipelines/wgsrpd/build.sh` und die SHA-256-Summe des Factsheet-PDFs in
`pipelines/floraveg-factsheets/build.sh`.
