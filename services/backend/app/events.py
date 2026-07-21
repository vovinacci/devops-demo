"""In-process item-event fanout for gRPC WatchItemEvents (RFC-0001 D3).

Backend emits after DB commit (emit-after-commit); delivery is
at-most-once with no durability. If nobody is subscribed, or a subscriber
is too slow to keep up, the event is simply not observed by it -- the
accepted gap that motivates the NATS capstone (ADR-0002). This module has
no dependency on generated gRPC code: it stays importable (and unit
testable) without a `make generate` run.
"""

import asyncio
import contextlib
import logging
from dataclasses import dataclass
from datetime import datetime

logger = logging.getLogger(__name__)

_DEFAULT_QUEUE_SIZE = 64

# Put into a subscriber's queue to signal it was disconnected for being
# too slow; identity-distinguishable from any real ItemEventPayload.
DISCONNECT = object()


@dataclass(frozen=True)
class ItemEventPayload:
    event_type: str  # "created" | "updated" | "deleted"
    item_id: int
    item_name: str
    event_time: datetime


class ItemEventBroadcaster:
    """Fanout of item events to zero or more WatchItemEvents streams.

    publish() is synchronous and must never block: it runs on the HTTP
    request path immediately after a DB commit, and a write must not wait
    on a slow gRPC consumer. Each subscriber owns a bounded queue; when
    full, that subscriber is disconnected (its queue receives the
    DISCONNECT sentinel) rather than the event being awaited into place.
    The still-open gRPC stream sees the sentinel and ends the RPC, which
    makes the analytics client reconnect and reconcile via ListItems
    (ADR-0002) -- the same recovery path as a network disconnect.
    """

    def __init__(self, maxsize: int = _DEFAULT_QUEUE_SIZE) -> None:
        self._maxsize = maxsize
        self._subscribers: set[asyncio.Queue] = set()

    def subscribe(self) -> asyncio.Queue:
        queue: asyncio.Queue = asyncio.Queue(maxsize=self._maxsize)
        self._subscribers.add(queue)
        return queue

    def unsubscribe(self, queue: asyncio.Queue) -> None:
        self._subscribers.discard(queue)

    def publish(self, event: ItemEventPayload) -> None:
        for queue in list(self._subscribers):
            try:
                queue.put_nowait(event)
            except asyncio.QueueFull:
                logger.warning("gRPC event subscriber too slow, disconnecting (event_type=%s)", event.event_type)
                self._subscribers.discard(queue)
                with contextlib.suppress(asyncio.QueueEmpty):
                    queue.get_nowait()  # drop the oldest queued event to make room for the sentinel
                queue.put_nowait(DISCONNECT)


broadcaster = ItemEventBroadcaster()
