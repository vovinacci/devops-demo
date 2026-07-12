# ADR-0002: gRPC direction, streaming ingestion, and buf governance

- Status: Accepted
- Date: 2026-07-12
- Extracted from: RFC-0001 D3, D8

## Context

Analytics needs item events from the backend (the source of truth). A
message broker is deliberately deferred to a capstone phase. Connection
direction vs data direction is a classic streaming confusion worth teaching
explicitly.

## Decision

Contract-first Protobuf in a root `proto/` buf module (package
`devopsdemo.items.v1`). Backend serves gRPC on :50051 (`ListItems`,
`GetItemStats` unary; `WatchItemEvents` server-streaming); analytics dials
the backend and owns all reconnect logic (exponential backoff + jitter;
snapshot-then-stream reconcile). Events are emitted after DB commit.
`buf` provides lint, format, codegen, and breaking-change detection as a CI
gate. Generated code is produced in the build (`make generate` locally,
build-stage generation in Docker) and never committed.

## Alternatives

- Reverse direction (backend pushes to analytics): rejected -- the source
  of truth must not depend on a consumer.
- Polling: rejected as teaching-poor.
- Broker now: deferred -- the visible at-most-once gap motivates the NATS
  capstone.
- Committing generated Go code (the traditional Go norm): rejected on the
  deciding criterion "does anyone import these services as Go modules?" --
  no; buf is already a CI dependency, so generate-in-build guarantees
  no proto/code drift.

## Consequences

- Easier: contract evolution is gated (`buf breaking`); hermetic builds.
- Harder: IDE completion requires one `make generate` after clone; events
  emitted during a consumer disconnect are unrecoverable -- the aggregate
  dip stays visible by design.
- Accepted: emit-after-commit crash window (a committed item whose event
  was never emitted) until the transactional outbox arrives with NATS.
