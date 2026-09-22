package output

import (
	"context"
	"errors"

	"github.com/jobrunner/situs/internal/domain"
)

// ErrNotFound signals that a query found no matching row. Callers map it to
// the API's NOT_FOUND error envelope.
var ErrNotFound = errors.New("not found")

// IngestTx is one atomic ingest run. Every Upsert is idempotent so a repinned
// artifact can simply be re-ingested.
type IngestTx interface {
	UpsertTypology(t domain.Typology) error
	UpsertHabitatType(h domain.HabitatType) error
	UpsertCrosswalk(c domain.Crosswalk) error
	UpsertSyntaxon(s domain.Syntaxon) error
	// SetSyntaxonParent sets the parent and its provenance on an already
	// written row. Separate from UpsertSyntaxon because the parent of the
	// 16 EEA-only units is only known once the FloraVeg rows and their
	// siblings are in the index.
	SetSyntaxonParent(id, parentID, provenance string) error
	// ClearSyntaxa empties habitat_type_syntaxon and syntaxon, in that
	// order, before IngestSyntaxa writes the current run's rows. The
	// hierarchy comes entirely from two pinned source files, so a row
	// absent from this run's files must not survive it — an Upsert can only
	// add or overwrite, never remove a row the source dropped, and the
	// syntaxa quellenumkehr made dropped rows the normal case: about 1310
	// ids changed identity in one run. Same reasoning as
	// DeleteTraitValuesForVocab in internal/adapters/sqlite/trait.go, one
	// level up: there a stale vocab_version could linger next to the new
	// one; here a stale EEA-style syntaxon id lingers next to its FloraVeg
	// replacement unless every prior row is gone before the new ones land.
	ClearSyntaxa() error
	LinkSyntaxon(key domain.HabitatTypeKey, syntaxonID string) error
	UpsertSpeciesRole(r domain.SpeciesRole) error
	// UpsertDerivedSpeciesRole writes a species role derived from an aggregate's
	// membership list. Unlike UpsertSpeciesRole it never overwrites an existing
	// row for the same (typology, code, verbatim_name, role) — an explicit
	// species_roles.csv row always wins, regardless of ingest order. Returns
	// suppressed=true when an existing row already occupied that key.
	UpsertDerivedSpeciesRole(r domain.SpeciesRole) (suppressed bool, err error)
	UpsertLocalization(l domain.Localization) error
	// UpsertDistribution records that a concept occurs in an area. Idempotent:
	// a repinned artifact is simply re-ingested.
	UpsertDistribution(conceptID string, a domain.Area) error
	// UpsertArea writes the name of one area. Names are an overlay on the
	// codes species_distribution carries: writing one neither creates nor
	// requires distribution data.
	UpsertArea(a domain.NamedArea) error
	// UpsertSyntaxonDistribution records that a syntaxon occurs in one area,
	// with occurrence being domain.OccurrenceVerified or
	// domain.OccurrenceUncertain. Idempotent, and a repeated ingest overwrites
	// the occurrence: the source is allowed to upgrade an uncertain record to
	// a verified one, and an index that kept the old value would answer from a
	// fassung nobody published.
	UpsertSyntaxonDistribution(syntaxonID, scheme, code, occurrence string) error
	// UpsertSyntaxonDistributionCoverage records that the source makes a
	// statement about this syntaxon at all. Without it "occurs in no
	// territory" is indistinguishable from "nobody looked".
	UpsertSyntaxonDistributionCoverage(syntaxonID, scheme string) error
	// UpsertDescription writes one habitat type's prose description.
	UpsertDescription(d domain.HabitatDescription) error
	// UpsertTraitValue writes one trait_value row for conceptID. Idempotent
	// like every other Upsert here: a repinned vocabulary is simply
	// re-ingested.
	UpsertTraitValue(conceptID string, tv domain.TraitValue) error
	// DeleteTraitValuesForVocab removes every trait_value row for vocab,
	// across every vocab_version. Called once per file before its rows are
	// (re-)written, so a version bump (e.g. EIVE 1.0 -> 1.1) does not leave
	// the old version's rows sitting next to the new one — trait_value's
	// primary key includes vocab_version, so an Upsert alone cannot replace
	// them.
	DeleteTraitValuesForVocab(vocab string) error
	// UpsertTraitVocabulary records that vocab/version was (re-)ingested —
	// pure ingest metadata for later drift detection, no factual content for
	// the reader. Idempotent.
	UpsertTraitVocabulary(vocab, version string) error
	Commit() error
	Rollback() error
}

// TypologySummary is one registered typology plus the measured count of
// habitat types it carries — the count is a query result, not a stored field
// of domain.Typology, so it stays out of the domain type.
type TypologySummary struct {
	Typology     domain.Typology
	HabitatTypes int
}

type Repository interface {
	Begin(ctx context.Context) (IngestTx, error)
	// Typology returns the registered typology, or ErrNotFound when the index
	// carries no such classification system.
	Typology(ctx context.Context, id domain.TypologyID) (domain.Typology, error)
	// Typologies lists every registered typology together with the measured
	// count of habitat types it carries, sorted by id. A count of 0 is a
	// typology registered but not (yet) filled — a normal state, not an error.
	Typologies(ctx context.Context) ([]TypologySummary, error)
	HabitatType(ctx context.Context, key domain.HabitatTypeKey) (domain.HabitatType, error)
	// Description returns a habitat type's prose description, or ErrNotFound
	// when none was ingested. Absence is the normal case — the factsheets
	// describe EUNIS level 3, the index holds eight levels — so a caller maps
	// ErrNotFound to "no description", not to a failed request.
	Description(ctx context.Context, key domain.HabitatTypeKey) (domain.HabitatDescription, error)
	// CrosswalksTo returns every crosswalk whose To.Typology is typology.
	CrosswalksTo(ctx context.Context, typology domain.TypologyID) ([]domain.Crosswalk, error)
	// Crosswalks returns every crosswalk touching key, in either direction —
	// one stored row answers both the EUNIS and the Annex I entry point.
	Crosswalks(ctx context.Context, key domain.HabitatTypeKey) ([]domain.Crosswalk, error)
	// SpeciesRoles returns a habitat type's species; role filters when non-empty.
	SpeciesRoles(ctx context.Context, key domain.HabitatTypeKey, role string) ([]domain.SpeciesRole, error)
	// SpeciesRolesByConcept returns every role a resolved concept plays.
	SpeciesRolesByConcept(ctx context.Context, conceptID string) ([]domain.SpeciesRole, error)
	// Syntaxon returns one vegetation unit, or ErrNotFound. It distinguishes a
	// syntaxon that exists but is linked to nothing from one that does not
	// exist at all.
	Syntaxon(ctx context.Context, id string) (domain.Syntaxon, error)
	// SyntaxonByEEACode returns the vegetation unit whose eea_code equals code,
	// or ErrNotFound. Used as a fallback for a syntaxon id that changed to the
	// EVC primary code: the old EEA-style id (e.g. PAP-01A) is looked up here
	// once the primary lookup by id has failed. Measured unique (no eea_code
	// collides with another syntaxon's id, no eea_code is duplicated), so this
	// never has more than one candidate row.
	SyntaxonByEEACode(ctx context.Context, code string) (domain.Syntaxon, error)
	// Syntaxa returns the vegetation units linked to a habitat type.
	Syntaxa(ctx context.Context, key domain.HabitatTypeKey) ([]domain.Syntaxon, error)
	// AllSyntaxa returns every vegetation unit the index holds, in id order.
	// Used by the FloraVeg hierarchy-matching pass to find every
	// already-ingested EUNIS alliance to match its own names against.
	AllSyntaxa(ctx context.Context) ([]domain.Syntaxon, error)
	// SyntaxonChildren returns the direct children of parentID, ordered by id.
	// An alliance has none: an empty slice is the answer, not an error — that
	// is the lower bound of the free data (see the known ceiling).
	SyntaxonChildren(ctx context.Context, parentID string) ([]domain.Syntaxon, error)
	// SyntaxonAncestors walks parent_id to the root, OUTERMOST first
	// (formation, class, order), and is empty for a formation. An unknown id
	// is ErrNotFound; a parent_id pointing at a missing row and a cycle are
	// both index defects and are reported as plain errors that do NOT wrap
	// ErrNotFound, so a caller cannot turn them into a 404 for an id that
	// exists.
	SyntaxonAncestors(ctx context.Context, id string) ([]domain.Syntaxon, error)
	// HabitatTypeCountForSyntaxon counts the edges of exactly this syntaxon,
	// not its descendants', and without loading them.
	HabitatTypeCountForSyntaxon(ctx context.Context, syntaxonID string) (int, error)
	// SyntaxaByRank returns every syntaxon of rank, ordered by id. A non-empty
	// lifeFormGroup keeps only those whose reachable formation carries that
	// group — the value is stored on formation rows alone. A rank the index
	// does not carry yields an empty list here; turning that into an
	// INVALID_QUERY is the read side's job, which needs SyntaxonRanks for it.
	SyntaxaByRank(ctx context.Context, rank, lifeFormGroup string) ([]domain.Syntaxon, error)
	// SyntaxonRanks lists the distinct ranks the index carries, sorted.
	SyntaxonRanks(ctx context.Context) ([]string, error)
	// HabitatTypeKeysForSyntaxon returns the habitat types a syntaxon is linked
	// to — the m:n direction.
	HabitatTypeKeysForSyntaxon(ctx context.Context, syntaxonID string) ([]domain.HabitatTypeKey, error)
	// Localization returns every localization matching entityType, entityKey,
	// lang and field — there can be more than one, one per source.
	// Localization returns every localized field of one entity in one language.
	// Selecting a field is the caller's policy, not the port's: name and
	// vernacular belong to the same answer, and filtering here cost a second
	// query per habitat type.
	Localization(ctx context.Context, entityType, entityKey, lang string) ([]domain.Localization, error)
	// AreasForConcepts maps each concept id to the area codes it occurs in,
	// within one scheme. A concept absent from the result has no distribution
	// data at all — that is "unknown", not "does not occur".
	AreasForConcepts(ctx context.Context, conceptIDs []string, scheme string) (map[string][]string, error)
	// KnownAreaCodes lists the area codes the index has data for. An area
	// filter must be validated against this: an unknown code has to be an
	// error, not a list of "does not occur". Read per scheme from its own
	// table: wgsrpd_l3 codes live in species_distribution, evc_territory
	// codes in syntaxon_distribution, and there is never a row of one in
	// the other.
	KnownAreaCodes(ctx context.Context, scheme string) ([]string, error)
	// AreasWithData lists the areas the index has distribution data for,
	// each with its ingested name — what a client needs to offer an area
	// filter that can actually answer. An area with a name but no data is
	// not in it; an area with data but no name keeps an empty name. Per
	// scheme from its own distribution table, for the same reason
	// KnownAreaCodes branches.
	AreasWithData(ctx context.Context, scheme string) ([]domain.NamedArea, error)
	// SyntaxonDistribution returns what the source says about one syntaxon in
	// one area scheme: the verified and uncertain codes, sorted, plus whether
	// there is any statement at all (Covered). A syntaxon with no coverage row
	// comes back Covered false and both lists empty — which the read side must
	// serve as "no distribution field", never as "occurs nowhere".
	SyntaxonDistribution(ctx context.Context, syntaxonID, scheme string) (domain.SyntaxonDistribution, error)
	// SyntaxonOccurrencesInArea maps syntaxon id -> occurrence for one area.
	// A syntaxon absent from the map has no row for that area, which is NOT
	// the same as one absent from SyntaxaWithCoverage.
	SyntaxonOccurrencesInArea(ctx context.Context, scheme, code string) (map[string]string, error)
	// SyntaxaWithCoverage is the set of syntaxa the source makes a statement
	// about. The read side needs it to keep the unjudgeable rows in an
	// ?area=-filtered list instead of dropping them.
	SyntaxaWithCoverage(ctx context.Context, scheme string) (map[string]bool, error)
	// ConceptIDs lists the distinct concept ids the index holds, so the
	// distribution step knows what to ask for.
	ConceptIDs(ctx context.Context) ([]string, error)
	// SearchSpeciesNames returns the index's own verbatim names containing
	// q (case-insensitive substring), at most limit of them, ordered by
	// name so the same query is stably reproducible.
	//
	// This is NOT name resolution: no fuzzy matching, no synonyms, no
	// author variants. It answers only "which names does THIS index carry,
	// and under which concept id" — real resolution is hostus' job.
	SearchSpeciesNames(ctx context.Context, q string, limit int) ([]domain.SpeciesName, error)
	// Traits returns every domain.TraitSet situs holds for conceptID,
	// grouped PER VOCABULARY, never mixed. vocabs empty means every
	// ingested vocabulary; otherwise filtered (serves GET .../traits?vocab=).
	Traits(ctx context.Context, conceptID string, vocabs []string) ([]domain.TraitSet, error)
	// KnownVocabs lists the trait vocabularies the index has data for. A
	// ?vocab= filter must be validated against this: an unknown value is an
	// error, not an empty answer that could be a typo.
	KnownVocabs(ctx context.Context) ([]string, error)
	// TraitsForConcepts returns every trait value the index holds for each
	// of conceptIDs, keyed by concept id. Concepts without any trait data
	// are absent from the map rather than present-but-empty: "no data" and
	// "an empty list of data" are the same fact, and one representation is
	// enough.
	//
	// One read for the whole list, not one per concept: an analysis over a
	// vegetation record asks about dozens of species at once.
	TraitsForConcepts(ctx context.Context, conceptIDs []string) (map[string][]domain.TraitValue, error)
}

// A NameResolver can fail in two ways that must not be confused, because they
// point at different systems: ErrResolverUnavailable is the resolver not
// answering (transport failure, timeout, its own 5xx), ErrResolverRejected is
// the resolver answering that the request was wrong (its 4xx) — that is a bug or
// misconfiguration on this side.
var (
	ErrResolverUnavailable = errors.New("name resolver unavailable")
	ErrResolverRejected    = errors.New("name resolver rejected the request")
)

// NameResolver crosswalks verbatim species names to concept IDs via hostus.
// The returned map omits any name that did not resolve — an absent key means
// unresolvable, never an empty-string concept id, so a present key alone is
// enough to treat a name as resolved. Callers deduplicate names before calling;
// an implementation batches and posts whatever it is given.
type NameResolver interface {
	Resolve(ctx context.Context, names []string) (map[string]string, error)
}

// DistributionSource yields the areas a concept occurs in. Separate from
// NameResolver on purpose: different question (concept -> areas, not name ->
// concept) and different failure semantics — a distribution outage must not
// abort an ingest, an unresolvable name path must.
type DistributionSource interface {
	Areas(ctx context.Context, conceptIDs []string) (map[string][]domain.Area, error)
}
