package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// Description returns the prose description of one habitat type, or
// output.ErrNotFound when none was ingested for it. Absence is the normal
// case: the factsheets describe EUNIS level 3, the index holds eight levels.
func (d *DB) Description(ctx context.Context, key domain.HabitatTypeKey) (domain.HabitatDescription, error) {
	out := domain.HabitatDescription{Key: key}
	row := d.QueryRowContext(ctx,
		`SELECT description_en, source, provenance FROM habitat_description
		 WHERE typology_id = ? AND code = ?`,
		string(key.Typology), key.Code)
	if err := row.Scan(&out.TextEN, &out.Source, &out.Provenance); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.HabitatDescription{}, fmt.Errorf("sqlite: description of %s: %w", key, output.ErrNotFound)
		}
		return domain.HabitatDescription{}, fmt.Errorf("sqlite: querying description of %s: %w", key, err)
	}
	return out, nil
}
