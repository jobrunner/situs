package application

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// These three tests together pin the fourth state: Distribution is nil when
// no coverage row exists, a present object with two empty lists when the
// source checked and found nothing, and a present object with the codes
// otherwise. Seeding goes straight into fakeRepo's own slices (the same
// storage UpsertSyntaxonDistribution/UpsertSyntaxonDistributionCoverage
// write to), not through a separate map: that keeps this test honest about
// what the repository actually persists.

func TestSyntaxonDetailTraegtVerbreitungBeiAbdeckung(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	repo.syntaxonCoverage = append(repo.syntaxonCoverage,
		fakeSyntaxonCoverage{SyntaxonID: "CA01A", Scheme: domain.SchemeEVCTerritory})
	repo.syntaxonDistribution = append(repo.syntaxonDistribution,
		fakeSyntaxonOccurrence{SyntaxonID: "CA01A", Scheme: domain.SchemeEVCTerritory, Code: "albania", Occurrence: domain.OccurrenceVerified},
		fakeSyntaxonOccurrence{SyntaxonID: "CA01A", Scheme: domain.SchemeEVCTerritory, Code: "austria-alps", Occurrence: domain.OccurrenceVerified},
		fakeSyntaxonOccurrence{SyntaxonID: "CA01A", Scheme: domain.SchemeEVCTerritory, Code: "czech-republic", Occurrence: domain.OccurrenceUncertain},
	)
	q := NewQueryService(repo)

	got, err := q.Syntaxon(context.Background(), "CA01A", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.Distribution == nil {
		t.Fatal("Distribution ist nil, obwohl eine Coverage-Zeile vorliegt")
	}
	if got.Distribution.AreaScheme != domain.SchemeEVCTerritory {
		t.Errorf("AreaScheme = %q", got.Distribution.AreaScheme)
	}
	if !slices.Equal(got.Distribution.Verified, []string{"albania", "austria-alps"}) {
		t.Errorf("Verified = %v", got.Distribution.Verified)
	}
	if !slices.Equal(got.Distribution.Uncertain, []string{"czech-republic"}) {
		t.Errorf("Uncertain = %v", got.Distribution.Uncertain)
	}
}

func TestSyntaxonDetailLaesstVerbreitungOhneAbdeckungWeg(t *testing.T) {
	// The bryophyte case: no statement at all. Nil, never an empty list — an
	// empty list would read as "occurs nowhere", which the source never said.
	repo := newFakeRepo()
	seedSyntaxa(repo, "RA01A")
	q := NewQueryService(repo)

	got, err := q.Syntaxon(context.Background(), "RA01A", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.Distribution != nil {
		t.Errorf("Distribution = %+v, erwartet nil", got.Distribution)
	}
}

// A failed distribution read must surface as an error, not silently
// disappear into a nil Distribution — that would read as "no coverage" when
// the truth is "could not tell".
func TestSyntaxonGibtVerbreitungsfehlerWeiter(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	repo.syntaxonDistributionErr = errBoom
	q := NewQueryService(repo)

	_, err := q.Syntaxon(context.Background(), "CA01A", "en")
	if err == nil || !strings.Contains(err.Error(), "reading distribution") {
		t.Errorf("Fehler = %v, erwartet einen Hinweis auf die Verbreitungsabfrage", err)
	}
}

func TestSyntaxonDetailTraegtLeereListenBeiGeprueftemNichtvorkommen(t *testing.T) {
	// Covered, no occurrence: "checked, occurs in no territory". THIS is the
	// state that must arrive as a present object with two empty lists — the
	// only way a client can tell it from the nil case above.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01B")
	repo.syntaxonCoverage = append(repo.syntaxonCoverage,
		fakeSyntaxonCoverage{SyntaxonID: "CA01B", Scheme: domain.SchemeEVCTerritory})
	q := NewQueryService(repo)

	got, err := q.Syntaxon(context.Background(), "CA01B", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.Distribution == nil {
		t.Fatal("Distribution ist nil, obwohl die Quelle geprueft hat")
	}
	if len(got.Distribution.Verified) != 0 || len(got.Distribution.Uncertain) != 0 {
		t.Errorf("Distribution = %+v, erwartet zwei leere Listen", got.Distribution)
	}
}

// seedAreaFixture builds the world SyntaxaByRank's area filter is asked
// about: CA01A verified in austria-alps, CA01B uncertain there, CA01C covered
// but absent (a definite statement), RA01A not covered at all (no statement
// at all — the bryophyte case).
func seedAreaFixture(repo *fakeRepo) {
	seedSyntaxa(repo, "CA01A", "CA01B", "CA01C", "RA01A")
	repo.syntaxonDistribution = append(repo.syntaxonDistribution,
		fakeSyntaxonOccurrence{SyntaxonID: "CA01A", Scheme: domain.SchemeEVCTerritory,
			Code: "austria-alps", Occurrence: domain.OccurrenceVerified},
		fakeSyntaxonOccurrence{SyntaxonID: "CA01B", Scheme: domain.SchemeEVCTerritory,
			Code: "austria-alps", Occurrence: domain.OccurrenceUncertain},
	)
	for _, id := range []string{"CA01A", "CA01B", "CA01C"} {
		repo.syntaxonCoverage = append(repo.syntaxonCoverage,
			fakeSyntaxonCoverage{SyntaxonID: id, Scheme: domain.SchemeEVCTerritory})
	}
	repo.knownAreaCodes = map[string][]string{
		domain.SchemeEVCTerritory: {"austria-alps", "albania"},
	}
}

func TestSyntaxaByRankMitGebietBehaeltDieUnbeurteilbaren(t *testing.T) {
	repo := newFakeRepo()
	seedAreaFixture(repo)
	q := NewQueryService(repo)

	got, err := q.SyntaxaByRank(context.Background(), domain.SyntaxonRankAlliance, "", "",
		input.SyntaxonAreaFilter{Code: "austria-alps", Include: []string{domain.OccurrenceVerified}})
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	ids := map[string]string{}
	for _, r := range got {
		ids[r.ID] = r.Occurrence
	}
	// CA01A matches; RA01A has no statement and is CARRIED, unmarked; CA01B
	// (uncertain, not included) and CA01C (a definite absence) are dropped.
	if len(ids) != 2 {
		t.Fatalf("Ergebnis = %v, erwartet genau CA01A und RA01A", ids)
	}
	if ids["CA01A"] != domain.OccurrenceVerified {
		t.Errorf("CA01A.occurrence = %q, erwartet verified", ids["CA01A"])
	}
	if occ, ok := ids["RA01A"]; !ok || occ != "" {
		t.Errorf("RA01A = %q/%v, erwartet mitgefuehrt und unmarkiert", occ, ok)
	}
}

func TestSyntaxaByRankMitIncludeUncertain(t *testing.T) {
	repo := newFakeRepo()
	seedAreaFixture(repo)
	q := NewQueryService(repo)

	got, err := q.SyntaxaByRank(context.Background(), domain.SyntaxonRankAlliance, "", "",
		input.SyntaxonAreaFilter{Code: "austria-alps", Include: []string{domain.OccurrenceUncertain}})
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	for _, r := range got {
		if r.ID == "CA01A" {
			t.Error("CA01A erscheint bei include=uncertain")
		}
		if r.ID == "CA01B" && r.Occurrence != domain.OccurrenceUncertain {
			t.Errorf("CA01B.occurrence = %q", r.Occurrence)
		}
	}
}

func TestSyntaxaByRankMitBeidenAuspraegungen(t *testing.T) {
	repo := newFakeRepo()
	seedAreaFixture(repo)
	q := NewQueryService(repo)

	got, err := q.SyntaxaByRank(context.Background(), domain.SyntaxonRankAlliance, "", "",
		input.SyntaxonAreaFilter{Code: "austria-alps",
			Include: []string{domain.OccurrenceVerified, domain.OccurrenceUncertain}})
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("Ergebnis = %d Zeilen, erwartet 3 (CA01A, CA01B, RA01A)", len(got))
	}
}

func TestSyntaxaByRankOhneFilterMarkiertNichts(t *testing.T) {
	repo := newFakeRepo()
	seedAreaFixture(repo)
	q := NewQueryService(repo)

	got, err := q.SyntaxaByRank(context.Background(), domain.SyntaxonRankAlliance, "", "",
		input.SyntaxonAreaFilter{})
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("Ergebnis = %d Zeilen, erwartet alle 4", len(got))
	}
	for _, r := range got {
		if r.Occurrence != "" {
			t.Errorf("%s traegt occurrence %q ohne aktiven Filter", r.ID, r.Occurrence)
		}
	}
}

func TestSyntaxaByRankMitUnbekanntemGebietscode(t *testing.T) {
	repo := newFakeRepo()
	seedAreaFixture(repo)
	q := NewQueryService(repo)

	_, err := q.SyntaxaByRank(context.Background(), domain.SyntaxonRankAlliance, "", "",
		input.SyntaxonAreaFilter{Code: "gibtsnicht", Include: []string{domain.OccurrenceVerified}})
	if !errors.Is(err, input.ErrUnknownArea) {
		t.Errorf("Fehler = %v, erwartet ErrUnknownArea", err)
	}
}

// The three reads syntaxonAreaLookup makes each have to surface their own
// failure — a swallowed one would silently answer as if the area had no
// data at all, which is a different (and false) statement.
func TestSyntaxaByRankMeldetFehlerDerGebietsabfragen(t *testing.T) {
	activeFilter := input.SyntaxonAreaFilter{Code: "austria-alps", Include: []string{domain.OccurrenceVerified}}

	knownAreaCodesBroken := newFakeRepo()
	seedAreaFixture(knownAreaCodesBroken)
	knownAreaCodesBroken.areasErr = errBoom
	if _, err := NewQueryService(knownAreaCodesBroken).
		SyntaxaByRank(context.Background(), domain.SyntaxonRankAlliance, "", "", activeFilter); err == nil {
		t.Error("SyntaxaByRank verschluckt den Fehler von KnownAreaCodes")
	}

	occurrencesBroken := newFakeRepo()
	seedAreaFixture(occurrencesBroken)
	occurrencesBroken.syntaxonOccurrencesInAreaErr = errBoom
	if _, err := NewQueryService(occurrencesBroken).
		SyntaxaByRank(context.Background(), domain.SyntaxonRankAlliance, "", "", activeFilter); err == nil {
		t.Error("SyntaxaByRank verschluckt den Fehler von SyntaxonOccurrencesInArea")
	}

	coverageBroken := newFakeRepo()
	seedAreaFixture(coverageBroken)
	coverageBroken.syntaxaWithCoverageErr = errBoom
	if _, err := NewQueryService(coverageBroken).
		SyntaxaByRank(context.Background(), domain.SyntaxonRankAlliance, "", "", activeFilter); err == nil {
		t.Error("SyntaxaByRank verschluckt den Fehler von SyntaxaWithCoverage")
	}
}
