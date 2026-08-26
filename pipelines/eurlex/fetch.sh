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
# TRANSPORT. Switching the scheme to https is not enough on its own, and that is
# measured, not assumed: the CELEX resource answers 303 and its Location points
# at plain **http**, so `curl -L` silently downgrades and the document arrives
# unencrypted. `--proto-redir =https` refuses the downgrade — and then the fetch
# fails outright. So the redirect is resolved in one step, its target is forced
# to https (the same URL serves the identical bytes over TLS, verified), and the
# body is fetched with `--proto '=https'` so no further downgrade is possible.
#
# INTEGRITY. Transport security alone would still not tell us we got the document
# we pinned. EXPECT_SHA256 does: it fails on a MITM *and* on the EU silently
# republishing the consolidated text, which is the whole point of pinning. A
# change must be a visible event that someone updates on purpose.
#
# bash 3.2 compatible (that is what macOS ships).
set -euo pipefail

CELEX="${CELEX:-01992L0043-20130701}"
OUT="${1:-eurlex-de.xhtml}"
# Digest of the German XHTML of CELEX 01992L0043-20130701, measured 2026-08-24.
# Set EXPECT_SHA256= (empty) only to explore a new source; never in CI.
EXPECT_SHA256="${EXPECT_SHA256:-308cf99100bbe0bffc0a72b6288e1513358b8b776ec32ca3786ee9571b95e2a6}"

ACCEPT="Accept: application/xhtml+xml"
LANGUAGE="Accept-Language: deu"
CELEX_URL="https://publications.europa.eu/resource/celex/${CELEX}"

echo "fetch: CELEX ${CELEX} -> ${OUT}"

# Step 1: resolve the redirect over TLS, without following it.
location=$(curl -sS -f --proto '=https' -o /dev/null -D - \
  -H "$ACCEPT" -H "$LANGUAGE" "$CELEX_URL" \
  | tr -d '\r' | awk 'tolower($1) == "location:" { print $2; exit }')

if [ -z "$location" ]; then
  echo "fetch: no Location header from ${CELEX_URL} — content negotiation changed?" >&2
  exit 1
fi

# Step 2: force the target to https. The office hands out an http URL here; the
# same path serves byte-identical content over TLS.
target="https://${location#*://}"

# Step 3: fetch the body, refusing any protocol but https.
curl -sS -f --proto '=https' -H "$ACCEPT" -o "$OUT" "$target"

# An empty or tiny file means the negotiation failed and returned a stub. Fail
# here rather than letting extract.py report "no Annex I entries" and look like
# a parser problem.
size=$(wc -c < "$OUT" | tr -d ' ')
if [ "$size" -lt 100000 ]; then
  echo "fetch: got only ${size} bytes from ${target} — negotiation failed?" >&2
  exit 1
fi

# The German text must actually be German. Without this an English or French
# fallback would be extracted and ingested as "official German".
if ! grep -q "ANHANG I" "$OUT"; then
  echo "fetch: no 'ANHANG I' in the response — wrong language served?" >&2
  exit 1
fi

if [ -n "$EXPECT_SHA256" ]; then
  if command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$OUT" | cut -d' ' -f1)
  else
    actual=$(sha256sum "$OUT" | cut -d' ' -f1)
  fi
  if [ "$actual" != "$EXPECT_SHA256" ]; then
    echo "fetch: digest mismatch for CELEX ${CELEX}" >&2
    echo "  expected ${EXPECT_SHA256}" >&2
    echo "  actual   ${actual}" >&2
    echo "  The source changed or the transfer was tampered with. Review the diff," >&2
    echo "  then update EXPECT_SHA256 in this script on purpose." >&2
    exit 1
  fi
  echo "fetch: digest verified"
fi

echo "fetch: ${size} bytes over https, German Annex I present"
