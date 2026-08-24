#!/usr/bin/env python3
"""Tests for merge.py — the promises the authored file has to keep."""
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from merge import normalise, rows_for, validate  # noqa: E402


def entry(code, name_en="English", name_de="Deutsch", vernacular=""):
    return {"code": code, "name_en": name_en, "name_de": name_de,
            "vernacular_de": vernacular}


class TestRows(unittest.TestCase):
    def test_every_row_is_situs_and_versioned(self):
        for row in rows_for(entry("R22", vernacular="Glatthaferwiese"), "0.3.0"):
            self.assertEqual(row["provenance"], "situs")
            self.assertEqual(row["source"], "situs@0.3.0")
            self.assertEqual(row["entity_key"], "eunis@2021:R22")
            self.assertEqual(row["lang"], "de")

    def test_an_empty_vernacular_emits_no_row(self):
        # Absent is information. An empty value would assert that the type has an
        # established German term which happens to be the empty string.
        fields = [r["field"] for r in rows_for(entry("R12"), "0.3.0")]
        self.assertEqual(fields, ["name"])

    def test_a_vernacular_emits_a_second_row(self):
        fields = [r["field"] for r in rows_for(entry("R22", vernacular="X"), "0.3.0")]
        self.assertEqual(fields, ["name", "vernacular"])

    def test_the_control_column_never_reaches_the_index(self):
        for row in rows_for(entry("R22", name_en="Low altitude hay meadow"), "0.3.0"):
            self.assertNotIn("Low altitude hay meadow", row.values())


class TestValidate(unittest.TestCase):
    def test_clean_file_has_no_problems(self):
        self.assertEqual(validate([entry("R22"), entry("R35")]), [])

    def test_a_vernacular_may_never_stand_alone(self):
        problems = validate([entry("R22", name_de="", vernacular="Glatthaferwiese")])
        self.assertTrue(any("no name_de" in p for p in problems), problems)

    def test_duplicate_codes_are_caught(self):
        self.assertTrue(any("duplicate" in p for p in validate([entry("R22"), entry("R22")])))

    def test_a_code_with_a_derived_name_must_not_be_authored(self):
        # Where a '=' crosswalk lends the official Annex I name, inventing one is
        # exactly what the spec forbids.
        problems = validate([entry("R22"), entry("N33")], expected_codes=["R22"])
        self.assertTrue(any("must not be authored" in p for p in problems), problems)

    def test_a_missing_code_is_caught(self):
        problems = validate([entry("R22")], expected_codes=["R22", "R35"])
        self.assertTrue(any("missing" in p for p in problems), problems)

    def test_name_en_drift_is_caught(self):
        # The control column is worthless if it does not match what is being
        # translated — a silent drift would make review look done when it is not.
        problems = validate(
            [entry("R22", name_en="Something else")],
            index_names={"R22": "Low and medium altitude hay meadow"},
        )
        self.assertTrue(any("drifted" in p for p in problems), problems)

    def test_nbsp_is_not_drift(self):
        # The EEA source data carries U+00A0 inside at least one name_en (V31).
        # A byte comparison would report a translation error where there is only
        # an invisible character from upstream.
        problems = validate(
            [entry("V31", name_en="a b c")],
            index_names={"V31": "a\xa0b c"},
        )
        self.assertEqual(problems, [])


class TestNormalise(unittest.TestCase):
    def test_collapses_nbsp_and_runs_of_space(self):
        self.assertEqual(normalise("a\xa0b   c\n"), "a b c")


if __name__ == "__main__":
    unittest.main()
