-- Migration 0001: item_events + event_buckets (ADR-0005, RFC-0001 D1/D7).

-- item_events is the raw stream landed from WatchItemEvents (PR-B ingest).
-- The unique index on (item_id, event_type, event_time) makes reconcile
-- idempotent: after a reconnect, the client replays ListItems and resumes
-- the stream (ADR-0002); if any event is observed twice, an
-- INSERT ... ON CONFLICT DO NOTHING against this constraint silently
-- drops the duplicate instead of double-counting it into event_buckets.
CREATE TABLE IF NOT EXISTS item_events (
    id          bigserial PRIMARY KEY,
    event_type  text NOT NULL CHECK (event_type IN ('created', 'updated', 'deleted')),
    item_id     bigint NOT NULL,
    item_name   text NOT NULL,
    event_time  timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (item_id, event_type, event_time)
);

CREATE INDEX IF NOT EXISTS item_events_event_time_idx ON item_events (event_time);

-- event_buckets are mutable upserts (ON CONFLICT DO UPDATE), never
-- finalized (ADR-0005 D1): a late event -- including seeded history,
-- an extremely late event source -- lands in its correct historical
-- bucket via ON CONFLICT instead of being rejected, so no watermark or
-- completeness machinery exists. The current (most recent) bucket is
-- therefore always partial by construction; reports read up to the last
-- *closed* bucket.
CREATE TABLE IF NOT EXISTS event_buckets (
    bucket_start timestamptz NOT NULL,
    event_type   text NOT NULL CHECK (event_type IN ('created', 'updated', 'deleted')),
    count        bigint NOT NULL DEFAULT 0,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (bucket_start, event_type)
);
