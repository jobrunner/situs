package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// DistributionReport is what an operator needs to judge the distribution step:
// how many concepts the index holds, how many of them the source knew, how
// many rows that produced, how many areas were dropped for being incomplete,
// and how many concept requests the source gave up on without aborting the
// whole batch. WithAreas == 0 is a visible statement.
type DistributionReport struct {
	Concepts   int
	WithAreas  int
	Rows       int
	Incomplete int
	Failed     int
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
		// A canceled/expired context is not a source outage to tolerate: the
		// caller (or a deadline) asked this run to stop, and it must fail
		// where that happened, not be swallowed here and surface later as an
		// unrelated failure in the next ingest step.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return rep, fmt.Errorf("distribution source call aborted: %w", err)
		}
		slog.WarnContext(ctx, "distribution source unavailable, the index stays unfiltered",
			"concepts", len(ids), "error", err)
		return rep, nil
	}
	if pf, ok := src.(output.PartialDistributionSource); ok {
		rep.Failed = pf.FailedConcepts()
	}
	if len(areas) == 0 {
		return rep, nil
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return rep, fmt.Errorf("beginning the distribution ingest: %w", err)
	}
	if err := upsertDistributionAreas(tx, ids, areas, &rep); err != nil {
		return rep, fmt.Errorf("%w (rollback: %w)", err, tx.Rollback())
	}
	if err := tx.Commit(); err != nil {
		return rep, fmt.Errorf("committing the distribution ingest: %w", err)
	}
	return rep, nil
}

// upsertDistributionAreas writes every area of every id via tx, tallying rep
// as it goes. A source implementing only DistributionSource (not the
// stricter hostus adapter contract) could hand back a half-filled Area; the
// writer, not just the one known reader, must not let that reach the index
// as empty-string rows.
func upsertDistributionAreas(tx output.IngestTx, ids []string, areas map[string][]domain.Area, rep *DistributionReport) error {
	for _, id := range ids {
		list := areas[id]
		if len(list) == 0 {
			continue
		}
		rep.WithAreas++
		for _, a := range list {
			if !a.IsComplete() {
				rep.Incomplete++
				continue
			}
			if err := tx.UpsertDistribution(id, a); err != nil {
				return err
			}
			rep.Rows++
		}
	}
	return nil
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
