# Exercise: Operate the baseline platform (Phase 0)

RFC-0000 is the baseline retrospective: a FastAPI + Postgres items app that
shipped with a *full observability stack from day one* (B2) -- Prometheus
SLO rules, a provisioned Grafana dashboard, and container/DB exporters, all
as code, none of it added later as an afterthought. This exercise is the
one every other exercise builds on: before you break a layer (01), trace a
gRPC call (02), or hunt a seeded anomaly (05), you prove to yourself that
"done" at baseline already means *operable* -- `/metrics`, SLOs, and a
dashboard -- not just "the endpoint returns 200".

## Objective

Bring up the core stack, tour the whitebox observability that exists at
baseline (the backend's health/metrics endpoints, the SLO recording rules,
the provisioned dashboard), then make one cross-cutting product change --
add a `description` field to items -- and *operate* it: migrate, confirm the
change is live in the running system, restart, and watch that every baseline
signal still holds. The lesson: shipping a field is not done until you can
see it working in the operated platform.

## Prerequisites

```shell
make up     # core: backend + frontend + postgres + full observability stack
make seed   # 20 items, so the signals have something to move
```

No optional profiles (`synthetic`, `analytics`, `reports`, `load`) at this
phase -- those arrive in 01-06. Confirm the core is up:

```shell
curl -sS http://localhost:8000/healthz            # {"status":"ok"}
curl -sS http://localhost:9090/-/ready            # Prometheus ready
curl -sS http://localhost:3000/api/health | jq .  # Grafana ok
```

## Part 1 -- the baseline is already operated

The backend implements the whitebox subset of the uniform service contract
(RFC-0001 D6, ADR-0004) that every later service also ships. Hit each
endpoint on `:8000` and read what it is actually telling you:

| Endpoint            | Purpose                                   | Proves                                        |
| ------------------- | ----------------------------------------- | --------------------------------------------- |
| `/healthz`          | Liveness -- process up, no deps checked   | The process is serving                        |
| `/readyz`           | Readiness -- runs `SELECT 1` on Postgres  | The dependency (DB) is reachable; 503 if not  |
| `/health`           | Compatibility alias for `/healthz`        | Old dashboards/scripts still resolve          |
| `/metrics`          | Prometheus exposition format              | RED metrics for HTTP, DB, and gRPC            |
| `/metrics/frontend` | POST sink for browser web-vitals          | Real-user LCP/INP/CLS reach the same TSDB     |

```shell
curl -sS http://localhost:8000/readyz | jq .      # note "database":"connected"
curl -sS http://localhost:8000/metrics | grep -E '^http_requests_total|^db_queries_total'
```

Note the distinction: `/readyz` fails (503) when Postgres is gone but
`/healthz` does not -- liveness and readiness answer different questions,
and the platform depends on both being separate.

Now the SLOs. `observability/prometheus_slo_rules.yml` defines the recording
rules that turn raw counters into an error budget over the three critical
endpoints (`/health`, `/items`, `/items/{item_id}`). Query them live in the
Prometheus UI (<http://localhost:9090>):

```promql
slo:availability:ratio7d            # non-5xx / total, target 0.9995
slo:latency:p95_7d                  # p95 of http_request_latency_seconds
slo:error_rate:actual_5xx_ratio7d   # 5xx / total, target 0.001
slo:availability:error_budget_burn7d # > 0 means the SLO is being violated
```

These exist at baseline -- error budgets are introduced as a normal part of
a service (B2), not an advanced add-on.

Finally the dashboard. Open Grafana (<http://localhost:3000>, admin/admin)
-> **DevOps Demo**. It is provisioned from
`observability/grafana/dashboards/devops-demo-dashboard.json` (a file, not a
UI artifact). Locate the panels that render the signals above:

- **SLO: Availability (99.95% target)**, **SLO: Latency p95 (< 200ms
  target)**, **SLO: Error Rate 5xx (< 0.1% target)**, **SLO: Error Budget
  Burn Rate (7 days)** -- the recording rules from above, visualized.
- **API RPS by endpoint** and **API p95 latency (sec) by endpoint** -- the
  RED view of `/items` traffic.
- **DB queries rate by operation** and **DB latency p95 (sec) by
  operation** -- Postgres work behind each request.
- **API Health Checks** -- the liveness/readiness signal over time.

That is the baseline definition of "done": a change is only shipped when it
is visible here.

## Part 2 -- operate a change end to end

The item model is deliberately minimal: `id` and `name` only
(`services/backend/app/models.py`). Add a `description` field and carry it
across every layer it touches. State the shape; do not paste every line.

1. **Model** (`app/models.py`): add to `Item`
   `description: Mapped[str | None] = mapped_column(String(500), nullable=True)`.
2. **Migration** (`app/../alembic/versions/`): author a revision by hand
   modeled on `20251026_0001_init_items.py` (the repo's migrations are
   clean, hand-written, not autogenerate dumps). Its `upgrade()` is one
   `op.add_column("items", sa.Column("description", sa.String(length=500), nullable=True))`;
   `downgrade()` drops it. Set `down_revision = "20251026_0001"`.
3. **Schemas** (`app/schemas.py`): add `description: str | None = None` to
   `ItemCreate` and `ItemOut` (use `Field(None, max_length=500)` to enforce
   the column limit at the API edge).
4. **CRUD** (`app/crud.py`): pass it through in `create_item` --
   `Item(name=data.name, description=data.description)`.
5. **API** (`app/main.py`): the `/items` GET and POST build `ItemOut(...)`
   *by hand*. Add `description=i.description` to both -- miss this and the
   column exists in Postgres but the API silently never returns it (the
   operability trap this step exists to teach).
6. **Frontend** (`services/frontend/src/App.jsx`): add a description input to
   the form, include it in the POST body, and render it in the item list.

Now *operate* the change. The backend image bakes its source in (no bind
mount) and applies migrations on boot via `alembic upgrade head`, so a
rebuild is required for the new code and migration to take effect:

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . \
  up -d --build api frontend
```

Confirm the migration actually applied -- do not trust that it "should
have":

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . \
  exec api python -m alembic -c /app/alembic.ini current   # new head, not 20251026_0001
```

Prove the field is live end to end, through the running system:

```shell
curl -sS -X POST http://localhost:8000/items \
  -H 'content-type: application/json' \
  -d '{"name":"phase0-demo","description":"operable, not just 200"}' | jq .
curl -sS http://localhost:8000/items | jq '.[] | select(.name=="phase0-demo")'
```

Then confirm the platform's own signals still hold. In the **DevOps Demo**
dashboard, watch **API RPS by endpoint** register the new `POST /items`
traffic, **DB queries rate by operation** show the `insert`, and the three
SLO panels stay green through the deploy. Restart once more and re-check
`/readyz` and `/metrics` -- the counters (`http_requests_total{endpoint="/items"}`)
carry on incrementing across the restart.

## Expected observations

- `/readyz` returns 503 the instant Postgres is unreachable while `/healthz`
  stays 200 -- readiness gates traffic on dependencies, liveness does not.
- The SLO recording rules are *derived*: `slo:availability:ratio7d` is
  non-5xx / total over the critical endpoints, so a burst of 4xx (a bad
  request) does *not* burn the availability budget, but a 5xx does. Adding a
  field the right way moves RPS and DB-insert panels without touching the
  error-budget-burn panels.
- The migration is the load-bearing step: the API can be rebuilt with the
  new schema code, but if `alembic upgrade head` did not run, `POST /items`
  fails against a table that has no `description` column. "Operable" means
  the schema and the code moved together.
- Skipping step 5 (the hand-built `ItemOut`) produces a green build, a
  passing migration, a persisted column -- and an API that never surfaces
  the field. Nothing is red; the platform is simply not doing what you
  think. This is exactly the failure the later monitoring layers exist to
  catch.

## Cleanup

```shell
make down   # stops all profiles; core included
```

To also drop the seeded data and the `description` column's data, remove the
volume:

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . \
  down -v
```

## Discussion questions

1. `slo:availability:ratio7d` counts `status!~"5.."` as success, so a flood
   of `409 Conflict` (duplicate item name) never burns the availability
   budget. Argue both sides: when is treating 4xx as "the service working
   correctly" the right call, and what class of real user-facing breakage
   does it hide?
2. The backend serves `/healthz` (liveness) and `/readyz` (readiness) as
   separate endpoints, and the compose healthcheck gates only on liveness.
   What would break if the container healthcheck used `/readyz` instead, on
   a stack where Postgres restarts independently of the API?
3. You added `description`, migrated, and the dashboard stayed green -- yet a
   green dashboard did not, by itself, prove the field reaches the API
   (step 5). What single panel or SLO rule *could* have caught that gap
   automatically, and why does the baseline deliberately not have it? (The
   three-layer monitoring of exercise 01 is one answer -- sketch which layer
   applies.)
