# Observability

Complete local observability stack: metrics (Prometheus), dashboards
(Grafana), logs (Loki + Alloy), SLO rules. Everything is provisioned from
files in `observability/` -- nothing lives only in a UI.

## Metrics

Collected automatically:

- **HTTP** -- request counts by method/endpoint/status, latency histograms
- **Database (app-side)** -- query counts, latency, errors by operation
- **Frontend Web Vitals** -- LCP, INP, CLS, FCP, TTFB (browser support
  varies: full set requires Chromium; WebKit lacks LCP/CLS)
- **PostgreSQL** -- connections, sizes, query statistics (postgres-exporter)
- **Containers** -- CPU, memory, network per service (cAdvisor)

Prometheus UI: http://localhost:9090. All metrics feed the Grafana
dashboard.

## Logs

All container logs flow through Grafana Alloy into Loki, structured with
container and service labels.

- Grafana Explore -> Loki datasource -> LogQL
- API: http://localhost:3100/loki/api/v1/query
- CLI: `make logs` (api; other services via
  `docker compose -f deploy/compose/docker-compose.yml --project-directory . logs -f <service>`)

## Dashboards

The **DevOps Demo** dashboard is provisioned from
`observability/grafana/dashboards/` and available right after `make up`
(Grafana: http://localhost:3000, credentials in the README): API RED
metrics, database and PostgreSQL panels, Web Vitals, container resources,
log panel, SLO status.

## Offered load

The **Load (k6)** dashboard (`load` compose profile, RFC-0001 D4,
ADR-0006) puts k6's own request rate ("offered load", pushed via the
built-in `-o experimental-prometheus-rw` output) next to the backend's
own observed RED p95 latency -- what loadgen asked the system to do,
beside what the system actually delivered. Also panels for per-scenario
client-observed latency, error rate, gRPC scenario latency, active VUs,
and the abuse/gRPC correctness signals (`loadgen/README.md` documents
every metric name and the exact expressions). The `report` scenario's own
signal (`loadgen_report_job_failed`) is nightly/full-only and gated on
`LOADGEN_REPORTS_URL` (RFC-0001 D12), so it is absent from this dashboard on
a normal `load`-profile run -- its outcome shows up on the Reports JVM
dashboard's job row instead. `make incident` /
`make heal` (`loadgen/README.md`) spike offered load or error rate on
demand to exercise this dashboard and the SLO burn-rate alerts live.

## Reports JVM dashboard

The **Reports JVM** dashboard (`observability/grafana/dashboards/reports.json`,
`reports` compose profile, RFC-0001 D2/D6 Phase 6) is the JVM showcase's
whitebox view: heap used vs committed and non-heap, GC pause rate and time
by action, live threads, uptime, HTTP request rate and p95 latency, and a
scrape-health stat (`up{job="reports"}`). Prometheus scrapes it via the
`reports` job (`observability/prometheus.yml`). As of Phase 6 PR-2 the **GC
sawtooth** exhibit is live: the report engine's bursty Apache POI / OpenPDF
allocation during generation drives the heap used-line up between collections
and down on each GC, and pushes the GC pause-rate/time panels -- garbage
collection made visible, the reason a heavyweight JVM framework earns its
place here (RFC-0001 D2). A dedicated **report-job row** reads the service's
custom Micrometer meters: jobs by terminal status
(`reports_jobs_completed_total{status,type,format}`), job-duration p95
(`reports_job_duration_seconds`, histogram buckets enabled in
`application.yml`), in-flight jobs (`reports_jobs_inflight`, a gauge capped at
`reports.job-concurrency`), and artifact throughput
(`reports_artifact_bytes`). These are empty until reports are generated; as of
Phase 6 PR-3 the loadgen `report` scenario fills them in the nightly/full run
(`make smoke-full` / `make up-full`, gated on `LOADGEN_REPORTS_URL`, RFC-0001
D12 -- see `loadgen/README.md`), and a manual `POST /reports` fills them any
time. Being an opt-in profile, the panels show "No data" and the scrape target
reads "down" when `reports` is not up (RFC-0001 D10).

## Monitoring Layers dashboard

The **Monitoring Layers** dashboard
(`observability/grafana/dashboards/monitoring-layers.json`, RFC-0001 D9,
ADR-0007) puts the three monitoring layers side by side: whitebox (the
backend's own RED metrics), blackbox (probe results), and synthetic (the
Rust canary's journey). The synthetic row reads the canary-exclusive
meters: journey success rate, per-step latency
(`canary_journey_step_duration_seconds` by `step`), the v2 pipeline-lag
panels, and -- as of Phase 6 -- the v3 report step:

- `canary_report_generation_seconds` -- report generation latency (submit
  -> `SUCCEEDED`), histogram; only `ok` outcomes are observed, so the p95
  panel reads real generation latency undiluted by timeouts/failures/skips.
- `canary_report_check_total{result}` -- report-step outcomes by result
  (`ok` / `timeout` / `failed` / `skipped`). Like the pipeline check,
  none of the four fails the canary journey itself (Hard rule 9, ADR-0008
  D10): a `skipped` (reports profile absent) or `timeout`/`failed` is a
  reports signal, and this panel is where it is visible.

The canary is the `synthetic` compose profile, so these panels show "No
data" when it is not up -- the same documented degradation as the other
additive profiles (RFC-0001 D10).

## Historical dashboard and the D5 boundary

The **Analytics History** dashboard (`analytics` compose profile,
RFC-0001 Phase 5 D5/Section 7, ADR-0003) queries `postgres-analytics`
directly via a dedicated Grafana **Postgres datasource** (`Analytics
Postgres`, uid `DS_ANALYTICS_PG`, provisioned in
`observability/grafana/provisioning/datasources/`) -- not Prometheus.
This is the D5 boundary made visible: **Prometheus is not backfilled**
(its panels, e.g. `Analytics Ingest`, show data only since the stack
last started); this dashboard reads `event_buckets`, `current_items`,
and `seed_marker` as durable business data, so a panel spanning the seam
(`now-30d -> now+1h`) between seeded history and live traffic shows no
discontinuity. "Ops metrics are ephemeral, business data is durable."

Panels: events/hour stacked by `event_type` (created/deleted only, see
`services/analytics/README.md`'s by-type seam note) and daily totals
over a 30-day default range, `current_items` live-vs-tombstoned counts,
and a `seed_marker` info panel (scale/seed/days/ref time/events
written) -- the same contract loadgen's scale guard checks at startup
(`loadgen/README.md`). Vertical annotations mark the 3 seeded story
anomalies (spike/outage/degradation), written by `analytics seed` at
seed time via the Grafana HTTP annotation API
(`services/analytics/README.md`'s Grafana annotations section) and
tagged `seed-anomaly` so this dashboard's annotation query finds them
regardless of which dashboard they were written from. `make up-workshop`
(`DEMO_TIME_SCALE=24`) is the fastest way to see all three at once;
`docs/exercises/05-find-the-seeded-anomalies.md` walks through finding
them and a live-verified caveat where two of the three leave no trace in
the hourly aggregate at that scale even though their annotation stays exact.

## SLOs

Prometheus recording rules define availability, latency, and error-rate
SLOs: `observability/prometheus_slo_rules.yml`, explained in
[SLO.md](../observability/SLO.md).

## Alerting

Prometheus evaluates the four alert rules in
`observability/prometheus_alerts.yml` and forwards them to Alertmanager
(`alerting:` block in `observability/prometheus.yml`), which owns
routing, grouping, and inhibition as code:
`observability/alertmanager/alertmanager.yml`. The visible receiver is
Mailpit (RFC-0001 Section 7, amended during Phase 4 -- see the RFC) --
notifications land at http://localhost:8025, both the web UI and the
REST API (`/api/v1/messages`).

- Alertmanager UI: http://localhost:9093 (active alerts, silences).
- Silence an alert during triage (amtool ships inside the Alertmanager
  image, nothing to install):
  `docker compose -f deploy/compose/docker-compose.yml --project-directory . exec alertmanager amtool silence add alertname=<name> --alertmanager.url=http://localhost:9093`,
  or use the Alertmanager UI directly -- see the relevant runbook's
  Escalation section.
- `AnalyticsStreamDown` inhibits `CanaryPipelineLagHigh`: one root cause,
  one notification.
