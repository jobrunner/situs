# How-to

- [Index aufbauen (`situs ingest`)](ingest.md)

**Quellartefakte pinnen und die CSVs erzeugen** ist dokumentiert — allerdings
außerhalb dieser Site, weil es die Pipeline und nicht den Dienst betrifft:
`pipelines/eunis/README.md` im Repo beschreibt Manifest/Pinning, die
Aufrufe und die gemessenen Eigenheiten der Rohdaten.

Noch nicht dokumentiert. Geplant, sobald der jeweilige Teil implementiert
ist:

- Deutsche Labels pflegen und abgeleitete Labels prüfen — beide Quellen sind
  gepinnt und laufen (`pipelines/eurlex` für die amtlichen Anhang-I-Namen,
  `data/localizations-de-situs.csv` für die verfassten EUNIS-Namen; gemessen
  in `../reference/measured-index.md`), ein How-to zum Pflegen fehlt noch
