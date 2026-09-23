#!/usr/bin/env bash
#
# situs collect-ingest-input.sh — füllt das Eingabeverzeichnis für `situs ingest`.
#
# Der Ingest liest ALLE seine Quellen aus einem Verzeichnis, die Pipelines
# schreiben aber jede in ihr eigenes out/ bzw. output/, und mehrere kuratierte
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
DEST="$(cd "$DEST" && pwd)"

# Ein Pipeline-Verzeichnis als Ziel würde Quelle auf Quelle kopieren und den
# Lauf mitten im Sammeln abbrechen. Das Ziel ist ein eigenes Verzeichnis.
case "$DEST" in
  "$REPO"/pipelines/*|"$REPO"/data|"$REPO"/data/*)
    echo "collect-ingest-input: $DEST liegt in den Quellen. Nimm ein eigenes" >&2
    echo "                      Zielverzeichnis, etwa out/ingest-input." >&2
    exit 2 ;;
esac

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
  # FloraVeg.EU-Hierarchie: primäre Quelle für Syntaxa. Ohne sie entsteht ein
  # Index ohne Hierarchie und das darf nicht stillschweigend passieren.
  "pipelines/eurovegchecklist/out/syntaxa_hierarchy.csv:syntaxa_hierarchy.csv"
  "data/syntaxa_formations.csv:syntaxa_formations.csv"
  # Die deutschen Namen der 25 Formationen. Pflicht, nicht optional: ohne sie
  # entsteht ein Index, der auf ?lang=de für jede Formation den englischen
  # Namen zeigt, und zwar lautlos.
  "data/localizations-de-syntaxa.csv:localizations_syntaxa.csv"
)

# Fehlt eine davon, läuft der Ingest trotzdem; die betroffenen Daten fehlen
# dann aber im Index. Das Skript sagt es, statt es geschehen zu lassen.
OPTIONAL=(
  "pipelines/wgsrpd/output/wgsrpd_areas.csv:wgsrpd_areas.csv"
  "pipelines/floraveg-factsheets/output/habitat_descriptions.csv:habitat_descriptions.csv"
  "pipelines/eive/output/eive-canonical.csv:eive_traits.csv"
  "pipelines/tichy/output/tichy-canonical.csv:tichy_traits.csv"
  "pipelines/midolo/output/midolo-canonical.csv:midolo_traits.csv"
  "pipelines/evc-distribution/out/syntaxon_distribution.csv:syntaxon_distribution.csv"
  "pipelines/evc-distribution/out/syntaxon_distribution_coverage.csv:syntaxon_distribution_coverage.csv"
  "pipelines/evc-distribution/out/evc_territories.csv:evc_territories.csv"
)

# Diese beiden erzeugt kein Pipeline-Lauf in diesem Repo: eurosl_crosswalk.csv
# und aggregate_members.csv kommen aus `hostus export-crosswalk`,
# localizations.csv aus pipelines/eurlex (siehe docs/how-to/ingest.md).
EXTERNAL=(
  "eurosl_crosswalk.csv"
  "aggregate_members.csv"
  "localizations.csv"
)

# Erst die verwalteten Namen entfernen: bliebe eine Ausgabe von gestern
# liegen, läse der Ingest sie, ohne dass irgendetwas sie als alt ausweist.
for entry in "${REQUIRED[@]}" "${OPTIONAL[@]}"; do
  rm -f "$DEST/${entry##*:}"
done

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
absent=()
for name in "${EXTERNAL[@]}"; do
  if [ -f "$DEST/$name" ]; then
    printf '  %-34s vorhanden\n' "$name"
  else
    printf '  %-34s FEHLT\n' "$name"
    absent+=("$name")
  fi
done

if [ ${#missing[@]} -gt 0 ]; then
  echo >&2
  echo "collect-ingest-input: Pflichtquellen fehlen:" >&2
  for m in "${missing[@]}"; do echo "  $m" >&2; done
  echo "Führe die zugehörige Pipeline aus (siehe docs/how-to/ingest.md)." >&2
  exit 1
fi

# Ohne eurosl_crosswalk.csv bricht der Ingest ab; ohne localizations.csv
# entsteht ein Index ganz ohne deutsche Labels, und zwar lautlos. Deshalb
# meldet das Skript hier keinen Vollzug, sondern was fehlt.
if [ ${#absent[@]} -gt 0 ]; then
  echo >&2
  echo "collect-ingest-input: das Verzeichnis ist NICHT vollständig." >&2
  for name in "${absent[@]}"; do
    case "$name" in
      eurosl_crosswalk.csv|aggregate_members.csv)
        echo "  $name  <- hostus export-crosswalk --db <hostus.sqlite> --out-dir $DEST" >&2 ;;
      localizations.csv)
        echo "  $name  <- pipelines/eurlex (siehe pipelines/eurlex/README.md)" >&2 ;;
    esac
  done
  echo "Ein Ingest ohne diese Dateien bricht ab oder liefert einen Index ohne" >&2
  echo "Artenrollen beziehungsweise ohne deutsche Labels." >&2
  exit 1
fi

echo
echo "Eingabeverzeichnis bereit: $DEST"
