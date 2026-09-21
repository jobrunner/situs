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
	// ErrUnknownVocab is a trait vocabulary the index has no data for. Same
	// treatment as ErrUnknownArea: a typo and a genuine absence must not
	// look the same -> INVALID_QUERY.
	ErrUnknownVocab = errors.New("unknown trait vocabulary")
	// ErrInvalidQuery is a malformed request parameter -> INVALID_QUERY. The
	// existing ErrUnknown* sentinels say "this value does not exist here";
	// this one says "this value is not well-formed at all".
	ErrInvalidQuery = errors.New("invalid query")
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

// GermanLabel is the additive German overlay of one entity. It is a struct, not
// a flat set of sibling fields, so a client cannot read the value without seeing
// the provenance that qualifies it — the whole point when some labels are
// official and others are situs' own translation.
type GermanLabel struct {
	Value string `json:"value"`
	// Vernacular is the established German term. It is present only where one
	// exists AND carries the same extent as the type; absent is information, not
	// an omission, so it is never an empty string.
	Vernacular string `json:"vernacular,omitempty"`
	// Provenance is official | curated | derived | situs. situs means situs
	// translated it and no external source stands behind it.
	Provenance string `json:"provenance"`
	Source     string `json:"source"`
}

// HabitatTypeSummary is a habitat type plus its additive label overlay:
// NameEN stays the identity, NameDE is added and carries its provenance.
type HabitatTypeSummary struct {
	Typology domain.TypologyID `json:"typology"`
	Code     string            `json:"code"`
	Level    *int              `json:"level,omitempty"`
	NameEN   string            `json:"name_en"`
	NameDE   *GermanLabel      `json:"name_de,omitempty"`
	// Priority is set only for annex1 types (priority habitat type).
	Priority *bool `json:"priority,omitempty"`
}

// SyntaxonRef is a vegetation unit a habitat type is linked to.
type SyntaxonRef struct {
	ID   string `json:"id"`
	Rank string `json:"rank"`
	Name string `json:"name"`
	// Author is the authorship citation, taken verbatim from FloraVeg.EU's own
	// already-separated column — absent for the units that only the EEA-EUNIS
	// source carries.
	Author string `json:"author,omitempty"`
	// ParentID references a FloraVeg order code (class -> order -> alliance
	// hierarchy) — absent only for a formation, the root of the hierarchy. A
	// plain string reference, not a schema-bound foreign key: two id schemes
	// (EUNIS alliance codes, FloraVeg class/order codes) coexist here.
	ParentID string `json:"parent_id,omitempty"`

	// AltCode is the EEA-EUNIS code of the same syntaxon — a client
	// holding an old code can switch over with it, without guessing.
	AltCode string `json:"alt_code,omitempty"`
	// Source is "evc" or "eunis": which source this row carries.
	Source string `json:"source,omitempty"`
	// ParentProvenance is "official" or "derived". A derived parent is
	// never presented as a source-backed statement.
	ParentProvenance string `json:"parent_provenance,omitempty"`
	// LifeFormGroup is only filled on formation rows, so empty in every
	// response that exists today. The field is here already because
	// sub-project B delivers the formations as []SyntaxonRef and filters
	// on it.
	LifeFormGroup string `json:"life_form_group,omitempty"`
}

// CrosswalkRef is the far side of a correspondence, seen from the queried type.
// Qualifier always reads "queried type <qualifier> this type".
type CrosswalkRef struct {
	Typology  domain.TypologyID `json:"typology"`
	Code      string            `json:"code"`
	Qualifier domain.Qualifier  `json:"qualifier"`
}

// AggregateSource lets a caller ask "what aggregate produced this match?".
// Name is omitempty: the index only ever persists the aggregate's concept
// id (domain.SpeciesRole.DerivedFrom), never its name, so this field stays
// unset until a future ingest step stores that name too.
type AggregateSource struct {
	ConceptID string `json:"concept_id"`
	Name      string `json:"name,omitempty"`
}

// SpeciesEntry is one species in its role. VerbatimName is always set;
// ConceptID is absent when the name did not resolve against the crosswalk.
type SpeciesEntry struct {
	ConceptID    string   `json:"concept_id,omitempty"`
	VerbatimName string   `json:"verbatim_name"`
	Role         string   `json:"role"`
	Fidelity     *float64 `json:"fidelity,omitempty"`
	Constancy    *float64 `json:"constancy,omitempty"`
	// InArea is nil when unknowable: no concept id, or a concept without
	// distribution rows. It is absent from the wire without an area filter.
	InArea *bool `json:"in_area,omitempty"`
	// Provenance is "observed" (default, omitted) or "derived_from_aggregate"
	// — present only in the derived case, so the common (observed) response
	// shape is unchanged for existing clients.
	Provenance  string           `json:"provenance,omitempty"`
	DerivedFrom *AggregateSource `json:"derived_from,omitempty"`
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
	// Description is the habitat's prose description in English, absent when
	// this type has none. It is never inherited from a parent type: a subtype
	// is not what its parent is.
	Description *DescriptionText `json:"description,omitempty"`
	// DescriptionDE is the German overlay of Description, same shape and same
	// rules as NameDE. Present only with ?lang=de, and only where a
	// translation was ingested.
	DescriptionDE *GermanLabel              `json:"description_de,omitempty"`
	Species       map[string][]SpeciesEntry `json:"species"`
	Syntaxa       []SyntaxonRef             `json:"syntaxa"`
	Crosswalks    []CrosswalkRef            `json:"crosswalks"`
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
	// Provenance/DerivedFrom mirror SpeciesEntry's: present only when this
	// role hit was derived from an aggregate's membership list.
	Provenance  string           `json:"provenance,omitempty"`
	DerivedFrom *AggregateSource `json:"derived_from,omitempty"`
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

// TraitValueView is the wire shape of one domain.TraitValue.
type TraitValueView struct {
	Dim        string   `json:"dim"`
	Value      float64  `json:"value"`
	NicheWidth *float64 `json:"niche_width,omitempty"`
	NSystems   *int     `json:"n_systems,omitempty"`
}

// TraitSetView groups every TraitValueView one vocabulary contributes for one
// concept — never merged across vocabularies.
type TraitSetView struct {
	Vocab        string           `json:"vocab"`
	VocabVersion string           `json:"vocab_version"`
	Values       []TraitValueView `json:"values"`
}

// TypologyView is one classification system the index carries. HabitatTypes
// is measured from the index, like every other figure in IndexInfo — a
// typology with 0 is registered but not (yet) filled, an honest answer, not
// an error.
type TypologyView struct {
	ID           domain.TypologyID `json:"id"`
	Scheme       string            `json:"scheme"`
	Version      string            `json:"version"`
	Name         string            `json:"name"`
	SourceRef    string            `json:"source_ref"`
	HabitatTypes int               `json:"habitat_types"`
}

// DescriptionText is a habitat type's description together with where its
// wording comes from. The provenance is not decoration: `official` is an
// external source's own text (the EUNIS-ESy factsheets), `situs` means situs
// wrote it from the sources named in Source, and nobody outside situs vouches
// for that wording.
type DescriptionText struct {
	Value      string `json:"value"`
	Provenance string `json:"provenance"`
	Source     string `json:"source"`
}

// AreaView is one distribution area a client can filter by. Name is the
// English WGSRPD name and is empty when no name was ingested for the code —
// the code stays the identity, the name is an overlay on it.
type AreaView struct {
	Scheme string `json:"scheme"`
	Code   string `json:"code"`
	Name   string `json:"name"`
}

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

// SpeciesSearchHit is one hit of the index-own name search.
//
// ConceptID has no omitempty on purpose: a name the index carries but could
// not resolve must arrive as an explicit null, so a client can tell "not
// resolvable" from "field forgotten". Filtering those hits out would state a
// higher resolution rate than the index actually has.
type SpeciesSearchHit struct {
	VerbatimName string  `json:"verbatim_name"`
	ConceptID    *string `json:"concept_id"`
}

// The search result bound. A default keeps a bare ?q= cheap; the maximum
// keeps one request from serving the whole index as an autocomplete answer.
const (
	DefaultSearchLimit = 20
	MaxSearchLimit     = 100
)

// DimensionSummary is one dimension's statistics over a species list.
//
// Absent fields are absent measurements, never zeroes: sd is omitted for a
// single value (a spread of zero is a claim; "cannot be computed" is a
// different one), and MeanNicheWeighted is omitted for vocabularies that
// carry no niche widths (Tichý, Midolo) so it can never be confused with
// the unweighted mean.
type DimensionSummary struct {
	Mean              float64  `json:"mean"`
	MeanNicheWeighted *float64 `json:"mean_niche_weighted,omitempty"`
	SD                *float64 `json:"sd,omitempty"`
	Min               float64  `json:"min"`
	Max               float64  `json:"max"`
	// N is how many species carried a value in this dimension, NMissing how
	// many of the known species did not. Per dimension, not global: EIVE
	// covers L/M/N/R/T unevenly, and a mean over 3 of 18 species is a
	// different statement than one over 17 of 18.
	N        int `json:"n"`
	NMissing int `json:"n_missing"`
	// NExcludedWeighted counts species left out of the weighted mean because
	// their niche width was not positive. Zero in the pinned data; reported
	// rather than silently repaired if a future ingest produces one.
	NExcludedWeighted int `json:"n_excluded_weighted,omitempty"`
}

// VocabSummary is one vocabulary's dimensions. Never merged across
// vocabularies: EIVE's 0–10 and Tichý's 1–12 are different scales, so their
// common mean would be an invented number.
type VocabSummary struct {
	VocabVersion string                      `json:"vocab_version"`
	Dimensions   map[string]DimensionSummary `json:"dimensions"`
}

// UnknownConcept is one concept id the index could not answer for, with the
// same two diagnoses the batch route uses.
type UnknownConcept struct {
	ConceptID string `json:"concept_id"`
	Reason    string `json:"reason"`
}

// TraitSummary is the indicator-value analysis over a species list.
type TraitSummary struct {
	Requested    int                     `json:"requested"`
	Known        int                     `json:"known"`
	Unknown      []UnknownConcept        `json:"unknown"`
	Vocabularies map[string]VocabSummary `json:"vocabularies"`
}

// QueryService is the read API's use cases over the local index. Every method
// is autark: it needs no upstream service.
type QueryService interface {
	// IndexInfo describes what the index holds, measured from the index itself.
	IndexInfo(ctx context.Context) (IndexInfo, error)
	// Typologies lists every typology the index carries, sorted by id, so a
	// client can discover which (typology, code) pairs it may even ask
	// about instead of guessing eunis@2021 and never learning that
	// eunis@2012 and annex1 exist too.
	Typologies(ctx context.Context) ([]TypologyView, error)
	// Areas lists the areas the ?area= filter can answer, with their names.
	Areas(ctx context.Context) ([]AreaView, error)
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
	// Traits returns a concept's indicator values, grouped per vocabulary,
	// never merged. vocab filters to one vocabulary; empty means every
	// ingested vocabulary. An unresolvable vocab is INVALID_QUERY; a
	// concept with no trait data is a normal empty answer, not NOT_FOUND.
	Traits(ctx context.Context, conceptID, vocab string) ([]TraitSetView, error)
	// SearchSpecies finds index-own verbatim names containing q. It is not
	// name resolution (no fuzzy, no synonyms) — see the repository port.
	// limit == nil means "unset" and takes DefaultSearchLimit; a non-nil
	// limit outside [1, MaxSearchLimit] — including an explicit 0 — is
	// rejected rather than clamped or silently defaulted.
	SearchSpecies(ctx context.Context, query string, limit *int) ([]SpeciesSearchHit, error)
	// SpeciesTraitSummary aggregates the indicator values of a species list,
	// strictly per vocabulary and dimension — never across vocabularies,
	// whose scales differ.
	SpeciesTraitSummary(ctx context.Context, conceptIDs []string) (TraitSummary, error)
}
