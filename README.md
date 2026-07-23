# DevOps Demo

A teaching platform: a complete example of taking a service from code to an
operated system -- tested, containerized, monitored, documented. "Done" means
*operable*, not "the endpoint returns 200".

The *process* is part of the product: decisions live in RFCs and ADRs,
conventions in the engineering principles, and every change follows them.
The platform is evolving from the current two-service baseline into a
polyglot system (Python, JS, Go, Rust, Kotlin) with contract-first gRPC,
shaped load generation, and three-layer monitoring -- see
[RFC-0001](docs/rfc/0001-polyglot-platform.md) for the plan and
[RFC-0000](docs/rfc/0000-baseline-retrospective.md) for the baseline.

## Structure

```text
devops-demo/
├── .github/                     # Workflows, PR/issue templates, Renovate config
├── deploy/
│   └── compose/
│       └── docker-compose.yml   # Main Docker Compose configuration
│
├── docs/                        # Documentation (see index below)
│   ├── adr/                     # Architecture Decision Records
│   ├── rfc/                     # Request for Comments -- design documents
│   ├── exercises/               # Student exercises
│   └── runbooks/                # Alert runbooks (meaning, triage, remediation)
│
├── loadgen/                      # k6 scenarios + Dockerfile (RFC-0001 D4, ADR-0006)
│   ├── scenarios/                # main.js (long-running) + incident.js (make incident)
│   └── lib/                      # env parsing + shared-profile stage scheduling
│
├── loadprofile/                 # Shared load-shape definition (RFC-0001 D5, ADR-0003)
│   └── parity/                  # Golden-file cross-language parity test + goldens
│
├── observability/               # Observability configuration
│   ├── prometheus.yml           # Prometheus configuration
│   ├── prometheus_slo_rules.yml # SLO recording rules
│   ├── prometheus_alerts.yml    # Alerting rules
│   ├── blackbox/                # blackbox_exporter config (synthetic profile)
│   ├── grafana/                 # Dashboards (JSON) + provisioning
│   ├── loki/                    # Loki configuration
│   └── alloy/                   # Grafana Alloy configuration
│
├── proto/                       # buf module -- cross-service gRPC contract (ADR-0002)
│   ├── buf.yaml                 # Module config: lint rules, breaking-change category
│   └── devopsdemo/items/v1/     # devopsdemo.items.v1: ItemService (backend serves)
│
├── scripts/                     # Doctor, toolchain drift gate
├── services/                    # Self-contained services (own Dockerfile,
│   ├── backend/                 #   Makefile, tests, README each)
│   ├── frontend/
│   ├── analytics/               # Go analytics service (analytics profile)
│   ├── canary/                  # Rust synthetic-journey canary (synthetic profile)
│   └── reports/                 # Kotlin/Spring Boot reports service (reports profile)
│
├── .devcontainer/               # Devcontainer / GitHub Codespaces setup
├── .env.example                 # Compose overrides template (see local-setup)
├── .mise.toml                   # Pinned toolchain versions (source of truth)
├── AGENTS.md                    # Canonical instructions for AI coding agents
├── LICENSE                      # MIT
├── Makefile                     # Single operational entry point (make help)
├── README.md                    # This file
└── SECURITY.md                  # How to report a vulnerability
```

## Quick Start

### Requirements

Verify everything with one command -- it checks tools, versions, the Docker
daemon, and resources, and tells you how to fix what is missing:

```shell
make doctor
```

Toolchain versions are pinned in `.mise.toml`
([mise](https://mise.jdx.dev) installs them; any other way to provide the
same versions works too). Minimum: Docker with Compose v2, GNU Make, 2 GB
free RAM, 3 GB free disk.

### Running the Project

- Start everything

  ```shell
  make up
  ```

- Seed initial data (could be done multiple times)

  ```shell
  make seed
  ```

- Seed 90 days of analytics history (RFC-0001 Phase 5 D5; needs the
  `analytics` profile up first)

  ```shell
  make up-full
  make seed-history
  ```

- Workshop mode: all profiles at `DEMO_TIME_SCALE=24` (one profile-day
  compresses to 1 wall-clock hour, RFC-0001 D5) -- seed at the same scale
  or loadgen's scale guard refuses to start against the mismatched seed
  marker (no marker yet or analytics absent: it continues)

  ```shell
  make up-workshop
  DEMO_TIME_SCALE=24 SEED_DAYS=3 make seed-history
  ```

  See [Exercise 05](docs/exercises/05-find-the-seeded-anomalies.md) for
  what to do with it.

After successful startup, all services will be available at the following URLs:

| Service               | URL                         | Credentials | Description                                                                               |
|-----------------------|-----------------------------|-------------|-------------------------------------------------------------------------------------------|
| **Frontend**          | http://localhost:8080       | -           | React application with CRUD interface for managing items                                  |
| **API**               | http://localhost:8000       | -           | FastAPI REST API server                                                                   |
| **API (gRPC)**        | localhost:50051             | -           | ItemService (ListItems, GetItemStats, WatchItemEvents) -- backend serves, analytics dials |
| **API Documentation** | http://localhost:8000/docs  | -           | Swagger UI with interactive API documentation                                             |
| **ReDoc**             | http://localhost:8000/redoc | -           | Alternative API documentation in ReDoc format                                             |
| **Grafana**           | http://localhost:3000       | admin/admin | Dashboards for metrics and logs visualization                                             |
| **Prometheus**        | http://localhost:9090       | -           | UI for viewing and querying metrics                                                       |
| **Loki**              | http://localhost:3100       | -           | API for accessing logs                                                                    |
| **Postgres Exporter** | http://localhost:9187       | -           | PostgreSQL metrics in Prometheus format                                                   |
| **cAdvisor**          | http://localhost:8081       | -           | Container and resource metrics                                                            |
| **Alertmanager**      | http://localhost:9093       | -           | Alert routing, grouping, silencing, inhibition                                            |
| **Mailpit**           | http://localhost:8025       | -           | Visible alert receiver: SMTP sink + web UI for notifications                              |
| **Canary**            | http://localhost:8085       | -           | Synthetic-journey canary (`synthetic` profile)                                            |
| **Analytics API**     | http://localhost:8082       | -           | Event ingestion + read API (items, stats, seed-marker) (`analytics` profile)              |
| **Reports API**       | http://localhost:8083       | -           | Kotlin/Spring Boot reports service, skeleton so far (`reports` profile)                   |

For the full command list run `make help`.

## Documentation

### Using and operating

- [Prerequisites](docs/prerequisites.md) -- required knowledge and learning resources
- [Local setup](docs/local-setup.md) -- development environment, dependencies, Docker
- [Architecture](docs/architecture.md) -- system components and how they connect
- [Observability](docs/observability.md) -- metrics, logs, dashboards, SLOs
- [Database and data](docs/data.md) -- schema, migrations, seeding
- [Running tests](docs/running-tests.md) -- unit and integration tests
- [Troubleshooting](docs/troubleshooting.md) -- common issues and solutions

### Process and decisions

- [Engineering principles](docs/engineering-principles.md) -- how and why we
  build this way; RFC/ADR lifecycle, change workflow, testing philosophy
- [RFCs](docs/rfc/) -- design documents; start with
  [RFC-0001](docs/rfc/0001-polyglot-platform.md)
- [ADRs](docs/adr/) -- one durable decision per file, extracted from the
  RFCs; immutable once accepted
- [CI/CD architecture](docs/ci.md) -- pipeline topology, gates, releases,
  branch protection
- [Contributing](docs/contributing.md) -- practical guide: commits, hooks,
  linting, tests
- [AGENTS.md](AGENTS.md) -- canonical instructions for AI coding agents
- [Security policy](SECURITY.md) -- how to report a vulnerability, scope
  notes for the intentional local-stack defaults

### Learning

- [Exercises](docs/exercises/00-baseline.md) -- structured assignments with
  difficulty levels; new exercise sets arrive with each platform phase

## License

This project is intended for educational purposes and licensed under the MIT
License -- see the [LICENSE](LICENSE) file for details.
