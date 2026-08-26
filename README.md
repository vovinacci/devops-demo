# DevOps Demo

A teaching platform: a complete example of taking a service from code to an
operated system -- tested, containerized, monitored, documented. "Done" means
*operable*, not "the endpoint returns 200".

The *process* is part of the product: decisions live in RFCs and ADRs,
conventions in the engineering principles, and every change follows them.
The polyglot platform is **built**: five runtimes (Python, JS, Go, Rust,
Kotlin) with contract-first gRPC, shaped load generation, three-layer
monitoring, and a static reports UI -- delivered through
[RFC-0001](docs/rfc/0001-polyglot-platform.md) and
[RFC-0002](docs/rfc/0002-reports-ui.md) (with
[RFC-0000](docs/rfc/0000-baseline-retrospective.md) as the baseline).

It runs on **two deployment targets**, and both are supported:
[Docker Compose](deploy/compose/README.md) as the fast local path, and
[Kubernetes on Kind](deploy/k8s/README.md) with Helm, the Gateway API and a
Prometheus Operator stack -- delivered through
[RFC-0003](docs/rfc/0003-kubernetes-kind-platform.md). Neither replaces the
other: compose is the smallest way to run the whole platform, Kubernetes is
where you see what changes when the same system has to be scheduled,
discovered and routed. After it comes an authentication capstone (a future
RFC-0004) that rides on that platform.

## Structure

```text
devops-demo/
├── .github/                     # Workflows, PR/issue templates, Renovate config
├── deploy/                      # One directory per deployment target
│   ├── compose/                 # Docker Compose stack (README.md)
│   │   └── docker-compose.yml
│   └── k8s/                     # Kubernetes on Kind (README.md)
│       ├── charts/              # Helm library chart + per-service + umbrella
│       ├── kind/                # Cluster config and cluster-scoped prerequisites
│       ├── schemas/             # Vendored CRD schemas for the offline gate
│       └── scripts/             # validate.sh, kind-up/deploy/seed/down
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
├── scripts/                     # Doctor, toolchain drift gate, make target logic (ADR-0019)
├── services/                    # Self-contained services (own Dockerfile,
│   ├── backend/                 #   Makefile, tests, README each)
│   ├── frontend/
│   ├── analytics/               # Go analytics service (analytics profile)
│   ├── canary/                  # Rust synthetic-journey canary (synthetic profile)
│   ├── reports/                 # Kotlin/Spring Boot reports service (reports profile)
│   └── reports-ui/              # Caddy static SPA over the reports API (reports-ui profile)
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

Two things this repo does not install for you: **Docker** (with Compose v2)
and **[mise](https://mise.jdx.dev)**. Everything else -- Python, Node, Go,
Rust, buf, k6, the JDK, Gradle -- is pinned in `.mise.toml` and installed by
mise in one step:

```shell
curl https://mise.run | sh          # if you do not have it yet
mise install                        # installs every pinned version
eval "$(mise activate zsh)"         # add to your shell rc (bash: activate bash)
```

Using mise is not mandatory -- any other way to provide the same pinned
versions works, and `make doctor` checks versions, not how you got them.
It is simply the shortest path, and the one the pins are written for.

Then verify everything with one command -- it checks tools, versions, the
Docker daemon, and resources, names anything missing, and tells you how to
fix it:

```shell
make doctor
```

Minimum for the compose stack: Docker Engine >= 24 with Compose v2, GNU Make, 2 GB free
RAM, 3 GB free disk. The Kind cluster needs substantially more -- it runs a
Kubernetes control plane, three nodes and an observability stack beside the
services ([`deploy/k8s/README.md`](deploy/k8s/README.md) has the detail).

### Running the Project

Two deployment targets, equally supported. Each has its own guide -- these are
the entry points, not a summary of what is there.

**Docker Compose** -- the fast local path, and what the exercises assume:

```shell
make up            # the core stack
make up-full       # every optional profile
make seed          # 20 items
make down
```

Service URLs, profiles, seeding and workshop mode:
**[`deploy/compose/README.md`](deploy/compose/README.md)**.

**Kubernetes (Kind)** -- the same system, scheduled and routed:

```shell
make kind-up                     # cluster + Envoy Gateway + kube-prometheus-stack
make kind-deploy PROFILE=full    # build, load into Kind, helm install
make kind-seed                   # items + analytics history
make kind-down
```

Routes, the Operator model and the Kind-isms:
**[`deploy/k8s/README.md`](deploy/k8s/README.md)**.

For the full command list run `make help`.

## Documentation

### Using and operating

- [Prerequisites](docs/prerequisites.md) -- required knowledge and learning resources
- [Docker Compose stack](deploy/compose/README.md) -- running the stack,
  profiles, service URLs, seeding
- [Kubernetes on Kind](deploy/k8s/README.md) -- charts, the cluster, the
  Gateway, the Operator, and the offline chart gate
- [Local setup](docs/local-setup.md) -- development environment, toolchains,
  running a single service outside compose
- [Architecture](docs/architecture.md) -- system components and how they connect
- [Observability](docs/observability.md) -- metrics, logs, dashboards, SLOs
- [Database and data](docs/data.md) -- schema, migrations, seeding
- [Running tests](docs/running-tests.md) -- unit and integration tests
- [Troubleshooting](docs/troubleshooting.md) -- common issues and solutions

### Process and decisions

- [Engineering principles](docs/engineering-principles.md) -- how and why we
  build this way; RFC/ADR lifecycle, change workflow, testing philosophy
- [RFCs](docs/rfc/) -- design documents; start with
  [RFC-0001](docs/rfc/0001-polyglot-platform.md) (the polyglot platform), then
  [RFC-0002](docs/rfc/0002-reports-ui.md) (the reports UI, a sibling RFC) and
  [RFC-0003](docs/rfc/0003-kubernetes-kind-platform.md) (Kubernetes on Kind, a
  second deployment target)
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
- [Course](docs/course.md) -- semester guide for teaching or self-studying the
  platform phase by phase: run current `main`, read how it grew via the tags

## License

This project is intended for educational purposes and licensed under the MIT
License -- see the [LICENSE](LICENSE) file for details.
