package sqlite

// read_localization.go holds the two localization read paths. Their own file,
// not db.go: that file sits at the CodeCharta ratchet's per-file complexity cap
// and this project's practice is to split rather than raise a baseline.

import (
	"context"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
)

// Localization returns every localization row matching entityType, entityKey and
// lang — every field (name, vernacular, …) and, per field, every source. Which
// field a caller wants is its own policy: name and vernacular belong to one
// answer, and filtering here cost a second query per entity.
//
// The rows come back ordered by (provenance, field, source), which is a TOTAL
// order: those three plus the pinned entity_type/entity_key/lang are exactly the
// primary key, so no two rows can tie. Both consumers
// (application.officialOrCuratedName and application.preferredLabel) promise an
// answer that does not depend on row order, and this makes that promise
// checkable rather than accidental.
//
// field is in the ORDER BY because dropping the field filter made it necessary:
// with several fields in one answer, (provenance, source) alone leaves the
// relative order of a name row and a vernacular row undefined. Ordering on the
// triple, not on source alone, is deliberate for the same reason as before — the
// WHERE now pins only the first three primary-key columns, so an ORDER BY source
// alone would be what sqlite_autoindex_localization_1 already yields and would be
// unobservable, hence impossible to regression-test and silently removable.
func (d *DB) Localization(ctx context.Context, entityType, entityKey, lang string) ([]domain.Localization, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT field, value, source, provenance, derived_from FROM localization
		 WHERE entity_type = ? AND entity_key = ? AND lang = ?
		 ORDER BY provenance, field, source`,
		entityType, entityKey, lang)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying localization %s/%s: %w", entityType, entityKey, err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Localization
	for rows.Next() {
		l := domain.Localization{EntityType: entityType, EntityKey: entityKey, Lang: lang}
		if err := rows.Scan(&l.Field, &l.Value, &l.Source, &l.Provenance, &l.DerivedFrom); err != nil {
			return nil, fmt.Errorf("sqlite: scanning localization %s/%s: %w", entityType, entityKey, err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading localization %s/%s: %w", entityType, entityKey, err)
	}
	return out, nil
}

// LocalizationsByEntityType returns every localization of one entity type in
// one language, bucketed by entity key. It exists so a LIST route costs one
// query instead of one per row: GET /v1/syntaxa lists 1326 alliances, and
// asking Localization per row would be 1326 round trips for at most 25
// answers.
//
// The ORDER BY repeats Localization's total order (provenance, field, source)
// for the same reason: preferredLabel promises an answer that does not depend
// on row order, and both entry points have to make that promise checkable.
// entity_key is first so the buckets arrive contiguously; it is part of the
// primary key, so the order stays total.
//
// An entity type with no rows is an empty map, never an error — a language
// nobody has translated into yet is the normal case.
func (d *DB) LocalizationsByEntityType(ctx context.Context, entityType, lang string) (
	map[string][]domain.Localization, error,
) {
	rows, err := d.QueryContext(ctx,
		`SELECT entity_key, field, value, source, provenance, derived_from FROM localization
		 WHERE entity_type = ? AND lang = ?
		 ORDER BY entity_key, provenance, field, source`,
		entityType, lang)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying localizations of %s: %w", entityType, err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string][]domain.Localization{}
	for rows.Next() {
		l := domain.Localization{EntityType: entityType, Lang: lang}
		if err := rows.Scan(&l.EntityKey, &l.Field, &l.Value, &l.Source, &l.Provenance, &l.DerivedFrom); err != nil {
			return nil, fmt.Errorf("sqlite: scanning localizations of %s: %w", entityType, err)
		}
		out[l.EntityKey] = append(out[l.EntityKey], l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading localizations of %s: %w", entityType, err)
	}
	return out, nil
}
