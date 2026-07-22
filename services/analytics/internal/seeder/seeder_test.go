package seeder_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/vovinacci/devops-demo/services/analytics/internal/loadshape"
	"github.com/vovinacci/devops-demo/services/analytics/internal/seeder"
	"github.com/vovinacci/devops-demo/services/analytics/internal/store"
)

// fakeStore records every batch handed to IngestEventsBatch, in call
// order, so tests can inspect the exact generated event set without a
// real Postgres.
type fakeStore struct {
	batches [][]store.EventRecord
}

func (f *fakeStore) IngestEventsBatch(_ context.Context, ev []store.EventRecord) (store.BatchResult, error) {
	cp := make([]store.EventRecord, len(ev))
	copy(cp, ev)
	f.batches = append(f.batches, cp)
	return store.BatchResult{Inserted: int64(len(ev))}, nil
}

func (f *fakeStore) all() []store.EventRecord {
	var out []store.EventRecord
	for _, b := range f.batches {
		out = append(out, b...)
	}
	return out
}

// testProfile mirrors loadprofile/profile.json's shape (Hard rule 8 does
// not require the unit test to read the checked-in file -- Run takes an
// already-parsed loadshape.Profile, so a literal here keeps this package
// testable with no filesystem/repo-layout coupling).
func testProfile() loadshape.Profile {
	return loadshape.Profile{
		BaseRatePerS:        8.0,
		Diurnal:             loadshape.Diurnal{Amplitude: 0.35, PhaseHours: 14.0},
		WeekdayCoefficients: [7]float64{1.05, 1.08, 1.1, 1.1, 1.15, 0.85, 0.75},
		NoisePct:            0.05,
		Trend:               loadshape.Trend{PctPerDay: 0.0015},
		Anomalies: []loadshape.Anomaly{
			{Name: "traffic-spike", Type: "spike", OffsetDays: -10, DurationHours: 6, Multiplier: 4.0},
			{Name: "ingestion-outage", Type: "outage", OffsetDays: -6, DurationHours: 4, Multiplier: 0.0},
			{Name: "gradual-degradation", Type: "degradation", OffsetDays: -2, DurationHours: 48, Multiplier: 0.3},
		},
	}
}

func testItems() []seeder.Item {
	return []seeder.Item{{ID: 1, Name: "widget"}, {ID: 2, Name: "gadget"}, {ID: 3, Name: "gizmo"}}
}

func baseConfig() seeder.Config {
	return seeder.Config{
		Days:      1,
		SeedValue: 42,
		Scale:     1,
		RefTime:   time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
	}
}

func eventKey(ev store.EventRecord) string {
	return fmt.Sprintf("%d|%s|%s|%s", ev.ItemID, ev.ItemName, ev.EventType, ev.EventTime.Format(time.RFC3339Nano))
}

func TestRunRefusesWithNoItems(t *testing.T) {
	_, err := seeder.Run(context.Background(), baseConfig(), testProfile(), nil, &fakeStore{})
	if err == nil {
		t.Fatal("want error for empty item set, got nil")
	}
}

func TestRunRefusesWithNonPositiveDays(t *testing.T) {
	cfg := baseConfig()
	cfg.Days = 0
	_, err := seeder.Run(context.Background(), cfg, testProfile(), testItems(), &fakeStore{})
	if err == nil {
		t.Fatal("want error for days=0, got nil")
	}
}

// TestRunIsDeterministic is the seeder's own parity contract (RFC-0001
// D5): the SAME seed, profile, ref time, and item set must produce a
// byte-identical event set on every run -- a live demo restarted
// mid-seed, or a re-run for idempotency, must reproduce exactly the same
// history, not merely "similar" history.
func TestRunIsDeterministic(t *testing.T) {
	cfg := baseConfig()
	items := testItems()
	profile := testProfile()

	store1 := &fakeStore{}
	if _, err := seeder.Run(context.Background(), cfg, profile, items, store1); err != nil {
		t.Fatalf("first run: %v", err)
	}
	store2 := &fakeStore{}
	if _, err := seeder.Run(context.Background(), cfg, profile, items, store2); err != nil {
		t.Fatalf("second run: %v", err)
	}

	ev1, ev2 := store1.all(), store2.all()
	if len(ev1) == 0 {
		t.Fatal("expected at least one generated event")
	}
	if len(ev1) != len(ev2) {
		t.Fatalf("event count differs between identical runs: %d vs %d", len(ev1), len(ev2))
	}
	for i := range ev1 {
		if eventKey(ev1[i]) != eventKey(ev2[i]) {
			t.Fatalf("event %d differs between identical runs: %+v vs %+v", i, ev1[i], ev2[i])
		}
	}
}

// TestRunDifferentSeedDiffers guards against a fold/refactor accidentally
// making --seed a no-op.
func TestRunDifferentSeedDiffers(t *testing.T) {
	cfg1 := baseConfig()
	cfg2 := baseConfig()
	cfg2.SeedValue = 43
	items := testItems()
	profile := testProfile()

	store1 := &fakeStore{}
	if _, err := seeder.Run(context.Background(), cfg1, profile, items, store1); err != nil {
		t.Fatalf("seed 42 run: %v", err)
	}
	store2 := &fakeStore{}
	if _, err := seeder.Run(context.Background(), cfg2, profile, items, store2); err != nil {
		t.Fatalf("seed 43 run: %v", err)
	}

	ev1, ev2 := store1.all(), store2.all()
	identical := len(ev1) == len(ev2)
	if identical {
		for i := range ev1 {
			if eventKey(ev1[i]) != eventKey(ev2[i]) {
				identical = false
				break
			}
		}
	}
	if identical {
		t.Fatal("different --seed values produced an identical event set")
	}
}

// TestRunEventCountPerHourMatchesLoadshape is the seam-invariant check at
// the generator level: every simulated hour must contain exactly
// round(Rate(bucketMid, profile, scale, ref) * 3600) events -- the same
// function k6 evaluates for live traffic (loadprofile/README.md).
func TestRunEventCountPerHourMatchesLoadshape(t *testing.T) {
	cfg := baseConfig()
	profile := testProfile()
	fs := &fakeStore{}
	if _, err := seeder.Run(context.Background(), cfg, profile, testItems(), fs); err != nil {
		t.Fatalf("run: %v", err)
	}

	windowEnd := cfg.RefTime.Truncate(time.Hour)
	windowStart := windowEnd.Add(-time.Duration(cfg.Days) * 24 * time.Hour)
	refUnix := float64(cfg.RefTime.Unix())

	counts := make(map[time.Time]int)
	for _, ev := range fs.all() {
		counts[ev.EventTime.Truncate(time.Hour)]++
		if ev.EventTime.Before(windowStart) || !ev.EventTime.Before(windowEnd) {
			t.Fatalf("event outside seed window: %s (window [%s, %s))", ev.EventTime, windowStart, windowEnd)
		}
	}

	totalHours := int(windowEnd.Sub(windowStart).Hours())
	for h := range totalHours {
		hourStart := windowStart.Add(time.Duration(h) * time.Hour)
		bucketMid := hourStart.Add(30 * time.Minute)
		want := int(math.Round(loadshape.Rate(float64(bucketMid.Unix()), profile, refUnix, cfg.Scale) * 3600))
		if got := counts[hourStart]; got != want {
			t.Fatalf("hour %s: want %d events (loadshape.Rate), got %d", hourStart, want, got)
		}
	}
}

// TestRunEventTypesAreCreatedOrDeletedOnly locks in the by-type seam fix:
// the backend has no PUT endpoint, so no live WatchItemEvents stream has
// ever emitted "updated" -- seeded history must not invent a type real
// traffic can't produce, or a by-event-type view would show a seam
// exactly where D5 requires none. Also checks the mix is roughly
// balanced, not degenerate (e.g. accidentally always "created").
func TestRunEventTypesAreCreatedOrDeletedOnly(t *testing.T) {
	cfg := baseConfig()
	fs := &fakeStore{}
	if _, err := seeder.Run(context.Background(), cfg, testProfile(), testItems(), fs); err != nil {
		t.Fatalf("run: %v", err)
	}

	events := fs.all()
	if len(events) == 0 {
		t.Fatal("expected at least one generated event")
	}

	var created, deleted int
	for _, ev := range events {
		switch ev.EventType {
		case "created":
			created++
		case "deleted":
			deleted++
		default:
			t.Fatalf("unexpected event type %q -- seeded history must be created/deleted only (no live producer emits updated)", ev.EventType)
		}
	}

	total := created + deleted
	if frac := float64(created) / float64(total); frac < 0.4 || frac > 0.6 {
		t.Fatalf("created fraction %.3f is far from the intended ~50/50 split (created=%d, deleted=%d)", frac, created, deleted)
	}
}

func TestRunRespectsContextCancellation(t *testing.T) {
	cfg := baseConfig()
	cfg.Days = 90 // large enough that cancellation lands mid-run, not after the last hour
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fs := &fakeStore{}
	_, err := seeder.Run(ctx, cfg, testProfile(), testItems(), fs)
	if err == nil {
		t.Fatal("want error when context is already canceled, got nil")
	}
}

// ctxCheckingStore mimics pgxpool's real behavior on an already-canceled
// context (pool.Begin fails instantly) -- unlike fakeStore, which ignores
// ctx entirely and would pass this test whether or not the cancellation
// fix is in place.
type ctxCheckingStore struct {
	batches [][]store.EventRecord
}

func (s *ctxCheckingStore) IngestEventsBatch(ctx context.Context, ev []store.EventRecord) (store.BatchResult, error) {
	if err := ctx.Err(); err != nil {
		return store.BatchResult{}, err
	}
	cp := make([]store.EventRecord, len(ev))
	copy(cp, ev)
	s.batches = append(s.batches, cp)
	return store.BatchResult{Inserted: int64(len(ev))}, nil
}

// cancelAfterNChecks wraps a context so its Nth Err() call triggers the
// real cancel -- lets a test deterministically land cancellation exactly
// on the seeder's own top-of-hour ctx.Err() check, with no timing race
// against real wall-clock work (unlike a manual SIGINT, which usually
// interrupts an in-flight IngestEventsBatch call instead -- a different,
// already-loud-failure code path, not the one this test targets).
type cancelAfterNChecks struct {
	context.Context
	cancel    context.CancelFunc
	triggerAt int
	checks    *int
}

func (c *cancelAfterNChecks) Err() error {
	*c.checks++
	if *c.checks == c.triggerAt {
		c.cancel()
	}
	return c.Context.Err()
}

// TestRunCancellationFlushUsesDetachedContext is the regression test for
// the fix: the ctx.Err() branch's "best-effort" flush must not be
// defeated by the very cancellation it is reacting to. BatchSize is set
// huge so no automatic mid-run flush ever fires -- IngestEventsBatch is
// called exactly once, by the cancellation branch, and only succeeds if
// it runs against a detached (not-yet-canceled) context.
func TestRunCancellationFlushUsesDetachedContext(t *testing.T) {
	cfg := baseConfig()
	cfg.Days = 5
	cfg.BatchSize = 10_000_000 // effectively unbounded: rules out any automatic mid-run flush

	base, cancel := context.WithCancel(context.Background())
	checks := 0
	ctx := &cancelAfterNChecks{Context: base, cancel: cancel, triggerAt: 3, checks: &checks}

	fs := &ctxCheckingStore{}
	result, err := seeder.Run(ctx, cfg, testProfile(), testItems(), fs)
	if err == nil {
		t.Fatal("want a cancellation error, got nil")
	}
	if len(fs.batches) != 1 {
		t.Fatalf("want exactly one flush (the cancellation branch's detached-context flush), got %d", len(fs.batches))
	}
	if result.EventsWritten == 0 {
		t.Fatal("want the pending batch accumulated before cancellation to have been flushed, got 0 events written")
	}
	if got := len(fs.batches[0]); int64(got) != result.EventsWritten {
		t.Fatalf("want the flushed batch size to match EventsWritten: batch=%d written=%d", got, result.EventsWritten)
	}
}

// TestRunCancellationDuringMidLoopFlushStillFlushesDetached covers the
// second cancellation surface: a small BatchSize forces the periodic
// mid-loop flush, and the cancel is timed (via the Err()-counting
// wrapper) to land on the store's own context check inside one of those
// flushes -- the store fails like real pgx, and the seeder must retry
// the same pending batch through the detached context instead of
// dropping it, while still returning the cancellation error.
func TestRunCancellationDuringMidLoopFlushStillFlushesDetached(t *testing.T) {
	cfg := baseConfig()
	cfg.Days = 5
	cfg.BatchSize = 50

	base, cancel := context.WithCancel(context.Background())
	checks := 0
	// Call order: top-of-hour Err() (#1), then one store-side Err() per
	// mid-loop flush -- triggering on #3 cancels inside the second
	// mid-loop flush attempt.
	ctx := &cancelAfterNChecks{Context: base, cancel: cancel, triggerAt: 3, checks: &checks}

	fs := &ctxCheckingStore{}
	result, err := seeder.Run(ctx, cfg, testProfile(), testItems(), fs)
	if err == nil {
		t.Fatal("want a cancellation error, got nil")
	}
	if len(fs.batches) < 2 {
		t.Fatalf("want at least one successful mid-loop flush plus the detached cancellation flush, got %d batches", len(fs.batches))
	}
	last := fs.batches[len(fs.batches)-1]
	if len(last) == 0 {
		t.Fatal("want the detached cancellation flush to carry the pending batch, got an empty final batch")
	}
	var total int64
	for _, b := range fs.batches {
		total += int64(len(b))
	}
	if total != result.EventsWritten {
		t.Fatalf("want EventsWritten to equal the sum of flushed batches: batches=%d written=%d", total, result.EventsWritten)
	}
}
