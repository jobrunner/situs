package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

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
