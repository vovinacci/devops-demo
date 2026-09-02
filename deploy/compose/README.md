# Docker Compose stack

The RFC-0001/0002 stack on Docker Compose: the fast local path, and the one
every exercise assumes unless it says otherwise. Its counterpart is
[`deploy/k8s/README.md`](../k8s/README.md), which runs the same system on
Kubernetes. Both are supported; neither replaces the other.

Compose is where you get the whole platform running in one command with the
smallest possible footprint. Kubernetes is where you see what changes when the
same system has to be scheduled, discovered and routed.

## Run it

```sh
make up                # the core stack
make up-full           # every optional profile
make seed              # 20 items
make down              # stop everything, all profiles
```

Raw `docker compose` from the repo root does not work -- the file lives here
and the project directory is the root, so use the make targets or the explicit
form:

```sh
docker compose -f deploy/compose/docker-compose.yml --project-directory . <cmd>
```

`make help` lists every target.

## Profiles

The core stack is profile-less. Additive profiles opt in more services:

| Profile | Adds |
| --- | --- |
| (none) | Postgres, backend, frontend, Prometheus, Grafana, Loki, Alloy, postgres_exporter, cAdvisor, Alertmanager, Mailpit |
| `analytics` | Go analytics service + its own Postgres |
| `synthetic` | Rust canary + blackbox_exporter |
| `reports` | Kotlin/Spring Boot reports service + its own Postgres |
| `reports-ui` | Caddy-served static SPA over the reports API |
| `load` | k6 load generator |

`make up-full` starts all of them. Combine them granularly on a constrained
laptop:

```sh
docker compose -f deploy/compose/docker-compose.yml --project-directory . \
  --profile analytics --profile load up -d
```

`reports` is the largest RAM increment of the set -- a JVM service plus its own
Postgres, which is precisely the point of the D2 "JVM showcase" exhibit. Leave
it out unless you are exercising it.

## URLs

Every service publishes on loopback only, and on IPv4 (`127.0.0.1`)
specifically -- so if a tool resolves `localhost` to `::1` first and does not
fall back, use `http://127.0.0.1:<port>` instead. Ports are the compose
stack's; the Kubernetes stack routes the same services by hostname through a
single gateway port instead (see its README).

Services marked **no UI** answer on their API paths and nothing at `/` -- a
bare port is a `404` by design, so the URL below is the endpoint worth
opening. The ones this repo builds also serve the uniform contract
(`/healthz`, `/readyz`, `/metrics`); Loki is third-party and implements none
of it, exposing `/ready` and `/metrics` instead.

| Service | URL | Credentials | Notes |
| --- | --- | --- | --- |
| Frontend | http://localhost:8080 | -- | React SPA, CRUD over the API |
| API | http://localhost:8000/items | -- | FastAPI REST, no UI: browse it through the docs below |
| API (gRPC) | localhost:50051 | -- | ItemService -- backend serves, analytics dials |
| API docs | http://localhost:8000/docs | -- | Swagger UI |
| API docs (ReDoc) | http://localhost:8000/redoc | -- | Alternative rendering |
| Grafana | http://localhost:3000 | admin/admin | Dashboards for metrics and logs |
| Prometheus | http://localhost:9090 | -- | Metrics, targets, rules |
| Loki | http://localhost:3100/ready | -- | Log API, no UI |
| Alertmanager | http://localhost:9093 | -- | Routing, grouping, silencing |
| Mailpit | http://localhost:8025 | -- | Visible alert receiver: SMTP sink + web UI |
| Postgres Exporter | http://localhost:9187 | -- | PostgreSQL metrics |
| cAdvisor | http://localhost:8081 | -- | Container resource metrics |
| Analytics API | http://localhost:8082/api/v1/stats | -- | `analytics` profile, no UI |
| Canary | http://localhost:8085/metrics | -- | `synthetic` profile, no UI: a probe, not a service to browse |
| Reports API | http://localhost:8083/reports | -- | `reports` profile, no UI: the UI is `reports-ui` below |
| Reports UI | http://localhost:8084 | -- | `reports-ui` profile |

## Seeding

```sh
make seed                                     # 20 items
make seed-reset                               # clear, then seed
make seed-history                             # 90 days of analytics history
SEED_DAYS=3 make seed-history                 # a quicker local check
```

`seed-history` needs the `analytics` profile already running -- it does not
start it for you (RFC-0001 Phase 5 D5).

## Workshop mode

All profiles at `DEMO_TIME_SCALE=24`, so one profile-day compresses into an
hour of wall-clock time (RFC-0001 D5):

```sh
make up-workshop
DEMO_TIME_SCALE=24 SEED_DAYS=3 make seed-history
```

Seed at the **same scale** the stack is running at, or loadgen's scale guard
refuses to start against the mismatched seed marker. (No marker yet, or
analytics absent: it continues.) See
[Exercise 05](../../docs/exercises/05-find-the-seeded-anomalies.md) for what
to do with the result.

## Overrides

`.env.example` at the repo root documents the compose overrides. Copy it to
`.env` and edit if you need to change the database name, credentials or
`DEMO_TIME_SCALE` defaults.

## Also see

- [Local setup](../../docs/local-setup.md) -- toolchains, running a single
  service outside compose, hooks, tests
- [Troubleshooting](../../docs/troubleshooting.md)
- [Observability](../../docs/observability.md) -- what the dashboards show
- [`deploy/k8s/README.md`](../k8s/README.md) -- the same system on Kubernetes
