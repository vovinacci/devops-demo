# Local Setup

## Requirements

- **macOS or Linux** -- Windows is not supported.
- **Docker Engine >= 24** with **Docker Compose v2** (the `docker compose` CLI plugin). Standalone `docker-compose` v1 is not supported. On macOS, [OrbStack](https://orbstack.dev/) is recommended.
- **Make** -- macOS: `xcode-select --install`; Linux: `sudo apt-get install build-essential` (Debian/Ubuntu) or `sudo yum groupinstall "Development Tools"` (RHEL/CentOS).
- **mise** -- toolchain manager that provides the pinned language toolchains (Python, Node, Rust, Go, buf, k6, and -- for the reports service -- a Temurin JDK and Gradle) from `.mise.toml`. Install: https://mise.jdx.dev. Shell activation: add `eval "$(mise activate zsh)"` to `~/.zshrc`.

Toolchain versions are pinned in `.mise.toml`. Never rely on system-installed versions; use `mise install` to pull the pinned toolchain. The reports service also ships a committed Gradle wrapper (`services/reports/gradlew`), so its build uses the wrapper's pinned Gradle regardless -- the mise `gradle` pin is for running Gradle directly and is drift-checked against the wrapper.

## First Steps After Cloning

```shell
mise install                  # pull pinned toolchains from .mise.toml
make doctor                   # verify tools, versions, Docker daemon, RAM/disk
```

`make doctor` is the authoritative environment check. Fix any errors it reports before continuing.

## Backend Setup

```shell
make venv-install             # create .venv in services/backend, install all deps
```

This is idempotent. The virtualenv lands at `services/backend/.venv`.

## Frontend Setup

```shell
cd services/frontend
npm install
```

## Environment Variables

DB credentials default to `app / app / appdb`. To override, copy the template and edit:

```shell
cp .env.example .env
```

`.env` is gitignored. See `.env.example` at the repo root for available variables.

When running services via Docker Compose, variables are injected automatically from `deploy/compose/docker-compose.yml`.

For local (non-Docker) backend development, export manually:

```shell
export DATABASE_URL="postgresql+asyncpg://app:app@localhost:5432/appdb"
export ALEMBIC_DATABASE_URL="postgresql+psycopg://app:app@localhost:5432/appdb"
```

## Running Services

### Full stack via Docker Compose

The compose file lives at `deploy/compose/docker-compose.yml`. Raw `docker compose` from the repo root does not work; use make targets or the explicit `-f` form:

```shell
make up                       # start all services (build if needed)
make down                     # stop and remove containers
make logs                     # tail logs
make seed                     # seed the database
make seed-reset               # drop and re-seed
make clean                    # stop + remove volumes
```

Equivalent raw form when no make target exists:

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . <cmd>
```

Services started: PostgreSQL, FastAPI backend, React frontend, Prometheus, Grafana, Loki, Grafana Alloy, Postgres Exporter, cAdvisor.

### Optional profiles

The core stack above is profile-less. Additive profiles opt in more services: `analytics` (Go analytics + its Postgres), `synthetic` (Rust canary + blackbox_exporter), `reports` (Kotlin/Spring Boot reports + its Postgres), and `load` (k6 loadgen). `make up-full` and `make up-workshop` start all of them; combine granularly on a constrained laptop, e.g. `docker compose ... --profile analytics --profile load up` to skip the JVM.

The `reports` profile is the largest RAM increment of the set -- it is a JVM service (heap plus its own Postgres), which is the point of the D2 "JVM showcase" exhibit. On a memory-constrained machine, leave `reports` out of the profile list unless you are exercising it.

### Backend only (local process, DB in Docker)

```shell
# start only the database
docker compose -f deploy/compose/docker-compose.yml --project-directory . up -d db

source services/backend/.venv/bin/activate
cd services/backend
alembic -c alembic.ini upgrade head
uvicorn app.main:app --reload --port 8000
```

API: http://localhost:8000 | Swagger: http://localhost:8000/docs

### Frontend only (local process)

```shell
cd services/frontend
npm run dev
```

Frontend: http://localhost:5173. Proxies API requests to `http://localhost:8000` (configured in `vite.config.js`).

## Pre-commit Hooks

Git hooks are managed via **prek** (pre-commit-compatible single binary; classic `pre-commit` is the fallback). The hook list lives in `.pre-commit-config.yaml` -- do not enumerate hooks here.

```shell
make pre-commit-install       # install hooks into .git/hooks/
make pre-commit-run           # run hooks against all files manually
```

## Testing

```shell
make test-backend             # starts db container if absent, runs backend tests
make test-frontend            # runs frontend tests
make test-reports             # reports tests (Testcontainers Postgres -- needs Docker)
make ci                       # full CI suite (what the pipeline runs)
```

See [docs/running-tests.md](./running-tests.md) for details.

## Troubleshooting

See [docs/troubleshooting.md](./troubleshooting.md). For CI/CD, see [docs/ci.md](./ci.md). For linting and formatting, see [docs/contributing.md](./contributing.md).
