package store

import (
	"context"
	"fmt"
	"time"
)

// DeleteEventsOlderThan deletes raw item_events rows whose event_time --
// never received_at (Hard rule 7, ADR-0005 D1), consistent with
// IngestEvent's own event-time bucket keying -- is older than cutoff.
//
// Delete-only, not "aggregate-then-delete" in the literal D7 sense: the
// hourly event_buckets rows are already the aggregation for every event
// this deletes, written at ingest time (see IngestEvent's bucket upsert in
// the same transaction as the raw insert), so there is no re-aggregation
// step to perform here. event_buckets and current_items are therefore
// left untouched by this query: buckets are the durable business-data
// aggregate that outlives the raw event by construction (ADR-0005),
// current_items is live state, and neither is history this job expires.
func (s *Store) DeleteEventsOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM item_events WHERE event_time < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete item_events older than %s: %w", cutoff, err)
	}
	return tag.RowsAffected(), nil
}
