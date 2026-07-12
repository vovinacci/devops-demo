# ADR-0007: Three-layer monitoring -- whitebox, blackbox, synthetic

- Status: Accepted
- Date: 2026-07-12
- Extracted from: RFC-0001 D9

## Context

Each monitoring layer catches failures the others cannot; knowing which
layer fired is half the diagnosis. The gRPC event stream has a documented
durability gap (ADR-0002) that no whitebox metric can see.

## Decision

Three distinct layers: whitebox (per-service `/metrics`, ADR-0004);
blackbox (`blackbox_exporter` -- HTTP status/latency, TLS expiry, DNS
probes against every endpoint); synthetic (the Rust canary executing a full
user journey on a schedule: create item -> poll analytics until visible ->
trigger report -> clean up). Canary-exclusive metrics: end-to-end pipeline
lag, journey success rate, per-step latency. Synthetic traffic is tagged
and filtered out of business dashboards while flowing through the real
pipeline. Runtime choice for the canary is Rust (axum + tokio): a
monitoring component must cost less than what it monitors -- static binary,
few MB RSS, no GC pauses blurring its own latency measurements -- and it is
the deliberate operational contrast with the JVM service.

## Alternatives

- Custom uptime pinger: rejected -- reinvents blackbox_exporter; standard
  tool for the standard job.
- No synthetic layer: rejected -- the ADR-0002 durability gap and
  cross-service correctness would be invisible.

## Consequences

- Easier: alert triage teaches itself -- students see which layer fires
  first per failure mode.
- Harder: the canary is custom code that grows with the platform (v1 CRUD
  journey, v2 pipeline lag, v3 reports); tagged-data filtering must be
  applied in dashboards and report queries.
