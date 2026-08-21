#!/usr/bin/env bash
#
# situs mutation-gate.sh — per-package mutation thresholds (RATCHET, raise-only).
# Usage: mutation-gate.sh [package ...]     (default: every package in the module)
#
# Runs gremlins once per package — never with a `...` wildcard, which gremlins
# does not expand and which silently produced zero mutants for the whole
# foundation. Fails when a package is below its threshold, is missing from
# .mutation-thresholds, or produces no mutants without declaring NONE.
set -euo pipefail

# LC_ALL=C forces a dot decimal separator so awk's numeric compares work in
# comma-locale environments (e.g. de_DE would stringify "98,15").
export LC_ALL=C

THRESHOLDS="${THRESHOLDS:-.mutation-thresholds}"
GREMLINS="${GREMLINS:-gremlins}"
MODULE="$(go list -m)"

[ -f "$THRESHOLDS" ] || { echo "mutation-gate: thresholds not found: $THRESHOLDS" >&2; exit 2; }
command -v "$GREMLINS" >/dev/null 2>&1 || { echo "mutation-gate: $GREMLINS not on PATH" >&2; exit 2; }

# Package list: the arguments, or every package in the module. Read with a plain
# `while read` loop, not mapfile: mapfile is a bash 4 builtin and the bash macOS
# ships is 3.2, so it would abort this script on the very platform the local run
# advertises.
if [ "$#" -gt 0 ]; then
  PACKAGES=("$@")
else
  PACKAGES=()
  while IFS= read -r p; do
    [ -n "$p" ] && PACKAGES+=("$p")
  done < <(go list ./... | sed "s|^${MODULE}/||;s|^${MODULE}\$|.|")
fi

OUT="$(mktemp)"
trap 'rm -f "$OUT"' EXIT

lookup() { # lookup <pkg> -> "<efficacy> <mcover>" | "NONE" | ""
  grep -vE '^\s*#|^\s*$' "$THRESHOLDS" | awk -v p="$1" '$1 == p { $1 = ""; sub(/^ +/, ""); print; exit }'
}

fail=0
for pkg in "${PACKAGES[@]}"; do
  want="$(lookup "$pkg")"
  if [ -z "$want" ]; then
    printf "%-42s   ▼ NOT IN %s (add it, with measured numbers or NONE)\n" "$pkg" "$THRESHOLDS"
    fail=1; continue
  fi

  "$GREMLINS" unleash "./$pkg" >"$OUT" 2>&1 || true

  if ! grep -q "Mutation testing completed" "$OUT"; then
    # Either gremlins errored, or it generated nothing at all. "No results to
    # report." is the vacuous-gate signature and must never read as a pass.
    if grep -q "No results to report" "$OUT"; then
      if [ "$want" = "NONE" ]; then
        printf "%-42s   n/a   (no mutants, as declared)\n" "$pkg"; continue
      fi
      printf "%-42s   ▼ NO MUTANTS GENERATED — the gate would measure nothing here\n" "$pkg"
      fail=1; continue
    fi
    printf "%-42s   ▼ gremlins failed:\n" "$pkg"; sed 's/^/      /' "$OUT"
    fail=1; continue
  fi

  eff="$(awk '/^Test efficacy:/ { gsub(/%/, "", $3); print $3; exit }' "$OUT")"
  mcov="$(awk '/^Mutator coverage:/ { gsub(/%/, "", $3); print $3; exit }' "$OUT")"
  counts="$(awk '/^Killed:/ { print; exit }' "$OUT")"

  if [ "$want" = "NONE" ]; then
    printf "%-42s   ▼ DECLARED NONE but generated mutants (%s) — give it real thresholds\n" "$pkg" "$counts"
    fail=1; continue
  fi

  read -r mineff minmcov <<<"$want"
  status=""
  if awk -v v="$eff" -v m="$mineff" 'BEGIN{exit !(v < m)}'; then status="efficacy"; fi
  if awk -v v="$mcov" -v m="$minmcov" 'BEGIN{exit !(v < m)}'; then status="${status:+$status+}mcover"; fi

  if [ -n "$status" ]; then
    printf "%-42s eff %6s%%/%s%%  mcov %6s%%/%s%%  ▼ BELOW THRESHOLD (%s)\n" \
      "$pkg" "$eff" "$mineff" "$mcov" "$minmcov" "$status"
    grep -E "^\s+(LIVED|NOT COVERED)" "$OUT" | sed 's/^/      /' || true
    fail=1
  else
    printf "%-42s eff %6s%% >= %s%%   mcov %6s%% >= %s%%\n" "$pkg" "$eff" "$mineff" "$mcov" "$minmcov"
  fi
done

[ "$fail" -eq 0 ] && echo "mutation-gate: OK"
exit "$fail"
