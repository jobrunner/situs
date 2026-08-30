package application

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

// strPtr is the shared helper for tests across this package that need a
// domain.SpeciesRole.ConceptID (a *string).
func strPtr(s string) *string { return &s }

func speciesRolesPaths(t *testing.T, dir string) (species, crosswalk, aggregateMembers string) {
	t.Helper()
	return filepath.Join(dir, "species_roles.csv"),
		filepath.Join(dir, "eurosl_crosswalk.csv"),
		filepath.Join(dir, "aggregate_members.csv")
}

func seedSpeciesRolesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Inula hirta,diagnostic,0.8,\n"+
			"eunis@2021,R22,Nonexistent name,constant,,0.5\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv",
		"name,concept_id\nInula hirta,wcvp:concept:1\n")
	writeCSV(t, dir, "aggregate_members.csv",
		"aggregate_concept_id,member_concept_id,member_name\n")
	return dir
}

func TestIngestSpeciesRoles_ResolvesViaTheLocalCrosswalk(t *testing.T) {
	dir := seedSpeciesRolesDir(t)
	species, crosswalk, aggregates := speciesRolesPaths(t, dir)

	repo := newFakeRepo()
	rep, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates)
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
		if r.Provenance != speciesProvenanceObserved {
			t.Errorf("%q Provenance = %q, want %q", r.VerbatimName, r.Provenance, speciesProvenanceObserved)
		}
		if r.VerbatimName == "Inula hirta" {
			sawResolved = true
			if r.ConceptID == nil || *r.ConceptID != "wcvp:concept:1" {
				t.Errorf("Inula hirta ConceptID = %v, want a pointer to wcvp:concept:1", r.ConceptID)
			}
		}
		if r.VerbatimName == "Nonexistent name" && r.ConceptID != nil {
			t.Error("unresolvable name stored with a concept id")
		}
	}
	if !sawResolved {
		t.Fatal("resolved row (Inula hirta) not found among stored roles")
	}
	if got := rep.ResolutionRate(); got != 0.5 {
		t.Errorf("ResolutionRate() = %v, want 0.5", got)
	}
}

// A row whose resolved concept id is itself an aggregate in
// aggregate_members.csv gets one derived row per member, carrying
// provenance/derived_from and never fidelity/constancy.
func TestIngestSpeciesRoles_DerivesMemberRowsFromAnAggregate(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Rubus fruticosus aggr.,diagnostic,0.7,\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv",
		"name,concept_id\nRubus fruticosus aggr.,wcvp:concept:99\n")
	writeCSV(t, dir, "aggregate_members.csv",
		"aggregate_concept_id,member_concept_id,member_name\n"+
			"wcvp:concept:99,wcvp:concept:100,Rubus caesius\n"+
			"wcvp:concept:99,wcvp:concept:101,Rubus plicatus\n")
	species, crosswalk, aggregates := speciesRolesPaths(t, dir)

	repo := newFakeRepo()
	rep, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates)
	if err != nil {
		t.Fatalf("IngestSpeciesRoles: %v", err)
	}
	if rep.DerivedRows != 2 {
		t.Errorf("DerivedRows = %d, want 2", rep.DerivedRows)
	}
	if len(repo.speciesRoles) != 3 {
		t.Fatalf("stored %d roles, want 3 (1 explicit + 2 derived)", len(repo.speciesRoles))
	}
	var sawCaesius bool
	for _, r := range repo.speciesRoles {
		if r.VerbatimName != "Rubus caesius" {
			continue
		}
		sawCaesius = true
		if r.Provenance != speciesProvenanceDerivedFromAggregate {
			t.Errorf("Provenance = %q, want %q", r.Provenance, speciesProvenanceDerivedFromAggregate)
		}
		if r.DerivedFrom == nil || *r.DerivedFrom != "wcvp:concept:99" {
			t.Errorf("DerivedFrom = %v, want wcvp:concept:99", r.DerivedFrom)
		}
		if r.Fidelity != nil || r.Constancy != nil {
			t.Errorf("derived row Fidelity/Constancy = %v/%v, want both nil", r.Fidelity, r.Constancy)
		}
	}
	if !sawCaesius {
		t.Fatal("derived row for Rubus caesius not found")
	}
}

// An explicit species_roles.csv row for a member species must survive
// untouched — the derived row for the same key must be suppressed, not
// overwrite it.
func TestIngestSpeciesRoles_ExplicitRowWinsOverDerivation(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Rubus fruticosus aggr.,diagnostic,0.7,\n"+
			"eunis@2021,R22,Rubus caesius,diagnostic,0.9,\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv",
		"name,concept_id\nRubus fruticosus aggr.,wcvp:concept:99\nRubus caesius,wcvp:concept:100\n")
	writeCSV(t, dir, "aggregate_members.csv",
		"aggregate_concept_id,member_concept_id,member_name\n"+
			"wcvp:concept:99,wcvp:concept:100,Rubus caesius\n")
	species, crosswalk, aggregates := speciesRolesPaths(t, dir)

	repo := newFakeRepo()
	rep, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates)
	if err != nil {
		t.Fatalf("IngestSpeciesRoles: %v", err)
	}
	if rep.DerivedRows != 0 || rep.SuppressedByExplicit != 1 {
		t.Errorf("report = %+v, want DerivedRows 0 / SuppressedByExplicit 1", rep)
	}
	var count int
	for _, r := range repo.speciesRoles {
		if r.VerbatimName != "Rubus caesius" {
			continue
		}
		count++
		if r.Provenance != speciesProvenanceObserved {
			t.Errorf("Provenance = %q, want %q (the explicit row must survive)", r.Provenance, speciesProvenanceObserved)
		}
		if r.Fidelity == nil || *r.Fidelity != 0.9 {
			t.Errorf("Fidelity = %v, want 0.9 (the explicit row's own value)", r.Fidelity)
		}
	}
	if count != 1 {
		t.Errorf("stored %d Rubus caesius rows, want 1 (no duplicate)", count)
	}
}

// A name with more than one distinct concept id in the crosswalk is not
// guessed: it stays unresolved and is counted separately.
func TestIngestSpeciesRoles_AmbiguousCrosswalkEntryStaysUnresolved(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Homonym species,diagnostic,,\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv",
		"name,concept_id\nHomonym species,wcvp:concept:1\nHomonym species,wcvp:concept:2\n")
	writeCSV(t, dir, "aggregate_members.csv",
		"aggregate_concept_id,member_concept_id,member_name\n")
	species, crosswalk, aggregates := speciesRolesPaths(t, dir)

	repo := newFakeRepo()
	rep, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates)
	if err != nil {
		t.Fatalf("IngestSpeciesRoles: %v", err)
	}
	if rep.AmbiguousCrosswalk != 1 || rep.Unresolved != 1 || rep.Resolved != 0 {
		t.Errorf("report = %+v, want AmbiguousCrosswalk 1 / Unresolved 1 / Resolved 0", rep)
	}
	if len(repo.speciesRoles) != 1 || repo.speciesRoles[0].ConceptID != nil {
		t.Errorf("stored roles = %+v, want one row with a nil ConceptID", repo.speciesRoles)
	}
}

// An ambiguous crosswalk entry must be logged, not just counted — an
// operator diagnosing a low resolution rate needs to see which names were
// ambiguous, not just how many.
func TestIngestSpeciesRoles_LogsAnAmbiguousCrosswalkEntry(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Homonym species,diagnostic,,\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv",
		"name,concept_id\nHomonym species,wcvp:concept:1\nHomonym species,wcvp:concept:2\n")
	writeCSV(t, dir, "aggregate_members.csv",
		"aggregate_concept_id,member_concept_id,member_name\n")
	species, crosswalk, aggregates := speciesRolesPaths(t, dir)

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	repo := newFakeRepo()
	if _, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates); err != nil {
		t.Fatalf("IngestSpeciesRoles: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "ambiguous") || !strings.Contains(got, "Homonym species") {
		t.Errorf("log = %q, want it to name the ambiguous verbatim name", got)
	}
}

// A missing aggregate_members.csv is not an error — derivation just does
// not run, and the rest of the ingest proceeds normally.
func TestIngestSpeciesRoles_MissingAggregateMembersFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Inula hirta,diagnostic,0.8,\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv",
		"name,concept_id\nInula hirta,wcvp:concept:1\n")
	species, crosswalk, aggregates := speciesRolesPaths(t, dir) // aggregate_members.csv never written

	repo := newFakeRepo()
	rep, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates)
	if err != nil {
		t.Fatalf("IngestSpeciesRoles: %v", err)
	}
	if rep.DerivedRows != 0 {
		t.Errorf("DerivedRows = %d, want 0", rep.DerivedRows)
	}
	if rep.Resolved != 1 {
		t.Errorf("Resolved = %d, want 1 — a missing aggregate file must not affect ordinary resolution", rep.Resolved)
	}
}

// A missing eurosl_crosswalk.csv is a hard error: nothing can resolve
// without it, the same severity as the old live-resolver outage.
func TestIngestSpeciesRoles_MissingCrosswalkFileIsAHardError(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Inula hirta,diagnostic,0.8,\n")
	species, crosswalk, aggregates := speciesRolesPaths(t, dir) // eurosl_crosswalk.csv never written

	repo := newFakeRepo()
	if _, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates); err == nil {
		t.Fatal("IngestSpeciesRoles with a missing eurosl_crosswalk.csv = nil error, want an error")
	}
	if repo.committed {
		t.Error("ingest committed despite a missing crosswalk file")
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
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Inula hirta,diagnostic,0.8,\n"+
			"not valid @@ id,R22,Bad typology,diagnostic,0.8,\n"+
			"eunis@2021,R22,Bad fidelity,diagnostic,not-a-number,\n"+
			"eunis@2021,R22,Bad constancy,constant,,not-a-number\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv", "name,concept_id\n")
	writeCSV(t, dir, "aggregate_members.csv", "aggregate_concept_id,member_concept_id,member_name\n")
	species, crosswalk, aggregates := speciesRolesPaths(t, dir)

	repo := newFakeRepo()
	rep, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates)
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

func TestIngestSpeciesRoles_MissingSpeciesRolesFileFails(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "eurosl_crosswalk.csv", "name,concept_id\n")
	writeCSV(t, dir, "aggregate_members.csv", "aggregate_concept_id,member_concept_id,member_name\n")
	species, crosswalk, aggregates := speciesRolesPaths(t, dir) // species_roles.csv never written

	repo := newFakeRepo()
	if _, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates); err == nil {
		t.Fatal("IngestSpeciesRoles with a missing source file = nil error, want an error")
	}
}

func TestIngestSpeciesRoles_BeginErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.beginErr = fmt.Errorf("boom")

	dir := seedSpeciesRolesDir(t)
	species, crosswalk, aggregates := speciesRolesPaths(t, dir)
	if _, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates); err == nil {
		t.Fatal("IngestSpeciesRoles with a Begin error = nil error, want an error")
	}
}

func TestIngestSpeciesRoles_RepositoryErrorRollsBackAndReturnsTheError(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertSpeciesRole"

	dir := seedSpeciesRolesDir(t)
	species, crosswalk, aggregates := speciesRolesPaths(t, dir)
	if _, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates); err == nil {
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

	dir := seedSpeciesRolesDir(t)
	species, crosswalk, aggregates := speciesRolesPaths(t, dir)
	_, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates)
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

	dir := seedSpeciesRolesDir(t)
	species, crosswalk, aggregates := speciesRolesPaths(t, dir)
	if _, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates); err == nil {
		t.Fatal("IngestSpeciesRoles with a Commit error = nil error, want an error")
	}
}

// A non-"does not exist" failure while checking aggregate_members.csv (e.g. a
// broken path component) must abort the ingest — only ENOENT is tolerated.
func TestIngestSpeciesRoles_AggregateMembersStatErrorIsAHardError(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\neunis@2021,R22,Inula hirta,diagnostic,0.8,\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv", "name,concept_id\nInula hirta,wcvp:concept:1\n")
	species := filepath.Join(dir, "species_roles.csv")
	crosswalk := filepath.Join(dir, "eurosl_crosswalk.csv")
	// aggregate_members.csv is not a directory — treating it as one makes
	// os.Stat fail with something other than "not exist" (ENOTDIR).
	notADir := filepath.Join(dir, "eurosl_crosswalk.csv", "aggregate_members.csv")

	repo := newFakeRepo()
	if _, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, notADir); err == nil {
		t.Fatal("IngestSpeciesRoles with an unstatable aggregate-members path = nil error, want an error")
	}
}

// A malformed aggregate_members.csv (missing a required column) aborts the
// ingest — the file exists, so this is not the "no derivation" case.
func TestIngestSpeciesRoles_MalformedAggregateMembersFileIsAHardError(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\neunis@2021,R22,Inula hirta,diagnostic,0.8,\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv", "name,concept_id\nInula hirta,wcvp:concept:1\n")
	writeCSV(t, dir, "aggregate_members.csv", "not_the_right_header\n")
	species, crosswalk, aggregates := speciesRolesPaths(t, dir)

	repo := newFakeRepo()
	if _, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates); err == nil {
		t.Fatal("IngestSpeciesRoles with a malformed aggregate_members.csv = nil error, want an error")
	}
}

// A repository failure while writing a derived row must roll back and
// surface the error, exactly like an explicit-row failure does.
func TestIngestSpeciesRoles_DerivedRowRepositoryErrorRollsBack(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Rubus fruticosus aggr.,diagnostic,0.7,\n")
	writeCSV(t, dir, "eurosl_crosswalk.csv",
		"name,concept_id\nRubus fruticosus aggr.,wcvp:concept:99\n")
	writeCSV(t, dir, "aggregate_members.csv",
		"aggregate_concept_id,member_concept_id,member_name\nwcvp:concept:99,wcvp:concept:100,Rubus caesius\n")
	species, crosswalk, aggregates := speciesRolesPaths(t, dir)

	repo := newFakeRepo()
	repo.failOn = "UpsertDerivedSpeciesRole"
	if _, err := IngestSpeciesRoles(context.Background(), repo, species, crosswalk, aggregates); err == nil {
		t.Fatal("IngestSpeciesRoles with a derived-row repository error = nil error, want an error")
	}
	if repo.committed {
		t.Error("ingest committed despite a derived-row repository error")
	}
	if !repo.rolledBack {
		t.Error("ingest did not roll back after a derived-row repository error")
	}
}
