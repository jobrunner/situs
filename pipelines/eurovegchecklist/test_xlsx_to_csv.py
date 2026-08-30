# pipelines/eurovegchecklist/test_xlsx_to_csv.py
import io
import json
import os
import tempfile
import unittest
import zipfile

from xlsx_to_csv import CSV_HEADERS, HeaderError, convert, primary_code, rank_and_parent


def _xml_escape(value):
    return value.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def _cell_xml(ci, value):
    ref = f"{chr(ord('A') + ci)}"
    return f'<c r="{ref}" t="inlineStr"><is><t>{_xml_escape(value)}</t></is></c>'


def make_workbook(rows, path, sheet_name="Vegetation units"):
    """Build a minimal single-sheet .xlsx on disk, same shape as
    pipelines/eunis/test_xlsx_to_csv.py's make_workbook."""
    workbook = (
        '<?xml version="1.0"?><workbook '
        'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
        'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
        f'<sheets><sheet name="{sheet_name}" sheetId="1" r:id="rId1"/></sheets></workbook>'
    )
    rels = (
        '<?xml version="1.0"?><Relationships '
        'xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" '
        'Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" '
        'Target="worksheets/sheet1.xml"/></Relationships>'
    )
    sheet_rows = "".join(
        "<row>" + "".join(_cell_xml(ci, v) for ci, v in enumerate(r)) + "</row>"
        for r in rows
    )
    sheet = (
        '<?xml version="1.0"?><worksheet '
        'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
        f"<sheetData>{sheet_rows}</sheetData></worksheet>"
    )
    with zipfile.ZipFile(path, "w") as z:
        z.writestr("xl/workbook.xml", workbook)
        z.writestr("xl/_rels/workbook.xml.rels", rels)
        z.writestr("xl/sharedStrings.xml", '<?xml version="1.0"?><sst></sst>')
        z.writestr("xl/worksheets/sheet1.xml", sheet)


class RankAndParentTest(unittest.TestCase):
    def test_class_code_has_no_parent(self):
        self.assertEqual(rank_and_parent("AA"), ("class", ""))

    def test_order_code_parent_is_the_class(self):
        self.assertEqual(rank_and_parent("AA01"), ("order", "AA"))

    def test_alliance_code_parent_is_the_order(self):
        self.assertEqual(rank_and_parent("AA01A"), ("alliance", "AA01"))

    def test_unrecognized_code_pattern_yields_empty_rank(self):
        self.assertEqual(rank_and_parent("not-a-code"), ("", ""))


class PrimaryCodeTest(unittest.TestCase):
    def test_strips_a_parenthesized_legacy_code(self):
        self.assertEqual(primary_code("AA01A (KOB-01A)"), "AA01A")

    def test_strips_a_legacy_code_with_no_space_before_the_paren(self):
        self.assertEqual(primary_code("AA01A(KOB-01A)"), "AA01A")

    def test_a_bare_code_with_no_parens_is_unchanged(self):
        self.assertEqual(primary_code("AA"), "AA")


class ConvertTest(unittest.TestCase):
    def test_writes_class_order_alliance_rows_with_derived_parents(self):
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "floraveg.xlsx")
            make_workbook(
                [
                    ["Code", "Name", "Author"],
                    ["AA", "Salicetea purpureae", "Moor 1958"],
                    ["AA01", "Salicetalia purpureae", "Moor 1958"],
                    ["AA01A", "Salicion albae", "Soó 1930"],
                ],
                xlsx,
            )
            out_dir = os.path.join(tmp, "out")
            os.makedirs(out_dir)
            report = convert(xlsx, out_dir)

            with open(os.path.join(out_dir, "syntaxa_hierarchy.csv"), encoding="utf-8") as f:
                content = f.read()
            self.assertIn("AA,class,Salicetea purpureae,Moor 1958,\n", content)
            self.assertIn("AA01,order,Salicetalia purpureae,Moor 1958,AA\n", content)
            self.assertIn("AA01A,alliance,Salicion albae,Soó 1930,AA01\n", content)
            self.assertEqual(report["classes"], 1)
            self.assertEqual(report["orders"], 1)
            self.assertEqual(report["alliances"], 1)
            self.assertEqual(report["skipped_rows"], 0)

    def test_skips_and_counts_a_code_matching_no_pattern(self):
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "floraveg.xlsx")
            make_workbook(
                [
                    ["Code", "Name", "Author"],
                    ["AA", "Salicetea purpureae", "Moor 1958"],
                    ["not-a-code", "Some legend row", ""],
                ],
                xlsx,
            )
            out_dir = os.path.join(tmp, "out")
            os.makedirs(out_dir)
            report = convert(xlsx, out_dir)
            self.assertEqual(report["classes"], 1)
            self.assertEqual(report["skipped_rows"], 1)

    def test_prefers_the_2025_06_12_updated_name_and_author_over_the_2016_ones(self):
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "floraveg.xlsx")
            make_workbook(
                [
                    [
                        "Code",
                        "Syntaxon_name (original EVC, Mucina et al. 2016)",
                        "Author (original EVC, Mucina et al. 2016)",
                        "Syntaxon_name (EVC, version 2025-06-12)",
                        "Author (EVC, version 2025-06-12)",
                    ],
                    [
                        "AA01A (KOB-01A)",
                        "Salicion albae (2016 name)",
                        "Soó 1930 (2016 author)",
                        "Salicion albae (2025 name)",
                        "Soó 1930 emend. 2025 (2025 author)",
                    ],
                ],
                xlsx,
            )
            out_dir = os.path.join(tmp, "out")
            os.makedirs(out_dir)
            convert(xlsx, out_dir)

            with open(os.path.join(out_dir, "syntaxa_hierarchy.csv"), encoding="utf-8") as f:
                content = f.read()
            self.assertIn(
                "AA01A,alliance,Salicion albae (2025 name),Soó 1930 emend. 2025 (2025 author),AA01\n",
                content,
            )
            self.assertNotIn("2016 name", content)
            self.assertNotIn("2016 author", content)

    def test_missing_required_header_raises_header_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "floraveg.xlsx")
            make_workbook([["Code", "Name"]], xlsx)  # no "Author" column
            out_dir = os.path.join(tmp, "out")
            os.makedirs(out_dir)
            with self.assertRaises(HeaderError):
                convert(xlsx, out_dir)


if __name__ == "__main__":
    unittest.main()
