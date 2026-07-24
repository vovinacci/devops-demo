# ADR-0013: Caddy as the static server and reverse proxy for API-consumer SPAs

- Status: Accepted
- Date: 2026-07-24
- Extracted from: RFC-0002 D2, D6

## Context

The reports service is API-only; RFC-0002 adds a small browser UI as a
separate `reports-ui` service that serves static assets and proxies the
reports API. The repo already has one static frontend: the React `frontend`,
served by nginx, which serves files and reverse-proxies `/api/` but exposes no
`/metrics`, `/healthz`, or `/readyz` -- so it does not meet the RFC-0001 D6
uniform service contract. A second static frontend is an opportunity to meet
D6 fully with the least machinery, and to make the contrast a deliberate
exhibit rather than an accident.

## Decision

`reports-ui` is served by **Caddy** (`caddy:2-alpine`, digest-pinned). A tiny
Caddyfile provides the whole service: a static `file_server`, a
`handle_path /api/* -> reverse_proxy reports:8083` (same-origin, so no CORS on
the reports API; `handle_path` strips the `/api` prefix so the backend sees
`/reports/...`, where a bare `reverse_proxy /api/*` would forward the prefix
and 404), `respond /healthz 200` (and `/readyz`, which for a stateless static
server equals liveness), and an explicit `metrics` handler exposing Caddy's
**native** Prometheus metrics on the `:8084` service listener (Caddy serves
them on its admin API by default, not the site listener, so the handler is
required; the admin API stays private). Caddy's built-in metrics are the
pivot: they let `reports-ui` satisfy D6's `/metrics` with no sidecar and no
hand-rolled exporter, which nginx cannot do natively. The repo consequently runs two static servers --
nginx for `frontend`, Caddy for `reports-ui` -- as an intentional contrast on
adjacent dashboards, the same pedagogy as the Rust canary beside the JVM.

## Alternatives

- nginx (matching `frontend`): rejected -- no native Prometheus exporter, so
  D6's `/metrics` would need a sidecar or a stub-status hack, teaching the
  opposite of "one small binary meets the whole contract".
- Serve the UI from the reports JVM (Spring static resources): rejected --
  couples an asset-serving concern to the JVM GC/heap showcase and inflates its
  image.
- A Node/Express or bespoke Go static server: rejected -- adds a build/
  dependency surface or a bespoke binary for what an off-the-shelf server does
  declaratively in a few Caddyfile lines.
- Direct browser -> reports with CORS: rejected -- pushes browser concerns
  into the JVM API and needs an allow-origin policy to maintain.

## Consequences

- Easier: `reports-ui` meets the full D6 contract with a single digest-pinned
  image and no dependency-audit surface (no build step); same-origin proxying
  removes CORS from the reports API.
- Harder: two static servers now exist in the repo. Framed as a deliberate
  contrast, with a documented (not adopted) future path to migrate `frontend`
  to Caddy for a single uniform static-server story.
