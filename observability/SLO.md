# Service Level Objectives (SLO)

This document describes Service Level Objectives (SLO) for critical API endpoints of the DevOps Demo project. SLO define target quality levels for the service and are used for monitoring and managing system reliability.

## Table of Contents

- [SLO Definition](#slo-definition)
- [Critical Endpoints](#critical-endpoints)
- [Prometheus Metrics](#prometheus-metrics)
- [Grafana Visualization](#grafana-visualization)
- [SLO Verification](#slo-verification)
- [Alerts](#alerts)
- [Configuration](#configuration)
- [Technical Calculation Details](#technical-calculation-details)

## SLO Definition

SLO define target quality levels for the service based on metrics that are important to users. The project uses three main types of SLO:

### 1. Availability

**Target:** 99.95% availability

**Window:** 7 days (rolling window)

**Scope:** Critical endpoints (`/health`, `/items`)

**Metric:** Percentage of successful requests (non-5xx) out of total requests

**Formula:**

```text
Availability = (Successful Requests / Total Requests) * 100%
```

**Error Budget:** 0.05% (maximum 5 errors per 10,000 requests)

**Explanation:** The service must be available 99.95% of the time over a 7-day period. This means a maximum of 0.05% downtime or errors is allowed.

### 2. Latency

**Target:** p95 < 200ms (0.2 seconds)

**Window:** 7 days (rolling window)

**Scope:** Critical endpoints (`/health`, `/items`)

**Metric:** 95th percentile of request latency

**Formula:**

```text
Latency p95 = 95th percentile of request duration
```

**Error Budget:** Requests exceeding 200ms are considered SLO violations

**Explanation:** 95% of all requests must be processed faster than 200 milliseconds. This means only 5% of requests can be slower.

### 3. Error Rate

**Target:** < 0.1% for 5xx errors

**Window:** 7 days (rolling window)

**Scope:** Critical endpoints (`/health`, `/items`)

**Metric:** Percentage of requests with 5xx status out of total

**Formula:**

```text
Error Rate = (5xx Requests / Total Requests) * 100%
```

**Error Budget:** Maximum 0.1% of requests can return 5xx errors

**Explanation:** Less than 0.1% of all requests should return server errors (5xx). Client errors (4xx) are not included in this SLO.

## Critical Endpoints

SLO apply to the following critical endpoints:

- `GET /health` - Health check endpoint for monitoring service status
- `GET /items` - Get list of items (most frequently used endpoint)
- `POST /items` - Create new item
- `DELETE /items/{item_id}` - Delete item

These endpoints are defined as critical because they:

- Are used most frequently
- Affect core application functionality
- Are critical for user experience

## Prometheus Metrics

All SLO metrics are calculated via Prometheus recording rules in the `observability/prometheus_slo_rules.yml` file. Recording rules allow pre-computing complex metrics for faster access and visualization.

### Recording Rules

#### Availability

**`slo:availability:ratio7d`**

- **Type:** Gauge
- **Description:** Current availability level over 7 days (0-1, where 1 = 100%)
- **Formula:** `rate(http_requests_total{status!~"5.."}[5m]) / rate(http_requests_total[5m])`
- **Target Value:** > 0.9995 (99.95%)

**`slo:availability:error_budget7d`**

- **Type:** Gauge
- **Description:** Consumed failure fraction (near 0 when healthy, grows with errors)
- **Formula:** `1 - slo:availability:ratio7d`
- **Interpretation:** Compare against the allowed budget of 0.0005 (100% - 99.95%)

**`slo:availability:error_budget_burn7d`**

- **Type:** Gauge
- **Description:** Error budget overrun (positive = SLO violation)
- **Formula:** `slo:availability:error_budget7d - (1 - 0.9995)`
- **Interpretation:** > 0 means SLO violation

#### Latency

**`slo:latency:p95_7d`**

- **Type:** Gauge
- **Description:** p95 latency over 7 days (in seconds)
- **Formula:** `histogram_quantile(0.95, rate(http_request_latency_seconds_bucket[5m]))`
- **Target Value:** < 0.2 (200ms)

**`slo:latency:ratio7d`**

- **Type:** Gauge
- **Description:** Percentage of requests < 200ms (0-1)
- **Formula:** `rate(http_request_latency_seconds_bucket{le="0.2"}[5m]) / rate(http_request_latency_seconds_count[5m])`
- **Target Value:** > 0.95 (95%)

**`slo:latency:error_budget7d`**

- **Type:** Gauge
- **Description:** Fraction of requests slower than 200ms (near 0 when healthy)
- **Formula:** `1 - slo:latency:ratio7d`
- **Interpretation:** Compare against the allowed budget of 0.05 (100% - 95%)

**`slo:latency:error_budget_burn7d`**

- **Type:** Gauge
- **Description:** Error budget overrun for latency (positive = SLO violation)
- **Formula:** `slo:latency:error_budget7d - (1 - 0.95)`
- **Interpretation:** > 0 means SLO violation

#### Error Rate

**`slo:error_rate:actual_5xx_ratio7d`**

- **Type:** Gauge
- **Description:** Actual percentage of 5xx errors
- **Formula:** `rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m])`
- **Target Value:** < 0.001 (0.1%)

**`slo:error_rate:error_budget_burn7d`**

- **Type:** Gauge
- **Description:** Error budget overrun for error rate
- **Formula:** `slo:error_rate:actual_5xx_ratio7d - 0.001`
- **Interpretation:** > 0 means SLO violation

## Grafana Visualization

SLO metrics are displayed in the main dashboard **DevOps Demo Dashboard** (available at http://localhost:3000 after starting services).

### Dashboard Panels

1. **SLO: Availability (99.95% target)**

   - **Type:** Stat panel
   - **Metric:** `slo:availability:ratio7d`
   - **Format:** Percentage (e.g., "99.97%")
   - **Colors:** Green if > 99.95%, red if < 99.95%

1. **SLO: Latency p95 (< 200ms target)**

   - **Type:** Stat panel
   - **Metric:** `slo:latency:p95_7d`
   - **Format:** Time in milliseconds (e.g., "185ms")
   - **Colors:** Green if < 200ms, red if >= 200ms

1. **SLO: Error Rate 5xx (< 0.1% target)**

   - **Type:** Stat panel
   - **Metric:** `slo:error_rate:actual_5xx_ratio7d`
   - **Format:** Percentage (e.g., "0.05%")
   - **Colors:** Green if < 0.1%, red if >= 0.1%

1. **SLO: Error Budget Burn Rate (7 days)**

- **Type:** Time series graph
- **Metrics:**
  - `slo:availability:error_budget_burn7d`
  - `slo:latency:error_budget_burn7d`
  - `slo:error_rate:error_budget_burn7d`
- **Format:** Graph with three lines for each SLO type
- **Interpretation:** Values > 0 indicate SLO violations

### Dashboard Access

1. Open Grafana: http://localhost:3000
2. Login with credentials: `admin/admin`
3. Navigate to "Dashboards" -> "DevOps Demo Dashboard"
4. Find SLO panels at the top of the dashboard

## SLO Verification

### Via Prometheus UI

Open Prometheus UI: http://localhost:9090 and use the following queries:

**Availability:**

```promql
slo:availability:ratio7d
```

**Latency p95:**

```promql
slo:latency:p95_7d
```

**Error Rate:**

```promql
slo:error_rate:actual_5xx_ratio7d
```

**Error Budget Burn:**

```promql
slo:availability:error_budget_burn7d
slo:latency:error_budget_burn7d
slo:error_rate:error_budget_burn7d
```

### Via Grafana

Open dashboard and check SLO panels at the top. All panels update automatically every 30 seconds.

### Via Command Line

Use `curl` to get metrics via Prometheus API:

```shell
# Availability
curl 'http://localhost:9090/api/v1/query?query=slo:availability:ratio7d'

# Latency p95
curl 'http://localhost:9090/api/v1/query?query=slo:latency:p95_7d'

# Error Rate
curl 'http://localhost:9090/api/v1/query?query=slo:error_rate:actual_5xx_ratio7d'
```

## Alerts

For automatic notifications about SLO violations, alerts can be added in Prometheus. Alerts are configured via Prometheus alerting rules.

### Example Alert Configuration

Add to `observability/prometheus.yml` or separate file with alerting rules:

```yaml
groups:
  - name: slo_alerts
    interval: 30s
    rules:
      - alert: SLODowntimeHigh
        expr: slo:availability:error_budget_burn7d > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "SLO Availability violated"
          description: "Service availability below 99.95% for 5 minutes"

      - alert: SLOLatencyHigh
        expr: slo:latency:p95_7d > 0.2
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "SLO Latency violated"
          description: "p95 latency exceeds 200ms for 5 minutes"

      - alert: SLOErrorRateHigh
        expr: slo:error_rate:error_budget_burn7d > 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "SLO Error Rate violated"
          description: "5xx error rate exceeds 0.1% for 5 minutes"
```

### Alertmanager Configuration

To send notifications, Alertmanager needs to be configured:

1. Add Alertmanager to `docker-compose.yml`
2. Configure notification channels (email, Slack, PagerDuty, etc.)
3. Connect Prometheus to Alertmanager

Detailed information about Alertmanager configuration is available in the [official documentation](https://prometheus.io/docs/alerting/latest/alertmanager/).

## Configuration

SLO configuration is located in the following files:

**Recording Rules:**

- `observability/prometheus_slo_rules.yml` - Definition of recording rules for calculating SLO metrics

**Prometheus configuration:**

- `observability/prometheus.yml` - Connection of recording rules to Prometheus via `rule_files`

**Grafana Dashboard:**

- `observability/grafana/dashboards/devops-demo-dashboard.json` - JSON dashboard configuration with SLO panels

### Changing SLO Target Values

To change SLO target values:

1. **Update recording rules** in `prometheus_slo_rules.yml`:
   - Change threshold values in formulas (e.g., `0.9995` for availability)
   - Update error budget calculations

2. **Update Grafana dashboard** in `devops-demo-dashboard.json`:
   - Change threshold values in panels
   - Update descriptions and annotations

3. **Restart services:**

   ```shell
   docker compose restart prometheus grafana
   ```

## Technical Calculation Details

### SLO Metric Calculation

SLO metrics are calculated via Prometheus recording rules using the `rate()` function over a 5-minute window. This provides the current request rate, which is a good approximation for SLO.

**Why 5-minute window:**

- Balance between accuracy and performance
- Fast problem detection
- Less load on Prometheus

**Limitations:**

- This is an approximation for a 7-day window
- Exact calculation over full 7 days would require storing all data for that period
- For most cases, the approximation is sufficiently accurate

### More Accurate Calculation

For more accurate SLO calculation over full 7 days, subqueries with `avg_over_time()` can be used:

```promql
# More accurate availability calculation over 7 days
avg_over_time(
  (
    rate(http_requests_total{status!~"5.."}[5m])
    /
    rate(http_requests_total[5m])
  )[7d:]
)
```

**Disadvantages of exact calculation:**

- Requires more Prometheus resources
- May be slower for large datasets
- More complex to configure and maintain

### Metric Updates

Recording rules are updated every 30 seconds (configured in `prometheus.yml` via `evaluation_interval`).

**Important:** For SLO to work correctly, Prometheus must collect metrics for at least several minutes. After first startup, SLO metrics will be available after accumulating sufficient data (at least 5 minutes).

### SLO Validation

To verify SLO calculation correctness:

1. **Check base metrics:**

   ```promql
   rate(http_requests_total[5m])
   rate(http_requests_total{status=~"5.."}[5m])
   ```

2. **Check recording rules:**

   ```promql
   slo:availability:ratio7d
   ```

3. **Compare with manual calculations:**

   - Calculate availability manually based on base metrics
   - Compare with recording rule result

---

**Additional Information:**

Detailed documentation about Prometheus and Grafana is available in the [official Prometheus documentation](https://prometheus.io/docs/) and
[Grafana documentation](https://grafana.com/docs/).
