package store_test

import (
	"testing"
	"time"

	"github.com/vovinacci/devops-demo/services/analytics/internal/store"
)

func TestDeleteEventsOlderThanDeletesOldKeepsRecent(t *testing.T) {
	s, pool, ctx := setUpStore(t)
	now := time.Now().UTC()
	old := now.Add(-10 * 24 * time.Hour)
	recent := now.Add(-time.Hour)

	if _, err := s.IngestEvent(ctx, store.EventRecord{
		ItemID: 1, ItemName: "old", EventType: "created", EventTime: old,
	}); err != nil {
		t.Fatalf("ingest old event: %v", err)
	}
	if _, err := s.IngestEvent(ctx, store.EventRecord{
		ItemID: 2, ItemName: "recent", EventType: "created", EventTime: recent,
	}); err != nil {
		t.Fatalf("ingest recent event: %v", err)
	}

	cutoff := now.Add(-7 * 24 * time.Hour)
	deleted, err := s.DeleteEventsOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("want 1 deleted row, got %d", deleted)
	}

	var oldCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM item_events WHERE item_id = 1`).Scan(&oldCount); err != nil {
		t.Fatalf("count old: %v", err)
	}
	if oldCount != 0 {
		t.Fatalf("old event should have been deleted, found %d rows", oldCount)
	}

	var recentCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM item_events WHERE item_id = 2`).Scan(&recentCount); err != nil {
		t.Fatalf("count recent: %v", err)
	}
	if recentCount != 1 {
		t.Fatalf("recent event should have survived, found %d rows", recentCount)
	}
}

func TestDeleteEventsOlderThanLeavesBucketsAndCurrentItemsUntouched(t *testing.T) {
	s, pool, ctx := setUpStore(t)
	now := time.Now().UTC()
	old := now.Add(-10 * 24 * time.Hour)

	if _, err := s.IngestEvent(ctx, store.EventRecord{
		ItemID: 3, ItemName: "widget", EventType: "created", EventTime: old,
	}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	var bucketsBefore, itemsBefore int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_buckets`).Scan(&bucketsBefore); err != nil {
		t.Fatalf("count buckets before: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM current_items`).Scan(&itemsBefore); err != nil {
		t.Fatalf("count current_items before: %v", err)
	}
	if bucketsBefore == 0 || itemsBefore == 0 {
		t.Fatalf("test setup should have produced a bucket and a current_item row: buckets=%d items=%d", bucketsBefore, itemsBefore)
	}

	cutoff := now.Add(-7 * 24 * time.Hour)
	if _, err := s.DeleteEventsOlderThan(ctx, cutoff); err != nil {
		t.Fatalf("delete: %v", err)
	}

	var bucketsAfter, itemsAfter int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_buckets`).Scan(&bucketsAfter); err != nil {
		t.Fatalf("count buckets after: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM current_items`).Scan(&itemsAfter); err != nil {
		t.Fatalf("count current_items after: %v", err)
	}
	if bucketsAfter != bucketsBefore {
		t.Fatalf("event_buckets must be untouched by retention (it is the D7 aggregate): before=%d after=%d", bucketsBefore, bucketsAfter)
	}
	if itemsAfter != itemsBefore {
		t.Fatalf("current_items must be untouched by retention (it is live state): before=%d after=%d", itemsBefore, itemsAfter)
	}
}

func TestDeleteEventsOlderThanEmptyTableReturnsZero(t *testing.T) {
	s, _, ctx := setUpStore(t)

	deleted, err := s.DeleteEventsOlderThan(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("delete on empty table: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("want 0 deleted rows on an empty table, got %d", deleted)
	}
}
