// Shared load-shape function (RFC-0001 D5, ADR-0003). Plain ES module --
// no bundler, no Node-only APIs -- so it imports unmodified into both k6
// (ESM support) and the Node parity runner (loadprofile/parity/compare.mjs).
//
// Duplicated by design in services/analytics/internal/loadshape/loadshape.go
// (Hard rule 8: profile + both impls + goldens change only together). Keep
// the two files line-for-line equivalent in *behavior* -- see
// loadprofile/README.md for the scale/noise semantics both must implement
// identically.

const SECONDS_PER_DAY = 86400;
const SECONDS_PER_HOUR = 3600;

// 64-bit constants for the splitmix64 noise hash, as BigInt (Number
// cannot hold 64-bit integers exactly). Masked to 64 bits after every
// operation since BigInt does not wrap on its own -- Go's uint64 does,
// so this mirrors that behavior explicitly.
const MASK64 = (1n << 64n) - 1n;
const GOLDEN_GAMMA = 0x9e3779b97f4a7c15n;
const MIX1 = 0xbf58476d1ce4e5b9n;
const MIX2 = 0x94d049bb133111ebn;

function splitmix64(seed) {
  let z = (seed + GOLDEN_GAMMA) & MASK64;
  z = ((z ^ (z >> 30n)) * MIX1) & MASK64;
  z = ((z ^ (z >> 27n)) * MIX2) & MASK64;
  z = z ^ (z >> 31n);
  return z & MASK64;
}

// noiseSample returns a deterministic pseudorandom value in [-1, 1] for
// the 1-minute bucket containing profileTime. Integer ops only (no
// float transcendentals) so the hash is bit-identical across languages.
function noiseSample(profileTime) {
  const bucket = BigInt(Math.floor(profileTime / 60));
  const h = splitmix64(bucket);
  const modded = h % 2000001n;
  return Number(modded) / 1000000 - 1;
}

// mondayIndex returns the weekday index of profileTime (0 = Monday ..
// 6 = Sunday), computed from days-since-epoch arithmetic (no Date
// object) so both impls do the same integer math. 1970-01-01 (day 0)
// was a Thursday -> Monday-index 3.
function mondayIndex(profileTime) {
  const daysSinceEpoch = Math.floor(profileTime / SECONDS_PER_DAY);
  return (((daysSinceEpoch % 7) + 3) % 7 + 7) % 7;
}

// hourOfDay returns the fractional UTC hour (0 <= h < 24) of profileTime.
function hourOfDay(profileTime) {
  const daysSinceEpoch = Math.floor(profileTime / SECONDS_PER_DAY);
  const secOfDay = profileTime - daysSinceEpoch * SECONDS_PER_DAY;
  return secOfDay / SECONDS_PER_HOUR;
}

// anomalyFactor returns the multiplicative factor anomaly `a` contributes
// at profileTime, given the reference time the anomaly's offset_days is
// relative to. 1 (no-op) outside the anomaly's [start, start+duration]
// window.
function anomalyFactor(a, profileTime, refUnixSeconds) {
  const start = refUnixSeconds + a.offset_days * SECONDS_PER_DAY;
  const end = start + a.duration_hours * SECONDS_PER_HOUR;
  if (profileTime < start || profileTime > end) {
    return 1;
  }
  switch (a.type) {
    case "spike":
      return a.multiplier;
    case "outage":
      return a.multiplier;
    case "degradation": {
      const duration = end - start;
      const frac = duration > 0 ? (profileTime - start) / duration : 1;
      return 1 + frac * (a.multiplier - 1);
    }
    default:
      return 1;
  }
}

// rate evaluates the shared load profile at unixSeconds, returning
// requests/second. `profile` is the parsed loadprofile/profile.json
// object. `refUnixSeconds` is the evaluation reference time ("now" in
// RFC-0001 D5 terms) that diurnal/weekday/trend/anomaly offsets are all
// computed relative to; k6 passes wall-clock time at runtime, the Go
// seeder passes its seed time, and goldens pin a fixed value so both
// impls are deterministic in CI.
//
// scale (DEMO_TIME_SCALE) compresses profile time around refUnixSeconds:
// profileTime = ref + (unixSeconds - ref) * scale. Every periodic input
// (diurnal, weekday, trend, anomaly offsets) is computed from profileTime,
// never from unixSeconds directly -- this is what keeps history (seeded
// backwards from ref) and live traffic (evaluated forwards from ref) on
// the same shape across a scale change (Hard rule 8 / ADR-0003).
export function rate(unixSeconds, profile, { scale = 1, refUnixSeconds } = {}) {
  const ref = refUnixSeconds;
  const profileTime = ref + (unixSeconds - ref) * scale;

  const diurnalFactor =
    1 +
    profile.diurnal.amplitude *
      Math.sin((2 * Math.PI * (hourOfDay(profileTime) - profile.diurnal.phase_hours)) / 24);

  const weekdayFactor = profile.weekday_coefficients[mondayIndex(profileTime)];

  const daysSinceRef = (profileTime - ref) / SECONDS_PER_DAY;
  const trendFactor = 1 + profile.trend.pct_per_day * daysSinceRef;

  const noiseFactor = 1 + profile.noise_pct * noiseSample(profileTime);

  let anomaly = 1;
  for (const a of profile.anomalies) {
    anomaly *= anomalyFactor(a, profileTime, ref);
  }

  const raw = profile.base_rate_per_s * diurnalFactor * weekdayFactor * trendFactor * noiseFactor * anomaly;
  return Math.max(0, raw);
}
