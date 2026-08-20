package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

type fakeDistSource struct {
	areas map[string][]domain.Area
	err   error
	asked [][]string
}

func (f *fakeDistSource) Areas(_ context.Context, ids []string) (map[string][]domain.Area, error) {
	f.asked = append(f.asked, ids)
	if f.err != nil {
		return nil, f.err
	}
	return f.areas, nil
}

func TestIngestDistribution_StoresAreasForTheIndexedConcepts(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "diagnostic"},
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R23"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "constant"},
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:2"), VerbatimName: "B", Role: "diagnostic"},
	}
	src := &fakeDistSource{areas: map[string][]domain.Area{
		"wcvp:concept:1": {{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}, {Scheme: domain.SchemeWGSRPDL3, Code: "FRA"}},
	}}

	rep, err := IngestDistribution(context.Background(), repo, src)
	if err != nil {
		t.Fatalf("IngestDistribution: %v", err)
	}
	if rep.Concepts != 2 {
		t.Errorf("Concepts = %d, want 2 distinct concept ids", rep.Concepts)
	}
	if rep.WithAreas != 1 || rep.Rows != 2 {
		t.Errorf("report = %+v, want WithAreas 1 / Rows 2", rep)
	}
	if len(src.asked) != 1 {
		t.Errorf("source was asked %d times, want once for all distinct ids", len(src.asked))
	}
	if len(src.asked[0]) != 2 {
		t.Errorf("asked for %v, want the two distinct ids (deduplicated)", src.asked[0])
	}
}

// The distribution is extra information, not a fact of the index. An outage
// must leave the index usable — unlike the species-role ingest, where a
// resolver failure aborts so that 13791 names are not all booked unresolvable.
func TestIngestDistribution_SourceFailureDoesNotAbort(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "diagnostic"},
	}
	src := &fakeDistSource{err: errors.New("upstream down")}

	rep, err := IngestDistribution(context.Background(), repo, src)
	if err != nil {
		t.Fatalf("IngestDistribution returned an error, want a usable report: %v", err)
	}
	if rep.WithAreas != 0 || rep.Rows != 0 {
		t.Errorf("report = %+v, want zeros — a visible statement, not a silent failure", rep)
	}
	if repo.committed {
		t.Error("nothing was written, so no transaction should have been committed")
	}
}

func TestIngestDistribution_NoConceptsIsNotAnError(t *testing.T) {
	repo := newFakeRepo()
	rep, err := IngestDistribution(context.Background(), repo, &fakeDistSource{})
	if err != nil {
		t.Fatalf("IngestDistribution: %v", err)
	}
	if rep.Concepts != 0 {
		t.Errorf("Concepts = %d, want 0", rep.Concepts)
	}
}

func TestIngestDistribution_ConceptIDsErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	wantErr := errors.New("index unreadable")
	repo.conceptIDsErr = wantErr

	rep, err := IngestDistribution(context.Background(), repo, &fakeDistSource{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("IngestDistribution error = %v, want it to wrap %v", err, wantErr)
	}
	if rep != (DistributionReport{}) {
		t.Errorf("report = %+v, want the zero report — the index was never even read", rep)
	}
}

func TestIngestDistribution_EmptyAreasMapIsNotAnError(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "diagnostic"},
	}
	src := &fakeDistSource{areas: map[string][]domain.Area{}}

	rep, err := IngestDistribution(context.Background(), repo, src)
	if err != nil {
		t.Fatalf("IngestDistribution: %v", err)
	}
	if rep.WithAreas != 0 || rep.Rows != 0 {
		t.Errorf("report = %+v, want zeros for an empty areas map", rep)
	}
	if repo.committed {
		t.Error("nothing to write, so no transaction should have been committed")
	}
}

func TestIngestDistribution_BeginErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "diagnostic"},
	}
	wantErr := errors.New("cannot open transaction")
	repo.beginErr = wantErr
	src := &fakeDistSource{areas: map[string][]domain.Area{
		"wcvp:concept:1": {{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
	}}

	rep, err := IngestDistribution(context.Background(), repo, src)
	if !errors.Is(err, wantErr) {
		t.Fatalf("IngestDistribution error = %v, want it to wrap %v", err, wantErr)
	}
	if rep.WithAreas != 0 || rep.Rows != 0 {
		t.Errorf("report = %+v, want zeros — nothing was written before Begin failed", rep)
	}
	if repo.committed {
		t.Error("ingest committed despite Begin having failed")
	}
}

func TestIngestDistribution_UpsertErrorRollsBack(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "diagnostic"},
	}
	repo.failOn = "UpsertDistribution"
	src := &fakeDistSource{areas: map[string][]domain.Area{
		"wcvp:concept:1": {{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
	}}

	if _, err := IngestDistribution(context.Background(), repo, src); err == nil {
		t.Fatal("IngestDistribution: want an error, got nil")
	}
	if !repo.rolledBack {
		t.Error("ingest did not roll back despite the injected UpsertDistribution failure")
	}
	if repo.committed {
		t.Error("ingest committed despite the injected UpsertDistribution failure")
	}
}

func TestIngestDistribution_CommitErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "diagnostic"},
	}
	wantErr := errors.New("disk full")
	repo.commitErr = wantErr
	src := &fakeDistSource{areas: map[string][]domain.Area{
		"wcvp:concept:1": {{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
	}}

	rep, err := IngestDistribution(context.Background(), repo, src)
	if !errors.Is(err, wantErr) {
		t.Fatalf("IngestDistribution error = %v, want it to wrap %v", err, wantErr)
	}
	if rep.Rows != 1 {
		t.Errorf("report = %+v, want Rows 1 — the row was upserted before the failed commit", rep)
	}
	if repo.committed {
		t.Error("Commit reported an error, so the transaction must not count as committed")
	}
}

// A source implementing only output.DistributionSource (not the stricter
// hostus adapter contract) could hand back a half-filled Area. The writer
// must guard against that itself, not rely on the one known reader having
// already filtered it.
func TestIngestDistribution_SkipsIncompleteAreasAndCountsThem(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "diagnostic"},
	}
	src := &fakeDistSource{areas: map[string][]domain.Area{
		"wcvp:concept:1": {
			{Scheme: domain.SchemeWGSRPDL3, Code: "GER"},
			{Scheme: domain.SchemeWGSRPDL3, Code: ""},
			{Scheme: "", Code: "FRA"},
		},
	}}

	rep, err := IngestDistribution(context.Background(), repo, src)
	if err != nil {
		t.Fatalf("IngestDistribution: %v", err)
	}
	if rep.Rows != 1 {
		t.Errorf("report = %+v, want Rows 1 — only the complete area was written", rep)
	}
	if rep.Incomplete != 2 {
		t.Errorf("report = %+v, want Incomplete 2 for the two half-filled areas", rep)
	}
	if len(repo.distribution) != 1 || repo.distribution[0].Area.Code != "GER" {
		t.Errorf("stored distribution = %+v, want exactly the one complete area", repo.distribution)
	}
}

// A canceled or expired context is not a source outage to tolerate: the run
// was told to stop, and that must fail here rather than resurface later as an
// unrelated failure in a subsequent ingest step.
func TestIngestDistribution_ContextCancellationAbortsInsteadOfWarning(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "diagnostic"},
	}
	src := &fakeDistSource{err: context.Canceled}

	_, err := IngestDistribution(context.Background(), repo, src)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("IngestDistribution error = %v, want it to wrap context.Canceled", err)
	}
}

// Failed is not IngestDistribution's to populate — a plain
// output.DistributionSource cannot report partial failure counts, and
// IngestDistribution must not need a type assertion to a concrete decorator
// to find out. The composition root (cmd/situs) fills Failed in afterward;
// see TestIngestCommand_ReportsFailedConceptsFromThePacedDecorator there.
func TestIngestDistribution_LeavesFailedAtZero(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "diagnostic"},
	}
	src := &fakeDistSource{areas: map[string][]domain.Area{
		"wcvp:concept:1": {{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
	}}

	rep, err := IngestDistribution(context.Background(), repo, src)
	if err != nil {
		t.Fatalf("IngestDistribution: %v", err)
	}
	if rep.Failed != 0 {
		t.Errorf("report = %+v, want Failed 0 — this use case never sets it", rep)
	}
}
