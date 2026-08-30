# tichy

Tichý et al. (2023) Indicator values, Vokabular `tichy2023`, Version 2.0.
Quelle: Zenodo-Record 10.5281/zenodo.7427088, Datei
`Indicator.values-tables-2022-11-07-Zenodo.v2.xlsx`.

```bash
./build.sh
```

Erzeugt `output/tichy-canonical.csv`, kanonisches Format wie bei `eive`
(siehe dessen README). `niche_width`/`n_systems` sind bei Tichý nie gefüllt.

Voraussetzung: `openpyxl` (`pip install openpyxl`) — `convert.py` liest die
XLSX-Quelldatei damit. Das ist eine bewusste, dokumentierte Abweichung von
der sonst stdlib-only-Pipeline-Konvention (siehe `CLAUDE.md`): der Konverter
wurde unverändert aus hostus übernommen.

Die Ausgabe muss vor `situs ingest` nach `<csv-dir>/tichy_traits.csv`
kopiert werden.
