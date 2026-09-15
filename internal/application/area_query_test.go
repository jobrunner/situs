package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// Areas is the discovery entry point for the ?area= filter: it lists exactly
// the codes that filter can answer, with their names.
func TestQueryService_Areas(t *testing.T) {
	repo := newFakeRepo()
	repo.namedAreas = []domain.NamedArea{
		{Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "FRA"}, NameEN: "France"},
		{Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}, NameEN: "Germany"},
	}

	got, err := NewQueryService(repo).Areas(context.Background())
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	want := []input.AreaView{
		{Scheme: domain.SchemeWGSRPDL3, Code: "FRA", Name: "France"},
		{Scheme: domain.SchemeWGSRPDL3, Code: "GER", Name: "Germany"},
	}
	if len(got) != len(want) {
		t.Fatalf("areas = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("areas[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// An index without distribution data answers an empty list, not an error:
// nothing is wrong, there is simply nothing to filter by yet.
func TestQueryService_Areas_EmptyIndexIsAnEmptyList(t *testing.T) {
	got, err := NewQueryService(newFakeRepo()).Areas(context.Background())
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("areas = %+v, want an empty, non-nil list", got)
	}
}

func TestQueryService_Areas_RepositoryErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.areasErr = errors.New("index unreadable")

	if _, err := NewQueryService(repo).Areas(context.Background()); err == nil {
		t.Fatal("Areas with the repository failing = nil error, want an error")
	}
}
