package application

import (
	"context"
	"fmt"
	"slices"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// syntaxonDistributionOf turns the repository's four-state value into the
// read API's three fields plus presence.
//
// The pointer IS the fourth state. Covered false becomes nil, which the JSON
// omits, and a client then knows nothing was claimed. Returning an empty
// object instead would claim "occurs in none of the 136 territories" for
// every bryophyte, lichen and algal alliance — 212 of 1326 rows, none of
// which the source says anything about.
func syntaxonDistributionOf(ctx context.Context, repo interface {
	SyntaxonDistribution(ctx context.Context, syntaxonID, scheme string) (domain.SyntaxonDistribution, error)
}, syntaxonID string) (*input.SyntaxonDistribution, error) {
	d, err := repo.SyntaxonDistribution(ctx, syntaxonID, domain.SchemeEVCTerritory)
	if err != nil {
		return nil, fmt.Errorf("reading distribution of syntaxon %q: %w", syntaxonID, err)
	}
	if !d.Covered {
		return nil, nil
	}
	// Both lists are non-nil even when empty: an omitted array and an empty
	// array are not the same thing for a client, and "checked, occurs nowhere"
	// must arrive as two empty arrays rather than two missing fields.
	verified, uncertain := d.Verified, d.Uncertain
	if verified == nil {
		verified = []string{}
	}
	if uncertain == nil {
		uncertain = []string{}
	}
	return &input.SyntaxonDistribution{
		AreaScheme: d.Scheme,
		Verified:   verified,
		Uncertain:  uncertain,
	}, nil
}

// filterSyntaxaByArea marks the hits and drops the definite misses, keeping
// every row the source says nothing about.
//
// Three outcomes per row, and the third is the one that is easy to get wrong:
//   - an occurrence in Include        -> kept, marked
//   - covered, but no matching row    -> dropped; this is a DEFINITE statement
//   - not covered                     -> kept, UNMARKED; nothing was claimed
//
// Same rule only_in_area follows on the species side: a list that silently
// loses what it cannot judge is dishonestly clean.
func filterSyntaxaByArea(refs []input.SyntaxonRef, occurrences map[string]string,
	covered map[string]bool, include []string) []input.SyntaxonRef {
	out := make([]input.SyntaxonRef, 0, len(refs))
	for _, r := range refs {
		occ, hasRow := occurrences[r.ID]
		switch {
		case hasRow && slices.Contains(include, occ):
			r.Occurrence = occ
			out = append(out, r)
		case covered[r.ID]:
			// The source looked and did not record this occurrence here.
			continue
		default:
			out = append(out, r)
		}
	}
	return out
}

// syntaxonAreaLookup validates the code against the index and reads the two
// maps the filter needs. An unknown code is an error, never a list of "does
// not occur" — a typo and a real absence must not look the same.
func (q *QueryService) syntaxonAreaLookup(ctx context.Context, filter input.SyntaxonAreaFilter) (map[string]string, map[string]bool, error) {
	known, err := q.repo.KnownAreaCodes(ctx, domain.SchemeEVCTerritory)
	if err != nil {
		return nil, nil, err
	}
	if !slices.Contains(known, filter.Code) {
		return nil, nil, fmt.Errorf("area %q: %w", filter.Code, input.ErrUnknownArea)
	}
	occurrences, err := q.repo.SyntaxonOccurrencesInArea(ctx, domain.SchemeEVCTerritory, filter.Code)
	if err != nil {
		return nil, nil, err
	}
	covered, err := q.repo.SyntaxaWithCoverage(ctx, domain.SchemeEVCTerritory)
	if err != nil {
		return nil, nil, err
	}
	return occurrences, covered, nil
}
