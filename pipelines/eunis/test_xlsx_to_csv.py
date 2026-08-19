import io
import json
import os
import tempfile
import unittest
import zipfile

from xlsx_to_csv import (
    CSV_HEADERS,
    _annex1_name_and_priority,
    _split_multi,
    _syntaxon_rank,
    convert,
    parse_annex1_crosswalks,
    parse_esy_species_roles,
    parse_eunis_classification,
    read_sheet,
    Measurements,
)


def _xml_escape(value):
    return value.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def _cell_xml(ci, value):
    ref = f"{chr(ord('A') + ci)}"
    return f'<c r="{ref}" t="inlineStr"><is><t>{_xml_escape(value)}</t></is></c>'


def make_xlsx(rows):
    """Build a minimal single-sheet .xlsx with inline strings."""
    sheet_rows = "".join(
        "<row>" + "".join(_cell_xml(ci, v) for ci, v in enumerate(r)) + "</row>"
        for r in rows
    )
    sheet = (
        '<?xml version="1.0"?><worksheet '
        'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
        f"<sheetData>{sheet_rows}</sheetData></worksheet>"
    )
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        z.writestr("xl/worksheets/sheet1.xml", sheet)
        z.writestr("xl/sharedStrings.xml", '<?xml version="1.0"?><sst></sst>')
    buf.seek(0)
    return buf


def make_workbook(named_sheets, path):
    """Build a minimal multi-sheet .xlsx on disk (workbook.xml + rels +
    one worksheet per entry) — the real shape the pipeline's parsers need,
    since they resolve sheet names via workbook.xml, not file order."""
    wb_sheets = "".join(
        f'<sheet name="{name}" sheetId="{i+1}" r:id="rId{i+1}"/>'
        for i, (name, _rows) in enumerate(named_sheets)
    )
    workbook = (
        '<?xml version="1.0"?><workbook '
        'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
        'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
        f"<sheets>{wb_sheets}</sheets></workbook>"
    )
    rels = "".join(
        f'<Relationship Id="rId{i+1}" '
        'Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" '
        f'Target="worksheets/sheet{i+1}.xml"/>'
        for i in range(len(named_sheets))
    )
    rels_xml = (
        '<?xml version="1.0"?><Relationships '
        'xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        f"{rels}</Relationships>"
    )
    with zipfile.ZipFile(path, "w") as z:
        z.writestr("xl/workbook.xml", workbook)
        z.writestr("xl/_rels/workbook.xml.rels", rels_xml)
        z.writestr("xl/sharedStrings.xml", '<?xml version="1.0"?><sst></sst>')
        for i, (_name, rows) in enumerate(named_sheets):
            sheet_rows = "".join(
                "<row>" + "".join(_cell_xml(ci, v) for ci, v in enumerate(r)) + "</row>"
                for r in rows
            )
            sheet = (
                '<?xml version="1.0"?><worksheet '
                'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
                f"<sheetData>{sheet_rows}</sheetData></worksheet>"
            )
            z.writestr(f"xl/worksheets/sheet{i+1}.xml", sheet)


class ReadSheetTest(unittest.TestCase):
    def test_reads_rows_as_strings(self):
        src = make_xlsx([["Code", "Name"], ["R22", "Hay meadow"]])
        self.assertEqual(
            read_sheet(src, "xl/worksheets/sheet1.xml"),
            [["Code", "Name"], ["R22", "Hay meadow"]],
        )

    def test_pads_short_rows_so_columns_line_up(self):
        src = make_xlsx([["Code", "Name"], ["R22"]])
        rows = read_sheet(src, "xl/worksheets/sheet1.xml")
        self.assertEqual(rows[1], ["R22", ""])


class SplitMultiTest(unittest.TestCase):
    def test_splits_on_semicolon_ignoring_newlines_and_space(self):
        self.assertEqual(_split_multi("e1.1;\ne1.12"), ["e1.1", "e1.12"])
        self.assertEqual(_split_multi("#; \n#"), ["#", "#"])

    def test_empty_and_whitespace_only_yield_no_tokens(self):
        self.assertEqual(_split_multi(""), [])
        self.assertEqual(_split_multi(" ;\n "), [])


class SyntaxonRankTest(unittest.TestCase):
    def test_ion_suffix_is_alliance(self):
        self.assertEqual(_syntaxon_rank("Arrhenatherion elatioris Luquet 1926"), "alliance")

    def test_etalia_suffix_is_order(self):
        self.assertEqual(_syntaxon_rank("Moltkeetalia petraeae Lakušić 1968"), "order")

    def test_etea_suffix_is_class(self):
        self.assertEqual(_syntaxon_rank("Molinio-Arrhenatheretea Tx. 1937"), "class")


class Annex1NameAndPriorityTest(unittest.TestCase):
    def test_leading_star_marks_priority_and_is_stripped(self):
        name, priority = _annex1_name_and_priority("* Pannonic loess steppic grasslands", "6250", "6250")
        self.assertEqual(name, "Pannonic loess steppic grasslands")
        self.assertEqual(priority, 1)

    def test_no_star_means_no_priority(self):
        name, priority = _annex1_name_and_priority("Dry grasslands", "6210", "6210")
        self.assertEqual(name, "Dry grasslands")
        self.assertEqual(priority, "")


EUNIS_HEADER = [
    "Level", "Code", "Name",
    "EUNIS 2012 relationship", "EUNIS 2012 code", "EUNIS 2012 name (english)",
    "Syntaxa code", "Syntaxa name",
]


class ParseEunisClassificationTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.NamedTemporaryFile(suffix=".xlsx", delete=False)
        self.tmp.close()
        self.addCleanup(os.unlink, self.tmp.name)
        rows = [
            EUNIS_HEADER,
            ["1", "R", "Grasslands", "", "", "", "", ""],
            ["2", "R1", "Dry grasslands", "", "", "", "", ""],
            ["3", "R11", "Pannonian steppe", "=", "E1.1", "Dry grasslands (2012)",
             "ARR-01A;\nMOL-99Z",
             "Arrhenatherion elatioris Luquet 1926;\nMoltkeetalia petraeae Lakušić 1968"],
            ["3", "R12", "Rock outcrops", "≈", "E1.2", "Outcrops (2012)", "", ""],
            ["3", "R13", "Sentinel target", "=", "x", "", "", ""],
            ["3", "R14", "Bundle mismatch", "=;\n<", "E1.4", "", "", ""],
        ]
        make_workbook([("Grassland", rows), ("Read me", [["ignore me"]])], self.tmp.name)

    def test_builds_the_habitat_type_tree_with_parent_codes(self):
        m = Measurements()
        result = parse_eunis_classification(self.tmp.name, m)
        by_code = {r["code"]: r for r in result["habitat_types_2021"]}
        self.assertEqual(by_code["R1"]["parent_code"], "R")
        self.assertEqual(by_code["R11"]["parent_code"], "R1")
        self.assertEqual(by_code["R14"]["parent_code"], "R1")
        self.assertEqual(m.max_habitat_level, 3)

    def test_version_crosswalk_skips_sentinel_and_mismatched_bundle(self):
        m = Measurements()
        result = parse_eunis_classification(self.tmp.name, m)
        codes = {(c["from_code"], c["to_code"], c["qualifier"]) for c in result["crosswalks"]}
        self.assertEqual(codes, {("R11", "E1.1", "="), ("R12", "E1.2", "≈")})
        self.assertIn("≈", m.qualifier_values)  # unknown symbol counted, not dropped

    def test_syntaxa_are_linked_with_measured_rank(self):
        m = Measurements()
        result = parse_eunis_classification(self.tmp.name, m)
        self.assertEqual(result["syntaxa"]["ARR-01A"][0], "alliance")
        self.assertEqual(result["syntaxa"]["MOL-99Z"][0], "order")
        self.assertEqual(m.syntaxa_ranks, {"alliance", "order"})
        links = {(l["code"], l["syntaxon_id"]) for l in result["habitat_type_syntaxa"]}
        self.assertEqual(links, {("R11", "ARR-01A"), ("R11", "MOL-99Z")})


ANNEX1_HEADER = [
    "revised EUNIS Code", "Revised EUNIS name", "relationship EUNIS to Annex I",
    "Annex I code", "Annex I name", "Comment",
]


class ParseAnnex1CrosswalksTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.NamedTemporaryFile(suffix=".xlsx", delete=False)
        self.tmp.close()
        self.addCleanup(os.unlink, self.tmp.name)
        rows = [
            ANNEX1_HEADER,
            ["R11", "Pannonian steppe", "=", "6210", "Semi-natural dry grasslands", ""],
            ["R12", "Rock outcrops", "", "x", "No Annex I type", ""],
            ["R13", "Loess steppe", "#", "6250", "* Pannonic loess steppic grasslands", ""],
            ["R14", "Quaking mire", "#;\n#", "7230;\n7140", "Alkaline fens;\n", ""],
            ["R15", "Two matches", "=;\n#", "6410;\n6420", "Name A;\nName B", ""],
        ]
        make_workbook([("Grasslands", rows)], self.tmp.name)

    def test_sentinel_x_produces_no_crosswalk_row(self):
        m = Measurements()
        result = parse_annex1_crosswalks(self.tmp.name, m)
        self.assertNotIn("R12", {c["from_code"] for c in result["crosswalks"]})

    def test_priority_habitat_is_flagged_and_star_stripped(self):
        m = Measurements()
        result = parse_annex1_crosswalks(self.tmp.name, m)
        self.assertEqual(
            result["habitat_types_annex1"]["6250"],
            ("Pannonic loess steppic grasslands", 1),
        )

    def test_mismatched_bundle_is_skipped_and_counted(self):
        m = Measurements()
        result = parse_annex1_crosswalks(self.tmp.name, m)
        self.assertNotIn("R14", {c["from_code"] for c in result["crosswalks"]})
        self.assertTrue(any("R14" in reason for _, _, reason in m.skipped_rows))

    def test_matched_bundle_yields_both_rows(self):
        m = Measurements()
        result = parse_annex1_crosswalks(self.tmp.name, m)
        r15 = [c for c in result["crosswalks"] if c["from_code"] == "R15"]
        self.assertEqual(
            {(c["to_code"], c["qualifier"]) for c in r15},
            {("6410", "="), ("6420", "#")},
        )

    def test_annex1_qualifier_histogram_counts_each_row(self):
        m = Measurements()
        parse_annex1_crosswalks(self.tmp.name, m)
        self.assertEqual(m.annex1_qualifier_counts.get("="), 2)  # R11 and R15


ESY_HEADER = ["Habitat code", "Habitat name", "Species type", "Species", "Value"]


class ParseEsySpeciesRolesTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.NamedTemporaryFile(suffix=".xlsx", delete=False)
        self.tmp.close()
        self.addCleanup(os.unlink, self.tmp.name)
        rows = [
            ESY_HEADER,
            ["N11", "Sand beach", "Diagnostic", "Cakile maritima ", "49.6"],
            ["N11", "Sand beach", "Constant", "Cakile maritima ", "90"],
            ["N11", "Sand beach", "Dominant", "Cakile maritima ", "20"],
            ["N11", "Sand beach", "Weird", "Unknown thing", "10"],
            ["", "Sand beach", "Diagnostic", "Missing code", "5"],
        ]
        make_workbook([("Data", rows)], self.tmp.name)

    def test_diagnostic_role_carries_fidelity_only(self):
        m = Measurements()
        rows = parse_esy_species_roles(self.tmp.name, m)
        diag = next(r for r in rows if r["role"] == "diagnostic")
        self.assertEqual(diag["fidelity"], "49.6")
        self.assertEqual(diag["constancy"], "")
        self.assertEqual(diag["verbatim_name"], "Cakile maritima")

    def test_constant_and_dominant_carry_constancy_only(self):
        m = Measurements()
        rows = parse_esy_species_roles(self.tmp.name, m)
        by_role = {r["role"]: r for r in rows}
        self.assertEqual(by_role["constant"]["constancy"], "90")
        self.assertEqual(by_role["dominant"]["constancy"], "20")
        self.assertEqual(by_role["constant"]["fidelity"], "")

    def test_unknown_species_type_and_missing_code_are_skipped_and_counted(self):
        m = Measurements()
        rows = parse_esy_species_roles(self.tmp.name, m)
        names = {r["verbatim_name"] for r in rows}
        self.assertNotIn("Unknown thing", names)
        self.assertNotIn("Missing code", names)
        self.assertEqual(len(m.skipped_rows), 2)


class ConvertIntegrationTest(unittest.TestCase):
    """End to end: three tiny in-memory-built workbooks in, six CSVs plus
    report.json out, no fixture on disk and no network."""

    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.addCleanup(lambda: __import__("shutil").rmtree(self.dir, ignore_errors=True))
        eunis_path = os.path.join(self.dir, "eunis.xlsx")
        annex1_path = os.path.join(self.dir, "annex1.xlsx")
        esy_path = os.path.join(self.dir, "esy.xlsx")
        make_workbook([("Grassland", [
            EUNIS_HEADER,
            ["1", "R", "Grasslands", "", "", "", "", ""],
            ["2", "R1", "Dry grasslands", "", "", "", "", ""],
            ["3", "R11", "Pannonian steppe", "=", "E1.1", "Dry grasslands (2012)",
             "ARR-01A", "Arrhenatherion elatioris Luquet 1926"],
        ])], eunis_path)
        make_workbook([("Grasslands", [
            ANNEX1_HEADER,
            ["R11", "Pannonian steppe", "=", "6210", "Semi-natural dry grasslands", ""],
        ])], annex1_path)
        make_workbook([("Data", [
            ESY_HEADER,
            ["R11", "Pannonian steppe", "Diagnostic", "Festuca valesiaca", "60"],
        ])], esy_path)
        self.out_dir = os.path.join(self.dir, "out")
        os.mkdir(self.out_dir)
        self.report = convert(eunis_path, annex1_path, esy_path, self.out_dir)

    def test_writes_every_csv_with_exactly_the_pinned_headers(self):
        for name, header in CSV_HEADERS.items():
            path = os.path.join(self.out_dir, name)
            self.assertTrue(os.path.isfile(path), f"{name} was not written")
            with open(path, encoding="utf-8") as f:
                first_line = f.readline().rstrip("\n")
            self.assertEqual(first_line, ",".join(header))

    def test_report_json_has_exactly_the_pinned_keys(self):
        path = os.path.join(self.out_dir, "report.json")
        with open(path, encoding="utf-8") as f:
            report = json.load(f)
        self.assertEqual(set(report.keys()), {
            "syntaxa_ranks", "max_habitat_level", "qualifier_values",
            "habitat_types", "annex1_crosswalks", "annex1_qualifier_histogram",
            "types_with_annex1", "types_with_annex1_same",
        })
        self.assertEqual(report, self.report)

    def test_measured_facts_are_plausible_for_the_seeded_data(self):
        self.assertEqual(self.report["max_habitat_level"], 3)
        self.assertEqual(self.report["syntaxa_ranks"], ["alliance"])
        self.assertEqual(self.report["types_with_annex1"], 1)
        self.assertEqual(self.report["types_with_annex1_same"], 1)


if __name__ == "__main__":
    unittest.main()
