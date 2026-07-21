# Prerequisites

Skills and knowledge needed to work with devops-demo. Toolchain versions are
pinned in `.mise.toml`; run `make doctor` to verify your environment.

Platform support: macOS and Linux only.

## Table of Contents

- [Backend](#backend)
- [Frontend](#frontend)
- [Canary](#canary)
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
