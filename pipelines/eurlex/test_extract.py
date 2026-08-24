#!/usr/bin/env python3
"""Tests for extract.py. The fixture is embedded, so the suite never touches the
network — a test that needs EUR-Lex to be up is a test that fails for reasons
that have nothing to do with the code."""
import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from extract import (  # noqa: E402
    build_report,
    extract_annex1,
    rows,
    write_csv,
)

# Mirrors the measured structure of CELEX 01992L0043-20130701: term/definition
# pairs in one-row tables, inline <span class="italics"> for scientific names,
# a leading asterisk for priority types, NBSP in the text.
FIXTURE = """
<p>ANHANG I</p>
<table><tr><td><p class="dlist-term">1150</p></td>
<td><p class="dlist-definition">* Lagunen des Küstenraumes (Strandseen)</p></td></tr></table>
<table><tr><td><p class="dlist-term">6510</p></td>
<td><p class="dlist-definition">Magere Flachland-Mähwiesen
(<span class="italics">Alopecurus pratensis</span>)</p></td></tr></table>
<table><tr><td><p class="dlist-term">9370</p></td>
<td><p class="dlist-definition">* Palmhaine von <span class="italics">Phönix</span>
</p></td></tr></table>
<table><tr><td><p class="dlist-term">21A0</p></td>
<td><p class="dlist-definition">Machair (* in Irland)</p></td></tr></table>
<p>ANHANG II</p>
<table><tr><td><p class="dlist-term">1352</p></td>
<td><p class="dlist-definition">Canis lupus</p></td></tr></table>
"""


class TestExtract(unittest.TestCase):
    def setUp(self):
        self.names = extract_annex1(FIXTURE)

    def test_extracts_code_and_german_name(self):
        self.assertEqual(self.names["1150"], "Lagunen des Küstenraumes (Strandseen)")
        self.assertEqual(self.names["9370"], "Palmhaine von Phönix")

    def test_keeps_alphanumeric_codes(self):
        # 44 of the 233 real entries look like this. A \\d{4} filter would drop
        # them without a word.
        self.assertIn("21A0", self.names)

    def test_strips_only_the_leading_priority_asterisk(self):
        self.assertFalse(self.names["1150"].startswith("*"))
        # An asterisk inside the text is part of the name, not a marker.
        self.assertEqual(self.names["21A0"], "Machair (* in Irland)")

    def test_keeps_scientific_names_dropping_only_the_markup(self):
        self.assertEqual(
            self.names["6510"],
            "Magere Flachland-Mähwiesen (Alopecurus pratensis)",
        )

    def test_stops_at_annex_ii(self):
        # The same dlist markup carries species in later annexes; extracting
        # from the whole document would mix a wolf into the habitat types.
        self.assertNotIn("1352", self.names)

    def test_refuses_a_document_without_the_annex_i_boundaries(self):
        with self.assertRaises(ValueError):
            extract_annex1("<p>nothing here</p>")


class TestReport(unittest.TestCase):
    def test_reports_both_directions(self):
        report = build_report({"1150": "a", "6510": "b"}, ["6510", "9999"])
        self.assertEqual(report["eurlex_codes"], 2)
        self.assertEqual(report["missing_in_index"], ["1150"])
        self.assertEqual(report["without_official_name"], ["9999"])

    def test_an_indexed_type_without_an_official_name_is_visible(self):
        # This is the direction that matters: it must never be discovered by
        # someone noticing an English label in the app.
        report = build_report({}, ["6510"])
        self.assertEqual(report["without_official_name_count"], 1)


class TestRows(unittest.TestCase):
    def test_every_row_is_official_and_attributed(self):
        for row in rows({"6510": "Magere Flachland-Mähwiesen"}):
            self.assertEqual(row["provenance"], "official")
            self.assertEqual(row["source"], "eur-lex:31992L0043")
            self.assertEqual(row["entity_key"], "annex1:6510")
            self.assertEqual(row["lang"], "de")
            self.assertEqual(row["field"], "name")

    def test_csv_roundtrip_keeps_umlauts(self):
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "localizations.csv")
            write_csv(path, {"6510": "Magere Flachland-Mähwiesen"})
            with open(path, encoding="utf-8") as fh:
                self.assertIn("Mähwiesen", fh.read())


if __name__ == "__main__":
    unittest.main()
