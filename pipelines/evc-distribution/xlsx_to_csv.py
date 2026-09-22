#!/usr/bin/env python3
"""Convert the pinned EVC alliance distribution XLSX into three CSVs.

Source: Zenodo record 11580949, version 2.0 (2024-06-12), CC-BY 4.0 —
"Distribution maps of vegetation alliances in Europe". Cite BOTH
Preislerová et al. (2022) Appl Veg Sci 25: e12642 and Preislerová et al.
(2024) Appl Veg Sci 27: e12766; see manifest.yaml.

Same rationale as pipelines/eunis/xlsx_to_csv.py: an .xlsx is a zip of XML the
stdlib reads, so no spreadsheet library joins situs' dependency list.

TWO things this parser must get right, both measured rather than assumed:

1. Cells are read by their OWN r-attribute, never by position. Excel omits
   empty cells, and with 136 mostly-empty territory columns per row a
   positional read is not merely imprecise: measured against the pinned file
   it yields the value set {"", "1", "U", "0", "2", ..., "86.111"} instead of
   the actual three values, because summary cells slide into territory
   columns.
2. The sheet is picked BY NAME. "Borja tabulka" is the workbook's FIRST sheet
   and is a working draft (country codes instead of territories, plus a Czech
   note about alliances still missing); taking the first non-legend sheet
   would parse it.

The 1 -> verified and U -> uncertain translation happens HERE, not in the Go
ingest: only the side that sees the raw cell can recognize an unknown value
and abort on it, and doing it twice would be two chances to disagree.
"""
import argparse
import csv
import json
import os
import re
import sys
import zipfile
import xml.etree.ElementTree as ET

NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}
_R_ID = "{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id"

SCHEME = "evc_territory"
DATA_SHEET = "European alliances"

CSV_HEADERS = {
    "syntaxon_distribution.csv": ["syntaxon_id", "area_scheme", "area_code", "occurrence"],
    "syntaxon_distribution_coverage.csv": ["syntaxon_id", "area_scheme"],
    "evc_territories.csv": ["area_scheme", "area_code", "name_en"],
}

# The four meta columns, spelled as the pinned file spells them. Code 2 is the
# EEA legacy code — kept in the required set so a format change that drops it
# fails loudly, even though the join runs on Code 1.
_META_HEADERS = ["Code 1", "Code 2", "Name", "Name with author citation"]

# The four summary labels. ONE constant, because the file uses the same four
# strings twice: as the last four column headers and as the first cell of the
# last four rows. Recognizing them in both places from one list is what keeps
# a renamed summary from being read as a territory in one place and a row in
# the other.
_SUMMARY_LABELS = [
    "Verified occurrences",
    "Uncertain occurrences",
    "All occurrences",
    "% of uncertain occurrences",
]

# The cell legend from the "Read me" sheet, verbatim: 1 = verified occurrence,
# U = uncertain occurrence, empty cell = absence. Absence is the ABSENCE of a
# row, never a row with an "absent" value.
_OCCURRENCE = {"1": "verified", "U": "uncertain"}

_ALLIANCE_RE = re.compile(r"^[A-Z]{2}[0-9]{2}[A-Z]$")


class SheetError(RuntimeError):
    """The named data sheet is not in the workbook. Never fall back to
    another one — the workbook's first sheet is a draft."""


class HeaderError(RuntimeError):
    """A meta column a parser needs is missing, or the header row's structure
    is not what this parser recognizes — e.g. a column at or past the
    summary-column boundary that is not one of the four known summary
    labels. Fail loudly instead of silently defaulting every cell to empty or
    silently reading an unknown column as a territory."""


class CellValueError(RuntimeError):
    """A territory cell holds something other than "1", "U" or empty. Either
    the format changed or the cell reference is being read wrong, and both
    must stop the run rather than pass through as a silent mismapping."""


class EmptyResultError(RuntimeError):
    """Not one alliance came out of the data sheet. build.sh clears out/
    before the run and the Go ingest treats the files it finds as a
    REPLACEMENT, so header-only CSVs do not mean "nothing new" — they delete
    the whole syntaxon distribution and coverage and commit an empty one. The
    skip path for a MISSING file stays as it is; only a present source that
    yields nothing is this defect."""


class SlugCollisionError(RuntimeError):
    """Two columns derive the same area_code. Picking a winner would silently
    merge two territories. Two columns spelled EXACTLY alike count too: they
    are not harmless duplicates but two evc_territories.csv rows on one
    area_code, which the SQLite primary key merges without a word."""


def col_index(ref):
    """Turn the column part of a cell reference ("AB7") into a 0-based column
    index. Excel omits empty cells; without this a gap shifts every following
    column."""
    n = 0
    for ch in ref:
        if not ch.isalpha():
            break
        n = n * 26 + (ord(ch.upper()) - 64)
    return n - 1


def slugify(column_name):
    """Column name -> area_code: lowercase, spaces and underscores to
    hyphens, everything else unchanged. "Austria Alps" -> "austria-alps",
    "Albania_coast" -> "albania-coast". An existing hyphen
    ("France Extra-Mediterranean") stays put."""
    return column_name.strip().lower().replace(" ", "-").replace("_", "-")


def _shared_strings(zf):
    try:
        root = ET.fromstring(zf.read("xl/sharedStrings.xml"))
    except KeyError:
        return []
    return ["".join(t.text or "" for t in si.iter(f"{{{NS['m']}}}t"))
            for si in root.findall("m:si", NS)]


def _cell_text(c, shared):
    if c.get("t") == "inlineStr":
        return "".join(t.text or "" for t in c.iter(f"{{{NS['m']}}}t"))
    v = c.find("m:v", NS)
    if v is None or v.text is None:
        return ""
    if c.get("t") == "s":
        return shared[int(v.text)]
    return v.text


def sheet_path(xlsx_path, sheet_name):
    """Internal zip path of the sheet with exactly this name."""
    with zipfile.ZipFile(xlsx_path) as zf:
        wb = ET.fromstring(zf.read("xl/workbook.xml"))
        rels = ET.fromstring(zf.read("xl/_rels/workbook.xml.rels"))
    relmap = {r.get("Id"): r.get("Target") for r in rels}
    names = []
    for sheet in wb.iter(f"{{{NS['m']}}}sheet"):
        name = (sheet.get("name") or "").strip()
        names.append(name)
        if name == sheet_name:
            target = relmap.get(sheet.get(_R_ID))
            if target:
                return "xl/" + target
    raise SheetError(
        f"{xlsx_path}: no sheet named {sheet_name!r}; the workbook has {names!r}")


def read_sheet(src, path):
    """Return the sheet as a list of {column index: cell text} per row. NOT a
    list of positional lists: see the module docstring."""
    with zipfile.ZipFile(src) as zf:
        shared = _shared_strings(zf)
        root = ET.fromstring(zf.read(path))
    rows = []
    for row in root.iter(f"{{{NS['m']}}}row"):
        cells = {}
        for c in row.findall("m:c", NS):
            text = _cell_text(c, shared)
            if text != "":
                cells[col_index(c.get("r") or "A")] = text
        rows.append(cells)
    return rows


def _meta_index(head, xlsx_path):
    """Column index of each meta header, or HeaderError."""
    by_name = {name.strip(): ci for ci, name in head.items()}
    missing = [h for h in _META_HEADERS if h not in by_name]
    if missing:
        raise HeaderError(
            f"{xlsx_path} [{DATA_SHEET}]: missing required column(s) {missing}")
    return {h: by_name[h] for h in _META_HEADERS}


def _territories(head, meta, xlsx_path):
    """The territory columns: every header column left of the summary
    columns, in column order. The boundary is STRUCTURAL, not a name count:
    the first column whose header matches _SUMMARY_LABELS marks where the
    territory zone ends, and every column at or past that point must also be
    a recognized summary label. A fifth (or renamed) summary column, or a
    summary column that precedes a territory column, is not silently read as
    a 137th territory or silently ignored — it is a sheet structure this
    parser does not recognize, so it aborts naming the column and its
    position rather than guessing."""
    meta_cols = set(meta.values())
    out = []
    seen = {}
    summary_boundary = None
    for ci in sorted(head):
        name = head[ci].strip()
        if ci in meta_cols or name == "":
            continue
        if name in _SUMMARY_LABELS:
            if summary_boundary is None:
                summary_boundary = ci
            continue
        if summary_boundary is not None:
            raise HeaderError(
                f"{xlsx_path} [{DATA_SHEET}]: column {name!r} at position "
                f"{ci} lies at or after the summary columns (starting at "
                f"position {summary_boundary}) but is not one of "
                f"{_SUMMARY_LABELS}; unrecognized sheet structure")
        code = slugify(name)
        if code in seen:
            first_name, first_ci = seen[code]
            raise SlugCollisionError(
                f"{xlsx_path} [{DATA_SHEET}]: columns {first_name!r} (position "
                f"{first_ci}) and {name!r} (position {ci}) both derive "
                f"area_code {code!r}")
        seen[code] = (name, ci)
        out.append((ci, code, name))
    return out


def _occurrence(raw, code, name, xlsx_path):
    try:
        return _OCCURRENCE[raw]
    except KeyError:
        raise CellValueError(
            f"{xlsx_path} [{DATA_SHEET}]: alliance {code!r}, territory "
            f"{name!r}: cell value {raw!r} is neither '1', 'U' nor empty"
        ) from None


def convert(xlsx_path, out_dir):
    rows = read_sheet(xlsx_path, sheet_path(xlsx_path, DATA_SHEET))
    if not rows:
        raise SheetError(f"{xlsx_path} [{DATA_SHEET}]: the sheet is empty")

    meta = _meta_index(rows[0], xlsx_path)
    territories = _territories(rows[0], meta, xlsx_path)
    code_col = meta["Code 1"]

    dist, coverage = [], []
    histogram = {}
    counts = {"verified": 0, "uncertain": 0}
    summary_rows, skipped = 0, []

    for row in rows[1:]:
        code = row.get(code_col, "").strip()
        if code == "":
            continue
        if code in _SUMMARY_LABELS:
            summary_rows += 1
            continue
        if not _ALLIANCE_RE.fullmatch(code):
            skipped.append(code)
            continue
        coverage.append({"syntaxon_id": code, "area_scheme": SCHEME})
        for ci, area_code, name in territories:
            raw = row.get(ci, "")
            histogram[raw] = histogram.get(raw, 0) + 1
            if raw == "":
                continue
            occurrence = _occurrence(raw, code, name, xlsx_path)
            counts[occurrence] += 1
            dist.append({"syntaxon_id": code, "area_scheme": SCHEME,
                         "area_code": area_code, "occurrence": occurrence})

    for code in skipped:
        print(f"skipped: {xlsx_path} [{DATA_SHEET}]: code {code!r} matches no "
              "alliance pattern", file=sys.stderr)

    # Before the first _write, so a failed conversion leaves out/ empty: the
    # ingest goes by file presence, and a written-then-abandoned CSV would be
    # collected as this run's result.
    if not coverage:
        raise EmptyResultError(
            f"{xlsx_path} [{DATA_SHEET}]: not one row carries an alliance code "
            f"({len(skipped)} skipped, {summary_rows} summary rows); writing "
            "the empty result would replace the whole syntaxon distribution "
            "with nothing")

    _write(out_dir, "syntaxon_distribution.csv", dist)
    _write(out_dir, "syntaxon_distribution_coverage.csv", coverage)
    _write(out_dir, "evc_territories.csv",
           [{"area_scheme": SCHEME, "area_code": c, "name_en": n}
            for _, c, n in territories])

    report = {
        "alliances": len(coverage),
        "covered": len(coverage),
        "territories": len(territories),
        "verified": counts["verified"],
        "uncertain": counts["uncertain"],
        "written": len(dist),
        "slug_collisions": [],
        "summary_rows": summary_rows,
        "skipped_rows": len(skipped),
        "value_histogram": histogram,
    }
    with open(os.path.join(out_dir, "report.json"), "w", encoding="utf-8") as f:
        json.dump(report, f, indent=2, ensure_ascii=False, sort_keys=True)
        f.write("\n")
    return report


def _write(out_dir, name, rows):
    with open(os.path.join(out_dir, name), "w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=CSV_HEADERS[name], lineterminator="\n")
        w.writeheader()
        for r in rows:
            w.writerow(r)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--xlsx", required=True)
    parser.add_argument("--out-dir", required=True)
    args = parser.parse_args(argv)
    try:
        report = convert(args.xlsx, args.out_dir)
    except (SheetError, HeaderError, CellValueError, SlugCollisionError,
            EmptyResultError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        sys.exit(1)
    print(json.dumps(report, indent=2, ensure_ascii=False, sort_keys=True))


if __name__ == "__main__":
    main()
