#!/usr/bin/env python3
"""Convert the pinned EUNIS/ESy XLSX artifacts into normalized CSVs.

XLSX parsing lives here, not in the Go binary: situs' dependency list has no
spreadsheet reader, and an .xlsx is just a zip of XML that the stdlib reads.
"""
import argparse
import csv
import json
import os
import sys
import zipfile
import xml.etree.ElementTree as ET

NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}
_R_ID = "{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id"

CSV_HEADERS = {
    "typologies.csv": ["id", "scheme", "version", "name", "source_ref"],
    "habitat_types.csv": [
        "typology_id", "code", "level", "name_en", "parent_code", "priority",
    ],
    "crosswalks.csv": [
        "from_typology", "from_code", "to_typology", "to_code", "qualifier",
    ],
    "syntaxa.csv": ["id", "rank", "name", "parent_id"],
    "habitat_type_syntaxa.csv": ["typology_id", "code", "syntaxon_id"],
    "species_roles.csv": [
        "typology_id", "code", "verbatim_name", "role", "fidelity", "constancy",
    ],
}

# Sheets that hold prose/legend, not data rows.
_NON_DATA_SHEETS = {"read me", "legend"}

# Source rows sometimes use a sentinel instead of leaving the target blank —
# 'x' in the Annex I code column means "no Annex I type", not a real code.
# Design invariant: absence is expressed as absence of rows, never a placeholder.
_NO_TARGET_SENTINELS = {"", "x"}


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


def read_sheet(src, sheet_path):
    """Return the sheet as a list of equal-length string rows."""
    with zipfile.ZipFile(src) as zf:
        shared = _shared_strings(zf)
        root = ET.fromstring(zf.read(sheet_path))
    rows = []
    for row in root.iter(f"{{{NS['m']}}}row"):
        rows.append([_cell_text(c, shared) for c in row.findall("m:c", NS)])
    width = max((len(r) for r in rows), default=0)
    for r in rows:
        r.extend([""] * (width - len(r)))
    return rows


def _data_sheets(xlsx_path):
    """List (name, internal zip path) for every non-legend sheet, in
    workbook order — the sheetN.xml file names do not reliably match the
    tab order, so this reads workbook.xml + its rels rather than guessing.
    """
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


def _split_multi(value):
    """Split a cell that bundles several values with ';' (sometimes with a
    newline instead of, or in addition to, the surrounding space). Measured
    on the real EUNIS artifacts: e.g. 'e1.1;\\ne1.12' or '#; \\n#'.
    """
    return [p.strip() for p in value.replace("\n", "").split(";") if p.strip()]


def _row_index(header):
    return {h.strip(): i for i, h in enumerate(header)}


def _cell(row, idx, name, default=""):
    i = idx.get(name)
    return row[i].strip() if i is not None and i < len(row) else default


class Measurements:
    """Accumulates the report.json facts across all parsers."""

    def __init__(self):
        self.syntaxa_ranks = set()
        self.max_habitat_level = 0
        self.qualifier_values = set()
        self.annex1_qualifier_counts = {}
        self.skipped_rows = []  # list of (source, sheet, reason)

    def skip(self, source, sheet, reason):
        self.skipped_rows.append((source, sheet, reason))

    def note_qualifier(self, q):
        self.qualifier_values.add(q)

    def note_annex1_qualifier(self, q):
        self.annex1_qualifier_counts[q] = self.annex1_qualifier_counts.get(q, 0) + 1


def parse_eunis_classification(xlsx_path, m):
    """Parse the "including crosswalks" workbook: the eunis@2021 habitat
    type tree (all levels), the eunis@2012 version crosswalk, and the
    Euroveg syntaxa linked at level 3.

    Returns a dict of row lists, keyed by output CSV name.
    """
    habitat_types_2021 = []
    habitat_types_2012 = {}
    crosswalks = []
    syntaxa = {}
    habitat_type_syntaxa = []

    for sheet_name, sheet_path in _data_sheets(xlsx_path):
        rows = read_sheet(xlsx_path, sheet_path)
        if not rows:
            continue
        idx = _row_index(rows[0])
        stack = []  # (level, code) ancestors, shallow-to-deep
        for row in rows[1:]:
            code = _cell(row, idx, "Code")
            if not code:
                continue
            level_s = _cell(row, idx, "Level")
            try:
                level = int(level_s)
            except ValueError:
                m.skip(xlsx_path, sheet_name, f"non-numeric level {level_s!r} for {code}")
                continue
            m.max_habitat_level = max(m.max_habitat_level, level)

            while stack and stack[-1][0] >= level:
                stack.pop()
            parent_code = stack[-1][1] if stack else ""
            stack.append((level, code))

            habitat_types_2021.append({
                "typology_id": "eunis@2021",
                "code": code,
                "level": level,
                "name_en": _cell(row, idx, "Name"),
                "parent_code": parent_code,
                "priority": "",
            })

            # Version crosswalk to eunis@2012 — only where the source
            # actually carries a relationship (measured: level 3 only).
            rel_2012 = _cell(row, idx, "EUNIS 2012 relationship")
            code_2012 = _cell(row, idx, "EUNIS 2012 code")
            name_2012_en = _cell(row, idx, "EUNIS 2012 name (english)")
            _link_crosswalk(
                m, xlsx_path, sheet_name, code, "eunis@2012",
                rel_2012, code_2012, name_2012_en, crosswalks,
                target_names=habitat_types_2012,
            )

            # Euroveg syntaxa (alliance, per the source; a lone "-etalia"
            # order-rank name has been measured among them, see report).
            syn_names = _split_multi(_cell(row, idx, "Syntaxa name"))
            syn_ids = _split_multi(_cell(row, idx, "Syntaxa code"))
            if syn_ids and len(syn_ids) != len(syn_names):
                m.skip(xlsx_path, sheet_name, f"syntaxa id/name count mismatch for {code}")
            elif syn_ids:
                for syn_id, syn_name in zip(syn_ids, syn_names):
                    rank = _syntaxon_rank(syn_name)
                    m.syntaxa_ranks.add(rank)
                    syntaxa.setdefault(syn_id, (rank, syn_name))
                    habitat_type_syntaxa.append({
                        "typology_id": "eunis@2021",
                        "code": code,
                        "syntaxon_id": syn_id,
                    })

    return {
        "habitat_types_2021": habitat_types_2021,
        "habitat_types_2012": habitat_types_2012,
        "crosswalks": crosswalks,
        "syntaxa": syntaxa,
        "habitat_type_syntaxa": habitat_type_syntaxa,
    }


def _syntaxon_rank(name):
    """Rank from nomenclature suffix (Mucina et al. convention): -etea is a
    class, -etalia an order, everything else in this source is an alliance
    (-ion). Measured on the real file: 1287 alliance-suffixed names and
    exactly one order ("Moltkeetalia petraeae")."""
    first = name.split()[0].lower().rstrip(".,") if name.split() else ""
    if first.endswith("etea"):
        return "class"
    if first.endswith("etalia"):
        return "order"
    return "alliance"


def _link_crosswalk(m, source, sheet, from_code, to_typology, rel, to_code, to_name,
                     out_rows, target_names=None):
    if to_code.strip().lower() in _NO_TARGET_SENTINELS:
        return  # absence of a correspondence — not a row, never a placeholder
    quals = _split_multi(rel)
    codes = _split_multi(to_code)
    names = _split_multi(to_name) if to_name else [""] * len(codes)
    if not codes:
        return
    if len(quals) != len(codes) or (to_name and len(names) != len(codes)):
        m.skip(source, sheet, f"crosswalk bundle mismatch for {from_code}->{to_typology}")
        return
    for qualifier, code, name in zip(quals, codes, names):
        if code.strip().lower() in _NO_TARGET_SENTINELS:
            continue
        m.note_qualifier(qualifier)
        out_rows.append({
            "from_typology": "eunis@2021",
            "from_code": from_code,
            "to_typology": to_typology,
            "to_code": code,
            "qualifier": qualifier,
        })
        if target_names is not None:
            target_names.setdefault(code, name)


def parse_annex1_crosswalks(xlsx_path, m):
    """Parse the "...with crosswalks to Annex I in separate rows.xlsx"
    workbook: one EUNIS<->Annex I relationship per row (mostly — a couple of
    rows still bundle two, see report)."""
    crosswalks = []
    habitat_types_annex1 = {}
    for sheet_name, sheet_path in _data_sheets(xlsx_path):
        rows = read_sheet(xlsx_path, sheet_path)
        if not rows:
            continue
        idx = _row_index(rows[0])
        for row in rows[1:]:
            eunis_code = _cell(row, idx, "revised EUNIS Code")
            if not eunis_code:
                continue
            rel = _cell(row, idx, "relationship EUNIS to Annex I")
            a1_code = _cell(row, idx, "Annex I code")
            a1_name = _cell(row, idx, "Annex I name")
            before = len(crosswalks)
            _link_crosswalk(
                m, xlsx_path, sheet_name, eunis_code, "annex1",
                rel, a1_code, a1_name, crosswalks,
            )
            for c in crosswalks[before:]:
                m.note_annex1_qualifier(c["qualifier"])
                if c["to_code"] not in habitat_types_annex1:
                    habitat_types_annex1[c["to_code"]] = _annex1_name_and_priority(a1_name, c["to_code"], a1_code)
    return {"crosswalks": crosswalks, "habitat_types_annex1": habitat_types_annex1}


def _annex1_name_and_priority(bundled_name, target_code, bundled_code):
    """Recover the (name, priority) pair for one code out of a possibly
    bundled Annex I name/code cell. A leading '*' in the official Habitats
    Directive listing marks a priority habitat type — it is documentation
    syntax, not part of the name, so it becomes the `priority` flag."""
    codes = _split_multi(bundled_code)
    names = _split_multi(bundled_name)
    name = target_code
    if target_code in codes and len(names) == len(codes):
        name = names[codes.index(target_code)]
    priority = 1 if name.startswith("*") else ""
    return (name.lstrip("*").strip(), priority)


def parse_esy_species_roles(xlsx_path, m):
    """Parse the ESy 'Characteristic-species-combinations.xlsx' Data sheet
    into species_role rows. Value semantics per the workbook's own Legend
    sheet: Diagnostic = phi coefficient*100 (-> fidelity), Constant/Dominant
    = percentage occurrence frequency (-> constancy; Dominant additionally
    requires >25% cover, which this schema has no separate slot for)."""
    role_map = {"diagnostic": "diagnostic", "constant": "constant", "dominant": "dominant"}
    rows_out = []
    for sheet_name, sheet_path in _data_sheets(xlsx_path):
        rows = read_sheet(xlsx_path, sheet_path)
        if not rows or "Habitat code" not in [h.strip() for h in rows[0]]:
            continue  # skip sheets that aren't the Data table (e.g. any left-over legend)
        idx = _row_index(rows[0])
        for row in rows[1:]:
            code = _cell(row, idx, "Habitat code")
            verbatim_name = _cell(row, idx, "Species")
            species_type = _cell(row, idx, "Species type").strip().lower()
            value = _cell(row, idx, "Value")
            if not code or not verbatim_name:
                m.skip(xlsx_path, sheet_name, "missing habitat code or species name")
                continue
            role = role_map.get(species_type)
            if role is None:
                m.skip(xlsx_path, sheet_name, f"unknown species type {species_type!r}")
                continue
            fidelity = value if role == "diagnostic" else ""
            constancy = value if role in ("constant", "dominant") else ""
            rows_out.append({
                "typology_id": "eunis@2021",
                "code": code,
                "verbatim_name": verbatim_name,
                "role": role,
                "fidelity": fidelity,
                "constancy": constancy,
            })
    return rows_out


def _write_csv(out_dir, name, rows):
    path = os.path.join(out_dir, name)
    with open(path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=CSV_HEADERS[name], lineterminator="\n")
        writer.writeheader()
        for row in rows:
            writer.writerow(row)


def build_report(m, counts):
    types_with_annex1 = {c["from_code"] for c in counts["annex1_crosswalks"]}
    types_with_annex1_same = {
        c["from_code"] for c in counts["annex1_crosswalks"] if c["qualifier"] == "="
    }
    return {
        "syntaxa_ranks": sorted(m.syntaxa_ranks),
        "max_habitat_level": m.max_habitat_level,
        "qualifier_values": sorted(m.qualifier_values),
        "habitat_types": counts["habitat_types"],
        "annex1_crosswalks": len(counts["annex1_crosswalks"]),
        "annex1_qualifier_histogram": dict(sorted(m.annex1_qualifier_counts.items())),
        "types_with_annex1": len(types_with_annex1),
        "types_with_annex1_same": len(types_with_annex1_same),
    }


def convert(eunis_xlsx, annex1_xlsx, esy_xlsx, out_dir):
    m = Measurements()

    classification = parse_eunis_classification(eunis_xlsx, m)
    annex1 = parse_annex1_crosswalks(annex1_xlsx, m)
    species_roles = parse_esy_species_roles(esy_xlsx, m)

    # Skipped rows are counted in the log, never dropped silently.
    for source, sheet, reason in m.skipped_rows:
        print(f"skipped: {source} [{sheet}]: {reason}", file=sys.stderr)

    typologies = [
        {"id": "eunis@2021", "scheme": "eunis", "version": "2021",
         "name": "EUNIS 2021", "source_ref": eunis_xlsx},
        {"id": "eunis@2012", "scheme": "eunis", "version": "2012",
         "name": "EUNIS 2012", "source_ref": eunis_xlsx},
        {"id": "annex1", "scheme": "annex1", "version": "92/43/EEC",
         "name": "Habitats Directive Annex I", "source_ref": annex1_xlsx},
    ]

    habitat_types = list(classification["habitat_types_2021"])
    habitat_types += [
        {"typology_id": "eunis@2012", "code": code, "level": "", "name_en": name,
         "parent_code": "", "priority": ""}
        for code, name in classification["habitat_types_2012"].items()
    ]
    habitat_types += [
        {"typology_id": "annex1", "code": code, "level": "", "name_en": name,
         "parent_code": "", "priority": priority}
        for code, (name, priority) in annex1["habitat_types_annex1"].items()
    ]

    crosswalks = classification["crosswalks"] + annex1["crosswalks"]

    syntaxa = [
        {"id": syn_id, "rank": rank, "name": name, "parent_id": ""}
        for syn_id, (rank, name) in classification["syntaxa"].items()
    ]

    _write_csv(out_dir, "typologies.csv", typologies)
    _write_csv(out_dir, "habitat_types.csv", habitat_types)
    _write_csv(out_dir, "crosswalks.csv", crosswalks)
    _write_csv(out_dir, "syntaxa.csv", syntaxa)
    _write_csv(out_dir, "habitat_type_syntaxa.csv", classification["habitat_type_syntaxa"])
    _write_csv(out_dir, "species_roles.csv", species_roles)

    report = build_report(m, {
        "habitat_types": len(habitat_types),
        "annex1_crosswalks": annex1["crosswalks"],
    })
    with open(f"{out_dir}/report.json", "w", encoding="utf-8") as f:
        json.dump(report, f, indent=2, ensure_ascii=False, sort_keys=True)
        f.write("\n")
    return report


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--eunis-xlsx", required=True)
    parser.add_argument("--annex1-xlsx", required=True)
    parser.add_argument("--esy-xlsx", required=True)
    parser.add_argument("--out-dir", required=True)
    args = parser.parse_args(argv)
    report = convert(args.eunis_xlsx, args.annex1_xlsx, args.esy_xlsx, args.out_dir)
    print(json.dumps(report, indent=2, ensure_ascii=False, sort_keys=True))


if __name__ == "__main__":
    main()
