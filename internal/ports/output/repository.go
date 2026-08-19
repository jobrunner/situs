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
	HabitatType(ctx context.Context, key domain.HabitatTypeKey) (domain.HabitatType, error)
	// CrosswalksTo returns every crosswalk whose To.Typology is typology.
	CrosswalksTo(ctx context.Context, typology domain.TypologyID) ([]domain.Crosswalk, error)
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
