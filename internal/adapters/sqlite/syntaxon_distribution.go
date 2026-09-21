package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
)

// SyntaxonDistribution reads the occurrence rows and the coverage row in two
// queries rather than one outer join: the coverage row is a different fact
// from the occurrence rows (it exists for a syntaxon with no occurrence at
// all), and folding both into one result set would make "no rows" ambiguous
// again — which is the exact confusion the second table exists to prevent.
func (d *DB) SyntaxonDistribution(ctx context.Context, syntaxonID, scheme string) (domain.SyntaxonDistribution, error) {
	out := domain.SyntaxonDistribution{
		Scheme:    scheme,
		Verified:  []string{},
		Uncertain: []string{},
	}

	var one int
	row := d.QueryRowContext(ctx,
		`SELECT 1 FROM syntaxon_distribution_coverage
		 WHERE syntaxon_id = ? AND area_scheme = ?`, syntaxonID, scheme)
	switch err := row.Scan(&one); {
	case err == nil:
		out.Covered = true
	case errors.Is(err, sql.ErrNoRows):
		out.Covered = false
	default:
		return domain.SyntaxonDistribution{}, fmt.Errorf("sqlite: reading syntaxon coverage: %w", err)
	}

	rows, err := d.QueryContext(ctx,
		`SELECT area_code, occurrence FROM syntaxon_distribution
		 WHERE syntaxon_id = ? AND area_scheme = ?
		 ORDER BY area_code`, syntaxonID, scheme)
	if err != nil {
		return domain.SyntaxonDistribution{}, fmt.Errorf("sqlite: reading syntaxon distribution: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var code, occurrence string
		if err := rows.Scan(&code, &occurrence); err != nil {
			return domain.SyntaxonDistribution{}, fmt.Errorf("sqlite: scanning syntaxon distribution: %w", err)
		}
		if occurrence == domain.OccurrenceUncertain {
			out.Uncertain = append(out.Uncertain, code)
			continue
		}
		out.Verified = append(out.Verified, code)
	}
	if err := rows.Err(); err != nil {
		return domain.SyntaxonDistribution{}, fmt.Errorf("sqlite: iterating syntaxon distribution: %w", err)
	}
	return out, nil
}

// SyntaxonOccurrencesInArea answers the ?area= filter with one query instead
// of one per syntaxon: the filtered list can hold 1326 entries.
func (d *DB) SyntaxonOccurrencesInArea(ctx context.Context, scheme, code string) (map[string]string, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT syntaxon_id, occurrence FROM syntaxon_distribution
		 WHERE area_scheme = ? AND area_code = ?`, scheme, code)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading occurrences in area: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]string{}
	for rows.Next() {
		var id, occurrence string
		if err := rows.Scan(&id, &occurrence); err != nil {
			return nil, fmt.Errorf("sqlite: scanning occurrence: %w", err)
		}
		out[id] = occurrence
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating occurrences in area: %w", err)
	}
	return out, nil
}

// SyntaxaWithCoverage is the set the read side needs to tell an absence from
// an unknown while filtering.
func (d *DB) SyntaxaWithCoverage(ctx context.Context, scheme string) (map[string]bool, error) {
	ids, err := d.queryStrings(ctx, "syntaxa with distribution coverage",
		`SELECT syntaxon_id FROM syntaxon_distribution_coverage
		 WHERE area_scheme = ?`, scheme)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}
