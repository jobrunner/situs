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
CODE_RE = re.compile(r"(\d{2}[0-9A-Z]{2})\s*$|^\s*(\d{2}[0-9A-Z]{2})\b")
PAL_LINE = re.compile(r"^\s*PAL\.CLASS\.\s*:?(.*)$", re.M)
PAL_CODE = re.compile(r"\d{2}(?:\.\d+)*[A-Za-z]?")
# Running header/footer of the document, repeated on every page.
NOISE = re.compile(r"^\s*(Interpretation Manual\s*-\s*EUR\s*28.*|Page\s*\d+)\s*$", re.M | re.I)
SECTION = re.compile(r"^\s*([1-5])\)\s", re.M)
# The "Explanatory Notes" page shows a complete specimen entry (2140) with
# annotations in the margin. It parses like a real entry, so it has to be
# recognised by its annotations, or 2140 ends up in the output twice.
EXAMPLE_MARKER = "Natura 2000 code"


def _clean(text: str) -> str:
    return re.sub(r"\s+", " ", text).strip()


def parse_entries(text: str):
    """Yields one dict per habitat type, in document order."""
    text = NOISE.sub("", text.replace("\f", "\n"))
    anchors = list(PAL_LINE.finditer(text))
    out = []
    for i, anchor in enumerate(anchors):
        heading = text[anchors[i - 1].end():anchor.start()] if i else text[:anchor.start()]
        if EXAMPLE_MARKER in heading:
            continue
        code, name = _heading(heading)
        if not code:
            continue
        body = text[anchor.end(): anchors[i + 1].start() if i + 1 < len(anchors) else len(text)]
        sections = _sections(body)
        out.append({
            "code": code,
            "name": name,
            "pal_class": " ".join(PAL_CODE.findall(anchor.group(1))),
            "definition": sections.get("1", ""),
            "species": sections.get("2", ""),
            "categories": sections.get("3", ""),
        })
    return out


def _heading(block: str):
    """Reads the code and the (possibly wrapped) name preceding a PAL.CLASS. line."""
    lines = [l for l in block.strip().splitlines() if l.strip()]
    for index in range(len(lines) - 1, -1, -1):
        match = CODE_RE.search(lines[index].strip())
        if not match:
            continue
        code = match.group(1) or match.group(2)
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

    entries = parse_entries(text)
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
        "duplicate_codes": sorted({e["code"] for e in entries
                                   if [x["code"] for x in entries].count(e["code"]) > 1}),
    }
    with open(os.path.join(os.path.dirname(out_path) or ".", "report.json"), "w", encoding="utf-8") as f:
        json.dump(report, f, indent=2, ensure_ascii=False)
        f.write("\n")
    print(json.dumps({k: v for k, v in report.items() if k != "without_definition"}, ensure_ascii=False))


if __name__ == "__main__":
    main()
