# Analytics

Go analytics service (RFC-0001 D1, ADR-0005) -- event ingestion,
aggregation, retention, and historical seed, with its own Postgres
instance. This PR (`feat/analytics-scaffold`) ships the scaffold only:
HTTP surface, database, migrations, Docker/CI/compose wiring. The gRPC
client that actually ingests events from the backend lands in
`feat/analytics-ingest` (RFC-0001 Phase 3 PR-B).

## Semantics (ADR-0005)

- **Event time, never arrival time.** Buckets are keyed on when an event
  happened, not when analytics received it -- required for the seeded
  history (an extremely late event source) to land in the correct
  historical bucket instead of collapsing into "now".
- **Mutable upserts, no watermarks.** `event_buckets` rows are updated via
  `ON CONFLICT DO UPDATE`, never finalized. A late event simply updates
  its bucket; there is no completeness decision to make, so no watermark
  machinery exists.
- **The current bucket is always partial.** Reports read up to the last
  *closed* bucket. "Completeness" is approximated by stream-connection
  liveness and last-received-event-time gauges (arriving with PR-B) --
  the same signal the canary's pipeline-lag metric measures.
- **Readiness is database-only.** `/readyz` pings Postgres and nothing
  else. Upstream gRPC stream health (PR-B) is a metrics/alerting concern,
  not a readiness concern -- a disconnected stream must not take the HTTP
  surface out of rotation (RFC-0001 D10 graceful degradation, mirroring
  the canary's own readyz discipline).

## Subcommands

- `serve` (default) -- runs the HTTP server.
- `seed` -- stub; exits `2` with `seed arrives with RFC-0001 Phase 5`
  (the historical seeder is a later phase, see RFC-0001 D5).
- `healthcheck` -- exits `0` if both `/healthz` and `/readyz` return `200`
  against the local process, non-zero otherwise. Exists so the Docker
  runtime image can be `distroless/static` (no shell, no wget/curl for a
  container `HEALTHCHECK` to exec) -- the binary probes itself instead.

## Endpoints (`:8082`)

- `/healthz` -- liveness, always `200` once the process is up.
- `/readyz` -- readiness = Postgres reachable (see Semantics above).
- `/metrics` -- Prometheus exposition format: Go runtime/process metrics
  plus `analytics_db_up` (mirrors the last `/readyz` outcome as a
  whitebox metric). Stream/ingest metrics arrive with PR-B.

## Environment variables

| Variable | Default | Meaning |
| -------- | ------- | ------- |
| `ANALYTICS_HTTP_ADDR` | `:8082` | HTTP listen address |
| `ANALYTICS_DATABASE_URL` | `postgres://analytics:analytics@localhost:5433/analytics?sslmode=disable` | Postgres connection string (own instance, ADR-0005) |

## Database

Own Postgres instance (`postgres-analytics` in compose, port `5433` on
the host for parity with `db`'s `5432`) -- service-owns-its-store is
visible in the compose topology (ADR-0005), not a second database on the
existing instance.

Migrations are hand-rolled (embedded SQL + a `schema_migrations` table,
`internal/store/migrate.go`) rather than a migration library: a ~40-line
runner a student can read start to end beats debugging a dependency's
internals, and analytics needs nothing more sophisticated than
sequential, idempotent, forward-only migrations. Applied at startup,
before the HTTP server starts serving.

`0001_init`: `item_events` (raw stream, unique on `(item_id, event_type,
event_time)` so re-reconcile after a stream reconnect is idempotent) and
`event_buckets` (hourly aggregates, mutable upserts -- see Semantics).

## Logs and tracing

Structured JSON logs to stdout (`internal/logging`). Every log line
carries a `trace_id` field -- the active OpenTelemetry span's W3C trace
ID, or 32 zeros when no span is active -- for correlation via Loki
(RFC-0001 D11, ADR-0010). OpenTelemetry is wired from this service's
first commit (`internal/otelsetup`): `service.name=analytics`, W3C
trace-context propagation set globally, HTTP handlers instrumented via
`otelhttp` (healthz/readyz/metrics excluded -- polled every few seconds,
never worth a span). No exporter is configured: spans are generated and
propagated, then dropped, until the trace backend (Tempo) lands
(RFC-0001 Section 10). gRPC-metadata trace propagation becomes live with
the client in PR-B.

## Development

```shell
make build   # go build -o bin/analytics ./cmd/analytics
make test    # unit tests + migration tests against a throwaway Postgres container
make lint    # gofmt -l + go vet + golangci-lint run
make run     # go run ./cmd/analytics serve (ANALYTICS_* env vars, see above)
```

`make test` starts and stops its own disposable Postgres container
(`docker run postgres:16-alpine ...`, port `55432`) around the test run
-- the lightest option that does not depend on the compose `analytics`
profile being up, and works the same locally and in CI (CI instead uses
a GitHub Actions service-container Postgres with the same schema).

Toolchain: Go version is pinned in the repo root `.mise.toml`
(ADR-0012); run `mise install` from the repo root, then `make doctor`
verifies it.

## Proto / codegen

Generated gRPC/protobuf Go stubs (`internal/pb/`, package `itemsv1`) are
never committed (Hard rule 1, RFC-0001 D8, ADR-0002). Regenerate with
`make generate` from the repo root (`cd proto && buf generate`, per
`proto/buf.gen.yaml`'s Go plugins) after cloning or after a `proto/`
change -- needed for IDE completion and for `go build`/`go test` to
compile `internal/pb` at all. The Dockerfile regenerates them in its
builder stage the same way, hermetically.

## Docker

Multi-stage build (RFC-0001 D6): the builder stage installs `buf` and
the Go protoc plugins at the versions pinned in `.mise.toml` /
`proto/buf.gen.yaml`, regenerates the gRPC stubs, then compiles a static
binary (`CGO_ENABLED=0`). The runtime stage is `gcr.io/distroless/static`
-- no shell, no package manager, smallest attack surface of any service
in this repo -- made possible by the `healthcheck` subcommand replacing
the shell-exec'd probe the other services' container healthchecks use.
Runs as the image's built-in non-root user.

`proto/` is reached as a named additional build context (compose:
`additional_contexts`; plain `docker build`:
`--build-context proto=proto`), the same pattern as
`services/backend/Dockerfile`.

## Roadmap

- **PR-B** (`feat/analytics-ingest`): gRPC client dialing
  `backend:50051`, snapshot reconcile + `WatchItemEvents` stream,
  event-time bucket upserts, stream/ingest metrics, read API.
- **PR-C** (`feat/analytics-retention`): aggregate-then-delete raw events
  older than N days (ADR-0005 D7).
