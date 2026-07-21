# Runbook: ProbeDown

## Meaning

Alert `ProbeDown` fires when `probe_success == 0` for a blackbox target for
more than 2 minutes. This is the blackbox layer (ADR-0007 D9): it means the
target failed an HTTP reachability check from `blackbox_exporter`, not that
its own `/metrics` are unhealthy (that would be a whitebox alert instead).

## First checks

1. Is the `synthetic` compose profile even up? Blackbox and its scrape
   targets only exist when `synthetic` is running (ADR-0008 D10):

   ```shell
   docker compose -f deploy/compose/docker-compose.yml --project-directory . \
     --profile synthetic ps blackbox
   ```

   If `synthetic` is not up, every `blackbox_http` target is expected to
   show `down` -- documented degradation, not an incident.

2. Is it one target or all of them? Check the Prometheus targets page
   (http://localhost:9090/targets) or query directly:

   ```promql
   probe_success
   ```

   All targets down -> suspect blackbox itself or the shared `devnet`
   network. One target down -> suspect that service.

## Dashboards / queries

- `probe_success` by `instance` -- 1 = reachable, 0 = failing.
- `probe_http_status_code` by `instance` -- HTTP status blackbox observed
  (0 means no response at all, e.g. connection refused/timeout).
- `probe_duration_seconds` by `instance` -- latency of the probe itself;
  a rising trend before a drop to 0 often means the target was degrading.

## Triage steps

1. Confirm scope (all vs one target) per above.
2. Check the failing service's own logs:

   ```shell
   docker compose -f deploy/compose/docker-compose.yml --project-directory . \
     logs --tail=200 <service>
   ```

3. Check the service's own health endpoint directly (bypass blackbox):

   ```shell
   curl -sv http://localhost:<port>/<health-path>
   ```

4. If `api:8000/readyz` is down but `api:8000/healthz` is up: the process
   is alive but a dependency (DB) is not ready -- check `db` container
   health, not the API process.

## Remediation

- Restart the failing service: `docker compose ... restart <service>`.
- If the target is a dependency the service tolerates being absent
  (see ADR-0008 graceful degradation), confirm the dependent service
  degrades rather than crash-loops before treating this as urgent.
- If blackbox itself is unreachable, check `blackbox` container logs and
  that `observability/blackbox/blackbox.yml` is mounted and valid.

## Escalation

If the cause is not evident after the above, or remediation does not
clear the alert, file an issue with the failing `instance` label, the
`probe_http_status_code` value, and relevant container logs attached.
