package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
)

// Rank-filtered syntaxa reads, split out of read_syntaxon.go so the
// per-file complexity ratchet (scripts/codecharta-ratchet.py) has somewhere
// to grow the GET /v1/syntaxa filter without crowding the id/ancestor/
// children reads that were already there — same rationale as query.go's
// earlier split into label.go: the sum moves with the code, so a cohesive
// split satisfies the gate honestly instead of asking for a raised baseline.

// SyntaxaByRank returns every syntaxon of rank, ordered by id. A non-empty
// lifeFormGroup keeps only those whose reachable formation carries that group.
func (d *DB) SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string) ([]domain.Syntaxon, error) {
	rows, err := d.syntaxaByRankRows(ctx, rank, lifeFormGroup)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying syntaxa of rank %q: %w", rank, err)
	}
	defer func() { _ = rows.Close() }()

	out, err := scanSyntaxa(rows)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading syntaxa of rank %q: %w", rank, err)
	}
	return out, nil
}

// maxSyntaxonAncestors is the number of STEPS from the deepest rank to the
// root: alliance -> order -> class -> formation is three steps and four nodes.
// The name says "ancestors", not "depth", because confusing 3 with 4 is
// otherwise a matter of time. A fourth step means a cycle in the parent_id
// graph or a further rank; either is an index defect and is reported with the
// id that triggered it instead of being experienced as an endless loop.
//
// It guards two seams at once: SyntaxonAncestors' walk-loop bound in
// read_syntaxon.go, and the `u.steps < 3` literal in
// syntaxaByRankRowsWithGroupSQL below — the two places that detect the same
// defect. It lives here, next to the SQL literal, so the two cannot drift
// apart unnoticed; TestSyntaxaByRankStepBoundMatchesMaxSyntaxonAncestors is
// the guard that actually enforces it, since gosec G201 forbids building the
// literal from this constant.
const maxSyntaxonAncestors = 3

// syntaxaByRankRowsWithGroupSQL is the recursive-CTE statement
// syntaxaByRankRows issues when lifeFormGroup is non-empty. It is a package
// constant — not inlined at the call site — so a test can read the same
// string the query runs and check its step bound against maxSyntaxonAncestors
// (see TestSyntaxaByRankStepBoundMatchesMaxSyntaxonAncestors): a %d built with
// fmt.Sprintf from a Go int and compared against a fixed string is not the SQL
// concatenation gosec G201 forbids, since nothing here is assembled INTO the
// query that QueryContext runs — the query stays this exact literal.
//
// life_form_group is stored on formation rows alone, so this variant has to
// join upwards, and the depth differs per rank (an alliance three steps, a
// class one). Building that depth from the rank string would be exactly the
// SQL string assembly gosec G201 forbids, and a statement per rank would be
// four near-identical literals that go stale the moment a further rank becomes
// a data row. One recursive CTE covers every depth instead.
//
// The step bound is not decoration: without it a cycle in parent_id would make
// this statement recurse until memory runs out — a hang rather than an error
// (see TestSyntaxaByRankLaeuftBeiEinemZykelNichtEndlos). It is a static literal
// in the statement, not a bound parameter: a value built from Go input at
// query time is what gosec G201 forbids, and this number is never runtime
// input — it is the same constant SyntaxonAncestors uses to detect the same
// defect, guarded to match by the test named above.
const syntaxaByRankRowsWithGroupSQL = `WITH RECURSIVE up(id, root, steps) AS (
	   SELECT id, id, 0 FROM syntaxon
	   UNION ALL
	   SELECT u.id, s.parent_id, u.steps + 1
	   FROM up u JOIN syntaxon s ON s.id = u.root
	   WHERE s.parent_id <> '' AND u.steps < 3
	 )
	 SELECT s.id, s.rank, s.name, s.author, s.parent_id, s.eea_code, s.source,
	        s.parent_provenance, s.life_form_group
	 FROM syntaxon s
	 JOIN up ON up.id = s.id
	 JOIN syntaxon f ON f.id = up.root AND f.rank = 'formation'
	 WHERE s.rank = ? AND f.life_form_group = ?
	 ORDER BY s.id`

// syntaxaByRankRows picks between TWO static statements — never one assembled
// from the rank value. The remaining placeholders bind in the order they
// appear (rank, group).
func (d *DB) syntaxaByRankRows(ctx context.Context, rank, lifeFormGroup string) (*sql.Rows, error) {
	if lifeFormGroup == "" {
		return d.QueryContext(ctx,
			`SELECT id, rank, name, author, parent_id, eea_code, source, parent_provenance,
			        life_form_group
			 FROM syntaxon WHERE rank = ? ORDER BY id`, rank)
	}
	return d.QueryContext(ctx, syntaxaByRankRowsWithGroupSQL, rank, lifeFormGroup)
}

// SyntaxonRanks lists the distinct ranks the index carries, sorted. The read
// side validates ?rank= against THIS and not against a wired list: the schema
// deliberately has no CHECK on rank so a further rank can be a data row, and a
// fixed enum one layer up would have taken that freedom back and made a new
// rank unfindable.
func (d *DB) SyntaxonRanks(ctx context.Context) ([]string, error) {
	rows, err := d.QueryContext(ctx, `SELECT DISTINCT rank FROM syntaxon ORDER BY rank`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: listing syntaxon ranks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []string{}
	for rows.Next() {
		var rank string
		if err := rows.Scan(&rank); err != nil {
			return nil, fmt.Errorf("sqlite: scanning syntaxon rank: %w", err)
		}
		out = append(out, rank)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading syntaxon ranks: %w", err)
	}
	return out, nil
}
