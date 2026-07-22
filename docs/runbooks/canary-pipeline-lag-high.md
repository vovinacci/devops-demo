# Runbook: CanaryPipelineLagHigh

## Meaning

Alert `CanaryPipelineLagHigh` fires when the canary's pipeline-lag step
(RFC-0001 D9 v2) has a p95 lag above 5s for more than 10 minutes *while
pipeline checks are still succeeding* (`canary_pipeline_check_total{result="ok"}`
has samples in the same window). This measures the create -> gRPC event
stream -> analytics ingest -> read API path end to end (the ADR-0002
durability gap) -- it is a **slow** pipeline, not a stuck one:

- A **stuck** pipeline (item never becomes visible) shows up as
  `canary_pipeline_check_total{result="timeout"}`, not this alert -- see
  "Dashboards / queries" below. That failure mode already pages via
  `AnalyticsStreamDown` if the stream is actually down; this alert
  deliberately does not duplicate it.
- An **absent** analytics profile shows up as
  `canary_pipeline_check_total{result="skipped"}`, and the `and on()`
  guard in the alert expression means a fully-skipped window (no "ok"
  samples at all) cannot fire this alert -- there is no p95 to compute.

This is distinct from `CanaryJourneyFailing`: per Hard rule 9 (ADR-0008
D10), the pipeline-lag step's outcome never fails the journey itself, so
a slow-but-eventually-successful pipeline is invisible to that alert.
This alert exists so that failure mode is not silent.

## First checks

1. Is this actually elevated latency, not a stuck pipeline? Compare the
   two:

   ```promql
   histogram_quantile(0.95, sum(rate(canary_pipeline_lag_seconds_bucket[15m])) by (le))
   sum(rate(canary_pipeline_check_total{result="timeout"}[15m]))
   ```

   A nonzero, climbing `timeout` rate alongside this alert means the
   pipeline is trending from slow to stuck -- treat it as heading toward
   an `AnalyticsStreamDown`-class incident, not a pure latency problem.

2. Is analytics itself healthy and is the gRPC stream connected? A slow
   pipeline is often a stream that is up but behind, not down:

   ```promql
   analytics_stream_connected
   time() - analytics_last_event_time_seconds
   ```

## Dashboards / queries

- `canary_pipeline_lag_seconds` (histogram) -- the alert's own signal;
  the `Monitoring layers` dashboard plots p95 next to per-step latency.
- `canary_pipeline_check_total` by `result` (`ok`/`timeout`/`skipped`) --
  distinguishes slow (this alert), stuck, and absent-analytics.
- `analytics_reconnects_total` -- a climbing rate here alongside rising
  lag points at reconnect churn as the cause, not steady-state slowness.
- `analytics_events_ingested_total` rate -- a real throughput bottleneck
  (vs. reconnect churn) shows as ingestion rate failing to keep up rather
  than reconnect attempts.

## Triage steps

1. Confirm scope per "First checks": slow vs. stuck, and whether
   `analytics_stream_connected` is `1`.
2. Check analytics logs for reconnect churn or slow-query warnings during
   the window the alert covers:

   ```shell
   docker compose -f deploy/compose/docker-compose.yml --project-directory . \
     logs --tail=200 analytics
   ```

3. Check `postgres-analytics` for load (connections, slow queries) -- the
   read API path (`GET /api/v1/items/{id}`) this step polls goes straight
   to that database.
4. If reconnects are climbing, this is likely the same root cause
   `AnalyticsStreamDown`'s runbook covers, just not yet crossed that
   alert's `for: 5m` / disconnected threshold -- follow that runbook's
   triage steps as well.

## Remediation

- If caused by reconnect churn: follow
  `docs/runbooks/analytics-stream-down.md` remediation.
- If caused by database load: the usual Postgres triage (check
  `pg_stat_activity`, long-running queries) applies; this is a generic
  resource problem, not something specific to the canary or analytics.
- No action is needed solely because this alert flapped once and cleared
  on its own -- `for: 10m` already tolerates a brief latency spike.

## Escalation

If lag stays elevated with analytics and its database both reporting
healthy, or the cause is not evident from logs and metrics, file an issue
with `canary_pipeline_lag_seconds` p95 over the incident window,
`canary_pipeline_check_total` by result, and `analytics` +
`postgres-analytics` container logs attached.
