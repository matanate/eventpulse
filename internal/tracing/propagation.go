package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// InjectMap writes the W3C traceparent from the active span in ctx into headers.
// No-op when no span is active (e.g. tracing disabled).
func InjectMap(ctx context.Context, headers map[string]string) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(headers))
}

// ExtractMap reads a W3C traceparent from headers and returns a context with
// the remote span context attached as a parent for the next span.
func ExtractMap(ctx context.Context, headers map[string]string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(headers))
}

// TraceID returns the hex-encoded trace ID of the active span in ctx,
// or an empty string when no span is active or tracing is disabled.
func TraceID(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}
