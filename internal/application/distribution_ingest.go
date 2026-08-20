package application

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/jobrunner/situs/internal/ports/output"
)

// DistributionReport is what an operator needs to judge the distribution step:
// how many concepts the index holds, how many of them the source knew, and how
// many rows that produced. WithAreas == 0 is a visible statement.
type DistributionReport struct {
	Concepts  int
	WithAreas int
	Rows      int
}

// IngestDistribution copies the areas of every indexed concept into the index.
//
// A source failure is reported, not returned: the distribution is extra
// information and an index without it is usable, only unfiltered. This is
// deliberately unlike IngestSpeciesRoles, where a resolver failure aborts so
// that every name is not booked as unresolvable.
func IngestDistribution(ctx context.Context, repo output.Repository, src output.DistributionSource) (DistributionReport, error) {
	ids, err := indexedConceptIDs(ctx, repo)
	if err != nil {
		return DistributionReport{}, err
	}
	rep := DistributionReport{Concepts: len(ids)}
	if len(ids) == 0 {
		return rep, nil
	}

	areas, err := src.Areas(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "distribution source unavailable, the index stays unfiltered",
			"concepts", len(ids), "error", err)
		return rep, nil
	}
	if len(areas) == 0 {
		return rep, nil
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return rep, fmt.Errorf("beginning the distribution ingest: %w", err)
	}
	for _, id := range ids {
		list := areas[id]
		if len(list) == 0 {
			continue
		}
		rep.WithAreas++
		for _, a := range list {
			if err := tx.UpsertDistribution(id, a); err != nil {
				return rep, fmt.Errorf("%w (rollback: %w)", err, tx.Rollback())
			}
			rep.Rows++
		}
	}
	if err := tx.Commit(); err != nil {
		return rep, fmt.Errorf("committing the distribution ingest: %w", err)
	}
	return rep, nil
}

// indexedConceptIDs returns the distinct concept ids the index holds, sorted so
// a run is reproducible.
func indexedConceptIDs(ctx context.Context, repo output.Repository) ([]string, error) {
	ids, err := repo.ConceptIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the indexed concept ids: %w", err)
	}
	sort.Strings(ids)
	return ids, nil
}
