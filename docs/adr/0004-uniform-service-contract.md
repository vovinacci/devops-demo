# ADR-0004: Uniform service contract

- Status: Accepted
- Date: 2026-07-12
- Extracted from: RFC-0001 D6

## Context

Five runtimes with different operational characteristics must remain
comparable -- "same operational requirements, five runtimes" is the
teaching goal. Without a contract, five ecosystems diverge immediately.

## Decision

Every service ships: `/healthz` (liveness) and `/readyz` (readiness), plus
the gRPC Health Checking Protocol where gRPC is served; `/metrics` in
Prometheus format; structured JSON logs to stdout; a multi-stage
Dockerfile; identical Make targets (`build`, `test`, `lint`, `run`); a
provisioned Grafana dashboard; a CI job with format, lint, tests, and a
dependency audit.

## Alternatives

- Per-service conventions: rejected -- destroys cross-runtime
  comparability, the point of the exercise.
- Implicit health via container healthchecks only (the v1 state): rejected
  -- the readiness/liveness split is itself a lesson (JVM startup).

## Consequences

- Easier: dashboards, alerts, and runbooks follow one pattern; new services
  are reviewable against a checklist (Definition of Done).
- Harder: every new service pays a fixed scaffolding cost before its first
  feature.
