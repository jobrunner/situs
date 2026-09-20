#!/usr/bin/env bash
#
# situs collect-ingest-input.sh — füllt das Eingabeverzeichnis für `situs ingest`.
#
# Der Ingest liest ALLE seine Quellen aus einem Verzeichnis, die Pipelines
# schreiben aber jede in ihr eigenes out/ bzw. output/, und zwei kuratierte
# Dateien liegen versioniert in data/. Dieses Skript ist die eine Stelle, die
# weiß, was dazugehört. Vorher stand diese Liste nur als Prosa in der
# Anleitung, und eine übersprungene Zeile ergab still einen Index ohne die
# betroffenen Daten: kein Fehler, nur kleinere Zahlen im Report.
#
# Usage: scripts/collect-ingest-input.sh <ziel-verzeichnis>
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="${1:-}"
if [ -z "$DEST" ]; then
  echo "usage: scripts/collect-ingest-input.sh <ziel-verzeichnis>" >&2
  exit 2
fi
mkdir -p "$DEST"

# quelle:zielname. Der Zielname ist der, unter dem der Ingest die Datei sucht;
# er weicht bei den Zeigerwerten bewusst vom Pipeline-Namen ab.
REQUIRED=(
  "pipelines/eunis/out/typologies.csv:typologies.csv"
  "pipelines/eunis/out/habitat_types.csv:habitat_types.csv"
  "pipelines/eunis/out/crosswalks.csv:crosswalks.csv"
  "pipelines/eunis/out/syntaxa.csv:syntaxa.csv"
  "pipelines/eunis/out/habitat_type_syntaxa.csv:habitat_type_syntaxa.csv"
  "pipelines/eunis/out/species_roles.csv:species_roles.csv"
  "data/annex1_descriptions.csv:annex1_descriptions.csv"
  "data/localizations_descriptions.csv:localizations_descriptions.csv"
)

# Fehlt eine davon, läuft der Ingest trotzdem; die betroffenen Daten fehlen
# dann aber im Index. Das Skript sagt es, statt es geschehen zu lassen.
OPTIONAL=(
  "pipelines/wgsrpd/output/wgsrpd_areas.csv:wgsrpd_areas.csv"
  "pipelines/floraveg-factsheets/output/habitat_descriptions.csv:habitat_descriptions.csv"
  "pipelines/eurovegchecklist/out/syntaxa_hierarchy.csv:syntaxa_hierarchy.csv"
  "pipelines/eive/output/eive-canonical.csv:eive_traits.csv"
  "pipelines/tichy/output/tichy-canonical.csv:tichy_traits.csv"
  "pipelines/midolo/output/midolo-canonical.csv:midolo_traits.csv"
)

# Diese beiden erzeugt kein Pipeline-Lauf in diesem Repo: eurosl_crosswalk.csv
# und aggregate_members.csv kommen aus `hostus export-crosswalk`,
# localizations.csv aus pipelines/eurlex (siehe docs/how-to/ingest.md).
EXTERNAL=(
  "eurosl_crosswalk.csv"
  "aggregate_members.csv"
  "localizations.csv"
)

copy() {
  local src="$REPO/${1%%:*}" dst="$DEST/${1##*:}"
  cp "$src" "$dst"
  printf '  %-34s <- %s\n' "${1##*:}" "${1%%:*}"
}

missing=()
echo "Pflichtquellen:"
for entry in "${REQUIRED[@]}"; do
  if [ -f "$REPO/${entry%%:*}" ]; then copy "$entry"; else missing+=("${entry%%:*}"); fi
done

echo "Optionale Quellen:"
for entry in "${OPTIONAL[@]}"; do
  if [ -f "$REPO/${entry%%:*}" ]; then
    copy "$entry"
  else
    printf '  %-34s FEHLT, wird beim Ingest übersprungen\n' "${entry##*:}"
  fi
done

echo "Nicht aus diesem Repo (selbst bereitstellen):"
for name in "${EXTERNAL[@]}"; do
  if [ -f "$DEST/$name" ]; then
    printf '  %-34s vorhanden\n' "$name"
  else
    printf '  %-34s fehlt\n' "$name"
  fi
done

if [ ${#missing[@]} -gt 0 ]; then
  echo >&2
  echo "collect-ingest-input: Pflichtquellen fehlen:" >&2
  for m in "${missing[@]}"; do echo "  $m" >&2; done
  echo "Führe die zugehörige Pipeline aus (siehe docs/how-to/ingest.md)." >&2
  exit 1
fi

echo
echo "Eingabeverzeichnis bereit: $DEST"
