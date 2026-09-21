package application

import (
	"context"
	"fmt"

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
