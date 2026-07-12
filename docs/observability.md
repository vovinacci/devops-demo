# Observability

The project includes a complete observability stack for monitoring, logging, and performance analysis.

## Metrics

The system automatically collects the following metric categories:

- **HTTP metrics**
  - Total number of HTTP requests by method and endpoint
  - Request latency (p50, p95, p99)
  - Error count by type (4xx, 5xx)

- **Database metrics**
  - Number of database queries
  - Database query latency
  - Connection error count
  - Database connection status

- **Frontend metrics (Web Vitals)**
  - LCP (Largest Contentful Paint) - main content load time
  - INP (Interaction to Next Paint) - interaction response time
  - CLS (Cumulative Layout Shift) - layout stability
  - FCP (First Contentful Paint) - first content render time
  - TTFB (Time to First Byte) - time to first byte

- **PostgreSQL metrics**
  - Number of active connections
  - Database and table sizes
  - Query and transaction statistics
  - Replication metrics (if configured)

All metrics are available through Prometheus at http://localhost:9090 and visualized in Grafana.

## Logs

Centralized logging is implemented through Grafana Alloy and Loki:

- **Automatic log collection** from all containers via Docker labels
- **Storage** in Loki with configured retention
- **Search and analysis** through Grafana Explore
- **Structured logs** with metadata (timestamp, level, service)

Log access:

- Via Grafana: Explore -> select Loki datasource -> LogQL queries
- Via API: http://localhost:3100/loki/api/v1/query
- Via Docker: `docker compose logs -f [service_name]`

## Grafana Dashboards

After startup, a ready-made dashboard **DevOps Demo Dashboard** is automatically available, which includes:

- General API statistics (requests per minute, latency, errors)
- Database metrics (active connections, queries, latency)
- Web Vitals metrics from frontend
- PostgreSQL metrics (DB size, active transactions)
- Trend graphs for various time periods

The dashboard is automatically picked up through Grafana provisioning and is available immediately after startup.

## Alerting

Prometheus is configured with basic SLO rules (Service Level Objectives), defined in `observability/prometheus_slo_rules.yml`. Detailed information about SLO is available in [SLO.md](../observability/SLO.md).
