package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
	"github.com/jobrunner/situs/internal/ports/output"
)

// The factsheet description and its German overlay live here rather than in
// query.go, same split as area.go and label.go: query.go orchestrates
// endpoints, the per-concern rules sit beside the concern.

// descriptionOf reads the factsheet text and, with lang=de, its German
// overlay. A type without a description is the normal case (the factsheets
// describe EUNIS level 3, the index holds eight levels), so ErrNotFound means
// "no description" and yields an empty answer, never a failed request.
func (q *QueryService) descriptionOf(ctx context.Context, key domain.HabitatTypeKey, lang string) (string, *input.GermanLabel, error) {
	d, err := q.repo.Description(ctx, key)
	if err != nil {
		if errors.Is(err, output.ErrNotFound) {
			return "", nil, nil
		}
		return "", nil, fmt.Errorf("fetching description of %s: %w", key, err)
	}
	if lang != deLang {
		return d.TextEN, nil, nil
	}
	labels, err := q.repo.Localization(ctx, "habitat_type", key.String(), deLang)
	if err != nil {
		return "", nil, fmt.Errorf("fetching German description of %s: %w", key, err)
	}
	return d.TextEN, preferredDescription(labels), nil
}
