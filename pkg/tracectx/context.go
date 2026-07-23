// Package tracectx provides trace-context access without depending on an
// application layer.
package tracectx

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// SpanIDFromContext returns the span ID stored in ctx, if present.
func SpanIDFromContext(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.HasSpanID() {
		return spanCtx.SpanID().String()
	}

	return ""
}

// TraceIDFromContext returns the trace ID stored in ctx, if present.
func TraceIDFromContext(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.HasTraceID() {
		return spanCtx.TraceID().String()
	}

	return ""
}
