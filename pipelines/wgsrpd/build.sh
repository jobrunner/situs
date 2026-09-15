#!/usr/bin/env bash
# WGSRPD level 3 area names -> wgsrpd_areas.csv.
#
# Source: tdwg/wgsrpd, "109-488-1-ED/2nd Edition/tblLevel3.txt" -- the TDWG
# World Geographical Scheme for Recording Plant Distributions, 2nd edition.
# The level3/ directory of that repository holds only shapefiles; the names
# live in the 2nd-edition tables. Pinned to a COMMIT, never to master: the
# names are the identity of the codes situs already stores, and they must not
# change under a rebuild without somebody bumping this line.
#
# Public domain / freely redistributable (TDWG standard); cite Brummitt (2001),
# World Geographical Scheme for Recording Plant Distributions, 2nd ed.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

PIN="9fb34dfc90eadd5f26439283c818067c9f06371f"
SOURCE_URL="https://raw.githubusercontent.com/tdwg/wgsrpd/${PIN}/109-488-1-ED/2nd%20Edition/tblLevel3.txt"

CACHE_DIR="${SCRIPT_DIR}/.cache"
OUT_DIR="${SCRIPT_DIR}/output"
mkdir -p "${CACHE_DIR}" "${OUT_DIR}"

# The cache path carries the PIN: keyed by filename alone, bumping PIN above
# would silently do nothing on any machine with a warm cache — the run would
# claim the new commit and convert the old file.
SRC_PATH="${CACHE_DIR}/${PIN}-tblLevel3.txt"
OUT_PATH="${OUT_DIR}/wgsrpd_areas.csv"

if [[ -f "${SRC_PATH}" ]]; then
  echo "WGSRPD: using cached ${SRC_PATH}"
else
  echo "WGSRPD: downloading ${SOURCE_URL}"
  curl -fsSL "${SOURCE_URL}" -o "${SRC_PATH}"
fi

python3 "${SCRIPT_DIR}/convert.py" "${SRC_PATH}" "${OUT_PATH}"

echo "WGSRPD: area CSV written to ${OUT_PATH}"
