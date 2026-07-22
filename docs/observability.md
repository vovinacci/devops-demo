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
