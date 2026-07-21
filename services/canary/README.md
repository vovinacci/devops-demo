# Canary

Rust synthetic-journey canary -- the synthetic layer of three-layer
monitoring (RFC-0001 D9, ADR-0007). Runs a full user journey against the
backend on a schedule and reports whether it actually works end to end,
which is the failure mode whitebox and blackbox checks cannot see.

## Journey (v1: backend CRUD)

On every tick of `CANARY_INTERVAL_SECONDS`:

1. `POST /items` on the backend, with a name prefixed `canary-` (the
   synthetic-traffic tag, ADR-0007 D9) so dashboards and report queries
   can filter it out of business data.
2. `GET /items` and confirm the created item is visible.
3. `DELETE /items/{id}` -- cleanup.

Cleanup is best-effort and unconditional: the item is deleted even if
step 2 does not find it, so a failing journey never leaves synthetic
data behind. Every step's duration is recorded regardless of outcome, so
the runbook can localize *which* step is slow or failing before it fails
outright.

v1 is REST-only (backend CRUD). v2 adds a pipeline-lag step once
analytics exists (Phase 3); v3 adds a report-trigger step once reports
exists (Phase 6) -- see RFC-0001 D9.

## Metrics

| Metric | Type | Labels | Meaning |
| ------ | ---- | ------ | ------- |
| `canary_journey_total` | counter | `result=success\|failure` | Journeys run, by outcome |
| `canary_journey_step_duration_seconds` | histogram | `step=create\|verify\|delete` | Per-step latency, recorded even on failure |
| `canary_journey_last_success_timestamp_seconds` | gauge | -- | Unix timestamp of the last fully successful journey |

## Endpoints (`:8080`)

- `/healthz` -- liveness, always `200` once the process is up.
- `/readyz` -- ready once the journey loop has started. The backend the
  canary journeys against is a thing it *monitors*, not a dependency it
  needs to be alive itself -- a down backend must not make the canary
  unready (RFC-0001 D10 graceful degradation). Journey failures are
  visible in `canary_journey_total` and the `CanaryJourneyFailing` alert,
  not in `/readyz`.
- `/metrics` -- Prometheus exposition format.

## Logs and tracing

Structured JSON logs to stdout. Every journey opens a root tracing span,
so its OpenTelemetry trace ID is attached to every log line for that
journey (RFC-0001 D11, ADR-0010): search logs by `trace_id` to see every
step of one journey together. The trace ID also travels on outbound
requests to the backend as a W3C `traceparent` header. No trace *backend*
(Tempo) is configured yet -- spans are generated and propagated, then
dropped, so this is log-based correlation only until Tempo lands.

## Environment variables

| Variable | Default | Meaning |
| -------- | ------- | ------- |
| `CANARY_BACKEND_URL` | `http://api:8000` | Base URL of the backend to journey against |
| `CANARY_INTERVAL_SECONDS` | `30` | Seconds between journeys |
| `CANARY_TIMEOUT_SECONDS` | `5` | Per-request timeout for each journey step |

## Known limitation: shutdown mid-journey

The journey loop runs in its own spawned task, independent of the HTTP
server's graceful shutdown (`axum::serve(...).with_graceful_shutdown`,
which only drains in-flight HTTP connections). If SIGTERM/SIGINT arrives
while a journey is between the `create` and `delete` steps, the process
exits without the loop task being drained, and the just-created
`canary-` prefixed item is not cleaned up. This is a narrow window (one
journey's worth of steps) and not silent: the runbook
(`docs/runbooks/canary-journey-failing.md`) covers checking for and
removing any leftover `canary-` items as part of triage/remediation.

## Development

```shell
make build   # cargo build --release
make test    # unit + integration tests, mock backend (wiremock), no real network
make lint    # cargo fmt --check + cargo clippy -D warnings
make run     # cargo run (CANARY_BACKEND_URL defaults to http://api:8000)
```

Toolchain: Rust version is pinned in the repo root `.mise.toml`
(ADR-0012); run `mise install` from the repo root, then `make doctor`
verifies it.

## Docker

Multi-stage build with `cargo-chef` for dependency-layer caching
(RFC-0001 D6): a `chef` base computes the dependency recipe once, so
`cargo build` only recompiles the canary's own code on a source-only
change. Runtime image is `debian-slim`, not distroless -- deliberately,
so the container healthcheck can exec `wget` against `/healthz` (a
distroless image ships no shell or HTTP client at all; see the `loki`
service comment in `deploy/compose/docker-compose.yml` for the tradeoff
this choice avoids). Runs as a non-root user, matching the other services.
