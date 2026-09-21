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
		`INSERT INTO syntaxon (id, rank, name, author, parent_id, alt_code, source,
		                       parent_provenance, life_form_group)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   rank = excluded.rank, name = excluded.name, author = excluded.author,
		   parent_id = excluded.parent_id, alt_code = excluded.alt_code,
		   source = excluded.source, parent_provenance = excluded.parent_provenance,
		   life_form_group = excluded.life_form_group`,
		s.ID, s.Rank, s.Name, s.Author, s.ParentID, s.AltCode, s.Source,
		s.ParentProvenance, s.LifeFormGroup)
	if err != nil {
		return fmt.Errorf("sqlite: upserting syntaxon %s: %w", s.ID, err)
	}
	return nil
}

// SetSyntaxonParent sets the parent and its provenance on an already-written
// row. Separate from UpsertSyntaxon: the parent of the 16 EEA-only units is
// only known once the FloraVeg rows and their siblings are in the index.
func (t *ingestTx) SetSyntaxonParent(id, parentID, provenance string) error {
	_, err := t.tx.ExecContext(t.ctx,
		`UPDATE syntaxon SET parent_id = ?, parent_provenance = ? WHERE id = ?`,
		parentID, provenance, id)
	if err != nil {
		return fmt.Errorf("sqlite: setting parent of syntaxon %s: %w", id, err)
	}
	return nil
}

// RelinkSyntaxon rewrites every habitat_type_syntaxon edge from syntaxon id
// from to to. Two statements, not one: an UPDATE would fail on the primary
// key if the habitat type already carries both edges (exactly the case for
// ASP-03/KC03). First add the target edges where they are missing, then
// remove the source edges.
func (t *ingestTx) RelinkSyntaxon(from, to string) error {
	if _, err := t.tx.ExecContext(t.ctx,
		`INSERT OR IGNORE INTO habitat_type_syntaxon (typology_id, code, syntaxon_id)
		 SELECT typology_id, code, ? FROM habitat_type_syntaxon WHERE syntaxon_id = ?`,
		to, from); err != nil {
		return fmt.Errorf("sqlite: relinking syntaxon %s to %s: %w", from, to, err)
	}
	if _, err := t.tx.ExecContext(t.ctx,
		`DELETE FROM habitat_type_syntaxon WHERE syntaxon_id = ?`, from); err != nil {
		return fmt.Errorf("sqlite: removing relinked syntaxon %s edges: %w", from, err)
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

// UpsertSyntaxonDistribution records one occurrence cell. Idempotent, and a
// repeated ingest overwrites the occurrence: the source is allowed to upgrade
// an uncertain record to a verified one.
func (t *ingestTx) UpsertSyntaxonDistribution(syntaxonID, scheme, code, occurrence string) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO syntaxon_distribution (syntaxon_id, area_scheme, area_code, occurrence)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(syntaxon_id, area_scheme, area_code) DO UPDATE SET
		   occurrence = excluded.occurrence`,
		syntaxonID, scheme, code, occurrence)
	if err != nil {
		return fmt.Errorf("sqlite: upserting syntaxon distribution %s/%s/%s: %w", syntaxonID, scheme, code, err)
	}
	return nil
}

// UpsertSyntaxonDistributionCoverage records that the source makes a
// statement about this syntaxon at all. Without it "occurs in no territory"
// is indistinguishable from "nobody looked".
func (t *ingestTx) UpsertSyntaxonDistributionCoverage(syntaxonID, scheme string) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT OR IGNORE INTO syntaxon_distribution_coverage (syntaxon_id, area_scheme)
		 VALUES (?, ?)`,
		syntaxonID, scheme)
	if err != nil {
		return fmt.Errorf("sqlite: upserting syntaxon distribution coverage %s/%s: %w", syntaxonID, scheme, err)
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
