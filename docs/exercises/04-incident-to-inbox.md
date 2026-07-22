# Exercise: Incident to inbox (Phase 4)

Phase 4 completes the alerting path (RFC-0001 Section 7): Alertmanager now
sits between Prometheus and a receiver a student can actually watch --
Mailpit. This exercise runs the full on-call loop end to end: trigger an
outage, watch the alert fire, watch the notification land in an inbox,
silence it during triage, heal the outage, watch the resolved mail arrive.

## Objective

Break a service, observe `ProbeDown` fire in Prometheus and forward to
Alertmanager, watch the notification appear in Mailpit
(http://localhost:8025), silence the alert with `amtool`, restore the
service, and confirm both the alert and the silence clear together with a
"resolved" mail.

## A reality check before you start

RFC-0001 Section 7 describes `make incident` as the mechanism that
"exercises the SLO burn-rate alerts live". As of this phase that sentence
is aspirational, not accurate, and this exercise is built around what
actually fires today rather than what was originally planned -- worth
sitting with for a moment, because "the doc says X but the system does Y"
is itself the most common on-call reality:

- `observability/prometheus_slo_rules.yml` defines **recording rules**
  only (`slo:availability:ratio7d`, `slo:latency:p95_7d`,
  `slo:error_rate:error_budget_burn7d`, ...) -- numbers Grafana can chart.
  None of them has a matching `alert:` rule. A recording rule computing a
  burn rate and an alert rule that pages on it are two different things;
  this repo currently has the first without the second.
- The four alerts that actually exist
  (`observability/prometheus_alerts.yml`) are `ProbeDown`,
  `CanaryJourneyFailing`, `CanaryPipelineLagHigh`, `AnalyticsStreamDown` --
  none of them key off `http_requests_total{status=~"5.."}` or any of the
  SLO recording rules above.
- `make incident INCIDENT_MODE=errors` rotates `409`/`422`/`404` responses
  (`loadgen/README.md`) -- all 4xx, never `5xx`. `blackbox_exporter`'s
  `probe_success` only checks that the endpoint answered at all, so it
  stays `1` throughout; `ProbeDown` cannot fire from this traffic no
  matter how long it runs. Step 2 below proves this live instead of just
  asserting it.
- Net result: **today, neither `INCIDENT_MODE=spike` nor
  `INCIDENT_MODE=errors` pages anyone.** They are a real, valuable
  exhibit for the Load dashboard (offered load, error rate, the SLO
  panels' numbers moving) -- just not, yet, an alerting exhibit. Wiring a
  real SLO burn-rate alert (multi-window burn rate, Google SRE workbook
  style, so a 7-day-window ratio doesn't take a week to page on a real
  burn) is future work -- see Discussion question 1.
- This exercise instead uses `docker compose stop analytics` as the
  trigger: a clean, single-alert path (`ProbeDown` only -- see Discussion
  question 3 for why `AnalyticsStreamDown` does not also fire) with a
  short, predictable `for: 2m` window, ideal for a workshop-length loop.
  Because the trigger is a stopped container rather than the
  `loadgen-incident` k6 overlay, recovery is `docker compose start
  analytics`, not `make heal` (`make heal` only kills a running
  `loadgen-incident` container -- it has nothing to do with a stopped
  compose service, a different failure mode entirely).

## Prerequisites

```shell
make up-full   # core + analytics + synthetic + load
```

Confirm the alerting path is actually wired before breaking anything:

```shell
curl -sS http://localhost:9090/api/v1/rules | jq '.data.groups[].rules[] | select(.type=="alerting") | .name'
# expect: ProbeDown, CanaryJourneyFailing, CanaryPipelineLagHigh, AnalyticsStreamDown

curl -sS http://localhost:9093/-/healthy -w '\n%{http_code}\n'   # alertmanager: 200
curl -sS http://localhost:8025/livez -w '\n%{http_code}\n'       # mailpit: 200
curl -sS http://localhost:8082/readyz -w '\n%{http_code}\n'      # analytics: 200
```

## Steps

### 1. Open the panels

```promql
probe_success{instance=~"http://analytics:8082.*"}
up{job="analytics"}
analytics_stream_connected
ALERTS{alertname="ProbeDown"}
```

Also keep the Alertmanager UI (http://localhost:9093) and Mailpit
(http://localhost:8025) open in browser tabs.

### 2. Prove the "reality check" above live (optional but recommended)

```shell
INCIDENT_MODE=errors INCIDENT_MINUTES=3 make incident
```

Watch the Load dashboard's error-rate panel and
`slo:error_rate:actual_5xx_ratio7d` climb (Prometheus UI: this metric is
`0` throughout -- the errors are all 4xx, not 5xx, so even the *recording
rule* barely moves, let alone an alert). Confirm
http://localhost:9090/alerts shows no new alert firing during this
window. Stop it early:

```shell
make heal
```

### 3. Trigger the real path

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . stop analytics
```

### 4. Watch it fire

- `probe_success{instance=~"http://analytics:8082.*"}` -> `0` within one
  blackbox scrape interval.
- After `for: 2m`, http://localhost:9090/alerts shows `ProbeDown` firing
  for `instance="http://analytics:8082/healthz"`.
- Within `group_wait` (15s) of Prometheus forwarding it, the Alertmanager
  UI shows the alert and Mailpit (http://localhost:8025) shows a new
  message addressed to `oncall@devops-demo.local`.
- Read `docs/runbooks/probe-down.md` and follow its "First checks"
  against the running stack (the "is the profile up" check will say yes,
  this is a real target failure, not the documented degradation the
  runbook first rules out).

### 5. Silence it during triage

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . \
  exec alertmanager amtool silence add \
  alertname=ProbeDown 'instance="http://analytics:8082/healthz"' \
  --comment "exercise 04: triage in progress" \
  --duration=10m \
  --alertmanager.url=http://localhost:9093
```

Confirm it in the Alertmanager UI's Silences tab, or:

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . \
  exec alertmanager amtool silence query --alertmanager.url=http://localhost:9093
```

No new mail arrives while the silence is active, even though the alert
is still technically firing underneath it -- a silence suppresses
notification, it does not resolve the alert.

### 6. Heal

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . start analytics
```

### 7. Watch recovery

- `probe_success` back to `1` on the next scrape.
- `up{job="analytics"}` back to `1` (it never actually left `0` for long
  -- analytics itself only takes a few seconds to become ready again).
- `ProbeDown` moves from `firing` to resolved in Prometheus once
  `probe_success == 0` stops being true (no `for:` delay on the way down
  -- resolution is immediate once the condition clears).
- Mailpit receives a second mail: the resolved notification
  (`send_resolved: true` in `observability/alertmanager/alertmanager.yml`)
  -- open it and confirm the subject/body marks it resolved, not a
  duplicate firing notice.
- The silence you created in step 5 has nothing left to suppress once
  the alert resolves; it simply expires at its `--duration` (or delete it
  early via `amtool silence expire <id>` if you want to confirm cleanup
  explicitly).

## Timing expectations

| Event | Elapsed from trigger |
| ----- | --------------------- |
| `probe_success` drops to 0 | one scrape interval (`observability/prometheus.yml` `scrape_interval`) |
| `ProbeDown` starts firing | ~2m (`for: 2m`) |
| Mail appears in Mailpit | ~2m + `group_wait` (15s) |
| `ProbeDown` resolves | immediate on next scrape after `docker compose start analytics` completes readiness |
| Resolved mail appears | next Alertmanager notification cycle, typically within `group_wait` again |

## Cleanup

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . start analytics
docker compose -f deploy/compose/docker-compose.yml --project-directory . \
  exec alertmanager amtool silence query --alertmanager.url=http://localhost:9093
# expire any silence still listed from step 5:
# amtool silence expire <id> --alertmanager.url=http://localhost:9093
make down   # or: docker compose ... --profile "*" down
```

## Discussion questions

1. Sketch what a real SLO burn-rate alert on top of
   `slo:error_rate:error_budget_burn7d` would need: a single
   `> 0` threshold on a 7-day-window ratio pages far too slowly for a
   real incident (the error budget for 7 days barely moves in 3 minutes).
   What would a multi-window, multi-burn-rate rule (fast + slow window,
   both required) look like here, and why does that design page quickly
   on a real fast burn while staying quiet on ordinary day-to-day noise?
2. `make incident INCIDENT_MODE=errors` produces real, visible
   `http_req_failed` and offered-load signal in k6's own Prometheus
   metrics and dashboard, but moves none of this repo's four alerts and
   barely moves the SLO recording rules. Is `loadgen`'s error traffic
   (`409`/`422`/`404`) the wrong shape to exercise the *backend's own*
   error-rate SLO, or is the real gap the missing alert rule from
   question 1? Argue both, then say which fix you'd ship first.
3. Stopping `analytics` fires `ProbeDown` but not `AnalyticsStreamDown`,
   even though the WatchItemEvents stream is certainly down too. Re-read
   `AnalyticsStreamDown`'s expression
   (`analytics_stream_connected == 0 and on() (up{job="analytics"} == 1)`)
   from exercise 03's discussion question 1: which half of that
   expression makes it stay silent here, and why is that the correct
   call (which alert would otherwise double-fire for the same root
   cause)?
4. `send_resolved: true` means Alertmanager emails twice for one outage:
   firing, then resolved. The silence created in step 5 covered the
   firing period but was not needed for the resolved mail to still
   arrive. Would a silence with a *shorter* duration than the outage
   change what a triaging on-call engineer sees when the alert resolves
   mid-silence? Would a *longer* one (spanning past resolution) suppress
   the resolved mail too -- check the silence semantics (does Alertmanager
   suppress notifications for a matched alert regardless of its state,
   or only while it is firing) before answering.
