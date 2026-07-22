# loadprofile

The one checked-in load-shape definition (RFC-0001 D5, ADR-0003): base
rate, diurnal wave, weekly pattern, drift, deterministic noise, and story
anomalies, evaluated as a single function of time by two independent
consumers -- the Go seeder (`services/analytics/internal/loadshape`,
history, RFC-0001 Phase 5) and k6 (`shape.js`, live traffic, Phase 6).
Both must produce the same rate for the same timestamp or the
`now-30d -> now+1h` dashboard panel shows a visible seam where synthetic
history ends and live traffic begins.

**Hard rule 8 (seam invariant):** `profile.json`, `shape.js`,
`services/analytics/internal/loadshape/loadshape.go`, and the goldens
under `parity/` change only together, in the same commit. Changing the
shape or the profile without regenerating the goldens is exactly the
drift this file exists to prevent -- CI's parity gate
(`.github/workflows/parity.yml`) fails on any of the four drifting from
the other three.

## Schema (`profile.json`)

| Field                        | Type              | Meaning                                                                                   |
| ---------------------------- | ----------------- | ----------------------------------------------------------------------------------------- |
| `base_rate_per_s`            | number            | Baseline requests/second before any other factor is applied.                              |
| `diurnal.amplitude`          | number (fraction) | Daily-wave swing around the base rate, e.g. `0.35` = +/-35%.                              |
| `diurnal.phase_hours`        | number (0-24)     | UTC hour of day where the diurnal wave peaks.                                             |
| `weekday_coefficients`       | number[7]         | Multiplicative factor per weekday, **index 0 = Monday .. index 6 = Sunday**.              |
| `noise_pct`                  | number (fraction) | Bound on the deterministic pseudorandom noise factor, e.g. `0.05` = +/-5%.                |
| `trend.pct_per_day`          | number (fraction) | Linear (not compounded) drift per day since the evaluation reference time.                |
| `anomalies[].name`           | string            | Human-readable label (used in Grafana annotations at seed time, Phase 5).                 |
| `anomalies[].type`           | string            | `spike`, `outage`, or `degradation` -- see below.                                         |
| `anomalies[].offset_days`    | number            | Anomaly start, in days relative to the evaluation reference time. Negative = in the past. |
| `anomalies[].duration_hours` | number            | How long the anomaly window stays active, starting at `offset_days`.                      |
| `anomalies[].multiplier`     | number            | Direct multiplier for `spike`/`outage` (`0` for outage); ramp target for `degradation`.   |

Anomaly types:

- **spike**: rate is multiplied by `multiplier` for the whole window (e.g. `4.0` = 4x traffic).
- **outage**: rate is multiplied by `multiplier` (`0.0`) for the whole window -- a hard gap.
- **degradation**: rate ramps *linearly* from the normal rate (factor 1) at
  the start of the window to `base * multiplier` at the end (e.g.
  `multiplier: 0.3` = down to 30% of normal by the end of the ramp) --
  a gradual, not instant, decline.

The three anomalies in `profile.json` are the Phase 5 history story
(RFC-0001 D5): one spike day, one outage gap, one gradual degradation.

## Rate function

Both `shape.js` (`rate(unixSeconds, profile, { scale, refUnixSeconds })`)
and `loadshape.go` (`Rate(unixSeconds, profile, refUnixSeconds, scale)`)
compute:

```text
profileTime = refUnixSeconds + (unixSeconds - refUnixSeconds) * scale

rate = base_rate_per_s
     * (1 + diurnal.amplitude * sin(2*pi * (hourOfDay(profileTime) - diurnal.phase_hours) / 24))
     * weekday_coefficients[mondayIndex(profileTime)]
     * (1 + trend.pct_per_day * (profileTime - refUnixSeconds) / 86400)
     * (1 + noise_pct * noiseSample(profileTime))
     * product(anomalyFactor(a, profileTime, refUnixSeconds) for a in anomalies)

rate = max(0, rate)
```

`hourOfDay` and `mondayIndex` are computed from integer seconds-since-epoch
arithmetic (not `Date`/`time.Time` calendar objects) so both languages do
provably the same math: `daysSinceEpoch = floor(profileTime / 86400)`,
`mondayIndex = ((daysSinceEpoch % 7) + 3) % 7` (1970-01-01, day 0, was a
Thursday, hence the `+3` to land on Monday-indexed 0).

### `refUnixSeconds` -- the evaluation reference time

`refUnixSeconds` stands in for RFC-0001 D5's "now": the seeder passes its
seed time (history is generated *backwards* from it, negative
`offset_days` and negative `daysSinceRef` are the past), k6 passes the
current wall-clock time at each evaluation (`Date.now() / 1000`, live
traffic evaluates near `daysSinceRef = 0`), and the goldens pin a fixed
value so CI is deterministic. `shape.js` requires it as a named option
(no default -- there is no sane default reference time); `loadshape.go`
takes it as a plain parameter for the same reason.

### `scale` -- `DEMO_TIME_SCALE` semantics

`scale` compresses profile time around `refUnixSeconds`:

```text
profileTime = refUnixSeconds + (unixSeconds - refUnixSeconds) * scale
```

Every periodic input -- diurnal phase, weekday, trend, anomaly windows --
is computed from `profileTime`, **never** from `unixSeconds` directly.
At `scale = 1` (the default), profile time and wall-clock time are
identical. At `scale = 24` (`make up-workshop`, "one day = one hour"),
one hour of wall-clock time sweeps through 24 hours of profile time
around the reference point -- the diurnal wave, weekday changes, and
trend/anomaly windows all run 24x faster, symmetrically on either side
of `refUnixSeconds`.

This is why `refUnixSeconds` has to be a parameter, not "now" baked into
the function: the seeder evaluates at negative offsets from its own seed
time (`refUnixSeconds` = seed time) while live k6 evaluates at
near-zero offsets from wall-clock time (`refUnixSeconds` = each call's
own `Date.now()`) -- both must warp identically around their own
reference for the seam at the join point to be shapeless. `shape.js`
defaults `scale` to `1` when the option is omitted; `loadshape.go` has no
default parameters, so callers pass `1.0` explicitly for real time.

Goldens are generated and checked at **both** `scale=1` and `scale=24` so
this definition, once pinned, cannot silently drift.

### Noise determinism

`noise_pct` bounds a deterministic pseudorandom factor -- deterministic
because the goldens must be exact, and because a live demo restarted
mid-run must replay the same noise at the same profile-time bucket
(this is the same seam-invariant reasoning as everything else here: any
process reading the same `profileTime` must get the same rate).

The noise sample for `profileTime` is:

1. `bucket = floor(profileTime / 60)` -- one bucket per minute of profile
   time.
2. `h = splitmix64(bucket)` -- a fixed-point [splitmix64](https://prng.di.unimi.it/splitmix64.c)
   mix (integer XOR/shift/multiply only, no floating-point transcendentals,
   so it hashes identically regardless of host FPU/libm): add the golden-ratio
   constant, then two rounds of `(z ^ (z >> shift)) * mixConstant`, then a
   final XOR-shift.
3. `noiseSample = (h % 2000001) / 1000000 - 1` -- maps the hash into
   `[-1, 1]` at roughly 1e-6 resolution.
4. `noiseFactor = 1 + noise_pct * noiseSample`.

Go's `uint64` wraps on overflow natively; `shape.js` uses `BigInt` with
an explicit `& ((1n << 64n) - 1n)` mask after every operation to get the
same wraparound behavior, since JavaScript `BigInt` does not wrap on its
own. Both implementations must keep `splitmix64` integer-only -- adding
any `Math.sin`/`math.Sin`-style call to the hash path would reintroduce
the exact cross-language ULP drift the noise hash exists to avoid.

## Parity goldens (`parity/`)

`golden-scale1.csv` and `golden-scale24.csv` are `unix_ts,rate` CSVs
(rate formatted `%.9f`) computed by the **Go implementation**
(canonical) over a fixed grid: every 30 minutes for the 14 days up to
and including `refUnixSeconds = 1767571200` (2026-01-05T00:00:00Z, a
Monday) -- 673 rows each. The window is chosen to land inside all three
`profile.json` anomalies (`offset_days` -10, -6, -2, all within the last
14 days before the reference time) as well as every weekday and at
least one full diurnal cycle; `TestGoldenGridSanity` in
`services/analytics/internal/loadshape/parity_test.go` fails the build if
a future anomaly-offset change moves an anomaly window outside the grid.

Two parity checks run in CI (`.github/workflows/parity.yml`) and via
`make parity`:

- **Go** (`parity_test.go`, `TestParityGoldens`): recomputes the grid
  with `loadshape.Rate` and compares against both goldens **exactly**
  (string-equal at the goldens' own `%.9f` formatting) -- since Go
  generated them, any mismatch means the goldens are stale relative to
  the current `profile.json`/`loadshape.go`.
- **JS** (`parity/compare.mjs`): recomputes the grid with `shape.js` and
  compares against both goldens within a **relative tolerance of
  `1e-9`** -- `Math.sin`/`Math.exp` in V8 and `math.Sin`/`math.Exp` in Go
  can differ by a few ULP on the same input even though both are
  correctly rounded, and exact-equality would make the gate flaky, not
  meaningful.

### Regenerating goldens

Goldens are regenerated **only** deliberately, as part of a reviewed
`profile.json`/`loadshape.go`/`shape.js` change (Hard rule 8 -- never
silently, never by CI):

```shell
cd services/analytics
REGEN_GOLDENS=1 go test ./internal/loadshape/... -run TestGenerateGoldens
```

This overwrites `parity/golden-scale1.csv` and `parity/golden-scale24.csv`
from the current Go implementation. Review the diff like any other
change to a checked-in contract, then run `make parity` to confirm the
JS side still agrees within tolerance.

## Local run

```shell
make parity   # node loadprofile/parity/compare.mjs + go test ./internal/loadshape/...
```
