# Analytics

Go analytics service (RFC-0001 D1, ADR-0005) -- event ingestion,
aggregation, retention, and historical seed, with its own Postgres
instance. Ships the HTTP surface, database, migrations, Docker/CI/compose
wiring (`feat/analytics-scaffold`), the gRPC ingest client
(`feat/analytics-ingest`, RFC-0001 Phase 3 PR-B, ADR-0002): backend
serves, analytics dials, and the retention job
(`feat/analytics-retention`, RFC-0001 Phase 3 PR-C, ADR-0005 D7).

## Ingest (RFC-0001 Phase 3 PR-B, ADR-0002)

`internal/ingest` owns the connection to the backend and all reconnect
logic (the backend never dials out and has zero awareness of connected
consumers):

1. Dial `ANALYTICS_BACKEND_GRPC_ADDR` (a fresh gRPC connection every
   cycle, including reconnects -- matches the ADR-0002 sequence diagram's
   repeated "dial" step rather than relying on gRPC-go's own transparent
   transport reconnection).
2. `ListItems` snapshot -> reconcile into `current_items` (upsert every
   returned item, tombstone every row missing from the snapshot that
   isn't already tombstoned). This recovers **state**, not missed
   **events**: intermediate updates and create/delete churn during a
   disconnect are unrecoverable by design, and the resulting event-count
   aggregate dip in `event_buckets` stays visible (ADR-0002) -- reconcile
   never synthesizes rows into `item_events` or `event_buckets`.
3. Resume `WatchItemEvents`: each event lands a raw `item_events` row
   (idempotent `ON CONFLICT (item_id, event_type, event_time) DO
   NOTHING` -- forward-note: two DISTINCT same-type events sharing an
   exact microsecond `event_time` would silently dedupe too, accepted at
   demo scale), an hourly `event_buckets` upsert keyed on
   `date_trunc('hour', event_time, 'UTC')` (**event time, never arrival
   time** -- Hard rule 7; the explicit `'UTC'` third argument keeps the
   truncation independent of the connection's session timezone)
   incremented only when the raw insert was genuinely new (never on a
   deduplicated replay), and a `current_items` state update (idempotent,
   last-event-time-wins so an out-of-order older event can't clobber
   more-recent state, harmless on replays regardless of the item_events
   dedup outcome).
4. On any error (dial, snapshot, or stream): exponential backoff +
   jitter (`ANALYTICS_INGEST_BACKOFF_BASE`/`_MAX`, equal-jitter,
   env-tunable) and back to step 1.

Metrics (`/metrics`): `analytics_stream_connected` (0/1 gauge),
`analytics_last_event_time_seconds` (gauge, the degenerate single-source
watermark, ADR-0005 D1), `analytics_events_ingested_total{event_type}`,
`analytics_events_deduplicated_total`, `analytics_reconnects_total`,
`analytics_snapshot_reconciles_total`, `analytics_snapshot_items`.

Tracing: one OTel span per reconnect cycle
(`analytics.ingest.cycle`), with every log line in that cycle carrying
its `trace_id` (`internal/logging`); outbound `ListItems`/
`WatchItemEvents` calls carry W3C trace context via a small manual
gRPC client interceptor (no `otelgrpc` dependency for two RPC shapes).

## Retention (RFC-0001 Phase 3 PR-C, ADR-0005 D7)

`internal/retention` runs a ticker loop deleting raw `item_events` rows
older than `ANALYTICS_RETENTION_DAYS` (default `7`) every
`ANALYTICS_RETENTION_INTERVAL` (default `1h`), plus once immediately at
startup -- an interval-only loop on a service restarted before its first
tick (a daily redeploy against the 1h default, for instance) would
otherwise never run at all.

**Delete-only, not "aggregate-then-delete" in the literal D7 sense.** D7
says the retention job aggregates, then deletes. In this design the
hourly `event_buckets` rows already ARE that aggregation: `IngestEvent`
writes the bucket upsert in the same transaction as the raw
`item_events` insert, at ingest time, not at retention time. So by the
time a raw event is old enough for this job to delete, its aggregate has
already existed for up to `ANALYTICS_RETENTION_DAYS` -- there is no
re-aggregation step left to perform here, and the aggregate outlives the
raw event by construction. `internal/store.DeleteEventsOlderThan` only
issues `DELETE FROM item_events WHERE event_time < cutoff`.

The cutoff is keyed on `event_time`, never `received_at` (Hard rule 7,
same as ingest's own bucket keying, ADR-0005 D1) -- retention must expire
data by when it happened, not by when analytics happened to observe it,
or seeded/late-arriving history could be deleted before it ever reaches
its correct bucket.

`event_buckets` and `current_items` are both left untouched by this job:
buckets are the durable business-data aggregate (ADR-0005), and
`current_items` is live state, not history -- neither is data this job
expires.

Metrics: `analytics_retention_runs_total{result="success"|"failure"}`,
`analytics_retention_deleted_rows_total` (counter),
`analytics_retention_last_success_timestamp_seconds` (gauge),
`analytics_retention_run_duration_seconds` (histogram). One `slog` line
per run (deleted count, cutoff, duration), with `trace_id` via a
dedicated `analytics.ingest.cycle`-style OTel span
(`analytics.retention.run`) per run.

No dedicated alert exists for retention failures this phase: a run
failure surfaces via `analytics_retention_runs_total{result="failure"}`
plus rising `analytics_retention_last_success_timestamp_seconds` age on
the `Analytics Ingest` dashboard instead. Adding an alert is not in the
Phase 3 plan; this is a deliberate scope decision, not an oversight.

## Read API (`/api/v1`)

- `GET /api/v1/items/{item_id}` -- `current_items` row as JSON
  (`item_id`, `name`, `first_seen`, `last_seen`, `tombstoned`). A row
  becomes known only via `WatchItemEvents` ingestion or a `ListItems`
  snapshot reconcile, never synthesized by this handler -- the canary v2
  pipeline-lag step polls this endpoint after creating an item, so
  "known" genuinely measures gRPC ingestion progress. `404` means never
  seen; a deleted-but-once-seen item still returns `200` with
  `tombstoned: true`, distinct from unknown.
- `GET /api/v1/stats` -- `event_buckets` totals by `event_type` over the
  last 24h, a cheap aggregate read (not a report -- real reporting is the
  Kotlin reports service, a later phase).

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
  *closed* bucket. "Completeness" is approximated by the
  `analytics_stream_connected` and `analytics_last_event_time_seconds`
  gauges -- the same signal the canary's pipeline-lag metric measures.
- **Readiness is database-only.** `/readyz` pings Postgres and nothing
  else. Upstream gRPC stream health is a metrics/alerting concern, not a
  readiness concern -- a disconnected stream must not take the HTTP
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
  whitebox metric), the ingest metrics listed above, and the retention
  metrics listed in Retention above.
- `/api/v1/items/{item_id}`, `/api/v1/stats` -- ingest read API, see above.

## Environment variables

| Variable | Default | Meaning |
| -------- | ------- | ------- |
| `ANALYTICS_HTTP_ADDR` | `:8082` | HTTP listen address |
| `ANALYTICS_DATABASE_URL` | `postgres://analytics:analytics@localhost:5433/analytics?sslmode=disable` | Postgres connection string (own instance, ADR-0005) |
| `ANALYTICS_BACKEND_GRPC_ADDR` | `localhost:50051` | Backend `ItemService` gRPC target analytics dials (ADR-0002); compose sets this to `api:50051` |
| `ANALYTICS_INGEST_BACKOFF_BASE` | `1s` | Reconnect backoff base delay (Go duration syntax) |
| `ANALYTICS_INGEST_BACKOFF_MAX` | `30s` | Reconnect backoff cap (Go duration syntax) |
| `ANALYTICS_RETENTION_INTERVAL` | `1h` | Retention job ticker interval (Go duration syntax); also runs once at startup regardless |
| `ANALYTICS_RETENTION_DAYS` | `7` | Raw `item_events` older than this many days (by `event_time`) are deleted each run (ADR-0005 D7) |

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
`0002_current_items`: `current_items` (reconcile target and read-API
backing store, see Ingest above).

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
(RFC-0001 Section 10). The ingest client's outbound `ListItems`/
`WatchItemEvents` calls carry that same W3C context via gRPC metadata
(see Ingest above).

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

- **PR-D** (`feat/canary-v2`): canary pipeline-lag step polling this
  service's `GET /api/v1/items/{item_id}` (RFC-0001 D9).
