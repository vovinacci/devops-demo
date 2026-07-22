# Runbook: AnalyticsStreamDown

## Meaning

Alert `AnalyticsStreamDown` fires when `analytics_stream_connected == 0`
for more than 5 minutes *while the analytics process itself is up*
(`up{job="analytics"} == 1`). This is the ingest-specific whitebox signal
(RFC-0001 Phase 3 PR-B, ADR-0002): the analytics HTTP surface is healthy
and `/readyz` is likely still `200` (readiness is DB-only, ADR-0005), but
its gRPC client has lost its connection to the backend's `ItemService`
and is stuck in the reconnect backoff loop -- no events are being
ingested for the duration of the outage.

This is distinct from `up{job="analytics"} == 0` (the whole process/
container is unreachable -- an ordinary target-down condition) and from
`ProbeDown` (blackbox HTTP reachability) -- both of those already cover
"analytics itself is down" without this alert's help.

## First checks

1. Is analytics itself actually up? If not, this alert should not be the
   one firing -- `up{job="analytics"}` should have already fired instead:

   ```promql
   up{job="analytics"}
   ```

2. Is the backend (`api`) up and reachable on its gRPC port? The stream
   is analytics dialing *out* (ADR-0002: analytics dials, backend never
   does), so the most common cause is the backend being down or
   unreachable, not analytics itself:

   ```shell
   docker compose -f deploy/compose/docker-compose.yml --project-directory . \
     --profile analytics ps api analytics
   ```

3. Is this the documented at-most-once dip window, not an ongoing
   outage? Check whether the stream has already recovered by the time
   you look -- `for: 5m` means the alert can still be firing on the
   tail end of a since-recovered blip:

   ```promql
   analytics_stream_connected
   ```

## Dashboards / queries

- `analytics_stream_connected` -- 0/1, the alert's own condition.
- `rate(analytics_reconnects_total[5m])` -- reconnect attempts in
  flight; a nonzero, climbing rate confirms the client is actively
  retrying (not stuck silently).
- `time() - analytics_last_event_time_seconds` -- staleness of the most
  recently ingested event; compare against `canary_pipeline_lag_seconds`
  (canary v2) for the same signal measured from the other end of the
  pipeline.
- `analytics_snapshot_reconciles_total` -- should tick up by one shortly
  after recovery (the reconnect sequence always re-snapshots before
  resuming the stream, ADR-0002).
- `analytics_events_ingested_total` / `analytics_events_deduplicated_total`
  -- resume of nonzero ingestion rate confirms events are flowing again.

## Triage steps

1. Confirm scope per "First checks" above: analytics up but stream down,
   backend reachability, and whether this is already recovering.
2. Check analytics logs for the reconnect loop's own error detail (dial
   failure vs. snapshot RPC failure vs. mid-stream error):

   ```shell
   docker compose -f deploy/compose/docker-compose.yml --project-directory . \
     logs --tail=200 analytics
   ```

3. Check the backend's gRPC port directly, bypassing analytics:

   ```shell
   grpcurl -plaintext localhost:50051 list devopsdemo.items.v1.ItemService
   ```

4. If the backend is up and reachable but the stream still won't
   reconnect, check `ANALYTICS_BACKEND_GRPC_ADDR` is correct for the
   environment (compose sets `api:50051`; a local `go run` defaults to
   `localhost:50051`).

## Remediation

- If the backend was down: restart it
  (`docker compose ... restart api`) and confirm `analytics_stream_connected`
  returns to `1` and `analytics_snapshot_reconciles_total` increments
  (the reconnect sequence always re-snapshots before resuming the
  stream).
- The aggregate dip for the disconnected window does **not** self-heal
  and is not supposed to (ADR-0002: snapshot recovers state, not missed
  events) -- do not attempt to backfill `event_buckets` for that window;
  the dip staying visible is the documented behavior, not a bug.
- If analytics itself needs restarting (e.g. stuck reconnect loop that
  doesn't clear on backend recovery), `docker compose ... restart
  analytics` is safe: `/readyz` will report DB-only readiness immediately
  and the ingest client re-snapshots on the next connect.

## Escalation

If the stream stays down for an extended period with the backend
confirmed healthy and reachable, or the reconnect loop's own logs show
repeated identical failures that don't match backend downtime, file an
issue with `analytics_stream_connected`, `analytics_reconnects_total`,
recent `analytics` container logs, and the backend's own status attached.

## Silencing

While this alert is firing, Alertmanager inhibits `CanaryPipelineLagHigh`
automatically (same root cause, see
`observability/alertmanager/alertmanager.yml`). If this alert itself is
already under active triage, silence it instead of letting it repeat
(amtool ships inside the Alertmanager image):
`docker compose -f deploy/compose/docker-compose.yml --project-directory . exec alertmanager amtool silence add alertname=AnalyticsStreamDown --alertmanager.url=http://localhost:9093`,
or via the Alertmanager UI at http://localhost:9093.
