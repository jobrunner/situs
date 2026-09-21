#!/usr/bin/env bash
# EVC alliance distribution -> syntaxon_distribution.csv + coverage + territories.
#
# Source: Zenodo record 11580949, version 2.0 (2024-06-12), CC-BY 4.0.
# Cite BOTH Preislerová et al. (2022) Appl Veg Sci 25: e12642 and
# Preislerová et al. (2024) Appl Veg Sci 27: e12766 — see manifest.yaml.
#
# Pinned to a VERSIONED record file, never to the record's "latest": the
# territory set and the alliance codes are the identity of what the index
# stores, and they must not change under a rebuild without somebody bumping
# this line.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

FILE="Distribution-maps-of-vegetation-alliances-in-Europe_ver-2_2024-06-12_database.xlsx"
SOURCE_URL="https://zenodo.org/records/11580949/files/${FILE}?download=1"
SHA256="f395b2ce984148dd05d5ec661dfc616f668f6e20c43f3cfb0f1de0362b75665e"

ART_DIR="${SCRIPT_DIR}/artifacts"
OUT_DIR="${SCRIPT_DIR}/out"
mkdir -p "${ART_DIR}" "${OUT_DIR}"
SRC_PATH="${ART_DIR}/${FILE}"

if [[ ! -f "${SRC_PATH}" ]]; then
  echo "evc-distribution: downloading ${SOURCE_URL}"
  curl -fsSL "${SOURCE_URL}" -o "${SRC_PATH}"
fi

# The checksum is verified on EVERY run, not only after a download: a cached
# artifact from a different record version would otherwise be converted while
# the run claims the pinned one.
if command -v shasum >/dev/null 2>&1; then
  echo "${SHA256}  ${SRC_PATH}" | shasum -a 256 -c -
else
  echo "${SHA256}  ${SRC_PATH}" | sha256sum -c -
fi

python3 "${SCRIPT_DIR}/xlsx_to_csv.py" --xlsx "${SRC_PATH}" --out-dir "${OUT_DIR}"

echo "evc-distribution: CSVs written to ${OUT_DIR}"
