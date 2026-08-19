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
	LinkSyntaxon(key domain.HabitatTypeKey, syntaxonID string) error
	UpsertSpeciesRole(r domain.SpeciesRole) error
	UpsertLocalization(l domain.Localization) error
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
	// HabitatTypeKeysForSyntaxon returns the habitat types a syntaxon is linked
	// to — the m:n direction.
	HabitatTypeKeysForSyntaxon(ctx context.Context, syntaxonID string) ([]domain.HabitatTypeKey, error)
	// Localization returns every localization matching entityType, entityKey,
	// lang and field — there can be more than one, one per source.
	Localization(ctx context.Context, entityType, entityKey, lang, field string) ([]domain.Localization, error)
}

// NameResolver crosswalks verbatim species names to concept IDs via hostus.
// The returned map omits any name that did not resolve — an absent key means
// unresolvable, never an empty-string concept id.
type NameResolver interface {
	Resolve(ctx context.Context, names []string) (map[string]string, error)
}
