# ADR-0008: Compose profiles and graceful degradation

- Status: Accepted
- Date: 2026-07-12
- Extracted from: RFC-0001 D10

## Context

The full platform (three Postgres instances, a JVM, the observability
stack) exceeds a student laptop's comfort zone. The original repo's promise
-- application plus full observability -- must remain the default.

## Decision

Compose profiles select at runtime: `core` (default: app + full
observability, ~2 GB) plus additive `analytics`, `reports`, `synthetic`,
`load`. `make up` brings up core; `make up-full` everything; granular
combos are documented for constrained machines. Graceful degradation is a
rule: services tolerate absent optional dependencies (the canary skips its
pipeline-lag step with a distinct metric label when analytics is down)
rather than failing.

Source organization is a separate axis from runtime selection: the compose
file lives at `deploy/compose/docker-compose.yml` behind a single Makefile
entrypoint (`--project-directory .` keeps paths and project name rooted at
the repo). When Phase 3 makes profiles real, the file splits via compose
`include:` into per-profile-group files -- profiles remain the runtime
switch, `include:` organizes source, one entrypoint stays.

## Alternatives

- Everything always on: rejected -- RAM footprint excludes students.
- Observability as an optional profile: rejected -- observability is the
  point of this repository.
- Overlay `-f` file chains for selection: rejected -- ordering-sensitive
  merges duplicate what profiles already do.
- Per-service compose fragments in service directories: rejected -- wiring
  is topology, not service-internal; `deploy/compose/` owns composition.

## Consequences

- Easier: documented minimum requirements per combo; CI can run a reduced
  profile set.
- Harder: optional-dependency tolerance must be designed into services
  (Hard rule 9) and tested.
