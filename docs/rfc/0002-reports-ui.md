# RFC-0002: Reports UI -- a static SPA over the reports API

- **Status:** Accepted
- **Author:** vovin
- **Created:** 2026-07-24
- **Relationship to RFC-0001:** sibling. RFC-0001 is **immutable** -- its
  five-runtime polyglot story, its decisions, and its scope freeze stand
  untouched. This RFC adds one integration exhibit on top of that platform and
  reasons about it in writing, like everything else here.

## 1. Summary

RFC-0001 Phase 6 shipped the Kotlin/Spring Boot `reports` service: an async
job engine (`POST /reports` -> `GET /reports/{id}` -> download) generating
items-summary reports in XLSX/PDF/CSV. It is **API-only** -- there is no way
to drive it from a browser, and its recent-jobs history is visible only
through raw `curl`.

This RFC adds a small, separate **`reports-ui`** service: a static
single-page app served by **Caddy**, which also reverse-proxies the reports
API on the same origin. It is deliberately the opposite of the React
`frontend` in every implementation choice -- vanilla HTML/CSS/JS with no build
step, a single-binary server that natively exposes Prometheus metrics -- so
the repo carries *two* static-frontend exhibits whose contrast is the lesson.
No new runtime is introduced; `reports-ui` is an **integration exhibit** (an
API-consumer SPA plus a reverse proxy), not a sixth language.

## 2. Motivation

- The reports engine is the JVM showcase, but a demo you can only exercise
  with `curl` is a screenshot, not a demo (RFC-0000's own lesson, restated).
  A student should be able to submit a report and watch it run.
- The existing React `frontend` proves the "SPA with a build toolchain +
  nginx" shape. It does **not** satisfy the D6 uniform service contract:
  its nginx serves static files and proxies `/api/`, but exposes no
  `/metrics`, `/healthz`, or `/readyz`. That gap is worth making visible and
  then *closing* in a second, contrasting static frontend.
- The reports service already persists every job in `report_jobs`. A jobs
  list is honest, shared server state waiting to be surfaced -- not something
  the UI should reconstruct client-side from what it happened to submit.

## 3. Goals

- A browser UI to submit an items-summary report (format `xlsx` | `pdf` |
  `csv`), watch its status transition, and download the artifact.
- A recent-jobs list backed by real server state, with per-job download
  links.
- A second static-frontend exhibit that **fully** satisfies the D6 uniform
  contract, in deliberate contrast to the React `frontend`.
- Zero new runtime, zero new build toolchain, minimal audit surface.

## 4. Non-goals (this RFC)

- Migrating the React `frontend` to Caddy or giving it `/metrics` (noted as a
  possible future ADR in Section 11, not adopted here).
- Auth, multi-user history, or report scheduling from the UI.
- New report types or any change to the reports engine beyond the one list
  endpoint in D5.
- A build step, framework, or package manager for the UI (D3 rejects these
  by design).

## 5. Decisions

Each decision is a candidate for extraction into a standalone ADR
(Section 11).

### D1. A separate `reports-ui` service

`reports-ui` is its own `services/reports-ui/` module with its own Dockerfile
and compose service, not a folder inside `reports` and not a route bolted
onto the React `frontend`.

- Rationale: it keeps the JVM `reports` service a pure API (its D6 story and
  its GC/heap exhibit stay uncluttered by a web asset pipeline), and it keeps
  the UI's runtime (a tiny static server) honestly separate from the business
  service's runtime. Service-owns-one-responsibility, the same principle as
  RFC-0001 D8.
- Rejected -- **serve the UI from the reports JVM** (Spring static resources):
  couples an unrelated asset-serving concern to the JVM showcase, inflates its
  image, and muddies the "this service exists to show GC under bursty
  allocation" narrative.
- Rejected -- **add a reports page to the React `frontend`**: drags the
  reports API behind the frontend's nginx and its build toolchain, and forfeits
  the chance to build the contrasting no-build exhibit this RFC is about.

### D2. Caddy as the static server and reverse proxy

The server is **Caddy** (`caddy:2-alpine`, digest-pinned per repo
convention), not nginx.

- Rationale: a tiny Caddyfile does everything this service needs -- a static
  `file_server`, a `reverse_proxy /api/* -> reports:8083`, a
  `respond /healthz 200`, and a **native `metrics` endpoint** (Caddy's admin/
  telemetry exposes Prometheus metrics out of the box). That last point is the
  crux: it lets `reports-ui` satisfy the full D6 contract with no sidecar and
  no hand-rolled exporter (see D6 below).
- Conscious tradeoff, stated plainly: the repo will then run **two** static
  servers -- nginx for `frontend`, Caddy for `reports-ui`. This is deliberate,
  not an oversight: the two sit on adjacent dashboards as a contrast, the same
  way the Rust canary sits next to the JVM (RFC-0001 Section 7). A future ADR
  could migrate `frontend` to Caddy for uniformity; this RFC does not (that
  would edit a service RFC-0001 owns).
- Rejected -- **nginx** (matching `frontend`): nginx has no native Prometheus
  exporter, so D6's `/metrics` would need a sidecar (nginx-prometheus-exporter)
  or a Lua/stub-status hack -- more moving parts to teach the *opposite* of the
  point, which is that a single small binary can meet the whole contract.
- Rejected -- **Node/Express or a Go static server**: either adds a build/
  dependency surface (Node) or a bespoke binary to maintain (Go) for what an
  off-the-shelf server does declaratively in a handful of Caddyfile lines.

### D3. Vanilla HTML + CSS + JS, no build step

The UI is hand-written `index.html` + `styles.css` + `app.js`, served raw.
No npm, no Vite, no bundler, no framework.

- Rationale: lean and legible -- a reader sees exactly what the browser runs,
  with nothing generated. It is a **deliberate contrast** to the React
  `frontend` (which exists to show the build-toolchain-plus-nginx shape), and
  it drives the audit surface to effectively zero: no `package-lock.json`, so
  no `npm audit` gate and no transitive-dependency CVE stream for a UI this
  small.
- Fitness: the whole UI is a form (type + format), a polled status view, and a
  jobs table. `fetch` + a little DOM code covers it without a framework earning
  its weight.
- Rejected -- **React/Vite** (matching `frontend`): a build toolchain and a
  dependency tree for three widgets; the `frontend` already carries that
  exhibit, and duplicating it here would waste the contrast.
- Rejected -- **a CSS/JS CDN**: the repo runs offline by design (RFC-0000 B2);
  external CDNs break that and add an uncontrolled supply-chain edge.

### D4. Same-origin reverse proxy, no CORS

The browser talks only to `reports-ui`'s own origin; Caddy reverse-proxies
`/api/*` to `reports:8083`, stripping the `/api` prefix.

- Rationale: same-origin means **no CORS** to configure on the reports service
  and no preflight round-trips -- the reports API stays a clean server-to-
  server contract with no browser-specific concessions. This mirrors the
  `frontend` -> nginx -> `api` shape already in the repo (nginx proxies
  `/api/`), so the pattern is consistent across both frontends.
- Rejected -- **direct browser -> `reports:8083` with CORS headers**: pushes
  browser concerns into the JVM service, needs an allow-origin policy to
  maintain, and exposes the reports port to the host purely for the browser.

### D5. Server-backed jobs list via `GET /reports?limit=N`

The recent-jobs list is real shared server state. This PR adds a list endpoint
to the reports service:

- `GET /reports?limit=N` returns recent jobs **newest first** (metadata only,
  never artifacts): `id`, `type`, `format`, `status`, `createdAt`,
  `finishedAt`, `artifactBytes`, and a `download` link when an artifact exists.
- `limit` is **clamped** server-side (default 20, min 1, max 100) so a client
  cannot request an unbounded scan. Ordering is index-backed
  (`report_jobs_created_at_idx`, `created_at DESC`).
- It routes cleanly beside the existing `GET /reports/{id}`: the bare
  `/reports` path hits the list, `/reports/{id}` hits the status resource.
- Rejected -- **client-only history** (the UI remembers what it POSTed in
  `localStorage`): dishonest and per-browser; the `report_jobs` table already
  is the shared truth, and a job created by the canary, loadgen, or another tab
  would be invisible.
- Rejected -- **a new bespoke `/jobs` resource**: `report_jobs` is the
  reports collection; the list of it belongs at the collection URL
  (`GET /reports`), the standard REST shape, distinct from the item URL
  (`GET /reports/{id}`).

### D6. The D6 uniform contract, satisfied via Caddy's native metrics

`reports-ui` ships the full RFC-0001 D6 contract, and does so with the static
server alone:

- `/healthz` -- `respond /healthz 200` in the Caddyfile (liveness: the static
  server is up).
- `/readyz` -- readiness for a stateless static server is liveness; it answers
  `200` the same way (documented as such -- there is no backing store to be
  un-ready against; the reports API has its own `/readyz`).
- `/metrics` -- Caddy's **native** Prometheus metrics, exposed on the service
  and scraped by Prometheus under a `reports-ui` job.
- Plus the rest of the D6 shape: a multi-stage-free but digest-pinned
  Dockerfile, the standard Make targets, structured logs to stdout (Caddy's
  JSON log), a provisioned Grafana panel/dashboard, and a CI job (lint the
  Caddyfile + HTML/ASCII gates -- no dependency audit needed, D3).

This is the payoff of D2: because Caddy exports metrics natively, the
contrast with the React `frontend` is not "one frontend is monitored and one
is not" by accident -- it is a **deliberate** exhibit that the `frontend`'s
nginx does not meet D6 (no `/metrics`) while `reports-ui`'s Caddy does, with a
documented path to close that gap later.

### D7. A `reports-ui` compose profile on host port 8084

`reports-ui` runs under a dedicated **`reports-ui`** compose profile, on host
port **8084** (next after `reports`' 8083), and is wired into `make up-full`.

- Rationale: profiles are the runtime selector (RFC-0001 D10, ADR-0008);
  `reports-ui` is optional in the same way `reports` is, and it hard-depends on
  `reports` being up. `make up-full` brings up every profile, so the UI is
  reachable in the full-stack demo without a new bespoke target.
- Port 8084 continues the repo's host-port ladder (8080 frontend, 8081
  cAdvisor, 8082 analytics, 8083 reports, 8085 canary; 8084 was the open slot).
- Graceful degradation (RFC-0001 D10): with the `reports` profile absent, the
  UI still serves its static assets and its own D6 endpoints; API calls fail
  visibly in the browser rather than the container failing to start. The UI
  does not `depends_on: service_healthy` the JVM in a way that blocks its own
  boot.

## 6. Service design

```text
services/reports-ui/
├── Dockerfile          # FROM caddy:2-alpine (digest-pinned); COPY site + Caddyfile
├── Caddyfile           # file_server + reverse_proxy /api/* + /healthz + metrics
├── Makefile            # build / test / lint / run (D6 target set)
├── README.md           # what it is, the Caddy/D6 story, the frontend contrast
└── site/
    ├── index.html      # submit form + status view + recent-jobs table
    ├── styles.css      # small hand-written stylesheet
    └── app.js          # fetch(): POST /api/reports, poll GET /api/reports/{id},
                        #          load GET /api/reports?limit=N
```

Request flow in the browser:

1. Submit form -> `POST /api/reports {type, format}` -> `reports-ui` Caddy
   proxies to `reports:8083/reports` -> `202` with the job id.
2. Poll `GET /api/reports/{id}` until `SUCCEEDED` | `FAILED`; render status.
3. On success, offer the `download` link (`/api/reports/{id}/download`).
4. Recent-jobs table loads from `GET /api/reports?limit=20` (D5), newest
   first, each row linking to its download when ready.

The reports service changes are limited to the D5 list endpoint: a repository
`list(limit)`, a controller `GET /reports` returning a lean summary DTO with
the clamp, a Testcontainers test, and README API docs. No change to the job
engine, the renderers, or the schema.

## 7. Delivery plan

| PR | Deliverable |
| --- | ----------- |
| PR-1 (this) | This RFC + ADR-0013 (Caddy static-server-plus-proxy pattern) + the reports `GET /reports?limit=N` list endpoint (repo `list`, controller + clamp, Testcontainers test, reports README API docs) + README pointers/fixes |
| PR-2 | The `reports-ui` service itself: Dockerfile, Caddyfile, vanilla `site/`, Makefile, README, the `reports-ui` compose profile on 8084 wired into `make up-full`, Prometheus `reports-ui` scrape job, a Grafana panel, and its CI job |

Splitting the API addition (PR-1) from the service (PR-2) keeps each PR
independently reviewable: PR-1 is a self-contained, tested backend change
plus design; PR-2 is pure new-service scaffolding against a settled contract.

## 8. Acceptance criteria

- **PR-1:** `GET /reports?limit=N` returns recent jobs newest first, metadata
  only, with `limit` clamped to `[1, 100]` (default 20); a Testcontainers test
  covers ordering, the lean shape, the default, and the clamp; the reports
  service build, Trivy lockfile audit, Docker build, and the repo doc/ASCII
  gates are green; this RFC and ADR-0013 lint clean.
- **PR-2:** `make up-full` brings up `reports-ui` on `:8084`; the UI submits a
  report, shows it reaching `SUCCEEDED`, downloads the artifact, and lists
  recent jobs; `reports-ui` answers `/healthz`, `/readyz`, and `/metrics`, and
  Prometheus scrapes the `reports-ui` job; with `reports` absent the UI still
  serves its assets and its own D6 endpoints.

## 9. Risks and mitigations

| Risk | Mitigation |
| ---- | ---------- |
| Two static servers (nginx + Caddy) read as inconsistency | Framed as a deliberate contrast exhibit (D2); future migration path noted (Section 11) |
| A hand-rolled UI rots without a test harness | Scope is tiny and behavior is thin; the reports API it drives is covered by its own integration tests; PR-2 keeps the UI a legible single page |
| `reports-ui` up without `reports` shows a broken UI | Graceful degradation (D7): static assets and D6 endpoints still serve; API errors surface visibly in the browser, not as a container that fails to boot |
| Unbounded jobs list query | Server-side clamp on `limit` (D5), index-backed ordering |

## 10. Relationship to RFC-0001

- RFC-0001 remains **immutable and authoritative** for the platform. This RFC
  neither edits nor reinterprets any RFC-0001 decision.
- No new runtime: `reports-ui` runs JS-in-the-browser + Caddy, both already in
  the stack. RFC-0001's *five-runtime* count is intact; `reports-ui` is an
  integration exhibit, not a sixth service language.
- It reuses RFC-0001's machinery wholesale: the D6 uniform contract (D6 here),
  compose profiles and graceful degradation (D7; ADR-0008), digest-pinned base
  images and CI gates (D12/D13), and OpenTelemetry-friendly structured logs.

## 11. ADRs to extract

1. ADR-0013: Caddy as the static server and reverse proxy for API-consumer
   SPAs -- the `reports-ui` pattern, its native-`/metrics` D6 win, and the
   nginx-`frontend` contrast (D2, D6). *(Extracted in this PR.)*

Possible future ADR (not adopted here): migrate the React `frontend` from
nginx to Caddy for a single static-server story and uniform D6 coverage.
