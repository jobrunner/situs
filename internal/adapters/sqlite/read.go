package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// The read side of the index. Every statement is a static string with `?`
// placeholders; nothing here concatenates a value into SQL.

// Typology returns the registered classification system, or output.ErrNotFound.
// The read API needs this to tell an unknown typology (a malformed question)
// from an unknown code inside a known one (a missing answer).
func (d *DB) Typology(ctx context.Context, id domain.TypologyID) (domain.Typology, error) {
	t := domain.Typology{ID: id}
	row := d.QueryRowContext(ctx,
		`SELECT scheme, version, name, source_ref FROM habitat_typology WHERE id = ?`,
		string(id))
	if err := row.Scan(&t.Scheme, &t.Version, &t.Name, &t.SourceRef); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Typology{}, fmt.Errorf("sqlite: typology %s: %w", id, output.ErrNotFound)
		}
		return domain.Typology{}, fmt.Errorf("sqlite: querying typology %s: %w", id, err)
	}
	return t, nil
}

// Crosswalks returns every crosswalk touching key, in either direction, in the
// orientation it is stored in. The caller decides which end it asked about.
func (d *DB) Crosswalks(ctx context.Context, key domain.HabitatTypeKey) ([]domain.Crosswalk, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT from_typology, from_code, to_typology, to_code, qualifier
		 FROM habitat_type_crosswalk
		 WHERE (from_typology = ? AND from_code = ?) OR (to_typology = ? AND to_code = ?)
		 ORDER BY to_typology, to_code, from_typology, from_code`,
		string(key.Typology), key.Code, string(key.Typology), key.Code)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying crosswalks of %s: %w", key, err)
	}
	defer func() { _ = rows.Close() }()

	out := []domain.Crosswalk{}
	for rows.Next() {
		var fromTypology, toTypology, qualifier string
		var c domain.Crosswalk
		if err := rows.Scan(&fromTypology, &c.From.Code, &toTypology, &c.To.Code, &qualifier); err != nil {
			return nil, fmt.Errorf("sqlite: scanning crosswalk of %s: %w", key, err)
		}
		c.From.Typology = domain.TypologyID(fromTypology)
		c.To.Typology = domain.TypologyID(toTypology)
		c.Qualifier = domain.Qualifier(qualifier)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading crosswalks of %s: %w", key, err)
	}
	return out, nil
}

// SpeciesRoles returns a habitat type's species. role filters when non-empty;
// the empty string means every role. Both variants are one static statement —
// the filter is a bound parameter, not appended SQL.
func (d *DB) SpeciesRoles(ctx context.Context, key domain.HabitatTypeKey, role string) ([]domain.SpeciesRole, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT concept_id, verbatim_name, role, fidelity, constancy FROM species_role
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
		`SELECT typology_id, code, concept_id, verbatim_name, role, fidelity, constancy
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
		var concept sql.NullString
		var fidelity, constancy sql.NullFloat64
		if err := rows.Scan(&typology, &r.Key.Code, &concept, &r.VerbatimName, &r.Role, &fidelity, &constancy); err != nil {
			return nil, fmt.Errorf("sqlite: scanning habitat types of concept %q: %w", conceptID, err)
		}
		r.Key.Typology = domain.TypologyID(typology)
		applySpeciesNullables(&r, concept, fidelity, constancy)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading habitat types of concept %q: %w", conceptID, err)
	}
	return out, nil
}

// Syntaxon, Syntaxa, AllSyntaxa and HabitatTypeKeysForSyntaxon live in
// read_syntaxon.go.

// areaChunkSize bounds how many concept ids go into one IN (...) list. SQLite
// caps bound parameters per statement (SQLITE_LIMIT_VARIABLE_NUMBER — measured
// at 32766 with this driver, but only 999 in builds predating SQLite 3.32), and
// the batch read route accepts hundreds of ids. 500 stays under even the legacy
// ceiling with room for the scheme parameter, so no caller can hit the limit at
// runtime whatever the driver was built with.
const areaChunkSize = 500

// AreasForConcepts maps each concept id to the area codes it occurs in. A
// concept without rows is absent from the map, never present with an empty
// slice — the caller distinguishes "unknown" from "known to occur nowhere".
func (d *DB) AreasForConcepts(ctx context.Context, conceptIDs []string, scheme string) (map[string][]string, error) {
	out := map[string][]string{}
	for chunk := range slices.Chunk(conceptIDs, areaChunkSize) {
		if err := d.appendAreasForChunk(ctx, out, chunk, scheme); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// appendAreasForChunk reads one bounded chunk into out.
func (d *DB) appendAreasForChunk(ctx context.Context, out map[string][]string, conceptIDs []string, scheme string) error {
	// Only placeholders are generated here, never values — the ids stay
	// arguments, so this is not SQL construction from input (gosec G201/G202).
	placeholders := strings.Repeat(",?", len(conceptIDs))[1:]
	args := make([]any, 0, len(conceptIDs)+1)
	for _, id := range conceptIDs {
		args = append(args, id)
	}
	args = append(args, scheme)

	rows, err := d.QueryContext(ctx,
		`SELECT concept_id, area_code FROM species_distribution
		 WHERE concept_id IN (`+placeholders+`) AND area_scheme = ?
		 ORDER BY concept_id, area_code`, args...)
	if err != nil {
		return fmt.Errorf("sqlite: reading distribution for %d concepts: %w", len(conceptIDs), err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id, code string
		if err := rows.Scan(&id, &code); err != nil {
			return fmt.Errorf("sqlite: scanning distribution: %w", err)
		}
		out[id] = append(out[id], code)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: iterating distribution: %w", err)
	}
	return nil
}

// KnownAreaCodes lists the distinct area codes the index has data for, in a
// given scheme. The read side validates an area filter against this: an
// unknown code becomes an error, not a silent "does not occur" answer.
func (d *DB) KnownAreaCodes(ctx context.Context, scheme string) ([]string, error) {
	return d.queryStrings(ctx, "area codes",
		`SELECT DISTINCT area_code FROM species_distribution
		 WHERE area_scheme = ? ORDER BY area_code`, scheme)
}

// ConceptIDs lists the distinct concept ids the index holds, so the
// distribution step knows what to ask hostus for.
func (d *DB) ConceptIDs(ctx context.Context) ([]string, error) {
	return d.queryStrings(ctx, "concept ids",
		`SELECT DISTINCT concept_id FROM species_role
		 WHERE concept_id IS NOT NULL ORDER BY concept_id`)
}

// scanSpeciesRole reads the columns shared by the species queries. The
// nullables stay nullable all the way into the domain: an unresolved name must
// arrive as a nil ConceptID, never as an empty string, and a missing fidelity
// must not become 0.0.
func scanSpeciesRole(rows *sql.Rows, r *domain.SpeciesRole) error {
	var concept sql.NullString
	var fidelity, constancy sql.NullFloat64
	if err := rows.Scan(&concept, &r.VerbatimName, &r.Role, &fidelity, &constancy); err != nil {
		return err
	}
	applySpeciesNullables(r, concept, fidelity, constancy)
	return nil
}

func applySpeciesNullables(r *domain.SpeciesRole, concept sql.NullString, fidelity, constancy sql.NullFloat64) {
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
}
