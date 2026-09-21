package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// maxSyntaxonAncestors is the number of STEPS from the deepest rank to the
// root: alliance -> order -> class -> formation is three steps and four nodes.
// The name says "ancestors", not "depth", because confusing 3 with 4 is
// otherwise a matter of time. A fourth step means a cycle in the parent_id
// graph or a further rank; either is an index defect and is reported with the
// id that triggered it instead of being experienced as an endless loop.
const maxSyntaxonAncestors = 3

// scanSyntaxa drains rows whose SELECT names the nine syntaxon columns in
// exactly this order. One scan site instead of four: a future tenth column
// must not reach three readers and be forgotten in the fourth.
func scanSyntaxa(rows *sql.Rows) ([]domain.Syntaxon, error) {
	out := []domain.Syntaxon{}
	for rows.Next() {
		var s domain.Syntaxon
		if err := rows.Scan(&s.ID, &s.Rank, &s.Name, &s.Author, &s.ParentID,
			&s.AltCode, &s.Source, &s.ParentProvenance, &s.LifeFormGroup); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Syntaxon reads, split out of read.go so the per-file complexity ratchet
// (scripts/codecharta-ratchet.py) has somewhere to grow the syntaxa-hierarchy
// feature's own reads without crowding the habitat-type/species reads that
// were already there — same rationale as query.go's earlier split into
// label.go: the sum moves with the code, so a cohesive split satisfies the
// gate honestly instead of asking for a raised baseline.

// Syntaxon returns one vegetation unit, or output.ErrNotFound.
func (d *DB) Syntaxon(ctx context.Context, id string) (domain.Syntaxon, error) {
	s := domain.Syntaxon{ID: id}
	row := d.QueryRowContext(ctx,
		`SELECT rank, name, author, parent_id, alt_code, source, parent_provenance, life_form_group
		 FROM syntaxon WHERE id = ?`, id)
	if err := row.Scan(&s.Rank, &s.Name, &s.Author, &s.ParentID, &s.AltCode, &s.Source,
		&s.ParentProvenance, &s.LifeFormGroup); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Syntaxon{}, fmt.Errorf("sqlite: syntaxon %q: %w", id, output.ErrNotFound)
		}
		return domain.Syntaxon{}, fmt.Errorf("sqlite: querying syntaxon %q: %w", id, err)
	}
	return s, nil
}

// Syntaxa returns the vegetation units linked to a habitat type.
func (d *DB) Syntaxa(ctx context.Context, key domain.HabitatTypeKey) ([]domain.Syntaxon, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT s.id, s.rank, s.name, s.author, s.parent_id, s.alt_code, s.source,
		        s.parent_provenance, s.life_form_group
		 FROM habitat_type_syntaxon l JOIN syntaxon s ON s.id = l.syntaxon_id
		 WHERE l.typology_id = ? AND l.code = ?
		 ORDER BY s.id`,
		string(key.Typology), key.Code)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying syntaxa of %s: %w", key, err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.Syntaxon{}
	for rows.Next() {
		var s domain.Syntaxon
		if err := rows.Scan(&s.ID, &s.Rank, &s.Name, &s.Author, &s.ParentID, &s.AltCode, &s.Source,
			&s.ParentProvenance, &s.LifeFormGroup); err != nil {
			return nil, fmt.Errorf("sqlite: scanning syntaxa of %s: %w", key, err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading syntaxa of %s: %w", key, err)
	}
	return out, nil
}

// AllSyntaxa returns every vegetation unit the index holds, in id order. Used
// by the FloraVeg hierarchy-matching pass to find every already-ingested
// EUNIS alliance to match its own names against.
func (d *DB) AllSyntaxa(ctx context.Context) ([]domain.Syntaxon, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, rank, name, author, parent_id, alt_code, source, parent_provenance, life_form_group
		 FROM syntaxon ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading all syntaxa: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.Syntaxon{}
	for rows.Next() {
		var s domain.Syntaxon
		if err := rows.Scan(&s.ID, &s.Rank, &s.Name, &s.Author, &s.ParentID, &s.AltCode, &s.Source,
			&s.ParentProvenance, &s.LifeFormGroup); err != nil {
			return nil, fmt.Errorf("sqlite: scanning syntaxon: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading all syntaxa: %w", err)
	}
	return out, nil
}

// SyntaxonChildren returns the direct children of parentID, ordered by id. It
// runs over idx_syntaxon_parent, so a navigation step needs no table scan.
//
// parentID is never empty in the serving path: the route /v1/syntaxon/{id}
// cannot match a blank segment. An empty parentID would legitimately return
// the formations — that is what GET /v1/syntaxa answers, through SyntaxaByRank.
func (d *DB) SyntaxonChildren(ctx context.Context, parentID string) ([]domain.Syntaxon, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, rank, name, author, parent_id, alt_code, source, parent_provenance,
		        life_form_group
		 FROM syntaxon WHERE parent_id = ? ORDER BY id`, parentID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying children of syntaxon %q: %w", parentID, err)
	}
	defer func() { _ = rows.Close() }()

	out, err := scanSyntaxa(rows)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading children of syntaxon %q: %w", parentID, err)
	}
	return out, nil
}

// HabitatTypeCountForSyntaxon counts the edges of exactly this syntaxon without
// loading them. Not the descendants' edges: habitat_type_syntaxon links
// alliances, so a class would report 0 either way — but only this reading makes
// the 0 an honest statement instead of a wrong aggregate.
func (d *DB) HabitatTypeCountForSyntaxon(ctx context.Context, syntaxonID string) (int, error) {
	var n int
	row := d.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM habitat_type_syntaxon WHERE syntaxon_id = ?`, syntaxonID)
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("sqlite: counting habitat types of syntaxon %q: %w", syntaxonID, err)
	}
	return n, nil
}

// SyntaxonAncestors walks parent_id to the root, OUTERMOST first (formation,
// then class, then order). Empty for a formation.
//
// Three failure modes, deliberately told apart: an unknown start id wraps
// output.ErrNotFound (a missing answer, 404); a parent_id pointing at a row
// that does not exist does NOT (an index defect, 500 — bridging it would serve
// a shortened breadcrumb trail as if it were complete); and more than
// maxSyntaxonAncestors steps means a cycle or a further rank, reported with the
// id that triggered it instead of looping.
func (d *DB) SyntaxonAncestors(ctx context.Context, id string) ([]domain.Syntaxon, error) {
	current, err := d.Syntaxon(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []domain.Syntaxon{}
	for steps := 0; current.ParentID != ""; steps++ {
		if steps == maxSyntaxonAncestors {
			return nil, fmt.Errorf(
				"sqlite: syntaxon %q has more than %d ancestors: the parent_id graph is cyclic or carries a further rank",
				id, maxSyntaxonAncestors)
		}
		parent, perr := d.Syntaxon(ctx, current.ParentID)
		if perr != nil {
			if errors.Is(perr, output.ErrNotFound) {
				return nil, fmt.Errorf("sqlite: syntaxon %q has parent_id %q, which no row carries",
					current.ID, current.ParentID)
			}
			return nil, fmt.Errorf("sqlite: walking the ancestors of %q: %w", id, perr)
		}
		out = append(out, parent)
		current = parent
	}
	slices.Reverse(out)
	return out, nil
}

// SyntaxonIDsByAltCode maps the EEA alt code to the syntaxon id. The syntaxa
// ingest uses it to resolve the habitat-type edges of the EEA source onto
// the FloraVeg primary codes. Rows without an alt code are absent from the
// map.
func (d *DB) SyntaxonIDsByAltCode(ctx context.Context) (map[string]string, error) {
	rows, err := d.QueryContext(ctx, `SELECT alt_code, id FROM syntaxon WHERE alt_code <> ''`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading syntaxon alt codes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var alt, id string
		if err := rows.Scan(&alt, &id); err != nil {
			return nil, fmt.Errorf("sqlite: scanning syntaxon alt code: %w", err)
		}
		out[alt] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading syntaxon alt codes: %w", err)
	}
	return out, nil
}

// HabitatTypeKeysForSyntaxon returns the habitat types a syntaxon is linked to.
func (d *DB) HabitatTypeKeysForSyntaxon(ctx context.Context, syntaxonID string) ([]domain.HabitatTypeKey, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT typology_id, code FROM habitat_type_syntaxon WHERE syntaxon_id = ?
		 ORDER BY typology_id, code`,
		syntaxonID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying habitat types of syntaxon %q: %w", syntaxonID, err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.HabitatTypeKey{}
	for rows.Next() {
		var typology, code string
		if err := rows.Scan(&typology, &code); err != nil {
			return nil, fmt.Errorf("sqlite: scanning habitat types of syntaxon %q: %w", syntaxonID, err)
		}
		out = append(out, domain.HabitatTypeKey{Typology: domain.TypologyID(typology), Code: code})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading habitat types of syntaxon %q: %w", syntaxonID, err)
	}
	return out, nil
}
