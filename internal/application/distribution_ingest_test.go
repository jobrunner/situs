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
	repo.conceptIDsErr = errors.New("index unreadable")

	if _, err := IngestDistribution(context.Background(), repo, &fakeDistSource{}); err == nil {
		t.Fatal("IngestDistribution: want an error, got nil")
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
	repo.beginErr = errors.New("cannot open transaction")
	src := &fakeDistSource{areas: map[string][]domain.Area{
		"wcvp:concept:1": {{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
	}}

	if _, err := IngestDistribution(context.Background(), repo, src); err == nil {
		t.Fatal("IngestDistribution: want an error, got nil")
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
	if repo.committed {
		t.Error("ingest committed despite the injected UpsertDistribution failure")
	}
}

func TestIngestDistribution_CommitErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}, ConceptID: strPtr("wcvp:concept:1"), VerbatimName: "A", Role: "diagnostic"},
	}
	repo.commitErr = errors.New("disk full")
	src := &fakeDistSource{areas: map[string][]domain.Area{
		"wcvp:concept:1": {{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
	}}

	if _, err := IngestDistribution(context.Background(), repo, src); err == nil {
		t.Fatal("IngestDistribution: want an error, got nil")
	}
}
