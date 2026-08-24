#!/usr/bin/env python3
"""Merge the situs-authored German names into the localizations.csv the Go
ingest reads.

Two sources, one output, because the ingest has one contract:

  - the official Annex I names extracted by extract.py (provenance=official)
  - data/localizations-de-situs.csv, hand-written (provenance=situs)

The authored file carries ONE row per habitat type with the English original
beside the translation, because that is the only shape in which a translation can
be reviewed without querying the index. name_en is a control column and is
dropped here; it never reaches the index.

An empty vernacular_de emits NO vernacular row. Absent is information — an empty
value would assert that the type has an established German term that happens to
be the empty string.
"""
import argparse
import csv
import io
import json
import sys

COLUMNS = ["entity_type", "entity_key", "lang", "field", "value", "source", "provenance"]
TYPOLOGY = "eunis@2021"


def read_authored(path):
    """Read the authored CSV, ignoring '#' comment lines (the group headers)."""
    with open(path, encoding="utf-8") as fh:
        body = "".join(line for line in fh if not line.startswith("#"))
    return list(csv.DictReader(io.StringIO(body)))


def normalise(s):
    """NBSP-insensitive comparison. The EEA source data carries U+00A0 inside at
    least one name_en (V31), so a byte comparison would report a translation
    error where there is only an invisible character from the upstream data."""
    return " ".join(s.replace("\xa0", " ").split())


def rows_for(entry, version):
    """Expand one authored entry into its localization rows."""
    key = f"{TYPOLOGY}:{entry['code']}"
    source = f"situs@{version}"
    yield {
        "entity_type": "habitat_type",
        "entity_key": key,
        "lang": "de",
        "field": "name",
        "value": entry["name_de"].strip(),
        "source": source,
        "provenance": "situs",
    }
    vernacular = entry.get("vernacular_de", "").strip()
    if vernacular:
        yield {
            "entity_type": "habitat_type",
            "entity_key": key,
            "lang": "de",
            "field": "vernacular",
            "value": vernacular,
            "source": source,
            "provenance": "situs",
        }


def validate(entries, expected_codes=None, index_names=None):
    """Return a list of problems. Empty means the file keeps its promises."""
    problems = []
    seen = set()
    for e in entries:
        code = e["code"]
        if code in seen:
            problems.append(f"{code}: duplicate row")
        seen.add(code)
        if not e["name_de"].strip():
            problems.append(f"{code}: no name_de — a vernacular may never stand alone")
        if index_names is not None and code in index_names:
            if normalise(index_names[code]) != normalise(e["name_en"]):
                problems.append(
                    f"{code}: name_en drifted from the index — the control column "
                    f"is worthless if it does not match what is being translated"
                )
    if expected_codes is not None:
        missing = sorted(set(expected_codes) - seen)
        extra = sorted(seen - set(expected_codes))
        if missing:
            problems.append(f"missing {len(missing)} codes, e.g. {missing[:8]}")
        if extra:
            # A code with an '=' crosswalk has a derived name already; inventing
            # one for it is exactly what the spec forbids.
            problems.append(f"{len(extra)} codes must not be authored: {extra[:8]}")
    return problems


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("authored", help="data/localizations-de-situs.csv")
    p.add_argument("--version", required=True, help="situs version for the source field")
    p.add_argument("--official", default="", help="localizations.csv from extract.py")
    p.add_argument("-o", "--out", default="localizations.csv")
    p.add_argument("--expected-codes", default="",
                   help="file with the codes that must be authored, one per line")
    p.add_argument("--index-names", default="",
                   help="TSV of code<TAB>name_en from the index, to catch drift")
    args = p.parse_args(argv)

    entries = read_authored(args.authored)

    expected = None
    if args.expected_codes:
        with open(args.expected_codes, encoding="utf-8") as fh:
            expected = [line.strip() for line in fh if line.strip()]
    names = None
    if args.index_names:
        names = {}
        with open(args.index_names, encoding="utf-8") as fh:
            for line in fh:
                if "\t" in line:
                    code, name = line.rstrip("\n").split("\t", 1)
                    names[code] = name

    problems = validate(entries, expected, names)
    if problems:
        print("merge: refusing to write, the authored file broke its promises:",
              file=sys.stderr)
        for problem in problems:
            print(f"  - {problem}", file=sys.stderr)
        return 1

    out = []
    if args.official:
        with open(args.official, encoding="utf-8") as fh:
            out.extend(csv.DictReader(fh))
    authored_rows = [r for e in entries for r in rows_for(e, args.version)]
    out.extend(authored_rows)

    with open(args.out, "w", encoding="utf-8", newline="") as fh:
        w = csv.DictWriter(fh, fieldnames=COLUMNS)
        w.writeheader()
        w.writerows(out)

    vernaculars = sum(1 for r in authored_rows if r["field"] == "vernacular")
    print(json.dumps({
        "authored_types": len(entries),
        "authored_rows": len(authored_rows),
        "with_vernacular": vernaculars,
        "total_rows": len(out),
    }, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
