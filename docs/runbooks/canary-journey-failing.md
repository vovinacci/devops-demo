# Runbook: CanaryJourneyFailing

## Meaning

Alert `CanaryJourneyFailing` fires when the canary has recorded at least
one failed journey in the last 10 minutes and zero successful ones for
more than 5 minutes. This is the synthetic layer (ADR-0007 D9): it means
the whole backend CRUD journey (create -> verify -> clean up) is not
completing end to end, not that any single component's own `/metrics`
looks unhealthy (that would be a whitebox alert) or that a probe target
is unreachable (that would be `ProbeDown`, the blackbox layer).

The "no recent success" half of the expression checks staleness of
`canary_journey_last_success_timestamp_seconds` (more than 60s old, or
never set), not a rate of the success counter -- a rate-based check
extrapolates over its whole window and stays misleadingly nonzero for
minutes after a real success, which would delay this alert. Combined
with `for: 5m`, a single failed journey between two successful ones
does not page; only sustained failure with no recent success does --
including before the canary has ever recorded a success at all (cold
start).

## First checks

1. Is the `synthetic` compose profile up? The canary only runs there
   (RFC-0001 D10):

   ```shell
   docker compose -f deploy/compose/docker-compose.yml --project-directory . \
     --profile synthetic ps canary
   ```

2. Is the canary process itself healthy? Its own liveness/readiness
   never reflect backend health (RFC-0001 D10 graceful degradation --
   the canary's dependency is the thing it *monitors*):

   ```shell
   curl -sv http://localhost:8085/healthz
   curl -sv http://localhost:8085/readyz
   ```

   Both `200` and the alert is firing -> the loop is running but the
   journey itself is failing (see triage below). Either non-200 -> the
   canary process itself is the problem, not what it is monitoring.

3. Is this the first journey right after a cold `make up-full`? The
   canary depends on `api` with `service_started`, not `service_healthy`
   (deliberate, RFC-0001 D10: the canary must be able to start and report
   failures while the backend it monitors is still down), so it can tick
   before the backend has finished its Alembic migration on startup. One
   failed journey in this narrow window is expected, not an incident --
   confirm by checking whether the very next scheduled journey succeeds.

## Dashboards / queries

- `canary_journey_total{result="failure"}` vs `{result="success"}` --
  the raw counters the alert expression is built from.
- `canary_journey_step_duration_seconds` by `step` (`create`, `verify`,
  `pipeline`, `report`, `delete`) -- per-step latency, recorded even for
  failed steps. A step
  whose duration climbs toward `CANARY_TIMEOUT_SECONDS` right before the
  failure rate rises tells you which step is degrading. The `pipeline`
  and `report` steps also appear here but are best-effort and never fail
  the journey verdict (Hard rule 9, ADR-0008 D10) -- do not chase them
  for *this* alert; their own signals are `canary_pipeline_check_total`
  and `canary_report_check_total` on the Monitoring Layers dashboard.
- `canary_journey_last_success_timestamp_seconds` -- age since the last
  success; `time() - canary_journey_last_success_timestamp_seconds` is
  the staleness in seconds.
- `Monitoring layers` Grafana dashboard puts these next to blackbox
  `probe_success` and whitebox RED metrics for the same time window.

## Triage steps

1. Check which step is failing. The canary's own JSON logs carry a
   `trace_id` per journey (RFC-0001 D11) -- filter Loki on
   `{container_name="canary"}` and read the `WARN` line, which names the
   failing step:

   ```shell
   docker compose -f deploy/compose/docker-compose.yml --project-directory . \
     logs --tail=200 canary
   ```

2. `create` step failing -> the backend is unreachable or rejecting
   writes. Cross-check `ProbeDown` for `api:8000/healthz` and
   `api:8000/readyz` (this is the blackbox layer catching the same
   root cause from the outside) and the backend's own logs.
3. `verify` step failing but `create` succeeding -> the item was written
   but is not visible on `GET /items` -- a read-your-write consistency
   problem (e.g. read replica lag, cache), not a reachability problem.
4. `delete` step failing -> cleanup is not completing; check for
   `canary-` prefixed items accumulating in the backend (the canary
   still reports success/failure from `verify`, but lingering synthetic
   data is itself worth fixing -- see ADR-0007 "cleans up after itself").
5. Confirm scope: is `ProbeDown` also firing for the backend? If yes,
   this is one incident with two symptoms across two monitoring layers.
   If `ProbeDown` is clean but the canary is failing, the backend is
   reachable but not behaving correctly -- a correctness gap only the
   synthetic layer catches.

## Remediation

- Restart the backend if it is the root cause:
  `docker compose ... restart api`.
- Restart the canary if its own process looks stuck (readyz not 200):
  `docker compose ... restart canary`.
- If `canary-` prefixed items are accumulating in the backend, they are
  safe to delete manually (they are the synthetic tag from ADR-0007 D9)
  once the root cause of failed cleanup is fixed.

## Escalation

If the failing step is not evident from logs and metrics, or
remediation does not clear the alert, file an issue with the `trace_id`
of a failing journey, the failing step, and relevant backend and canary
container logs attached.
