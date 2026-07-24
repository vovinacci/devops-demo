# Reports UI

A small static single-page app over the reports API, served by **Caddy**
(RFC-0002, ADR-0013). It submits an items-summary report, watches the job reach
`SUCCEEDED`, downloads the artifact, and lists recent jobs from real server
state. It is deliberately the opposite of the React `frontend` in every
implementation choice: vanilla HTML/CSS/JS with **no build step** (RFC-0002 D3),
and a single-binary server that natively exposes Prometheus metrics -- so the
repo carries two static-frontend exhibits whose contrast is the lesson.

No new runtime is introduced: `reports-ui` is an integration exhibit (an
API-consumer SPA plus a reverse proxy), not a sixth language.

## What it does

1. Submit form -> `POST /api/reports {type, format}` -> Caddy proxies to
   `reports:8083/reports` -> `202` with the job id.
2. Poll `GET /api/reports/{id}` until `SUCCEEDED` | `FAILED`; a status pill
   tracks the transition.
3. On success, offer the `download` link (`/api/reports/{id}/download`).
4. Recent-jobs table loads from `GET /api/reports?limit=20` (newest first,
   RFC-0002 D5), each row linking to its download when ready.

All fetches go to the **same origin** under `/api/*` -- no CORS, no hardcoded
host (`site/app.js`).

## The Caddyfile

The whole service is a tiny `Caddyfile`:

- `handle /healthz` and `handle /readyz` -- `respond 200`. Readiness for a
  stateless static server equals liveness (there is no backing store to be
  un-ready against); the reports API has its own `/readyz` reflecting Postgres.
- `handle /metrics { metrics }` -- Caddy's **native** Prometheus metrics on the
  `:8084` service listener. Caddy serves these on its admin API by default
  (`localhost:2019`), NOT the site listener, so this explicit handler is
  required; the global `metrics` option enables the per-server instrumentation
  (`caddy_http_requests_total`, request-duration histograms). The admin API is
  never published in compose, so it stays private.
- `handle_path /api/* { reverse_proxy reports:8083 }` -- same-origin proxy.
  `handle_path` **strips** the matched `/api` prefix, so the reports service
  sees `/reports/...`, not `/api/reports/...`. A bare `reverse_proxy /api/*`
  would preserve the URI and 404 -- getting this wrong is a silent failure, so
  the directive is named explicitly (RFC-0002 D4).
- `handle { root * /srv; file_server }` -- the static SPA.
- A global `log { output stdout, format json }` -- structured access logs that
  Alloy tails into Loki, like every other service (RFC-0001 D6/D11).

## D6 uniform contract

`reports-ui` ships the full RFC-0001 D6 contract with the static server alone
(the payoff of choosing Caddy over nginx, ADR-0013):

- `/healthz` -- liveness (`200` once the process is up).
- `/readyz` -- readiness = liveness for a stateless static server (`200`).
- `/metrics` -- Caddy's native Prometheus exposition on `:8084`.

Prometheus scrapes the public `:8084/metrics` under the `reports-ui` job; a
provisioned Grafana dashboard (`Reports UI (Caddy)`) reads it.

## Graceful degradation (RFC-0001 D10, Hard rule 9)

`reports` is an opt-in profile, and `reports-ui` does **not** hard-depend on it.
With `reports` absent the page still serves its static assets and its own D6
endpoints; the `/api/*` calls fail (503, Caddy's active health check having
marked the upstream down), and `site/app.js` surfaces a friendly
"reports service unavailable" banner and an empty jobs table instead of a blank
crash. The UI recovers on its next poll once `reports` is back.

## Run it

```shell
# whole stack incl. reports + reports-ui (and backend/db/analytics for content)
make up-full            # from the repo root; UI at http://localhost:8084

# just this service (proxied /api calls fail until reports is also up)
make -C services/reports-ui run
```

`http://localhost:8084` serves the page; `:8084/metrics` the Prometheus text.
The admin API (`:2019`) is intentionally not exposed.

## Development

```shell
make build   # docker build (caddy validate runs in the build)
make test    # caddy validate + a container smoke check (/healthz, /metrics)
make lint    # caddy fmt (no-op check) + caddy validate + ASCII-only assets
make format  # caddy fmt --overwrite
make run     # compose up the reports-ui profile
```

There is no host toolchain to install (RFC-0002 D3: no build step): every
`caddy` invocation runs through the digest-pinned `caddy:2-alpine` image, the
same image the Dockerfile builds from.

## Docker

Single-stage, digest-pinned `caddy:2-alpine` (repo convention). `caddy validate`
runs in the build so a malformed config never reaches a running container. The
container runs as a non-root user (`caddy`, uid 1000) with `/config` and `/data`
chowned to it; `:8084` is unprivileged. A `HEALTHCHECK` hits `/healthz` via the
image's own `wget`.
