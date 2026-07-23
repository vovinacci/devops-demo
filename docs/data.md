# Database and data seeding

## Schema

PostgreSQL, single table:

- **`items`**
  - `id` (SERIAL PRIMARY KEY)
  - `name` (VARCHAR(100) UNIQUE NOT NULL)

Credentials default to `app`/`app`/`appdb`, overridable via `.env`
(see `.env.example`).

## Migrations

Managed by Alembic; applied automatically when the api service starts.

- Migration sources: `services/backend/alembic/versions/`
- Create a new migration (backend venv, db running):

  ```shell
  cd services/backend
  alembic revision --autogenerate -m "change description"
  ```

- Apply manually inside the running container:

  ```shell
  docker compose -f deploy/compose/docker-compose.yml --project-directory . \
    exec api python -m alembic -c /app/alembic.ini upgrade head
  ```

## Seed data

```shell
make seed        # add 20 test items
make seed-reset  # clear all data, then reseed
make seed-dry    # show what would be created, no writes
```

Seed script flags (`python -m app.seed`): `--count N`, `--only-reset`,
`--dry-run`.

## Analytics and reports databases

The `analytics` and `reports` services each own a separate Postgres
instance (`postgres-analytics`, `postgres-reports`), not a schema on the
backend `db` -- service-owns-its-store, visible in the compose topology
(RFC-0001 D1/D2, ADR-0005). The analytics schema and its hand-rolled
embedded-SQL migrations are documented in
[`services/analytics/README.md`](../services/analytics/README.md); the
reports schema follows.

## Reports database (`postgres-reports`)

The reports service (RFC-0001 D2 Phase 6) stores report-job metadata in its
own Postgres instance and writes the generated artifacts (XLSX/PDF/CSV) to a
named volume -- never as BLOBs in the database (RFC-0001 D2, deliberately
avoided). Single table:

- **`report_jobs`**
  - `id` (text PRIMARY KEY) -- job UUID
  - `type` (text) -- report type, e.g. `items-summary`
  - `format` (text) -- `xlsx` | `pdf` | `csv` (CHECK-constrained)
  - `status` (text) -- `PENDING` | `RUNNING` | `SUCCEEDED` | `FAILED`
    (CHECK-constrained) -- the job state machine
  - `params` (jsonb) -- the request parameters, for audit and dashboard slicing
  - `created_at`, `started_at`, `finished_at` (timestamptz) -- lifecycle timestamps
  - `error` (text) -- failure detail when `status = FAILED`
  - `artifact_path` (text), `artifact_bytes` (bigint) -- path on the artifact
    volume and its size, populated when `status = SUCCEEDED`

Credentials default to `reports`/`reports`/`reports`, overridable via the
`REPORTS_POSTGRES_*` compose variables (same demo-grade posture as every
other credential in this repo).

### Migrations (Flyway)

Managed by **Flyway** (`spring-boot-starter` idiom, RFC-0001 D2: "the
heavyweight framework is the lesson"), applied automatically at reports
startup. This is the deliberate contrast with the analytics service's
hand-rolled runner -- same job, two honest approaches.

- Migration sources: `services/reports/src/main/resources/db/migration/`
  (`V1__report_jobs.sql`, ...).
- Because Flyway runs at startup and needs Postgres reachable *then*, the
  reports app -- unlike the PR-1 skeleton -- no longer boots with the DB
  absent. This is fine and gated: compose orders `reports` after
  `postgres-reports: service_healthy`, and `spring.flyway.connect-retries`
  covers a DB that is merely slow to accept connections. The
  readiness/liveness split still holds exactly: liveness (`/healthz`) never
  depends on the DB, and a running app that later loses Postgres keeps
  `/healthz` at 200 while `/readyz` reports 503
  (`services/reports/README.md`).
