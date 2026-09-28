package trace

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// SpanIDFromContext returns the span id from ctx, or an empty string.
func SpanIDFromContext(ctx context.Context) string {
	span := trace.SpanContextFromContext(ctx)
	if span.HasSpanID() {
		return span.SpanID().String()
	}
	return ""
}

// TraceIDFromContext returns the trace id from ctx, or an empty string.
func TraceIDFromContext(ctx context.Context) string {
	span := trace.SpanContextFromContext(ctx)
	if span.HasTraceID() {
		return span.TraceID().String()
	}
	return ""
}

// TracerFromContext returns a tracer in ctx, otherwise returns a global tracer.
func TracerFromContext(ctx context.Context) (tracer trace.Tracer) {
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		tracer = span.TracerProvider().Tracer(TraceName)
	} else {
		tracer = otel.Tracer(TraceName)
	}
	return
}
