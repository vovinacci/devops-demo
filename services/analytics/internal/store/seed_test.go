package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vovinacci/devops-demo/services/analytics/internal/store"
)

// freshStore drops and re-migrates the schema on pool, returning a
// *store.Store backed by a clean set of tables. Used (unlike setUpStore
// in current_items_test.go, one store per test) when a single test needs
// two independent "runs" of the schema in sequence -- e.g. comparing
// serial IngestEvent calls against one IngestEventsBatch call, which
// must each start from an empty database to be comparable at all.
func freshStore(ctx context.Context, t *testing.T, pool *pgxpool.Pool) *store.Store {
	t.Helper()
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS item_events, event_buckets, current_items, seed_marker, schema_migrations`); err != nil {
		t.Fatalf("drop tables: %v", err)
	}
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store.New(pool)
}

// currentItemsSnapshot reads every current_items row for the given IDs,
// keyed by item_id, so two schema "runs" can be compared row for row.
func currentItemsSnapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ids []int64) map[int64]store.CurrentItem {
	t.Helper()
	snap := make(map[int64]store.CurrentItem, len(ids))
	for _, id := range ids {
		var item store.CurrentItem
		var tombstonedAt *time.Time
		err := pool.QueryRow(ctx, `
			SELECT item_id, name, first_seen, last_seen, tombstoned_at
			FROM current_items WHERE item_id = $1
		`, id).Scan(&item.ItemID, &item.Name, &item.FirstSeen, &item.LastSeen, &tombstonedAt)
		if err != nil {
			t.Fatalf("read current_items %d: %v", id, err)
		}
		item.Tombstoned = tombstonedAt != nil
		snap[id] = item
	}
	return snap
}

func bucketSnapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool) map[string]int64 {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT bucket_start::text, event_type, count FROM event_buckets ORDER BY bucket_start, event_type
	`)
	if err != nil {
		t.Fatalf("query event_buckets: %v", err)
	}
	defer rows.Close()

	snap := make(map[string]int64)
	for rows.Next() {
		var bucketStart, eventType string
		var count int64
		if err := rows.Scan(&bucketStart, &eventType, &count); err != nil {
			t.Fatalf("scan bucket row: %v", err)
		}
		snap[bucketStart+"|"+eventType] = count
	}
	return snap
}

// sampleBatch builds a mixed, deliberately-messy set of events: repeated
// item IDs, out-of-order event_time, a same-batch exact duplicate (to
// exercise ON CONFLICT DO NOTHING within one multi-row INSERT), and a
// late older event after a delete (tombstone gating).
func sampleBatch(base time.Time) []store.EventRecord {
	return []store.EventRecord{
		{ItemID: 1, ItemName: "widget", EventType: "created", EventTime: base},
		{ItemID: 2, ItemName: "gadget", EventType: "created", EventTime: base.Add(5 * time.Minute)},
		{ItemID: 1, ItemName: "widget-v2", EventType: "updated", EventTime: base.Add(10 * time.Minute)},
		// Exact duplicate of the first event -- must dedupe both within
		// this batch and be a no-op raw insert.
		{ItemID: 1, ItemName: "widget", EventType: "created", EventTime: base},
		{ItemID: 3, ItemName: "gizmo", EventType: "created", EventTime: base.Add(20 * time.Minute)},
		{ItemID: 2, ItemName: "gadget", EventType: "deleted", EventTime: base.Add(30 * time.Minute)},
		// Late-arriving OLDER create for item 2, after its delete: must
		// not un-tombstone (last-event-time-wins, same as IngestEvent).
		{ItemID: 2, ItemName: "gadget-old-name", EventType: "created", EventTime: base.Add(25 * time.Minute)},
		{ItemID: 3, ItemName: "gizmo-v2", EventType: "updated", EventTime: base.Add(90 * time.Minute)}, // next hour bucket
	}
}

// TestIngestEventsBatchEqualsSerialIngestEvent is the batch-vs-single
// equivalence proof the Phase 5 plan requires: the SAME input events
// applied one-by-one via IngestEvent, then applied (against a freshly
// reset schema) in one call to IngestEventsBatch, must leave item_events,
// event_buckets, and current_items in identical final states.
func TestIngestEventsBatchEqualsSerialIngestEvent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS item_events, event_buckets, current_items, seed_marker, schema_migrations`)
	})
	base := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	events := sampleBatch(base)
	ids := []int64{1, 2, 3}

	serialStore := freshStore(ctx, t, pool)
	for _, ev := range events {
		if _, err := serialStore.IngestEvent(ctx, ev); err != nil {
			t.Fatalf("serial ingest %+v: %v", ev, err)
		}
	}
	var serialEventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM item_events`).Scan(&serialEventCount); err != nil {
		t.Fatalf("count serial item_events: %v", err)
	}
	serialBuckets := bucketSnapshot(ctx, t, pool)
	serialItems := currentItemsSnapshot(ctx, t, pool, ids)

	batchStore := freshStore(ctx, t, pool) // resets to an empty schema -- a fair comparison
	if _, err := batchStore.IngestEventsBatch(ctx, events); err != nil {
		t.Fatalf("batch ingest: %v", err)
	}
	var batchEventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM item_events`).Scan(&batchEventCount); err != nil {
		t.Fatalf("count batch item_events: %v", err)
	}
	batchBuckets := bucketSnapshot(ctx, t, pool)
	batchItems := currentItemsSnapshot(ctx, t, pool, ids)

	if serialEventCount != batchEventCount {
		t.Fatalf("item_events row count differs: serial=%d batch=%d", serialEventCount, batchEventCount)
	}
	if len(serialBuckets) != len(batchBuckets) {
		t.Fatalf("event_buckets row count differs: serial=%d batch=%d", len(serialBuckets), len(batchBuckets))
	}
	for k, v := range serialBuckets {
		if batchBuckets[k] != v {
			t.Fatalf("bucket %s differs: serial=%d batch=%d", k, v, batchBuckets[k])
		}
	}
	for _, id := range ids {
		s, b := serialItems[id], batchItems[id]
		if s.Name != b.Name || !s.FirstSeen.Equal(b.FirstSeen) || !s.LastSeen.Equal(b.LastSeen) || s.Tombstoned != b.Tombstoned {
			t.Fatalf("current_items %d differs: serial=%+v batch=%+v", id, s, b)
		}
	}

	// Sanity on the actual business semantics, not just serial==batch:
	// item 2's tombstone must survive the late older create (both paths).
	if !batchItems[2].Tombstoned {
		t.Fatalf("item 2 should stay tombstoned after a late older create, got %+v", batchItems[2])
	}
}

func TestIngestEventsBatchDedupWithinBatch(t *testing.T) {
	s, pool, ctx := setUpStore(t)
	eventTime := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	ev := store.EventRecord{ItemID: 1, ItemName: "widget", EventType: "created", EventTime: eventTime}

	res, err := s.IngestEventsBatch(ctx, []store.EventRecord{ev, ev, ev})
	if err != nil {
		t.Fatalf("batch ingest: %v", err)
	}
	if res.Inserted != 1 {
		t.Fatalf("want 1 inserted for 3 identical events in one batch, got %d", res.Inserted)
	}

	var count int64
	if err := pool.QueryRow(ctx,
		`SELECT count FROM event_buckets WHERE bucket_start = date_trunc('hour', $1::timestamptz, 'UTC') AND event_type = 'created'`,
		eventTime,
	).Scan(&count); err != nil {
		t.Fatalf("read bucket count: %v", err)
	}
	if count != 1 {
		t.Fatalf("want bucket count 1 (within-batch dup must not double count), got %d", count)
	}
}

func TestIngestEventsBatchDedupAcrossBatches(t *testing.T) {
	s, _, ctx := setUpStore(t)
	events := sampleBatch(time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC))

	first, err := s.IngestEventsBatch(ctx, events)
	if err != nil {
		t.Fatalf("first batch: %v", err)
	}
	if first.Inserted == 0 {
		t.Fatalf("want first batch to insert at least one row, got %d", first.Inserted)
	}

	second, err := s.IngestEventsBatch(ctx, events)
	if err != nil {
		t.Fatalf("second (replayed) batch: %v", err)
	}
	if second.Inserted != 0 {
		t.Fatalf("want a byte-identical replayed batch to insert 0 new rows (idempotent re-seed), got %d", second.Inserted)
	}
}

func TestIngestEventsBatchEmptyIsNoOp(t *testing.T) {
	s, _, ctx := setUpStore(t)
	res, err := s.IngestEventsBatch(ctx, nil)
	if err != nil {
		t.Fatalf("empty batch: %v", err)
	}
	if res.Inserted != 0 {
		t.Fatalf("want 0 inserted for an empty batch, got %d", res.Inserted)
	}
}

// seedMarkersEqual compares field by field, using time.Time.Equal for
// the timestamp fields (a round-trip through Postgres drops the
// monotonic reading and may return a different, semantically-identical
// *time.Location value -- struct `==` is not safe for time.Time here).
func seedMarkersEqual(a, b store.SeedMarker) bool {
	return a.Scale == b.Scale && a.RefUnix == b.RefUnix && a.Seed == b.Seed &&
		a.Days == b.Days && a.EventsWritten == b.EventsWritten && a.SeededAt.Equal(b.SeededAt)
}

func TestSeedMarkerUpsertAndGet(t *testing.T) {
	s, _, ctx := setUpStore(t)

	if _, found, err := s.GetSeedMarker(ctx); err != nil || found {
		t.Fatalf("want not found before any seed run: found=%v err=%v", found, err)
	}

	seededAt := time.Now().UTC().Truncate(time.Second)
	marker := store.SeedMarker{Scale: 1, RefUnix: 1767571200, Seed: 42, Days: 90, SeededAt: seededAt, EventsWritten: 12345}
	if err := s.UpsertSeedMarker(ctx, marker); err != nil {
		t.Fatalf("upsert seed marker: %v", err)
	}

	got, found, err := s.GetSeedMarker(ctx)
	if err != nil || !found {
		t.Fatalf("get after upsert: found=%v err=%v", found, err)
	}
	if !seedMarkersEqual(got, marker) {
		t.Fatalf("want %+v, got %+v", marker, got)
	}

	// A second seed run (e.g. `make seed-history` re-run) overwrites the
	// single row rather than accumulating history of its own.
	marker2 := store.SeedMarker{Scale: 24, RefUnix: 1767571300, Seed: 43, Days: 30, SeededAt: seededAt.Add(time.Hour), EventsWritten: 999}
	if upsertErr := s.UpsertSeedMarker(ctx, marker2); upsertErr != nil {
		t.Fatalf("second upsert: %v", upsertErr)
	}
	got2, found, err := s.GetSeedMarker(ctx)
	if err != nil || !found || !seedMarkersEqual(got2, marker2) {
		t.Fatalf("want marker replaced by second upsert: found=%v err=%v got=%+v", found, err, got2)
	}
}
