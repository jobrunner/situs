// distribution_integrity_test.go checks the spec's promise that a bryophyte/
// lichen/algal alliance never carries a distribution or coverage statement
// against a REAL IngestSyntaxa + IngestSyntaxonDistribution run, not a
// hand-seeded fixture: a fixture built with UpsertSyntaxon /
// UpsertSyntaxonDistribution directly is correct by construction and would
// stay green even if the distribution ingest tagged the wrong alliance.
package application

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jobrunner/situs/internal/adapters/sqlite"
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

// --- Repeat ingest: the distribution tables are REPLACED, not merged. ---

// seededDistributionIndex is an index the way a previous run left it: the
// syntaxa hierarchy from minimalHierarchy (alliances CA01A and RA01A) plus an
// older distribution fassung — CA01A in two territories, RA01A covered without
// a single occurrence, which is the "checked, occurs nowhere" state.
func seededDistributionIndex(t *testing.T) *sqlite.DB {
	t.Helper()
	repo := openIntegrityDB(t)
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	if _, err := IngestSyntaxa(context.Background(), repo, dir); err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	distPath, covPath := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CA01A,evc_territory,austria-alps,verified\n"+
			"CA01A,evc_territory,czech-republic,verified\n",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\nRA01A,evc_territory\n")
	if _, err := IngestSyntaxonDistribution(context.Background(), repo, distPath, covPath); err != nil {
		t.Fatalf("IngestSyntaxonDistribution (Altstand): %v", err)
	}
	return repo
}

// A territory the newly pinned artifact no longer names must not survive the
// run: the stale row would claim an occurrence the current source does not
// carry.
func TestVerbreitungIngestErsetztVerschwundeneVorkommenszeile(t *testing.T) {
	repo := seededDistributionIndex(t)
	distPath, covPath := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CA01A,evc_territory,austria-alps,verified\n",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\nRA01A,evc_territory\n")
	if _, err := IngestSyntaxonDistribution(context.Background(), repo, distPath, covPath); err != nil {
		t.Fatalf("IngestSyntaxonDistribution (neuer Stand): %v", err)
	}

	got, err := repo.SyntaxonDistribution(context.Background(), "CA01A", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if len(got.Verified) != 1 || got.Verified[0] != "austria-alps" {
		t.Errorf("Verified = %v, erwartet [austria-alps] — czech-republic steht nicht mehr in der Quelle", got.Verified)
	}
}

// The case the coverage table exists for: RA01A was `absence` (coverage row,
// no occurrence row) and the new source does not mention it at all. It must
// come back `unknown` — a surviving coverage row would turn "nobody looked"
// into "checked, occurs nowhere", the exact inversion of the promise.
func TestVerbreitungIngestMachtAusVerschwundenerCoverageWiederUnbekannt(t *testing.T) {
	repo := seededDistributionIndex(t)
	distPath, covPath := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CA01A,evc_territory,austria-alps,verified\n",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\n")
	if _, err := IngestSyntaxonDistribution(context.Background(), repo, distPath, covPath); err != nil {
		t.Fatalf("IngestSyntaxonDistribution (neuer Stand): %v", err)
	}

	got, err := repo.SyntaxonDistribution(context.Background(), "RA01A", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if got.Covered {
		t.Error("RA01A ist weiterhin covered — aus `absence` muss wieder `unknown` werden, wenn die Quelle es nicht mehr nennt")
	}
	covered, err := repo.SyntaxaWithCoverage(context.Background(), domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxaWithCoverage: %v", err)
	}
	if covered["RA01A"] {
		t.Error("RA01A steht noch in der Coverage-Menge")
	}
	if !covered["CA01A"] {
		t.Error("CA01A fehlt in der Coverage-Menge, obwohl die neue Quelle es nennt")
	}
}

// No distribution file pinned yet is "no statement this run", not "delete
// everything": the ingest skips before it ever opens a transaction, so the
// rows an earlier run wrote stay untouched.
func TestVerbreitungIngestOhneDateiLaesstAltstandStehen(t *testing.T) {
	repo := seededDistributionIndex(t)
	dir := t.TempDir()
	if _, err := IngestSyntaxonDistribution(context.Background(), repo,
		filepath.Join(dir, "syntaxon_distribution.csv"),
		filepath.Join(dir, "syntaxon_distribution_coverage.csv")); err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}

	got, err := repo.SyntaxonDistribution(context.Background(), "CA01A", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if len(got.Verified) != 2 {
		t.Errorf("Verified = %v, erwartet die zwei Zeilen des Altstands", got.Verified)
	}
	covered, err := repo.SyntaxaWithCoverage(context.Background(), domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxaWithCoverage: %v", err)
	}
	if !covered["RA01A"] {
		t.Error("RA01A verlor seine Coverage-Zeile, obwohl gar keine Verbreitungsdatei gepinnt ist")
	}
}

// Atomic: the clear and every write live in one transaction, so a run that
// fails after the clear (here: a coverage file missing its area_scheme column,
// read after the occurrence rows are written) leaves the index exactly as it
// was.
func TestVerbreitungIngestBleibtAtomarBeiFehlerNachDemLeeren(t *testing.T) {
	repo := seededDistributionIndex(t)
	distPath, covPath := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CA01A,evc_territory,austria-alps,verified\n",
		"syntaxon_id\nCA01A\n")
	if _, err := IngestSyntaxonDistribution(context.Background(), repo, distPath, covPath); err == nil {
		t.Fatal("IngestSyntaxonDistribution lief trotz fehlender Spalte durch")
	}

	got, err := repo.SyntaxonDistribution(context.Background(), "CA01A", domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if len(got.Verified) != 2 {
		t.Errorf("Verified = %v, erwartet die zwei Zeilen des Altstands — der fehlgeschlagene Lauf haette nichts loeschen duerfen", got.Verified)
	}
	covered, err := repo.SyntaxaWithCoverage(context.Background(), domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("SyntaxaWithCoverage: %v", err)
	}
	if !covered["RA01A"] {
		t.Error("RA01A verlor seine Coverage-Zeile in einem Lauf, der fehlgeschlagen ist")
	}
}
