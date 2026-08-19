// Package output holds the driven ports — what the application needs from the
// outside world. Adapters implement them; the composition root injects them.
// Later tasks add Repository, IngestTx and NameResolver here.
package output

import "context"

// Tracer is the driven port for distributed tracing. It keeps OpenTelemetry out
// of domain and application code. The default implementation is NoOpTracer, so
// the composition root can always inject a non-nil Tracer.
type Tracer interface {
	// Start opens a span and returns a context carrying it. The caller must End
	// the span exactly once. If ctx already carries a span, the new one becomes
	// its child.
	Start(ctx context.Context, name string, opts ...StartSpanOption) (context.Context, Span)
}

// Span is an in-progress operation being traced.
type Span interface {
	SetAttributes(attrs ...Attribute)
	AddEvent(name string, attrs ...Attribute)
	RecordError(err error)
	SetStatus(code StatusCode, description string)
	End()
}

// StatusCode is the outcome of a span.
type StatusCode int

// Span outcomes.
const (
	StatusUnset StatusCode = iota
	StatusOK
	StatusError
)

// Attribute is a typed key/value pair on a span.
type Attribute struct {
	Key   string
	Value any
}

// StartSpanOption configures a span at creation time.
type StartSpanOption func(*startSpanConfig)

type startSpanConfig struct {
	attributes []Attribute
}

// WithAttributes sets initial attributes on the span.
func WithAttributes(attrs ...Attribute) StartSpanOption {
	return func(c *startSpanConfig) { c.attributes = append(c.attributes, attrs...) }
}

// NoOpTracer is the zero-value Tracer; it discards everything.
type NoOpTracer struct{}

// Start implements Tracer.
func (NoOpTracer) Start(ctx context.Context, _ string, _ ...StartSpanOption) (context.Context, Span) {
	return ctx, noOpSpan{}
}

type noOpSpan struct{}

func (noOpSpan) SetAttributes(_ ...Attribute)      {}
func (noOpSpan) AddEvent(_ string, _ ...Attribute) {}
func (noOpSpan) RecordError(_ error)               {}
func (noOpSpan) SetStatus(_ StatusCode, _ string)  {}
func (noOpSpan) End()                              {}

var _ Tracer = NoOpTracer{}
