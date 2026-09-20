#!/usr/bin/env bash
# EUNIS Habitat Factsheets (FloraVeg.EU / EUNIS-ESy) -> habitat_descriptions.csv.
#
# Source: Chytry et al., "EUNIS Habitat Factsheets", version 2021-06-01,
# distributed with the EUNIS-ESy expert system (Zenodo
# doi:10.5281/zenodo.4812736, CC-BY-4.0). Cite Chytry et al. (2020), Applied
# Vegetation Science 23: 648-675, https://doi.org/10.1111/avsc.12519.
#
# The document is pinned by SHA-256, not by URL alone: the file name carries a
# version, but the host may replace its content, and a silently changed
# description set is exactly the drift this check exists to catch.
#
# Needs poppler's pdftohtml (checked below). It is an external CLI tool, not a
# Python library -- the stdlib-only rule for pipelines stays intact.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

SOURCE_URL="https://files.ibot.cas.cz/cevs/downloads/floraveg/Habitat-factsheets-EUNIS-habitats-2021-06-01.pdf"
SOURCE_SHA256="a414101f76df200124aeae935e9861863d9c30b66e583d2a93542200a0a6dae1"

if ! command -v pdftohtml >/dev/null 2>&1; then
  echo "factsheets: pdftohtml (poppler) is required but not installed." >&2
  echo "            macOS: brew install poppler   Debian/Ubuntu: apt install poppler-utils" >&2
  exit 1
fi

CACHE_DIR="${SCRIPT_DIR}/.cache"
OUT_DIR="${SCRIPT_DIR}/output"
mkdir -p "${CACHE_DIR}" "${OUT_DIR}"

SRC_PATH="${CACHE_DIR}/${SOURCE_SHA256}-factsheets.pdf"
OUT_PATH="${OUT_DIR}/habitat_descriptions.csv"

if [[ -f "${SRC_PATH}" ]]; then
  echo "factsheets: using cached ${SRC_PATH}"
else
  echo "factsheets: downloading ${SOURCE_URL}"
  curl -fsSL "${SOURCE_URL}" -o "${SRC_PATH}"
fi

# shasum ships with macOS, sha256sum with coreutils on Linux — neither is
# guaranteed on the other, same fallback as pipelines/eurlex/fetch.sh.
if command -v shasum >/dev/null 2>&1; then
  ACTUAL="$(shasum -a 256 "${SRC_PATH}" | cut -d' ' -f1)"
elif command -v sha256sum >/dev/null 2>&1; then
  ACTUAL="$(sha256sum "${SRC_PATH}" | cut -d' ' -f1)"
else
  echo "factsheets: neither shasum nor sha256sum found — cannot verify the pin." >&2
  exit 1
fi
if [[ "${ACTUAL}" != "${SOURCE_SHA256}" ]]; then
  echo "factsheets: checksum mismatch — the pinned document changed." >&2
  echo "            expected ${SOURCE_SHA256}" >&2
  echo "            actual   ${ACTUAL}" >&2
  echo "            Inspect the new file and bump SOURCE_SHA256 deliberately." >&2
  rm -f "${SRC_PATH}"
  exit 1
fi
echo "factsheets: checksum verified"

python3 "${SCRIPT_DIR}/convert.py" "${SRC_PATH}" "${OUT_PATH}"

echo "factsheets: description CSV written to ${OUT_PATH}"
