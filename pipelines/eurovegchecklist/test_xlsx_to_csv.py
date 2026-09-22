# pipelines/eurovegchecklist/test_xlsx_to_csv.py
import os
import tempfile
import unittest
import zipfile

import xlsx_to_csv
from xlsx_to_csv import EEACodeCollisionError, HeaderError, col_index, convert, rank_and_parent, split_code


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


class SplitCodeTest(unittest.TestCase):
    def test_split_code_trennt_primaercode_und_eea_code(self):
        self.assertEqual(split_code("AA01A (PAP-01A)"), ("AA01A", "PAP-01A"))
        self.assertEqual(split_code("  AA01 (PAP-01)  "), ("AA01", "PAP-01"))

    def test_strips_a_legacy_code_with_no_space_before_the_paren(self):
        self.assertEqual(split_code("AA01A(KOB-01A)"), ("AA01A", "KOB-01A"))

    def test_split_code_ohne_klammer_liefert_leeren_eea_code(self):
        self.assertEqual(split_code("AA01A"), ("AA01A", ""))


class ColIndexTest(unittest.TestCase):
    def test_col_index_rechnet_zellbezug_in_spalte(self):
        self.assertEqual(col_index("A1"), 0)
        self.assertEqual(col_index("Z9"), 25)
        self.assertEqual(col_index("AA1"), 26)
        self.assertEqual(col_index("EF12"), 135)


class ReadSheetTest(unittest.TestCase):
    def setUp(self):
        self._tmpdir = tempfile.TemporaryDirectory()
        self.tmp = self._tmpdir.name
        self.addCleanup(self._tmpdir.cleanup)

    def _write_xlsx_with_sheet(self, sheet_xml):
        """Write a minimal XLSX with exactly this sheet1.xml."""
        path = os.path.join(self.tmp, "sheet.xlsx")
        with zipfile.ZipFile(path, "w") as zf:
            zf.writestr(
                "xl/workbook.xml",
                '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"'
                ' xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
                '<sheets><sheet name="Data" sheetId="1" r:id="rId1"/></sheets></workbook>',
            )
            zf.writestr(
                "xl/_rels/workbook.xml.rels",
                '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
                '<Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>',
            )
            zf.writestr("xl/worksheets/sheet1.xml", sheet_xml)
        return path

    def test_read_sheet_haelt_luecken_offen(self):
        # A row where Excel omits the second cell: B is missing, C is
        # present. Positional reading would shift "x" into B's place.
        sheet = (
            '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
            '<sheetData>'
            '<row r="1"><c r="A1" t="inlineStr"><is><t>A</t></is></c>'
            '<c r="C1" t="inlineStr"><is><t>x</t></is></c></row>'
            '</sheetData></worksheet>'
        )
        path = self._write_xlsx_with_sheet(sheet)
        rows = xlsx_to_csv.read_sheet(path, "xl/worksheets/sheet1.xml")
        self.assertEqual(rows[0], ["A", "", "x"])


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
            self.assertIn("AA,class,Salicetea purpureae,Moor 1958,,\n", content)
            self.assertIn("AA01,order,Salicetalia purpureae,Moor 1958,AA,\n", content)
            self.assertIn("AA01A,alliance,Salicion albae,Soó 1930,AA01,\n", content)
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
                "AA01A,alliance,Salicion albae (2025 name),Soó 1930 emend. 2025 (2025 author),AA01,KOB-01A\n",
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

    def _convert_rows(self, rows):
        """Build an XLSX from row dicts (keys Code/Name/Author) and return
        the report dict from convert()."""
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "floraveg.xlsx")
            header = ["Code", "Name", "Author"]
            make_workbook(
                [header] + [[row.get(h, "") for h in header] for row in rows],
                xlsx,
            )
            out_dir = os.path.join(tmp, "out")
            os.makedirs(out_dir)
            return convert(xlsx, out_dir)

    def test_report_zaehlt_eea_codes(self):
        rows = [
            {"Code": "AA (PAP)", "Name": "K", "Author": ""},
            {"Code": "AA01 (PAP-01)", "Name": "O", "Author": ""},
        ]
        report = self._convert_rows(rows)
        self.assertEqual(report["eea_codes"], 2)
        self.assertNotIn("eea_code_collisions", report)

    def test_kollidierender_eea_code_bricht_die_konvertierung_ab(self):
        # AA01 and AB01 are two different primary codes both claiming the
        # eea_code PAP-01 — the join key IngestSyntaxa's byEEA map uses.
        # Silently picking one (as the old "last one wins" report used to)
        # would resolve an EEA edge or a stale-eea-code relink onto an
        # arbitrary one of the two; this must fail instead.
        rows = [
            {"Code": "AA01 (PAP-01)", "Name": "O", "Author": ""},
            {"Code": "AB01 (PAP-01)", "Name": "O2", "Author": ""},
        ]
        with self.assertRaises(EEACodeCollisionError) as ctx:
            self._convert_rows(rows)
        self.assertIn("PAP-01", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
