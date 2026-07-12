# ADR-0006: k6 as load generator and CI performance gate

- Status: Accepted
- Date: 2026-07-12
- Extracted from: RFC-0001 D4

## Context

Dashboards need continuous, realistically shaped traffic (diurnal shape,
error traffic, occasional expensive queries), and CI needs performance
assertions. One tool should serve both so the demo and the gate cannot
diverge.

## Decision

k6 is the single traffic generator: one long-running compose service under
`loadgen/` using `ramping-arrival-rate` executors driven by the shared load
profile (ADR-0003). Scenario mix includes browse, CRUD writes,
invalid/abuse traffic, report triggers, direct gRPC, and an occasional
expensive query for a real p99 tail. `make incident` / `make heal` switch
spike or error-storm modes. The same scripts run in CI with thresholds
(e.g. p95 latency, error rate) as gates. k6 metrics reach Prometheus via
remote-write (receiver must be enabled in Prometheus).

## Alternatives

- Locust: rejected -- weaker always-on shaping.
- Custom Go/Rust generator: rejected -- duplicates lessons other services
  already carry, adds maintenance without teaching value.

## Consequences

- Easier: "offered load" and observed latency sit side by side; performance
  is a test, not a vibe.
- Harder: scenario code is JavaScript (k6's language, not a service
  runtime); Prometheus needs `--web.enable-remote-write-receiver` wired in
  the same phase.
