package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the analytics database access layer: the current_items read/
// write surface (ingest reconcile + read API) plus item_events/
// event_buckets writes (ingest). Wraps a *pgxpool.Pool rather than
// exposing it directly so callers (internal/httpserver, internal/ingest)
// depend on narrow interfaces instead of pgx.
type Store struct {
	pool *pgxpool.Pool
}

// New wraps an already-connected, already-migrated pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Ping satisfies httpserver.Pinger: /readyz is DB-only (ADR-0005).
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// CurrentItem is one current_items row, tombstoned_at collapsed to a bool
// for API consumers that only need to know "gone or not".
type CurrentItem struct {
	ItemID     int64
	Name       string
	FirstSeen  time.Time
	LastSeen   time.Time
	Tombstoned bool
}

// SnapshotItem is one ListItems response row, the reconcile input.
type SnapshotItem struct {
	ItemID int64
	Name   string
}

// EventRecord is one WatchItemEvents stream event, the ingest input.
type EventRecord struct {
	ItemID    int64
	ItemName  string
	EventType string // "created" | "updated" | "deleted" (item_events CHECK)
	EventTime time.Time
}

// BucketStat is one event_buckets aggregate row for the stats read API.
type BucketStat struct {
	EventType string
	Count     int64
}

// ReconcileSnapshot applies a ListItems snapshot to current_items: upsert
// every returned item, then tombstone every row not in the snapshot that
// isn't already tombstoned (ADR-0002 reconnect sequence). now is the
// snapshot's observation time -- reconcile recovers *state*, not *event
// time*, so it has no better timestamp than "when this snapshot was
// pulled" (contrast with IngestEvent, which uses the event's own
// event_time throughout).
func (s *Store) ReconcileSnapshot(ctx context.Context, items []SnapshotItem, now time.Time) (upserted, tombstoned int, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin reconcile: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	ids := make([]int64, len(items))
	for i, item := range items {
		ids[i] = item.ItemID

		tag, execErr := tx.Exec(ctx, `
			INSERT INTO current_items (item_id, name, first_seen, last_seen, tombstoned_at)
			VALUES ($1, $2, $3, $3, NULL)
			ON CONFLICT (item_id) DO UPDATE SET
				name          = EXCLUDED.name,
				last_seen     = GREATEST(current_items.last_seen, EXCLUDED.last_seen),
				tombstoned_at = NULL
		`, item.ItemID, item.Name, now)
		if execErr != nil {
			return 0, 0, fmt.Errorf("upsert item %d: %w", item.ItemID, execErr)
		}
		upserted += int(tag.RowsAffected())
	}

	// item_id != ALL($1) with an empty ids correctly tombstones every
	// still-live row -- an empty snapshot legitimately means "nothing
	// exists upstream anymore".
	tag, err := tx.Exec(ctx, `
		UPDATE current_items
		SET tombstoned_at = $1
		WHERE tombstoned_at IS NULL AND item_id != ALL($2)
	`, now, ids)
	if err != nil {
		return upserted, 0, fmt.Errorf("tombstone missing items: %w", err)
	}
	tombstoned = int(tag.RowsAffected())

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit reconcile: %w", err)
	}
	return upserted, tombstoned, nil
}

// IngestEvent lands one stream event: raw insert (idempotent replay
// guard), hourly bucket upsert keyed on event time, and a current_items
// state update -- one transaction per event (single ingest writer
// goroutine, demo scale). inserted reports whether the raw event was new
// (RowsAffected==1 on the ON CONFLICT DO NOTHING insert); the caller uses
// it to decide whether the bucket counter and the deduplicated-events
// metric increment.
func (s *Store) IngestEvent(ctx context.Context, ev EventRecord) (inserted bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin ingest: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	// Unique (item_id, event_type, event_time) makes replay after a
	// reconnect idempotent. Forward-note (ADR-0005/PR-A review): two
	// DISTINCT same-type events sharing an exact microsecond event_time
	// would silently dedupe here too -- accepted at demo scale.
	tag, err := tx.Exec(ctx, `
		INSERT INTO item_events (event_type, item_id, item_name, event_time)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (item_id, event_type, event_time) DO NOTHING
	`, ev.EventType, ev.ItemID, ev.ItemName, ev.EventTime)
	if err != nil {
		return false, fmt.Errorf("insert item_event: %w", err)
	}
	inserted = tag.RowsAffected() == 1

	if inserted {
		// date_trunc('hour', event_time, 'UTC') -- EVENT TIME, never
		// arrival time (Hard rule 7, ADR-0005 D1): required for seeded
		// history (an extremely late event source) to land in its
		// correct historical bucket instead of collapsing into "now".
		// The explicit 'UTC' third argument (PG16) makes the truncation
		// independent of the connection/session timezone -- without it,
		// the same event_time could round to a different bucket_start
		// depending on what timezone the session happens to be in, which
		// would silently split or merge buckets across environments.
		// Only incremented when the raw insert above actually inserted,
		// so a deduplicated replay never double-counts.
		if _, err := tx.Exec(ctx, `
			INSERT INTO event_buckets (bucket_start, event_type, count)
			VALUES (date_trunc('hour', $1::timestamptz, 'UTC'), $2, 1)
			ON CONFLICT (bucket_start, event_type) DO UPDATE
			SET count = event_buckets.count + 1, updated_at = now()
		`, ev.EventTime, ev.EventType); err != nil {
			return false, fmt.Errorf("upsert bucket: %w", err)
		}
	}

	// current_items is state, not an event-count aggregate: applying the
	// same event twice is harmless (idempotent upsert), so this runs
	// regardless of `inserted` -- unlike item_events/event_buckets it has
	// no dedup concern of its own.
	var tombstonedAt *time.Time
	if ev.EventType == "deleted" {
		tombstonedAt = &ev.EventTime
	}
	// name and tombstoned_at only take the incoming event's value when
	// its event_time is the new last_seen (the same "wins" condition
	// GREATEST already applies to last_seen itself) -- otherwise an
	// out-of-order older event (replay, reordered delivery, or once the
	// Phase-5 seeder can push arbitrarily out-of-order history through
	// this path) would clobber more-recent state: e.g. a late "created"
	// arriving after a "deleted" must not un-tombstone the row.
	if _, err := tx.Exec(ctx, `
		INSERT INTO current_items (item_id, name, first_seen, last_seen, tombstoned_at)
		VALUES ($1, $2, $3, $3, $4)
		ON CONFLICT (item_id) DO UPDATE SET
			last_seen     = GREATEST(current_items.last_seen, EXCLUDED.last_seen),
			name          = CASE WHEN EXCLUDED.last_seen >= current_items.last_seen
			                      THEN EXCLUDED.name ELSE current_items.name END,
			tombstoned_at = CASE WHEN EXCLUDED.last_seen >= current_items.last_seen
			                      THEN EXCLUDED.tombstoned_at ELSE current_items.tombstoned_at END
	`, ev.ItemID, ev.ItemName, ev.EventTime, tombstonedAt); err != nil {
		return inserted, fmt.Errorf("upsert current_item: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit ingest: %w", err)
	}
	return inserted, nil
}

// GetCurrentItem is the read side of the /api/v1/items/{item_id} handler.
// found=false (not an error) means never seen -- the handler's 404 case.
func (s *Store) GetCurrentItem(ctx context.Context, itemID int64) (item CurrentItem, found bool, err error) {
	var tombstonedAt *time.Time
	err = s.pool.QueryRow(ctx, `
		SELECT item_id, name, first_seen, last_seen, tombstoned_at
		FROM current_items WHERE item_id = $1
	`, itemID).Scan(&item.ItemID, &item.Name, &item.FirstSeen, &item.LastSeen, &tombstonedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CurrentItem{}, false, nil
	}
	if err != nil {
		return CurrentItem{}, false, fmt.Errorf("get current_item %d: %w", itemID, err)
	}
	item.Tombstoned = tombstonedAt != nil
	return item, true, nil
}

// StatsLast24h sums event_buckets by event_type over the last 24h --
// cheap aggregate read, not a report (RFC-0001 Phase 3 PR-B scope; real
// reporting is the Kotlin reports service, a later phase). No date_trunc
// / session-timezone concern here: bucket_start and now() are both
// timestamptz, so the comparison is on absolute instants regardless of
// the connection's session timezone (unlike IngestEvent's bucket write,
// which must pin the timezone explicitly because date_trunc's rounding
// step is timezone-sensitive).
func (s *Store) StatsLast24h(ctx context.Context) ([]BucketStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT event_type, COALESCE(SUM(count), 0)
		FROM event_buckets
		WHERE bucket_start >= now() - interval '24 hours'
		GROUP BY event_type
		ORDER BY event_type
	`)
	if err != nil {
		return nil, fmt.Errorf("query stats: %w", err)
	}
	defer rows.Close()

	var stats []BucketStat
	for rows.Next() {
		var stat BucketStat
		if err := rows.Scan(&stat.EventType, &stat.Count); err != nil {
			return nil, fmt.Errorf("scan stats row: %w", err)
		}
		stats = append(stats, stat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stats: %w", err)
	}
	return stats, nil
}
