package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
)

// SpeciesRoles returns a habitat type's species. role filters when non-empty;
// the empty string means every role. Both variants are one static statement —
// the filter is a bound parameter, not appended SQL.
func (d *DB) SpeciesRoles(ctx context.Context, key domain.HabitatTypeKey, role string) ([]domain.SpeciesRole, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT concept_id, verbatim_name, role, fidelity, constancy, provenance, derived_from FROM species_role
		 WHERE typology_id = ? AND code = ? AND (? = '' OR role = ?)
		 ORDER BY role, verbatim_name`,
		string(key.Typology), key.Code, role, role)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying species of %s: %w", key, err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.SpeciesRole{}
	for rows.Next() {
		r := domain.SpeciesRole{Key: key}
		if err := scanSpeciesRole(rows, &r); err != nil {
			return nil, fmt.Errorf("sqlite: scanning species of %s: %w", key, err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading species of %s: %w", key, err)
	}
	return out, nil
}

// SpeciesRolesByConcept returns every role a resolved concept plays. An empty
// result means the index knows no such concept: a concept id exists here only
// because a species-role row carries it.
func (d *DB) SpeciesRolesByConcept(ctx context.Context, conceptID string) ([]domain.SpeciesRole, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT typology_id, code, concept_id, verbatim_name, role, fidelity, constancy, provenance, derived_from
		 FROM species_role WHERE concept_id = ?
		 ORDER BY typology_id, code, role`,
		conceptID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying habitat types of concept %q: %w", conceptID, err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.SpeciesRole{}
	for rows.Next() {
		var typology string
		var r domain.SpeciesRole
		var concept, derivedFrom sql.NullString
		var fidelity, constancy sql.NullFloat64
		if err := rows.Scan(&typology, &r.Key.Code, &concept, &r.VerbatimName, &r.Role, &fidelity, &constancy,
			&r.Provenance, &derivedFrom); err != nil {
			return nil, fmt.Errorf("sqlite: scanning habitat types of concept %q: %w", conceptID, err)
		}
		r.Key.Typology = domain.TypologyID(typology)
		applySpeciesNullables(&r, concept, derivedFrom, fidelity, constancy)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading habitat types of concept %q: %w", conceptID, err)
	}
	return out, nil
}

// scanSpeciesRole reads the columns shared by the species queries. The
// nullables stay nullable all the way into the domain: an unresolved name must
// arrive as a nil ConceptID, never as an empty string, and a missing fidelity
// must not become 0.0.
func scanSpeciesRole(rows *sql.Rows, r *domain.SpeciesRole) error {
	var concept, derivedFrom sql.NullString
	var fidelity, constancy sql.NullFloat64
	if err := rows.Scan(&concept, &r.VerbatimName, &r.Role, &fidelity, &constancy, &r.Provenance, &derivedFrom); err != nil {
		return err
	}
	applySpeciesNullables(r, concept, derivedFrom, fidelity, constancy)
	return nil
}

func applySpeciesNullables(r *domain.SpeciesRole, concept, derivedFrom sql.NullString, fidelity, constancy sql.NullFloat64) {
	if concept.Valid {
		id := concept.String
		r.ConceptID = &id
	}
	if fidelity.Valid {
		f := fidelity.Float64
		r.Fidelity = &f
	}
	if constancy.Valid {
		c := constancy.Float64
		r.Constancy = &c
	}
	if derivedFrom.Valid {
		df := derivedFrom.String
		r.DerivedFrom = &df
	}
}
