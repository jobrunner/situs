package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobrunner/situs/internal/ports/input"
)

// SearchSpecies finds the index's own verbatim names containing q.
//
// limit == 0 means "unset" and takes DefaultSearchLimit. A negative or
// oversized limit is rejected rather than clamped: silently answering a
// different question than the one asked is how a client ends up believing
// it saw every hit.
func (q *QueryService) SearchSpecies(ctx context.Context, query string, limit int) ([]input.SpeciesSearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("q must not be empty: %w", input.ErrInvalidQuery)
	}
	switch {
	case limit == 0:
		limit = input.DefaultSearchLimit
	case limit < 0 || limit > input.MaxSearchLimit:
		return nil, fmt.Errorf("limit must be between 1 and %d: %w", input.MaxSearchLimit, input.ErrInvalidQuery)
	}

	names, err := q.repo.SearchSpeciesNames(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("searching species names for %q: %w", query, err)
	}
	out := make([]input.SpeciesSearchHit, 0, len(names))
	for _, n := range names {
		out = append(out, input.SpeciesSearchHit{VerbatimName: n.VerbatimName, ConceptID: n.ConceptID})
	}
	return out, nil
}
