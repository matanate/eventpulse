package tracing_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/matanate/eventpulse/internal/tracing"
)

func TestInjectExtractRoundTrip(t *testing.T) {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "root")
	defer span.End()
	wantTraceID := span.SpanContext().TraceID()

	headers := make(map[string]string)
	tracing.InjectMap(ctx, headers)

	if headers["traceparent"] == "" {
		t.Fatal("expected traceparent header to be set after inject")
	}

	// Verify the trace ID is present in the traceparent header value.
	if len(headers["traceparent"]) < 32 {
		t.Fatalf("traceparent value too short: %q", headers["traceparent"])
	}

	// Extract into a fresh context, then start a child span ג€” it must inherit the trace ID.
	extracted := tracing.ExtractMap(context.Background(), headers)
	_, child := tp.Tracer("test").Start(extracted, "child")
	defer child.End()

	if child.SpanContext().TraceID() != wantTraceID {
		t.Errorf("trace ID mismatch after propagation: got %s, want %s",
			child.SpanContext().TraceID(), wantTraceID)
	}
}

func TestExtractEmptyHeaders(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	// Extracting empty headers must not panic and must return a valid context.
	ctx := tracing.ExtractMap(context.Background(), map[string]string{})
	if ctx == nil {
		t.Error("ExtractMap returned nil context")
	}
}

func TestTraceIDNoActiveSpan(t *testing.T) {
	id := tracing.TraceID(context.Background())
	if id != "" {
		t.Errorf("expected empty trace ID for context with no active span, got %q", id)
	}
}

func TestTraceIDWithActiveSpan(t *testing.T) {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "root")
	defer span.End()

	id := tracing.TraceID(ctx)
	if id == "" {
		t.Fatal("expected non-empty trace ID for context with active span")
	}
	if id != span.SpanContext().TraceID().String() {
		t.Errorf("trace ID mismatch: got %s, want %s", id, span.SpanContext().TraceID().String())
	}
}
