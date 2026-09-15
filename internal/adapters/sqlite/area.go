package sqlite

import (
	"context"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
)

// AreasWithData lists the areas of one scheme the index has distribution data
// for, each with its ingested name, sorted by code.
//
// The distribution table decides membership and the area table only lends the
// name: an area nobody occurs in would be a filter that can only ever answer
// empty, and a code with data but no name row stays in the list with an empty
// name instead of being dropped.
func (d *DB) AreasWithData(ctx context.Context, scheme string) ([]domain.NamedArea, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT d.area_code, COALESCE(a.name_en, '')
		 FROM (SELECT DISTINCT area_scheme, area_code FROM species_distribution
		       WHERE area_scheme = ?) d
		 LEFT JOIN area a ON a.area_scheme = d.area_scheme AND a.area_code = d.area_code
		 ORDER BY d.area_code`, scheme)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading areas with data: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.NamedArea{}
	for rows.Next() {
		a := domain.NamedArea{Area: domain.Area{Scheme: scheme}}
		if err := rows.Scan(&a.Code, &a.NameEN); err != nil {
			return nil, fmt.Errorf("sqlite: scanning area: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating areas: %w", err)
	}
	return out, nil
}
