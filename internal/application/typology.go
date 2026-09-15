package application

import (
	"context"
	"fmt"

	"github.com/jobrunner/situs/internal/ports/input"
)

// Typologies lists every typology the index carries, with the measured
// count of habitat types each one has. It is the discovery entry point for
// the (typology, code) addressing every habitat-type route uses: a client
// otherwise has no way to learn that eunis@2012 and annex1 exist alongside
// eunis@2021.
//
// Split out of query.go on purpose: the codecharta ratchet keeps that file's
// complexity from creeping back up, one endpoint's worth of logic at a time.
func (q *QueryService) Typologies(ctx context.Context) ([]input.TypologyView, error) {
	summaries, err := q.repo.Typologies(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing typologies: %w", err)
	}
	out := make([]input.TypologyView, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, input.TypologyView{
			ID:           s.Typology.ID,
			Scheme:       s.Typology.Scheme,
			Version:      s.Typology.Version,
			Name:         s.Typology.Name,
			SourceRef:    s.Typology.SourceRef,
			HabitatTypes: s.HabitatTypes,
		})
	}
	return out, nil
}
