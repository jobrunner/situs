package domain

// The provenance vocabulary of a description. Two kinds of text share the
// table and they must never be confused: the EUNIS-ESy factsheet wording is
// published by its authors, the Annex I text is written by situs out of the
// Interpretation Manual, the EUNIS crosswalk and the index's own species data.
const (
	// DescriptionProvenanceOfficial is the wording of an external, citable
	// source, taken over unchanged.
	DescriptionProvenanceOfficial = "official"
	// DescriptionProvenanceSitus is the weakest claim: situs wrote this text
	// from its sources, and no external body vouches for the wording.
	DescriptionProvenanceSitus = "situs"
)

// HabitatDescription is the prose description of a habitat type. It is a
// property OF the type, not a second naming of it: NameEN stays the identity,
// this adds what the type actually is.
//
// A description is never inherited downwards: a subtype is not what its parent
// is, only similar to it.
type HabitatDescription struct {
	Key    HabitatTypeKey
	TextEN string
	// Source names the artifact the text came from or was derived from, so a
	// reader can tell which version stands behind it.
	Source string
	// Provenance is official | situs. It decides how much the wording can be
	// leaned on, and it is never guessed: the ingest sets it per source file.
	Provenance string
}
