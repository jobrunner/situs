package main

import (
	"bytes"
	"encoding/csv"
	"os"
	"slices"
	"strings"
	"testing"
)

// The curated CSVs in data/ are what makes an ingest reproducible: they were
// written with AI assistance and cannot be regenerated, so they are versioned
// rather than produced. These tests are the only thing standing between a
// careless edit and a shipped index, because no pipeline validates them.

// Each file is read through its own function rather than one taking a path: a
// literal path is what keeps this out of gosec's file-inclusion rule.
func annexDescriptions(t *testing.T) [][]string {
	t.Helper()
	content, err := os.ReadFile("../../data/annex1_descriptions.csv")
	if err != nil {
		t.Fatalf("reading data/annex1_descriptions.csv: %v", err)
	}
	return parseCurated(t, "data/annex1_descriptions.csv", content, 3)
}

func descriptionOverlay(t *testing.T) [][]string {
	t.Helper()
	content, err := os.ReadFile("../../data/localizations_descriptions.csv")
	if err != nil {
		t.Fatalf("reading data/localizations_descriptions.csv: %v", err)
	}
	return parseCurated(t, "data/localizations_descriptions.csv", content, 7)
}

func parseCurated(t *testing.T, name string, content []byte, columns int) [][]string {
	t.Helper()
	records, err := csv.NewReader(bytes.NewReader(content)).ReadAll()
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	if len(records) < 2 {
		t.Fatalf("%s carries no rows", name)
	}
	// encoding/csv only holds rows against the HEADER, so a file edited down
	// to fewer columns parses cleanly and every index below panics. Checked
	// once here, the tests report the defect instead of crashing on it.
	if len(records[0]) != columns {
		t.Fatalf("%s has %d columns, want %d", name, len(records[0]), columns)
	}
	return records
}

// The house style for texts situs writes itself: no dashes standing in for
// punctuation, no markup. It does NOT apply to wording situs only passes on:
// the English EUNIS descriptions are their authors' text and come from the
// pipeline, not from here.
func TestCuratedTextsFollowTheHouseStyle(t *testing.T) {
	forbidden := []struct{ needle, what string }{
		{"\u2014", "em dash"},
		{"\u2013", "en dash"},
		{" - ", "hyphen as punctuation"},
		// Not a bare "*": the asterisk is the official marker of a priority
		// habitat type ("*91E0"), and a description may well quote it. Only
		// the doubled form is markup.
		{"**", "markup"},
		{"`", "markup"},
		{"](", "markup link"},
	}

	for _, record := range annexDescriptions(t)[1:] {
		for _, rule := range forbidden {
			if strings.Contains(record[2], rule.needle) {
				t.Errorf("annex1 %s (en) contains %s", record[1], rule.what)
			}
		}
	}

	for _, record := range descriptionOverlay(t)[1:] {
		if record[6] != "situs" {
			continue
		}
		for _, rule := range forbidden {
			if strings.Contains(record[4], rule.needle) {
				t.Errorf("%s (%s) contains %s", record[1], record[2], rule.what)
			}
		}
	}

	// The German formation names are situs' own wording too, so the same rules
	// hold. The hyphen rule matters here: "Hoch- und Niedermoore" is a suspended
	// compound and must not drift into " - ".
	for _, record := range syntaxonLabels(t)[1:] {
		for _, rule := range forbidden {
			if strings.Contains(record[4], rule.needle) {
				t.Errorf("syntaxon %s (%s) contains %s", record[1], record[3], rule.what)
			}
		}
	}
}

// Both languages have to cover the same set of Annex I types. A text present
// in one language only would serve an English description with ?lang=de and
// look like a translation gap that never gets noticed.
func TestCuratedAnnexOneTextsCoverBothLanguages(t *testing.T) {
	english := map[string]bool{}
	for _, record := range annexDescriptions(t)[1:] {
		if record[0] != "annex1" {
			t.Errorf("annex1_descriptions.csv carries typology %q, want annex1", record[0])
		}
		if text := strings.TrimSpace(record[2]); text == "" {
			t.Errorf("annex1 %s has an empty English description", record[1])
		}
		english[record[1]] = true
	}

	german := map[string]bool{}
	for _, record := range descriptionOverlay(t)[1:] {
		if !strings.HasPrefix(record[1], "annex1:") {
			continue
		}
		code := strings.TrimPrefix(record[1], "annex1:")
		if text := strings.TrimSpace(record[4]); text == "" {
			t.Errorf("annex1 %s has an empty German description", code)
		}
		german[code] = true
	}

	for code := range english {
		if !german[code] {
			t.Errorf("annex1 %s has an English description but no German one", code)
		}
	}
	for code := range german {
		if !english[code] {
			t.Errorf("annex1 %s has a German description but no English one", code)
		}
	}
	if len(english) == 0 {
		t.Fatal("no Annex I descriptions shipped at all")
	}
}

// The overlay carries what situs wrote; its provenance vocabulary is not the
// same as the description table's, and a typo here would be served as an
// out-of-enum value.
func TestCuratedOverlayRowsAreWellFormed(t *testing.T) {
	for _, record := range descriptionOverlay(t)[1:] {
		if record[0] != "habitat_type" {
			t.Errorf("%s: entity_type %q, want habitat_type", record[1], record[0])
		}
		if record[2] != "de" {
			t.Errorf("%s: lang %q, want de", record[1], record[2])
		}
		if record[3] != "description" {
			t.Errorf("%s: field %q, want description", record[1], record[3])
		}
		switch record[6] {
		case "official", "curated", "derived", "situs":
		default:
			t.Errorf("%s: provenance %q is outside the vocabulary", record[1], record[6])
		}
	}
}

func syntaxonLabels(t *testing.T) [][]string {
	t.Helper()
	content, err := os.ReadFile("../../data/localizations-de-syntaxa.csv")
	if err != nil {
		t.Fatalf("reading data/localizations-de-syntaxa.csv: %v", err)
	}
	return parseCurated(t, "data/localizations-de-syntaxa.csv", content, 7)
}

func formationLetters(t *testing.T) []string {
	t.Helper()
	content, err := os.ReadFile("../../data/syntaxa_formations.csv")
	if err != nil {
		t.Fatalf("reading data/syntaxa_formations.csv: %v", err)
	}
	records := parseCurated(t, "data/syntaxa_formations.csv", content, 3)
	out := make([]string, 0, len(records)-1)
	for _, record := range records[1:] {
		out = append(out, record[0])
	}
	return out
}

// The two curated syntaxa files have to agree. A formation without a German
// name would be served in English under ?lang=de and look like a hierarchy
// gap, and a label for a letter no formation carries is a row the index
// silently never reads.
func TestCuratedSyntaxonLabelsCoverEveryFormation(t *testing.T) {
	named := map[string]int{}
	for _, record := range syntaxonLabels(t)[1:] {
		if record[3] == "name" {
			named[record[1]]++
		}
	}

	letters := formationLetters(t)
	if len(letters) == 0 {
		t.Fatal("no formations shipped at all")
	}
	for _, letter := range letters {
		switch named[letter] {
		case 1:
		case 0:
			t.Errorf("formation %s has no German name", letter)
		default:
			// Two names for one formation would make which one is served
			// depend on the source column, which nothing here varies.
			t.Errorf("formation %s carries %d German names, want exactly 1", letter, named[letter])
		}
	}
	for letter := range named {
		if !slices.Contains(letters, letter) {
			t.Errorf("a German name is shipped for %q, which is not a formation", letter)
		}
	}
}

// A vernacular without a name would be an established German term hanging off
// nothing: preferredLabel only ever pairs it with the winning name, so it
// would never be served and the translation would be silently lost.
func TestCuratedSyntaxonVernacularsHangOffAName(t *testing.T) {
	names, vernaculars := map[string]bool{}, []string{}
	for _, record := range syntaxonLabels(t)[1:] {
		switch record[3] {
		case "name":
			names[record[1]] = true
		case "vernacular":
			vernaculars = append(vernaculars, record[1])
		default:
			t.Errorf("%s: field %q, want name or vernacular", record[1], record[3])
		}
	}
	for _, key := range vernaculars {
		if !names[key] {
			t.Errorf("%s has a vernacular but no name", key)
		}
	}
}

// The rows are what the localization table keys on; a typo in any of these
// four columns writes a row no read path ever finds.
func TestCuratedSyntaxonLabelRowsAreWellFormed(t *testing.T) {
	for _, record := range syntaxonLabels(t)[1:] {
		if record[0] != "syntaxon" {
			t.Errorf("%s: entity_type %q, want syntaxon", record[1], record[0])
		}
		if record[2] != "de" {
			t.Errorf("%s: lang %q, want de", record[1], record[2])
		}
		if strings.TrimSpace(record[4]) == "" {
			t.Errorf("%s: empty %s", record[1], record[3])
		}
		// situs wrote these and nothing outside situs vouches for them. Any
		// stronger claim here would outrank an official label that arrives later.
		if record[5] != "situs" || record[6] != "situs" {
			t.Errorf("%s: source/provenance = %q/%q, want situs/situs", record[1], record[5], record[6])
		}
	}
}
