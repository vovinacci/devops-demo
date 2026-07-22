// Package loadshape implements the shared load-shape function (RFC-0001
// D5, ADR-0003): the Phase 5 seeder and k6 (loadprofile/shape.js)
// evaluate the same rate-of-time function from the one checked-in
// loadprofile/profile.json, so seeded history and live traffic join
// without a visible seam. Deliberately dependency-free (stdlib only --
// no internal/pb) so it stays importable from both the seeder and any
// standalone golden-file generator.
//
// Duplicated by design in loadprofile/shape.js (Hard rule 8: profile +
// both impls + goldens change only together). Keep this file and
// shape.js line-for-line equivalent in *behavior* -- see
// loadprofile/README.md for the scale/noise semantics both must
// implement identically.
package loadshape

import (
	"encoding/json"
	"math"
)

const (
	secondsPerDay  = 86400.0
	secondsPerHour = 3600.0
)

// Diurnal is the daily traffic-wave component of Profile.
type Diurnal struct {
	// Amplitude is the fractional swing around base_rate_per_s (0.35 = +/-35%).
	Amplitude float64 `json:"amplitude"`
	// PhaseHours is the UTC hour of day (0-24) where the diurnal wave peaks.
	PhaseHours float64 `json:"phase_hours"`
}

// Trend is the long-run drift component of Profile.
type Trend struct {
	// PctPerDay is the linear fractional change per day since the
	// evaluation reference time (0.0015 = +0.15%/day). Linear, not
	// compounded -- keeps the two implementations trivially identical.
	PctPerDay float64 `json:"pct_per_day"`
}

// Anomaly is one story anomaly (spike, outage, or gradual degradation)
// embedded in the profile, active over [ref+offset_days, +duration_hours].
type Anomaly struct {
	Name string `json:"name"`
	// Type is one of "spike", "outage", "degradation".
	Type string `json:"type"`
	// OffsetDays is relative to the evaluation reference time; negative
	// is in the past (seeded history), positive is in the future.
	OffsetDays float64 `json:"offset_days"`
	// DurationHours is how long the anomaly window stays active.
	DurationHours float64 `json:"duration_hours"`
	// Multiplier is the factor applied: direct multiplier for spike/outage
	// (0 for outage), the ramp target for degradation (rate reaches
	// base*Multiplier linearly by the end of the window).
	Multiplier float64 `json:"multiplier"`
}

// Profile is the parsed form of loadprofile/profile.json -- the single
// checked-in load-shape definition (Hard rule 4: changes only with an
// ADR/RFC amendment, together with both impls and the goldens).
type Profile struct {
	BaseRatePerS float64 `json:"base_rate_per_s"`
	Diurnal      Diurnal `json:"diurnal"`
	// WeekdayCoefficients is Monday..Sunday (index 0 = Monday), each a
	// multiplicative factor on base_rate_per_s.
	WeekdayCoefficients [7]float64 `json:"weekday_coefficients"`
	// NoisePct bounds the deterministic pseudorandom noise factor
	// (0.05 = +/-5%); see noiseSample for the determinism contract.
	NoisePct  float64   `json:"noise_pct"`
	Trend     Trend     `json:"trend"`
	Anomalies []Anomaly `json:"anomalies"`
}

// ParseProfile parses loadprofile/profile.json bytes into a Profile.
func ParseProfile(data []byte) (Profile, error) {
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// Rate evaluates the shared load profile at unixSeconds, returning
// requests/second. refUnixSeconds is the evaluation reference time
// ("now" in RFC-0001 D5 terms) that diurnal/weekday/trend/anomaly
// offsets are all computed relative to; the seeder passes its seed
// time, k6 passes wall-clock time at runtime, and goldens pin a fixed
// value so both impls are deterministic in CI.
//
// scale (DEMO_TIME_SCALE) compresses profile time around
// refUnixSeconds: profileTime = ref + (unixSeconds-ref)*scale. Every
// periodic input (diurnal, weekday, trend, anomaly offsets) is computed
// from profileTime, never from unixSeconds directly -- this is what
// keeps history (seeded backwards from ref) and live traffic (evaluated
// forwards from ref) on the same shape across a scale change (Hard rule
// 8 / ADR-0003). Pass scale=1 for real time; loadprofile/shape.js
// defaults its scale parameter to 1 when omitted, Go requires it
// explicit since the language has no default parameters.
func Rate(unixSeconds float64, profile Profile, refUnixSeconds float64, scale float64) float64 {
	profileTime := refUnixSeconds + (unixSeconds-refUnixSeconds)*scale

	diurnalFactor := 1 + profile.Diurnal.Amplitude*math.Sin(
		2*math.Pi*(hourOfDay(profileTime)-profile.Diurnal.PhaseHours)/24,
	)
	weekdayFactor := profile.WeekdayCoefficients[mondayIndex(profileTime)]

	daysSinceRef := (profileTime - refUnixSeconds) / secondsPerDay
	trendFactor := 1 + profile.Trend.PctPerDay*daysSinceRef

	noiseFactor := 1 + profile.NoisePct*noiseSample(profileTime)

	anomaly := 1.0
	for _, a := range profile.Anomalies {
		anomaly *= anomalyFactor(a, profileTime, refUnixSeconds)
	}

	raw := profile.BaseRatePerS * diurnalFactor * weekdayFactor * trendFactor * noiseFactor * anomaly
	return math.Max(0, raw)
}

// hourOfDay returns the fractional UTC hour (0 <= h < 24) of profileTime.
func hourOfDay(profileTime float64) float64 {
	daysSinceEpoch := math.Floor(profileTime / secondsPerDay)
	secOfDay := profileTime - daysSinceEpoch*secondsPerDay
	return secOfDay / secondsPerHour
}

// mondayIndex returns the weekday index of profileTime (0 = Monday ..
// 6 = Sunday), computed from days-since-epoch arithmetic (no time.Time)
// so both impls do the same integer math. 1970-01-01 (day 0) was a
// Thursday -> Monday-index 3.
func mondayIndex(profileTime float64) int {
	daysSinceEpoch := int64(math.Floor(profileTime / secondsPerDay))
	idx := ((daysSinceEpoch%7)+3)%7 + 7
	return int(idx % 7)
}

// noiseSample returns a deterministic pseudorandom value in [-1, 1] for
// the 1-minute bucket containing profileTime. Integer ops only (no
// float transcendentals) so the hash is bit-identical across languages.
func noiseSample(profileTime float64) float64 {
	bucket := int64(math.Floor(profileTime / 60))
	h := splitmix64(uint64(bucket)) // bucket is non-negative for any post-epoch profileTime
	modded := h % 2000001
	return float64(modded)/1000000 - 1
}

// splitmix64 is the noise hash: a fixed-point splitmix64 mix, chosen for
// integer-only, easily-reproduced-in-any-language arithmetic. uint64
// wraps on overflow natively in Go, mirroring the explicit BigInt
// masking loadprofile/shape.js needs to get the same behavior in
// JavaScript.
func splitmix64(seed uint64) uint64 {
	z := seed + 0x9e3779b97f4a7c15
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	return z
}

// anomalyFactor returns the multiplicative factor anomaly a contributes
// at profileTime, given the reference time a.OffsetDays is relative to.
// 1 (no-op) outside the anomaly's [start, start+duration] window.
func anomalyFactor(a Anomaly, profileTime, refUnixSeconds float64) float64 {
	start := refUnixSeconds + a.OffsetDays*secondsPerDay
	end := start + a.DurationHours*secondsPerHour
	if profileTime < start || profileTime > end {
		return 1
	}
	switch a.Type {
	case "spike", "outage":
		return a.Multiplier
	case "degradation":
		duration := end - start
		frac := 1.0
		if duration > 0 {
			frac = (profileTime - start) / duration
		}
		return 1 + frac*(a.Multiplier-1)
	default:
		return 1
	}
}
