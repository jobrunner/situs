package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// maxLoggedConceptFailures caps how many individual per-concept failures get
// their own log line. Beyond that, the run-end aggregate line (which always
// fires once len(failed) > 0) says how many there were — a real outage on
// this call must not put thousands of nearly identical lines in the log.
const maxLoggedConceptFailures = 3

// pacedDistributionSource wraps a DistributionSource that has no pacing of
// its own (Areas issues one hostus request per concept) and spaces those
// requests out, one concept at a time, so a full ingest run does not fail in
// a wall of 429s.
//
// It also tolerates individual concept requests failing instead of
// discarding the whole batch: a timeout on the last few hundred concepts must
// not throw away minutes of work and leave the index unfiltered.
// FailedConcepts reports how many of the last Areas call's requests were
// tolerated this way.
// A canceled/expired context is the one failure that is not tolerated —
// that is the run being told to stop, not a data problem, and it must fail
// here, not resurface as an unrelated error two ingest steps later. If every
// single request fails, Areas reports that as a whole-batch failure (nil
// map, error) so IngestDistribution treats it exactly like the previous
// all-or-nothing behavior: zeros in the report, plus the warning — and
// FailedConcepts resets to 0 for that call, since the count only means
// something for a call that otherwise returned a usable partial result.
//
// Its own file: the per-file complexity ratchet (scripts/codecharta-ratchet.py)
// sums complexity over a whole file, and Areas' branching (pacing, per-concept
// tolerance, context cancellation) is a cohesive unit best measured on its own
// rather than crowding ingest.go's orchestration — same rationale as
// internal/application/syntaxa_read.go's split from syntaxa_ingest.go.
type pacedDistributionSource struct {
	src    output.DistributionSource
	pause  time.Duration
	failed int
}

func (p *pacedDistributionSource) Areas(ctx context.Context, conceptIDs []string) (map[string][]domain.Area, error) {
	out := map[string][]domain.Area{}
	p.failed = 0
	var lastErr error
	for i, id := range conceptIDs {
		if i > 0 {
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(p.pause):
			}
		}
		areas, err := p.src.Areas(ctx, []string{id})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return out, err
			}
			p.failed++
			lastErr = err
			if p.failed <= maxLoggedConceptFailures {
				slog.WarnContext(ctx, "distribution request for one concept failed, continuing with the rest",
					"concept_id", id, "error", err)
			}
			continue
		}
		for k, v := range areas {
			out[k] = v
		}
	}
	if p.failed > 0 {
		slog.WarnContext(ctx, "some distribution requests failed, the index will be partially filtered",
			"failed", p.failed, "requested", len(conceptIDs))
	}
	if len(conceptIDs) > 0 && p.failed == len(conceptIDs) {
		err := fmt.Errorf("all %d distribution requests failed, last error: %w", p.failed, lastErr)
		p.failed = 0
		return nil, err
	}
	return out, nil
}

// FailedConcepts reports how many concept requests the last Areas call
// tolerated instead of aborting on. 0 both when nothing failed and when
// everything failed (see the type doc comment) — it answers "how many were
// skipped in an otherwise-successful run", not "was there any failure".
func (p *pacedDistributionSource) FailedConcepts() int { return p.failed }
