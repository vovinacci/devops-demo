// Package retention implements the analytics data lifecycle job (RFC-0001
// D7, ADR-0005): a ticker loop deleting raw item_events rows older than a
// configurable horizon so continuous load does not grow the raw table
// unbounded. See runOnce for why this is delete-only rather than the
// literal "aggregate-then-delete" D7 describes.
package retention

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
)

// DataStore is the retention write surface the Runner depends on,
// satisfied by *store.Store. Kept as an interface (matching
// internal/ingest.DataStore's pattern) so the loop can be exercised
// without a full Store type in scope.
type DataStore interface {
	DeleteEventsOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

// Runner owns the retention ticker loop.
type Runner struct {
	cfg   Config
	store DataStore
}

// New builds a Runner.
func New(cfg Config, dataStore DataStore) *Runner {
	return &Runner{cfg: cfg, store: dataStore}
}

// Run executes the retention loop until ctx is canceled (graceful
// shutdown, cmd/analytics serve()). It runs once immediately, then on
// every tick of cfg.Interval -- an interval-only loop on a service
// restarted before its first tick (a daily redeploy against a 1h default
// interval, for instance) would otherwise never run at all.
func (r *Runner) Run(ctx context.Context) {
	r.runOnce(ctx)

	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runOnce(ctx)
		}
	}
}

// RunOnce executes exactly one retention pass immediately, without
// starting the ticker loop. Exists for the historical seeder's closing
// pass (RFC-0001 Phase 5 D5, .omc/handoffs/team-plan.md): after
// backfilling raw events far into the past, the seeder finishes with the
// SAME retention code the live service runs, so the end state is
// canonical regardless of --days -- not a seeder-specific reimplementation.
func (r *Runner) RunOnce(ctx context.Context) {
	r.runOnce(ctx)
}

// runOnce deletes raw item_events rows older than the configured
// retention horizon, keyed on event_time (Hard rule 7: never received_at
// -- consistent with ingest's own event-time bucketing, ADR-0005 D1).
//
// This is delete-only, not "aggregate-then-delete" in the literal sense
// ADR-0005 D7 describes: the hourly event_buckets rows ARE the
// aggregation for every event this job ever deletes -- internal/store's
// IngestEvent writes the bucket upsert in the same transaction as the raw
// insert, at ingest time, not at retention time. So by the time a raw
// event is old enough to delete, its aggregate has already existed for up
// to RetentionDays -- there is no re-aggregation step to perform here.
// event_buckets and current_items are therefore untouched by this run:
// buckets are the durable business-data aggregate that outlives the raw
// event by construction, current_items is live state, and neither is
// history this job expires.
func (r *Runner) runOnce(ctx context.Context) {
	tracer := otel.Tracer("analytics/retention")
	ctx, span := tracer.Start(ctx, "analytics.retention.run")
	defer span.End()

	start := time.Now()
	cutoff := start.UTC().AddDate(0, 0, -r.cfg.RetentionDays)

	deleted, err := r.store.DeleteEventsOlderThan(ctx, cutoff)
	duration := time.Since(start)
	runDurationSeconds.Observe(duration.Seconds())

	if err != nil {
		runsTotal.WithLabelValues("failure").Inc()
		slog.ErrorContext(ctx, "retention run failed",
			"error", err, "cutoff", cutoff, "duration", duration.String())
		return
	}

	runsTotal.WithLabelValues("success").Inc()
	deletedRowsTotal.Add(float64(deleted))
	lastSuccessTimestampSeconds.Set(float64(start.Unix()))
	slog.InfoContext(ctx, "retention run complete",
		"deleted", deleted, "cutoff", cutoff, "duration", duration.String())
}
