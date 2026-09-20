"""Tests for convert.py -- the parsing rules, against fixtures, no poppler."""
import unittest

import convert


def page(nodes):
    """Builds one pdftohtml -xml page from (top, left, text, bold) tuples."""
    body = "".join(
        f'<text top="{t}" left="{l}" width="10" height="13" font="0">'
        + (f"<b>{x}</b>" if b else x)
        + "</text>"
        for t, l, x, b in nodes
    )
    return f'<page number="1" height="1262" width="892">{body}</page>'


class ParsePagesTest(unittest.TestCase):
    def test_extracts_code_name_and_description(self):
        xml = page([
            (144, 80, "N11 – Atlantic, Baltic and Arctic sand beach", True),
            (200, 80, "Beaches of the Atlantic coast.", False),
            (400, 80, "Corresponding alliances in EuroVegChecklist 2016", False),
            (420, 80, "> AMM-01A Ammophilion", False),
        ])
        self.assertEqual(convert.parse_pages(xml),
                         [("N11", "Atlantic, Baltic and Arctic sand beach",
                           "Beaches of the Atlantic coast.")])

    # The reason this pipeline uses pdftohtml at all: a wrapped title is set
    # in body-text size, so only the bold flag keeps it out of the description.
    def test_a_wrapped_title_stays_out_of_the_description(self):
        xml = page([
            (144, 80, "MA223 – Atlantic upper-mid saltmarsh and reed, rush", True),
            (170, 80, "and sedge bed", True),
            (200, 80, "Middle zone of Atlantic salt marshes.", False),
        ])
        code, name, desc = convert.parse_pages(xml)[0]
        self.assertEqual(name, "Atlantic upper-mid saltmarsh and reed, rush and sedge bed")
        self.assertEqual(desc, "Middle zone of Atlantic salt marshes.")

    # pdftohtml emits nodes unordered: the species table of a page can precede
    # its title. Unsorted, the description would collect the species list.
    def test_nodes_are_read_in_reading_order_not_document_order(self):
        xml = page([
            (900, 132, "Ammophila arenaria", False),
            (144, 80, "N13 – Shifting coastal dune", True),
            (200, 80, "Dunes with marram grass.", False),
            (850, 80, "Characteristic species combination", False),
        ])
        self.assertEqual(convert.parse_pages(xml)[0][2], "Dunes with marram grass.")

    def test_description_stops_at_every_section_heading(self):
        for heading in ("Corresponding alliances in EuroVegChecklist 2016",
                        "Characteristic species combination",
                        "Diagnostic species (phi coefficient * 100)",
                        "Constant species (percentage frequencies)",
                        "Dominant species (percentage frequencies)"):
            with self.subTest(heading=heading):
                xml = page([
                    (144, 80, "R22 – Hay meadow", True),
                    (200, 80, "Meadows.", False),
                    (300, 80, heading, False),
                    (320, 80, "must not appear", False),
                ])
                self.assertEqual(convert.parse_pages(xml)[0][2], "Meadows.")

    # Front matter and the map/species pages carry no bold title line.
    def test_a_page_without_a_title_is_skipped(self):
        xml = page([(144, 80, "EUNIS Habitat Factsheets", True),
                    (200, 80, "version 2021-06-01", False)])
        self.assertEqual(convert.parse_pages(xml), [])

    def test_entities_and_whitespace_are_normalised(self):
        xml = page([
            (144, 80, "Q3 – Palsa mires", True),
            (200, 80, "Mires &amp;  bogs", False),
            (210, 80, "in the north.", False),
        ])
        self.assertEqual(convert.parse_pages(xml)[0][2], "Mires & bogs in the north.")


class GroupIntroTest(unittest.TestCase):
    # The source glues the next EUNIS group's introduction onto the end of the
    # last factsheet of a group, with no separator at all.
    def test_cuts_a_glued_on_group_introduction(self):
        text = ("Clear-felled or burnt land that previously supported forest."
                "Non-coastal habitats on substrates with no or little development of soil, "
                "mostly with less than 30% vegetation cover.")
        got, marker = convert.strip_group_intro(text)
        self.assertEqual(got, "Clear-felled or burnt land that previously supported forest.")
        self.assertTrue(marker)

    def test_leaves_an_untouched_description_alone(self):
        text = "Boreal and Arctic sparsely vegetated siliceous boulders."
        self.assertEqual(convert.strip_group_intro(text), (text, None))

    # U22 glues two sentences of its OWN text together. Cutting there would
    # lose real description, so only the listed group introductions are cut.
    def test_an_internal_glued_sentence_is_kept(self):
        text = ("Siliceous screes at cool sites in mountain ranges of Europe."
                "The screes are colonised by mostly acidophilous plants.")
        self.assertEqual(convert.strip_group_intro(text), (text, None))
        self.assertTrue(convert.GLUED_RE.search(text), "the join should still be reportable")

    # U53 ends "...(U4-3).Hard rock surfaces..." — the join sits behind a
    # closing bracket, which a letters-only pattern walks straight past.
    def test_detects_a_join_behind_a_closing_bracket(self):
        self.assertTrue(convert.GLUED_RE.search("excluded (U4-3).Hard rock surfaces"))
