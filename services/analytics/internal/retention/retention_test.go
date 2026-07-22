package retention

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/vovinacci/devops-demo/services/analytics/internal/store"
)

// testPool connects to the same throwaway Postgres internal/store's own
// tests use (services/analytics/Makefile's test target / CI service
// container); skipped, not failed, when absent.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ANALYTICS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ANALYTICS_TEST_DATABASE_URL not set -- run via `make test` (services/analytics/Makefile)")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func setUpStore(t *testing.T) (*store.Store, *pgxpool.Pool) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS item_events, event_buckets, current_items, seed_marker, schema_migrations`)
	})

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store.New(pool), pool
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

// TestRunOnceAtStartupDeletesOldKeepsRecentAndUntouchedAggregates exercises
// Runner.Run against a real Postgres with a long (1h) ticker interval --
// the assertions can only pass if the startup-time immediate run fires,
// since no tick would land within the test's timeout otherwise.
func TestRunOnceAtStartupDeletesOldKeepsRecentAndUntouchedAggregates(t *testing.T) {
	dataStore, pool := setUpStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	old := now.Add(-10 * 24 * time.Hour)
	recent := now.Add(-time.Hour)

	if _, err := dataStore.IngestEvent(ctx, store.EventRecord{
		ItemID: 1, ItemName: "old", EventType: "created", EventTime: old,
	}); err != nil {
		t.Fatalf("ingest old event: %v", err)
	}
	if _, err := dataStore.IngestEvent(ctx, store.EventRecord{
		ItemID: 2, ItemName: "recent", EventType: "created", EventTime: recent,
	}); err != nil {
		t.Fatalf("ingest recent event: %v", err)
	}

	var bucketsBefore int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_buckets`).Scan(&bucketsBefore); err != nil {
		t.Fatalf("count buckets before: %v", err)
	}

	runsBefore := testutil.ToFloat64(runsTotal.WithLabelValues("success"))
	deletedBefore := testutil.ToFloat64(deletedRowsTotal)

	runner := New(Config{Interval: time.Hour, RetentionDays: 7}, dataStore)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); runner.Run(runCtx) }()
	t.Cleanup(func() { cancel(); <-done })

	waitFor(t, 5*time.Second, func() bool {
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM item_events WHERE item_id = 1`).Scan(&n)
		return n == 0
	})

	var recentCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM item_events WHERE item_id = 2`).Scan(&recentCount); err != nil {
		t.Fatalf("count recent: %v", err)
	}
	if recentCount != 1 {
		t.Fatalf("recent event should survive the retention run, got %d rows", recentCount)
	}

	var bucketsAfter int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_buckets`).Scan(&bucketsAfter); err != nil {
		t.Fatalf("count buckets after: %v", err)
	}
	if bucketsAfter != bucketsBefore {
		t.Fatalf("event_buckets must be untouched by retention: before=%d after=%d", bucketsBefore, bucketsAfter)
	}

	if got := testutil.ToFloat64(runsTotal.WithLabelValues("success")); got <= runsBefore {
		t.Fatalf("want analytics_retention_runs_total{result=success} to increase from %v, got %v", runsBefore, got)
	}
	if got := testutil.ToFloat64(deletedRowsTotal); got <= deletedBefore {
		t.Fatalf("want analytics_retention_deleted_rows_total to increase from %v, got %v", deletedBefore, got)
	}
	if got := testutil.ToFloat64(lastSuccessTimestampSeconds); got == 0 {
		t.Fatalf("want analytics_retention_last_success_timestamp_seconds to be set, got %v", got)
	}
}
