-- Migration 0002: current_items (ADR-0002 reconcile target; RFC-0001 Phase 3 PR-B).
--
-- current_items is STATE recovered by the ListItems snapshot reconcile and
-- kept live by WatchItemEvents stream updates -- NOT an aggregate derived
-- from item_events (those stay the raw event log; reconcile never
-- synthesizes events into item_events or event_buckets, D3 dips stay
-- visible). It backs the read API (GET /api/v1/items/{item_id}) that the
-- canary v2 pipeline-lag step polls: a row becomes "known" only via
-- stream ingestion or snapshot reconcile, so pipeline lag genuinely
-- measures the gRPC ingestion path, not a side channel.
--
-- Tombstoned rows are kept, not deleted: a deleted-but-once-seen item
-- must still resolve to 200 (tombstoned=true) rather than 404 (unknown),
-- per the read API's documented semantics.
CREATE TABLE IF NOT EXISTS current_items (
    item_id       bigint PRIMARY KEY,
    name          text NOT NULL,
    first_seen    timestamptz NOT NULL,
    last_seen     timestamptz NOT NULL,
    tombstoned_at timestamptz
);
