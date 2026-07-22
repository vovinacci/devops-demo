package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vovinacci/devops-demo/services/analytics/internal/store"
)

// setUpStore migrates a clean schema and returns a ready *store.Store
// plus the raw pool (for assertion queries the Store type deliberately
// doesn't expose), cleaning up its tables on test completion. Shares
// testPool (defined in migrate_test.go, same package) with the migration
// tests.
func setUpStore(t *testing.T) (*store.Store, *pgxpool.Pool, context.Context) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS item_events, event_buckets, current_items, seed_marker, schema_migrations`)
	})

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store.New(pool), pool, ctx
}

func TestIngestEventSameHourIncrementsOneBucket(t *testing.T) {
	s, pool, ctx := setUpStore(t)
	base := time.Date(2026, 7, 20, 10, 15, 0, 0, time.UTC)

	for i, minute := range []int{0, 30} {
		ev := store.EventRecord{
			ItemID:    1,
			ItemName:  "widget",
			EventType: "created",
			EventTime: base.Add(time.Duration(minute) * time.Minute),
		}
		if i > 0 {
			ev.EventType = "updated" // distinct event_type so the unique index doesn't dedupe it
		}
		if _, err := s.IngestEvent(ctx, ev); err != nil {
			t.Fatalf("ingest event %d: %v", i, err)
		}
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_buckets WHERE bucket_start = date_trunc('hour', $1::timestamptz, 'UTC')`,
		base,
	).Scan(&count); err != nil {
		t.Fatalf("count buckets: %v", err)
	}
	if count != 2 {
		t.Fatalf("want 2 bucket rows (created, updated) sharing one hour, got %d", count)
	}
}

func TestIngestEventDifferentHoursTwoBuckets(t *testing.T) {
	s, pool, ctx := setUpStore(t)
	first := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	second := time.Date(2026, 7, 20, 11, 0, 0, 0, time.UTC)

	for _, eventTime := range []time.Time{first, second} {
		ev := store.EventRecord{ItemID: 1, ItemName: "widget", EventType: "created", EventTime: eventTime}
		if _, err := s.IngestEvent(ctx, ev); err != nil {
			t.Fatalf("ingest event at %s: %v", eventTime, err)
		}
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM event_buckets WHERE event_type = 'created'`,
	).Scan(&count); err != nil {
		t.Fatalf("count buckets: %v", err)
	}
	if count != 2 {
		t.Fatalf("want 2 distinct hourly buckets, got %d", count)
	}
}

func TestIngestEventDuplicateDoesNotDoubleCountBucket(t *testing.T) {
	s, pool, ctx := setUpStore(t)
	eventTime := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	ev := store.EventRecord{ItemID: 5, ItemName: "widget", EventType: "created", EventTime: eventTime}

	inserted1, err := s.IngestEvent(ctx, ev)
	if err != nil || !inserted1 {
		t.Fatalf("first ingest: inserted=%v err=%v", inserted1, err)
	}
	inserted2, err := s.IngestEvent(ctx, ev)
	if err != nil || inserted2 {
		t.Fatalf("replay ingest: inserted=%v err=%v (want inserted=false)", inserted2, err)
	}

	var bucketCount int64
	if err := pool.QueryRow(ctx,
		`SELECT count FROM event_buckets WHERE bucket_start = date_trunc('hour', $1::timestamptz, 'UTC') AND event_type = 'created'`,
		eventTime,
	).Scan(&bucketCount); err != nil {
		t.Fatalf("read bucket count: %v", err)
	}
	if bucketCount != 1 {
		t.Fatalf("want bucket count 1 (replay must not double count), got %d", bucketCount)
	}
}

func TestReconcileSnapshotUpsertsAndTombstones(t *testing.T) {
	s, _, ctx := setUpStore(t)
	now := time.Now().UTC()

	if _, _, err := s.ReconcileSnapshot(ctx, []store.SnapshotItem{
		{ItemID: 1, Name: "one"},
		{ItemID: 2, Name: "two"},
	}, now); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	item, found, err := s.GetCurrentItem(ctx, 1)
	if err != nil || !found || item.Tombstoned {
		t.Fatalf("item 1 after first reconcile: item=%+v found=%v err=%v", item, found, err)
	}

	// Second reconcile omits item 2 -- it must be tombstoned, not deleted.
	later := now.Add(time.Minute)
	_, tombstoned, reconcileErr := s.ReconcileSnapshot(ctx, []store.SnapshotItem{
		{ItemID: 1, Name: "one-renamed"},
	}, later)
	if reconcileErr != nil || tombstoned != 1 {
		t.Fatalf("second reconcile: tombstoned=%d err=%v", tombstoned, reconcileErr)
	}

	item2, found, err := s.GetCurrentItem(ctx, 2)
	if err != nil || !found || !item2.Tombstoned {
		t.Fatalf("item 2 after second reconcile: item=%+v found=%v err=%v", item2, found, err)
	}

	item1, _, err := s.GetCurrentItem(ctx, 1)
	if err != nil || item1.Name != "one-renamed" {
		t.Fatalf("item 1 should be renamed: item=%+v err=%v", item1, err)
	}
}

func TestIngestEventLateOlderEventDoesNotUntombstone(t *testing.T) {
	s, _, ctx := setUpStore(t)
	deleteTime := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	lateOlderCreateTime := deleteTime.Add(-time.Hour) // event_time predates the delete, arrives after it

	if _, err := s.IngestEvent(ctx, store.EventRecord{
		ItemID: 9, ItemName: "widget", EventType: "deleted", EventTime: deleteTime,
	}); err != nil {
		t.Fatalf("delete: %v", err)
	}

	item, found, err := s.GetCurrentItem(ctx, 9)
	if err != nil || !found || !item.Tombstoned {
		t.Fatalf("item after delete: item=%+v found=%v err=%v", item, found, err)
	}

	// A late, out-of-order "created" event with an *older* event_time
	// than the delete must not un-tombstone the row or roll its name
	// back -- last_seen (the most recent event_time seen) decides who
	// wins, not arrival order.
	if _, ingestErr := s.IngestEvent(ctx, store.EventRecord{
		ItemID: 9, ItemName: "widget-old-name", EventType: "created", EventTime: lateOlderCreateTime,
	}); ingestErr != nil {
		t.Fatalf("late older created: %v", ingestErr)
	}

	item, found, err = s.GetCurrentItem(ctx, 9)
	if err != nil || !found || !item.Tombstoned {
		t.Fatalf("item must remain tombstoned after late older event: item=%+v found=%v err=%v", item, found, err)
	}
	if item.Name != "widget" {
		t.Fatalf("name must remain from the winning (later) event, got %q", item.Name)
	}
	if !item.LastSeen.Equal(deleteTime) {
		t.Fatalf("last_seen must stay at the later delete time, got %s", item.LastSeen)
	}
}

func TestStatsLast24hSumsByEventType(t *testing.T) {
	s, _, ctx := setUpStore(t)
	now := time.Now().UTC()

	events := []store.EventRecord{
		{ItemID: 1, ItemName: "a", EventType: "created", EventTime: now},
		{ItemID: 2, ItemName: "b", EventType: "created", EventTime: now.Add(-time.Hour)},
		{ItemID: 1, ItemName: "a", EventType: "deleted", EventTime: now.Add(-2 * time.Hour)},
		// Outside the 24h window -- must not be counted.
		{ItemID: 3, ItemName: "c", EventType: "created", EventTime: now.Add(-48 * time.Hour)},
	}
	for _, ev := range events {
		if _, err := s.IngestEvent(ctx, ev); err != nil {
			t.Fatalf("ingest %+v: %v", ev, err)
		}
	}

	stats, err := s.StatsLast24h(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}

	counts := make(map[string]int64, len(stats))
	for _, stat := range stats {
		counts[stat.EventType] = stat.Count
	}
	if counts["created"] != 2 {
		t.Fatalf("want 2 created in last 24h, got %d (stats=%+v)", counts["created"], stats)
	}
	if counts["deleted"] != 1 {
		t.Fatalf("want 1 deleted in last 24h, got %d (stats=%+v)", counts["deleted"], stats)
	}
}

func TestGetCurrentItemUnknownNotFound(t *testing.T) {
	s, _, ctx := setUpStore(t)
	_, found, err := s.GetCurrentItem(ctx, 12345)
	if err != nil {
		t.Fatalf("get unknown item: %v", err)
	}
	if found {
		t.Fatalf("expected found=false for never-seen item")
	}
}
