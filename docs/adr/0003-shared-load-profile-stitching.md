# ADR-0003: Shared load profile and history stitching contract

- Status: Accepted
- Date: 2026-07-12
- Extracted from: RFC-0001 D5

## Context

Dashboards without traffic and history are screenshots. Seeded history and
live load must join without a visible seam: a panel spanning
`now-30d -> now+1h` shows one continuous shape.

## Decision

One checked-in load-shape definition, `loadprofile/profile.json` (base rate,
diurnal amplitude/phase, weekday coefficients, noise, trend, anomalies).
Two consumers evaluate the same function of time: the Go seeder (history,
backwards from seed time) and k6 (live arrival rate). A golden-file parity
test in CI compares both implementations on identical timestamps. The
seeder pushes events through the live ingestion path, never directly into
aggregate tables. `DEMO_TIME_SCALE` is a parameter of the shared evaluation
(seeder records it; loadgen refuses on mismatch). Prometheus is not
backfilled: historical dashboards query analytics Postgres; ops metrics are
ephemeral, business data is durable.

## Alternatives

- Independent seed and load shapes: rejected -- drift is the seam.
- Direct INSERTs into aggregates for speed: rejected -- bypasses the
  aggregation code and drifts from it; slower seed accepted.
- Backfilling Prometheus: rejected -- wrong tool for durable business data.

## Consequences

- Easier: the seam invariant is testable in CI, not eyeballed.
- Harder: the shape function is duplicated in Go and JS (parity-tested);
  prolonged downtime leaves a live-side gap -- `make seed` is the reset
  button, no engineering around it.
