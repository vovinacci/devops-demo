import asyncio
import time
from datetime import UTC, datetime

import pytest

from app.events import DISCONNECT, ItemEventBroadcaster, ItemEventPayload


def _event(item_id: int = 1, event_type: str = "created") -> ItemEventPayload:
    return ItemEventPayload(event_type=event_type, item_id=item_id, item_name=f"item-{item_id}", event_time=datetime.now(UTC))


@pytest.mark.asyncio
async def test_publish_delivers_to_all_subscribers():
    broadcaster = ItemEventBroadcaster()
    q1, q2 = broadcaster.subscribe(), broadcaster.subscribe()

    broadcaster.publish(_event(item_id=42))

    assert (await q1.get()).item_id == 42
    assert (await q2.get()).item_id == 42


@pytest.mark.asyncio
async def test_publish_with_no_subscribers_is_a_noop():
    broadcaster = ItemEventBroadcaster()
    broadcaster.publish(_event())  # must not raise


@pytest.mark.asyncio
async def test_unsubscribe_stops_delivery():
    broadcaster = ItemEventBroadcaster()
    q = broadcaster.subscribe()
    broadcaster.unsubscribe(q)

    broadcaster.publish(_event())

    assert q.empty()


@pytest.mark.asyncio
async def test_slow_subscriber_disconnected_without_blocking_publish():
    """A subscriber that never drains its queue must not slow down (let
    alone block) the publisher -- publish() runs on the HTTP request path
    right after a DB commit (RFC-0001 D3 emit-after-commit)."""
    broadcaster = ItemEventBroadcaster(maxsize=2)
    q = broadcaster.subscribe()

    start = time.perf_counter()
    for i in range(20):
        broadcaster.publish(_event(item_id=i))
    elapsed = time.perf_counter() - start

    assert elapsed < 0.5, "publish() must never block on a full subscriber queue"
    assert q not in broadcaster._subscribers, "full subscriber must be disconnected"

    drained = []
    while not q.empty():
        drained.append(q.get_nowait())
    assert drained[-1] is DISCONNECT, "disconnected subscriber's queue ends with the DISCONNECT sentinel"


@pytest.mark.asyncio
async def test_full_queue_does_not_raise_queue_full_to_caller():
    broadcaster = ItemEventBroadcaster(maxsize=1)
    q = broadcaster.subscribe()

    broadcaster.publish(_event(item_id=1))
    broadcaster.publish(_event(item_id=2))  # queue is now full: must disconnect, not raise

    assert q not in broadcaster._subscribers


@pytest.mark.asyncio
async def test_independent_subscribers_dont_affect_each_other():
    broadcaster = ItemEventBroadcaster(maxsize=1)
    slow, fast = broadcaster.subscribe(), broadcaster.subscribe()

    broadcaster.publish(_event(item_id=1))
    await fast.get()  # fast subscriber keeps up
    broadcaster.publish(_event(item_id=2))  # slow subscriber's queue overflows here

    assert slow not in broadcaster._subscribers
    assert fast in broadcaster._subscribers
    assert (await asyncio.wait_for(fast.get(), timeout=1)).item_id == 2
