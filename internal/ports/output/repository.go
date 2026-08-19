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
}
