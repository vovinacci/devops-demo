package ingest

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	itemsv1 "github.com/vovinacci/devops-demo/services/analytics/internal/pb/devopsdemo/items/v1"
	"github.com/vovinacci/devops-demo/services/analytics/internal/store"
)

// fakeItemService is a minimal in-test ItemService (RFC-0001 D3 shape):
// ListItems returns a fixed snapshot, WatchItemEvents sends a fixed
// sequence of events once and then blocks until the stream's own context
// is canceled -- letting the test control disconnection by stopping the
// surrounding *grpc.Server rather than the handler returning.
type fakeItemService struct {
	itemsv1.UnimplementedItemServiceServer
	items  []*itemsv1.Item
	events []*itemsv1.ItemEvent
}

func (f *fakeItemService) ListItems(context.Context, *itemsv1.ListItemsRequest) (*itemsv1.ListItemsResponse, error) {
	return &itemsv1.ListItemsResponse{Items: f.items}, nil
}

func (f *fakeItemService) WatchItemEvents(_ *itemsv1.WatchItemEventsRequest, stream grpc.ServerStreamingServer[itemsv1.ItemEvent]) error {
	for _, event := range f.events {
		if err := stream.Send(event); err != nil {
			return err
		}
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}

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
		_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS item_events, event_buckets, current_items, schema_migrations`)
	})

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store.New(pool), pool
}

// listenOn binds addr, retrying briefly -- used to rebind the same
// address a moment after the previous *grpc.Server holding it was
// stopped (TIME_WAIT/OS teardown is not instantaneous).
func listenOn(t *testing.T, addr string) net.Listener {
	t.Helper()
	var lastErr error
	for range 40 {
		lis, err := net.Listen("tcp", addr)
		if err == nil {
			return lis
		}
		lastErr = err
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("listen on %s: %v", addr, lastErr)
	return nil
}

// freeAddr reserves an ephemeral port and immediately releases it, for
// listenOn to rebind (possibly more than once, across a simulated server
// restart) at a fixed, known address.
func freeAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve addr: %v", err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()
	return addr
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

func shortBackoffConfig(addr string) Config {
	return Config{BackendAddr: addr, BackoffBase: 20 * time.Millisecond, BackoffMax: 100 * time.Millisecond}
}

func TestBackoffDurationIsBounded(t *testing.T) {
	base := 10 * time.Millisecond
	maxDelay := 100 * time.Millisecond
	for attempt := 1; attempt <= 20; attempt++ {
		d := backoffDuration(attempt, base, maxDelay)
		if d < 0 || d > maxDelay {
			t.Fatalf("attempt %d: backoff %s out of bounds [0, %s]", attempt, d, maxDelay)
		}
	}
}

func TestBackoffDurationGrowsWithAttempt(t *testing.T) {
	base := 10 * time.Millisecond
	maxDelay := 10 * time.Second

	// Jitter only ever adds on top of half the exponential delay, so the
	// minimum possible value at a later attempt (delay/2, no jitter)
	// still exceeds the maximum possible value at an earlier attempt
	// (delay/2 + full jitter) once they're far enough apart -- compare
	// attempt 1 against attempt 5 for headroom against test flakiness.
	small := backoffDuration(1, base, maxDelay)
	large := backoffDuration(5, base, maxDelay)
	if large <= small {
		t.Fatalf("expected backoff to grow: attempt1=%s attempt5=%s", small, large)
	}
}

func itemEvent(itemID int64, name string, eventType itemsv1.EventType, eventTime time.Time) *itemsv1.ItemEvent {
	return &itemsv1.ItemEvent{
		EventType: eventType,
		Item:      &itemsv1.Item{Id: itemID, Name: name},
		EventTime: timestamppb.New(eventTime),
	}
}

// TestLastEventTimeWatermarkOnlyAdvances exercises handleEvent directly
// (no server needed) against dates far outside any other test in this
// file (2027 / 2019), so the assertions hold regardless of test order or
// the package-level metric's state left over from other tests -- the
// watermark is a monotonic max across the whole process, by design.
func TestLastEventTimeWatermarkOnlyAdvances(t *testing.T) {
	dataStore, _ := setUpStore(t)
	client := New(Config{BackendAddr: "unused:0"}, dataStore)
	ctx := context.Background()

	newer := time.Date(2027, 6, 1, 12, 0, 0, 0, time.UTC)
	older := time.Date(2019, 1, 1, 12, 0, 0, 0, time.UTC)

	if err := client.handleEvent(ctx, itemEvent(101, "a", itemsv1.EventType_EVENT_TYPE_CREATED, newer)); err != nil {
		t.Fatalf("handle newer event: %v", err)
	}
	afterNewer := testutil.ToFloat64(lastEventTimeSeconds)
	if afterNewer != float64(newer.Unix()) {
		t.Fatalf("watermark after newer event: want %v got %v", newer.Unix(), afterNewer)
	}

	if err := client.handleEvent(ctx, itemEvent(102, "b", itemsv1.EventType_EVENT_TYPE_CREATED, older)); err != nil {
		t.Fatalf("handle older event: %v", err)
	}
	afterOlder := testutil.ToFloat64(lastEventTimeSeconds)
	if afterOlder != afterNewer {
		t.Fatalf("watermark must not regress on an older event: want %v (unchanged), got %v", afterNewer, afterOlder)
	}
}

func TestCycleSnapshotReconcileThenStreamIngestWithDedup(t *testing.T) {
	dataStore, pool := setUpStore(t)
	ctx := context.Background()

	eventTime := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)
	createdEvent := &itemsv1.ItemEvent{
		EventType: itemsv1.EventType_EVENT_TYPE_CREATED,
		Item:      &itemsv1.Item{Id: 1, Name: "widget"},
		EventTime: timestamppb.New(eventTime),
	}

	addr := freeAddr(t)
	lis := listenOn(t, addr)
	grpcServer := grpc.NewServer()
	itemsv1.RegisterItemServiceServer(grpcServer, &fakeItemService{
		items:  []*itemsv1.Item{{Id: 1, Name: "widget"}, {Id: 2, Name: "gadget"}},
		events: []*itemsv1.ItemEvent{createdEvent, createdEvent}, // exact duplicate -- same (item_id, event_type, event_time)
	})
	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(grpcServer.Stop)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	client := New(shortBackoffConfig(addr), dataStore)
	done := make(chan struct{})
	go func() { defer close(done); client.Run(runCtx) }()
	t.Cleanup(func() { cancel(); <-done })

	// Reconcile: both snapshot items land in current_items.
	waitFor(t, 5*time.Second, func() bool {
		_, found, err := dataStore.GetCurrentItem(ctx, 2)
		return err == nil && found
	})
	item1, found, err := dataStore.GetCurrentItem(ctx, 1)
	if err != nil || !found || item1.Tombstoned {
		t.Fatalf("item 1 after reconcile: item=%+v found=%v err=%v", item1, found, err)
	}

	// Stream: the duplicate event must dedupe, not double-count the bucket.
	waitFor(t, 5*time.Second, func() bool {
		var count int64
		_ = pool.QueryRow(ctx,
			`SELECT count FROM event_buckets WHERE bucket_start = date_trunc('hour', $1::timestamptz, 'UTC') AND event_type = 'created'`,
			eventTime,
		).Scan(&count)
		return count == 1
	})

	var eventRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM item_events WHERE item_id = 1`).Scan(&eventRows); err != nil {
		t.Fatalf("count item_events: %v", err)
	}
	if eventRows != 1 {
		t.Fatalf("want exactly 1 raw item_events row (duplicate deduped), got %d", eventRows)
	}
}

func TestReconnectAfterServerRestartResnapshotsAndCountsReconnect(t *testing.T) {
	dataStore, _ := setUpStore(t)
	ctx := context.Background()
	addr := freeAddr(t)

	server1 := grpc.NewServer()
	itemsv1.RegisterItemServiceServer(server1, &fakeItemService{
		items: []*itemsv1.Item{{Id: 1, Name: "a"}, {Id: 2, Name: "b"}},
	})
	lis1 := listenOn(t, addr)
	go func() { _ = server1.Serve(lis1) }()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	client := New(shortBackoffConfig(addr), dataStore)
	done := make(chan struct{})
	go func() { defer close(done); client.Run(runCtx) }()
	t.Cleanup(func() { cancel(); <-done })

	waitFor(t, 5*time.Second, func() bool {
		_, found, err := dataStore.GetCurrentItem(ctx, 2)
		return err == nil && found
	})

	reconnectsBefore := testutil.ToFloat64(reconnectsTotal)

	// "Kill the fake server stream": stop the whole server, forcing the
	// client's Recv() to error and the reconnect loop to engage.
	server1.Stop()

	server2 := grpc.NewServer()
	itemsv1.RegisterItemServiceServer(server2, &fakeItemService{
		items: []*itemsv1.Item{{Id: 1, Name: "a-v2"}, {Id: 3, Name: "c"}}, // item 2 dropped, item 3 new
	})
	lis2 := listenOn(t, addr)
	go func() { _ = server2.Serve(lis2) }()
	t.Cleanup(server2.Stop)

	// Re-snapshot against the new server instance: item 2 tombstoned,
	// item 3 appears, item 1 renamed.
	waitFor(t, 10*time.Second, func() bool {
		item2, found, err := dataStore.GetCurrentItem(ctx, 2)
		return err == nil && found && item2.Tombstoned
	})
	_, found, err := dataStore.GetCurrentItem(ctx, 3)
	if err != nil || !found {
		t.Fatalf("item 3 after reconnect: found=%v err=%v", found, err)
	}
	item1, found, err := dataStore.GetCurrentItem(ctx, 1)
	if err != nil || !found || item1.Name != "a-v2" {
		t.Fatalf("item 1 after reconnect: item=%+v found=%v err=%v", item1, found, err)
	}

	if got := testutil.ToFloat64(reconnectsTotal); got <= reconnectsBefore {
		t.Fatalf("want analytics_reconnects_total to increase from %v, got %v", reconnectsBefore, got)
	}
}

func TestBackoffBoundedUnderSustainedFailure(t *testing.T) {
	// No server listening at all -- every cycle fails at dial/ListItems.
	// With a short cap, the client must keep retrying (never give up)
	// and never sleep longer than the cap between attempts.
	addr := freeAddr(t)
	dataStore, _ := setUpStore(t)

	cfg := Config{BackendAddr: addr, BackoffBase: 5 * time.Millisecond, BackoffMax: 40 * time.Millisecond}
	client := New(cfg, dataStore)
	before := testutil.ToFloat64(reconnectsTotal)

	runCtx, cancel := context.WithCancel(context.Background())
	stop := time.AfterFunc(500*time.Millisecond, cancel)
	defer stop.Stop()

	done := make(chan struct{})
	go func() { defer close(done); client.Run(runCtx) }()
	<-done

	if after := testutil.ToFloat64(reconnectsTotal); after <= before {
		t.Fatalf("expected reconnects_total to increase from %v under sustained failure, got %v", before, after)
	}
}
