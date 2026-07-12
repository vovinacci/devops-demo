# ADR-0010: OpenTelemetry instrumentation from day one

- Status: Accepted
- Date: 2026-07-12
- Extracted from: RFC-0001 D11

## Context

One request path spans frontend -> backend -> gRPC stream -> analytics ->
reports, and the canary measures an end-to-end lag it cannot decompose
without traces. Retrofitting trace-context propagation across five services
is the expensive part and the classic regret.

## Decision

Every service is instrumented with OpenTelemetry from its first commit --
`tracing` + `tracing-opentelemetry` (Rust), OTel SDK (Go, Python),
Micrometer Tracing (Kotlin) -- with W3C trace-context propagation through
HTTP headers and gRPC metadata. The tracing backend (Tempo) is deferred:
until it lands, trace IDs appear in structured JSON logs, so log-trace
correlation works in Loki immediately.

## Alternatives

- Instrument later, when a backend exists: rejected -- propagation retrofit
  across five services is a five-service refactor; instrumenting now costs
  little.
- Deploy Tempo now: rejected -- RAM budget and phase scope are full; adding
  it later is one compose service plus config.

## Consequences

- Easier: correlation via logs today; full trace visualization is a drop-in
  later.
- Harder: small per-service instrumentation overhead carried from the first
  commit, with no visual payoff until Tempo arrives.
