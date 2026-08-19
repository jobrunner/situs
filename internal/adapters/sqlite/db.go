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
