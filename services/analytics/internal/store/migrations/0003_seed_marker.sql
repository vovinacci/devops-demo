-- Migration 0003: seed_marker (RFC-0001 Phase 5 D5, ADR-0003).
--
-- Single-row table (id always 1) recording the parameters of the most
-- recent successful `analytics seed` run: the scale it seeded at, its
-- reference time, its --seed value, the --days window, and how many
-- events it wrote. Written once, at the END of a seed run (success
-- only) -- a partial/aborted run leaves no marker, so GET
-- /api/v1/seed-marker genuinely means "a seed run completed", not "one
-- was attempted". loadgen's scale guard (Phase 5 PR-2) reads this at
-- startup: refuses on a scale mismatch (seeded at one DEMO_TIME_SCALE,
-- run at another breaks the seam invariant, Hard rule 8), warns and
-- continues when the row is absent (must not brick `make up` with no
-- seed).
CREATE TABLE IF NOT EXISTS seed_marker (
    id             int PRIMARY KEY CHECK (id = 1),
    scale          double precision NOT NULL,
    ref_unix       bigint NOT NULL,
    seed           bigint NOT NULL,
    days           int NOT NULL,
    seeded_at      timestamptz NOT NULL,
    events_written bigint NOT NULL
);
