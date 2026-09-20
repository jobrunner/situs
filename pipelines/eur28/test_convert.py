"""Tests for convert.py — the parsing rules, against fixtures, no poppler."""
import unittest

import convert


ENTRY = """\
6230                            * Species-rich Nardus grasslands, on siliceous
                                substrates in mountain areas
PAL.CLASS.: 35.1, 36.31

1)      Closed, dry or mesophile, perennial Nardus grasslands occupying siliceous soils.
        Species-rich sites should be interpreted as sites remarkable for a high number of species.

2)      Plants: Arnica montana, Nardus stricta.
        Animals: Miramella alpina.

3)      Corresponding categories
        German classification : "34060101 gemaehter Borstgrasrasen".

5)      Sjoers, H. (1967). Nordisk vaextgeografi.
"""


class ParseEntriesTest(unittest.TestCase):
    def test_reads_code_name_palclass_and_sections(self):
        entry = convert.parse_entries(ENTRY)[0]
        self.assertEqual(entry["code"], "6230")
        self.assertEqual(entry["name"],
                         "* Species-rich Nardus grasslands, on siliceous substrates in mountain areas")
        self.assertEqual(entry["pal_class"], "35.1 36.31")
        self.assertTrue(entry["definition"].startswith("Closed, dry or mesophile"))
        self.assertIn("high number of species", entry["definition"])
        self.assertTrue(entry["species"].startswith("Plants: Arnica montana"))
        self.assertIn("German classification", entry["categories"])
        # Section 5 is literature and has no place in the working material.
        self.assertNotIn("Nordisk", entry["definition"])

    # Codes run 1110, 62C0 and 91AA: the last position is not always a digit.
    def test_accepts_letters_in_the_last_code_position(self):
        for code in ("1110", "62C0", "91AA", "91E0"):
            with self.subTest(code=code):
                text = f"{code}    Some habitat name\nPAL.CLASS.: 41.1\n\n1)  A definition.\n"
                self.assertEqual(convert.parse_entries(text)[0]["code"], code)

    # The manual's "Explanatory Notes" page carries a fully annotated specimen
    # entry for 2140. Parsed naively it yields a second, bogus 2140.
    def test_skips_the_annotated_specimen_entry(self):
        text = ("Explanatory Notes\n"
                "  Natura 2000 code; this is the four digit\n"
                "  code given in the standard data-entry form\n"
                "     2140      * Decalcified fixed dunes\n"
                "     PAL.CLASS.: 16.23\n"
                "     1) Decalcified dunes colonised by Empetrum nigrum heaths.\n")
        self.assertEqual(convert.parse_entries(text), [])

    def test_running_header_and_footer_are_dropped(self):
        text = ("4010    Northern Atlantic wet heaths\n"
                "PAL.CLASS.: 31.11\n\n"
                "1)      Wet heaths with Erica tetralix.\n"
                "Interpretation Manual - EUR28                         Page 31\n"
                "        They occur on peaty soils.\n")
        definition = convert.parse_entries(text)[0]["definition"]
        self.assertIn("Wet heaths with Erica tetralix. They occur on peaty soils.", definition)
        self.assertNotIn("Interpretation Manual", definition)
        self.assertNotIn("Page 31", definition)

    def test_a_wrapped_heading_is_joined(self):
        text = ("91E0                * Alluvial forests with Alnus glutinosa and Fraxinus excelsior\n"
                "                    (Alno-Padion, Alnion incanae, Salicion albae)\n"
                "PAL.CLASS.: 44.3, 44.2, 44.13\n\n"
                "1)      Riparian forests.\n")
        entry = convert.parse_entries(text)[0]
        self.assertEqual(entry["name"],
                         "* Alluvial forests with Alnus glutinosa and Fraxinus excelsior "
                         "(Alno-Padion, Alnion incanae, Salicion albae)")

    def test_an_entry_without_sections_still_yields_its_code(self):
        text = "1110    Sandbanks\nPAL.CLASS.: 11.125\n\nNo numbered sections here.\n"
        entry = convert.parse_entries(text)[0]
        self.assertEqual(entry["code"], "1110")
        self.assertEqual(entry["definition"], "")
