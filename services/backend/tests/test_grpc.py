import asyncio

import grpc
import pytest
import pytest_asyncio
from grpc_health.v1 import health_pb2, health_pb2_grpc
from prometheus_client import REGISTRY

from app.crud import create_item, delete_item
from app.events import broadcaster
from app.grpc_server import ITEM_SERVICE_FULL_NAME, build_grpc_server, items_pb2, items_pb2_grpc
from app.schemas import ItemCreate


@pytest_asyncio.fixture
async def grpc_channel():
    """In-process server bound to an ephemeral port -- same servicer, health
    service, and metrics interceptor as production (app.grpc_server), just
    not on the fixed :50051 (RFC-0001 D3, ADR-0002)."""
    server = await build_grpc_server()
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    channel = grpc.aio.insecure_channel(f"127.0.0.1:{port}")
    try:
        yield channel
    finally:
        await channel.close()
        await server.stop(grace=None)


async def _wait_for_subscriber(timeout: float = 2.0) -> None:
    """Poll until WatchItemEvents has subscribed server-side -- the gRPC
    call is dispatched asynchronously, so a test must not publish an event
    before the servicer has called broadcaster.subscribe()."""
    loop = asyncio.get_event_loop()
    deadline = loop.time() + timeout
    while not broadcaster._subscribers:
        if loop.time() > deadline:
            pytest.fail("WatchItemEvents never subscribed")
        await asyncio.sleep(0.01)


@pytest.mark.asyncio
async def test_list_items_returns_seeded_items(grpc_channel):
    stub = items_pb2_grpc.ItemServiceStub(grpc_channel)
    resp = await stub.ListItems(items_pb2.ListItemsRequest())
    assert len(resp.items) == 10  # seeded by the autouse _db_seed_session fixture


@pytest.mark.asyncio
async def test_get_item_stats_matches_item_count(grpc_channel):
    stub = items_pb2_grpc.ItemServiceStub(grpc_channel)
    resp = await stub.GetItemStats(items_pb2.GetItemStatsRequest())
    assert resp.total_items == 10


@pytest.mark.asyncio
async def test_health_check_serving(grpc_channel):
    stub = health_pb2_grpc.HealthStub(grpc_channel)

    overall = await stub.Check(health_pb2.HealthCheckRequest(service=""))
    assert overall.status == health_pb2.HealthCheckResponse.SERVING

    item_service = await stub.Check(health_pb2.HealthCheckRequest(service=ITEM_SERVICE_FULL_NAME))
    assert item_service.status == health_pb2.HealthCheckResponse.SERVING


@pytest.mark.asyncio
async def test_watch_item_events_created_then_deleted(grpc_channel, db):
    stub = items_pb2_grpc.ItemServiceStub(grpc_channel)
    call = stub.WatchItemEvents(items_pb2.WatchItemEventsRequest())
    await _wait_for_subscriber()

    item = await create_item(db, ItemCreate(name="grpc-watch-test"))
    created = await asyncio.wait_for(call.read(), timeout=2)
    assert created.event_type == items_pb2.EVENT_TYPE_CREATED
    assert created.item.id == item.id
    assert created.item.name == "grpc-watch-test"

    ok = await delete_item(db, item.id)
    assert ok
    deleted = await asyncio.wait_for(call.read(), timeout=2)
    assert deleted.event_type == items_pb2.EVENT_TYPE_DELETED
    assert deleted.item.id == item.id
    assert deleted.item.name == "grpc-watch-test"

    call.cancel()


@pytest.mark.asyncio
async def test_watch_item_events_client_cancel_increments_handled_total(grpc_channel):
    """Client cancel is WatchItemEvents' primary termination path (an
    analytics client disconnecting), not an error -- but asyncio.CancelledError
    is a BaseException, not caught by the interceptor's `except Exception`,
    so it previously reached `finally` with `code` never assigned and raised
    UnboundLocalError there instead of recording the call (H2)."""
    stub = items_pb2_grpc.ItemServiceStub(grpc_channel)

    def handled_total() -> float:
        return REGISTRY.get_sample_value("grpc_server_handled_total", {"grpc_method": "WatchItemEvents", "grpc_code": "OK"}) or 0.0

    before = handled_total()
    call = stub.WatchItemEvents(items_pb2.WatchItemEventsRequest())
    await _wait_for_subscriber()

    call.cancel()

    # Cancellation propagates server-side asynchronously: wait for the
    # servicer's own `finally` (broadcaster.unsubscribe) to run first, as
    # a signal that the exception has reached the interceptor wrapping it.
    loop = asyncio.get_event_loop()
    deadline = loop.time() + 2.0
    while broadcaster._subscribers:
        if loop.time() > deadline:
            pytest.fail("WatchItemEvents subscriber was never cleaned up after client cancel")
        await asyncio.sleep(0.01)

    deadline = loop.time() + 2.0
    while handled_total() <= before:
        if loop.time() > deadline:
            pytest.fail("grpc_server_handled_total never incremented for WatchItemEvents after client cancel")
        await asyncio.sleep(0.01)

    assert handled_total() == before + 1


@pytest.mark.asyncio
async def test_metrics_interceptor_counts_unary_calls(grpc_channel):
    stub = items_pb2_grpc.ItemServiceStub(grpc_channel)

    def handled_total() -> float:
        return REGISTRY.get_sample_value("grpc_server_handled_total", {"grpc_method": "ListItems", "grpc_code": "OK"}) or 0.0

    def handling_seconds_count() -> float:
        return REGISTRY.get_sample_value("grpc_server_handling_seconds_count", {"grpc_method": "ListItems"}) or 0.0

    before_total, before_seconds = handled_total(), handling_seconds_count()
    await stub.ListItems(items_pb2.ListItemsRequest())

    assert handled_total() == before_total + 1
    assert handling_seconds_count() == before_seconds + 1


@pytest.mark.asyncio
async def test_metrics_interceptor_counts_stream_messages(grpc_channel, db):
    stub = items_pb2_grpc.ItemServiceStub(grpc_channel)
    call = stub.WatchItemEvents(items_pb2.WatchItemEventsRequest())
    await _wait_for_subscriber()

    def messages_total() -> float:
        return REGISTRY.get_sample_value("grpc_server_stream_messages_total", {"grpc_method": "WatchItemEvents"}) or 0.0

    before = messages_total()
    item = await create_item(db, ItemCreate(name="grpc-metrics-test"))
    await asyncio.wait_for(call.read(), timeout=2)

    assert messages_total() == before + 1

    call.cancel()
    await delete_item(db, item.id)
