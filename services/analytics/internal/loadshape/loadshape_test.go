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
