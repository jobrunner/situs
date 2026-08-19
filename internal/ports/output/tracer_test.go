package output_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/situs/internal/ports/output"
)

// The NoOp tracer is what the composition root injects while tracing is
// disabled, so "it discards everything without touching the context" is a real
// guarantee the rest of the code relies on.
func TestNoOpTracerReturnsTheSameContextAndASilentSpan(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKey{}, "kept")

	gotCtx, span := output.NoOpTracer{}.Start(ctx, "ingest",
		output.WithAttributes(output.Attribute{Key: "rows", Value: 3}))

	if gotCtx.Value(ctxKey{}) != "kept" {
		t.Error("Start returned a context that lost its values")
	}
	if span == nil {
		t.Fatal("Start returned a nil span — callers must never nil-check")
	}

	// Every span method must be safe to call and do nothing observable.
	span.SetAttributes(output.Attribute{Key: "code", Value: "R22"})
	span.AddEvent("upserted")
	span.RecordError(errors.New("boom"))
	span.SetStatus(output.StatusError, "boom")
	span.End()
}

type ctxKey struct{}
