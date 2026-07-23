package loadshape

import (
	"strings"
	"testing"
)

// flatProfile isolates the diurnal term: no noise, no trend, no
// anomalies, all weekday coefficients 1.
const flatProfile = `{
  "base_rate_per_s": 10.0,
  "diurnal": {"amplitude": 0.5, "phase_hours": 14.0},
  "weekday_coefficients": [1, 1, 1, 1, 1, 1, 1],
  "noise_pct": 0.0,
  "trend": {"pct_per_day": 0.0},
  "anomalies": []
}`

func TestDiurnalPeaksAtPhaseHours(t *testing.T) {
	p, err := ParseProfile([]byte(flatProfile))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// 2026-01-05T00:00:00Z; phase_hours=14 -> peak at 14:00, trough at
	// 02:00. phase_hours is the documented UTC peak hour
	// (loadprofile/README.md) -- this is the regression guard for the
	// sin-vs-cos review finding (sin put the peak 6h late).
	ref := 1767571200.0
	atPeak := Rate(ref+14*3600, p, ref, 1)
	atTrough := Rate(ref+2*3600, p, ref, 1)
	if atPeak != 15.0 {
		t.Fatalf("rate at phase_hours: want 15.0 (base * (1+amplitude)), got %v", atPeak)
	}
	if atTrough != 5.0 {
		t.Fatalf("rate at phase_hours+12h: want 5.0 (base * (1-amplitude)), got %v", atTrough)
	}
}

func TestParseProfileRejectsWrongWeekdayCount(t *testing.T) {
	bad := strings.Replace(flatProfile, "[1, 1, 1, 1, 1, 1, 1]", "[1, 1, 1, 1, 1, 1]", 1)
	if _, err := ParseProfile([]byte(bad)); err == nil || !strings.Contains(err.Error(), "exactly 7") {
		t.Fatalf("want exactly-7 validation error, got %v", err)
	}
}

func TestParseProfileRejectsUnknownAnomalyType(t *testing.T) {
	bad := strings.Replace(flatProfile, `"anomalies": []`,
		`"anomalies": [{"name": "x", "type": "earthquake", "offset_days": -1, "duration_hours": 1, "multiplier": 2}]`, 1)
	if _, err := ParseProfile([]byte(bad)); err == nil || !strings.Contains(err.Error(), "unknown type") {
		t.Fatalf("want unknown-type validation error, got %v", err)
	}
}

func TestAnomalyRealWindowScaleOne(t *testing.T) {
	// scale=1: real time IS profile time, so the real window is exactly
	// [ref+offset_days, ref+offset_days+duration_hours] -- no compression.
	ref := 1767571200.0
	a := Anomaly{Name: "traffic-spike", Type: "spike", OffsetDays: -10, DurationHours: 6, Multiplier: 4.0}
	start, end := AnomalyRealWindow(a, ref, 1)
	wantStart := ref + a.OffsetDays*secondsPerDay
	wantEnd := wantStart + a.DurationHours*secondsPerHour
	if start != wantStart || end != wantEnd {
		t.Fatalf("scale=1 window: want [%v, %v], got [%v, %v]", wantStart, wantEnd, start, end)
	}
}

func TestAnomalyRealWindowScale24Compresses(t *testing.T) {
	// scale=24 (workshop mode, RFC-0001 D5): the SAME profile-time window
	// occupies 1/24th the real wall-clock span around ref -- this is the
	// window seeded events actually land in, since the seeder generates
	// events across real hours while Rate() evaluates them at compressed
	// profile time.
	ref := 1767571200.0
	a := Anomaly{Name: "ingestion-outage", Type: "outage", OffsetDays: -6, DurationHours: 4, Multiplier: 0.0}
	start, end := AnomalyRealWindow(a, ref, 24)
	wantStart := ref + (a.OffsetDays*secondsPerDay)/24
	wantEnd := ref + (a.OffsetDays*secondsPerDay+a.DurationHours*secondsPerHour)/24
	if start != wantStart || end != wantEnd {
		t.Fatalf("scale=24 window: want [%v, %v], got [%v, %v]", wantStart, wantEnd, start, end)
	}
	if got := end - start; got <= 0 || got >= a.DurationHours*secondsPerHour {
		t.Fatalf("scale=24 window duration: want shorter than %v (uncompressed), got %v",
			a.DurationHours*secondsPerHour, got)
	}
}
