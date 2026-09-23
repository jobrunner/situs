#!/usr/bin/env python3
"""Tests for merge.py — the promises the authored file has to keep."""
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from merge import normalise, rows_for, validate, version_problem  # noqa: E402


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

class TestSchemaDrift(unittest.TestCase):
    """A guardrail that crashes is not a guardrail. Schema drift must come back as
    a problem list, not as a KeyError three frames down."""

    def test_a_missing_column_is_reported_not_raised(self):
        problems = validate([{"code": "R22", "name_en": "x"}])  # no name_de at all
        self.assertTrue(problems, "expected a problem list, got none")
        self.assertTrue(any("column" in p or "name_de" in p for p in problems), problems)

    def test_a_short_row_is_reported_not_raised(self):
        # csv.DictReader fills missing trailing cells with None, which used to
        # blow up on .strip().
        problems = validate([{"code": "R22", "name_en": "x", "name_de": None,
                              "vernacular_de": None}])
        self.assertTrue(problems, "expected a problem list, got none")

    def test_a_missing_code_column_is_reported_not_raised(self):
        problems = validate([{"name_en": "x", "name_de": "y"}])
        self.assertTrue(problems, "expected a problem list, got none")

    def test_rows_for_tolerates_an_absent_vernacular_column(self):
        fields = [r["field"] for r in rows_for({"code": "R22", "name_de": "x"}, "0.3.0")]
        self.assertEqual(fields, ["name"])


if __name__ == "__main__":
    unittest.main()


class TestVersionStamp(unittest.TestCase):
    """The source field is data. A version that is not a version poisons it.

    This is not hypothetical: the prod index built on 2026-08-31 carries
    `situs@0.2.0 # x-release-please-version` in `localization.source`, because
    the README told the caller to pass `$(cat VERSION)` and that file keeps a
    release-please marker behind the number. Nothing noticed for three weeks.
    """

    def test_a_bare_version_is_accepted(self):
        self.assertEqual(version_problem("0.13.0"), "")

    def test_the_release_please_marker_is_rejected(self):
        problem = version_problem("0.13.0 # x-release-please-version")
        self.assertIn("0.13.0 # x-release-please-version", problem)
        # The message has to name the fix, not just the fault: the caller is a
        # shell line in a README, and "invalid version" would send them reading
        # merge.py instead of correcting the pipe.
        self.assertIn("cut -d' ' -f1", problem)

    def test_any_whitespace_is_rejected(self):
        self.assertNotEqual(version_problem("0.13.0\n"), "")
        self.assertNotEqual(version_problem("0 13"), "")

    def test_an_empty_version_is_rejected(self):
        self.assertNotEqual(version_problem(""), "")

    # A comment marker without whitespace would still be a comment, and a
    # comma would split the CSV column it is written into.
    def test_a_structural_character_is_rejected(self):
        for bad in ("0.13.0#x", "0.13.0,extra", '0.13.0"'):
            with self.subTest(bad=bad):
                self.assertNotEqual(version_problem(bad), "")
