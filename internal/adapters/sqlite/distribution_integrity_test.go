package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// seedCryptogamHierarchy builds two independent four-step chains: formation C
// (vascular plants, as elsewhere in this package) and formation R, the first
// of the cryptogam sections R-Y. Only CA01A gets a distribution row and a
// coverage row; RA01A gets neither, matching what the real EVC distribution
// source does — it only ever speaks about vascular-plant alliances.
func seedCryptogamHierarchy(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, s := range []domain.Syntaxon{
		{ID: "C", Rank: domain.SyntaxonRankFormation, Name: "Formation C",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA", Rank: domain.SyntaxonRankClass, Name: "Class CA", ParentID: "C",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01", Rank: domain.SyntaxonRankOrder, Name: "Order CA01", ParentID: "CA",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01A", Rank: domain.SyntaxonRankAlliance, Name: "Alliance CA01A", ParentID: "CA01",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "R", Rank: domain.SyntaxonRankFormation, Name: "Formation R",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "RA", Rank: domain.SyntaxonRankClass, Name: "Class RA", ParentID: "R",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "RA01", Rank: domain.SyntaxonRankOrder, Name: "Order RA01", ParentID: "RA",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "RA01A", Rank: domain.SyntaxonRankAlliance, Name: "Alliance RA01A", ParentID: "RA01",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
	} {
		if err := tx.UpsertSyntaxon(s); err != nil {
			t.Fatalf("UpsertSyntaxon(%s): %v", s.ID, err)
		}
	}
	if err := tx.UpsertArea(domain.NamedArea{
		Area:   domain.Area{Scheme: domain.SchemeEVCTerritory, Code: "austria-alps"},
		NameEN: "Austria (Alps)",
	}); err != nil {
		t.Fatalf("UpsertArea: %v", err)
	}
	if err := tx.UpsertSyntaxonDistribution("CA01A", domain.SchemeEVCTerritory, "austria-alps", domain.OccurrenceVerified); err != nil {
		t.Fatalf("UpsertSyntaxonDistribution: %v", err)
	}
	if err := tx.UpsertSyntaxonDistributionCoverage("CA01A", domain.SchemeEVCTerritory); err != nil {
		t.Fatalf("UpsertSyntaxonDistributionCoverage: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// The spec's central promise as a test: no bryophyte, lichen or algal
// alliance carries a distribution statement, and none of them appears as
// "does not occur" either. The second half is the one that is easy to lose —
// a filter that drops the unjudgeable rows would satisfy the first half and
// break the promise.
func TestKeinKryptogamenverbandTraegtEineVerbreitungsaussage(t *testing.T) {
	db := openTestDB(t)
	seedCryptogamHierarchy(t, db)

	rows, err := db.QueryContext(context.Background(),
		`SELECT a.id FROM syntaxon a
		 JOIN syntaxon o ON o.id = a.parent_id
		 JOIN syntaxon c ON c.id = o.parent_id
		 JOIN syntaxon_distribution_coverage v ON v.syntaxon_id = a.id
		 WHERE a.rank = 'alliance' AND c.parent_id IN ('R','S','T','U','V','W','X','Y')`)
	if err != nil {
		t.Fatalf("Abfrage: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var wrong []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		wrong = append(wrong, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if len(wrong) != 0 {
		t.Errorf("Kryptogamen-Verbaende mit Verbreitungsaussage: %v", wrong)
	}
}

func TestKryptogamenverbandErscheintNichtAlsNichtvorkommen(t *testing.T) {
	db := openTestDB(t)
	seedCryptogamHierarchy(t, db)

	got, err := db.SyntaxonDistribution(context.Background(), "RA01A", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if got.Covered {
		t.Fatal("RA01A ist covered, obwohl die Quelle nichts ueber Moosverbaende sagt")
	}

	// And it stays in an ?area=-filtered list, unmarked: dropping it would be
	// the same false claim in the other direction.
	occurrences, err := db.SyntaxonOccurrencesInArea(context.Background(),
		domain.SchemeEVCTerritory, "austria-alps")
	if err != nil {
		t.Fatalf("SyntaxonOccurrencesInArea: %v", err)
	}
	if _, ok := occurrences["RA01A"]; ok {
		t.Error("RA01A traegt eine Vorkommenszeile")
	}
	covered, err := db.SyntaxaWithCoverage(context.Background(), domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxaWithCoverage: %v", err)
	}
	if covered["RA01A"] {
		t.Error("RA01A steht in der Coverage-Menge")
	}
}

func TestJedeVerbreitungszeileZeigtAufEinVorhandenesSyntaxon(t *testing.T) {
	// No foreign keys by design, so this is the check that replaces them.
	db := openTestDB(t)
	seedCryptogamHierarchy(t, db)

	for _, query := range []string{
		`SELECT d.syntaxon_id FROM syntaxon_distribution d
		 LEFT JOIN syntaxon s ON s.id = d.syntaxon_id WHERE s.id IS NULL`,
		`SELECT v.syntaxon_id FROM syntaxon_distribution_coverage v
		 LEFT JOIN syntaxon s ON s.id = v.syntaxon_id WHERE s.id IS NULL`,
	} {
		func() {
			rows, err := db.QueryContext(context.Background(), query)
			if err != nil {
				t.Fatalf("Abfrage: %v", err)
			}
			defer func() { _ = rows.Close() }()
			var dangling []string
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					t.Fatalf("Scan: %v", err)
				}
				dangling = append(dangling, id)
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("rows.Err: %v", err)
			}
			if len(dangling) != 0 {
				t.Errorf("baumelnde Verweise: %v", dangling)
			}
		}()
	}
}
