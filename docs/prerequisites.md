# Prerequisites

Skills and knowledge needed to work with devops-demo. Toolchain versions are
pinned in `.mise.toml`; run `make doctor` to verify your environment.

Platform support: macOS and Linux only.

## Table of Contents

- [Backend](#backend)
- [Frontend](#frontend)
- [Canary](#canary)
- [Analytics](#analytics)
- [Proto](#proto)
- [Loadgen](#loadgen)
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
