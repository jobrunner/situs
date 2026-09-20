package main

import (
	"bytes"
	"encoding/csv"
	"os"
	"strings"
	"testing"
)

// The curated CSVs in data/ are what makes an ingest reproducible: they were
// written with AI assistance and cannot be regenerated, so they are versioned
// rather than produced. These tests are the only thing standing between a
// careless edit and a shipped index, because no pipeline validates them.

// The two files are read through their own functions rather than one taking a
// path: a literal path is what keeps this out of gosec's file-inclusion rule,
// and there are exactly two of them.
func annexDescriptions(t *testing.T) [][]string {
	t.Helper()
	content, err := os.ReadFile("../../data/annex1_descriptions.csv")
	if err != nil {
		t.Fatalf("reading data/annex1_descriptions.csv: %v", err)
	}
	return parseCurated(t, "data/annex1_descriptions.csv", content)
}

func descriptionOverlay(t *testing.T) [][]string {
	t.Helper()
	content, err := os.ReadFile("../../data/localizations_descriptions.csv")
	if err != nil {
		t.Fatalf("reading data/localizations_descriptions.csv: %v", err)
	}
	return parseCurated(t, "data/localizations_descriptions.csv", content)
}

func parseCurated(t *testing.T, name string, content []byte) [][]string {
	t.Helper()
	records, err := csv.NewReader(bytes.NewReader(content)).ReadAll()
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	if len(records) < 2 {
		t.Fatalf("%s carries no rows", name)
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
		{"*", "markup"},
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
