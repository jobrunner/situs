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

	got, err := NewQueryService(repo).Areas(context.Background(), domain.SchemeWGSRPDL3)
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
	got, err := NewQueryService(newFakeRepo()).Areas(context.Background(), domain.SchemeWGSRPDL3)
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

	if _, err := NewQueryService(repo).Areas(context.Background(), domain.SchemeWGSRPDL3); err == nil {
		t.Fatal("Areas with the repository failing = nil error, want an error")
	}
}

// Areas passes the scheme through unchanged: species areas (wgsrpd_l3) and
// syntaxa territories (evc_territory) never mix into one flat list, because
// the same code string could mean something different in each scheme.
func TestAreasReichtDasSchemaDurch(t *testing.T) {
	repo := newFakeRepo()
	repo.namedAreas = []domain.NamedArea{
		{Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}, NameEN: "Germany"},
		{Area: domain.Area{Scheme: domain.SchemeEVCTerritory, Code: "austria-alps"}, NameEN: "Austria Alps"},
	}
	q := NewQueryService(repo)

	terr, err := q.Areas(context.Background(), domain.SchemeEVCTerritory)
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if len(terr) != 1 || terr[0].Scheme != domain.SchemeEVCTerritory || terr[0].Code != "austria-alps" {
		t.Errorf("Territorien = %+v", terr)
	}

	wg, err := q.Areas(context.Background(), domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if len(wg) != 1 || wg[0].Code != "GER" {
		t.Errorf("WGSRPD = %+v", wg)
	}
}
