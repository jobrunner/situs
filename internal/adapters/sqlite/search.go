package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
)

// likeEscape makes q a literal for a LIKE pattern: the wildcards % and _
// and the escape character itself lose their special meaning. Without this,
// a query of "%" would match every name in the index instead of the (zero)
// names that actually contain a percent sign.
//
// The backslash is escaped FIRST — doing it last would also escape the
// backslashes this function just introduced.
func likeEscape(q string) string {
	q = strings.ReplaceAll(q, `\`, `\\`)
	q = strings.ReplaceAll(q, "%", `\%`)
	q = strings.ReplaceAll(q, "_", `\_`)
	return q
}

// SearchSpeciesNames implements output.Repository.
//
// DISTINCT over both columns, not just the name: measured on the pinned
// index no name carries more than one concept id, but if a future ingest
// produced one, showing both rows is the honest answer — silently keeping
// whichever row sqlite returned first would hide the ambiguity.
//
// ORDER BY carries concept_id as a second key, not just verbatim_name: the
// port promises a stably reproducible order, and two rows sharing a name
// would otherwise tie on the first key, leaving sqlite free to return them
// in either order from one call to the next — and to drop a different one
// of the two at the LIMIT boundary each time. concept_id can be NULL; sqlite
// orders NULL before any non-NULL value in ASC, so the tie still breaks the
// same way every time.
//
// sqlite's LIKE is case-insensitive for ASCII by default, which is what the
// binomial Latin names in this index are.
func (d *DB) SearchSpeciesNames(ctx context.Context, q string, limit int) ([]domain.SpeciesName, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT DISTINCT verbatim_name, concept_id FROM species_role
		 WHERE verbatim_name LIKE ? ESCAPE '\'
		 ORDER BY verbatim_name, concept_id
		 LIMIT ?`,
		"%"+likeEscape(q)+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: searching species names for %q: %w", q, err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.SpeciesName{}
	for rows.Next() {
		var n domain.SpeciesName
		var conceptID sql.NullString
		if err := rows.Scan(&n.VerbatimName, &conceptID); err != nil {
			return nil, fmt.Errorf("sqlite: scanning species name for %q: %w", q, err)
		}
		if conceptID.Valid {
			id := conceptID.String
			n.ConceptID = &id
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading species names for %q: %w", q, err)
	}
	return out, nil
}
