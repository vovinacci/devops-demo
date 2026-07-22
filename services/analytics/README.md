# Analytics

Go analytics service (RFC-0001 D1, ADR-0005) -- event ingestion,
aggregation, retention, and historical seed, with its own Postgres
instance. Ships the HTTP surface, database, migrations, Docker/CI/compose
wiring (`feat/analytics-scaffold`), the gRPC ingest client
(`feat/analytics-ingest`, RFC-0001 Phase 3 PR-B, ADR-0002): backend
serves, analytics dials, and the retention job
(`feat/analytics-retention`, RFC-0001 Phase 3 PR-C, ADR-0005 D7), and the
historical seeder (`feat/seeder`, RFC-0001 Phase 5 D5, ADR-0003): strict
per-event seeding through the same ingestion path, see Historical seed
below.

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

## Historical seed (RFC-0001 Phase 5 D5, ADR-0003)

`analytics seed [--days N] [--seed N] [--profile PATH]` generates
deterministic history backwards from the moment it starts, through the
SAME ingestion path live events use -- no direct aggregate writes
(`.omc/handoffs/team-plan.md`'s binding "strict D5 letter" decision, the
one deliberately rejected alternative being a faster two-tier
seed/aggregate-direct design).

```shell
make up-full        # or: docker compose -f deploy/compose/docker-compose.yml --project-directory . --profile analytics up -d --build
make seed-history
```

`make seed-history` first tops up backend items (`make seed`, additive --
adds 20 more each call) so the seeder has real item IDs to reference,
then runs `analytics seed` via `compose run --rm --no-deps` under the
`analytics` profile. `SEED_DAYS`/`SEED_SEED` env vars override the
`--days 90`/`--seed 42` defaults (e.g. `make seed-history SEED_DAYS=3`
for a quick local check).

### Generation

For every hour in `[now - days*24h, now)`, aligned to hour boundaries:

1. `internal/loadshape.Rate(bucketMid, profile, scale, ref=now)` gives
   the hour's rate (events/s); `bucketMid = hourStart + 30m` is the
   representative instantaneous sample for that hour's average.
   `round(rate * 3600)` is the exact event count for that hour -- proven
   exact, not approximate, by `TestRunEventCountPerHourMatchesLoadshape`
   (`internal/seeder`).
2. Each event's exact timestamp (uniform within the hour), event type,
   and item assignment (uniform pick from the ADR-0002 `ListItems`
   snapshot) are drawn from one PRNG stream seeded by `--seed`. Event
   type is created/deleted only, roughly balanced (50/50) -- deliberately
   **not** created/updated/deleted: the backend has no PUT endpoint, so
   `EVENT_TYPE_UPDATED` (`proto/devopsdemo/items/v1/items.proto`) is
   contract-first for a producer that does not exist yet, and no live
   `WatchItemEvents` stream has ever emitted one. Seeding a lookalike
   "updated" bucket would itself be a seam: a by-event-type dashboard or
   query would show synthetic history diverging from live shape exactly
   where D5 requires it to be seamless. The 50/50 split is an independent
   weighted draw per event, not literal create-then-delete pairing --
   over a 90-day window with many events per item, a realistic
   live/tombstoned mix emerges on its own without extra bookkeeping.
3. **PRNG choice:** `math/rand/v2`'s `PCG` source (`rand.NewPCG`), not
   the auto-seeded top-level convenience functions -- PCG's algorithm is
   fully specified, so a given `--seed` reproduces the identical stream
   across Go versions and platforms. Chosen over a hand-rolled LCG for
   the same determinism guarantee with no bespoke arithmetic to
   maintain. `internal/seeder.TestRunIsDeterministic` and
   `TestRunDifferentSeedDiffers` are the parity/no-op regression guards.
4. `--profile` (default `$ANALYTICS_LOADPROFILE_PATH`, itself defaulting
   to `../../loadprofile/profile.json`) reads the same checked-in
   `loadprofile/profile.json` k6 evaluates for live traffic (Hard rule
   8). In the container, `ANALYTICS_LOADPROFILE_PATH` points at
   `/etc/analytics/loadprofile/profile.json`, baked in at build time from
   a `loadprofile=./loadprofile` named build context (compose:
   `additional_contexts`) -- the same pattern `loadgen/Dockerfile` uses
   for the same directory. It is read at container **runtime** by `seed`
   (not at build time; there is nothing to compile), so it lives in the
   Dockerfile's runtime stage, not the builder.

**RFC "spread `created_at`" deviation, documented:** RFC-0001 D5 describes
backend items seeded with spread `created_at` timestamps first. The
backend's `Item` row carries no `created_at` column, so this seeder
coordinates through item **identity** instead: synthetic events reference
real item IDs pulled live via `ListItems`, never invented IDs. Item
identity, not a backdated timestamp, is the coordination point between
backend and analytics history.

### Writing: batched, but still per-event (the D5 constraint)

`internal/store.IngestEventsBatch` is the only concession the strict D5
letter allows: one Postgres transaction per batch (~8,000 events, the
seeder's `defaultBatchSize` -- see Runtime below for the measured
throughput this yields) instead of one per event, with **identical**
semantics to `IngestEvent` called once per event in order:

- Multi-row `INSERT ... ON CONFLICT (item_id, event_type, event_time) DO
  NOTHING RETURNING` -- the same unique-index dedup guard as live
  ingest; `DO NOTHING` (unlike `DO UPDATE`) tolerates duplicate keys
  within one multi-row statement, so no pre-dedup of the batch is
  needed for this half.
- Bucket increments computed **only** from that `RETURNING` set (rows
  genuinely new), grouped by `date_trunc('hour', event_time, 'UTC')` and
  `event_type`, via a CTE chain in one round trip -- a deduplicated
  replay never double-counts, exactly like `IngestEvent`'s
  inserted-gated bucket write.
- `current_items`: Postgres forbids a multi-row `ON CONFLICT DO UPDATE`
  from touching the same row twice in one statement, so events are
  first folded to at most one row per `item_id` in Go
  (`foldCurrentItems`) -- the last event (in the batch's own order)
  whose `event_time` ties or exceeds the running maximum for that item
  wins, the same last-event-time-wins rule `IngestEvent`'s `GREATEST`/
  `CASE` applies one event at a time. The single folded row is then
  upserted with that identical `CASE` logic, which itself compares
  against whatever is already committed (from a prior batch or the
  reconcile snapshot) -- so the fold only needs to get the *within-batch*
  winner right; cross-batch correctness falls out of the SQL `CASE`
  unconditionally.

**Batch-equals-single, proven, not asserted:** `internal/store`'s
`TestIngestEventsBatchEqualsSerialIngestEvent` applies the same
deliberately messy event set (repeated items, out-of-order times, an
exact same-batch duplicate, a late-older-event-after-a-delete) two ways
-- one `IngestEvent` call per event against a fresh schema, one
`IngestEventsBatch` call against another fresh schema -- and asserts
`item_events` row count, every `event_buckets` row, and every
`current_items` row are identical between the two. `
TestIngestEventsBatchDedupWithinBatch`/`DedupAcrossBatches` cover the
two dedup axes separately.

### Closing retention pass

After generation, the seeder runs `internal/retention`'s `Runner.RunOnce`
once -- the exact same retention code the live service's ticker loop
runs (RFC-0001 D5's own "same aggregation/bucketing/retention code"
clause), not a seeder-specific reimplementation. `event_buckets` and
`current_items` are untouched (see Retention above); only raw
`item_events` older than `ANALYTICS_RETENTION_DAYS` (default 7) are
deleted, trimming the transient full-window raw data down to the live
service's normal retention horizon.

### Seed marker and the read API

On success (only), the seeder writes a single-row `seed_marker` table
(migration `0003_seed_marker`: `scale`, `ref_unix`, `seed`, `days`,
`seeded_at`, `events_written`) via `UpsertSeedMarker`, backing
`GET /api/v1/seed-marker` (see Read API below). A partial or failed run
returns before this write, so the marker's presence genuinely means "a
seed run completed" -- the contract Phase 5 PR-2's loadgen scale guard
depends on (refuses to start on a `DEMO_TIME_SCALE` mismatch against
what history was seeded at, warns-and-continues when the row is absent).

### Runtime, disk, and re-run semantics (measured on a laptop, `--days 90`, default 20-item-increments backend)

- **Throughput:** ~120,000-130,000 events/s sustained (generation +
  batched write combined); a full `--days 90` run wrote 58.36M events in
  8m21s wall time, well under the demo's 30-45 minute budget.
- **Closing retention pass:** deleted 54.19M rows (83 of the 90 seeded
  days, `ANALYTICS_RETENTION_DAYS=7` default) in 19.3s.
- **Disk, transient peak then not reclaimed by this run:** `item_events`
  physically occupied ~12 GB after the retention `DELETE` (`pgdata-
  analytics` volume: ~13.8 GB) even though only ~4.17M *logical* rows
  (7 days) remained -- Postgres `DELETE` marks rows dead, it does not
  shrink the table; only `VACUUM`/autovacuum reclaims the space over
  time (autovacuum will eventually run given default settings and normal
  write traffic, but do not expect the volume to shrink immediately
  after a seed run). Budget headroom accordingly, especially on a
  constrained laptop (RFC-0001 Section 11 risk); `VACUUM FULL` reclaims
  it immediately but takes an exclusive lock, not something to run
  casually against a live demo.
- **Seam verified, not just generated:** after a seed run, hourly
  `event_buckets` totals equal `round(loadshape.Rate(bucketMid, profile,
  scale, ref) * 3600)` **exactly** for every hour (checked directly
  against the running stack, not just the unit test) -- e.g. two
  adjacent hours measured `14267`/`13100` actual vs `14267.40`/`13100.31`
  expected. The `ingestion-outage` anomaly (4h, multiplier 0) produces
  exactly 4 consecutive hours with **zero** rows in `event_buckets`
  (all 3 event types absent, not merely zero-count rows) -- confirmed
  directly, not inferred.
- **Re-run semantics: idempotent-ish, not a guarantee across separate
  invocations.** Two axes both drift between separate `analytics seed`
  runs, by design:
  - `ref_time` is captured fresh (`time.Now()`) every invocation, never
    persisted or reused -- a second run's anomaly windows and trend
    factor sit at a *slightly* different absolute position (RFC-0001 D5:
    "different ref time creates overlapping history"). Measured: holding
    the backend item snapshot fixed and re-running
    `analytics seed --days 2 --seed 42` about 30 seconds later against
    an already-seeded table wrote only 5,317 new rows against ~941K
    existing (99.4% deduplicated) -- the residual is exactly this drift,
    concentrated near the degradation anomaly's boundary (closest to
    "now").
  - `make seed-history`'s own `seed` prerequisite is additive (`make
    seed` always adds 20 more backend items), so two `make seed-history`
    invocations see two *different* item snapshots -- the PRNG's
    per-event item pick (`rng.IntN(len(items))`) depends on the snapshot
    size, so nearly nothing dedupes across two `make seed-history` runs
    even though the underlying `IngestEventsBatch` dedup guard itself
    works perfectly (proven at the store layer, see above). This is a
    property of the backend's additive seed script, not a seeder bug.
  - **What IS provably idempotent:** replaying the exact same event set
    (same items, same timestamps, same types) through
    `IngestEventsBatch` -- whether as one batch or split differently
    across batches -- inserts each event exactly once
    (`TestIngestEventsBatchDedupAcrossBatches`). A seed run interrupted
    mid-way (`SIGINT`) and simply re-run is safe to leave as-is or
    re-run: the *portion already written* dedupes correctly regardless
    of how the retry's batches happen to be sliced.
  - **Full reset, honest guidance:** to get a clean, fully reproducible
    re-seed (e.g. for a course exercise that must start from the same
    state every time), reset the analytics volume rather than re-running
    on top of existing data:
    `docker compose -f deploy/compose/docker-compose.yml --project-directory . --profile analytics down -v` (drops `pgdata-analytics`), then bring the
    profile back up and run `make seed-history` once.

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
- `GET /api/v1/seed-marker` -- the `seed_marker` row as JSON (`scale`,
  `ref_unix`, `seed`, `days`, `seeded_at`, `events_written`) (RFC-0001
  Phase 5 D5, see Historical seed above). `200` only once a seed run has
  completed successfully; `404` on a never-seeded stack -- Phase 5 PR-2's
  loadgen scale guard treats these differently (refuse on a scale
  mismatch vs. warn-and-continue when absent).

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
- `seed [--days N] [--seed N] [--profile PATH]` -- the historical seeder
  (RFC-0001 Phase 5 D5, see Historical seed above). Exits non-zero on any
  failure (bad flags, unreachable Postgres/backend, zero backend items,
  a write failure) with a `slog` error line explaining which.
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
| `ANALYTICS_LOADPROFILE_PATH` | `../../loadprofile/profile.json` | `seed` subcommand only (RFC-0001 Phase 5 D5): path to `loadprofile/profile.json`. Compose sets this to `/etc/analytics/loadprofile/profile.json`, baked in from the `loadprofile` build context (see Docker below) |
| `DEMO_TIME_SCALE` | `1` | `seed` subcommand only: the shared load-shape scale (Hard rule 8, ADR-0003) -- must match whatever loadgen runs with, or the seam invariant breaks |

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
backing store, see Ingest above). `0003_seed_marker`: `seed_marker`
(single-row, RFC-0001 Phase 5 D5, see Historical seed above).

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
`services/backend/Dockerfile`. `loadprofile/` is a second named build
context (RFC-0001 Phase 5 D5, same pattern `loadgen/Dockerfile` uses for
the same directory), copied straight into the **runtime** stage (not the
builder) at `/etc/analytics/loadprofile/` -- `seed` reads
`profile.json` at container runtime, there is nothing to compile.

## Roadmap

- **PR-D** (`feat/canary-v2`): canary pipeline-lag step polling this
  service's `GET /api/v1/items/{item_id}` (RFC-0001 D9).
- **Phase 5 PR-2** (`feat/history-dashboards`): Grafana historical
  dashboards over `event_buckets`, Grafana annotations for the seeded
  anomalies, loadgen's `/api/v1/seed-marker` scale guard.
