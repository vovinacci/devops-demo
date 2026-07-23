package grafana

import (
	"fmt"
	"time"

	"github.com/vovinacci/devops-demo/services/analytics/internal/loadshape"
)

// AnomalyTag marks every seed-time anomaly annotation, both on write (one
// of the tags every annotation below carries) and on read (the tag
// ReplaceAnomalyAnnotations searches by before replacing stale rows from
// a prior seed run).
const AnomalyTag = "seed-anomaly"

// Annotation is one Grafana annotation to write for a seeded anomaly,
// already in the millisecond-epoch form the Grafana HTTP API expects.
type Annotation struct {
	TimeUnixMilli    int64
	TimeEndUnixMilli int64
	Tags             []string
	Text             string
}

// ComputeAnnotations derives one Annotation per profile anomaly, windowed
// in real wall-clock time via loadshape.AnomalyRealWindow -- the same
// window the seeder's own events actually land in (Hard rule 8,
// ADR-0003), not the profile-time window Rate evaluates internally. A
// workshop-mode seed (scale=24) therefore still annotates the true
// seeded event range, not a 24x-wider one.
func ComputeAnnotations(profile loadshape.Profile, refTime time.Time, scale float64) []Annotation {
	refUnix := float64(refTime.Unix())
	out := make([]Annotation, 0, len(profile.Anomalies))
	for _, a := range profile.Anomalies {
		startUnix, endUnix := loadshape.AnomalyRealWindow(a, refUnix, scale)
		out = append(out, Annotation{
			TimeUnixMilli:    int64(startUnix * 1000),
			TimeEndUnixMilli: int64(endUnix * 1000),
			Tags:             []string{AnomalyTag, a.Type},
			Text:             fmt.Sprintf("%s (seeded, x%.2f)", a.Name, a.Multiplier),
		})
	}
	return out
}
