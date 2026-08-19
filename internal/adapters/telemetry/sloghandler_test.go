package telemetry_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/jobrunner/situs/internal/adapters/telemetry"
)

func newLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(telemetry.NewSpanContextHandler(
		slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}),
	))
}

func TestHandlerAddsTraceIDWhenContextCarriesASpan(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf)

	traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	if err != nil {
		t.Fatalf("building trace id: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("0102030405060708")
	if err != nil {
		t.Fatalf("building span id: %v", err)
	}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))

	logger.InfoContext(ctx, "request")

	got := buf.String()
	if !strings.Contains(got, "trace_id=0102030405060708090a0b0c0d0e0f10") {
		t.Errorf("log = %q, want it to carry the trace_id", got)
	}
	if !strings.Contains(got, "span_id=0102030405060708") {
		t.Errorf("log = %q, want it to carry the span_id", got)
	}
}

func TestHandlerLogsWithoutTraceIDWhenThereIsNoSpan(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf)

	logger.InfoContext(context.Background(), "request")

	got := buf.String()
	if !strings.Contains(got, "request") {
		t.Errorf("log = %q, want the message to pass through", got)
	}
	if strings.Contains(got, "trace_id") {
		t.Errorf("log = %q, want no trace_id without a span", got)
	}
}

func TestHandlerKeepsAttrsAndGroupsOfTheInnerHandler(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf).With("service", "situs").WithGroup("http")

	logger.Info("request", "status", 200)

	got := buf.String()
	if !strings.Contains(got, "service=situs") {
		t.Errorf("log = %q, want the WithAttrs attribute", got)
	}
	if !strings.Contains(got, "http.status=200") {
		t.Errorf("log = %q, want the WithGroup prefix", got)
	}
}

func TestHandlerRespectsTheInnerLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(telemetry.NewSpanContextHandler(
		slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}),
	))

	logger.Info("suppressed")

	if buf.Len() != 0 {
		t.Errorf("log = %q, want nothing below the inner handler's level", buf.String())
	}
}
