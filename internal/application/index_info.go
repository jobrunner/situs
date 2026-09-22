package application

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// IndexInfo measures what the index holds. Nothing here is configured: a
// client's whole reason to ask is to find out whether *this* index can answer
// its concept ids, and a configured claim could not tell it that.
//
// Kept in its own file, not query.go: query.go is the read side's use-case
// layer and is watched by the codecharta complexity ratchet (see
// .codecharta-ratchet.json's _query_note) — every prior growth spot
// (preferredLabel, descriptionOf, syntaxaOf's mapping) was moved out rather
// than raising that baseline, and the syntaxa-distribution measurement below
// follows the same rule.
func (q *QueryService) IndexInfo(ctx context.Context) (input.IndexInfo, error) {
	ids, err := q.repo.ConceptIDs(ctx)
	if err != nil {
		return input.IndexInfo{}, fmt.Errorf("listing concept ids: %w", err)
	}
	areas, err := q.repo.KnownAreaCodes(ctx, domain.SchemeWGSRPDL3)
	if err != nil {
		return input.IndexInfo{}, fmt.Errorf("listing known area codes: %w", err)
	}
	covered, err := q.repo.SyntaxaWithCoverage(ctx, domain.SchemeEVCTerritory)
	if err != nil {
		return input.IndexInfo{}, fmt.Errorf("counting syntaxa with distribution: %w", err)
	}
	return input.IndexInfo{
		ConceptBackbones:        backbonesOf(ids),
		SpeciesWithConcept:      len(ids),
		AreaScheme:              domain.SchemeWGSRPDL3,
		AreasWithData:           len(areas),
		SyntaxonAreaScheme:      domain.SchemeEVCTerritory,
		SyntaxaWithDistribution: len(covered),
	}, nil
}

// backbonesOf reduces concept ids to their distinct prefixes, sorted so the
// answer does not depend on row order. An id without a prefix is reported as
// the empty-string backbone rather than hidden — a malformed id in the index is
// something the operator should see.
func backbonesOf(conceptIDs []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range conceptIDs {
		prefix, _, _ := strings.Cut(id, ":")
		if !seen[prefix] {
			seen[prefix] = true
			out = append(out, prefix)
		}
	}
	slices.Sort(out)
	return out
}
