// Package seeder implements the RFC-0001 Phase 5 D5 historical seeder:
// deterministic per-event history generated backwards from a fixed
// reference time, driven by the SAME loadprofile shape function the live
// k6 side evaluates (internal/loadshape), and written through the SAME
// ingestion semantics as the live gRPC stream (internal/store's
// IngestEventsBatch) -- never direct aggregate writes
// (.omc/handoffs/team-plan.md binding decision: strict D5 letter, no
// two-tier shortcut). Dependency-free of gRPC/pb: the caller
// (cmd/analytics) pulls the ADR-0002 ListItems snapshot and passes plain
// Items in, keeping this package trivially unit-testable without a
// backend or a database.
package seeder

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"

	"github.com/vovinacci/devops-demo/services/analytics/internal/loadshape"
	"github.com/vovinacci/devops-demo/services/analytics/internal/store"
)

// defaultBatchSize is the transaction size for IngestEventsBatch when
// Config.BatchSize is unset. team-plan.md: "batch size ~5-10k, benchmark"
// -- 8000 sits in that range; see services/analytics/README.md for the
// measured throughput this yields.
const defaultBatchSize = 8000

// cancelFlushTimeout bounds the best-effort flush that runs on
// cancellation (see the ctx.Err() branch in Run): it must run against a
// context that is NOT already canceled (a canceled ctx would fail
// pgxpool.Begin instantly), but it also must not hang forever if
// Postgres is genuinely unreachable during shutdown.
const cancelFlushTimeout = 30 * time.Second

// Event type mix (RFC-0001 Phase 5 D5's by-type seam): seeded history
// must match what live traffic can actually produce, not just the
// aggregate rate. The backend has no PUT endpoint, so
// EVENT_TYPE_UPDATED (proto/devopsdemo/items/v1/items.proto) is
// contract-first for a producer that does not exist yet -- no live
// WatchItemEvents stream has ever emitted one. Seeding a lookalike
// "updated" bucket would itself be the seam: any by-event-type
// dashboard or query would show synthetic history diverging from live
// shape exactly where it should be seamless. Seeded events are
// therefore created/deleted only, roughly balanced (an independent
// weighted draw per event, not literal create-then-delete pairing --
// over a 90-day window with many events per item, a realistic
// live/tombstoned mix emerges on its own without extra bookkeeping).
const createdWeight = 0.5

// DataStore is the seeder's write surface, satisfied by *store.Store.
type DataStore interface {
	IngestEventsBatch(ctx context.Context, ev []store.EventRecord) (store.BatchResult, error)
}

// Item is one backend item pulled via the real ADR-0002 ListItems
// snapshot (RFC-0001 Phase 5 plan): synthetic events reference real
// item IDs, the coordination point substituting for the "spread
// created_at" RFC text -- backend items carry no created_at column, so
// item identity (not a backdated timestamp) is what ties seeded history
// to real items.
type Item struct {
	ID   int64
	Name string
}

// Config is one seed run's parameters (RFC-0001 Phase 5 D5).
type Config struct {
	// Days is the history window, counting back from RefTime, aligned to
	// hour boundaries.
	Days int
	// SeedValue is the PRNG seed (--seed): together with an identical
	// Days, Scale, RefTime, and item set, the same value reproduces the
	// identical event set (timestamps, type mix, item assignment) --
	// verified by TestRunIsDeterministic. --seed alone does NOT
	// guarantee a byte-identical re-run: RefTime (below) is captured
	// fresh by the caller on every real `analytics seed` invocation, so
	// two separate runs with the same --seed still drift near "now"
	// (anomaly/trend windows shift with RefTime) -- a measured same-day
	// re-run deduplicated 99.4% of events, not 100% (see
	// services/analytics/README.md's Historical seed section for the
	// full re-run/idempotency story).
	SeedValue int64
	// Scale is DEMO_TIME_SCALE, applied identically to loadshape.Rate as
	// the live k6 side (Hard rule 8, ADR-0003) -- passing a value here
	// that loadgen does not also use breaks the seam invariant by
	// design; the seed_marker row exists so loadgen can detect exactly
	// that mismatch.
	Scale float64
	// RefTime is the seed run's "now", captured ONCE by the caller
	// before generation starts (loadprofile/README.md's refUnixSeconds
	// contract: the seeder evaluates at negative offsets from this fixed
	// point, never by re-reading wall-clock time mid-run, which would
	// make every hour's rate a function of how long generation itself
	// has taken).
	RefTime time.Time
	// BatchSize bounds IngestEventsBatch's transaction size. <= 0 uses
	// defaultBatchSize.
	BatchSize int
}

// Result summarizes a completed (or partially completed, on
// cancellation) seed run.
type Result struct {
	EventsWritten int64
}

// Run generates deterministic history for cfg.Days, backwards from
// cfg.RefTime, and writes it through ds.IngestEventsBatch. "Deterministic"
// means: identical cfg (including RefTime) and items reproduce the
// identical event set (TestRunIsDeterministic) -- it does NOT mean two
// separate real invocations dedup to zero, since RefTime is caller-
// supplied and typically fresh each time (see cfg.SeedValue's doc
// comment and services/analytics/README.md's Historical seed section
// for the measured, honest re-run numbers). ctx cancellation (SIGINT)
// stops generation after flushing whatever is already pending, using a
// detached context so that final flush is not itself defeated by the
// cancellation that triggered it -- a partial seed is safe to leave
// as-is and safe to re-run: IngestEventsBatch's ON CONFLICT DO NOTHING
// dedup means replaying already-written events is a no-op.
func Run(ctx context.Context, cfg Config, profile loadshape.Profile, items []Item, ds DataStore) (Result, error) {
	if len(items) == 0 {
		return Result{}, fmt.Errorf("no backend items available -- run `make seed` first (backend items are the referents synthetic events reference)")
	}
	if cfg.Days <= 0 {
		return Result{}, fmt.Errorf("days must be positive, got %d", cfg.Days)
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	// PCG, not the package-level convenience functions: math/rand/v2's
	// PCG algorithm is fully specified (unlike the auto-seeded top-level
	// functions), so a given --seed reproduces the identical stream
	// across Go versions and platforms -- exactly the determinism
	// contract this seeder needs, without hand-rolling an LCG. The
	// second PCG stream parameter only needs to decorrelate from the
	// first; XORing with the golden-ratio constant (the same constant
	// loadshape's splitmix64 uses, purely by convention here, not a
	// shared algorithm) is enough for that.
	rng := rand.New(rand.NewPCG(uint64(cfg.SeedValue), uint64(cfg.SeedValue)^0x9e3779b97f4a7c15))

	refUnix := float64(cfg.RefTime.Unix())
	// Truncate, not Round: aligns the window to hour boundaries (RFC-0001
	// D5), and since cfg.Days*24h is an exact multiple of an hour,
	// truncating RefTime once and subtracting the (integer-hour) window
	// length gives the identical windowStart as truncating
	// (RefTime - window length) directly -- so windowEnd-windowStart is
	// exactly cfg.Days*24h, never off by a partial hour.
	windowEnd := cfg.RefTime.Truncate(time.Hour)
	windowStart := windowEnd.Add(-time.Duration(cfg.Days) * 24 * time.Hour)
	totalHours := int(windowEnd.Sub(windowStart).Hours())

	start := time.Now()
	var totalWritten int64
	pending := make([]store.EventRecord, 0, batchSize)

	flush := func(flushCtx context.Context) error {
		if len(pending) == 0 {
			return nil
		}
		res, err := ds.IngestEventsBatch(flushCtx, pending)
		if err != nil {
			return fmt.Errorf("ingest batch: %w", err)
		}
		totalWritten += res.Inserted
		pending = pending[:0]
		return nil
	}

	for h := range totalHours {
		if err := ctx.Err(); err != nil {
			// ctx is already canceled here, so flushing against it would
			// fail instantly (pgxpool.Begin gives up before dialing) --
			// "save whatever is pending" needs its own detached, bounded
			// context to actually happen, not the same canceled context
			// that triggered this branch. The outer cancellation error is
			// still returned regardless of whether this flush succeeds;
			// this is purely about not silently losing already-generated
			// events on the way out.
			pendingCount := len(pending)
			flushCtx, cancelFlush := context.WithTimeout(context.WithoutCancel(ctx), cancelFlushTimeout)
			flushErr := flush(flushCtx)
			cancelFlush()

			logCtx := context.WithoutCancel(ctx)
			if flushErr != nil {
				slog.ErrorContext(logCtx, "seed cancellation flush failed, pending batch lost",
					"error", flushErr, "pending_lost", pendingCount)
			} else if pendingCount > 0 {
				slog.InfoContext(logCtx, "seed canceled, flushed pending batch before exit",
					"flushed", pendingCount, "events_written", totalWritten)
			}
			return Result{EventsWritten: totalWritten}, fmt.Errorf("seed canceled after %d/%d hours: %w", h, totalHours, err)
		}

		hourStart := windowStart.Add(time.Duration(h) * time.Hour)
		// bucketMid, not hourStart: Rate is an instantaneous rate
		// sample, and the hour's midpoint is the least-biased single
		// point to represent the whole hour's average.
		bucketMid := hourStart.Add(30 * time.Minute)
		rate := loadshape.Rate(float64(bucketMid.Unix()), profile, refUnix, cfg.Scale)
		n := int(math.Round(rate * 3600))

		for range n {
			offsetSeconds := rng.Float64() * 3600
			item := items[rng.IntN(len(items))]
			pending = append(pending, store.EventRecord{
				ItemID:    item.ID,
				ItemName:  item.Name,
				EventType: drawEventType(rng),
				EventTime: hourStart.Add(time.Duration(offsetSeconds * float64(time.Second))),
			})

			if len(pending) >= batchSize {
				if err := flush(ctx); err != nil {
					return Result{EventsWritten: totalWritten}, err
				}
			}
		}

		if (h+1)%24 == 0 || h == totalHours-1 {
			elapsed := time.Since(start)
			var evPerSec float64
			if elapsed.Seconds() > 0 {
				evPerSec = float64(totalWritten) / elapsed.Seconds()
			}
			slog.InfoContext(ctx, "seed progress",
				"day", (h/24)+1, "total_days", cfg.Days,
				"events_written", totalWritten, "events_per_sec", evPerSec)
		}
	}

	if err := flush(ctx); err != nil {
		return Result{EventsWritten: totalWritten}, err
	}
	return Result{EventsWritten: totalWritten}, nil
}

// drawEventType picks one deterministic event type per synthetic event
// from rng (see the createdWeight mix above): created/deleted only.
func drawEventType(rng *rand.Rand) string {
	if rng.Float64() < createdWeight {
		return "created"
	}
	return "deleted"
}
