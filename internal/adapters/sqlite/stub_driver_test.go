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
	// stubModeCheckpointedThenFails answers PRAGMA wal_checkpoint like a
	// quiet database and then fails the journal_mode switch — the one
	// FinalizeForServing branch that needs the first statement to succeed.
	stubModeCheckpointedThenFails
	// stubModeQueryErr fails every query outright, which is how a statement
	// that cannot even be prepared reaches its caller.
	stubModeQueryErr
	// stubModeFirstSyntaxonLookupThenFails answers the first direct-id
	// syntaxon lookup (SyntaxonAncestors' own starting point) with one row
	// carrying a non-empty parent_id, then fails every further one with a
	// plain query error. It is the deterministic way to reach
	// SyntaxonAncestors' inner-walk failure branch — the outer lookup must
	// succeed and a later one must fail, without racing a context
	// cancellation against real row timing (see the file comment above).
	stubModeFirstSyntaxonLookupThenFails
	// stubModeSyntaxonDistributionRowsErr answers SyntaxonDistribution's
	// coverage check with zero rows (a clean "not covered") and then fails
	// the distribution-rows query's row iteration — the two queries are not
	// distinguishable by text alone under a mode that fails every query, so
	// this follows the stubModeFirstSyntaxonLookupThenFails pattern instead.
	stubModeSyntaxonDistributionRowsErr
	// stubModeSyntaxonDistributionScanErr is the same, but the
	// distribution-rows query yields one row a string destination cannot
	// scan.
	stubModeSyntaxonDistributionScanErr
)

// isSyntaxonDistributionMode reports whether mode is one of the two above,
// which both need the coverage query answered cleanly before failing the
// distribution-rows query that follows it.
func isSyntaxonDistributionMode(mode stubMode) bool {
	return mode == stubModeSyntaxonDistributionRowsErr || mode == stubModeSyntaxonDistributionScanErr
}

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

type stubConn struct {
	mode stubMode
	// syntaxonLookups counts direct-id syntaxon lookups, used only by
	// stubModeFirstSyntaxonLookupThenFails to tell the outer call from the
	// ones the ancestor walk makes afterward.
	syntaxonLookups int
}

func (c *stubConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("stub: Prepare is not implemented, QueryContext is used directly")
}
func (c *stubConn) Close() error { return nil }
func (c *stubConn) Begin() (driver.Tx, error) {
	return nil, errors.New("stub: Begin is not implemented")
}

// stubQueryRule maps a substring found in a query's text to the column set a
// stubRows answering it must carry — the table stubColumnRules below is
// checked in order, first match wins.
type stubQueryRule struct {
	contains string
	cols     []string
}

var stubColumnRules = []stubQueryRule{
	{"LEFT JOIN area", []string{"area_code", "name_en"}},
	{"habitat_type_crosswalk", []string{"from_typology", "from_code", "to_typology", "to_code", "qualifier"}},
	{"FROM localization", []string{"value", "source", "provenance", "derived_from"}},
	{"FROM species_role WHERE concept_id", []string{
		"typology_id", "code", "concept_id", "verbatim_name", "role", "fidelity", "constancy",
	}},
	{"FROM species_role", []string{"concept_id", "verbatim_name", "role", "fidelity", "constancy"}},
	{"JOIN syntaxon", []string{"id", "rank", "name", "author", "parent_id"}},
	{"WHERE parent_id = ?", []string{
		"id", "rank", "name", "author", "parent_id", "eea_code", "source", "parent_provenance", "life_form_group",
	}},
	// "WHERE rank = ?" is SyntaxaByRank's unfiltered statement; the group-filtered
	// recursive CTE already matches the "JOIN syntaxon" rule above, and its final
	// SELECT reads "WHERE s.rank = ?" — a different string, no collision.
	{"WHERE rank = ?", []string{
		"id", "rank", "name", "author", "parent_id", "eea_code", "source", "parent_provenance", "life_form_group",
	}},
	{"DISTINCT rank FROM syntaxon", []string{"rank"}},
	{"WHERE eea_code = ?", []string{
		"id", "rank", "name", "author", "parent_id", "eea_code", "source", "parent_provenance", "life_form_group",
	}},
	{"FROM syntaxon ORDER BY id", []string{"id", "rank", "name", "author", "parent_id"}},
	{"eea_code, id FROM syntaxon", []string{"eea_code", "id"}},
	{"FROM habitat_type_syntaxon", []string{"typology_id", "code"}},
	{"concept_id, area_code FROM species_distribution", []string{"concept_id", "area_code"}},
	{"DISTINCT area_code FROM species_distribution", []string{"area_code"}},
	{"1 FROM syntaxon_distribution_coverage", []string{"1"}},
	{"syntaxon_id FROM syntaxon_distribution_coverage", []string{"syntaxon_id"}},
	{"syntaxon_id, occurrence FROM syntaxon_distribution", []string{"syntaxon_id", "occurrence"}},
	{"area_code, occurrence FROM syntaxon_distribution", []string{"area_code", "occurrence"}},
	{"DISTINCT area_code FROM syntaxon_distribution", []string{"area_code"}},
	{"FROM trait_value", []string{"vocab", "vocab_version", "dim", "value", "niche_width", "n_systems"}},
	{"DISTINCT vocab FROM trait_vocabulary", []string{"vocab"}},
}

// QueryContext picks the column set by matching the table name in the query
// text — every list read queries one table (or one join), so this is enough to
// serve them all without needing a real SQL engine.
func (c *stubConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if rows, err := c.openTimeRows(query); rows != nil || err != nil {
		return rows, err
	}
	if c.mode == stubModeFirstSyntaxonLookupThenFails && strings.Contains(query, "FROM syntaxon WHERE id = ?") {
		c.syntaxonLookups++
		if c.syntaxonLookups == 1 {
			return &stubSyntaxonRow{}, nil
		}
		return nil, errStubSyntaxonWalk
	}
	if isSyntaxonDistributionMode(c.mode) {
		if rows, ok := c.syntaxonDistributionRows(query); ok {
			return rows, nil
		}
	}
	for _, rule := range stubColumnRules {
		if strings.Contains(query, rule.contains) {
			return &stubRows{cols: rule.cols, mode: c.mode}, nil
		}
	}
	return nil, fmt.Errorf("stub: unexpected query %q", query)
}

// syntaxonDistributionRows answers the two queries SyntaxonDistribution
// issues, under the two modes built for exercising its second query's error
// paths: the coverage check gets a clean "no row" answer (so Covered stays
// false and the code proceeds to the distribution-rows query), and only that
// second query then fails. ok is false for any other query, letting the
// generic dispatch above handle it.
func (c *stubConn) syntaxonDistributionRows(query string) (driver.Rows, bool) {
	if strings.Contains(query, "syntaxon_distribution_coverage") {
		return &stubZeroRows{cols: []string{"1"}}, true
	}
	if strings.Contains(query, "area_code, occurrence FROM syntaxon_distribution") {
		inner := stubModeRowsErr
		if c.mode == stubModeSyntaxonDistributionScanErr {
			inner = stubModeScanErr
		}
		return &stubRows{cols: []string{"area_code", "occurrence"}, mode: inner}, true
	}
	return nil, false
}

// stubZeroRows answers a query with zero rows: Next() reports io.EOF on the
// very first call, which is how QueryRowContext's Scan surfaces sql.ErrNoRows.
type stubZeroRows struct{ cols []string }

func (r *stubZeroRows) Columns() []string           { return r.cols }
func (r *stubZeroRows) Close() error                { return nil }
func (r *stubZeroRows) Next(_ []driver.Value) error { return io.EOF }

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

// openTimeRows answers the statements the two openers run — the schema check's
// table list and, in the mode built for it, FinalizeForServing's two pragmas.
// (nil, nil) means "not mine": the query falls through to the read-side switch.
func (c *stubConn) openTimeRows(query string) (driver.Rows, error) {
	if c.mode == stubModeQueryErr {
		return nil, errStubQuery
	}
	if strings.Contains(query, "sqlite_master") {
		return &stubRows{cols: []string{"name"}, mode: c.mode}, nil
	}
	return c.finalizeRows(query)
}

// finalizeRows answers the two statements FinalizeForServing runs, but only in
// the mode built for it.
func (c *stubConn) finalizeRows(query string) (driver.Rows, error) {
	if c.mode != stubModeCheckpointedThenFails {
		return nil, nil
	}
	if strings.Contains(query, "wal_checkpoint") {
		return &stubCheckpointRows{}, nil
	}
	if strings.Contains(query, "journal_mode") {
		return nil, errStubJournalMode
	}
	return nil, nil
}

// errStubQuery is what a database that cannot run the statement at all answers.
var errStubQuery = errors.New("stub: query failed")

// errStubSyntaxonWalk is what a syntaxon lookup answers once
// stubModeFirstSyntaxonLookupThenFails has already served its one good row.
var errStubSyntaxonWalk = errors.New("stub: syntaxon lookup failed")

// stubSyntaxonRow is the one successful row
// stubModeFirstSyntaxonLookupThenFails serves: a syntaxon whose parent_id is
// non-empty, so SyntaxonAncestors' loop makes a second lookup — the one this
// mode then fails.
type stubSyntaxonRow struct{ done bool }

func (r *stubSyntaxonRow) Columns() []string {
	return []string{"rank", "name", "author", "parent_id", "eea_code", "source", "parent_provenance", "life_form_group"}
}
func (r *stubSyntaxonRow) Close() error { return nil }
func (r *stubSyntaxonRow) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	vals := []string{"alliance", "Stub", "", "STUB-PARENT", "", "", "", ""}
	for i := range dest {
		dest[i] = vals[i]
	}
	return nil
}

// errStubJournalMode is what a database that cannot leave WAL answers.
var errStubJournalMode = errors.New("stub: journal_mode switch failed")

// stubCheckpointRows is one quiet PRAGMA wal_checkpoint answer: nothing busy,
// nothing left in the log.
type stubCheckpointRows struct{ done bool }

func (r *stubCheckpointRows) Columns() []string { return []string{"busy", "log", "checkpointed"} }
func (r *stubCheckpointRows) Close() error      { return nil }
func (r *stubCheckpointRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	for i := range dest {
		dest[i] = int64(0)
	}
	return nil
}
