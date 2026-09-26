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

	// EEACode is the EEA-EUNIS code of the same syntaxon — a client
	// holding an old code can switch over with it, without guessing.
	EEACode string `json:"eea_code,omitempty"`
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

	// NameDE is the additive German overlay, present only with lang=de AND
	// only where a translation exists. Only the 25 formations are translated
	// (data/localizations-de-syntaxa.csv): an alliance inheriting its
	// formation's label would be a wrong name, not a fallback, so the field
	// simply stays absent there.
	//
	// Name keeps the identity, exactly as NameEN does on a habitat type: the
	// scientific syntaxon name is what a client cites, and the German label is
	// what it shows.
	NameDE *GermanLabel `json:"name_de,omitempty"`

	// Occurrence is "verified" or "uncertain" and is set only in an
	// ?area=-filtered list. MISSING means no statement exists — the same
	// three-valuedness as in_area on the species side. Without it the carried
	// row would be indistinguishable from a confirmed one, and a list that
	// keeps everything but marks nothing is just as dishonest as one that
	// throws away.
	Occurrence string `json:"occurrence,omitempty"`
}

// SyntaxonDetail is a syntaxon with its surroundings: the way up and the direct
// children. Both answer the same question ("where am I and where can I go?")
// and therefore belong in the same response — a separate /children or
// /ancestors route would be a second way to the same data and would make a
// breadcrumb trail cost three requests.
//
// SyntaxonRef is EMBEDDED, so Go promotes its fields into this same JSON
// object: id, rank, name, author, parent_id, eea_code, source,
// parent_provenance and life_form_group are siblings of ancestors and children
// on the wire, not a nested object. The OpenAPI schema models that as allOf and
// a test pins it, because a nested schema and a flat wire format would be two
// contracts claiming to be one.
//
// life_form_group is deliberately NOT repeated here: a field of the same name
// in the outer struct would shadow the embedded one and put two fields on one
// JSON key. For every rank but formation the value is derived from the
// formation the ancestor path reaches — derived, not stored, and verifiable by
// the client because ancestors travels in the same response.
type SyntaxonDetail struct {
	SyntaxonRef

	// Ancestors is the way to the root, OUTERMOST first (formation, then
	// class, then order), and empty for a formation. The order is fixed so a
	// client can print it unchanged as a breadcrumb trail.
	//
	// No omitempty, and never nil: an empty list and a missing field are
	// different statements for a client.
	Ancestors []SyntaxonRef `json:"ancestors"`

	// Children are the direct children, ordered by id. Empty for an alliance —
	// the lower bound of the data, not an error. No omitempty, same reason as
	// Ancestors.
	Children []SyntaxonRef `json:"children"`

	// DirectHabitatTypeCount is the number of habitat types linking EXACTLY
	// this syntaxon, not its descendants'. For a class or formation it is
	// therefore almost always 0, because habitat_type_syntaxon links alliances
	// (and in one case an order). The name says so, so a client does not read
	// the 0 as "this class touches no EUNIS type"; aggregating over the
	// descendants is a question of its own.
	DirectHabitatTypeCount int `json:"direct_habitat_type_count"`

	// Distribution is the source's statement about this syntaxon. Nil when
	// there is none (no coverage row) — then NOTHING is known about its
	// occurrence, which is strictly different from "occurs nowhere". Measured:
	// 212 of 1326 alliances are nil, including every bryophyte, lichen and
	// algal one, because the source covers vascular-plant dominated vegetation
	// only.
	Distribution *SyntaxonDistribution `json:"distribution,omitempty"`
}

// SyntaxonDistribution is the source's statement about where a syntaxon
// occurs. Verified and Uncertain are sorted code lists.
//
// Absence is deliberately NOT enumerated: 136 minus the occupied codes would
// be an invented list, and the client knows the scheme from GET /v1/areas
// ?scheme=evc_territory.
type SyntaxonDistribution struct {
	AreaScheme string   `json:"area_scheme"`
	Verified   []string `json:"verified"`
	Uncertain  []string `json:"uncertain"`
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

// SyntaxonAreaFilter is the ?area=/?include= pair on the syntaxa list.
//
// Include is the set of occurrence values that count as a hit, default
// {verified}. It is a SET rather than a single value because "verified or
// uncertain" is a real question and two requests plus a client-side merge
// would be a worse answer.
type SyntaxonAreaFilter struct {
	Code    string
	Include []string
}

// Active reports whether a filter was asked for at all.
func (f SyntaxonAreaFilter) Active() bool { return f.Code != "" }

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
	// AreaScheme names the vocabulary ?area= codes come from ON THE SPECIES
	// ROUTES. It keeps its singular name although there are now two schemes:
	// a field name in a published answer is not a matter of taste, and
	// renaming it would break every client that reads it. What it means is
	// spelled out here and in the OpenAPI description instead.
	AreaScheme string `json:"area_scheme"`
	// AreasWithData is the number of distinct area codes with distribution
	// rows. It is zero until a distribution ingest has run — which is a true
	// statement about the index, not a placeholder.
	AreasWithData int `json:"areas_with_data"`
	// SyntaxonAreaScheme names the vocabulary ?area= codes come from ON THE
	// SYNTAXA ROUTES. Named even when SyntaxaWithDistribution is zero: the
	// scheme is a property of this release, the count one of this index.
	SyntaxonAreaScheme string `json:"syntaxon_area_scheme"`
	// SyntaxaWithDistribution is the number of syntaxa the source makes any
	// statement about (the coverage rows). It is zero until a distribution
	// ingest has run — a true statement about the index, not a placeholder.
	// Measured against the pinned artifacts: 1114 of 1326 alliances.
	SyntaxaWithDistribution int `json:"syntaxa_with_distribution"`
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
	// Areas lists the areas of one scheme the ?area= filter can answer, with
	// their names. The scheme is a parameter and not a constant because situs
	// stores two (domain.SchemeWGSRPDL3 for species, domain.SchemeEVCTerritory
	// for syntaxa) and a flat list mixing both would be ambiguous: the caller
	// could not tell which vocabulary a code belongs to without reading every
	// entry's scheme field.
	Areas(ctx context.Context, scheme string) ([]AreaView, error)
	// HabitatType returns one type with its species, syntaxa and crosswalks.
	// filter marks (and, if OnlyInArea, prunes) the species by area.
	HabitatType(ctx context.Context, key domain.HabitatTypeKey, lang string, filter AreaFilter) (HabitatTypeDetail, error)
	// MatchHabitatTypes ranks habitat types by how well they explain a set of
	// observed concept ids. The score is a log-likelihood, never a probability.
	MatchHabitatTypes(ctx context.Context, req MatchRequest) (MatchResult, error)
	// SpeciesHabitatTypes returns the habitat types a concept has a role in.
	SpeciesHabitatTypes(ctx context.Context, conceptID, lang string, filter AreaFilter) ([]HabitatTypeRole, error)
	// HabitatTypeSpecies returns a type's species, filtered by role when role
	// is non-empty, and marked (or pruned) by area per filter.
	HabitatTypeSpecies(ctx context.Context, key domain.HabitatTypeKey, role string, filter AreaFilter) ([]SpeciesEntry, error)
	// SyntaxonHabitatTypes returns the habitat types a syntaxon is linked to.
	SyntaxonHabitatTypes(ctx context.Context, syntaxonID, lang string) ([]HabitatTypeSummary, error)
	// Syntaxon returns one vegetation unit with its ancestor path and its
	// direct children — the whole navigation step in one answer. An unknown id
	// is ErrNotFound; a parent_id pointing at a missing row is an inconsistent
	// index and is reported as such, never bridged.
	//
	// lang=de overlays the German formation label on the unit itself, on every
	// ancestor and on every child. The ancestors matter most: they are what a
	// client prints as a breadcrumb, and an untranslated root would end a
	// German trail in an English word.
	Syntaxon(ctx context.Context, id, lang string) (SyntaxonDetail, error)
	// SyntaxaByRank lists every syntaxon of rank, ordered by id, narrowed to
	// one life-form group when lifeFormGroup is non-empty (the two filters act
	// as AND). rank is validated against what the index carries: an unknown
	// value is ErrInvalidQuery naming the ranks that would have worked, never
	// an empty list that reads as "there are none".
	//
	// It returns SyntaxonRef and not SyntaxonDetail on purpose: the roots need
	// neither an ancestor path (empty) nor a child list (that is the next
	// step), and 25 details with 150 children each would be an answer nobody
	// asked for.
	//
	// filter, when active, keeps only syntaxa with a matching occurrence PLUS
	// those the source says nothing about.
	//
	// lang=de overlays the German label. It costs ONE query for the whole list,
	// not one per row — see Repository.LocalizationsByEntityType.
	SyntaxaByRank(ctx context.Context, rank, lifeFormGroup, lang string,
		filter SyntaxonAreaFilter) ([]SyntaxonRef, error)
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

// MatchRequest ist die Anfrage an POST /v1/habitat-types/match.
type MatchRequest struct {
	ConceptIDs []string
	Typology   string
	Level      int
	Area       string
	Limit      int
}

// MatchInput spiegelt eine Eingabe-ID zurueck.
type MatchInput struct {
	ConceptID string `json:"concept_id"`
	Known     bool   `json:"known"`
	Reason    string `json:"reason,omitempty"`
}

// MatchSpecies nennt eine Eingabe-Art, die fuer diesen Habitattyp gesprochen
// hat, mit der Rolle und der Kennzahl, die dabei zaehlte. Im Gelaende ist das
// die Anschlussfrage: welche der anderen Kennarten suche ich jetzt?
type MatchSpecies struct {
	ConceptID string   `json:"concept_id"`
	Role      string   `json:"role"`
	Constancy *float64 `json:"constancy,omitempty"`
	Fidelity  *float64 `json:"fidelity,omitempty"`
}

// MatchEntry ist ein Rang der Antwort. Score ist ein Log-Likelihood, keine
// Wahrscheinlichkeit: belastbar ist die Reihenfolge, nicht der Betrag.
//
// Kein name_de: die Route nimmt keinen lang-Parameter, und ein in der
// Spezifikation gefuehrtes Feld, das nie erscheint, ist Doku-Drift.
type MatchEntry struct {
	Typology string         `json:"typology"`
	Code     string         `json:"code"`
	NameEN   string         `json:"name_en"`
	Score    float64        `json:"score"`
	Matched  int            `json:"matched"`
	Of       int            `json:"of"`
	Species  []MatchSpecies `json:"species"`
}

// MatchResult ist die Antwort. Beide Listen sind immer da, auch leer.
type MatchResult struct {
	Input   []MatchInput `json:"input"`
	Matches []MatchEntry `json:"matches"`
}
