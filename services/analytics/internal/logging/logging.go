// Package logging provides structured JSON logging to stdout with the
// active OpenTelemetry span's trace ID attached to every record
// (RFC-0001 D6, D11) -- until Tempo lands, this is the correlation
// mechanism: search logs by trace_id to see every step of one request.
package logging

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// zeroTraceID is emitted when no span is active on the context, so every
// log line carries the trace_id field whether or not the call site is
// currently traced.
const zeroTraceID = "00000000000000000000000000000000"

type handler struct {
	slog.Handler
}

// New returns a JSON slog.Logger to stdout with trace_id injection.
func New() *slog.Logger {
	base := slog.NewJSONHandler(os.Stdout, nil)
	return slog.New(&handler{Handler: base})
}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	traceID := zeroTraceID
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		traceID = span.TraceID().String()
	}
	r.AddAttrs(slog.String("trace_id", traceID))
	return h.Handler.Handle(ctx, r)
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &handler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{Handler: h.Handler.WithGroup(name)}
}
