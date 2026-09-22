// distribution_integrity_test.go checks the spec's promise that a bryophyte/
// lichen/algal alliance never carries a distribution or coverage statement
// against a REAL IngestSyntaxa + IngestSyntaxonDistribution run, not a
// hand-seeded fixture: a fixture built with UpsertSyntaxon /
// UpsertSyntaxonDistribution directly is correct by construction and would
// stay green even if the distribution ingest tagged the wrong alliance.
package application

import (
	"context"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// cryptogamCoverage runs IngestSyntaxa over minimalHierarchy — which already
// gives formation C (phanerogam, per minimalFormations) an alliance CA01A and
// formation R (bryophyte_lichen) an alliance RA01A — then
// IngestSyntaxonDistribution over the given distribution/coverage CSVs, and
// reports which of the cryptogam alliance's rows the source actually wrote.
func cryptogamCoverage(t *testing.T, dist, coverage string) (occurrenceCount int, covered bool) {
	t.Helper()
	repo := openIntegrityDB(t)
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	if _, err := IngestSyntaxa(context.Background(), repo, dir); err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	distPath, covPath := writeDistFiles(t, dist, coverage)
	if _, err := IngestSyntaxonDistribution(context.Background(), repo, distPath, covPath); err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}

	occ, err := repo.SyntaxonOccurrencesInArea(context.Background(), domain.SchemeEVCTerritory, "austria-alps")
	if err != nil {
		t.Fatalf("SyntaxonOccurrencesInArea: %v", err)
	}
	_, hasOccurrence := occ["RA01A"]
	coverageSet, err := repo.SyntaxaWithCoverage(context.Background(), domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxaWithCoverage: %v", err)
	}
	count := 0
	if hasOccurrence {
		count = 1
	}
	return count, coverageSet["RA01A"]
}

// The clean case: the real EVC distribution source only ever speaks about
// vascular-plant alliances (CA01A here), never about a moss alliance
// (RA01A). This must survive an actual ingest, not just a fixture built to
// already look that way.
func TestVerbreitungIngestLaesstMoosverbandOhneAussage(t *testing.T) {
	occCount, covered := cryptogamCoverage(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CA01A,evc_territory,austria-alps,verified\n",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\n")
	if occCount != 0 {
		t.Error("RA01A traegt eine Vorkommenszeile, obwohl die Quelle nur CA01A nennt")
	}
	if covered {
		t.Error("RA01A steht in der Coverage-Menge, obwohl die Quelle nur CA01A nennt")
	}
}

// The violating variant: proves the check above actually bites. If the
// source DOES name the moss alliance, both signals must flip — otherwise the
// clean test above would pass no matter what the ingest does, which is
// exactly the vacuity the fixture-based test had.
func TestVerbreitungIngestErkenntMoosverbandMitAussage(t *testing.T) {
	occCount, covered := cryptogamCoverage(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"RA01A,evc_territory,austria-alps,verified\n",
		"syntaxon_id,area_scheme\nRA01A,evc_territory\n")
	if occCount != 1 {
		t.Error("RA01A traegt keine Vorkommenszeile, obwohl die Quelle sie diesmal nennt")
	}
	if !covered {
		t.Error("RA01A steht nicht in der Coverage-Menge, obwohl die Quelle sie diesmal nennt")
	}
}
