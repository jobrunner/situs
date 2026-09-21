package application

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
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
