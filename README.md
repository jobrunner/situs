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
  (`=`, `<`, `>`, `#`).

Habitattyp-Namen gibt es zusätzlich **auf Deutsch**; für Anhang-I-Typen im
amtlichen Wortlaut der Richtlinie.

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

```bash
make verify      # fmt-check, vet, lint, test, arch, debt, build
make build       # ./situs
./situs serve    # HTTP auf 127.0.0.1:8080
```

Erreichbar sind `GET /health/live`, `GET /health/ready`, `GET /metrics`,
`GET /openapi`, `GET /v1/info` und die Lese-Endpunkte:

```bash
./situs ingest --csv-dir pipelines/eunis/out --db /tmp/situs.sqlite
SITUS_INDEX_PATH=/tmp/situs.sqlite ./situs serve

curl -s 'localhost:8080/v1/habitat-type/eunis@2021/R22?lang=de'
curl -s 'localhost:8080/v1/habitat-type/annex1/6510'
curl -s 'localhost:8080/v1/species/<conceptId>/habitat-types'
curl -s 'localhost:8080/v1/syntaxon/<syntaxonId>/habitat-types'
```

Details in `docs/reference/http-api.md`.

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
| FFH-Richtlinie Anhang I, deutsche Fassung (EUR-Lex) | amtliche deutsche LRT-Bezeichnungen | EU-Recht |

Die Artefakte werden **gepinnt** (URL + Prüfsumme in
`pipelines/eunis/manifest.yaml`) und **nicht** ins Repo eingecheckt.
