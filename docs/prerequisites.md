# Prerequisites

Skills and knowledge needed to work with devops-demo. Toolchain versions are
pinned in `.mise.toml`; run `make doctor` to verify your environment.

Platform support: macOS and Linux only.

## Table of Contents

- [Backend](#backend)
- [Frontend](#frontend)
- [Canary](#canary)
- [Analytics](#analytics)
- [Reports](#reports)
- [Reports UI](#reports-ui)
- [Proto](#proto)
- [Loadgen](#loadgen)
- [Kubernetes](#kubernetes)
- [Infrastructure](#infrastructure)
- [Learning Resources](#learning-resources)

---

## Backend

### Python

- Async/await, asyncio event loop, coroutines
- Type hints, generics, Pydantic-style annotations
- Context managers, decorators, comprehensions
- OOP: classes, inheritance, polymorphism
- Modules, packages, virtual environments

### FastAPI

- REST endpoints, path/query/body parameters
- Dependency injection via `Depends()`
- Pydantic request/response models
- Middleware, CORS, OAuth2/JWT basics
- Auto-generated OpenAPI docs

### SQLAlchemy (async)

- Declarative models, relationships, foreign keys
- `AsyncSession`, connection pooling, transactions
- ORM queries vs Core; eager/lazy loading
- Alembic migrations: autogenerate, upgrade/downgrade, data migrations

### PostgreSQL

- SQL DDL: tables, constraints, indexes
- Transactions, ACID, EXPLAIN ANALYZE
- Joins, aggregations, CTEs
- Connection strings, pg_isready health checks

### Backend Testing

- pytest: fixtures, parametrize, markers, plugins
- pytest-asyncio: async fixtures, event loop management
- httpx: `AsyncClient` + `ASGITransport` for FastAPI integration tests

### Backend Code Quality

- Ruff: linting + Black-style formatting, import sorting
- Mypy: strict static type checking, pyproject.toml config
- Git hooks run via prek -- see [engineering-principles.md](engineering-principles.md)

---

## Frontend

### JavaScript / JSX

- ES2020+: arrow functions, destructuring, spread, optional chaining,
  nullish coalescing, async/await, modules
- JSX: expressions, conditional rendering, lists, event handling

### React

- Functional components, hooks (useState, useEffect, useCallback,
  useMemo, useContext, useRef)
- Props, state, Context API, error boundaries
- Component composition, memoization

### Vite

- Dev server with HMR, production builds, code splitting
- `VITE_*` environment variables, plugin ecosystem

### Frontend Testing

- Vitest: test structure, mocking (`vi.mock`, `vi.fn`, `vi.spyOn`), coverage
- React Testing Library: `render`, `screen` queries, `user-event`, `waitFor`
- `@testing-library/jest-dom` matchers; jsdom environment

### Frontend Code Quality

- ESLint + Prettier: linting, formatting, editor integration

### Web APIs

- Fetch API: requests, headers, response handling, CORS
- Web Vitals: CLS, INP, LCP, FCP, TTFB

---

## Canary

### Rust

- Ownership, borrowing, lifetimes; `Option`/`Result` and `?`
- `async`/`await` on Tokio; tasks, `select!`, graceful shutdown
- Traits, generics, `impl Trait`

### axum + tokio

- Router, extractors (`State`), handlers returning `IntoResponse`
- Shared state via `Arc`; `axum::serve` with graceful shutdown

### Observability in Rust

- `prometheus` crate: `Registry`, `CounterVec`/`HistogramVec`/`Gauge`
- `tracing` + `tracing-subscriber` (JSON layer); `tracing-opentelemetry`
  and W3C trace-context propagation (see ADR-0010)

### Canary Testing

- `wiremock`: mock HTTP servers for integration tests, no real network
- `#[tokio::test]` async test functions

### Canary Code Quality

- `cargo fmt`, `cargo clippy -D warnings`
- `cargo-deny`: license/advisory/source policy (`services/canary/deny.toml`)

---

## Analytics

### Go

- Goroutines, channels, `select`; `context.Context` for cancellation/timeouts
- `error` values and `errors.Is`/`errors.As`; `defer`
- `embed.FS` for compiling SQL migrations into the binary
- Structured logging via `log/slog`

### pgx (Postgres driver)

- `pgxpool`: connection pooling, `Ping`, bounded retry on startup
- Parameterized queries, `QueryRow`/`Exec`, transactions (`Begin`/`Commit`/`Rollback`)

### Observability in Go

- `prometheus/client_golang`: `promhttp.Handler`, `promauto`, custom gauges
- OpenTelemetry Go SDK: `TracerProvider`, `propagation.TraceContext`,
  `otelhttp` middleware; see ADR-0010
- `log/slog` custom `Handler` wrapping (trace ID injection)

### Analytics Testing

- `net/http/httptest`: handler tests against a fake `Pinger`, no real Postgres
- Integration tests gated on `ANALYTICS_TEST_DATABASE_URL` (skipped without a
  real Postgres -- see `services/analytics/Makefile`)

### Analytics Code Quality

- `gofmt`, `go vet`, `golangci-lint run` (`services/analytics/.golangci.yml`)
- `govulncheck`: known-vulnerability scan against the module + stdlib

### Data model semantics (ADR-0005)

- Event-time vs arrival-time bucketing
- Mutable upserts (`ON CONFLICT DO UPDATE`) vs stream-processor watermarks
- Liveness vs readiness for a service with an optional upstream dependency
  (RFC-0001 D10)

---

## Reports

The `reports` service (RFC-0001 Phase 6, D2) is the JVM exhibit: the same
uniform service contract (D6) implemented on a fifth runtime.

### Kotlin

- Null safety (`?`, `?:`, `!!`), data classes, sealed classes, enums
- Extension functions, scope functions (`let`, `apply`, `run`), `val`/`var`
- Collections API, destructuring, string templates

### Spring Boot

- Constructor injection, component scanning, `@RestController`,
  `@Service`, `@Configuration`
- Typed configuration (`@ConfigurationProperties`) bound from environment
  variables; profiles
- Spring Web MVC: request mapping, `ResponseEntity`, status codes, the
  `Location` header on `202 Accepted`
- Spring Boot Actuator: health groups behind `/healthz` and `/readyz`
  (the D6 contract paths, not the Actuator defaults)

### Persistence

- Spring JDBC (`JdbcTemplate` + `RowMapper`), not an ORM
- Flyway: versioned migrations, baseline, migration-on-startup
- PostgreSQL: the reports service owns its own database
  (`postgres-reports`), same ownership rule as analytics (ADR-0005)

### Concurrency on the JVM

- `kotlinx-coroutines`: `suspend` functions, structured concurrency,
  dispatchers
- Bounded job dispatch (`reports.job-concurrency`) and why the bound is
  the point: it is what makes the heap sawtooth legible
- Job lifecycle as state: `PENDING` -> `RUNNING` -> `SUCCEEDED`/`FAILED`,
  polled by the client rather than pushed

### Report generation

- Apache POI (XLSX) -- also the deliberate bursty-allocation workload
  behind the GC-sawtooth exhibit
- OpenPDF (PDF); CSV needs no library
- Artifact storage on a volume, streamed back on download

### Observability on the JVM

- Micrometer with the Prometheus registry; JVM metrics: heap used vs
  committed, GC pause count and time, thread and class-loading metrics
- `micrometer-tracing-bridge-otel`: W3C trace-context propagation across
  the same mesh as the Python/Go/Rust services (ADR-0010)
- JSON logs via `logstash-logback-encoder` (D6 contract)
- Graceful degradation (D10): a report succeeds with an "unavailable"
  analytics section rather than failing when analytics is down

### Reports Testing

- JUnit 5 (`useJUnitPlatform`), `spring-boot-starter-test`
- Testcontainers: a real PostgreSQL per test run, no in-memory substitute
- `spring-boot-testcontainers` service connections

### Reports Code Quality

- Gradle Kotlin DSL, the version catalog (`libs.versions.toml`) as the
  single place a dependency version is written
- `ktlint` via the Gradle plugin
- Dependency pinning that overrides the Boot BOM when a CVE requires it

---

## Reports UI

The `reports-ui` service (RFC-0002, ADR-0013) is an integration exhibit, not
a sixth language: a static single-page app over the reports API, served and
reverse-proxied by Caddy.

### The web platform without a framework

- Vanilla HTML/CSS/JS with **no build step** (RFC-0002 D3) -- the
  deliberate opposite of the React `frontend`
- DOM APIs: `querySelector`, element creation, event listeners
- `fetch` against the same origin under `/api/*`: no CORS, no hardcoded
  host

### Polling a job-based API

- Submit (`202` + job id from `Location`), poll until a terminal state,
  then offer the download link
- Rendering server state (recent jobs, newest first) rather than local
  state

### Caddy

- The Caddyfile as the whole service: `handle` blocks, `root` +
  `file_server` for the static assets, and `handle_path /api/*` whose
  prefix strip is what makes `reverse_proxy` hit the upstream path
- Why a reverse proxy makes the SPA same-origin, and what that removes
  (CORS, a build-time API host)
- Caddy's native Prometheus metrics: the `metrics` global option plus the
  `metrics` handler on the service listener

### Operability of a static server

- The D6 contract applies unchanged: `/healthz`, `/readyz`, `/metrics` on
  `:8084`, served by the proxy itself and not forwarded upstream
- Why `/readyz` deliberately does not depend on the reports API being up
  (RFC-0002 D7/D10)

---

## Proto

The `proto/` module (ADR-0002) is the single cross-service source of truth
for the backend/analytics gRPC contract.

### buf

- Install via `mise install` (pinned in `.mise.toml`); verify with
  `make doctor`
- `buf.yaml`: module config, lint rules, breaking-change category
- Commands: `buf lint`, `buf format`, `buf breaking` -- governs the
  contract itself; does not generate Python code (see below)

### Python gRPC codegen

- `make generate` (RFC-0001 D8, ADR-0002), or `make generate-backend` for
  this half alone: a single `python -m
  grpc_tools.protoc` invocation produces message classes, `.pyi` stubs,
  and the gRPC service stubs into `services/backend/app/proto_gen`
  (gitignored, never committed). `grpcio-tools` bundles its own protoc,
  so this needs no separate `protoc`/`buf generate` step -- an earlier
  approach routed python/pyi generation through `buf generate`'s
  `protoc_builtin` plugins, which turned out to silently depend on a
  system `protoc` binary and broke in clean CI/Docker environments.
- Run once after clone (or after a `proto/` change) for IDE completion.

### Go gRPC codegen

- `make generate` also runs `buf generate` (or `make generate-analytics` for
  this half alone) (`proto/buf.gen.yaml`, Go-only:
  managed mode; `protoc-gen-go`/`protoc-gen-go-grpc` invoked via `go tool`,
  versioned by go.mod tool directives, Renovate-managed) into
  `services/analytics/internal/pb` (gitignored, never committed). Unlike
  the Python path, this one goes through `buf generate` directly -- the
  plugins are real standalone Go binaries (`go install`-able), not a
  bundled protoc the way `grpcio-tools` is for Python.

### grpcurl (optional)

- Manual gRPC exploration against the backend's `:50051` (`grpcurl -plaintext
  localhost:50051 list`, `grpcurl -plaintext localhost:50051
  devopsdemo.items.v1.ItemService/ListItems`) -- see
  [exercise 02](exercises/02-grpc-contract.md). Not required: a small
  Python client works just as well.

### Protobuf / gRPC

- proto3 syntax: messages, enums, services, well-known types
  (`google.protobuf.Timestamp`)
- proto3 field presence: default values vs "unset" (see items.proto
  comments for the discipline used here)
- Unary vs server-streaming RPCs; connection direction vs data direction
  (ADR-0002)

---

## Loadgen

`loadgen/` (RFC-0001 D4, ADR-0006) is a single k6 image running scripts
against the shared load profile (`loadprofile/`, RFC-0001 D5).

### k6 scripting

- k6's own JS dialect: ES modules, an init context that runs once before
  any VU/iteration (script-level `import`/top-level code, including
  `open()` for local files and `client.load()` for gRPC protos) versus
  per-iteration exported functions
- `ramping-arrival-rate` / `constant-arrival-rate` executors: `stages`
  computed once at init time in this repo (`loadgen/lib/schedule.js`),
  not evaluated live per request -- see `loadgen/README.md`'s "Reference
  time" section for why
- `http.expectedStatuses()` / `responseCallback`: marking an intentional
  4xx as not-a-failure, distinct from k6 `check()` (pass/fail reporting,
  a separate metric)
- Custom metrics (`k6/metrics`: `Rate`, `Counter`, `Trend`) for
  correctness signals k6's built-ins don't cover (e.g. gRPC has no
  `http_req_failed` equivalent)
- `k6/net/grpc`: unary `client.invoke()`, proto loaded at runtime from a
  mounted/copied `.proto` file, not compiled in

### k6 outputs

- `-o experimental-prometheus-rw`: built into the stock k6 v2.x binary
  (not an xk6 extension); requires Prometheus's
  `--web.enable-remote-write-receiver`
- Metric name mapping worth knowing when writing a dashboard query:
  Counter -> `k6_<name>_total`, Rate -> `k6_<name>_rate`, Gauge ->
  `k6_<name>` (no suffix), Trend -> `k6_<name>_<stat>` per
  `K6_PROMETHEUS_RW_TREND_STATS` (e.g. `_p95`, `_p99`, `_avg`) -- every
  k6 tag (including the automatic `scenario` tag) becomes a label

### Threshold design (D4 CI-gate contract)

- Per-scenario submetrics via tags (`http_req_duration{scenario:browse}`),
  not one blanket threshold across a mixed scenario set
- A Rate-type remote-write metric (e.g. `k6_http_req_failed_rate`) is a
  snapshot fraction per unique tag combination per push interval --
  averaging it across combinations is not weighted by request volume and
  reads wrong; a Counter's `rate()` with a tag filter
  (`expected_response="false"`) is the request-volume-correct way to
  compute an aggregate error rate for a dashboard

---

## Kubernetes

The second supported deployment target (RFC-0003): the same platform on Kind,
behind Gateway API, packaged as Helm charts over one library chart.

### Core objects

- Pod, Deployment, Service, ConfigMap, Secret, Job; namespaces and labels
- The **pod template** is what a rollout watches: nothing about a referenced
  ConfigMap or Secret triggers one on its own (exercise 10)
- Liveness and readiness probes, and how they map onto the D6 contract paths
  (`/healthz`, `/readyz`) the compose stack already serves
- Requests and limits, and why the measured per-service footprint is where
  the numbers come from rather than a round guess

### Kind

- A multi-node cluster in Docker; the node image is a Kubernetes version and
  is digest-pinned (`deploy/k8s/kind/cluster.yaml`)
- `kind load` puts locally built images into the cluster -- there is no
  registry in the loop
- `kubectl` tolerates one minor of skew from the API server, so the client
  pin and the node image move together
  (`scripts/check-toolchain-drift.sh`)

### Helm

- Chart layout: `Chart.yaml`, `values.yaml`, `templates/`, dependencies
- Go templating: `include`, named templates, `tpl`, `required`, pipelines
- An **umbrella chart** over per-service charts, all sharing one **library
  chart** (`charts/common`) that makes the D6 contract executable rather
  than documented -- probes, ports, ServiceMonitor and the config checksum
  are written once
- `checksum/config` annotations on the pod template: the standard answer to
  the config-only upgrade that would otherwise be a silent no-op
- `helm upgrade --install`, release lifecycle, values precedence

### Validating without a cluster

- `helm template | kubeconform -strict` per profile, offline
  (`make lint-k8s`, `deploy/k8s/scripts/validate.sh`) -- CI runs the same
  script
- CRD schemas have to be supplied (`deploy/k8s/schemas/`): a strict
  validator does not know `gateway.networking.k8s.io` or
  `monitoring.coreos.com` on its own

### Gateway API and Envoy Gateway

- `Gateway`, `HTTPRoute`, `GRPCRoute`, and the implementation-specific
  `EnvoyProxy` parameters
- One entry point routed by hostname, replacing compose's port-per-service
  publishing -- the same services, addressed differently
- Ingress vs Gateway API: why the newer API splits infrastructure ownership
  from route ownership

### Prometheus Operator

- `ServiceMonitor` and `PrometheusRule` as CRs replacing a static scrape
  file and a rules file
- `jobLabel` names a label **on the Service** whose value becomes the `job`
  label on every scraped series -- the difference between a green pipeline
  and a dashboard that matches nothing (exercise 09)
- Operator-generated labels vs the labels existing queries were written
  against; portability of queries across both stacks

### Operating a cluster

- `kubectl get/describe/logs/exec`, `rollout status`, `-o jsonpath`
- `port-forward` and its cost: a forgotten forward keeps the local port, so
  the next process that wants it fails loudly (`bind: address already in
  use`) -- while a client that assumed the port belonged to something else
  goes on getting plausible answers from the wrong backend, which is the
  half nobody notices (exercise 09)
- Reading events and pod state when a deploy reports success and nothing
  changed

---

## Infrastructure

### Docker

- Dockerfile syntax, multi-stage builds, layer caching, `.dockerignore`
- Image security best practices

### Docker Compose

Requires Docker Compose v2 (`docker compose` plugin -- v1 is not supported).

- Service definitions, networks, volumes, health checks
- Environment variables, dependency ordering
- Compose files live in `deploy/compose/`

### Observability

The full observability stack is documented in [observability.md](observability.md).

- **Prometheus**: metric types (Counter, Gauge, Histogram, Summary), PromQL,
  scrape config, alerting rules
- **Grafana**: dashboards, data sources, provisioning
- **Loki**: LogQL, label-based indexing, log aggregation
- **Grafana Alloy**: Docker log collection, label extraction, metrics forwarding
- **Postgres Exporter**: PostgreSQL metrics for Prometheus

### CI/CD

CI workflows are documented in [ci.md](ci.md).

- GitHub Actions: workflow YAML, jobs, steps, secrets, matrix builds,
  service containers, artifact caching
- Renovate: automated dependency updates across all ecosystems (see docs/ci.md)

### Code Quality Tools

- yamllint: YAML syntax + style validation
- hadolint: Dockerfile best-practice and security checks
- Make: task automation (`make doctor`, `make test`, `make lint`, etc.)

### General Concepts

- **YAML**: syntax, anchors, multi-line strings
- **nginx**: reverse proxy, static file serving, location routing
- **Git**: conventional commits, PR workflow, branch management
- **Environment variables**: `.env` files, Docker Compose env, CI secrets
- **Networking**: ports, Docker networks, DNS resolution, service discovery
- **Volumes**: named vs anonymous, bind mounts, lifecycle management

---

## Learning Resources

### Backend resources

| Topic | Link |
| --- | --- |
| Python docs | <https://docs.python.org/3/> |
| asyncio | <https://docs.python.org/3/library/asyncio.html> |
| Real Python | <https://realpython.com/> |
| FastAPI docs | <https://fastapi.tiangolo.com/> |
| FastAPI tutorial | <https://fastapi.tiangolo.com/tutorial/> |
| SQLAlchemy docs | <https://docs.sqlalchemy.org/> |
| SQLAlchemy async | <https://docs.sqlalchemy.org/en/20/orm/extensions/asyncio.html> |
| Alembic docs | <https://alembic.sqlalchemy.org/> |
| pytest docs | <https://docs.pytest.org/> |
| pytest-asyncio | <https://pytest-asyncio.readthedocs.io/> |
| Ruff docs | <https://docs.astral.sh/ruff/> |
| Mypy docs | <https://mypy.readthedocs.io/> |
| PostgreSQL docs | <https://www.postgresql.org/docs/> |

### Frontend resources

| Topic | Link |
| --- | --- |
| React docs | <https://react.dev/> |
| Vite docs | <https://vitejs.dev/> |
| Vitest docs | <https://vitest.dev/> |
| React Testing Library | <https://testing-library.com/docs/react-testing-library/intro/> |
| Web Vitals | <https://web.dev/vitals/> |

### Analytics resources

| Topic | Link |
| --- | --- |
| Go Tour | <https://go.dev/tour/> |
| Effective Go | <https://go.dev/doc/effective_go> |
| pgx docs | <https://pkg.go.dev/github.com/jackc/pgx/v5> |
| client_golang docs | <https://pkg.go.dev/github.com/prometheus/client_golang> |
| OpenTelemetry Go | <https://opentelemetry.io/docs/languages/go/> |
| log/slog docs | <https://pkg.go.dev/log/slog> |
| golangci-lint docs | <https://golangci-lint.run/> |
| govulncheck docs | <https://go.dev/security/vuln/> |

### Reports resources

| Topic | Link |
| --- | --- |
| Kotlin docs | <https://kotlinlang.org/docs/home.html> |
| Kotlin coroutines | <https://kotlinlang.org/docs/coroutines-guide.html> |
| Spring Boot docs | <https://docs.spring.io/spring-boot/index.html> |
| Spring Boot Actuator | <https://docs.spring.io/spring-boot/reference/actuator/index.html> |
| Flyway docs | <https://documentation.red-gate.com/flyway> |
| Micrometer docs | <https://docs.micrometer.io/micrometer/reference/> |
| Apache POI docs | <https://poi.apache.org/components/spreadsheet/> |
| Testcontainers docs | <https://java.testcontainers.org/> |
| Gradle Kotlin DSL | <https://docs.gradle.org/current/userguide/kotlin_dsl.html> |
| ktlint docs | <https://pinterest.github.io/ktlint/latest/> |

### Reports UI resources

| Topic | Link |
| --- | --- |
| Caddy docs | <https://caddyserver.com/docs/> |
| Caddyfile directives | <https://caddyserver.com/docs/caddyfile/directives> |
| Caddy metrics | <https://caddyserver.com/docs/metrics> |
| MDN Web Docs | <https://developer.mozilla.org/> |
| Fetch API | <https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API> |

### Proto resources

| Topic | Link |
| --- | --- |
| Protobuf docs | <https://protobuf.dev/> |
| gRPC docs | <https://grpc.io/docs/> |
| buf docs | <https://buf.build/docs/> |

### Canary resources

| Topic | Link |
| --- | --- |
| The Rust Book | <https://doc.rust-lang.org/book/> |
| Tokio docs | <https://tokio.rs/tokio/tutorial> |
| axum docs | <https://docs.rs/axum/latest/axum/> |
| tracing docs | <https://docs.rs/tracing/latest/tracing/> |
| OpenTelemetry Rust | <https://opentelemetry.io/docs/languages/rust/> |
| wiremock docs | <https://docs.rs/wiremock/latest/wiremock/> |
| cargo-deny docs | <https://embarkstudios.github.io/cargo-deny/> |

### Kubernetes resources

| Topic | Link |
| --- | --- |
| Kubernetes docs | <https://kubernetes.io/docs/home/> |
| Kubernetes concepts | <https://kubernetes.io/docs/concepts/> |
| kind docs | <https://kind.sigs.k8s.io/> |
| Helm docs | <https://helm.sh/docs/> |
| Helm library charts | <https://helm.sh/docs/topics/library_charts/> |
| kubeconform | <https://github.com/yannh/kubeconform> |
| Gateway API | <https://gateway-api.sigs.k8s.io/> |
| Envoy Gateway | <https://gateway.envoyproxy.io/docs/> |
| Prometheus Operator | <https://prometheus-operator.dev/docs/getting-started/introduction/> |

### Infrastructure resources

| Topic | Link |
| --- | --- |
| Docker docs | <https://docs.docker.com/> |
| Docker Compose docs | <https://docs.docker.com/compose/> |
| Prometheus docs | <https://prometheus.io/docs/> |
| Grafana docs | <https://grafana.com/docs/> |
| Loki docs | <https://grafana.com/docs/loki/latest/> |
| Grafana Alloy docs | <https://grafana.com/docs/alloy/latest/> |
| GitHub Actions docs | <https://docs.github.com/en/actions> |
| Renovate docs | <https://docs.renovatebot.com/> |

### General

| Topic | Link |
| --- | --- |
| DevOps Roadmap | <https://roadmap.sh/devops> |
| 12-Factor App | <https://12factor.net/> |
| Pro Git | <https://git-scm.com/book> |
