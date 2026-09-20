package domain

// HabitatDescription is the prose description of a habitat type, as published
// in the EUNIS-ESy factsheets. It is a property OF the type, not a second
// naming of it: NameEN stays the identity, this adds what the type actually is.
//
// Only the typology's own level carries one — the factsheets describe EUNIS
// level 3, and a description is never inherited downwards: a subtype is not
// what its parent is, only similar to it.
type HabitatDescription struct {
	Key    HabitatTypeKey
	TextEN string
	// Source names the artifact the text came from, so a reader can tell which
	// factsheet version stands behind it.
	Source string
}
