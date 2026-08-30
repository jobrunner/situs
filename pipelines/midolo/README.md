# midolo

Midolo et al. (2023) Disturbance indicator values, Vokabular `midolo2023`,
Version 3. Quelle: Zenodo-Record 10.5281/zenodo.7116957, Datei
`disturbance_indicator_values.csv`.

```bash
./build.sh
```

Erzeugt `output/midolo-canonical.csv`, kanonisches Format wie bei `eive`
(siehe dessen README). `niche_width`/`n_systems` sind bei Midolo nie
gefüllt.

Die Ausgabe muss vor `situs ingest` nach `<csv-dir>/midolo_traits.csv`
kopiert werden.
