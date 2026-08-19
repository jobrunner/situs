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

// Localization returns every localization row matching entityType, entityKey,
// lang and field — there can be more than one, one per source (an official
// row and a derived row for the same entity/field are both kept).
//
// The rows come back clustered by provenance and, within a provenance, by
// source. Both consumers (application.officialOrCuratedName and
// application.preferredLabel) rank on provenance and promise an answer that does
// not depend on row order, and source is unique per matched key (it is the last
// column of the primary key), so this is a total order — two official rows from
// different sources can no longer resolve differently between two ingests.
//
// Ordering on the pair, not on source alone, is deliberate: the WHERE pins the
// first four columns of the primary key, so source alone is what
// sqlite_autoindex_localization_1 already yields and an ORDER BY source would be
// unobservable — correct, but impossible to regression-test and therefore
// silently removable.
func (d *DB) Localization(ctx context.Context, entityType, entityKey, lang, field string) ([]domain.Localization, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT value, source, provenance, derived_from FROM localization
		 WHERE entity_type = ? AND entity_key = ? AND lang = ? AND field = ?
		 ORDER BY provenance, source`,
		entityType, entityKey, lang, field)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying localization %s/%s: %w", entityType, entityKey, err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Localization
	for rows.Next() {
		l := domain.Localization{EntityType: entityType, EntityKey: entityKey, Lang: lang, Field: field}
		if err := rows.Scan(&l.Value, &l.Source, &l.Provenance, &l.DerivedFrom); err != nil {
			return nil, fmt.Errorf("sqlite: scanning localization %s/%s: %w", entityType, entityKey, err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading localization %s/%s: %w", entityType, entityKey, err)
	}
	return out, nil
}
