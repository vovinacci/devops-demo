package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// BatchResult summarizes what one IngestEventsBatch call actually wrote,
// for the seeder's progress logging -- not required for correctness.
type BatchResult struct {
	// Inserted is the count of item_events rows genuinely new (the
	// ON CONFLICT DO NOTHING dedup survivors), mirroring IngestEvent's
	// own inserted return value, summed over the whole batch.
	Inserted int64
	// BucketsTouched is the count of distinct (bucket_start, event_type)
	// rows upserted by this batch -- zero when every event in the batch
	// deduplicated. Diagnostic only (seeder progress logging).
	BucketsTouched int64
}

// IngestEventsBatch lands many events through the SAME ingestion
// semantics as IngestEvent (RFC-0001 Phase 5 D5: the seeder is just an
// extremely late event source pushed through the live ingestion path,
// never direct aggregate writes), one transaction per batch instead of
// one per event -- the only concession Phase 5's strict-D5-letter
// mitigation allows (.omc/handoffs/team-plan.md).
//
// Three invariants this must hold, proven equal to calling IngestEvent
// once per event in ev order (see internal/store/seed_test.go):
//
//  1. Raw insert: multi-row INSERT ... ON CONFLICT (item_id, event_type,
//     event_time) DO NOTHING -- identical unique-index dedup guard as
//     IngestEvent, and DO NOTHING (unlike DO UPDATE) tolerates duplicate
//     keys within the same multi-row statement, so no pre-dedup is
//     needed here.
//  2. Bucket increments: computed ONLY from the RETURNING set of that
//     insert (rows actually new), grouped by
//     date_trunc('hour', event_time, 'UTC') and event_type -- a
//     deduplicated replay (this batch replaying an already-seeded
//     window, or overlapping with a prior batch) must not double-count,
//     exactly like IngestEvent's inserted-gated bucket write.
//  3. current_items: last-event-time-wins, same CASE logic as
//     IngestEvent, applied unconditionally (regardless of whether the
//     raw insert deduplicated) -- current_items is state, not an
//     event-count aggregate. Postgres' "ON CONFLICT DO UPDATE command
//     cannot affect row a second time" forbids a multi-row upsert with
//     the same item_id twice in one statement, so ev is folded to at
//     most one row per item_id first (foldCurrentItems) before the
//     upsert -- proven to converge to the identical final state as
//     applying every event serially (see seed_test.go's fold proof).
func (s *Store) IngestEventsBatch(ctx context.Context, ev []EventRecord) (BatchResult, error) {
	if len(ev) == 0 {
		return BatchResult{}, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BatchResult{}, fmt.Errorf("begin batch ingest: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	eventTypes := make([]string, len(ev))
	itemIDs := make([]int64, len(ev))
	itemNames := make([]string, len(ev))
	eventTimes := make([]time.Time, len(ev))
	for i, e := range ev {
		eventTypes[i] = e.EventType
		itemIDs[i] = e.ItemID
		itemNames[i] = e.ItemName
		eventTimes[i] = e.EventTime
	}

	var inserted, bucketsTouched int64
	// bucket_upsert is referenced by the final SELECT purely so Postgres
	// executes it at all: an unreferenced data-modifying CTE never runs,
	// even though it has side effects further up the WITH chain.
	if err := tx.QueryRow(ctx, `
		WITH input(event_type, item_id, item_name, event_time) AS (
			SELECT * FROM unnest($1::text[], $2::bigint[], $3::text[], $4::timestamptz[])
		),
		ins AS (
			INSERT INTO item_events (event_type, item_id, item_name, event_time)
			SELECT event_type, item_id, item_name, event_time FROM input
			ON CONFLICT (item_id, event_type, event_time) DO NOTHING
			RETURNING event_type, event_time
		),
		bucket_deltas AS (
			SELECT date_trunc('hour', event_time, 'UTC') AS bucket_start, event_type, count(*) AS delta
			FROM ins
			GROUP BY 1, 2
		),
		bucket_upsert AS (
			INSERT INTO event_buckets (bucket_start, event_type, count)
			SELECT bucket_start, event_type, delta FROM bucket_deltas
			ON CONFLICT (bucket_start, event_type) DO UPDATE
			SET count = event_buckets.count + EXCLUDED.count, updated_at = now()
			RETURNING 1
		)
		SELECT (SELECT count(*) FROM ins), (SELECT count(*) FROM bucket_upsert)
	`, eventTypes, itemIDs, itemNames, eventTimes).Scan(&inserted, &bucketsTouched); err != nil {
		return BatchResult{}, fmt.Errorf("batch insert item_events/event_buckets: %w", err)
	}

	folded := foldCurrentItems(ev)
	foldedIDs := make([]int64, len(folded))
	foldedNames := make([]string, len(folded))
	foldedFirstSeen := make([]time.Time, len(folded))
	foldedLastSeen := make([]time.Time, len(folded))
	foldedTombstonedAt := make([]*time.Time, len(folded))
	for i, f := range folded {
		foldedIDs[i] = f.itemID
		foldedNames[i] = f.name
		foldedFirstSeen[i] = f.firstSeen
		foldedLastSeen[i] = f.lastSeen
		foldedTombstonedAt[i] = f.tombstonedAt
	}

	if _, err := tx.Exec(ctx, `
		WITH input(item_id, name, first_seen, last_seen, tombstoned_at) AS (
			SELECT * FROM unnest($1::bigint[], $2::text[], $3::timestamptz[], $4::timestamptz[], $5::timestamptz[])
		)
		INSERT INTO current_items (item_id, name, first_seen, last_seen, tombstoned_at)
		SELECT item_id, name, first_seen, last_seen, tombstoned_at FROM input
		ON CONFLICT (item_id) DO UPDATE SET
			last_seen     = GREATEST(current_items.last_seen, EXCLUDED.last_seen),
			name          = CASE WHEN EXCLUDED.last_seen >= current_items.last_seen
			                      THEN EXCLUDED.name ELSE current_items.name END,
			tombstoned_at = CASE WHEN EXCLUDED.last_seen >= current_items.last_seen
			                      THEN EXCLUDED.tombstoned_at ELSE current_items.tombstoned_at END
	`, foldedIDs, foldedNames, foldedFirstSeen, foldedLastSeen, foldedTombstonedAt); err != nil {
		return BatchResult{}, fmt.Errorf("batch upsert current_items: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return BatchResult{}, fmt.Errorf("commit batch ingest: %w", err)
	}
	return BatchResult{Inserted: inserted, BucketsTouched: bucketsTouched}, nil
}

// foldedItem is one current_items row folded from a batch of events for
// the same item_id, in ev's own slice order.
type foldedItem struct {
	itemID       int64
	name         string
	firstSeen    time.Time
	lastSeen     time.Time
	tombstonedAt *time.Time
}

// foldCurrentItems reduces ev to at most one row per item_id, replicating
// IngestEvent's per-event current_items upsert applied serially in ev's
// order:
//
//   - firstSeen is the event_time of whichever event is FIRST in ev for
//     that item_id -- matching IngestEvent's INSERT ... VALUES tuple,
//     which only takes effect (creates the row) the first time an item_id
//     is ever seen; every later conflict leaves first_seen untouched by
//     the DO UPDATE SET clause, so it never matters which value is
//     supplied on a conflict.
//   - lastSeen/name/tombstonedAt come from the LAST event in ev (for that
//     item_id) whose event_time is >= the running maximum event_time seen
//     so far for that item_id. Because that running maximum only grows,
//     this is provably identical to applying IngestEvent's
//     "EXCLUDED.last_seen >= current_items.last_seen" gate once per event
//     in order (see seed_test.go) -- ties always go to the
//     later-processed event, exactly like the live path's GREATEST/CASE.
//
// The single folded row is then handed to one SQL-level upsert per
// item_id, whose own CASE performs the final comparison against whatever
// is already committed in current_items (from a prior batch or the
// snapshot reconcile) -- so this function only needs to get the
// *within-batch* winner right; cross-batch correctness falls out of the
// SQL CASE unconditionally, regardless of what came before.
func foldCurrentItems(ev []EventRecord) []foldedItem {
	order := make([]int64, 0, len(ev))
	firstSeen := make(map[int64]time.Time, len(ev))
	winners := make(map[int64]*foldedItem, len(ev))

	for _, e := range ev {
		if _, seen := firstSeen[e.ItemID]; !seen {
			firstSeen[e.ItemID] = e.EventTime
			order = append(order, e.ItemID)
		}

		w, exists := winners[e.ItemID]
		if !exists {
			w = &foldedItem{itemID: e.ItemID}
			winners[e.ItemID] = w
		}
		if !exists || !e.EventTime.Before(w.lastSeen) {
			w.name = e.ItemName
			w.lastSeen = e.EventTime
			if e.EventType == "deleted" {
				t := e.EventTime
				w.tombstonedAt = &t
			} else {
				w.tombstonedAt = nil
			}
		}
	}

	folded := make([]foldedItem, len(order))
	for i, id := range order {
		w := winners[id]
		w.firstSeen = firstSeen[id]
		folded[i] = *w
	}
	return folded
}

// SeedMarker is the seed_marker row (RFC-0001 Phase 5 D5): the
// parameters of the most recently completed `analytics seed` run.
type SeedMarker struct {
	Scale         float64
	RefUnix       int64
	Seed          int64
	Days          int
	SeededAt      time.Time
	EventsWritten int64
}

// UpsertSeedMarker records the completion of a seed run. Called exactly
// once, at the end of a successful run -- never on failure or partway
// through -- so GetSeedMarker's presence/absence genuinely means
// "a seed run completed", the contract loadgen's scale guard (Phase 5
// PR-2) depends on.
func (s *Store) UpsertSeedMarker(ctx context.Context, m SeedMarker) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO seed_marker (id, scale, ref_unix, seed, days, seeded_at, events_written)
		VALUES (1, $1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			scale          = EXCLUDED.scale,
			ref_unix       = EXCLUDED.ref_unix,
			seed           = EXCLUDED.seed,
			days           = EXCLUDED.days,
			seeded_at      = EXCLUDED.seeded_at,
			events_written = EXCLUDED.events_written
	`, m.Scale, m.RefUnix, m.Seed, m.Days, m.SeededAt, m.EventsWritten)
	if err != nil {
		return fmt.Errorf("upsert seed_marker: %w", err)
	}
	return nil
}

// GetSeedMarker is the read side backing GET /api/v1/seed-marker.
// found=false (not an error) means no seed run has ever completed.
func (s *Store) GetSeedMarker(ctx context.Context) (SeedMarker, bool, error) {
	var m SeedMarker
	err := s.pool.QueryRow(ctx, `
		SELECT scale, ref_unix, seed, days, seeded_at, events_written
		FROM seed_marker WHERE id = 1
	`).Scan(&m.Scale, &m.RefUnix, &m.Seed, &m.Days, &m.SeededAt, &m.EventsWritten)
	if errors.Is(err, pgx.ErrNoRows) {
		return SeedMarker{}, false, nil
	}
	if err != nil {
		return SeedMarker{}, false, fmt.Errorf("get seed_marker: %w", err)
	}
	return m, true, nil
}
