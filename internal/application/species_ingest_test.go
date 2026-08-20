package application

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// strPtr is the shared helper for tests across this package that need a
// domain.SpeciesRole.ConceptID (a *string).
func strPtr(s string) *string { return &s }

type fakeResolver map[string]string

func (r fakeResolver) Resolve(_ context.Context, names []string) (map[string]string, error) {
	out := make(map[string]string, len(names))
	for _, n := range names {
		if id, ok := r[n]; ok {
			out[n] = id
		}
	}
	return out, nil
}

// erroringResolver pins the brief's second client test at the application
// layer: a resolver failure must abort the ingest, not be recorded as "every
// name unresolvable".
type erroringResolver struct{}

func (erroringResolver) Resolve(context.Context, []string) (map[string]string, error) {
	return nil, fmt.Errorf("hostus unavailable")
}

func seedSpeciesRolesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Inula hirta,diagnostic,0.8,\n"+
			"eunis@2021,R22,Nonexistent name,constant,,0.5\n")
	return dir
}

func TestIngestSpeciesRoles_KeepsUnresolvableNames(t *testing.T) {
	dir := seedSpeciesRolesDir(t)
	path := filepath.Join(dir, "species_roles.csv")

	repo := newFakeRepo()
	resolver := fakeResolver{"Inula hirta": "wcvp:concept:1"}
	rep, err := IngestSpeciesRoles(context.Background(), repo, resolver, path)
	if err != nil {
		t.Fatalf("IngestSpeciesRoles: %v", err)
	}
	if rep.Rows != 2 || rep.Resolved != 1 || rep.Unresolved != 1 {
		t.Errorf("report = %+v, want Rows 2 / Resolved 1 / Unresolved 1", rep)
	}
	if len(repo.speciesRoles) != 2 {
		t.Fatalf("stored %d roles, want 2 (the unresolvable one is kept)", len(repo.speciesRoles))
	}
	var sawResolved bool
	for _, r := range repo.speciesRoles {
		if r.VerbatimName == "Inula hirta" {
			sawResolved = true
			if r.ConceptID == nil || *r.ConceptID != "wcvp:concept:1" {
				t.Errorf("Inula hirta ConceptID = %v, want a pointer to wcvp:concept:1", r.ConceptID)
			}
		}
		if r.VerbatimName == "Nonexistent name" && r.ConceptID != nil {
			t.Error("unresolvable name stored with a concept id")
		}
		if r.VerbatimName == "" {
			t.Error("verbatim name must always be stored")
		}
	}
	if !sawResolved {
		t.Fatal("resolved row (Inula hirta) not found among stored roles")
	}
	if got := rep.ResolutionRate(); got != 0.5 {
		t.Errorf("ResolutionRate() = %v, want 0.5", got)
	}
}

func TestSpeciesReport_ResolutionRateOfNoRowsIsZero(t *testing.T) {
	if got := (SpeciesReport{}).ResolutionRate(); got != 0 {
		t.Errorf("ResolutionRate() of an empty report = %v, want 0", got)
	}
}

// A malformed row is skipped and counted, exactly like every other ingest
// file — species roles are not a special case.
func TestIngestSpeciesRoles_SkipsMalformedRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "species_roles.csv")
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Inula hirta,diagnostic,0.8,\n"+
			"not valid @@ id,R22,Bad typology,diagnostic,0.8,\n"+
			"eunis@2021,R22,Bad fidelity,diagnostic,not-a-number,\n"+
			"eunis@2021,R22,Bad constancy,constant,,not-a-number\n")

	repo := newFakeRepo()
	rep, err := IngestSpeciesRoles(context.Background(), repo, fakeResolver{}, path)
	if err != nil {
		t.Fatalf("IngestSpeciesRoles: %v", err)
	}
	if rep.Rows != 1 {
		t.Errorf("Rows = %d, want 1 (three malformed rows skipped)", rep.Rows)
	}
	if rep.Skipped != 3 {
		t.Errorf("Skipped = %d, want 3 — SpeciesReport must report skips the same way IngestReport does", rep.Skipped)
	}
}

// A resolver error must abort the ingest; it must never be silently recorded
// as "every name unresolvable".
func TestIngestSpeciesRoles_MissingFileFails(t *testing.T) {
	repo := newFakeRepo()
	path := filepath.Join(t.TempDir(), "species_roles.csv")
	if _, err := IngestSpeciesRoles(context.Background(), repo, fakeResolver{}, path); err == nil {
		t.Fatal("IngestSpeciesRoles with a missing source file = nil error, want an error")
	}
}

func TestIngestSpeciesRoles_ResolverErrorAbortsTheIngest(t *testing.T) {
	repo := newFakeRepo()
	if _, err := IngestSpeciesRoles(context.Background(), repo, erroringResolver{}, filepath.Join(seedSpeciesRolesDir(t), "species_roles.csv")); err == nil {
		t.Fatal("IngestSpeciesRoles with a resolver error = nil error, want an error")
	}
	if repo.committed {
		t.Error("ingest committed despite a resolver error")
	}
}

func TestIngestSpeciesRoles_BeginErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.beginErr = fmt.Errorf("boom")

	path := filepath.Join(seedSpeciesRolesDir(t), "species_roles.csv")
	if _, err := IngestSpeciesRoles(context.Background(), repo, fakeResolver{}, path); err == nil {
		t.Fatal("IngestSpeciesRoles with a Begin error = nil error, want an error")
	}
}

func TestIngestSpeciesRoles_RepositoryErrorRollsBackAndReturnsTheError(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertSpeciesRole"

	path := filepath.Join(seedSpeciesRolesDir(t), "species_roles.csv")
	if _, err := IngestSpeciesRoles(context.Background(), repo, fakeResolver{}, path); err == nil {
		t.Fatal("IngestSpeciesRoles with a repository error = nil error, want an error")
	}
	if repo.committed {
		t.Error("ingest committed despite a repository error")
	}
	if !repo.rolledBack {
		t.Error("ingest did not roll back after a repository error")
	}
}

func TestIngestSpeciesRoles_RollbackErrorIsWrappedWithTheOriginal(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertSpeciesRole"
	repo.rollbackErr = fmt.Errorf("connection lost")

	path := filepath.Join(seedSpeciesRolesDir(t), "species_roles.csv")
	_, err := IngestSpeciesRoles(context.Background(), repo, fakeResolver{}, path)
	if err == nil {
		t.Fatal("IngestSpeciesRoles = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "connection lost") {
		t.Errorf("error = %q, want it to mention the rollback failure too", err)
	}
}

func TestIngestSpeciesRoles_CommitErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.commitErr = fmt.Errorf("disk full")

	path := filepath.Join(seedSpeciesRolesDir(t), "species_roles.csv")
	if _, err := IngestSpeciesRoles(context.Background(), repo, fakeResolver{}, path); err == nil {
		t.Fatal("IngestSpeciesRoles with a Commit error = nil error, want an error")
	}
}
