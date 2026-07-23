# loadgen

k6 shaped load generator and incident overlay (RFC-0001 D4, ADR-0006).
One long-running compose service (`loadgen`, profile `load`) plus a
one-shot incident script driven by `make incident` / `make heal`.

## Why k6

One tool serves both the always-on demo traffic and the CI performance
gate, so the demo and the gate cannot drift apart (ADR-0006). The stock
`grafana/k6:2.1.0` image already bundles everything this repo needs:

- HTTP (`k6/http`) and gRPC (`k6/net/grpc`) are core k6 modules, not
  extensions.
- The Prometheus remote-write output (`-o experimental-prometheus-rw`,
  `internal/output/prometheusrw` in k6's source) is built into the stock
  binary too -- the name still says "experimental", but as of k6 v2.x it
  is not an xk6 build-time extension the way it was in older k6
  releases. No custom xk6 image is built here.

## Reference time (`LOADGEN_REF_UNIX`)

The shared load profile (`loadprofile/`, RFC-0001 D5) requires an
explicit `refUnixSeconds` -- `shape.js` throws if it is omitted, by
design (see `loadprofile/README.md`). k6's scenarios are
`ramping-arrival-rate` executors whose `stages` array is precomputed
once, at script-init time (`loadgen/lib/schedule.js`), by evaluating
`rate()` at `refUnixSeconds + i*stepSeconds` for every step across the
run's duration -- never `Date.now()` inside the shape evaluation itself.

This matters beyond style: `scale` (`DEMO_TIME_SCALE`) compresses
profile time *around a fixed reference*
(`profileTime = ref + (unixSeconds - ref) * scale`). If the reference
moved with every evaluation (e.g. calling `Date.now()` again for each
stage), it would always equal the timestamp being evaluated, collapsing
`(unixSeconds - ref) * scale` to zero and silently defeating
`DEMO_TIME_SCALE` for live traffic.

Compose `environment:` entries are static strings -- they cannot
shell-interpolate `date +%s` themselves -- so `loadgen/entrypoint.sh`
supplies `LOADGEN_REF_UNIX` exactly once, at container start, before k6
loads any script:

```sh
: "${LOADGEN_REF_UNIX:=$(date +%s)}"
export LOADGEN_REF_UNIX
exec k6 "$@"
```

Overridable: set `LOADGEN_REF_UNIX` before starting the container to
reproduce a specific run. Running `k6` directly (outside the image)
needs `LOADGEN_REF_UNIX=$(date +%s)` (or `k6 run -e LOADGEN_REF_UNIX=...`)
exported first, or the script refuses to start.

Because the long-running scenario set precomputes its whole stage
schedule from one fixed reference at boot, `k6` naturally exits once
`LOADGEN_DURATION_HOURS` of stages complete -- `restart: unless-stopped`
on the compose service is what brings it back with a *fresh* reference,
not a loop inside the script.

## Scenarios (`scenarios/main.js`)

Weights are a share of the shared profile's `rate()` output
(requests/second), not independent rates:

Weights split the shared profile's arrival rate across scenario
*iterations*; an iteration is not one request (browse/crud make 2 HTTP
calls, grpc 2 invokes, expensive a batch), so the request-level mix
intentionally differs from the percentages below.

| Scenario    | Weight          | What it does                                                                                                                        |
| ----------- | --------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `browse`    | 60%             | `GET /items` (backend) + `GET /` (frontend)                                                                                         |
| `crud`      | 20%             | `POST /items` -> `DELETE /items/{id}`, same iteration -- keeps item count from growing unbounded over a long run                    |
| `abuse`     | 10%             | Three intentionally invalid requests, rotated by iteration: malformed JSON body, unknown web-vital name, delete of a nonexistent id |
| `grpc`      | 5%              | Unary `ListItems` + `GetItemStats` against `api:50051`, proto loaded at runtime                                                     |
| `expensive` | 5% base, bursty | Concurrent fan-out of `GET /items` (via `http.batch`), on a burst/idle cycle -- see "Honest limitations" below                      |
| `report`    | nightly/full    | `POST /reports` (rotate xlsx/pdf/csv) -> poll to terminal -> download; gated on `LOADGEN_REPORTS_URL` -- see below                  |

### Report scenario (`report`, RFC-0001 Phase 6 PR-3, D12)

The fifth scenario RFC-0001 D4 originally listed (report-trigger), landed
in Phase 6 once the `reports` JVM service existed. It drives the documented
async report API (`services/reports/README.md`): `POST /reports` with a
`{"type":"items-summary","format":...}` body rotating `xlsx`/`pdf`/`csv` by
iteration, expects `202` and reads the job id from the (case-insensitive)
`Location` header, polls `GET /reports/{id}` on a bounded ~30s loop until the
job is `SUCCEEDED` or `FAILED`, then `GET /reports/{id}/download` on success.

Two properties set it apart from the other five:

- **Gated on an enable-signal, not scheduled by default.**
  `LOADGEN_REPORTS_URL` (`lib/env.js`, `env.reportsEnabled`) has no default
  URL -- its *presence* is what adds `report` to `options.scenarios` in both
  `scenarios/main.js` and `scenarios/smoke.js`. The per-PR `make smoke` gate
  leaves it unset, so the JVM stays out of that gate entirely (RFC-0001 D12
  CI resource constraint); the nightly `make smoke-full` and local
  `make up-full` set it to `http://reports:8083` and enable the scenario.
- **A low fixed rate, not the shared diurnal profile.** Each iteration
  triggers a heavy async POI/PDF render on a bounded-concurrency JVM plus a
  poll loop that can hold its VU for ~30s, so it runs `constant-arrival-rate`
  at ~0.33 jobs/s (1 per 3s) -- enough to exercise the endpoint and drive the
  GC sawtooth on the Reports JVM dashboard, not to model report traffic.

`loadgen_report_job_failed` is its correctness signal: `add(true)` on a
non-202 submit, a `FAILED` job, a poll-timeout, or a non-200 download;
`add(false)` on a downloadable `SUCCEEDED`. A `loadgen_report_duration` Trend
records the accepted->succeeded wall-clock (observed, not gated). Both live
under the `loadgen_` prefix, like the abuse/gRPC signals; the threshold is in
the Thresholds section below.

Item names created by `crud` are prefixed `loadgen-` (distinct from the
canary's `canary-` prefix, RFC-0001 D9) so synthetic writes stay
filterable in business dashboards.

### Honest limitations

The backend has no artificially slow or unindexed query to hammer, so
`expensive` does not fabricate one. Instead it fires several concurrent
ordinary `GET /items` requests per iteration (`http.batch`, not a
sequential loop) on a periodic burst/idle cycle
(`EXPENSIVE_BURST_CYCLE` / `EXPENSIVE_BURST_MULTIPLIER` in
`scenarios/main.js`) -- real connection/queueing pressure produces a
genuine p99 tail, without claiming a query that isn't there.

Live traffic never actually experiences `profile.json`'s three story
anomalies (spike/outage/degradation): all three have negative
`offset_days`, meaning they are windows in the *past* relative to
whatever `refUnixSeconds` k6 captured at its own start -- they are the
Phase 5 seeder's history story, not a live-incident mechanism. One
edge case is visible on a fresh k6 start: the `gradual-degradation`
anomaly's window is `[ref-2d, ref-2d+48h]`, and `48h` is exactly `2d`,
so its window ends exactly at `ref` -- the very first stage (offset 0)
briefly reflects the tail of that anomaly (rates land near 30% of
normal for one 15-minute stage) before normal diurnal shape resumes.
This is a property of the shared, canonical `shape.js` (Hard rule 8:
not touched by this PR) and is cosmetic -- one stage out of many, at
every container start.

## Scale guard (`setup()`, RFC-0001 Phase 5 PR-2, D5 enforcement)

`scenarios/main.js` exports a `setup()` that runs once, before any
scenario/VU starts: `lib/scaleguard.js`'s `checkSeedMarker()` fetches
analytics' `GET /api/v1/seed-marker` (`LOADGEN_ANALYTICS_URL`, default
`http://analytics:8082`) and reasons over three outcomes:

| Outcome | Behavior |
| ------- | -------- |
| `200` + `marker.scale !== DEMO_TIME_SCALE` | **Refuse:** `setup()` throws (`seed scale X != DEMO_TIME_SCALE Y -- re-run \`make seed-history\` or match the env`), which aborts the entire k6 run before any traffic is generated. A scale mismatch silently breaks the seam invariant (ADR-0003 Hard rule 8): history seeded at one diurnal frequency, live traffic evaluated at another. |
| `404` | Analytics has never completed a seed run on this stack (fresh `make up`/`make up-full` with no `make seed-history` yet). Not an error: an info log, then continue -- there is nothing to be inconsistent with. |
| Connection error, timeout, or any other status | `analytics` is a separate opt-in compose profile from `load` (D10 graceful degradation): its absence (or a slow/broken response) must not brick `make up` with no analytics profile at all. A warning log, then continue. k6 does not throw on network-level failures (it returns `status: 0` with an `error` field), so this path is a status check, not a try/catch. |

The HTTP call is bounded to 2 seconds (`SEED_MARKER_TIMEOUT` in
`lib/scaleguard.js`): `setup()` blocks every scenario from starting until
it returns, so an unreachable analytics must not meaningfully delay
loadgen's own startup.

**Operational note, live-verified:** the guard only runs once, at `setup()`
time -- a long-running `loadgen` container that already passed the check
keeps running unaffected if `analytics` is later re-seeded at a
*different* scale (the seed-marker row changes underneath it, but nothing
re-triggers `setup()` on an already-running process). The mismatch only
surfaces the next time `loadgen` starts, e.g. after
`docker compose ... restart loadgen` -- and because the compose service
is `restart: unless-stopped`, a restart against a genuinely mismatched
marker crash-loops (`setup()` throws every time) until the scales are
reconciled. Do NOT leave a running `loadgen` straddling a
differently-scaled reseed: its live traffic keeps the OLD scale's shape
against the NEW seed's history, which is exactly the seam discontinuity
the guard exists to prevent (Hard rule 8). The procedure is: stop
`loadgen` first (`docker compose ... stop loadgen`), re-seed at the new
scale, then start it again only once the fresh seed marker matches the
`DEMO_TIME_SCALE` it will run with. `make up-workshop` avoids this by construction
-- it starts `loadgen` and `analytics` at the same `DEMO_TIME_SCALE=24`
together, so the very first seed after it already matches.

`scenarios/smoke.js` re-exports the same `setup` from `main.js` (not a
separate copy) -- this is what makes the CI/nightly e2e gate exercise the
guard's absent/never-seeded paths too, not just the long-running service.

## Thresholds (RFC-0001 D4 CI-gate contract)

Defined per-scenario via k6's automatic `scenario` tag, not as one
blanket threshold -- a global `http_req_failed` threshold would be
tripped by `abuse`'s intentional 4xx responses:

```text
http_req_duration{scenario:browse}     p(95)<300ms
http_req_duration{scenario:crud}       p(95)<300ms
http_req_duration{scenario:expensive}  p(95)<800ms   (looser: this scenario exists to produce a real tail)
http_req_failed{scenario:browse}       rate<1%
http_req_failed{scenario:crud}         rate<1%
http_req_failed{scenario:expensive}    rate<1%
grpc_req_duration{scenario:grpc}       p(95)<300ms
loadgen_grpc_call_failed               rate<1%   (custom Rate: grpc has no http_req_failed equivalent)
loadgen_abuse_unexpected_status        rate<1%   (custom Rate: did abuse get the status it was designed to provoke; transport failures excluded)
loadgen_report_job_failed              rate<5%   (custom Rate: async report job outcome; nightly/full only, see below)
```

`loadgen_report_job_failed` is gated at **5%**, looser than the 1%
pure-correctness signals: a report job *should* succeed, but it is an async
POI/PDF render on a bounded-concurrency JVM under nightly load with
best-effort analytics enrichment (RFC-0001 D10) and a bounded client-side
poll -- eventual-consistency slack that warrants a touch more tolerance than
the synchronous gates. It lives in the shared `lib/thresholds.js` object
unconditionally, which is safe even though the per-PR `make smoke` never
schedules `report`: a threshold whose metric receives zero samples is simply
not evaluated by k6 (same property that lets `smoke.js` skip `expensive`), so
the unset-URL per-PR run is unaffected. It is only ever exercised in the
nightly/full path (RFC-0001 D12).

`abuse` requests are marked EXPECTED via
`http.expectedStatuses(400, 404, 422)` (a k6 `responseCallback`) -- k6
does not count an expected 4xx in `http_req_failed` at all, so there is
deliberately no `http_req_failed{scenario:abuse}` threshold;
`loadgen_abuse_unexpected_status` is the scenario's real correctness
signal instead. It measures abuse-status correctness only -- a
transport-level failure under load (connection reset/timeout, where k6
reports `res.status === 0` because no HTTP response arrived) is not an
abuse-status divergence and is excluded from the rate; a genuine backend
outage is caught by the gated `http_req_failed{scenario:browse|crud}`
thresholds instead. These live in `lib/thresholds.js` (PR-4), imported by
both `scenarios/main.js` and `scenarios/smoke.js` so the CI/nightly gate
cannot drift from what the long-running service enforces on itself.

(422, not 400: `POST /items` binds its body straight into a Pydantic
model, so a malformed-JSON parse failure there is FastAPI's own
automatic request-validation error, not the backend's hand-written 400
-- that one only exists on `/metrics/frontend`, which parses the body
itself. Verified live against a running backend.)

## Incident mode (`make incident` / `make heal`)

`scenarios/incident.js` is a *separate* one-shot script, not a mode
switch inside `scenarios/main.js` -- the long-running loadgen keeps
running the normal shared-profile mix throughout; `make incident` layers
a second, temporary k6 process on top:

```shell
make incident                                  # INCIDENT_MODE=spike, 5 minutes
INCIDENT_MODE=errors INCIDENT_MINUTES=10 make incident
make heal                                       # from a different terminal: kills the overlay early
```

- `INCIDENT_MODE=spike` (default): `constant-arrival-rate` at
  `INCIDENT_SPIKE_MULTIPLIER`x (default 10) the shared profile's
  *current* rate, hammering `GET /items` -- an offered-load spike.
- `INCIDENT_MODE=errors`: a moderate, steady rate
  (`INCIDENT_ERROR_RATE_PER_S`, default 5/s) of requests rotated across
  a duplicate-name conflict (409), malformed JSON (422, FastAPI's
  automatic body validation), and a delete of a nonexistent id (404) --
  none marked expected, so they count against `http_req_failed` for real
  and move the Load dashboard's error-rate panel and the
  `slo:error_rate:*` recording rules (`observability/prometheus_slo_rules.yml`).
  Correction (found live during PR-4/exercise 04): all three are 4xx, so
  they do not move any *alert* today -- `observability/prometheus_alerts.yml`
  has no SLO burn-rate alert rule yet, only the four unrelated alerts
  (`ProbeDown`, `CanaryJourneyFailing`, `CanaryPipelineLagHigh`,
  `AnalyticsStreamDown`), and `blackbox_exporter`'s `probe_success` only
  checks reachability, which 4xx responses do not affect either. See
  `docs/exercises/04-incident-to-inbox.md` for the live-verified reality
  check and what a real SLO burn-rate alert would need.

`incident.js` has no thresholds: it exists to fail loudly, not to gate
anything. `make incident` runs the container under the fixed name
`loadgen-incident` (`docker compose run --rm --name loadgen-incident`);
`make heal` targets that exact name (`docker rm -f loadgen-incident`)
from any terminal, so it can stop an incident started elsewhere.

## CI/nightly smoke gate (`scenarios/smoke.js`, PR-4)

Another one-shot script, distinct from both `main.js` and `incident.js`:
`make smoke` (`.github/workflows/e2e.yml`) and `make smoke-full`
(`.github/workflows/nightly.yml`, longer `SMOKE_DURATION_SECONDS`) run it
against a freshly-brought-up stack as the CI/nightly e2e gate (RFC-0001
D12). It re-exports `browse`, `crud`, `abuse`, `grpcScenario`, and `report`
straight from `main.js` and imports the exact same `THRESHOLDS` from
`lib/thresholds.js` -- a short `constant-arrival-rate` run of the same
code, not a separate approximation of it. `expensive` is not exercised
here (its burst modulation is stage-index-driven, meaningless in a flat
single-stage run); its threshold key stays imported anyway since a
threshold with zero matching samples is simply not evaluated by k6, not
a failure. `report` is scheduled only when `LOADGEN_REPORTS_URL` is set
(the nightly/full path, RFC-0001 D12) -- per-PR `make smoke` leaves it
unset so the JVM stays out of that gate; its threshold, likewise
zero-sampled per-PR, is safe to keep imported unconditionally too. See `docs/ci.md` for what the surrounding workflow asserts
beyond the k6 thresholds (health endpoints, Prometheus targets, and, on
the nightly/full run only, canary + blackbox).

## Environment variables

| Variable | Default | Meaning |
| ---------------------------- | ---------------- | ------- |
| `LOADGEN_REF_UNIX` | (set by entrypoint) | RFC-0001 D5 evaluation reference; required |
| `DEMO_TIME_SCALE` | `1` | Shared profile time-scale (ADR-0003) |
| `LOADGEN_DURATION_HOURS` | `24` | Total wall-clock span the precomputed stage schedule covers before k6 exits |
| `LOADGEN_STAGE_MINUTES` | `15` | Stage granularity; 96 stages per scenario at the defaults |
| `LOADGEN_PREALLOCATED_VUS` | `10` | Per-scenario `preAllocatedVUs` |
| `LOADGEN_MAX_VUs` | `50` | Per-scenario `maxVUs` |
| `LOADGEN_BACKEND_URL` | `http://api:8000` | Backend REST base URL |
| `LOADGEN_WEB_URL` | `http://web:80` | Frontend base URL |
| `LOADGEN_GRPC_ADDR` | `api:50051` | Backend gRPC address |
| `LOADGEN_PROTO_DIR` | `/home/k6/proto` | gRPC `client.load()` import path |
| `LOADGEN_ANALYTICS_URL` | `http://analytics:8082` | Analytics base URL for `setup()`'s scale guard (Phase 5 PR-2), see Scale guard above |
| `LOADGEN_REPORTS_URL` | (unset) | Reports service base URL; **its presence is the enable-signal** for the `report` scenario (Phase 6 PR-3, D12). No default on purpose -- set only in nightly/full (`http://reports:8083`), unset per-PR so the JVM stays out of that gate. See Report scenario above |
| `INCIDENT_MODE` | `spike` | `spike` or `errors` (`incident.js` only) |
| `INCIDENT_MINUTES` | `5` | Incident duration (`incident.js` only) |
| `INCIDENT_SPIKE_MULTIPLIER` | `10` | Spike-mode rate multiplier (`incident.js` only) |
| `INCIDENT_ERROR_RATE_PER_S` | `5` | Errors-mode arrival rate (`incident.js` only) |
| `SMOKE_DURATION_SECONDS` | `75` | Per-scenario run length (`smoke.js` only); nightly.yml overrides to a longer run |

## Metrics (Prometheus remote-write)

`-o experimental-prometheus-rw` pushes to
`http://prometheus:9090/api/v1/write` (Prometheus requires
`--web.enable-remote-write-receiver`, set in
`deploy/compose/docker-compose.yml`). `K6_PROMETHEUS_RW_TREND_STATS`
is set to `p(95),p(99),avg`. All k6 metrics arrive prefixed `k6_`;
Trend metrics (e.g. `http_req_duration`) get one series per configured
stat (`k6_http_req_duration_p95`, `_p99`, `_avg`); Counters get `_total`
(`k6_http_reqs_total`); Rate metrics get `_rate`
(`k6_http_req_failed_rate`); Gauges are unsuffixed (`k6_vus`). Every k6
tag (including the automatic `scenario` tag) becomes a Prometheus label.
`observability/grafana/dashboards/load.json` queries these directly --
see that file for the exact expressions verified live against a running
stack.

## Local run (outside compose)

```shell
export LOADGEN_REF_UNIX=$(date +%s)
docker run --rm -e LOADGEN_REF_UNIX \
  -e LOADGEN_BACKEND_URL=http://host.docker.internal:8000 \
  -e LOADGEN_WEB_URL=http://host.docker.internal:8080 \
  -e LOADGEN_GRPC_ADDR=host.docker.internal:50051 \
  devops-demo-loadgen:latest run /home/k6/scenarios/main.js
```

Validate script syntax/derived options without running a load test:

```shell
docker run --rm devops-demo-loadgen:latest inspect \
  -e LOADGEN_REF_UNIX=1767571200 /home/k6/scenarios/main.js
```
