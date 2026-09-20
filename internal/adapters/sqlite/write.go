package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
)

// ingestTx is one atomic ingest run: every Upsert method is idempotent so a
// repinned artifact can simply be re-ingested.
type ingestTx struct {
	ctx context.Context
	tx  *sql.Tx
}

func (t *ingestTx) UpsertTypology(ty domain.Typology) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO habitat_typology (id, scheme, version, name, source_ref)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   scheme=excluded.scheme, version=excluded.version,
		   name=excluded.name, source_ref=excluded.source_ref`,
		string(ty.ID), ty.Scheme, ty.Version, ty.Name, ty.SourceRef)
	if err != nil {
		return fmt.Errorf("sqlite: upserting typology %s: %w", ty.ID, err)
	}
	return nil
}

func (t *ingestTx) UpsertHabitatType(h domain.HabitatType) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO habitat_type (typology_id, code, level, name_en, parent_code, priority)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(typology_id, code) DO UPDATE SET
		   level=excluded.level, name_en=excluded.name_en,
		   parent_code=excluded.parent_code, priority=excluded.priority`,
		string(h.Key.Typology), h.Key.Code, h.Level, h.NameEN, h.ParentCode, h.Priority)
	if err != nil {
		return fmt.Errorf("sqlite: upserting habitat type %s: %w", h.Key, err)
	}
	return nil
}

func (t *ingestTx) UpsertCrosswalk(c domain.Crosswalk) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO habitat_type_crosswalk (from_typology, from_code, to_typology, to_code, qualifier)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(from_typology, from_code, to_typology, to_code) DO UPDATE SET
		   qualifier=excluded.qualifier`,
		string(c.From.Typology), c.From.Code, string(c.To.Typology), c.To.Code, string(c.Qualifier))
	if err != nil {
		return fmt.Errorf("sqlite: upserting crosswalk %s -> %s: %w", c.From, c.To, err)
	}
	return nil
}

func (t *ingestTx) UpsertSyntaxon(s domain.Syntaxon) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO syntaxon (id, rank, name, author, parent_id)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   rank=excluded.rank, name=excluded.name, author=excluded.author, parent_id=excluded.parent_id`,
		s.ID, s.Rank, s.Name, s.Author, s.ParentID)
	if err != nil {
		return fmt.Errorf("sqlite: upserting syntaxon %s: %w", s.ID, err)
	}
	return nil
}

// UpsertSyntaxonAuthor sets name and author on an already-upserted syntaxon.
// name always updates — a match always has a real FloraVeg name to give. An
// empty parentID leaves the stored parent_id untouched — a repeated
// hierarchy-ingest pass without a fresh match must not erase a previous one.
//
// This method enriches a row that must already exist (its caller only ever
// passes an id read back from the index moments earlier); an id the index
// does not carry means the index changed under the ingest or the caller
// drifted out of sync with its own read, and is reported as an error rather
// than silently doing nothing.
//
// Existence is checked with a dedicated SELECT rather than the UPDATE's own
// RowsAffected(): SQLite counts a row as "changed" once it matches the WHERE
// clause, even when every assigned value equals what was already stored —
// which a repeated hierarchy-ingest pass (the very idempotency this method's
// callers rely on) hits routinely. Trusting RowsAffected()==0 as "not found"
// would misreport that ordinary no-op re-run as a missing row.
func (t *ingestTx) UpsertSyntaxonAuthor(id, name, author, parentID string) error {
	var exists bool
	if err := t.tx.QueryRowContext(t.ctx,
		`SELECT EXISTS(SELECT 1 FROM syntaxon WHERE id = ?)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("sqlite: checking syntaxon %s exists: %w", id, err)
	}
	if !exists {
		return fmt.Errorf("sqlite: syntaxon %s not found for author update", id)
	}

	var err error
	if parentID == "" {
		_, err = t.tx.ExecContext(t.ctx,
			`UPDATE syntaxon SET name = ?, author = ? WHERE id = ?`, name, author, id)
		if err != nil {
			return fmt.Errorf("sqlite: setting name/author of syntaxon %s: %w", id, err)
		}
	} else {
		_, err = t.tx.ExecContext(t.ctx,
			`UPDATE syntaxon SET name = ?, author = ?, parent_id = ? WHERE id = ?`, name, author, parentID, id)
		if err != nil {
			return fmt.Errorf("sqlite: setting name/author/parent of syntaxon %s: %w", id, err)
		}
	}
	return nil
}

func (t *ingestTx) LinkSyntaxon(key domain.HabitatTypeKey, syntaxonID string) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO habitat_type_syntaxon (typology_id, code, syntaxon_id)
		 VALUES (?, ?, ?)
		 ON CONFLICT(typology_id, code, syntaxon_id) DO NOTHING`,
		string(key.Typology), key.Code, syntaxonID)
	if err != nil {
		return fmt.Errorf("sqlite: linking syntaxon %s to %s: %w", syntaxonID, key, err)
	}
	return nil
}

// normalizedProvenance falls back to the table default when a caller leaves
// Provenance unset — binding an empty string would otherwise bypass
// species_role's `DEFAULT 'observed'` and let an out-of-contract value (never
// "observed" or "derived_from_aggregate") into the index.
func normalizedProvenance(p string) string {
	if p == "" {
		return "observed"
	}
	return p
}

func (t *ingestTx) UpsertSpeciesRole(r domain.SpeciesRole) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO species_role (typology_id, code, concept_id, verbatim_name, role, fidelity, constancy, provenance, derived_from)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(typology_id, code, verbatim_name, role) DO UPDATE SET
		   concept_id=excluded.concept_id, fidelity=excluded.fidelity, constancy=excluded.constancy,
		   provenance=excluded.provenance, derived_from=excluded.derived_from`,
		string(r.Key.Typology), r.Key.Code, r.ConceptID, r.VerbatimName, r.Role, r.Fidelity, r.Constancy,
		normalizedProvenance(r.Provenance), r.DerivedFrom)
	if err != nil {
		return fmt.Errorf("sqlite: upserting species role %q in %s: %w", r.VerbatimName, r.Key, err)
	}
	return nil
}

// UpsertDerivedSpeciesRole writes r only if no row exists yet for its
// (typology, code, verbatim_name, role) key — an explicit species_roles.csv
// row, ingested first, always wins over a later aggregate-derived one.
func (t *ingestTx) UpsertDerivedSpeciesRole(r domain.SpeciesRole) (bool, error) {
	res, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO species_role (typology_id, code, concept_id, verbatim_name, role, fidelity, constancy, provenance, derived_from)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(typology_id, code, verbatim_name, role) DO NOTHING`,
		string(r.Key.Typology), r.Key.Code, r.ConceptID, r.VerbatimName, r.Role, r.Fidelity, r.Constancy,
		normalizedProvenance(r.Provenance), r.DerivedFrom)
	if err != nil {
		return false, fmt.Errorf("sqlite: upserting derived species role %q in %s: %w", r.VerbatimName, r.Key, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: reading rows affected for derived species role %q in %s: %w", r.VerbatimName, r.Key, err)
	}
	return affected == 0, nil
}

func (t *ingestTx) UpsertLocalization(l domain.Localization) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO localization (entity_type, entity_key, lang, field, value, source, provenance, derived_from)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(entity_type, entity_key, lang, field, source) DO UPDATE SET
		   value=excluded.value, provenance=excluded.provenance, derived_from=excluded.derived_from`,
		l.EntityType, l.EntityKey, l.Lang, l.Field, l.Value, l.Source, l.Provenance, l.DerivedFrom)
	if err != nil {
		return fmt.Errorf("sqlite: upserting localization %s/%s/%s: %w", l.EntityType, l.EntityKey, l.Field, err)
	}
	return nil
}

func (t *ingestTx) UpsertDistribution(conceptID string, a domain.Area) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO species_distribution (concept_id, area_scheme, area_code)
		 VALUES (?, ?, ?)
		 ON CONFLICT(concept_id, area_scheme, area_code) DO NOTHING`,
		conceptID, a.Scheme, a.Code)
	if err != nil {
		return fmt.Errorf("sqlite: upserting distribution %s for %s: %w", a, conceptID, err)
	}
	return nil
}

// UpsertArea writes one area name. Idempotent like every other Upsert here,
// and a repinned artifact whose name changed replaces the old one.
func (t *ingestTx) UpsertArea(a domain.NamedArea) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO area (area_scheme, area_code, name_en)
		 VALUES (?, ?, ?)
		 ON CONFLICT(area_scheme, area_code) DO UPDATE SET name_en = excluded.name_en`,
		a.Scheme, a.Code, a.NameEN)
	if err != nil {
		return fmt.Errorf("sqlite: upserting area %s: %w", a.Area, err)
	}
	return nil
}

// UpsertDescription writes one habitat type's prose description. Idempotent
// like every other Upsert here; a repinned factsheet replaces the text.
func (t *ingestTx) UpsertDescription(d domain.HabitatDescription) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO habitat_description (typology_id, code, description_en, source, provenance)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(typology_id, code) DO UPDATE SET
		   description_en = excluded.description_en, source = excluded.source,
		   provenance = excluded.provenance`,
		string(d.Key.Typology), d.Key.Code, d.TextEN, d.Source, d.Provenance)
	if err != nil {
		return fmt.Errorf("sqlite: upserting description of %s: %w", d.Key, err)
	}
	return nil
}

func (t *ingestTx) Commit() error {
	if err := t.tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: committing ingest transaction: %w", err)
	}
	return nil
}

func (t *ingestTx) Rollback() error {
	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("sqlite: rolling back ingest transaction: %w", err)
	}
	return nil
}
