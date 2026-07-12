# ADR-0005: Analytics data ownership and retention

- Status: Accepted
- Date: 2026-07-12
- Extracted from: RFC-0001 D1, D7

## Context

The Go analytics service ingests item events and serves aggregates.
Continuous load means unbounded raw-event growth. Aggregation semantics
must survive very late events -- the historical seeder is just an extremely
late event source (see ADR-0003).

## Decision

Analytics owns its store: a separate Postgres container (not a second
database in the existing instance). Buckets are keyed strictly on event
time, never arrival time, and are mutable upserts (`ON CONFLICT DO
UPDATE`) -- never finalized, so no watermark machinery exists. A retention
job aggregates-then-deletes raw events older than N days; that horizon is
the de-facto lateness bound. Reports read up to the last closed bucket.

## Alternatives

- Second database in the shared Postgres instance: rejected --
  service-owns-its-store must be visible in the compose topology.
- Arrival-time bucketing: rejected -- collapses seeded history into "now"
  and destroys the stitching invariant.
- Real watermarking (Flink/Beam-style): rejected as out of scope -- needed
  only with multiple producers/partitions; referenced as further reading.
- Keeping raw events forever: rejected -- retention is a first-class ops
  lesson, not housekeeping.

## Consequences

- Easier: late data lands correctly with no special cases; ownership
  boundaries are visible and teachable.
- Harder: extra RAM for a dedicated Postgres; the current bucket is always
  partial, and "completeness" is only approximated by stream liveness plus
  last-received-event-time gauges (the same signal the canary measures).
