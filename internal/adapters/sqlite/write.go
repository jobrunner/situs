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
		`INSERT INTO syntaxon (id, rank, name, parent_id)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   rank=excluded.rank, name=excluded.name, parent_id=excluded.parent_id`,
		s.ID, s.Rank, s.Name, s.ParentID)
	if err != nil {
		return fmt.Errorf("sqlite: upserting syntaxon %s: %w", s.ID, err)
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

func (t *ingestTx) UpsertSpeciesRole(r domain.SpeciesRole) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO species_role (typology_id, code, concept_id, verbatim_name, role, fidelity, constancy)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(typology_id, code, verbatim_name, role) DO UPDATE SET
		   concept_id=excluded.concept_id, fidelity=excluded.fidelity, constancy=excluded.constancy`,
		string(r.Key.Typology), r.Key.Code, r.ConceptID, r.VerbatimName, r.Role, r.Fidelity, r.Constancy)
	if err != nil {
		return fmt.Errorf("sqlite: upserting species role %q in %s: %w", r.VerbatimName, r.Key, err)
	}
	return nil
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
