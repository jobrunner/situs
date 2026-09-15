"""Tests for convert.py — the guards, not the happy path alone."""
import json
import os
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
CONVERT = os.path.join(HERE, "convert.py")
HEADER = "L3 code*L3 area*L2 code*L3 ISOcode*Ed2status*Notes\r\n"


def run(source_text, encoding="cp1252"):
    """Runs convert.py over source_text, returns (returncode, csv, report)."""
    with tempfile.TemporaryDirectory() as d:
        src = os.path.join(d, "tblLevel3.txt")
        out = os.path.join(d, "wgsrpd_areas.csv")
        with open(src, "w", encoding=encoding, newline="") as f:
            f.write(source_text)
        proc = subprocess.run([sys.executable, CONVERT, src, out],
                              capture_output=True, text=True)
        csv_text = None
        report = None
        if os.path.exists(out):
            with open(out, encoding="utf-8") as f:
                csv_text = f.read()
        report_path = os.path.join(d, "report.json")
        if os.path.exists(report_path):
            with open(report_path, encoding="utf-8") as f:
                report = json.load(f)
        return proc, csv_text, report


class ConvertTest(unittest.TestCase):
    def test_emits_scheme_code_and_name(self):
        proc, csv_text, report = run(HEADER + "GER*Germany*11,00*DE**\r\n")
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertEqual(csv_text.splitlines()[0], "area_scheme,area_code,name_en")
        self.assertIn("wgsrpd_l3,GER,Germany", csv_text)
        self.assertEqual(report["areas"], 1)
        self.assertEqual(report["skipped_rows"], 0)

    def test_decodes_the_source_encoding(self):
        _, csv_text, _ = run(HEADER + "FOR*Føroyar*10,00*FO**\r\n")
        self.assertIn("wgsrpd_l3,FOR,Føroyar", csv_text)

    # A truncated line is what the skipped counter exists for: it must be
    # counted and stepped over, never abort the run with a traceback.
    def test_a_short_row_is_skipped_not_fatal(self):
        proc, csv_text, report = run(HEADER + "GER\r\nAUT*Austria*11,00*AT**\r\n")
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertEqual(report["areas"], 1)
        self.assertEqual(report["skipped_rows"], 1)
        self.assertIn("wgsrpd_l3,AUT,Austria", csv_text)

    def test_a_duplicate_code_is_written_once(self):
        _, csv_text, report = run(HEADER
                                  + "GER*Germany*11,00*DE**\r\n"
                                  + "GER*Germany again*11,00*DE**\r\n")
        self.assertEqual(report["areas"], 1)
        self.assertEqual(report["skipped_rows"], 1)
        self.assertNotIn("again", csv_text)

    # The header is the only check that the pinned artifact is still the file
    # this converter was written for — a changed layout must stop the run.
    def test_an_unexpected_header_fails(self):
        proc, _, _ = run("code*area\r\nGER*Germany\r\n")
        self.assertNotEqual(proc.returncode, 0)
        self.assertIn("unexpected header", proc.stderr)


if __name__ == "__main__":
    unittest.main()
