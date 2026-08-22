package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// stub_driver_test.go provides a minimal database/sql/driver double so the
// read side's rows.Err()/Scan error returns can be exercised
// deterministically, without racing a context cancellation against
// row iteration timing (which is not reproducible run to run — see the git
// history of write_test.go for the test this replaced).

type stubMode int

const (
	// stubModeRowsErr makes the first Next call fail with a non-EOF error,
	// which surfaces through sql.Rows.Err() after the loop.
	stubModeRowsErr stubMode = iota
	// stubModeScanErr succeeds one Next call with a column value no string
	// destination can hold, which surfaces through rows.Scan.
	stubModeScanErr
)

// newStubDB builds a *sql.DB backed by the stub driver — no schema, no file,
// just enough of the driver.Conn/driver.Rows contract for one QueryContext
// call from any of the list reads.
func newStubDB(t *testing.T, mode stubMode) *sql.DB {
	t.Helper()
	db := sql.OpenDB(&stubConnector{mode: mode})
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type stubConnector struct{ mode stubMode }

func (c *stubConnector) Connect(context.Context) (driver.Conn, error) {
	return &stubConn{mode: c.mode}, nil
}
func (c *stubConnector) Driver() driver.Driver { return stubDriver{} }

// stubDriver is never actually used to open a connection (stubConnector.Connect
// bypasses it) but driver.Connector requires one.
type stubDriver struct{}

func (stubDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("stub: Open is not implemented, use the Connector")
}

type stubConn struct{ mode stubMode }

func (c *stubConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("stub: Prepare is not implemented, QueryContext is used directly")
}
func (c *stubConn) Close() error { return nil }
func (c *stubConn) Begin() (driver.Tx, error) {
	return nil, errors.New("stub: Begin is not implemented")
}

// QueryContext picks the column set by matching the table name in the query
// text — every list read queries one table (or one join), so this is enough to
// serve them all without needing a real SQL engine.
func (c *stubConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "habitat_type_crosswalk"):
		return &stubRows{cols: []string{"from_typology", "from_code", "to_typology", "to_code", "qualifier"}, mode: c.mode}, nil
	case strings.Contains(query, "FROM localization"):
		return &stubRows{cols: []string{"value", "source", "provenance", "derived_from"}, mode: c.mode}, nil
	case strings.Contains(query, "FROM species_role WHERE concept_id"):
		return &stubRows{cols: []string{
			"typology_id", "code", "concept_id", "verbatim_name", "role", "fidelity", "constancy",
		}, mode: c.mode}, nil
	case strings.Contains(query, "FROM species_role"):
		return &stubRows{cols: []string{"concept_id", "verbatim_name", "role", "fidelity", "constancy"}, mode: c.mode}, nil
	case strings.Contains(query, "JOIN syntaxon"):
		return &stubRows{cols: []string{"id", "rank", "name", "parent_id"}, mode: c.mode}, nil
	case strings.Contains(query, "FROM habitat_type_syntaxon"):
		return &stubRows{cols: []string{"typology_id", "code"}, mode: c.mode}, nil
	case strings.Contains(query, "concept_id, area_code FROM species_distribution"):
		return &stubRows{cols: []string{"concept_id", "area_code"}, mode: c.mode}, nil
	case strings.Contains(query, "DISTINCT area_code FROM species_distribution"):
		return &stubRows{cols: []string{"area_code"}, mode: c.mode}, nil
	default:
		return nil, fmt.Errorf("stub: unexpected query %q", query)
	}
}

type stubRows struct {
	cols []string
	mode stubMode
	done bool
}

func (r *stubRows) Columns() []string { return r.cols }
func (r *stubRows) Close() error      { return nil }

// errStubRowsIteration is the sentinel that surfaces through sql.Rows.Err()
// in stubModeRowsErr — never io.EOF, which the sql package treats as a
// normal end of rows rather than a failure.
var errStubRowsIteration = errors.New("stub: row iteration failed")

func (r *stubRows) Next(dest []driver.Value) error {
	if r.mode == stubModeRowsErr {
		return errStubRowsIteration
	}
	// stubModeScanErr: yield exactly one row whose first value is a type no
	// *string destination can hold, then end normally. Production code
	// returns on the Scan error before ever calling Next again, so this
	// io.EOF is defensive, not exercised by the current tests.
	if r.done {
		return io.EOF
	}
	r.done = true
	for i := range dest {
		dest[i] = int64(0)
	}
	dest[0] = struct{}{} // unconvertible to string
	return nil
}
