package application

import (
	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// speciesEntry converts one indexed species role into its wire shape.
func speciesEntry(r domain.SpeciesRole) input.SpeciesEntry {
	e := input.SpeciesEntry{
		VerbatimName: r.VerbatimName,
		Role:         r.Role,
		Fidelity:     r.Fidelity,
		Constancy:    r.Constancy,
	}
	if r.ConceptID != nil {
		e.ConceptID = *r.ConceptID
	}
	e.Provenance, e.DerivedFrom = derivedProvenance(r)
	return e
}

// derivedProvenance carries a species role's Provenance/DerivedFrom onto the
// wire only for the derived_from_aggregate case — the common (observed) shape
// stays unchanged for existing clients. Shared by speciesEntry and
// SpeciesHabitatTypes so neither grows a second copy of this branching.
func derivedProvenance(r domain.SpeciesRole) (string, *input.AggregateSource) {
	if r.Provenance != "derived_from_aggregate" {
		return "", nil
	}
	if r.DerivedFrom == nil {
		return r.Provenance, nil
	}
	return r.Provenance, &input.AggregateSource{ConceptID: *r.DerivedFrom}
}
