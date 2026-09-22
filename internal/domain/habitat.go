package domain

// Typology is a habitat classification system in a given fassung.
type Typology struct {
	ID        TypologyID
	Scheme    string
	Version   string
	Name      string
	SourceRef string
}

// HabitatType is an abstract type within a typology — not a biotope in the
// landscape. Level/ParentCode are nil/empty for typologies without hierarchy;
// Priority is set only for annex1 (priority habitat type).
type HabitatType struct {
	Key        HabitatTypeKey
	Level      *int
	NameEN     string
	ParentCode string
	Priority   *bool
}

// Crosswalk is a correspondence between two habitat types. The same shape
// carries both the EUNIS version crosswalk and the EUNIS->annex1 crosswalk.
type Crosswalk struct {
	From      HabitatTypeKey
	To        HabitatTypeKey
	Qualifier Qualifier
}

// The ranks of the syntaxa hierarchy. "formation" is the root (the
// sections A-Y of the EuroVegChecklist) and the only rank with an empty
// ParentID.
const (
	SyntaxonRankFormation = "formation"
	SyntaxonRankClass     = "class"
	SyntaxonRankOrder     = "order"
	SyntaxonRankAlliance  = "alliance"
)

// The source of a syntaxon ROW, not of its factual data.
const (
	SyntaxonSourceEVC   = "evc"
	SyntaxonSourceEUNIS = "eunis"
)

const (
	ParentProvenanceOfficial = "official"
	ParentProvenanceDerived  = "derived"
)

const (
	LifeFormPhanerogam      = "phanerogam"
	LifeFormBryophyteLichen = "bryophyte_lichen"
	LifeFormAlgae           = "algae"
)

type Syntaxon struct {
	ID   string
	Rank string // "formation" (the root) | "class" | "order" | "alliance"
	Name string // syntaxon name only, without authorship; FloraVeg's clean
	// name on a FloraVeg-sourced/matched row, else the historical EUNIS
	// combi-string (author embedded) for an unmatched EUNIS alliance
	Author   string // author citation; "" if no clean split is known
	ParentID string

	// EEACode is the EEA-EUNIS code of the same syntaxon. Empty for
	// formations and for units known to only one of the two sources.
	EEACode string

	// Source names the source of the row: SyntaxonSourceEVC or
	// SyntaxonSourceEUNIS.
	Source string

	// ParentProvenance distinguishes a parent taken from the source
	// (ParentProvenanceOfficial) from a derived one
	// (ParentProvenanceDerived). Always official when ParentID is empty.
	ParentProvenance string

	// LifeFormGroup is only set on formation rows.
	LifeFormGroup string
}

// SpeciesRole is a species' role in a habitat type. VerbatimName is always
// set; ConceptID is nil when the name could not be resolved against the local
// crosswalk.
type SpeciesRole struct {
	Key          HabitatTypeKey
	ConceptID    *string
	VerbatimName string
	Role         string // "diagnostic" | "constant" | "dominant"
	Fidelity     *float64
	Constancy    *float64
	// Provenance is "observed" (an explicit species_roles.csv row) or
	// "derived_from_aggregate" (written because ConceptID's aggregate lists
	// this species as a member). A derived row never carries Fidelity or
	// Constancy — neither was ever measured for the member itself.
	Provenance string
	// DerivedFrom is the aggregate's concept id that produced this row; nil
	// unless Provenance is "derived_from_aggregate".
	DerivedFrom *string
}

// Localization is an additive label overlay. Provenance is "official",
// "curated" or "derived"; DerivedFrom records the origin of a derived value.
type Localization struct {
	EntityType  string // "habitat_type" | "syntaxon"
	EntityKey   string
	Lang        string
	Field       string // "name" | "description" | "key"
	Value       string
	Source      string
	Provenance  string
	DerivedFrom string
}
