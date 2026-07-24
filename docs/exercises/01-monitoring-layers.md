# Exercise: Break the monitoring layers (Phase 1)

RFC-0001 Phase 1 adds the blackbox and synthetic layers on top of the
whitebox layer that already existed (ADR-0007). This exercise breaks the
backend on purpose and watches which layer notices first, and why -- the
whole point of having three layers is that each catches failure modes the
others cannot. Exercise 03 extends the same idea with a fourth signal once
analytics is in the picture.

## Objective

Stop the backend while the `synthetic` profile is running and observe, in
order: whitebox (`up{job="api"}`), blackbox (`probe_success` / `ProbeDown`),
and synthetic (the canary journey / `CanaryJourneyFailing`). Explain *why*
they fire in that order -- or don't fire at all.

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

1. In Grafana (http://localhost:3000, admin/admin) or the Prometheus UI
   (http://localhost:9090), open the three queries below in separate
   panels or browser tabs so you can watch them update together:

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

3. Watch the three queries. Note the wall-clock time each one first shows
   the failure and how many consecutive scrape/journey intervals it took
   (compare against `scrape_interval: 15s` in `observability/prometheus.yml`
   and `CANARY_INTERVAL_SECONDS`, default 30s).

4. Check `/readyz` on the canary again -- it must still be `200`. If it is
   not, that is a bug, not the expected behavior (RFC-0001 D10): the
   canary's own readiness must never depend on the backend it monitors.

5. After both `ProbeDown` (`for: 2m`) and `CanaryJourneyFailing`
   (`for: 5m`) have had a chance to evaluate their windows, open the
   Prometheus Alerts page (http://localhost:9090/alerts) and confirm both
   are firing.

6. Read both runbooks (`docs/runbooks/probe-down.md` and
   `docs/runbooks/canary-journey-failing.md`) and follow their "First
   checks" sections against the running stack.

7. Restart the backend and confirm all three signals recover:

   ```shell
   docker compose -f deploy/compose/docker-compose.yml --project-directory . start api
   ```

## Expected observations

- `up{job="api"}` goes to `0` almost immediately (one missed scrape,
  `scrape_interval: 15s`) -- whitebox notices fastest because it scrapes
  the process directly, but it only tells you the process is unreachable,
  not whether the system as a whole is doing its job.
- `probe_success` for the `api` blackbox targets goes to `0` on a similar
  timescale, confirmed from *outside* the process via `blackbox_exporter`
  -- a second, independent signal for the same root cause.
- `canary_journey_total{result="failure"}` only increments on the next
  scheduled journey (`CANARY_INTERVAL_SECONDS`, default 30s) -- slower by
  construction, but the canary is the only layer that would also catch a
  backend that is *up* and *reachable* but returning wrong data (a
  correctness bug that whitebox/blackbox liveness checks cannot see).
- `CanaryJourneyFailing` fires on a compound condition -- at least one
  failure in the last 10 minutes (`increase(canary_journey_total{result="failure"}[10m]) > 0`)
  *and* more than 60s elapsed since the last success
  (`time() - max(canary_journey_last_success_timestamp_seconds) > 60`) --
  then a `for: 5m` window on top. It is a gauge-of-staleness check, not a
  rate threshold. Combined with the `for: 5m`, it fires noticeably later than
  `ProbeDown` (`for: 2m`) even though the underlying cause is identical --
  deliberate anti-flap tuning, not a slower detector.

## Cleanup

First restart the backend and, while the stack is still up, confirm no
`canary-` prefixed items were left behind (the canary's best-effort cleanup
runs even when the journey fails, but it is worth checking after an exercise
that intentionally broke things):

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . start api
curl -sS http://localhost:8000/items | jq '[.[] | select(.name | startswith("canary-"))]'
```

Then tear the stack down (this stops the backend, so it must come after the
check above):

```shell
make down   # or: docker compose -f deploy/compose/docker-compose.yml --project-directory . --profile "*" down
```

## Discussion questions

1. Which layer would catch a backend that returns `200 OK` on every
   endpoint but has silently stopped persisting writes? Which layers would
   *not* catch it, and why?
2. `ProbeDown` alerts on `probe_success == 0` (a per-target condition)
   while `CanaryJourneyFailing` requires *both* recent failures *and* zero
   recent successes. What failure pattern would fire one but not the
   other?
3. The canary's `/readyz` never reflects backend health. Sketch a design
   where it did -- what would break, and for whom?
