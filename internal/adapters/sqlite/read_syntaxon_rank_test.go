package sqlite

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func TestSyntaxaByRankOhneGruppenfilter(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankClass, "")
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if !slices.Equal(syntaxonIDs(got), []string{"CA", "RA"}) {
		t.Errorf("Klassen = %v, erwartet [CA RA]", syntaxonIDs(got))
	}
}

func TestSyntaxaByRankFiltertVerbaendeUeberDreiEbenenNachOben(t *testing.T) {
	db := openHierarchyDB(t)

	// Per subproject A the filter value sits on the formation row ONLY. An
	// alliance is three levels away — which is what this test exercises.
	got, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankAlliance, domain.LifeFormBryophyteLichen)
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if !slices.Equal(syntaxonIDs(got), []string{"RA01A"}) {
		t.Errorf("Moosverbaende = %v, erwartet [RA01A]", syntaxonIDs(got))
	}

	phanerogam, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankAlliance, domain.LifeFormPhanerogam)
	if err != nil {
		t.Fatalf("SyntaxaByRank(phanerogam): %v", err)
	}
	if !slices.Equal(syntaxonIDs(phanerogam), []string{"CA01A", "CA01B"}) {
		t.Errorf("phanerogame Verbaende = %v, erwartet [CA01A CA01B]", syntaxonIDs(phanerogam))
	}
}

func TestSyntaxaByRankFiltertAuchAufDerFormationsebene(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankFormation, domain.LifeFormBryophyteLichen)
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if !slices.Equal(syntaxonIDs(got), []string{"R"}) {
		t.Errorf("Formationen = %v, erwartet [R]", syntaxonIDs(got))
	}
}

func TestSyntaxaByRankUnbekannterRangIstEineLeereListe(t *testing.T) {
	db := openHierarchyDB(t)

	// Validating the value is the read side's job, not the repository's: here
	// "there are none" is the honest answer; the use case turns it into
	// INVALID_QUERY (task 3).
	got, err := db.SyntaxaByRank(t.Context(), "association", "")
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Ergebnis = %v, erwartet eine leere, nicht-nil Liste", got)
	}
}

func TestSyntaxaByRankLaeuftBeiEinemZykelNichtEndlos(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, s := range []domain.Syntaxon{
		{ID: "ZYK-A", Rank: domain.SyntaxonRankAlliance, Name: "A", ParentID: "ZYK-B",
			Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived},
		{ID: "ZYK-B", Rank: domain.SyntaxonRankOrder, Name: "B", ParentID: "ZYK-A",
			Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived},
	} {
		if err := tx.UpsertSyntaxon(s); err != nil {
			t.Fatalf("UpsertSyntaxon(%s): %v", s.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Without the step bound in the CTE this query does not fail — it never
	// returns at all. It has to terminate and yield an empty list: neither row
	// reaches a formation.
	got, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankAlliance, domain.LifeFormPhanerogam)
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Ergebnis = %v, erwartet leer — keine Zeile erreicht eine Formation", syntaxonIDs(got))
	}
}

// TestSyntaxaByRankStepBoundMatchesMaxSyntaxonAncestors guards the seam a
// comment alone cannot: the recursive CTE's step bound is a literal, not a
// bound parameter (mandatory per the task brief — building it from Go input
// is exactly the SQL assembly gosec G201 forbids), so nothing at compile time
// ties it to maxSyntaxonAncestors. Without this test, raising
// maxSyntaxonAncestors for a fifth rank would silently leave the CTE cutting
// off one step early — SyntaxaByRank would then report the new deepest rank
// as always empty under any group filter, and every other test here (built
// on a three-step hierarchy) would stay green.
func TestSyntaxaByRankStepBoundMatchesMaxSyntaxonAncestors(t *testing.T) {
	want := fmt.Sprintf("u.steps < %d", maxSyntaxonAncestors)
	if !strings.Contains(syntaxaByRankRowsWithGroupSQL, want) {
		t.Errorf("syntaxaByRankRowsWithGroupSQL does not contain %q — the CTE step bound has drifted from maxSyntaxonAncestors (%d)",
			want, maxSyntaxonAncestors)
	}
}

func TestSyntaxonRanksLiefertDieVorhandenenRaengeSortiert(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxonRanks(t.Context())
	if err != nil {
		t.Fatalf("SyntaxonRanks: %v", err)
	}
	if !slices.Equal(got, []string{"alliance", "class", "formation", "order"}) {
		t.Errorf("Raenge = %v, erwartet [alliance class formation order]", got)
	}
}

func TestSyntaxonRanksEinesLeerenIndexIstLeerUndNichtNil(t *testing.T) {
	db := openTestDB(t)

	got, err := db.SyntaxonRanks(t.Context())
	if err != nil {
		t.Fatalf("SyntaxonRanks: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Raenge = %v, erwartet eine leere, nicht-nil Liste", got)
	}
}
