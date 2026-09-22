package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jobrunner/situs/internal/domain"
)

// fakeDeadlineSource fails every request — or only the one id failOn names —
// with an error whose chain carries context.DeadlineExceeded, the shape an
// http.Client.Timeout produces while the caller's ctx is alive.
type fakeDeadlineSource struct {
	failOn string
}

func (f *fakeDeadlineSource) Areas(_ context.Context, ids []string) (map[string][]domain.Area, error) {
	id := ids[0]
	if f.failOn == "" || id == f.failOn {
		return nil, fmt.Errorf("calling hostus for %s: %w", id, context.DeadlineExceeded)
	}
	return map[string][]domain.Area{id: {{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}}}, nil
}

// One request timing out is a per-concept outage, not the run being told to
// stop: the ctx is the authority on that, not the error's chain.
func TestPacedDistributionSource_ToleratesARequestTimeoutWhileTheContextIsAlive(t *testing.T) {
	paced := &pacedDistributionSource{src: &fakeDeadlineSource{failOn: "b"}, pause: time.Millisecond}

	got, err := paced.Areas(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Areas: %v, want one timed-out concept to be tolerated", err)
	}
	if _, ok := got["a"]; !ok {
		t.Error("concept a is missing, want it kept despite b's timeout")
	}
	if _, ok := got["c"]; !ok {
		t.Error("concept c is missing, want it kept despite b's timeout")
	}
	if n := paced.FailedConcepts(); n != 1 {
		t.Errorf("FailedConcepts() = %d, want 1", n)
	}
}

// Every request timing out is still a source outage, and IngestDistribution
// tells that from a stopped run by the error's chain. So the whole-batch
// error must not carry the per-request deadline upward, or the documented
// warn-and-zero path turns into a failed ingest.
func TestPacedDistributionSource_AllRequestsTimingOutIsNotReportedAsAnExpiredContext(t *testing.T) {
	paced := &pacedDistributionSource{src: &fakeDeadlineSource{}, pause: time.Millisecond}

	_, err := paced.Areas(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("Areas: want an error when every request failed, got nil")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want it NOT to satisfy errors.Is(context.DeadlineExceeded): the run's ctx never expired", err)
	}
}

// The ctx being done is what ends the run, whatever the source returns for
// the request that was in flight when it happened.
func TestPacedDistributionSource_ADoneContextEndsTheRunEvenOnATimeoutError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	paced := &pacedDistributionSource{src: &fakeDeadlineSource{}, pause: time.Millisecond}

	_, err := paced.Areas(ctx, []string{"a", "b"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Areas returned %v, want context.Canceled", err)
	}
}
