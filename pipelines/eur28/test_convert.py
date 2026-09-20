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
    # entry for 2140, which parses exactly like a real one. The catalogue
    # entry comes later in the document and is the one that must survive.
    def test_the_catalogue_entry_wins_over_the_front_matter_specimen(self):
        text = ("Explanatory Notes\n"
                "  Natura 2000 code; this is the four digit code\n"
                "     2140      * Decalcified fixed dunes\n"
                "     PAL.CLASS.: 16.23\n"
                "     1) A specimen shown in the front matter.\n\n"
                "2140    * Decalcified fixed dunes with Empetrum nigrum\n"
                "PAL.CLASS.: 16.23\n\n"
                "1)      Decalcified dunes colonised by Empetrum nigrum heaths of the coasts.\n")
        entries, dropped = convert.parse_entries(text, count_skipped=True)
        self.assertEqual(len(entries), 1)
        self.assertEqual(entries[0]["name"], "* Decalcified fixed dunes with Empetrum nigrum")
        self.assertTrue(entries[0]["definition"].startswith("Decalcified dunes colonised"))
        self.assertEqual(dropped, 1)

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


class BoundaryTest(unittest.TestCase):
    TWO = ("6510    Lowland hay meadows\n"
           "PAL.CLASS.: 38.2\n\n"
           "1)      Species-rich hay meadows.\n\n"
           "3)      Corresponding categories\n"
           "        German classification: \"Gluehwiese\".\n\n"
           "6520    Mountain hay meadows\n"
           "PAL.CLASS.: 38.3\n\n"
           "1)      Mesophile hay meadows of the mountains.\n")

    # The block between two PAL.CLASS. anchors ends with the NEXT entry's
    # heading. Without cutting there, every entry's last section swallows the
    # following code and name.
    def test_an_entry_does_not_swallow_the_next_heading(self):
        first, second = convert.parse_entries(self.TWO)
        self.assertEqual(first["code"], "6510")
        self.assertNotIn("6520", first["categories"])
        self.assertNotIn("Mountain hay meadows", first["categories"])
        self.assertEqual(first["categories"],
                         'Corresponding categories German classification: "Gluehwiese".')
        self.assertEqual(second["code"], "6520")
        self.assertEqual(second["definition"], "Mesophile hay meadows of the mountains.")

    def test_an_entry_ending_at_section_one_keeps_its_definition_clean(self):
        text = ("4010    Northern Atlantic wet heaths\n"
                "PAL.CLASS.: 31.11\n\n"
                "1)      Wet heaths with Erica tetralix.\n\n"
                "4020    * Temperate Atlantic wet heaths\n"
                "PAL.CLASS.: 31.12\n\n"
                "1)      Wet heaths of the Atlantic coast.\n")
        first = convert.parse_entries(text)[0]
        self.assertEqual(first["definition"], "Wet heaths with Erica tetralix.")

    # The specimen-entry guard must look at the heading, not at the previous
    # entry's prose: the phrase can legitimately appear in a definition, and
    # dropping the following habitat type over it would be silent data loss.
    def test_the_specimen_guard_does_not_react_to_prose(self):
        text = ("1110    Sandbanks\n"
                "PAL.CLASS.: 11.125\n\n"
                "1)      Sandbanks reported under their Natura 2000 code in the standard form.\n\n"
                "6510    Lowland hay meadows\n"
                "PAL.CLASS.: 38.2\n\n"
                "1)      Species-rich hay meadows.\n")
        self.assertEqual([e["code"] for e in convert.parse_entries(text)], ["1110", "6510"])

    # A heading that cannot be read must be counted, not invented from
    # whatever four digits the backward scan finds in the references above.
    def test_an_unreadable_heading_is_skipped_and_counted(self):
        text = ("1110    Sandbanks\n"
                "PAL.CLASS.: 11.125\n\n"
                "1)      Sandbanks.\n"
                "5)      Sjoers, H. (1967). Nordisk vaextgeografi. 1967\n\n"
                "        no code on this line at all\n"
                "PAL.CLASS.: 38.2\n\n"
                "1)      Species-rich hay meadows.\n")
        entries, skipped = convert.parse_entries(text, count_skipped=True)
        self.assertEqual([e["code"] for e in entries], ["1110"])
        self.assertEqual(skipped, 1)
        self.assertNotIn("1967", [e["code"] for e in entries])
