// Package sqlite is the driven adapter for the local, read-mostly index. It
// uses modernc.org/sqlite, a pure-Go driver, so the binary stays CGO-free.
// Task 3 adds the schema and the read/write implementations of the Repository
// port; this file only owns opening and closing a connection.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver
)

// DriverName is the driver registered by modernc.org/sqlite.
const DriverName = "sqlite"

// Open opens the index at dsn and verifies it is reachable. dsn is a file path
// or ":memory:".
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open(DriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite index %q: %w", dsn, err)
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("pinging sqlite index %q: %w", dsn, err), db.Close())
	}
	return db, nil
}
