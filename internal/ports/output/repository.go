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
	// UpsertSyntaxonAuthor sets Name and Author on an already-upserted syntaxon
	// and, if parentID is non-empty, its ParentID — used ONLY by the
	// hierarchy-matching pass to enrich an existing EUNIS alliance row on a
	// FloraVeg match. name is FloraVeg's own clean name, replacing the
	// historical EUNIS combi-string; it is always set (a match always has a
	// real FloraVeg name), unlike parentID, which can legitimately be empty.
	UpsertSyntaxonAuthor(id, name, author, parentID string) error
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

type Repository interface {
	Begin(ctx context.Context) (IngestTx, error)
	// Typology returns the registered typology, or ErrNotFound when the index
	// carries no such classification system.
	Typology(ctx context.Context, id domain.TypologyID) (domain.Typology, error)
	HabitatType(ctx context.Context, key domain.HabitatTypeKey) (domain.HabitatType, error)
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
	// Syntaxa returns the vegetation units linked to a habitat type.
	Syntaxa(ctx context.Context, key domain.HabitatTypeKey) ([]domain.Syntaxon, error)
	// AllSyntaxa returns every vegetation unit the index holds, in id order.
	// Used by the FloraVeg hierarchy-matching pass to find every
	// already-ingested EUNIS alliance to match its own names against.
	AllSyntaxa(ctx context.Context) ([]domain.Syntaxon, error)
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
	// error, not a list of "does not occur".
	KnownAreaCodes(ctx context.Context, scheme string) ([]string, error)
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
