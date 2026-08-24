#!/usr/bin/env bash
#
# Fetch the German consolidated Habitats Directive from the EU Publications
# Office. Kept separate from extract.py so the test suite never needs the
# network.
#
# The web UI at eur-lex.europa.eu answers HTTP 202 with an empty body to a plain
# curl — it expects a browser session. The Cellar repository is the documented
# machine-readable route and is what a pinned pipeline should use anyway:
# content negotiation on the CELEX identifier, no scraping of a rendered page.
#
# bash 3.2 compatible (that is what macOS ships).
set -euo pipefail

CELEX="${CELEX:-01992L0043-20130701}"
OUT="${1:-eurlex-de.xhtml}"
URL="http://publications.europa.eu/resource/celex/${CELEX}"

echo "fetch: CELEX ${CELEX} -> ${OUT}"
curl -sS -f -L \
  -H "Accept: application/xhtml+xml" \
  -H "Accept-Language: deu" \
  -o "$OUT" \
  "$URL"

# An empty or tiny file means the negotiation failed and returned a stub. Fail
# here rather than letting extract.py report "no Annex I entries" and look like
# a parser problem.
size=$(wc -c < "$OUT" | tr -d ' ')
if [ "$size" -lt 100000 ]; then
  echo "fetch: got only ${size} bytes — content negotiation failed?" >&2
  exit 1
fi

# The German text must actually be German. Without this an English or French
# fallback would be extracted and ingested as "official German".
if ! grep -q "ANHANG I" "$OUT"; then
  echo "fetch: no 'ANHANG I' in the response — wrong language served?" >&2
  exit 1
fi

echo "fetch: ${size} bytes, German Annex I present"
