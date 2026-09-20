#!/usr/bin/env python3
"""EUNIS Habitat Factsheets (PDF) -> habitat_descriptions.csv.

The factsheet PDF carries one habitat per page: a bold title line
"CODE - Name", then the description, then the sections (alliances,
characteristic species). Only the description is extracted here; the
alliances and species already reach the index through pipelines/eunis.

Why pdftohtml and not pdftotext: at 25 factsheets the title wraps, and its
continuation line is set in BODY TEXT SIZE (measured). Geometry alone
therefore assigns it to the description -- "MA223" would start with "and
sedge bed". pdftohtml -xml marks title AND continuation as <b>, which
separates them exactly. Nodes come out unordered, so they are sorted into
reading order first.

Only the Python standard library is used; poppler's pdftohtml is an external
CLI tool, checked by build.sh before this runs.
"""
import csv
import html
import json
import os
import re
import subprocess
import sys

TYPOLOGY = "eunis@2021"

# A title is bold and reads "CODE - Name" (en dash in this document).
CODE_RE = re.compile(r"^([A-Z]{1,2}[0-9A-Za-z]{1,6})\s*[–—-]\s*(.*)$")
NODE_RE = re.compile(r'<text top="(\d+)" left="(\d+)"[^>]*>(.*?)</text>', re.S)
PAGE_SPLIT = "<page "

# Five factsheets have the introduction of the FOLLOWING EUNIS group glued to
# the end of their description, without any separator -- a typesetting fault of
# the source document ("...before tree cover returns.Non-coastal habitats on
# substrates..."). There is no structural signal to key on, so the group
# introductions are listed verbatim and cut where they start. Anything else
# that looks glued (a sentence end directly followed by a capital) is reported
# in report.json as suspicious_joins and left alone: U22 has such a join inside
# its own text, and cutting it would lose real description.
GROUP_INTROS = (
    "Non-coastal habitats on substrates with no or little development of soil",
    "Accumulations of boulders, stones, rock fragments, pebbles, gravels",
    "Unvegetated, sparsely vegetated, and bryophyte or lichen vegetated cliffs",
    "High mountain zones and high latitude land masses occupied by glaciers",
    "Miscellaneous bare habitats, including glacial moraines",
    "Hard rock surfaces, rock jumbles, loose material deposits",
)

# Where the description ends. The factsheet layout puts these after it.
SECTIONS = (
    "Corresponding alliances",
    "Characteristic species",
    "Diagnostic species",
    "Constant species",
    "Dominant species",
    "Distribution",
)


def _plain(node: str) -> str:
    """Strips inline markup and unescapes entities."""
    return html.unescape(re.sub(r"<[^>]+>", "", node)).strip()


def parse_pages(xml_text):
    """Yields (code, name, description) per factsheet page.

    Kept separate from the subprocess call so it can be tested against a
    fixture without poppler and without the 24 MB source document.
    """
    out = []
    for page in xml_text.split(PAGE_SPLIT)[1:]:
        nodes = sorted(
            (int(top), int(left), _plain(body), "<b>" in body)
            for top, left, body in NODE_RE.findall(page)
        )
        start = next((i for i, n in enumerate(nodes) if n[3] and CODE_RE.match(n[2])), None)
        if start is None:
            continue
        match = CODE_RE.match(nodes[start][2])
        code, name = match.group(1), match.group(2).strip()
        body = []
        for _, _, text, bold in nodes[start + 1:]:
            if not text or text.startswith(SECTIONS):
                if text.startswith(SECTIONS):
                    break
                continue
            if bold and CODE_RE.match(text):
                break
            if bold:
                name = f"{name} {text}".strip()
            else:
                body.append(text)
        out.append((code, _squash(name), _squash(" ".join(body))))
    return out


def _squash(text: str) -> str:
    return re.sub(r"\s+", " ", text).strip()


def strip_group_intro(text: str):
    """Cuts a glued-on group introduction; returns (text, cut_marker_or_None)."""
    for intro in GROUP_INTROS:
        index = text.find(intro)
        if index != -1:
            return text[:index].rstrip(), intro
    return text, None


# A sentence end directly followed by a capital letter: two blocks the source
# glued together. Reported, never cut automatically -- see GROUP_INTROS.
# The closing bracket matters: U53 ends "...(U4-3).Hard rock surfaces...", and
# a letters-only pattern walks straight past it.
GLUED_RE = re.compile(r"[a-z)]\.[A-Z]")


def main():
    pdf_path, out_path = sys.argv[1:3]

    xml_text = subprocess.run(
        ["pdftohtml", "-xml", "-i", "-stdout", pdf_path],
        capture_output=True, text=True, check=True,
    ).stdout

    items = parse_pages(xml_text)
    cut, joins = [], []
    cleaned = []
    for code, name, desc in items:
        desc, marker = strip_group_intro(desc)
        if marker:
            cut.append(code)
        if GLUED_RE.search(desc):
            joins.append(code)
        cleaned.append((code, name, desc))
    items = cleaned
    without = [code for code, _, desc in items if not desc]

    with open(out_path, "w", encoding="utf-8", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(["typology_id", "code", "description_en"])
        for code, _, desc in items:
            if desc:
                writer.writerow([TYPOLOGY, code, desc])

    report = {
        "source": os.path.basename(pdf_path),
        "typology_id": TYPOLOGY,
        "factsheets": len(items),
        "with_description": len(items) - len(without),
        "without_description": without,
        "group_intro_cut": cut,
        "suspicious_joins": joins,
    }
    with open(os.path.join(os.path.dirname(out_path) or ".", "report.json"), "w", encoding="utf-8") as f:
        json.dump(report, f, indent=2, ensure_ascii=False)
        f.write("\n")
    print(json.dumps(report, ensure_ascii=False))


if __name__ == "__main__":
    main()
