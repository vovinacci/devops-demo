# Reports

Kotlin / Spring Boot 3 reports service (RFC-0001 D2) -- the JVM showcase of
the polyglot platform, with its own Postgres instance and an artifact
volume. PR-1 (`feat/reports-skeleton`, Phase 6) shipped the skeleton and the
D6 uniform service contract. This PR (`feat/report-engine`, Phase 6 PR-2)
adds the **report engine**: an async job API (`POST /reports` ->
`GET /reports/{id}` -> download) generating an items-summary report in
XLSX/PDF/CSV, backed by Flyway-migrated job metadata in Postgres and
artifacts on the named volume. The bursty Apache POI / OpenPDF allocation is
the deliberate workload that makes the GC sawtooth visible on the
`Reports JVM` dashboard -- the reason this service exists (see Why a
heavyweight framework here, below).

## Why a heavyweight framework here

The JVM is on this platform to teach what the JVM makes visible (RFC-0001
D2): startup time, heap sizing against a container memory limit, GC
behaviour under bursty allocation, and readiness-vs-liveness. Spring Boot
was chosen over a lean framework (Ktor) deliberately -- the framework's
weight *is* the lesson. Report generation (Apache POI / OpenPDF) is bursty,
blocking allocation; that workload is what makes the GC sawtooth show up on
the `Reports JVM` Grafana dashboard (see Observability below). It runs on a
bounded dispatcher on purpose: enough concurrency to allocate in visible
bursts, capped so the exhibit is a sawtooth, not an OOM.

## Stack and the choices behind it

- **Spring Boot 3.5 + Kotlin, Spring MVC (not WebFlux), JDK 21 virtual
  threads.** Report generation is blocking (POI/PDF); virtual threads carry
  blocking work at scale without a reactive rewrite, and the blocking model
  keeps the heap/GC lesson legible.
- **Coroutines for the async job runner (RFC-0001 D2).** `POST /reports`
  returns immediately; the render runs off-thread via structured concurrency
  on a **bounded** dispatcher -- a fixed platform-thread pool sized by
  `reports.job-concurrency`. Bounded on purpose: bursty POI/PDF allocation is
  a controlled GC exhibit, not an OOM. Every state transition is persisted, so
  a job is traceable end to end and never lost on crash/shutdown.
- **JDBC datasource (HikariCP), not R2DBC.** The readiness probe reflects
  real Postgres connectivity via Actuator's `db` health indicator, and the
  blocking driver matches the blocking report workload.
- **Flyway migrations** (`spring-boot-starter` idiom, RFC-0001 D2: "the
  heavyweight framework is the lesson") for the `report_jobs` schema -- the
  deliberate contrast with the analytics service's hand-rolled runner. Flyway
  runs at startup and needs Postgres reachable then, so unlike the PR-1
  skeleton the app no longer boots with the DB absent; this is gated (see
  Database below) and the readiness/liveness split is unaffected.
- **Apache POI (XLSX) and OpenPDF (PDF)** for report rendering; CSV needs no
  library. POI is also the deliberate bursty-allocation workload behind the GC
  sawtooth. OpenPDF (a maintained MPL/LGPL fork of iText 4) was chosen over
  lower-level PDFBox for a concise document model and over iText 7+ (AGPL) for
  licence cleanliness. Both are pinned in the version catalog; their CVE
  posture is checked by the `reports.yml` Trivy lockfile audit.
- **Gradle Kotlin DSL + version catalog (`gradle/libs.versions.toml`) +
  committed wrapper.** The wrapper (`gradlew` + `gradle/wrapper/`) pins
  Gradle 8.14.x for reproducible, zero-preinstall builds locally, in CI, and
  in the Docker builder stage; the JDK is pinned in the repo-root `.mise.toml`
  and both are checked against drift by `scripts/check-toolchain-drift.sh`.
- **ktlint** for format + lint (`ktlintCheck` / `ktlintFormat`), wired into
  `make lint` / `make format`. detekt was left out to keep the toolchain lean.

## D6 uniform contract

Implemented with Spring Boot Actuator, no hand-rolled controllers:

- `/healthz` -- liveness, always `200` once the process is up. Actuator
  `liveness` health group (`livenessState`) surfaced at this exact path via
  the group's `additional-path` on the main server port.
- `/readyz` -- readiness = application state + Postgres reachable. Actuator
  `readiness` group (`readinessState` + the `db` datasource health
  indicator); `503` until the pool can reach Postgres, `200` once it can.
  The app still *starts* with the DB absent (the pool initializes lazily and
  readiness reports `503`), then flips ready when Postgres comes up (D10
  spirit) -- there is no analytics dependency in this service to degrade on.
- `/metrics` -- Prometheus exposition format, the Micrometer Prometheus
  registry's Actuator endpoint remapped from `/actuator/prometheus` to
  `/metrics` (root management base-path + endpoint path-mapping).

## Logs and tracing

Structured JSON logs to stdout (`logback-spring.xml`, Logstash encoder).
Every line carries `service`, and `trace_id` / `span_id` from Micrometer
Tracing's MDC -- emitted under those snake_case names for parity with the Go
analytics service's `trace_id` field, so one Loki query correlates across
both (RFC-0001 D11). OpenTelemetry is wired from this first commit
(`micrometer-tracing-bridge-otel`, W3C propagation, sampling 1.0); no
exporter is configured, so spans are generated and dropped until the trace
backend (Tempo) lands (RFC-0001 Section 10) -- log-trace correlation via
Loki works now.

## Endpoints (`:8083`)

- `POST /reports` -- submit a report job (async). See Report API below.
- `GET /reports` -- list recent jobs, newest first (metadata only). See
  Report API below.
- `GET /reports/{id}` -- job status and, when done, a download link.
- `GET /reports/{id}/download` -- stream the generated artifact.
- `/healthz` -- liveness (see D6 uniform contract above).
- `/readyz` -- readiness = Postgres reachable.
- `/metrics` -- Prometheus exposition: JVM runtime/GC/process metrics plus
  HTTP server and HikariCP pool metrics from Micrometer, and the custom
  report-job meters (see Observability below).

## Report API

Async job model (RFC-0001 D2): the POST returns immediately; the job runs
off the request thread on a bounded coroutine dispatcher; the client polls
for status and then downloads.

### `POST /reports`

Request body:

```json
{ "type": "items-summary", "format": "xlsx", "params": {} }
```

- `type` -- report type. Supported: `items-summary` (more slot in without
  touching the job machinery).
- `format` -- `xlsx` | `pdf` | `csv`.
- `params` -- optional, report-type-specific; stored as `jsonb` for audit.

Responses:

- `202 Accepted` with a `Location: /reports/{id}` header and a body
  `{ "id", "status": "PENDING", "location" }`.
- `400 Bad Request` on an unknown `type` or `format`.

### `GET /reports`

Lists recent jobs, **newest first**, for a jobs view (the `reports-ui`
recent-jobs list, RFC-0002 D5). Metadata only -- never artifacts, request
`params`, or `error` detail:

```json
[
  {
    "id": "…", "type": "items-summary", "format": "xlsx",
    "status": "SUCCEEDED",
    "createdAt": "…", "finishedAt": "…",
    "artifactBytes": 12345,
    "download": "/reports/{id}/download"
  }
]
```

- `limit` (query, optional) -- how many rows to return. Clamped server-side to
  `[1, 100]`, default `20`, so a client cannot request an unbounded scan.
  Ordering is index-backed (`report_jobs_created_at_idx`, `created_at DESC`).
- `download` is present on a row only once its artifact exists; callers fetch
  the full record via `GET /reports/{id}` and the file via the download
  sub-resource.

### `GET /reports/{id}`

Returns the job's current state:

```json
{
  "id": "…", "type": "items-summary", "format": "xlsx",
  "status": "SUCCEEDED",
  "createdAt": "…", "startedAt": "…", "finishedAt": "…",
  "error": null, "artifactBytes": 12345,
  "download": "/reports/{id}/download"
}
```

`status` is the job state machine: `PENDING` -> `RUNNING` ->
`SUCCEEDED` | `FAILED`. `download` is present only once `SUCCEEDED`;
`error` carries the failure detail when `FAILED`. `404` for an unknown id.

### `GET /reports/{id}/download`

Streams the artifact with the correct `Content-Type`
(`application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`,
`application/pdf`, or `text/csv`) and a
`Content-Disposition: attachment; filename="{id}.{ext}"`. A separate
download sub-resource (rather than content negotiation on the status
resource) keeps polling cheap and the file response cleanly typed. `404`
for an unknown id or a missing artifact; `409` if the job is not
`SUCCEEDED` (nothing to download yet).

### Report content

The one concrete report, **items summary**, pulls the item list from the
backend REST `/items` (the source of truth) and enriches it with the
analytics last-24h event-type aggregates (`GET /api/v1/stats`). All three
formats render the *same* logical model (one `ItemsSummaryModel`,
format-specific renderers): a summary block (generated-at, total / synthetic
/ real item counts), an items table (`id`, `name`, `synthetic`), and an
analytics section. Synthetic traffic (canary/loadgen items, name-prefixed
per RFC-0001 D9) is tagged in the `synthetic` column and counted separately,
so it is labeled rather than silently inflating the business figures.

### Graceful degradation (RFC-0001 D10, Hard rule 9)

The backend is the source of truth: if it is unreachable the job **fails**
(there is no report without items), surfacing the error on
`GET /reports/{id}`. Analytics is **best-effort**: if it is absent or
unreachable the job still **succeeds** on backend data alone, and the
analytics section carries a distinct "unavailable" note instead of numbers
-- the gap is visible, never hidden, and never a job failure.

### Graceful shutdown and startup reconciliation

The web tier shuts down gracefully (`server.shutdown: graceful`). In-flight
report jobs drain for up to 10s on shutdown; jobs that finish in time complete
normally. A CPU-bound POI/PDF render still running past the grace is
interrupted, but such a render has no coroutine suspension point and may ignore
the thread interrupt (and the worker threads are daemon), so a job still
rendering at JVM exit is **not** marked `FAILED` mid-flight.

The guarantee that no job is left dangling in a non-terminal state comes from
**startup reconciliation** instead: on boot, every job still `PENDING` or
`RUNNING` is reconciled to `FAILED` with a clear error. There is exactly one
reports instance (compose), so any non-terminal row at startup is by definition
orphaned by a prior crash or interrupted shutdown. This is also the
durable-recovery half of the job-state persistence: a crash leaves the last
durable state, the next startup cleans it up.

## Environment variables

| Variable | Default | Meaning |
| -------- | ------- | ------- |
| `REPORTS_HTTP_PORT` | `8083` | HTTP listen port (also serves `/healthz`, `/readyz`, `/metrics`) |
| `REPORTS_DATABASE_URL` | `jdbc:postgresql://localhost:5434/reports` | Postgres JDBC URL (own instance -- see Database below); compose sets this to `jdbc:postgresql://postgres-reports:5432/reports` |
| `REPORTS_DATABASE_USER` | `reports` | Postgres user |
| `REPORTS_DATABASE_PASSWORD` | `reports` | Postgres password (demo-grade default, same posture as every other credential in this repo) |
| `REPORTS_ARTIFACT_DIR` | `./artifacts` | Directory for generated report artifacts; compose mounts a named volume here at `/var/lib/reports/artifacts` |
| `REPORTS_BACKEND_URL` | `http://localhost:8000` | Backend base URL (report source of truth); compose sets `http://api:8000` |
| `REPORTS_ANALYTICS_URL` | `http://localhost:8082` | Analytics base URL (best-effort enrichment, D10); compose sets `http://analytics:8082` |
| `REPORTS_HTTP_TIMEOUT` | `5s` | Per-request timeout for the backend/analytics calls (short, so an absent analytics fails fast into the degraded path) |
| `REPORTS_JOB_CONCURRENCY` | `2` | Bounded report-job dispatcher width -- how many jobs render concurrently (small by design, the GC exhibit) |
| `JAVA_OPTS` | `-XX:MaxRAMPercentage=75.0` | JVM flags (set in the Dockerfile); caps heap at 75% of the container memory limit via cgroup awareness |

## Database

Own Postgres instance (`postgres-reports` in compose, host port `5434` --
next after `db`'s `5432` and `postgres-analytics`' `5433`). Service-owns-its-
store, visible in the compose topology (RFC-0001 D2/D1), not a schema on a
shared database. Generated artifacts go on a named volume, never as BLOBs in
Postgres (RFC-0001 D2, deliberately avoided).

Job metadata lives in a single `report_jobs` table (schema and columns:
[`docs/data.md`](../../docs/data.md)), migrated by **Flyway**
(`src/main/resources/db/migration/`). Flyway is the `spring-boot-starter`
idiom -- "the heavyweight framework is the lesson" (RFC-0001 D2) -- and the
deliberate contrast with the analytics service's hand-rolled runner.

Because Flyway runs migrations at startup and needs Postgres reachable
*then*, the app -- unlike the PR-1 skeleton -- no longer boots with the DB
absent. This is gated, not a regression: compose orders `reports` after
`postgres-reports: service_healthy`, the Testcontainers tests start Postgres
first, and `spring.flyway.connect-retries` covers a DB that is merely slow to
accept connections at boot. The readiness/liveness split still holds exactly:
liveness (`/healthz`) never depends on the DB, and a *running* app that later
loses Postgres keeps `/healthz` at 200 while `/readyz` reports 503 via the
Actuator `db` indicator (the PR-1 skeleton test proves this).

## Observability (dashboard + scrape)

Provisioned Grafana dashboard `Reports JVM`
(`observability/grafana/dashboards/reports.json`, RFC-0001 D6, Hard rule
11), sitting next to the canary/analytics dashboards. The JVM row shows
scrape health (`up{job="reports"}`), uptime, live threads, heap used vs
committed and non-heap, GC pause rate and time by action, HTTP request rate
and p95 latency. As of PR-2 the **GC sawtooth is live**: the report engine's
bursty POI/OpenPDF allocation drives the heap used-line up between
collections and down on each GC (the reason this service exists, RFC-0001
D2). A dedicated **report-job row** reads the custom Micrometer meters:

- `reports_jobs_submitted_total` -- jobs accepted (counter).
- `reports_jobs_completed_total{status,type,format}` -- terminal outcomes.
- `reports_job_duration_seconds` -- job duration (timer with histogram
  buckets, so the dashboard's p95 panel has real quantiles).
- `reports_jobs_inflight` -- jobs currently rendering (gauge, capped at
  `reports.job-concurrency`).
- `reports_artifact_bytes` -- generated artifact size (distribution summary;
  `_sum` rate is the throughput panel).

These are empty until reports are generated (the loadgen report scenario
lands in PR-3; a manual `POST /reports` fills them now). Job lifecycle events
(accepted/started/succeeded/failed) are logged as structured JSON with
`trace_id`/`span_id` and the report id, so a job is traceable end to end in
Loki (RFC-0001 D11).

Prometheus scrapes the service via the `reports` job
(`observability/prometheus.yml`, target `reports:8083/metrics`). `reports`
is an opt-in compose profile, so the target reads "down" on the Prometheus
targets page and panels show "No data" when the profile is not up --
expected, documented degradation (RFC-0001 D10), the same as the
analytics/synthetic-profile targets.

## Dependency audit and locking

Dependency locking is enabled (`dependencyLocking { lockAllConfigurations() }`
in `build.gradle.kts`), pinning every resolved dependency -- transitive
included -- into `gradle.lockfile`. Two purposes: reproducible resolution
(the build fails if a resolved version drifts from the lock), and a
source-level dependency inventory the CI audit scans. The `reports.yml`
audit job runs Trivy over `gradle.lockfile` for CRITICAL CVEs (RFC-0001 D6,
ADR-0011) -- Trivy's filesystem jar analyzer does not descend a Spring Boot
executable jar's `BOOT-INF/lib`, so the committed lockfile, not the jar, is
what it enumerates (every resolved dependency, transitive included).
Regenerate the lockfile after any dependency change:

```shell
./gradlew dependencies --write-locks
```

## Development

```shell
make build   # ./gradlew clean bootJar
make test    # ./gradlew test (integration tests use Testcontainers Postgres -- needs Docker)
make lint    # ./gradlew ktlintCheck
make format  # ./gradlew ktlintFormat
make run     # ./gradlew bootRun (REPORTS_* env vars, see above)
```

Tests are integration tests against a real Postgres in a container
(Testcontainers + Spring Boot `@ServiceConnection`), matching the repo's
testing philosophy (engineering-principles.md Section 7) and the analytics
service's approach -- no mocked database. They need a running Docker daemon
locally; CI provides one on the runner.

Toolchain: the JDK and Gradle versions are pinned in the repo-root
`.mise.toml` (ADR-0012); run `mise install` from the repo root, then `make
doctor` verifies them. The committed Gradle wrapper is what every target,
CI job, and the Docker builder stage actually invoke.

## Docker

Multi-stage build (RFC-0001 D6): a JDK 21 builder stage compiles the
bootable jar with the committed wrapper (dependencies resolved in their own
layer so a source-only edit does not re-download the ecosystem), and a slim
JRE 21 runtime stage runs it as a non-root user. Base images are
digest-pinned (repo convention). The runtime is `eclipse-temurin:21-jre`
with `wget` for the container healthcheck against `/healthz` -- the same
shell-exec probe pattern as the canary (analytics is the exception, its
distroless image carries a `healthcheck` subcommand instead). The JVM caps
its heap at 75% of the container memory limit (`-XX:MaxRAMPercentage`).

## Roadmap

- **PR-2** (`feat/report-engine`, done): the `POST /reports` async job API,
  `GET /reports/{id}` status + `/download`, XLSX/PDF/CSV generation via Apache
  POI / OpenPDF, Flyway job-metadata migrations, custom report-job metrics,
  the live GC-sawtooth dashboard enrichment, and D10 analytics degradation.
- **PR-3**: the report k6 load scenario (`loadgen`) and canary v3 -- a
  report-trigger step in the synthetic journey (RFC-0001 D9), plus the
  nightly JVM e2e workflow.
