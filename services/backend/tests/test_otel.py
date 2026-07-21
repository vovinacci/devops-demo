"""OpenTelemetry retrofit tests (RFC-0001 D11, ADR-0010).

Production attaches no SpanProcessor at all (see app.otel) -- spans are
real but never exported. `span_exporter` below adds one processor to
the same global provider just for these tests, so assertions can see
real span data without changing what ships.
"""

import logging

import grpc
import pytest
from opentelemetry import trace
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter

from app.grpc_server import build_grpc_server, items_pb2, items_pb2_grpc
from app.otel import TraceIdFilter

# W3C spec example traceparent (https://www.w3.org/TR/trace-context/#examples-of-http-traceparent-headers)
KNOWN_TRACEPARENT = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
KNOWN_TRACE_ID = "4bf92f3577b34da6a3ce929d0e0e4736"
ZERO_TRACE_ID = "0" * 32


# The SDK's TracerProvider has no public remove_span_processor, so a
# per-test processor would accumulate on the global provider across the
# module. Attach exactly one processor for the whole module and clear
# the shared exporter before each test instead.
_EXPORTER = InMemorySpanExporter()
_PROCESSOR_ATTACHED = False


@pytest.fixture
def span_exporter():
    global _PROCESSOR_ATTACHED
    if not _PROCESSOR_ATTACHED:
        trace.get_tracer_provider().add_span_processor(SimpleSpanProcessor(_EXPORTER))
        _PROCESSOR_ATTACHED = True
    _EXPORTER.clear()
    yield _EXPORTER


def _trace_ids(exporter: InMemorySpanExporter) -> list[str]:
    return [format(s.context.trace_id, "032x") for s in exporter.get_finished_spans()]


@pytest.mark.asyncio
async def test_http_traceparent_propagates_to_server_span(client, span_exporter):
    r = await client.get("/items", headers={"traceparent": KNOWN_TRACEPARENT})
    assert r.status_code == 200
    trace_ids = _trace_ids(span_exporter)
    assert trace_ids, "expected FastAPI instrumentation to create a server span"
    assert all(tid == KNOWN_TRACE_ID for tid in trace_ids)


@pytest.mark.asyncio
async def test_http_no_traceparent_gets_a_fresh_trace_id(client, span_exporter):
    r = await client.get("/items")
    assert r.status_code == 200
    trace_ids = _trace_ids(span_exporter)
    assert trace_ids
    for tid in trace_ids:
        assert tid != ZERO_TRACE_ID
        assert tid != KNOWN_TRACE_ID


@pytest.mark.asyncio
@pytest.mark.parametrize("path", ["/healthz", "/readyz", "/health", "/metrics"])
async def test_excluded_routes_create_no_server_span(client, span_exporter, path):
    r = await client.get(path, headers={"traceparent": KNOWN_TRACEPARENT})
    assert r.status_code == 200
    assert span_exporter.get_finished_spans() == ()


@pytest.mark.asyncio
async def test_metrics_frontend_is_still_traced(client, span_exporter):
    """The /metrics$ exclusion is end-anchored: the real web-vitals route
    under /metrics/frontend must keep producing server spans."""
    r = await client.post(
        "/metrics/frontend",
        json={"name": "LCP", "value": 1.2},
        headers={"traceparent": KNOWN_TRACEPARENT},
    )
    assert r.status_code == 200
    trace_ids = _trace_ids(span_exporter)
    assert trace_ids
    assert all(tid == KNOWN_TRACE_ID for tid in trace_ids)


@pytest.mark.asyncio
async def test_grpc_traceparent_metadata_propagates_to_server_span(span_exporter):
    server = await build_grpc_server()
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        channel = grpc.aio.insecure_channel(f"127.0.0.1:{port}")
        try:
            stub = items_pb2_grpc.ItemServiceStub(channel)
            await stub.GetItemStats(
                items_pb2.GetItemStatsRequest(),
                metadata=(("traceparent", KNOWN_TRACEPARENT),),
            )
        finally:
            await channel.close()
    finally:
        await server.stop(grace=None)

    trace_ids = _trace_ids(span_exporter)
    assert trace_ids, "expected the gRPC otel interceptor to create a server span"
    assert all(tid == KNOWN_TRACE_ID for tid in trace_ids)


def test_trace_id_filter_placeholder_when_no_span():
    """No active span -> the zero-trace_id placeholder, and filtering
    never raises (must not crash uninstrumented log call sites)."""
    record = logging.LogRecord("test", logging.INFO, __file__, 1, "msg", None, None)
    assert TraceIdFilter().filter(record) is True
    assert record.trace_id == ZERO_TRACE_ID


def test_trace_id_filter_matches_active_span_trace_id():
    tracer = trace.get_tracer("test-otel")
    with tracer.start_as_current_span("probe") as span:
        record = logging.LogRecord("test", logging.INFO, __file__, 1, "msg", None, None)
        TraceIdFilter().filter(record)
        assert record.trace_id == format(span.get_span_context().trace_id, "032x")
