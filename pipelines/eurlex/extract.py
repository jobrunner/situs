#!/usr/bin/env python3
"""Extract the official German Annex I habitat-type names from the Habitats
Directive and emit the localizations.csv the Go ingest reads.

Why this lives outside the Go binary: the same reason pipelines/eunis/ does. The
source is a legal text in XHTML, and parsing it in Go would mean a new
dependency. The ingest reads only CSV.

Pinned source: CELEX 01992L0043-20130701 (consolidated German version, after the
accession of Croatia). Reusable under Decision 2011/833/EU with attribution, so
every emitted row carries source=eur-lex:31992L0043.

Measured structure of the document (not assumed):

    <p class="dlist-term">9370</p>            <- the code
    <p class="dlist-definition">* Palmhaine von <span class="italics">Phönix</span></p>

A LEADING asterisk marks a priority habitat type and is not part of the name —
habitat_type.priority already carries that fact. An asterisk elsewhere in the
text is part of the name ("Machair (* in Irland)") and is kept.
"""
import argparse
import csv
import json
import re
import sys

CELEX = "01992L0043-20130701"
SOURCE = "eur-lex:31992L0043"
TYPOLOGY = "annex1"

# Codes are four characters, but NOT four digits: 44 of the 233 entries are
# alphanumeric (21A0 Machair, 40A0 peri-Pannonic scrub, 62A0 …). A \d{4} filter
# would drop them silently, which is the whole class of bug this pipeline exists
# to avoid.
CODE = re.compile(r"[0-9A-Z]{4}\Z")

PAIR = re.compile(
    r'<p class="dlist-term">(.*?)</p>.*?<p class="dlist-definition">(.*?)</p>',
    re.S,
)


def _text(fragment):
    """Strip inline markup and normalise whitespace, including NBSP."""
    without_tags = re.sub(r"<[^>]+>", "", fragment)
    return re.sub(r"\s+", " ", without_tags.replace("\xa0", " ")).strip()


def annex1_segment(html):
    """Narrow the document to Annex I.

    The dlist markup is reused by later annexes (species lists), so extracting
    from the whole document would mix habitat codes with species entries.
    """
    start = html.find("ANHANG I")
    end = html.find("ANHANG II")
    if start < 0 or end < 0 or end <= start:
        raise ValueError(
            "cannot locate the Annex I segment (ANHANG I .. ANHANG II) — "
            "document structure changed, refusing to extract from the whole text"
        )
    return html[start:end]


def extract_annex1(html):
    """Return {code: official German name} for every Annex I habitat type."""
    out = {}
    for term, definition in PAIR.findall(annex1_segment(html)):
        code = _text(term)
        if not CODE.match(code):
            continue
        name = _text(definition)
        # Only a leading asterisk is the priority marker.
        if name.startswith("*"):
            name = name[1:].strip()
        out[code] = name
    return out


def build_report(names, index_codes):
    """Compare what the directive offers against what the index carries.

    Both directions are reported. A code the index does not carry is a property
    of the index's source data; an indexed type without an official name would be
    a gap in this pipeline — and must not be discovered by someone noticing an
    English label in the app.
    """
    missing_in_index = sorted(set(names) - set(index_codes))
    without_official_name = sorted(set(index_codes) - set(names))
    return {
        "celex": CELEX,
        "source": SOURCE,
        "eurlex_codes": len(names),
        "index_codes": len(index_codes),
        "missing_in_index": missing_in_index,
        "missing_in_index_count": len(missing_in_index),
        "without_official_name": without_official_name,
        "without_official_name_count": len(without_official_name),
    }


def rows(names):
    for code in sorted(names):
        yield {
            "entity_type": "habitat_type",
            "entity_key": f"{TYPOLOGY}:{code}",
            "lang": "de",
            "field": "name",
            "value": names[code],
            "source": SOURCE,
            "provenance": "official",
        }


COLUMNS = ["entity_type", "entity_key", "lang", "field", "value", "source", "provenance"]


def write_csv(path, names):
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=COLUMNS)
        writer.writeheader()
        writer.writerows(rows(names))


def index_codes_from(path):
    """Read the Annex I codes the index carries, one per line. Empty when no
    path is given: the report then says nothing about coverage rather than
    claiming full coverage."""
    if not path:
        return []
    with open(path, encoding="utf-8") as fh:
        return [line.strip() for line in fh if line.strip()]


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("html", help="the XHTML fetched by fetch.sh")
    p.add_argument("-o", "--out", default="localizations.csv")
    p.add_argument("-r", "--report", default="report.json")
    p.add_argument(
        "--index-codes",
        default="",
        help="file with the annex1 codes the index carries, one per line",
    )
    args = p.parse_args(argv)

    with open(args.html, encoding="utf-8") as fh:
        names = extract_annex1(fh.read())
    if not names:
        print("extract: no Annex I entries found — refusing to write an empty CSV",
              file=sys.stderr)
        return 2

    write_csv(args.out, names)
    report = build_report(names, index_codes_from(args.index_codes))
    with open(args.report, "w", encoding="utf-8") as fh:
        json.dump(report, fh, ensure_ascii=False, indent=2, sort_keys=True)
        fh.write("\n")

    print(f"extract: {len(names)} official German Annex I names -> {args.out}")
    if report["without_official_name_count"]:
        print(f"extract: WARNING {report['without_official_name_count']} indexed "
              f"types have no official name: {report['without_official_name'][:10]}",
              file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
