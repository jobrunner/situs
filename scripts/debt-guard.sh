#!/usr/bin/env bash
#
# situs debt-guard.sh — fast, test-free technical-debt RATCHET checks.
# Runs in the git pre-commit hook and CI.
#
#   1. Suppression budget: total #nosec + //nolint may not exceed the baseline
#      in .debt-budget (ratchet DOWN only — a new suppression forces either a
#      fix, or a justified bump of the baseline in the PR).
#   2. No new debt markers: TODO/FIXME/HACK/XXX comment markers stay at zero
#      (track debt in docs, not in the tree).
#
# Add project-specific guards (like "no hardcoded X in package Y") as extra
# blocks following the same pattern.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
BUDGET_FILE=".debt-budget"
status=0

[ -f "$BUDGET_FILE" ] || { echo "debt-guard: baseline file not found: $BUDGET_FILE" >&2; exit 2; }

# count <pattern> — matching directive lines in first-party *.go. Tolerant of
# zero matches (grep exits 1 with no hits, which would abort under pipefail).
count() {
  { grep -rn "$1" --include='*.go' . || true; } \
    | { grep -vc '/\.go/mod/' || true; } | tr -d ' '
}

# 1. Suppression budget.
nosec=$(count '#nosec')
nolint=$(count '//nolint')
total=$((nosec + nolint))
baseline=$(grep -vE '^\s*#|^\s*$' "$BUDGET_FILE" | head -1 | tr -d ' ')

echo "suppressions: #nosec=$nosec //nolint=$nolint total=$total (baseline $baseline)"
if [ "$total" -gt "$baseline" ]; then
  echo "  ▼ debt-guard: FAIL — suppressions grew past the baseline." >&2
  echo "    Remove a suppression (preferred), or justify a bump in .debt-budget." >&2
  status=1
elif [ "$total" -lt "$baseline" ]; then
  echo "  ✓ suppressions dropped — lower the baseline in .debt-budget to $total to lock it in."
fi

# 2. No new debt markers (leading-marker form only, so prose doesn't trip it).
markers=$(grep -rnE '//[[:space:]]*(TODO|FIXME|HACK|XXX)([[:space:]:(]|$)' --include='*.go' . \
  | grep -v '/\.go/mod/' || true)
if [ -n "$markers" ]; then
  echo "  ▼ debt-guard: FAIL — debt markers found (track in docs instead):" >&2
  echo "$markers" | sed 's/^/      /' >&2
  status=1
else
  echo "debt markers: none"
fi

[ "$status" -eq 0 ] && echo "debt-guard: OK"
exit "$status"
