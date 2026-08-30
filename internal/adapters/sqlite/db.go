// Package sqlite is the driven adapter for the local, read-mostly index. It
// uses modernc.org/sqlite, a pure-Go driver, so the binary stays CGO-free.
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver
)

// DriverName is the driver registered by modernc.org/sqlite.
const DriverName = "sqlite"

//go:embed schema.sql
var schema string

// DB is the local habitat-type index: a thin wrapper over *sql.DB that also
// implements output.Repository.
type DB struct {
	*sql.DB
}

// Open opens the index at dsn, verifies it is reachable and applies the
// schema. dsn is a file path or ":memory:".
func Open(ctx context.Context, dsn string) (*DB, error) {
	sqlDB, err := sql.Open(DriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite index %q: %w", dsn, err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("pinging sqlite index %q: %w", dsn, err), sqlDB.Close())
	}
	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON"} {
		if _, err := sqlDB.ExecContext(ctx, pragma); err != nil {
			return nil, errors.Join(fmt.Errorf("applying %q to %q: %w", pragma, dsn, err), sqlDB.Close())
		}
	}
	if _, err := sqlDB.ExecContext(ctx, schema); err != nil {
		return nil, errors.Join(fmt.Errorf("applying schema to %q: %w", dsn, err), sqlDB.Close())
	}
	return &DB{DB: sqlDB}, nil
}

// Begin starts one atomic ingest run.
func (d *DB) Begin(ctx context.Context) (output.IngestTx, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite: beginning ingest transaction: %w", err)
	}
	return &ingestTx{ctx: ctx, tx: tx}, nil
}

// queryStrings runs a "SELECT DISTINCT <col> ..." query that yields exactly
// one string column per row and collects the results. what is used only in
// error messages, so ConceptIDs, KnownVocabs and KnownAreaCodes each get their
// own wording despite sharing this body.
func (d *DB) queryStrings(ctx context.Context, what, query string, args ...any) ([]string, error) {
	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading %s: %w", what, err)
	}
	defer func() { _ = rows.Close() }()

	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("sqlite: scanning %s: %w", what, err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating %s: %w", what, err)
	}
	return out, nil
}

// HabitatType looks up an abstract habitat type by its (typology, code) key.
// It returns output.ErrNotFound (wrapped) when no such row exists.
func (d *DB) HabitatType(ctx context.Context, key domain.HabitatTypeKey) (domain.HabitatType, error) {
	var level sql.NullInt64
	var priority sql.NullInt64
	h := domain.HabitatType{Key: key}

	row := d.QueryRowContext(ctx,
		`SELECT level, name_en, parent_code, priority FROM habitat_type WHERE typology_id = ? AND code = ?`,
		string(key.Typology), key.Code)
	if err := row.Scan(&level, &h.NameEN, &h.ParentCode, &priority); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.HabitatType{}, fmt.Errorf("sqlite: habitat type %s: %w", key, output.ErrNotFound)
		}
		return domain.HabitatType{}, fmt.Errorf("sqlite: querying habitat type %s: %w", key, err)
	}
	if level.Valid {
		l := int(level.Int64)
		h.Level = &l
	}
	if priority.Valid {
		p := priority.Int64 != 0
		h.Priority = &p
	}
	return h, nil
}

// CrosswalksTo returns every crosswalk whose To.Typology is typology — used
// by DeriveGermanLabels to find every type crosswalked to Annex I.
func (d *DB) CrosswalksTo(ctx context.Context, typology domain.TypologyID) ([]domain.Crosswalk, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT from_typology, from_code, to_typology, to_code, qualifier
		 FROM habitat_type_crosswalk WHERE to_typology = ?
		 ORDER BY from_typology, from_code, to_code`,
		string(typology))
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying crosswalks to %s: %w", typology, err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Crosswalk
	for rows.Next() {
		var fromTypology, toTypology, qualifier string
		var c domain.Crosswalk
		if err := rows.Scan(&fromTypology, &c.From.Code, &toTypology, &c.To.Code, &qualifier); err != nil {
			return nil, fmt.Errorf("sqlite: scanning crosswalk to %s: %w", typology, err)
		}
		c.From.Typology = domain.TypologyID(fromTypology)
		c.To.Typology = domain.TypologyID(toTypology)
		c.Qualifier = domain.Qualifier(qualifier)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading crosswalks to %s: %w", typology, err)
	}
	return out, nil
}

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
