"""OpenTelemetry setup for the backend (RFC-0001 D11, ADR-0010).

Mirrors services/canary/src/otel.rs: every span is created and recorded
with a real, random W3C trace ID -- but no SpanProcessor is ever
attached to the provider, so nothing tries to export a span anywhere
(the trace *backend*, Tempo, is deferred; see ADR-0010). The Rust SDK
needs a no-op exporter to express "record but don't ship" because a
SpanProcessor there requires one; the Python SDK has no such
requirement -- a TracerProvider with zero processors already records
real spans (real IDs, real parent/child links) that are simply never
handed to anything. Trace IDs still reach structured logs via
`TraceIdFilter` below, which is the correlation mechanism until Tempo
lands.
"""

import logging

import grpc
from opentelemetry import trace
from opentelemetry.instrumentation.grpc import aio_server_interceptor
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.sampling import ALWAYS_ON, ParentBased


def setup_tracing() -> None:
    """Installs the global TracerProvider. Must run before the FastAPI
    and gRPC instrumentation below, and before any span is created --
    both read the global provider at instrument-time."""
    provider = TracerProvider(
        resource=Resource.create({"service.name": "backend"}),
        sampler=ParentBased(ALWAYS_ON),
    )
    trace.set_tracer_provider(provider)


def grpc_server_interceptor() -> grpc.aio.ServerInterceptor:
    """W3C trace-context extraction for the gRPC server (D11).

    Uses opentelemetry-instrumentation-grpc's `aio_server_interceptor()`
    factory directly, added to `build_grpc_server()`'s interceptors list
    the same way `_RedMetricsInterceptor` already is -- rather than the
    package's other public entry point, `GrpcAioInstrumentorServer`,
    which globally monkeypatches `grpc.aio.server` at `.instrument()`
    time to inject itself. That makes whether tracing is wired at all
    depend on import order (only `grpc.aio.server()` calls made *after*
    `.instrument()` runs are affected) -- awkward here, where
    tests/test_grpc.py builds servers via `build_grpc_server()` directly.
    The factory function has no such ordering hazard: it returns a plain
    `grpc.aio.ServerInterceptor` instance, composes explicitly and
    deterministically with `_RedMetricsInterceptor`, and (placed first in
    the list, so it wraps outermost) establishes trace context before the
    metrics interceptor runs.
    """
    return aio_server_interceptor()


class TraceIdFilter(logging.Filter):
    """Injects `record.trace_id` -- 32 lowercase hex chars, all zeros if
    no span is active -- so the basicConfig format string in app.main
    can correlate a log line with a trace (D11/ADR-0010: Tempo is
    deferred, so this, not a trace UI, is how a log line maps to a
    request today)."""

    def filter(self, record: logging.LogRecord) -> bool:
        span_context = trace.get_current_span().get_span_context()
        record.trace_id = format(span_context.trace_id, "032x") if span_context.is_valid else "0" * 32
        return True


def install_trace_id_log_filter() -> None:
    """Attaches `TraceIdFilter` to the root logger and its handlers.

    The handler attachment is the one that matters: a `Filter` on a
    Logger only runs for records originating at that exact logger, not
    for records propagating up from descendants -- Python's log
    propagation walks the handler chain, it does not re-run each
    ancestor Logger's `filter()`. `app.main`, `app.grpc_server`, and
    every other module here use `logging.getLogger(__name__)` and
    propagate to root by default, so attaching to root's handler(s)
    (installed by `logging.basicConfig` in app.main) covers all of them.
    Also attaching to the root Logger itself is belt-and-suspenders, for
    anything logged directly via the root logger.
    """
    root = logging.getLogger()
    trace_filter = TraceIdFilter()
    root.addFilter(trace_filter)
    for handler in root.handlers:
        handler.addFilter(trace_filter)
