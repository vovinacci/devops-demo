package ingest

import (
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Ingest/stream metrics (RFC-0001 Phase 3 PR-B). streamConnected and
// lastEventTimeSeconds are the degenerate single-source watermark
// (ADR-0005 D1): stream liveness plus last-received-event-time
// approximates "completeness" since buckets are mutable upserts with no
// real watermark machinery -- the same signal the canary's pipeline-lag
// metric measures from the other end.
var (
	streamConnected = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "analytics_stream_connected",
		Help: "1 if the WatchItemEvents stream is currently connected, 0 otherwise.",
	})

	lastEventTimeSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "analytics_last_event_time_seconds",
		Help: "Unix seconds of the event_time of the most recently ingested stream event " +
			"(the degenerate watermark, ADR-0005 D1) -- not updated by snapshot reconcile.",
	})

	// event_type is bounded (created|updated|deleted, the item_events
	// CHECK constraint's own domain), never free text.
	eventsIngestedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_events_ingested_total",
		Help: "Stream events landed into item_events, by event_type.",
	}, []string{"event_type"})

	eventsDeduplicatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "analytics_events_deduplicated_total",
		Help: "Stream events dropped as duplicates by the item_events ON CONFLICT DO NOTHING guard.",
	})

	reconnectsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "analytics_reconnects_total",
		Help: "Reconnect attempts after a lost connection or stream error (ADR-0002 backoff loop).",
	})

	snapshotReconcilesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "analytics_snapshot_reconciles_total",
		Help: "Completed ListItems snapshot reconciles (initial connect + every reconnect).",
	})

	snapshotItems = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "analytics_snapshot_items",
		Help: "Item count returned by the most recent ListItems snapshot.",
	})
)

// lastEventTimeUnix guards lastEventTimeSeconds: a watermark must only
// ever advance. Stream events are consumed by a single writer goroutine
// in practice, but a plain compare-and-swap costs nothing and stays
// correct if that assumption ever changes. Without it, a stream event
// arriving out of order (or, once the Phase-5 seeder exists, seeded
// history landing well in the past) would yank the gauge backwards,
// falsely reporting the pipeline as caught up.
var lastEventTimeUnix atomic.Int64

// advanceLastEventTime updates lastEventTimeSeconds only if t is newer
// than the current watermark.
func advanceLastEventTime(t time.Time) {
	next := t.Unix()
	for {
		current := lastEventTimeUnix.Load()
		if next <= current {
			return
		}
		if lastEventTimeUnix.CompareAndSwap(current, next) {
			lastEventTimeSeconds.Set(float64(next))
			return
		}
	}
}
