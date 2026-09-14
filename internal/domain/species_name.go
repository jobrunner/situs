package domain

// SpeciesName is one verbatim species name the index itself carries, with
// the concept id it resolved to — nil when the ingest could not resolve it.
//
// Deliberately nullable rather than dropped: a name the index knows but
// could not resolve is part of the truth about this index. Hiding those
// rows would suggest a higher resolution rate than the measured one.
type SpeciesName struct {
	VerbatimName string
	ConceptID    *string
}
