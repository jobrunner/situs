// Package input holds the driving ports — what the application offers to
// primary adapters. The HTTP adapter depends on these interfaces, never on
// concrete application services.
//
// The response DTOs below carry JSON tags because the HTTP contract has one
// definition rather than two that can drift. They are view models, not a wire
// format any adapter must accept: a non-HTTP driving adapter (MCP, CLI, gRPC)
// must map them to its own representation instead of reusing these tags.
package input

import (
	"context"
	"errors"

	"github.com/jobrunner/situs/internal/domain"
)

// HealthChecker backs the readiness probe, so an orchestrator does not route
// traffic prematurely.
//
// What the current wiring guarantees, and no more: startup fails outright when
// the index cannot be opened, so a serving process had a usable index at
// construction time. The probe does NOT re-check the index while serving — an
// index that becomes unreadable later still reports ready. Making that real
// needs a Ping on output.Repository, which is a deliberate follow-up, not
// something this comment may promise in advance.
type HealthChecker interface {
	Ready(ctx context.Context) bool
}

// The read API's failure modes, as sentinels the HTTP adapter maps to the error
// envelope's codes. They live here, not in ports/output, so the driving adapter
// never has to import a driven port to classify an answer.
var (
	// ErrNotFound is an unknown habitat type, concept or syntaxon within a
	// known typology -> NOT_FOUND.
	ErrNotFound = errors.New("not found")
	// ErrUnknownTypology is a typology the index does not carry -> INVALID_QUERY.
	// The query names a classification system that does not exist, which is a
	// malformed question, not a missing answer (see the spec's Fehlerbehandlung).
	ErrUnknownTypology = errors.New("unknown typology")
	// ErrUnknownArea is an area code the index has no data for. It must not be
	// answered with a list of "does not occur": a typo and a genuine absence
	// would look the same -> INVALID_QUERY.
	ErrUnknownArea = errors.New("unknown area")
)

// The role vocabulary of species_role, closed for this foundation.
const (
	RoleDiagnostic = "diagnostic"
	RoleConstant   = "constant"
	RoleDominant   = "dominant"
)

// The DTOs below are the read API's view models and carry their JSON tags here
// on purpose: the wire shape is part of the published contract, so it has one
// definition both the use case and its adapter agree on instead of two that can
// drift. Every slice/map is non-nil when returned, so JSON never shows null
// where the contract promises a list.

// HabitatTypeSummary is a habitat type plus its additive label overlay:
// NameEN stays the identity, NameDE is added and carries its provenance.
type HabitatTypeSummary struct {
	Typology         domain.TypologyID `json:"typology"`
	Code             string            `json:"code"`
	Level            *int              `json:"level,omitempty"`
	NameEN           string            `json:"name_en"`
	NameDE           string            `json:"name_de,omitempty"`
	NameDEProvenance string            `json:"name_de_provenance,omitempty"`
	// Priority is set only for annex1 types (priority habitat type).
	Priority *bool `json:"priority,omitempty"`
}

// SyntaxonRef is a vegetation unit a habitat type is linked to.
type SyntaxonRef struct {
	ID   string `json:"id"`
	Rank string `json:"rank"`
	Name string `json:"name"`
}

// CrosswalkRef is the far side of a correspondence, seen from the queried type.
// Qualifier always reads "queried type <qualifier> this type".
type CrosswalkRef struct {
	Typology  domain.TypologyID `json:"typology"`
	Code      string            `json:"code"`
	Qualifier domain.Qualifier  `json:"qualifier"`
}

// SpeciesEntry is one species in its role. VerbatimName is always set;
// ConceptID is absent when the name did not resolve against hostus.
type SpeciesEntry struct {
	ConceptID    string   `json:"concept_id,omitempty"`
	VerbatimName string   `json:"verbatim_name"`
	Role         string   `json:"role"`
	Fidelity     *float64 `json:"fidelity,omitempty"`
	Constancy    *float64 `json:"constancy,omitempty"`
	// InArea is nil when unknowable: no concept id, or a concept without
	// distribution rows. It is absent from the wire without an area filter.
	InArea *bool `json:"in_area,omitempty"`
}

// AreaFilter is the caller's view on a species list. Code is a WGSRPD level 3
// code — the frontend derives it from GPS, so situs needs no ISO mapping (and
// the "CZE = Czechia-Slovakia" ambiguity never arises). OnlyInArea drops the
// definite absences; the unknowns always stay.
type AreaFilter struct {
	Code       string
	OnlyInArea bool
}

// Active reports whether a filter was asked for at all.
func (f AreaFilter) Active() bool { return f.Code != "" }

// HabitatTypeDetail answers GET /v1/habitat-type/{typology}/{code}. Species is
// keyed by role and always carries the three known roles, empty where there is
// nothing — a role bucket present but empty is information, absence is not.
type HabitatTypeDetail struct {
	HabitatTypeSummary
	Species    map[string][]SpeciesEntry `json:"species"`
	Syntaxa    []SyntaxonRef             `json:"syntaxa"`
	Crosswalks []CrosswalkRef            `json:"crosswalks"`
}

// HabitatTypeRole is a habitat type together with the role the queried species
// plays in it, plus that type's syntaxa.
type HabitatTypeRole struct {
	HabitatTypeSummary
	Role      string        `json:"role"`
	Fidelity  *float64      `json:"fidelity,omitempty"`
	Constancy *float64      `json:"constancy,omitempty"`
	Syntaxa   []SyntaxonRef `json:"syntaxa"`
	// InArea is nil when unknowable: no concept id, or a concept without
	// distribution rows. It is absent from the wire without an area filter.
	InArea *bool `json:"in_area,omitempty"`
}

// ConceptResolution is one entry of the batch answer. Known is false and
// HabitatTypes empty when the index cannot answer — the input is reported back
// either way, never dropped.
type ConceptResolution struct {
	ConceptID    string            `json:"concept_id"`
	Known        bool              `json:"known"`
	Reason       string            `json:"reason,omitempty"`
	InArea       *bool             `json:"in_area,omitempty"`
	HabitatTypes []HabitatTypeRole `json:"habitat_types"`
}

// The two diagnoses an unanswerable concept id can have. They are different
// faults: a wrong backbone is the caller's, a concept without facts is the
// data's limit — one label for both sends people looking in the wrong place.
const (
	ReasonUnknownBackbone = "unknown_backbone"
	ReasonUnknownConcept  = "unknown_concept"
)

// IndexInfo is the index's self-description, so a client can check up front
// whether its concept ids can match at all instead of discovering a backbone
// mismatch through empty answers. Every field is measured from the index, never
// configured — a figure nobody can trust is worse than no figure.
//
// It deliberately does not carry the backbone's *fassung* (e.g. "wcvp
// 2026-06-15"): the ingest does not record it today, and inventing one here
// would be exactly the untrustworthy figure. That gap is noted in the spec.
type IndexInfo struct {
	// ConceptBackbones are the distinct id prefixes present, sorted. Normally
	// one; more than one means the index was built from mixed sources.
	ConceptBackbones []string `json:"concept_backbones"`
	// SpeciesWithConcept is the number of distinct concept ids the index holds.
	SpeciesWithConcept int `json:"species_with_concept"`
	// AreaScheme names the vocabulary ?area= codes come from.
	AreaScheme string `json:"area_scheme"`
	// AreasWithData is the number of distinct area codes with distribution
	// rows. It is zero until a distribution ingest has run — which is a true
	// statement about the index, not a placeholder.
	AreasWithData int `json:"areas_with_data"`
}

// QueryService is the read API's use cases over the local index. Every method
// is autark: it needs no upstream service.
type QueryService interface {
	// IndexInfo describes what the index holds, measured from the index itself.
	IndexInfo(ctx context.Context) (IndexInfo, error)
	// HabitatType returns one type with its species, syntaxa and crosswalks.
	// filter marks (and, if OnlyInArea, prunes) the species by area.
	HabitatType(ctx context.Context, key domain.HabitatTypeKey, lang string, filter AreaFilter) (HabitatTypeDetail, error)
	// SpeciesHabitatTypes returns the habitat types a concept has a role in.
	SpeciesHabitatTypes(ctx context.Context, conceptID, lang string, filter AreaFilter) ([]HabitatTypeRole, error)
	// HabitatTypeSpecies returns a type's species, filtered by role when role
	// is non-empty, and marked (or pruned) by area per filter.
	HabitatTypeSpecies(ctx context.Context, key domain.HabitatTypeKey, role string, filter AreaFilter) ([]SpeciesEntry, error)
	// SyntaxonHabitatTypes returns the habitat types a syntaxon is linked to.
	SyntaxonHabitatTypes(ctx context.Context, syntaxonID, lang string) ([]HabitatTypeSummary, error)
	// SpeciesSetHabitatTypes answers a whole field record at once: one entry per
	// input concept id, in input order, duplicates included.
	SpeciesSetHabitatTypes(ctx context.Context, conceptIDs []string, lang string, filter AreaFilter) ([]ConceptResolution, error)
}
