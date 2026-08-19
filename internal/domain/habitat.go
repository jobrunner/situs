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

type Syntaxon struct {
	ID       string
	Rank     string // "class" | "order" | "alliance"
	Name     string
	ParentID string
}

// SpeciesRole is a species' role in a habitat type. VerbatimName is always
// set; ConceptID is nil when the name could not be resolved via hostus.
type SpeciesRole struct {
	Key          HabitatTypeKey
	ConceptID    *string
	VerbatimName string
	Role         string // "diagnostic" | "constant" | "dominant"
	Fidelity     *float64
	Constancy    *float64
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
