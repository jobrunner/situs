#!/usr/bin/env python3
"""Interpretation Manual EUR 28 (PDF) -> annex1_source.csv.

The manual is the official interpretation of the Annex I habitat types. Each
entry has the same shape:

    CODE     Name (may wrap over several lines)
    PAL.CLASS.: 35.1, 36.31

    1)  definition
    2)  Plants: ... Animals: ...
    3)  Corresponding categories / sub-types
    ...

Anchoring on the PAL.CLASS. line rather than on the heading is what makes the
parse reliable: the heading wraps, is indented differently per entry and its
code column is not separable by geometry, while PAL.CLASS. appears exactly
once per habitat type.

The output is WORKING MATERIAL, not something situs serves. The descriptions
situs publishes are written from it (plus the EUNIS crosswalk and the index's
own species data) and carry provenance `situs`; this CSV is what makes that
derivation checkable.

Standard library only; poppler's pdftotext is an external CLI tool, checked by
build.sh before this runs.
"""
import csv
import json
import os
import re
import subprocess
import sys

TYPOLOGY = "annex1"

# "1130", "91E0", "62C0", "91AA" — two digits plus two digits-or-letters.
# Anchored at the START of the line on purpose: in this document the code
# always opens its line, while a references line ending in a year ("Nordisk
# vaextgeografi. 1967") would otherwise be read as a heading.
CODE_RE = re.compile(r"^(\d{2}[0-9A-Z]{2})\b")
PAL_LINE = re.compile(r"^\s*PAL\.CLASS\.\s*:?(.*)$", re.M)
PAL_CODE = re.compile(r"\d{2}(?:\.\d+)*[A-Za-z]?")
# Running header/footer of the document, repeated on every page.
NOISE = re.compile(r"^\s*(Interpretation Manual\s*-\s*EUR\s*28.*|Page\s*\d+)\s*$", re.M | re.I)
SECTION = re.compile(r"^\s*([1-5])\)\s", re.M)
# The "Explanatory Notes" page of the front matter shows a complete specimen
# entry (2140, annotated in the margin). It parses exactly like a real entry,
# so the code alone cannot tell them apart. What does tell them apart is
# position: the specimen sits in the front matter, the real entry in the
# catalogue, and the catalogue comes second. A repeated code therefore keeps
# its LAST occurrence, and the dropped ones are counted in the report.


def _clean(text: str) -> str:
    return re.sub(r"\s+", " ", text).strip()


def parse_entries(text: str, count_skipped: bool = False, deduplicate: bool = True):
    """Yields one dict per habitat type, in document order.

    The block between two PAL.CLASS. anchors is NOT one entry: it is the
    previous entry's body followed by the next entry's heading. Both halves
    are separated here, or every entry ends with the following code and name
    glued to its last section (measured: 95 of 233 before this was fixed).
    """
    text = NOISE.sub("", text.replace("\f", "\n"))
    anchors = list(PAL_LINE.finditer(text))
    out, skipped = [], 0
    pending = None  # heading read ahead of the anchor it belongs to
    for i, anchor in enumerate(anchors):
        block = text[anchors[i - 1].end():anchor.start()] if i else text[:anchor.start()]
        body_lines, heading_lines = _split_heading(block)
        if pending is not None:
            pending["body"] = "\n".join(body_lines)
            out.append(pending)
            pending = None
        if not heading_lines:
            skipped += 1
            continue
        code, name = _heading("\n".join(heading_lines))
        if not code:
            skipped += 1
            continue
        pending = {"code": code, "name": name,
                   "pal_class": " ".join(PAL_CODE.findall(anchor.group(1))), "body": ""}
    if pending is not None:
        pending["body"] = text[anchors[-1].end():] if anchors else ""
        out.append(pending)

    entries = []
    for e in out:
        sections = _sections(e.pop("body"))
        entries.append({**e,
                        "definition": sections.get("1", ""),
                        "species": sections.get("2", ""),
                        "categories": sections.get("3", "")})
    if not deduplicate:
        return (entries, skipped) if count_skipped else entries
    entries, dropped = _keep_last_per_code(entries)
    return (entries, skipped + dropped) if count_skipped else entries


def duplicate_codes(entries):
    """Codes appearing more than once, counted BEFORE deduplication."""
    seen, repeated = set(), set()
    for entry in entries:
        if entry["code"] in seen:
            repeated.add(entry["code"])
        seen.add(entry["code"])
    return sorted(repeated)


def _keep_last_per_code(entries):
    """Keeps the last entry per code; returns (kept, number dropped)."""
    last = {}
    for index, entry in enumerate(entries):
        last[entry["code"]] = index
    kept = [e for index, e in enumerate(entries) if last[e["code"]] == index]
    return kept, len(entries) - len(kept)


# A heading is at most this many lines: the code line plus a wrapped name.
# Bounding the backward scan is what stops a code being invented from the
# references of the previous entry (a line ending in "1967" is not a code).
MAX_HEADING_LINES = 4


def _split_heading(block: str):
    """Splits a block into (body of the previous entry, heading of the next)."""
    lines = block.splitlines()
    tail = [(index, line) for index, line in enumerate(lines) if line.strip()][-MAX_HEADING_LINES:]
    for index, line in tail:
        if CODE_RE.search(line.strip()):
            return lines[:index], lines[index:]
    return lines, []


def _heading(block: str):
    """Reads the code and the (possibly wrapped) name preceding a PAL.CLASS. line."""
    lines = [l for l in block.strip().splitlines() if l.strip()]
    for index in range(len(lines) - 1, -1, -1):
        match = CODE_RE.search(lines[index].strip())
        if not match:
            continue
        code = match.group(1)
        rest = lines[index].strip()[match.end():].strip()
        name = " ".join([rest] + [l.strip() for l in lines[index + 1:]])
        return code, _clean(name)
    return None, ""


def _sections(body: str):
    """Splits a body into its numbered sections."""
    marks = list(SECTION.finditer(body))
    out = {}
    for i, mark in enumerate(marks):
        end = marks[i + 1].start() if i + 1 < len(marks) else len(body)
        out.setdefault(mark.group(1), _clean(body[mark.end():end]))
    return out


def main():
    pdf_path, out_path = sys.argv[1:3]

    text = subprocess.run(
        ["pdftotext", "-layout", pdf_path, "-"],
        capture_output=True, text=True, check=True,
    ).stdout

    raw_duplicates = duplicate_codes(parse_entries(text, deduplicate=False))
    entries, skipped = parse_entries(text, count_skipped=True)
    with open(out_path, "w", encoding="utf-8", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(["typology_id", "code", "name_en", "pal_class", "definition_en", "species_en", "categories_en"])
        for e in entries:
            writer.writerow([TYPOLOGY, e["code"], e["name"], e["pal_class"],
                             e["definition"], e["species"], e["categories"]])

    report = {
        "source": os.path.basename(pdf_path),
        "typology_id": TYPOLOGY,
        "entries": len(entries),
        "with_definition": sum(1 for e in entries if e["definition"]),
        "with_species": sum(1 for e in entries if e["species"]),
        "with_categories": sum(1 for e in entries if e["categories"]),
        "without_definition": [e["code"] for e in entries if not e["definition"]],
        # Reported from the RAW parse: after deduplication this list could
        # only ever be empty, and a source that starts repeating codes would
        # look unchanged.
        "duplicate_codes": raw_duplicates,
        # Anchors whose heading could not be read, plus earlier occurrences of
        # a repeated code (the front matter's specimen entry). A re-pin that
        # changes the layout shows up here instead of silently yielding fewer
        # types.
        "skipped_anchors": skipped,
    }
    with open(os.path.join(os.path.dirname(out_path) or ".", "report.json"), "w", encoding="utf-8") as f:
        json.dump(report, f, indent=2, ensure_ascii=False)
        f.write("\n")
    print(json.dumps({k: v for k, v in report.items() if k != "without_definition"}, ensure_ascii=False))


if __name__ == "__main__":
    main()
