# Exercise: Find the seeded anomalies (Phase 5)

RFC-0001 Section 9 names this exercise directly: Phase 5's own example is
"find the three seeded anomalies". Phase 5 wires the historical seeder
(`analytics seed`, D5) and Grafana annotations into the platform; this
exercise closes the phase by putting a student in front of the Analytics
History dashboard with three real anomalies baked into the data and asking
them to find all three -- without being told upfront what they are called
or where they sit.

This exercise also runs in **workshop mode** (`DEMO_TIME_SCALE=24`,
`make up-workshop`): the same three-anomaly story compresses from a 90-day
seed into a few minutes, on a laptop, in one sitting.

## Objective

Bring up a workshop-scale stack, seed compressed history through it, then
use the Analytics History dashboard and direct SQL to identify all three
anomalies from `loadprofile/profile.json` -- a sharp spike, a hard outage,
and a gradual degradation -- before reading the reveal below. Along the
way, discover a genuine, live-verified limitation of running the seeder at
24x: two of the three anomalies compress into a window shorter than the
seeder's own sampling granularity and may leave no visible trace in the
hourly aggregate at all, even though Grafana's annotation layer marks all
three correctly regardless.

## Prerequisites

```shell
make up-workshop
DEMO_TIME_SCALE=24 SEED_DAYS=3 make seed-history
```

`--days` in `analytics seed` (`SEED_DAYS` here) counts real wall-clock days
of history, **not** profile-time days -- `DEMO_TIME_SCALE` does not shrink
it (`services/analytics/internal/seeder/seeder.go`: the seed window is
`[now - days*24h, now)` in real hours, full stop). All three
`profile.json` anomalies sit within the last 10 real hours before the
seed's reference time regardless of `--days` (their `offset_days` values
top out at `-10`, and `DEMO_TIME_SCALE=24` divides every offset by 24 --
see the reveal for the exact math), so `SEED_DAYS=3` is plenty of margin
and finishes in well under a minute. A full `SEED_DAYS=90` run at scale 24
would cost the same ~8-9 minutes a real-time run does
(`services/analytics/README.md`'s measured throughput) for zero extra
benefit at this scale: those extra 87 days of real history just seed a
period of the loadshape function so far in the past it stops looking like a
demo diurnal wave.

Confirm the seed completed at the scale the stack is running:

```shell
curl -sS http://localhost:8082/api/v1/seed-marker | jq
# expect: "scale": 24, "days": 3
```

If you want a guaranteed-clean run (no history from an earlier exercise
mixed in), reset the analytics volume first
(`services/analytics/README.md`'s "Full reset" guidance):

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . --profile analytics down -v
make up-workshop
DEMO_TIME_SCALE=24 SEED_DAYS=3 make seed-history
```

## Steps

1. Open Grafana (http://localhost:3000, admin/admin) -> Dashboards ->
   **Analytics History**.

2. The dashboard's default range is `now-30d -> now`, sized for a
   real-time (`scale=1`) 90-day seed. At `scale=24` with 3 seeded days, the
   interesting window is much narrower -- switch the time picker to **Last
   12 hours**.

3. **Before** touching the annotation toggle or hovering anything: look at
   the "Events per Hour by Type" bar panel and try to spot anomalies from
   the shape of the bars alone. Write down, roughly, how many hours before
   "now" each candidate starts and how long it seems to last. Do this for
   both `created` and `deleted` series (they move together).

4. Now look at the vertical markers: the dashboard's "Seeded anomalies"
   annotation layer (tag `seed-anomaly`, written by `analytics seed` at
   seed time) is on by default. Hover each one for its name and multiplier.
   Compare against what you wrote down in step 3 -- did the bars actually
   show all three, or only some of them?

5. Confirm directly against Postgres (bypassing Grafana's rendering
   entirely):

   ```shell
   docker compose -f deploy/compose/docker-compose.yml --project-directory . exec -T postgres-analytics \
     psql -U analytics -d analytics -c \
     "SELECT bucket_start, event_type, count FROM event_buckets
      WHERE bucket_start > now() - interval '12 hours'
      ORDER BY bucket_start, event_type;"
   ```

   And the annotations themselves, straight from Grafana's API (ground
   truth, independent of any dashboard rendering):

   ```shell
   curl -sS -u "${GRAFANA_USER:-admin}:${GRAFANA_PASSWORD:-admin}" "http://localhost:3000/api/annotations?tags=seed-anomaly" | jq '.[] | {text, time, timeEnd}'  # gitleaks:allow -- rule fires on the -u flag shape; no literal secret here, creds come from env with demo defaults
   ```

6. For each annotation, compute where its window *should* fall using
   `loadprofile/profile.json`'s `offset_days`/`duration_hours` for that
   anomaly and the seed marker's `ref_unix`, given `DEMO_TIME_SCALE=24`
   compresses everything around the reference time by dividing both the
   offset and the duration by the scale (this is `AnomalyRealWindow` in
   `services/analytics/internal/loadshape/loadshape.go`, inverted by hand):

   ```text
   real_start    = ref_unix + offset_days * 86400 / scale
   real_duration = duration_hours * 3600 / scale
   ```

   (`offset_days` values are negative, so the addition lands each window
   *before* the reference time -- `-10` days becomes `ref - 10h` at scale
   24.) Check your three computed windows against the annotation timestamps
   from step 5. They should match exactly (to the second).

## Expected observations -- the reveal

| Name | Type | Profile offset / duration | Real window at `scale=24` | Visible in the hourly bars? |
| --- | --- | --- | --- | --- |
| `traffic-spike` | spike (x4.0) | -10 days, 6 hours | `ref - 10h`, lasting 15 minutes | Usually not (see below) |
| `ingestion-outage` | outage (x0.0) | -6 days, 4 hours | `ref - 6h`, lasting 10 minutes | Usually not (see below) |
| `gradual-degradation` | degradation (ramp to x0.3) | -2 days, 48 hours | `ref - 2h` to `ref` (ends exactly at "now") | Yes, reliably |

This table is not a guess -- it is what three separate live seed runs
(different `--seed`, different reference times, same
`loadprofile/profile.json`) actually produced against a running stack:

- **`gradual-degradation` reliably shows up** as a real, measurable decline
  across the two hourly buckets its 2-hour real window spans
  (`round(rate * 3600)` events per hour, generated once per real hour --
  `services/analytics/internal/seeder/seeder.go`). One representative run
  measured a drop from ~18,100 events/hour just before the window to
  ~15,200 (roughly 25% into the ramp) and ~9,700 (roughly 70% into it,
  close to the ramp's x0.3 floor) -- a visible, sustained slope, exactly
  what "gradual" should look like.
- **`traffic-spike` and `ingestion-outage` usually do not show up at all**
  in the hourly aggregate at `scale=24`, even though the annotation marking
  them is exact. The reason is sampling granularity, not a bug: the seeder
  computes one rate sample per real *hour*
  (`bucketMid = hourStart + 30m`, `loadshape.Rate(bucketMid, ...)`), and at
  `scale=24` these two anomalies' real windows are only 15 and 10 minutes
  wide -- far shorter than the hour between samples. Whether any hour's
  30-minutes-past-the-hour sample point happens to land inside a 15- or
  10-minute window is close to a coin flip (roughly a 1-in-4 and 1-in-6
  chance per anomaly, depending on where the seed's reference second falls),
  and across three live runs it landed inside neither window a single time.
  Contrast this with `scale=1` (the default, real-time seed): there the
  same `ingestion-outage` anomaly's real window is a full 4 hours -- an
  exact multiple of the sampling interval -- which is why
  `services/analytics/README.md`'s own measured numbers report "exactly 4
  consecutive hours with zero rows". Same `profile.json`, same code path,
  two very different outcomes, purely a function of how the anomaly's
  duration compares to the bucket size at each scale.
- **The Grafana annotation is unaffected by any of this.** It is computed
  directly from `AnomalyRealWindow(anomaly, ref, scale)` -- the same
  offset/duration math, inverted -- independent of whatever the aggregated
  `event_buckets` rows happen to show. The annotation is always exactly
  right; the hourly bar chart is a lossy view of the underlying rate
  function, and at 24x compression two of the three anomalies fall below
  its resolution.

If you want to see `traffic-spike` and `ingestion-outage` reliably change
the shape of the bars, redo the setup at real time -- note this is NOT
`make up-workshop` (that target hardcodes `DEMO_TIME_SCALE=24`), the old
scale-24 seed marker must go first (a scale-1 `loadgen` would refuse to
start against it), and the seed needs at least 11 days so the `-10`-day
anomaly falls inside the window:

```shell
make down
docker compose -f deploy/compose/docker-compose.yml --project-directory . --profile analytics down -v
make up-full
SEED_DAYS=11 make seed-history
```

At real time both anomalies' windows equal their full profile-time duration
(6h and 4h), several times longer than the hourly sampling interval, and
both produce a clean, visible signature (the outage as zero-count hours, the
spike as a multi-hour surge).

## Cleanup

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . --profile "*" down
# or, to also drop the seeded data:
docker compose -f deploy/compose/docker-compose.yml --project-directory . --profile analytics down -v
```

## Discussion questions

1. The same `ingestion-outage` anomaly produces "exactly 4 consecutive
   hours with zero rows" at `scale=1` but *usually* no visible effect at
   `scale=24` (it showed none across the three documented runs), from the
   identical `profile.json` entry and identical seeder code.
   (a) State the general rule for which of `profile.json`'s anomalies stay
   visible at a given scale, as a relationship between
   `duration_hours / scale` and the seeder's one-sample-per-real-hour
   interval. (b) A retroactive alert on gradual decline (a recording rule
   comparing each hour's rate to a trailing baseline) could catch
   `gradual-degradation`. Explain why the *same* alerting approach cannot
   *reliably* catch `traffic-spike` or `ingestion-outage` at `scale=24`, no
   matter how the threshold is tuned, as long as the seeder samples once per
   real hour -- and why "reliably" is the right word (the anomaly can still
   be caught on the rare hour a sample happens to land inside its window).
2. `AnomalyRealWindow` inverts the same `profileTime = ref + (real-ref)*scale`
   mapping `loadshape.Rate` uses forward. Given that, why do all three
   anomalies' *wall-clock* positions shrink toward "now" as
   `DEMO_TIME_SCALE` increases, even though `offset_days` and
   `duration_hours` never change? Sketch what happens to
   `gradual-degradation`'s visibility specifically if the scale were pushed
   to 168 (one profile-week per real hour).
3. Hard rule 7 requires bucketing on event time, never arrival time.
   Suppose the seeder instead bucketed by arrival (wall-clock write) time --
   what would happen to the seam invariant this whole platform depends on,
   and would it make the two short anomalies *more* visible, *less*
   visible, or just wrong in a different way? Use RFC-0001 D1's own
   reasoning ("arrival-time bucketing would collapse 90 days of history
   into 'now'") rather than re-deriving it from scratch.
