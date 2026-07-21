# Exercise: Break the monitoring layers (Phase 1)

RFC-0001 Phase 1 adds the blackbox and synthetic layers on top of the
whitebox layer that already existed. This exercise breaks the backend on
purpose and watches which layer notices first, and why -- the point of
having three layers is that they catch different failure modes
(ADR-0007).

## Objective

Stop the backend while the `synthetic` profile is running, and observe,
in order: whitebox (`up{job="api"}`), blackbox (`ProbeDown`), and
synthetic (the canary journey). Explain *why* they fire in that order
(or don't fire at all).

## Prerequisites

```shell
make up-full   # core + synthetic profile (blackbox + canary)
```

Confirm all three layers are actually up before breaking anything:

```shell
curl -sS http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, health}'
curl -sS http://localhost:8085/readyz          # canary readyz: 200 regardless of backend state
```

## Steps

1. In Grafana (http://localhost:3000) or the Prometheus UI
   (http://localhost:9090), open three queries in separate panels or
   browser tabs so you can watch them update together:

   ```promql
   up{job="api"}
   probe_success{instance=~"http://api:8000.*"}
   canary_journey_total
   ```

2. Stop the backend, but leave everything else (including Postgres)
   running:

   ```shell
   docker compose -f deploy/compose/docker-compose.yml --project-directory . stop api
   ```

3. Watch the three queries above. Note the wall-clock time each one
   first shows the failure, and how many consecutive scrape/journey
   intervals it took (compare to `scrape_interval` in
   `observability/prometheus.yml` and `CANARY_INTERVAL_SECONDS`).

4. Check `/readyz` on the canary again -- it should still be `200`. If
   it is not, that is a bug, not the expected behavior (RFC-0001 D10):
   the canary's own readiness must never depend on the backend it
   monitors.

5. After both `ProbeDown` and `CanaryJourneyFailing` have had a chance
   to evaluate their `for:` windows, check the Prometheus Alerts page
   (http://localhost:9090/alerts) and confirm both are firing.

6. Read both runbooks (`docs/runbooks/probe-down.md` and
   `docs/runbooks/canary-journey-failing.md`) and follow their "First
   checks" sections against the running stack.

7. Restart the backend and confirm all three signals recover:

   ```shell
   docker compose -f deploy/compose/docker-compose.yml --project-directory . start api
   ```

## Expected observations

- `up{job="api"}` goes to `0` almost immediately (one missed scrape,
  `scrape_interval: 15s`) -- whitebox notices fastest because it is
  scraping the process directly, but it only tells you the process is
  unreachable, not whether the system as a whole is doing its job.
- `probe_success` for the `api` blackbox targets goes to `0` on a
  similar timescale, confirmed from *outside* the process via
  `blackbox_exporter` -- a second, independent signal for the same root
  cause.
- `canary_journey_total{result="failure"}` only increments on the next
  scheduled journey (`CANARY_INTERVAL_SECONDS`, default 30s) -- slower
  by construction, but it is the only layer that would also catch a
  backend that is *up* and *reachable* but returning wrong data (a
  correctness bug that whitebox/blackbox liveness checks cannot see).
- `CanaryJourneyFailing` has a `for: 5m` window on top of the `and rate(...) == 0`
  condition, so it fires noticeably later than `ProbeDown` (`for: 2m`)
  even though the underlying cause is identical -- this is deliberate
  anti-flap tuning, not a slower detector.

## Cleanup

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . start api
make down   # or: docker compose ... --profile "*" down
```

Confirm no `canary-` prefixed items were left behind in the backend
(the canary's best-effort cleanup runs even when the journey fails, but
it is worth checking after an exercise that intentionally broke things):

```shell
curl -sS http://localhost:8000/items | jq '[.[] | select(.name | startswith("canary-"))]'
```

## Discussion questions

1. Which layer would catch a backend that returns `200 OK` on every
   endpoint but has silently stopped persisting writes? Which layers
   would *not* catch it?
2. Why does `ProbeDown` alert on `probe_success == 0` (a per-target
   condition) while `CanaryJourneyFailing` requires *both* recent
   failures *and* zero recent successes? What failure pattern would
   fire one but not the other?
3. The canary's `/readyz` never reflects backend health. Sketch a
   design where it did -- what would break, and for whom?
