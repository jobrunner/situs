#!/usr/bin/env bash
# Interpretation Manual of European Union Habitats, EUR 28 -> annex1_source.csv.
#
# Source: European Commission, DG Environment (April 2013), the official
# interpretation of the Annex I habitat types. The Commission's own historic
# link (ec.europa.eu/environment/nature/legislation/habitatsdirective/docs/)
# returns 404 today; the file is served from the EEA's Central Data Repository,
# which is EU infrastructure rather than a third-party mirror. The Spanish
# ministry's copy was verified to be byte-identical (same SHA-256).
#
# Reuse under Decision 2011/833/EU with attribution; cite as
# "European Commission (2013), Interpretation Manual of European Union
# Habitats, EUR 28".
#
# WHAT THIS IS FOR: the CSV is WORKING MATERIAL, not what situs serves. The
# published Annex I descriptions are written from it together with the EUNIS
# crosswalk and the index's own species data, and carry provenance `situs`.
# Pinning the source by checksum is what makes that derivation checkable.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

SOURCE_URL="https://cdr.eionet.europa.eu/help/natura2000/Documents/Int_Manual_EU28.pdf"
SOURCE_SHA256="7a7193afb181d6a057df6c705ba60413847aebea7a0eb98330c78d064022efde"

if ! command -v pdftotext >/dev/null 2>&1; then
  echo "eur28: pdftotext (poppler) is required but not installed." >&2
  echo "       macOS: brew install poppler   Debian/Ubuntu: apt install poppler-utils" >&2
  exit 1
fi

CACHE_DIR="${SCRIPT_DIR}/.cache"
OUT_DIR="${SCRIPT_DIR}/output"
mkdir -p "${CACHE_DIR}" "${OUT_DIR}"

SRC_PATH="${CACHE_DIR}/${SOURCE_SHA256}-eur28.pdf"
OUT_PATH="${OUT_DIR}/annex1_source.csv"

if [[ -f "${SRC_PATH}" ]]; then
  echo "eur28: using cached ${SRC_PATH}"
else
  echo "eur28: downloading ${SOURCE_URL}"
  curl -fsSL "${SOURCE_URL}" -o "${SRC_PATH}"
fi

# shasum ships with macOS, sha256sum with the GNU coreutils; neither is
# guaranteed on the other, same fallback as pipelines/eurlex/fetch.sh.
if command -v shasum >/dev/null 2>&1; then
  ACTUAL="$(shasum -a 256 "${SRC_PATH}" | cut -d' ' -f1)"
elif command -v sha256sum >/dev/null 2>&1; then
  ACTUAL="$(sha256sum "${SRC_PATH}" | cut -d' ' -f1)"
else
  echo "eur28: neither shasum nor sha256sum found — cannot verify the pin." >&2
  exit 1
fi
if [[ "${ACTUAL}" != "${SOURCE_SHA256}" ]]; then
  echo "eur28: checksum mismatch — the pinned document changed." >&2
  echo "       expected ${SOURCE_SHA256}" >&2
  echo "       actual   ${ACTUAL}" >&2
  echo "       Inspect the new file and bump SOURCE_SHA256 deliberately." >&2
  rm -f "${SRC_PATH}"
  exit 1
fi
echo "eur28: checksum verified"

python3 "${SCRIPT_DIR}/convert.py" "${SRC_PATH}" "${OUT_PATH}"

echo "eur28: working material written to ${OUT_PATH}"
