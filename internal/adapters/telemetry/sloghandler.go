// Package telemetry holds the observability adapters. SpanContextHandler is the
// slog decorator that correlates logs with traces.
package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// SpanContextHandler is a slog.Handler decorator that injects the current span's
// trace_id and span_id into every record carrying a context, so logs and traces
// correlate.
type SpanContextHandler struct {
	inner slog.Handler
}

// NewSpanContextHandler wraps an existing handler.
func NewSpanContextHandler(inner slog.Handler) *SpanContextHandler {
	return &SpanContextHandler{inner: inner}
}

// Enabled implements slog.Handler.
func (h *SpanContextHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

// Handle implements slog.Handler.
func (h *SpanContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, r)
}

// WithAttrs implements slog.Handler.
func (h *SpanContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SpanContextHandler{inner: h.inner.WithAttrs(attrs)}
}

// WithGroup implements slog.Handler.
func (h *SpanContextHandler) WithGroup(name string) slog.Handler {
	return &SpanContextHandler{inner: h.inner.WithGroup(name)}
}

var _ slog.Handler = (*SpanContextHandler)(nil)
