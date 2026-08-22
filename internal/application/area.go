package application

import (
	"context"
	"fmt"
	"slices"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// areaLookup resolves the filter once per request: it validates the code
// against the index and returns the areas of the concepts in play. It returns
// nil, nil when no filter was asked for, so the read path makes no extra
// database call in the common case.
func (q *QueryService) areaLookup(ctx context.Context, filter input.AreaFilter, conceptIDs []string) (map[string][]string, error) {
	if !filter.Active() {
		return nil, nil
	}
	known, err := q.repo.KnownAreaCodes(ctx, domain.SchemeWGSRPDL3)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(known, filter.Code) {
		return nil, fmt.Errorf("area %q: %w", filter.Code, input.ErrUnknownArea)
	}
	return q.repo.AreasForConcepts(ctx, conceptIDs, domain.SchemeWGSRPDL3)
}

// inArea is the three-state answer: nil when not knowable.
func inArea(areas map[string][]string, conceptID, code string) *bool {
	if areas == nil || conceptID == "" {
		return nil
	}
	list, ok := areas[conceptID]
	if !ok {
		return nil // the concept has no distribution data at all
	}
	yes := slices.Contains(list, code)
	return &yes
}

// conceptIDsOf collects the resolved concept ids a set of species roles
// carries, so areaLookup queries only the concepts actually in play.
func conceptIDsOf(roles []domain.SpeciesRole) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		if r.ConceptID != nil {
			out = append(out, *r.ConceptID)
		}
	}
	return out
}

// markAndFilter sets InArea on every entry and, when filter.OnlyInArea, drops
// the entries whose absence is definite. An entry whose InArea is unknowable
// (nil) always stays: a list that silently loses what it cannot judge would be
// dishonestly clean.
func markAndFilter(entries []input.SpeciesEntry, areas map[string][]string, filter input.AreaFilter) []input.SpeciesEntry {
	out := make([]input.SpeciesEntry, 0, len(entries))
	for _, e := range entries {
		e.InArea = inArea(areas, e.ConceptID, filter.Code)
		if filter.OnlyInArea && e.InArea != nil && !*e.InArea {
			continue
		}
		out = append(out, e)
	}
	return out
}
