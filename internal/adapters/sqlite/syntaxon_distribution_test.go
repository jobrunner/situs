package sqlite

import (
	"context"
	"slices"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// seedDistribution fills the two syntaxon distribution tables with a small
// world: CA01A has two verified and one uncertain occurrence, CA01B is
// covered but has no occurrence row at all ("checked, occurs in no
// territory"), and CA01C has no coverage row — nothing is known about it,
// which must never read as absence.
func seedDistribution(t *testing.T, db *DB) {
	t.Helper()
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, row := range []struct{ id, code, occ string }{
		{"CA01A", "austria-alps", domain.OccurrenceVerified},
		{"CA01A", "albania", domain.OccurrenceVerified},
		{"CA01A", "czech-republic", domain.OccurrenceUncertain},
	} {
		if err := tx.UpsertSyntaxonDistribution(row.id, domain.SchemeEVCTerritory, row.code, row.occ); err != nil {
			t.Fatalf("UpsertSyntaxonDistribution: %v", err)
		}
	}
	for _, id := range []string{"CA01A", "CA01B"} {
		if err := tx.UpsertSyntaxonDistributionCoverage(id, domain.SchemeEVCTerritory); err != nil {
			t.Fatalf("UpsertSyntaxonDistributionCoverage: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestSyntaxonDistributionLiefertSortierteListen(t *testing.T) {
	db := openTestDB(t)
	seedDistribution(t, db)

	got, err := db.SyntaxonDistribution(context.Background(), "CA01A", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if !got.Covered {
		t.Error("Covered = false, erwartet true")
	}
	if !slices.Equal(got.Verified, []string{"albania", "austria-alps"}) {
		t.Errorf("Verified = %v, erwartet [albania austria-alps]", got.Verified)
	}
	if !slices.Equal(got.Uncertain, []string{"czech-republic"}) {
		t.Errorf("Uncertain = %v, erwartet [czech-republic]", got.Uncertain)
	}
	if got.Scheme != domain.SchemeEVCTerritory {
		t.Errorf("Scheme = %q", got.Scheme)
	}
}

func TestSyntaxonDistributionTrenntAbsenceVonUnknown(t *testing.T) {
	db := openTestDB(t)
	seedDistribution(t, db)

	// Covered, but not a single occurrence: "checked, occurs nowhere".
	absent, err := db.SyntaxonDistribution(context.Background(), "CA01B", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if !absent.Covered {
		t.Error("CA01B: Covered = false, erwartet true — die Coverage-Zeile IST die Aussage")
	}
	if len(absent.Verified) != 0 || len(absent.Uncertain) != 0 {
		t.Errorf("CA01B traegt Vorkommen: %+v", absent)
	}

	// No coverage row: nothing is known.
	unknown, err := db.SyntaxonDistribution(context.Background(), "CA01C", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if unknown.Covered {
		t.Error("CA01C: Covered = true, erwartet false")
	}
}

func TestSyntaxonDistributionIstSchemabezogen(t *testing.T) {
	db := openTestDB(t)
	seedDistribution(t, db)

	got, err := db.SyntaxonDistribution(context.Background(), "CA01A", domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if got.Covered || len(got.Verified) != 0 {
		t.Errorf("CA01A unter wgsrpd_l3 = %+v, erwartet leer und uncovered", got)
	}
}

func TestSyntaxonOccurrencesInArea(t *testing.T) {
	db := openTestDB(t)
	seedDistribution(t, db)

	got, err := db.SyntaxonOccurrencesInArea(context.Background(), domain.SchemeEVCTerritory, "austria-alps")
	if err != nil {
		t.Fatalf("SyntaxonOccurrencesInArea: %v", err)
	}
	if len(got) != 1 || got["CA01A"] != domain.OccurrenceVerified {
		t.Errorf("Karte = %v, erwartet genau CA01A->verified", got)
	}

	// An area with no rows is an empty map and no error: "nobody occurs here"
	// is a valid answer for a code the scheme knows.
	empty, err := db.SyntaxonOccurrencesInArea(context.Background(), domain.SchemeEVCTerritory, "armenia")
	if err != nil {
		t.Fatalf("SyntaxonOccurrencesInArea: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("armenia = %v, erwartet leer", empty)
	}
}

func TestSyntaxaWithCoverage(t *testing.T) {
	db := openTestDB(t)
	seedDistribution(t, db)

	got, err := db.SyntaxaWithCoverage(context.Background(), domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxaWithCoverage: %v", err)
	}
	if len(got) != 2 || !got["CA01A"] || !got["CA01B"] || got["CA01C"] {
		t.Errorf("Menge = %v, erwartet genau CA01A und CA01B", got)
	}
}

func TestUpsertSyntaxonDistributionIstIdempotent(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// Twice the same cell, then the same cell with the other occurrence: a
	// repeated ingest must neither fail on the primary key nor keep the stale
	// value.
	for _, occ := range []string{domain.OccurrenceVerified, domain.OccurrenceVerified, domain.OccurrenceUncertain} {
		if err := tx.UpsertSyntaxonDistribution("CA01A", domain.SchemeEVCTerritory, "albania", occ); err != nil {
			t.Fatalf("UpsertSyntaxonDistribution(%s): %v", occ, err)
		}
	}
	if err := tx.UpsertSyntaxonDistributionCoverage("CA01A", domain.SchemeEVCTerritory); err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if err := tx.UpsertSyntaxonDistributionCoverage("CA01A", domain.SchemeEVCTerritory); err != nil {
		t.Fatalf("Coverage zweimal: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.SyntaxonDistribution(context.Background(), "CA01A", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if len(got.Verified) != 0 || !slices.Equal(got.Uncertain, []string{"albania"}) {
		t.Errorf("Verbreitung = %+v, erwartet nur uncertain=[albania]", got)
	}
}

func TestUpsertSyntaxonDistributionLehntFremdeAuspraegungAb(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// The CHECK is the last line of defense: a value the pipeline should have
	// rejected must not reach the index silently.
	if err := tx.UpsertSyntaxonDistribution("CA01A", domain.SchemeEVCTerritory, "albania", "1"); err == nil {
		t.Fatal("UpsertSyntaxonDistribution akzeptierte occurrence=\"1\"")
	}
	_ = tx.Rollback()
}
