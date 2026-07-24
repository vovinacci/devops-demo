# Exercise: Break the event stream (Phase 3)

RFC-0001 Section 9 calls this out explicitly: "break the event stream,
watch which monitoring layer fires first". Phase 3 wires the gRPC ingest
client (Phase 3 PR-B, ADR-0002) into the three-layer monitoring stack
(ADR-0007) built in Phases 1-2. This exercise breaks the
backend-to-analytics event stream on purpose and watches four signals --
whitebox, blackbox, the new `AnalyticsStreamDown` alert, and the synthetic
canary -- notice in a specific order, for different reasons, and recover in
a specific order too. It is the multi-service sequel to exercise 01.

## Objective

Disconnect analytics from the backend while both are running, observe the
ordering of `up{job="api"}`, `ProbeDown`, `AnalyticsStreamDown`, and the
canary journey, then reconnect and confirm the reconcile-not-replay lesson
from ADR-0002: the event-count aggregate dip for the outage window stays
visible permanently -- it does not get backfilled.

## Prerequisites

```shell
make up-full   # core + analytics + synthetic (+ reports, load if present)
```

Confirm the relevant layers are actually up before breaking anything:

```shell
curl -sS http://localhost:8082/readyz                       # analytics: DB-only, should be 200
curl -sS http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, health}'
curl -sS http://localhost:9090/api/v1/rules | jq '.data.groups[] | select(.name=="analytics_ingest")'
```

Create a baseline item so there is at least one row analytics has already
reconciled and can poll for later:

```shell
curl -sS -X POST http://localhost:8000/items -H "Content-Type: application/json" \
  -d '{"name": "stream-exercise-item"}' | tee /tmp/item.json
ID=$(python3 -c "import json; print(json.load(open('/tmp/item.json'))['id'])")
curl -sS "http://localhost:8082/api/v1/items/$ID" | jq
```

## Steps

1. Open the five queries below side by side (Grafana or the Prometheus UI)
   so you can watch them update together:

   ```promql
   up{job="api"}
   probe_success{instance=~"http://api:8000.*"}
   analytics_stream_connected
   canary_journey_total
   canary_pipeline_check_total
   ```

2. Break the connection. Either works; Option B is closer to a real
   network partition than a full process outage:

   ```shell
   # Option A: stop the backend entirely
   docker compose -f deploy/compose/docker-compose.yml --project-directory . stop api

   # Option B: leave the backend up, cut analytics off from it only
   docker network disconnect devops-demo_devnet analytics
   ```

3. Watch the queries. Note the wall-clock time each one first shows the
   break:
   - `up{job="api"}` (Option A only) -- near-immediate, one missed scrape.
   - `probe_success` for the `api` blackbox target (Option A only) --
     similar timescale, confirmed from outside the process.
   - `analytics_stream_connected` -- drops to `0` as soon as the current
     `Recv()` on the stream errors (within moments of either option).
   - `canary_journey_total{result="failure"}` -- Option A only: with the
     backend down, `create` itself fails. Option B leaves the backend up,
     so all backend calls keep succeeding and this counter never shows a
     failure during the outage.
   - `canary_pipeline_check_total{result="skipped"}` -- the signal Option
     B *does* produce. The disconnect cuts analytics off from the canary
     too, so the pipeline-lag step's first poll gets a connection failure
     and records `skipped` on the canary's next journey -- detected on the
     first poll, well inside one journey interval and far faster than
     `AnalyticsStreamDown`'s 5-minute window. Note the honest limitation:
     the canary cannot tell "analytics profile not up" from "analytics
     partitioned away" -- both are connect failures (a `timeout` result
     would instead mean analytics answered but the item never appeared,
     e.g. only the gRPC stream is broken). Per Hard rule 9 (ADR-0008 D10)
     neither fails the journey (`canary_journey_total` stays all
     `success`): a strictly finer-grained signal than journey
     success/failure, visible only in this counter and in
     `canary_pipeline_lag_seconds` gaining no new samples.

4. Confirm `analytics` itself stays healthy throughout (graceful
   degradation, RFC-0001 D10 -- an ingest outage must not take the HTTP
   surface down):

   ```shell
   curl -sS http://localhost:8082/readyz -w '\n%{http_code}\n'   # still 200
   curl -sS http://localhost:8082/metrics | grep -E 'analytics_(stream_connected|reconnects_total)'
   ```

   `analytics_reconnects_total` should be climbing -- the backoff loop is
   actively retrying, not stuck.

5. While the stream is down, create a couple more items against the
   backend (Option B only -- Option A has no backend to call):

   ```shell
   curl -sS -X POST http://localhost:8000/items -H "Content-Type: application/json" \
     -d '{"name": "lost-during-outage"}'
   ```

   These events are emitted by the backend with zero consumers connected
   -- per ADR-0002 they are simply unobserved, gone the moment they are
   published. This is the point of the exercise: not a bug to route
   around, but the documented at-most-once transport gap the NATS capstone
   (RFC-0001 Section 10) exists to fix.

6. After 5 minutes, confirm `AnalyticsStreamDown` is `firing` (its
   `for: 5m` window has elapsed) at http://localhost:9090/alerts, and read
   `docs/runbooks/analytics-stream-down.md`'s "First checks" against the
   running stack.

7. Reconnect:

   ```shell
   # Option A
   docker compose -f deploy/compose/docker-compose.yml --project-directory . start api
   # Option B
   docker network connect devops-demo_devnet analytics
   ```

8. Watch the recovery, in order: `analytics_stream_connected` back to `1`,
   `analytics_snapshot_reconciles_total` incrementing by exactly one (the
   reconnect sequence always re-snapshots before resuming the stream,
   ADR-0002), then `up`/`probe_success`/`canary_journey_total` recovering
   on their own schedules.

9. Confirm the item created during the outage (step 5, Option B) is now
   visible via a fresh snapshot reconcile, even though its `created` event
   was never streamed:

   ```shell
   curl -sS http://localhost:8000/items | jq '[.[] | select(.name == "lost-during-outage")]'
   NEW_ID=$(curl -sS http://localhost:8000/items | jq '[.[] | select(.name=="lost-during-outage")][0].id')
   curl -sS "http://localhost:8082/api/v1/items/$NEW_ID" | jq
   ```

   The item is `known` (state was recovered by the snapshot), but check
   `/api/v1/stats` -- the `created` event count for that hour will be one
   short of what it should be, permanently.

## Expected observations

- The four signals fire in a strict order because they measure different
  things: whitebox notices the process is unreachable, blackbox confirms
  it from outside, `AnalyticsStreamDown` notices the *consumer's*
  connection specifically (and only when analytics itself is otherwise
  healthy -- under Option B, `up{job="api"}`/`ProbeDown` never fire at
  all, since the backend never went down), and the canary is slowest
  because it only samples on its own schedule.
- With canary v2's pipeline-lag step, "the canary" is no longer one signal
  but two, and they diverge under Option B specifically:
  `canary_journey_total` (the CRUD journey against the backend) stays all
  `success` throughout, while `canary_pipeline_check_total{result="skipped"}`
  fires within one journey interval. This is the concrete illustration of
  Hard rule 9 (ADR-0008 D10): the pipeline-lag step is deliberately
  decoupled from the journey's own success/failure verdict, so a real,
  measurable pipeline problem is visible without ever making the canary
  itself look "down".
- `analytics_reconnects_total` climbs throughout the outage -- the client
  never gives up, it backs off and retries (ADR-0002), bounded by
  `ANALYTICS_INGEST_BACKOFF_MAX` (default 30s).
- `/readyz` on analytics never flips to `503` during any of this -- stream
  health is a metrics/alerting concern, not a readiness concern (ADR-0005),
  the same discipline the canary's own `/readyz` follows (exercise 01).
- The reconnect sequence always does a `ListItems` snapshot reconcile
  *before* resuming the stream -- state is recovered (the item created
  during the outage becomes known), but the *event* and its bucket
  increment are not. This is D3's central lesson: snapshot recovers state,
  not history, and the aggregate dip for the outage window is permanent,
  not a temporary lag that catches up.

## Cleanup

```shell
docker network connect devops-demo_devnet analytics 2>/dev/null || true
docker compose -f deploy/compose/docker-compose.yml --project-directory . start api
make down   # or: docker compose ... --profile "*" down
```

Confirm no exercise items were left behind:

```shell
curl -sS http://localhost:8000/items | jq '[.[] | select(.name | test("stream-exercise-item|lost-during-outage"))]'
```

## Discussion questions

1. `AnalyticsStreamDown`'s expression is
   `analytics_stream_connected == 0 and on() (up{job="analytics"} == 1)`.
   (a) Why does it need the `up{job="analytics"} == 1` half at all -- what
   would go wrong (which alert would double-fire, or which real outage
   would it miss) without it? (b) Option A and Option B produce identical
   `analytics_stream_connected`/`AnalyticsStreamDown` behavior but
   different whitebox/blackbox behavior. If you had *only* this alert (no
   whitebox/blackbox layer at all), could you tell the two options apart
   from inside analytics alone, and what would you have to add to?
2. The exercise shows the aggregate dip never gets backfilled. Sketch, at
   a high level, what a transactional outbox (RFC-0001 Section 10, the
   NATS capstone) would have to add on the backend side to make
   emitted-but-unobserved events recoverable, and why that requires a
   durable broker rather than a smarter analytics-side retry.
3. `CanaryPipelineLagHigh` requires at least one `result="ok"` sample in
   its window to fire at all -- during this outage checks are all
   `skipped`, so it never fires; only `AnalyticsStreamDown` does. Is that
   the right call, or should a canary pipeline-lag alert be able to fire on
   sustained `timeout` or `skipped` results too? Argue both sides using
   what `AnalyticsStreamDown` already covers.
