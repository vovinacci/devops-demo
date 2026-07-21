// Package otelsetup wires OpenTelemetry tracing (RFC-0001 D11, ADR-0010):
// service.name=analytics, W3C trace-context propagation, no exporter.
// Mirrors services/backend/app/otel.py and services/canary/src/otel.rs --
// every service is instrumented from its first commit; the trace
// *backend* (Tempo) is deferred, so spans are generated and propagated
// then dropped, not shipped anywhere. Trace IDs still land in structured
// logs (internal/logging) for correlation via Loki in the meantime.
package otelsetup

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Init installs the global TracerProvider and W3C propagator. The
// returned provider must stay alive for the process lifetime and be
// Shutdown on exit, or spans stop recording the moment it is dropped.
func Init(_ context.Context) *sdktrace.TracerProvider {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	res := resource.NewSchemaless(attribute.String("service.name", "analytics"))

	// No span processor/exporter registered: this is the "no exporter
	// configured" half of D11 -- spans exist for ID generation and
	// propagation only.
	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)
	return tp
}
