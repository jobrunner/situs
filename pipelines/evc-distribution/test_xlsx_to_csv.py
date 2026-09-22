import csv
import json
import os
import tempfile
import unittest
import zipfile

from xlsx_to_csv import (
    AllianceCodeCollisionError,
    CellValueError,
    EmptyResultError,
    HeaderError,
    SheetError,
    SlugCollisionError,
    col_index,
    convert,
    read_sheet,
    sheet_path,
    slugify,
)

SHEET = "European alliances"


def _esc(value):
    return value.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def _col_ref(ci):
    """0 -> A, 25 -> Z, 26 -> AA, 135 -> EF. The inverse of col_index, spelled
    out here so a fixture can name a column past Z at all."""
    ref = ""
    ci += 1
    while ci:
        ci, rem = divmod(ci - 1, 26)
        ref = chr(ord("A") + rem) + ref
    return ref


def make_workbook(path, rows, sheet_name=SHEET, extra_sheets=()):
    """Build a multi-sheet .xlsx on disk. rows is a list of dicts
    {column index: cell text} so a fixture can LEAVE OUT a cell the way Excel
    does — which is the whole point of the r-attribute reading."""
    sheets = [(sheet_name, rows)] + [(n, r) for n, r in extra_sheets]
    sheet_tags = "".join(
        f'<sheet name="{_esc(n)}" sheetId="{i + 1}" r:id="rId{i + 1}"/>'
        for i, (n, _) in enumerate(sheets)
    )
    workbook = (
        '<?xml version="1.0"?><workbook '
        'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
        'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
        f"<sheets>{sheet_tags}</sheets></workbook>"
    )
    rel_tags = "".join(
        f'<Relationship Id="rId{i + 1}" '
        'Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" '
        f'Target="worksheets/sheet{i + 1}.xml"/>'
        for i in range(len(sheets))
    )
    rels = (
        '<?xml version="1.0"?><Relationships '
        'xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        f"{rel_tags}</Relationships>"
    )
    with zipfile.ZipFile(path, "w") as z:
        z.writestr("xl/workbook.xml", workbook)
        z.writestr("xl/_rels/workbook.xml.rels", rels)
        z.writestr("xl/sharedStrings.xml", '<?xml version="1.0"?><sst></sst>')
        for i, (_, sheet_rows) in enumerate(sheets):
            body = ""
            for ri, cells in enumerate(sheet_rows, start=1):
                tags = "".join(
                    f'<c r="{_col_ref(ci)}{ri}" t="inlineStr"><is><t>{_esc(v)}</t></is></c>'
                    for ci, v in sorted(cells.items())
                )
                body += f'<row r="{ri}">{tags}</row>'
            z.writestr(
                f"xl/worksheets/sheet{i + 1}.xml",
                '<?xml version="1.0"?><worksheet '
                'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
                f"<sheetData>{body}</sheetData></worksheet>",
            )


def row(*pairs):
    return dict(pairs)


# The four meta columns and the four summary columns, spelled exactly as the
# pinned file spells them.
META = ["Code 1", "Code 2", "Name", "Name with author citation"]
SUMS = [
    "Verified occurrences",
    "Uncertain occurrences",
    "All occurrences",
    "% of uncertain occurrences",
]


def header(*territories):
    cells = {i: h for i, h in enumerate(META)}
    for i, t in enumerate(territories):
        cells[len(META) + i] = t
    for i, s in enumerate(SUMS):
        cells[len(META) + len(territories) + i] = s
    return cells


class ColIndexTest(unittest.TestCase):
    def test_single_letter_columns(self):
        self.assertEqual(col_index("A1"), 0)
        self.assertEqual(col_index("Z9"), 25)

    def test_two_letter_columns(self):
        self.assertEqual(col_index("AA1"), 26)
        self.assertEqual(col_index("AB7"), 27)

    def test_the_last_territory_column_of_the_pinned_file(self):
        # Column 140 (0-based 139) is "Ukraine Transcarpathian", the last
        # territory before the four summary columns.
        self.assertEqual(col_index("EJ1116"), 139)


class SlugifyTest(unittest.TestCase):
    def test_lowercases_and_hyphenates_a_space(self):
        self.assertEqual(slugify("Austria Alps"), "austria-alps")

    def test_hyphenates_the_coast_underscore(self):
        self.assertEqual(slugify("Albania_coast"), "albania-coast")

    def test_keeps_an_existing_hyphen(self):
        self.assertEqual(slugify("France Extra-Mediterranean"), "france-extra-mediterranean")

    def test_a_bare_country_is_its_own_slug(self):
        self.assertEqual(slugify("Armenia"), "armenia")


class ReadSheetTest(unittest.TestCase):
    def test_a_missing_cell_does_not_shift_the_following_ones(self):
        # THE test that makes positional reading fail. Excel omits the empty
        # cell at index 1; reading row.findall("c") in order would put "U" at
        # index 1 instead of 2.
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "gap.xlsx")
            make_workbook(xlsx, [row((0, "AB02C"), (2, "U"))])
            rows = read_sheet(xlsx, "xl/worksheets/sheet1.xml")
            self.assertEqual(rows[0], {0: "AB02C", 2: "U"})


class SheetSelectionTest(unittest.TestCase):
    def test_picks_the_named_sheet_not_the_first_one(self):
        # In the pinned file "Borja tabulka" is sheet1 and the data sheet is
        # sheet2. Taking the first non-legend sheet would parse the draft.
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "two.xlsx")
            make_workbook(
                xlsx,
                [header("Albania"), row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (4, "1"))],
                sheet_name="Borja tabulka",
                extra_sheets=[(SHEET, [header("Armenia"), row((0, "AB01A"), (1, "KOB-01A"), (2, "N"), (3, "N A"), (4, "1"))])],
            )
            self.assertEqual(sheet_path(xlsx, SHEET), "xl/worksheets/sheet2.xml")

    def test_a_missing_data_sheet_aborts_naming_it(self):
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "nodata.xlsx")
            make_workbook(xlsx, [header("Albania")], sheet_name="Read me")
            with self.assertRaises(SheetError) as ctx:
                convert(xlsx, tmp)
            self.assertIn(SHEET, str(ctx.exception))


class ConvertTest(unittest.TestCase):
    def _convert(self, rows, territories=("Albania", "Albania_coast", "Armenia")):
        tmp = tempfile.mkdtemp()
        self.addCleanup(lambda: None)
        xlsx = os.path.join(tmp, "dist.xlsx")
        make_workbook(xlsx, [header(*territories)] + list(rows))
        out_dir = os.path.join(tmp, "out")
        os.makedirs(out_dir)
        report = convert(xlsx, out_dir)
        return report, out_dir

    def _rows_of(self, out_dir, name):
        with open(os.path.join(out_dir, name), encoding="utf-8") as f:
            return list(csv.DictReader(f))

    def test_translates_the_cell_codes_into_the_long_form(self):
        report, out_dir = self._convert([
            row((0, "AB02C"), (1, "KOB-02C"), (2, "Festucion versicoloris"),
                (3, "Festucion versicoloris Krajina 1933"), (4, "1"), (6, "U")),
        ])
        got = self._rows_of(out_dir, "syntaxon_distribution.csv")
        self.assertEqual(got, [
            {"syntaxon_id": "AB02C", "area_scheme": "evc_territory",
             "area_code": "albania", "occurrence": "verified"},
            {"syntaxon_id": "AB02C", "area_scheme": "evc_territory",
             "area_code": "armenia", "occurrence": "uncertain"},
        ])
        self.assertEqual(report["verified"], 1)
        self.assertEqual(report["uncertain"], 1)

    def test_an_empty_cell_writes_no_row_at_all(self):
        _, out_dir = self._convert([
            row((0, "AA01A"), (1, "PAP-01A"), (2, "Papaverion dahliani"),
                (3, "Papaverion dahliani Hofmann ex Daniëls 2016"), (5, "1")),
        ])
        got = self._rows_of(out_dir, "syntaxon_distribution.csv")
        self.assertEqual([r["area_code"] for r in got], ["albania-coast"])

    def test_every_alliance_row_gets_a_coverage_row_even_with_no_occurrence(self):
        report, out_dir = self._convert([
            row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (4, "1")),
            row((0, "AB01A"), (1, "KOB-01A"), (2, "M"), (3, "M A")),
        ])
        got = self._rows_of(out_dir, "syntaxon_distribution_coverage.csv")
        self.assertEqual(got, [
            {"syntaxon_id": "AA01A", "area_scheme": "evc_territory"},
            {"syntaxon_id": "AB01A", "area_scheme": "evc_territory"},
        ])
        self.assertEqual(report["alliances"], 2)
        self.assertEqual(report["covered"], 2)

    def test_strips_a_trailing_blank_off_the_code(self):
        # Measured: 'JD02B ', 'JD02C ' and 'JE01B ' carry a trailing blank in
        # the pinned file. Unstripped they join with nothing.
        _, out_dir = self._convert([
            row((0, "JD02B "), (1, "AMM-02B"),
                (2, "Diantho attenuati-Scrophularion caninae"), (3, "x"), (4, "1")),
        ])
        got = self._rows_of(out_dir, "syntaxon_distribution.csv")
        self.assertEqual(got[0]["syntaxon_id"], "JD02B")

    def test_writes_one_territory_row_per_column_never_a_summary_column(self):
        report, out_dir = self._convert([
            row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"),
                (4, "1"), (7, "3"), (8, "0"), (9, "3"), (10, "0")),
        ])
        got = self._rows_of(out_dir, "evc_territories.csv")
        self.assertEqual([r["area_code"] for r in got], ["albania", "albania-coast", "armenia"])
        self.assertEqual([r["name_en"] for r in got],
                         ["Albania", "Albania_coast", "Armenia"])
        self.assertEqual(report["territories"], 3)
        # The four summary cells sit at 7..10 and must not become occurrences.
        self.assertEqual(report["verified"], 1)
        self.assertEqual(report["uncertain"], 0)

    def test_skips_the_four_trailing_summary_rows(self):
        report, out_dir = self._convert([
            row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (4, "1")),
            row((0, "Verified occurrences"), (4, "133")),
            row((0, "Uncertain occurrences"), (4, "77")),
            row((0, "All occurrences"), (4, "210")),
            row((0, "% of uncertain occurrences"), (4, "36.666666666666664")),
        ])
        self.assertEqual(report["alliances"], 1)
        self.assertEqual(report["summary_rows"], 4)
        self.assertEqual(report["skipped_rows"], 0)
        got = self._rows_of(out_dir, "syntaxon_distribution.csv")
        self.assertEqual([r["syntaxon_id"] for r in got], ["AA01A"])

    def test_an_unknown_cell_value_aborts_naming_value_row_and_column(self):
        with self.assertRaises(CellValueError) as ctx:
            self._convert([
                row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (5, "2")),
            ])
        message = str(ctx.exception)
        self.assertIn("'2'", message)
        self.assertIn("AA01A", message)
        self.assertIn("Albania_coast", message)

    def test_a_slug_collision_aborts_naming_both_columns(self):
        with self.assertRaises(SlugCollisionError) as ctx:
            self._convert(
                [row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"))],
                territories=("Austria Alps", "Austria_Alps"),
            )
        message = str(ctx.exception)
        self.assertIn("Austria Alps", message)
        self.assertIn("Austria_Alps", message)

    def test_two_identical_column_headers_abort_too(self):
        # The collision check used to compare the ORIGINAL names, so two
        # columns spelled exactly alike slipped past it and produced two
        # evc_territories.csv rows on one area_code — silently merged later
        # by the SQLite primary key.
        with self.assertRaises(SlugCollisionError) as ctx:
            self._convert(
                [row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"))],
                territories=("Austria Alps", "Austria Alps"),
            )
        self.assertIn("Austria Alps", str(ctx.exception))

    def test_a_repeated_alliance_code_aborts_before_any_csv(self):
        # Two rows on one alliance code do not merely duplicate a row: the
        # coverage table's primary key and the distribution ingest's upsert
        # both resolve on (syntaxon_id, area_code), so the published result
        # carries the first row's cells wherever the second one is empty and
        # the second one's everywhere else — neither source row.
        tmp = tempfile.mkdtemp()
        xlsx = os.path.join(tmp, "dist.xlsx")
        make_workbook(xlsx, [
            header("Albania", "Albania_coast"),
            row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (4, "1")),
            row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (5, "U")),
        ])
        out_dir = os.path.join(tmp, "out")
        os.makedirs(out_dir)
        with self.assertRaises(AllianceCodeCollisionError) as ctx:
            convert(xlsx, out_dir)
        self.assertIn("AA01A", str(ctx.exception))
        self.assertEqual(sorted(os.listdir(out_dir)), [])

    def test_the_repeated_code_is_recognized_after_stripping(self):
        # 'JD02B ' and 'JD02B' are one code, as everywhere else in this
        # converter — the collision must not hinge on a trailing blank.
        tmp = tempfile.mkdtemp()
        xlsx = os.path.join(tmp, "dist.xlsx")
        make_workbook(xlsx, [
            header("Albania"),
            row((0, "JD02B "), (1, "AMM-02B"), (2, "N"), (3, "N A"), (4, "1")),
            row((0, "JD02B"), (1, "AMM-02B"), (2, "N"), (3, "N A")),
        ])
        out_dir = os.path.join(tmp, "out")
        os.makedirs(out_dir)
        with self.assertRaises(AllianceCodeCollisionError):
            convert(xlsx, out_dir)

    def test_a_sheet_without_a_single_alliance_aborts_before_any_csv(self):
        # build.sh clears out/ before the run and IngestSyntaxonDistribution
        # treats present files as a REPLACEMENT: header-only CSVs would clear
        # the whole syntaxon distribution and commit an empty one. The run must
        # fail, and fail before writing anything the ingest could collect.
        tmp = tempfile.mkdtemp()
        xlsx = os.path.join(tmp, "dist.xlsx")
        make_workbook(xlsx, [header("Albania"), row((0, "not an alliance"))])
        out_dir = os.path.join(tmp, "out")
        os.makedirs(out_dir)
        with self.assertRaises(EmptyResultError) as ctx:
            convert(xlsx, out_dir)
        self.assertIn("alliance", str(ctx.exception))
        self.assertEqual(sorted(os.listdir(out_dir)), [])

    def test_a_data_sheet_with_only_a_header_aborts_too(self):
        tmp = tempfile.mkdtemp()
        xlsx = os.path.join(tmp, "dist.xlsx")
        make_workbook(xlsx, [header("Albania")])
        out_dir = os.path.join(tmp, "out")
        os.makedirs(out_dir)
        with self.assertRaises(EmptyResultError):
            convert(xlsx, out_dir)
        self.assertEqual(sorted(os.listdir(out_dir)), [])

    def test_a_missing_meta_column_raises_header_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "dist.xlsx")
            make_workbook(xlsx, [{0: "Code 1", 1: "Name"}])
            out_dir = os.path.join(tmp, "out")
            os.makedirs(out_dir)
            with self.assertRaises(HeaderError):
                convert(xlsx, out_dir)

    def test_a_fifth_unrecognized_summary_column_aborts_instead_of_becoming_a_territory(self):
        # A future format change might add a fifth summary column. It must
        # not be silently read as a 137th territory just because its cell
        # values happen to look plausible.
        head = {
            0: "Code 1", 1: "Code 2", 2: "Name", 3: "Name with author citation",
            4: "Albania", 5: "Albania_coast", 6: "Armenia",
            7: "Verified occurrences", 8: "Uncertain occurrences",
            9: "All occurrences", 10: "% of uncertain occurrences",
            11: "Total records",
        }
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "dist.xlsx")
            make_workbook(xlsx, [head, row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (4, "1"))])
            out_dir = os.path.join(tmp, "out")
            os.makedirs(out_dir)
            with self.assertRaises(HeaderError) as ctx:
                convert(xlsx, out_dir)
            self.assertIn("Total records", str(ctx.exception))

    def test_a_summary_column_before_a_territory_column_aborts(self):
        # The summary columns are structurally expected at the END of the
        # sheet. A summary column that precedes a territory column is not
        # the layout this parser recognizes, so it must abort rather than
        # silently reorder or silently drop the later territory.
        head = {
            0: "Code 1", 1: "Code 2", 2: "Name", 3: "Name with author citation",
            4: "Albania", 5: "Verified occurrences", 6: "Albania_coast",
        }
        with tempfile.TemporaryDirectory() as tmp:
            xlsx = os.path.join(tmp, "dist.xlsx")
            make_workbook(xlsx, [head, row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (4, "1"))])
            out_dir = os.path.join(tmp, "out")
            os.makedirs(out_dir)
            with self.assertRaises(HeaderError) as ctx:
                convert(xlsx, out_dir)
            self.assertIn("Albania_coast", str(ctx.exception))

    def test_the_report_carries_the_measured_value_histogram(self):
        report, _ = self._convert([
            row((0, "AA01A"), (1, "PAP-01A"), (2, "N"), (3, "N A"), (4, "1"), (5, "U")),
        ])
        self.assertEqual(report["value_histogram"], {"": 1, "1": 1, "U": 1})
        self.assertEqual(report["slug_collisions"], [])


if __name__ == "__main__":
    unittest.main()
