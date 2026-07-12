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
│   ├── rfc/                     # Request for Comments -- design documents
│   └── exercises/               # Student exercises
│
├── observability/               # Observability configuration
│   ├── prometheus.yml           # Prometheus configuration
│   ├── prometheus_slo_rules.yml # SLO rules
│   ├── grafana/                 # Dashboards (JSON) + provisioning
│   ├── loki/                    # Loki configuration
│   └── alloy/                   # Grafana Alloy configuration
│
├── scripts/                     # Doctor, toolchain drift gate
├── services/                    # Self-contained services (own Dockerfile,
│   ├── backend/                 #   tests, README each)
│   └── frontend/
│
├── AGENTS.md                    # Canonical instructions for AI coding agents
├── Makefile                     # Single operational entry point (make help)
└── README.md                    # This file
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

After successful startup, all services will be available at the following URLs:

| Service               | URL                         | Credentials | Description                                              |
|-----------------------|-----------------------------|-------------|----------------------------------------------------------|
| **Frontend**          | http://localhost:8080       | -           | React application with CRUD interface for managing items |
| **API**               | http://localhost:8000       | -           | FastAPI REST API server                                  |
| **API Documentation** | http://localhost:8000/docs  | -           | Swagger UI with interactive API documentation            |
| **ReDoc**             | http://localhost:8000/redoc | -           | Alternative API documentation in ReDoc format            |
| **Grafana**           | http://localhost:3000       | admin/admin | Dashboards for metrics and logs visualization            |
| **Prometheus**        | http://localhost:9090       | -           | UI for viewing and querying metrics                      |
| **Loki**              | http://localhost:3100       | -           | API for accessing logs                                   |
| **Postgres Exporter** | http://localhost:9187       | -           | PostgreSQL metrics in Prometheus format                  |
| **cAdvisor**          | http://localhost:8081       | -           | Container and resource metrics                           |

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
