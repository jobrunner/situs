# eive

EIVE 1.0 (Dengler et al. 2023) — europaweite Zeigerwerte (M/N/R/L/T),
Neuberechnung der klassischen Ellenberg-Werte. Quelle: Zenodo-Record
10.5281/zenodo.7534792, Datei `EIVE_Paper_1.0_SM_08.xlsx`, Sheet
"mainTable". CC-BY-4.0 — jede Weiterverwendung muss Dengler et al. (2023),
Vegetation Classification and Survey 4: 7-29 (https://doi.org/10.3897/VCS.98324)
zitieren.

```bash
./build.sh
```

Lädt (oder nutzt einen Cache), konvertiert nach `output/eive-canonical.csv`
im kanonischen, **pipe-getrennten** Format
`taxon|vocab|vocab_version|dim|value|niche_width|n_systems`, das
`situs ingest` einliest. `niche_width`/`n_systems` sind bei EIVE immer
gefüllt.
