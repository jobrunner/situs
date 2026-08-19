#!/usr/bin/env bash
#
# situs coverage-gate.sh — per-package statement-coverage floors (RATCHET, raise-only).
# Usage: coverage-gate.sh <coverage.out>
#
# Reads a `go test -coverprofile` file, computes per-package coverage, and fails
# if any floored package (from .coverage-floors) drops below its floor. Raise
# floors as coverage improves; never lower without justification.
set -euo pipefail

# LC_ALL=C forces a dot decimal separator so awk's numeric string compares work
# in comma-locale environments (e.g. de_DE would stringify "100,0").
export LC_ALL=C

PROFILE="${1:?usage: coverage-gate.sh <coverage.out>}"
FLOORS="${FLOORS:-.coverage-floors}"
MODULE="$(go list -m 2>/dev/null || echo "")"

[ -f "$PROFILE" ] || { echo "coverage-gate: profile not found: $PROFILE" >&2; exit 2; }
[ -f "$FLOORS" ]  || { echo "coverage-gate: floors not found: $FLOORS" >&2; exit 2; }

BYPKG="$(mktemp)"
trap 'rm -f "$BYPKG"' EXIT

# Aggregate statements + covered statements per package (and a global TOTAL).
awk -v module="$MODULE/" '
  NR == 1 && $1 == "mode:" { next }
  {
    path = $1; sub(/:.*/, "", path); sub(module, "", path)
    pkg = path; sub(/\/[^\/]*$/, "", pkg)
    stmts = $2; cnt = $3
    tot[pkg] += stmts; gtot += stmts
    if (cnt > 0) { cov[pkg] += stmts; gcov += stmts }
  }
  END {
    for (p in tot) printf "%s %d %d\n", p, cov[p], tot[p]
    printf "TOTAL %d %d\n", gcov, gtot
  }
' "$PROFILE" > "$BYPKG"

fail=0
# For each floored package, look up its aggregated numbers and compare.
while read -r pkg floor; do
  [ -z "$pkg" ] && continue
  case "$pkg" in \#*) continue ;; esac
  # A floored package missing from the profile is a failure, not a pass: it was
  # renamed, deleted or excluded from the test run, and skipping it silently turns
  # the ratchet off for exactly the package someone just moved. Distinct from a
  # package that genuinely has no statements.
  if ! line=$(grep -E "^${pkg} " "$BYPKG"); then
    printf "%-42s   ▼ NOT IN PROFILE (renamed, deleted, or untested?)\n" "$pkg"; fail=1; continue
  fi
  read -r _ c t <<<"$line"
  [ "$t" -eq 0 ] && { printf "%-42s   n/a   (no statements)\n" "$pkg"; continue; }
  pct=$(awk -v c="$c" -v t="$t" 'BEGIN{printf "%.1f", 100*c/t}')
  if awk -v p="$pct" -v f="$floor" 'BEGIN{exit !(p < f)}'; then
    printf "%-42s %6s%% < %s%%  ▼ BELOW FLOOR\n" "$pkg" "$pct" "$floor"; fail=1
  else
    printf "%-42s %6s%% >= %s%%\n" "$pkg" "$pct" "$floor"
  fi
done < <(grep -vE '^\s*#|^\s*$' "$FLOORS")

[ "$fail" -eq 0 ] && echo "coverage-gate: OK"
exit "$fail"
