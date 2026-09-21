#!/usr/bin/env python3
"""Convert the pinned FloraVeg.EU EuroVegChecklist XLSX into
syntaxa_hierarchy.csv (code|rank|name|author|parent_code|alt_code).

Same rationale as pipelines/eunis/xlsx_to_csv.py: an .xlsx is a zip of XML the
stdlib reads, so no spreadsheet library joins situs' dependency list.

rank and parent_code are derived EXCLUSIVELY from the code's own pattern
(class "AA" -> order "AA01" -> alliance "AA01A"), never guessed from the name
— author is a direct copy of FloraVeg's own, already-separated column, never a
heuristic split of a combined string.
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

CSV_HEADERS = {
    "syntaxa_hierarchy.csv": ["code", "rank", "name", "author", "parent_code", "alt_code"],
}

# Maps a row's rank to its counter key in the report dict below. Explicit on
# purpose: string-arithmetic pluralization ("class" -> "classes" vs. plain
# "+s") would silently misname a future rank instead of failing loudly.
_RANK_COUNTER_KEY = {"class": "classes", "order": "orders", "alliance": "alliances"}

_NON_DATA_SHEETS = {"read me", "legend"}

_REQUIRED_HEADERS = ["Code", "Name", "Author"]
# The real FloraVeg export (Step 6, measured against the pinned v4 file, not
# guessed) spells the name/author columns after the two parallel EVC versions
# it carries (original Mucina et al. 2016 vs. an update dated 2025-06-12 in
# the column headers themselves). The updated columns are listed first and
# preferred; the "original" columns are the fallback so an older export
# without the update columns still parses.
_HEADER_ALIASES = {
    "Name": [
        "Name",
        "Syntaxon_name (EVC, version 2025-06-12)",
        "Syntaxon_name (original EVC, Mucina et al. 2016)",
    ],
    "Author": [
        "Author",
        "Authors",
        "Author(s)",
        "Author (EVC, version 2025-06-12)",
        "Author (original EVC, Mucina et al. 2016)",
    ],
}

_CLASS_RE = re.compile(r"^[A-Z]{2}$")
_ORDER_RE = re.compile(r"^[A-Z]{2}[0-9]{2}$")
_ALLIANCE_RE = re.compile(r"^[A-Z]{2}[0-9]{2}[A-Z]$")

# The real FloraVeg export's Code cell carries the primary EVC code plus the
# historical EEA code in parentheses, e.g. "AA01A (PAP-01A)". That
# parenthesized part IS the EEA-EUNIS source's own code scheme and therefore
# the exact join key between both sources — hence it is output, not dropped.
_CODE_CELL_RE = re.compile(r"^(\S+)\s*\(([^)]*)\)\s*$")


def split_code(cell):
    """Split a Code cell into (primary code, alt code). A cell with no
    parenthesized part yields an empty alt code — not an error."""
    cell = cell.strip()
    m = _CODE_CELL_RE.match(cell)
    if m:
        return m.group(1), m.group(2).strip()
    return cell, ""


class HeaderError(RuntimeError):
    """A data sheet is missing a column a parser needs — fail loudly instead
    of silently defaulting every cell to empty."""


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


def col_index(ref):
    """Convert a cell reference's column part ("AB7") into a 0-based column
    index. Excel omits empty cells from the XML; without this conversion, a
    gap would shift every following column."""
    n = 0
    for ch in ref:
        if not ch.isalpha():
            break
        n = n * 26 + (ord(ch.upper()) - 64)
    return n - 1


def read_sheet(src, sheet_path):
    """Return the sheet as a list of equal-length string rows, each cell at
    the column its own r-attribute names."""
    with zipfile.ZipFile(src) as zf:
        shared = _shared_strings(zf)
        root = ET.fromstring(zf.read(sheet_path))
    rows = []
    for row in root.iter(f"{{{NS['m']}}}row"):
        cells = {}
        for c in row.findall("m:c", NS):
            cells[col_index(c.get("r") or "A")] = _cell_text(c, shared)
        width = max(cells) + 1 if cells else 0
        rows.append([cells.get(i, "") for i in range(width)])
    width = max((len(r) for r in rows), default=0)
    for r in rows:
        r.extend([""] * (width - len(r)))
    return rows


def _data_sheets(xlsx_path):
    """List (name, internal zip path) for every non-legend sheet, in workbook
    order — mirrors pipelines/eunis/xlsx_to_csv.py's _data_sheets."""
    with zipfile.ZipFile(xlsx_path) as zf:
        wb = ET.fromstring(zf.read("xl/workbook.xml"))
        rels = ET.fromstring(zf.read("xl/_rels/workbook.xml.rels"))
    relmap = {r.get("Id"): r.get("Target") for r in rels}
    sheets = []
    for sheet in wb.iter(f"{{{NS['m']}}}sheet"):
        name = (sheet.get("name") or "").strip()
        if name.lower() in _NON_DATA_SHEETS:
            continue
        target = relmap.get(sheet.get(_R_ID))
        if target:
            sheets.append((name, "xl/" + target))
    return sheets


def _row_index(header, aliases=None):
    idx = {h.strip(): i for i, h in enumerate(header)}
    for canonical, alternatives in (aliases or {}).items():
        if canonical in idx:
            continue
        for alt in alternatives:
            if alt in idx:
                idx[canonical] = idx[alt]
                break
    return idx


def _require_headers(idx, required, source, sheet):
    missing = [h for h in required if h not in idx]
    if missing:
        raise HeaderError(f"{source} [{sheet}]: missing required column(s) {missing}")


def _cell(row, idx, name, default=""):
    i = idx.get(name)
    return row[i].strip() if i is not None and i < len(row) else default


def rank_and_parent(code):
    """Derive (rank, parent_code) from the code's own pattern alone — never
    from the name. An unrecognized pattern yields ("", ""), so the caller can
    skip and count it instead of guessing."""
    code = code.strip()
    if _CLASS_RE.fullmatch(code):
        return "class", ""
    if _ORDER_RE.fullmatch(code):
        return "order", code[:2]
    if _ALLIANCE_RE.fullmatch(code):
        return "alliance", code[:-1]
    return "", ""


def convert(xlsx_path, out_dir):
    rows_out = []
    counts = {"classes": 0, "orders": 0, "alliances": 0}
    skipped = []
    # Tracks which primary code first claimed an alt_code, so a second,
    # different primary code claiming the same alt_code is a real collision —
    # not just the same row's alt_code seen twice.
    alt_seen = {}
    collisions = set()

    for sheet_name, sheet_path in _data_sheets(xlsx_path):
        rows = read_sheet(xlsx_path, sheet_path)
        if not rows:
            continue
        idx = _row_index(rows[0], aliases=_HEADER_ALIASES)
        _require_headers(idx, _REQUIRED_HEADERS, xlsx_path, sheet_name)
        for row in rows[1:]:
            raw_code = _cell(row, idx, "Code")
            if not raw_code:
                continue
            code, alt = split_code(raw_code)
            rank, parent_code = rank_and_parent(code)
            if not rank:
                skipped.append((sheet_name, code))
                continue
            if alt:
                if alt in alt_seen and alt_seen[alt] != code:
                    collisions.add(alt)
                alt_seen[alt] = code
            counts[_RANK_COUNTER_KEY[rank]] += 1
            rows_out.append({
                "code": code,
                "rank": rank,
                "name": _cell(row, idx, "Name"),
                "author": _cell(row, idx, "Author"),
                "parent_code": parent_code,
                "alt_code": alt,
            })

    for sheet_name, code in skipped:
        print(f"skipped: {xlsx_path} [{sheet_name}]: code {code!r} matches no class/order/alliance pattern",
              file=sys.stderr)

    path = os.path.join(out_dir, "syntaxa_hierarchy.csv")
    with open(path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=CSV_HEADERS["syntaxa_hierarchy.csv"], lineterminator="\n")
        writer.writeheader()
        for row in rows_out:
            writer.writerow(row)

    report = {
        "classes": counts["classes"],
        "orders": counts["orders"],
        "alliances": counts["alliances"],
        "total_rows": len(rows_out),
        "skipped_rows": len(skipped),
        "alt_codes": sum(1 for r in rows_out if r["alt_code"]),
        "alt_code_collisions": sorted(collisions),
    }
    with open(os.path.join(out_dir, "report.json"), "w", encoding="utf-8") as f:
        json.dump(report, f, indent=2, ensure_ascii=False, sort_keys=True)
        f.write("\n")
    return report


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--xlsx", required=True)
    parser.add_argument("--out-dir", required=True)
    args = parser.parse_args(argv)
    try:
        report = convert(args.xlsx, args.out_dir)
    except HeaderError as exc:
        print(f"error: {exc}", file=sys.stderr)
        sys.exit(1)
    print(json.dumps(report, indent=2, ensure_ascii=False, sort_keys=True))


if __name__ == "__main__":
    main()
