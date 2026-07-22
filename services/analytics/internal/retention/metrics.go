package retention

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Retention metrics (RFC-0001 Phase 3 PR-C, ADR-0005 D7). No dedicated
// alert exists for this phase: a run failure surfaces via
// runs_total{result="failure"} plus last-success staleness on the
// dashboard instead (see services/analytics/README.md).
var (
	runsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_retention_runs_total",
		Help: "Retention job runs, by result (success|failure).",
	}, []string{"result"})

	deletedRowsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "analytics_retention_deleted_rows_total",
		Help: "Cumulative raw item_events rows deleted by the retention job.",
	})

	lastSuccessTimestampSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "analytics_retention_last_success_timestamp_seconds",
		Help: "Unix seconds of the start time of the last successful retention run.",
	})

	runDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "analytics_retention_run_duration_seconds",
		Help:    "Retention run wall-clock duration.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30},
	})
)
