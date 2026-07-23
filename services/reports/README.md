# Reports

Kotlin / Spring Boot 3 reports service (RFC-0001 D2) -- the JVM showcase of
the polyglot platform, with its own Postgres instance and an artifact
volume. This PR (`feat/reports-skeleton`, RFC-0001 Phase 6 PR-1) ships the
skeleton and the D6 uniform service contract only: the app starts, connects
to Postgres, and serves `/healthz`, `/readyz`, `/metrics` with structured
JSON logs and OpenTelemetry wired in. Report generation itself (`POST
/reports` -> `GET /reports/{id}`, XLSX/PDF/CSV) lands in PR-2.

## Why a heavyweight framework here

The JVM is on this platform to teach what the JVM makes visible (RFC-0001
D2): startup time, heap sizing against a container memory limit, GC
behaviour under bursty allocation, and readiness-vs-liveness. Spring Boot
was chosen over a lean framework (Ktor) deliberately -- the framework's
weight *is* the lesson. Report generation (Apache POI / PDF, PR-2) is
bursty, blocking allocation; that workload is what makes the GC sawtooth
show up on the `Reports JVM` Grafana dashboard (shipped here in PR-1, see
Observability below; PR-2's workload is what drives its heap/GC panels).

## Stack and the choices behind it

- **Spring Boot 3.5 + Kotlin, Spring MVC (not WebFlux), JDK 21 virtual
  threads.** Report generation is blocking (POI/PDF); virtual threads carry
  blocking work at scale without a reactive rewrite, and the blocking model
  keeps the heap/GC lesson legible. Coroutines (RFC-0001 D2) are wired
  (`kotlinx-coroutines-reactor`) for the async job orchestration PR-2 adds --
  Spring MVC supports `suspend` handlers directly.
- **JDBC datasource (HikariCP), not R2DBC.** The readiness probe reflects
  real Postgres connectivity via Actuator's `db` health indicator, and the
  blocking driver matches the blocking report workload. No schema or
  migrations yet (PR-2); the skeleton only needs a live pool so the app
  becomes ready.
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

- `/healthz` -- liveness (see D6 uniform contract above).
- `/readyz` -- readiness = Postgres reachable.
- `/metrics` -- Prometheus exposition: JVM runtime/GC/process metrics plus
  HTTP server and HikariCP pool metrics from Micrometer.

## Environment variables

| Variable | Default | Meaning |
| -------- | ------- | ------- |
| `REPORTS_HTTP_PORT` | `8083` | HTTP listen port (also serves `/healthz`, `/readyz`, `/metrics`) |
| `REPORTS_DATABASE_URL` | `jdbc:postgresql://localhost:5434/reports` | Postgres JDBC URL (own instance -- see Database below); compose sets this to `jdbc:postgresql://postgres-reports:5432/reports` |
| `REPORTS_DATABASE_USER` | `reports` | Postgres user |
| `REPORTS_DATABASE_PASSWORD` | `reports` | Postgres password (demo-grade default, same posture as every other credential in this repo) |
| `REPORTS_ARTIFACT_DIR` | `./artifacts` | Directory for generated report artifacts (PR-2); compose mounts a named volume here at `/var/lib/reports/artifacts` |
| `JAVA_OPTS` | `-XX:MaxRAMPercentage=75.0` | JVM flags (set in the Dockerfile); caps heap at 75% of the container memory limit via cgroup awareness |

## Database

Own Postgres instance (`postgres-reports` in compose, host port `5434` --
next after `db`'s `5432` and `postgres-analytics`' `5433`). Service-owns-its-
store, visible in the compose topology (RFC-0001 D2/D1), not a schema on a
shared database. No migrations in this PR; report job metadata schema lands
in PR-2. Generated artifacts go on a named volume, never as BLOBs in
Postgres (RFC-0001 D2, deliberately avoided).

## Observability (dashboard + scrape)

Provisioned Grafana dashboard `Reports JVM`
(`observability/grafana/dashboards/reports.json`, RFC-0001 D6, Hard rule
11), sitting next to the canary/analytics dashboards. Phase 6 PR-1 panels
come from metrics the skeleton already exposes at `/metrics`: scrape health
(`up{job="reports"}`), uptime, live threads, heap used vs committed and
non-heap, GC pause rate and time by action, HTTP request rate and p95
latency. A note panel flags that the GC-sawtooth exhibit -- bursty heap
allocation from POI/PDF generation making GC visible, the reason this
service exists (RFC-0001 D2) -- lands with PR-2's report workload, which
enriches the heap/GC panels.

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
what it enumerates (133 dependencies at time of writing). Regenerate the
lockfile after any dependency change:

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
its heap at 75% of the container memory limit (`-XX:MaxRAMPercentage`); the
deeper GC-visibility tuning arrives with PR-2's report workload.

## Roadmap

- **PR-2** (`feat/reports-generation`): the `POST /reports` async job API and
  `GET /reports/{id}` status/download, XLSX/PDF/CSV generation via Apache
  POI, job-metadata migrations, enrichment of the JVM dashboard with the
  GC-sawtooth panels the bursty POI workload makes visible, and the report
  k6 scenario.
- **PR-3**: canary v3 -- a report-trigger step in the synthetic journey
  (RFC-0001 D9).
