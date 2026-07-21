// Package ingest is the analytics gRPC ingestion client (RFC-0001 D3,
// ADR-0002): dials the backend, reconciles a ListItems snapshot into
// current_items, then consumes the WatchItemEvents stream, reconnecting
// with exponential backoff + jitter on any error. Backend serves,
// analytics dials -- this package owns all reconnect logic; the backend
// has zero awareness of it.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"time"

	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	itemsv1 "github.com/vovinacci/devops-demo/services/analytics/internal/pb/devopsdemo/items/v1"
	"github.com/vovinacci/devops-demo/services/analytics/internal/store"
)

// DataStore is the ingest write surface the Client depends on, satisfied
// by *store.Store. Kept as an interface (matching httpserver.Pinger's
// pattern) so ingest logic can be exercised without a full Store type in
// scope; the test suite here still runs it against a real throwaway
// Postgres, per internal/store's own test convention.
type DataStore interface {
	ReconcileSnapshot(ctx context.Context, items []store.SnapshotItem, now time.Time) (upserted, tombstoned int, err error)
	IngestEvent(ctx context.Context, ev store.EventRecord) (inserted bool, err error)
}

// errMalformedEvent marks events that can never be persisted (unknown
// enum value from a newer/older contract): retrying is pointless, they
// are logged and dropped. Every other handleEvent error is a transient
// persistence failure and is retried for the same event.
var errMalformedEvent = errors.New("malformed event")

// persistRetryDelay paces same-event retries after a transient store
// failure. Deliberately simple (no exponential growth): if the DB stays
// down, the backend's bounded per-subscriber queue disconnects this
// client anyway and the normal reconnect+snapshot path takes over.
const persistRetryDelay = time.Second

var eventTypeNames = map[itemsv1.EventType]string{
	itemsv1.EventType_EVENT_TYPE_CREATED: "created",
	itemsv1.EventType_EVENT_TYPE_UPDATED: "updated",
	itemsv1.EventType_EVENT_TYPE_DELETED: "deleted",
}

// Client owns the connection to the backend and the reconnect loop.
type Client struct {
	cfg   Config
	store DataStore
}

// New builds a Client. Dialing is lazy and happens inside Run/cycle: a
// fresh gRPC connection is created for every cycle (initial connect and
// every subsequent reconnect alike), matching the ADR-0002 sequence
// diagram's repeated "dial :50051" step rather than relying on gRPC-go's
// own transparent transport reconnection.
func New(cfg Config, dataStore DataStore) *Client {
	return &Client{cfg: cfg, store: dataStore}
}

// Run executes the reconnect loop until ctx is canceled (graceful
// shutdown, cmd/analytics serve()). It never returns early on error: a
// disconnected or errored stream is logged and retried with backoff --
// stream health is a metrics/alerting concern, not fatal to the process
// (mirrors readyz staying DB-only, see internal/httpserver).
func (c *Client) Run(ctx context.Context) {
	attempt := 0
	for ctx.Err() == nil {
		if attempt > 0 {
			reconnectsTotal.Inc()
			wait := backoffDuration(attempt, c.cfg.BackoffBase, c.cfg.BackoffMax)
			slog.InfoContext(ctx, "ingest reconnecting after backoff", "attempt", attempt, "wait", wait.String())
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}

		connected, err := c.cycle(ctx)
		if ctx.Err() != nil {
			return
		}
		if connected {
			// This failure happened after a successful stream, not
			// during a sustained outage -- the next retry starts fresh
			// at the base backoff instead of continuing to grow.
			attempt = 0
		}
		if err != nil {
			slog.ErrorContext(ctx, "ingest cycle ended", "error", err)
		}
		attempt++
	}
}

// cycle performs one dial -> ListItems snapshot reconcile -> resume
// WatchItemEvents stream run (ADR-0002 reconnect sequence). connected
// reports whether the stream was opened at least once, letting Run tell
// a "never connected" failure apart from a "connected, then dropped" one.
func (c *Client) cycle(ctx context.Context) (connected bool, err error) {
	tracer := otel.Tracer("analytics/ingest")
	ctx, span := tracer.Start(ctx, "analytics.ingest.cycle")
	defer span.End()

	conn, err := grpc.NewClient(c.cfg.BackendAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(traceUnaryClientInterceptor()),
		grpc.WithChainStreamInterceptor(traceStreamClientInterceptor()),
	)
	if err != nil {
		streamConnected.Set(0)
		return false, fmt.Errorf("dial %s: %w", c.cfg.BackendAddr, err)
	}
	defer func() { _ = conn.Close() }()

	client := itemsv1.NewItemServiceClient(conn)

	if reconcileErr := c.snapshotReconcile(ctx, client); reconcileErr != nil {
		streamConnected.Set(0)
		return false, fmt.Errorf("snapshot reconcile: %w", reconcileErr)
	}

	stream, err := client.WatchItemEvents(ctx, &itemsv1.WatchItemEventsRequest{})
	if err != nil {
		streamConnected.Set(0)
		return false, fmt.Errorf("open watch stream: %w", err)
	}

	streamConnected.Set(1)
	slog.InfoContext(ctx, "ingest stream connected", "addr", c.cfg.BackendAddr)
	defer streamConnected.Set(0)

	for {
		event, recvErr := stream.Recv()
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) {
				return true, fmt.Errorf("stream closed by server: %w", recvErr)
			}
			return true, fmt.Errorf("stream recv: %w", recvErr)
		}

		// A single bad event must not kill the whole stream, but the
		// two failure classes differ: a malformed event can never
		// succeed (log, drop), while a persistence failure is a
		// transient DB blip -- retry the SAME event before the next
		// Recv, or it would be silently lost while the stream stays
		// healthy.
		for {
			handleErr := c.handleEvent(ctx, event)
			if handleErr == nil {
				break
			}
			if errors.Is(handleErr, errMalformedEvent) {
				slog.ErrorContext(ctx, "discarding malformed event", "error", handleErr)
				break
			}
			slog.ErrorContext(ctx, "ingest event persist failed, retrying", "error", handleErr)
			select {
			case <-ctx.Done():
				return true, ctx.Err()
			case <-time.After(persistRetryDelay):
			}
		}
	}
}

// snapshotReconcile pulls ListItems and reconciles it into current_items
// (upsert by ID, tombstone missing items) -- state recovery, not missed-
// event recovery: intermediate updates and create/delete churn during a
// disconnect are unrecoverable by design, and the resulting event-count
// aggregate dip stays visible (ADR-0002).
func (c *Client) snapshotReconcile(ctx context.Context, client itemsv1.ItemServiceClient) error {
	resp, err := client.ListItems(ctx, &itemsv1.ListItemsRequest{})
	if err != nil {
		return fmt.Errorf("ListItems: %w", err)
	}

	protoItems := resp.GetItems()
	items := make([]store.SnapshotItem, len(protoItems))
	for i, item := range protoItems {
		items[i] = store.SnapshotItem{ItemID: item.GetId(), Name: item.GetName()}
	}

	upserted, tombstoned, err := c.store.ReconcileSnapshot(ctx, items, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}

	snapshotReconcilesTotal.Inc()
	snapshotItems.Set(float64(len(items)))
	slog.InfoContext(ctx, "snapshot reconcile complete",
		"items", len(items), "upserted", upserted, "tombstoned", tombstoned)
	return nil
}

// handleEvent lands one stream event via Store.IngestEvent and updates
// the ingest metrics based on whether it was a genuinely new event or a
// dedup-guarded replay.
func (c *Client) handleEvent(ctx context.Context, event *itemsv1.ItemEvent) error {
	eventType, ok := eventTypeNames[event.GetEventType()]
	if !ok {
		return fmt.Errorf("%w: unknown event type %v", errMalformedEvent, event.GetEventType())
	}

	ev := store.EventRecord{
		ItemID:    event.GetItem().GetId(),
		ItemName:  event.GetItem().GetName(),
		EventType: eventType,
		EventTime: event.GetEventTime().AsTime(),
	}

	inserted, err := c.store.IngestEvent(ctx, ev)
	if err != nil {
		return fmt.Errorf("ingest event: %w", err)
	}

	if inserted {
		eventsIngestedTotal.WithLabelValues(eventType).Inc()
		advanceLastEventTime(ev.EventTime)
	} else {
		eventsDeduplicatedTotal.Inc()
	}

	slog.InfoContext(ctx, "ingest event",
		"item_id", ev.ItemID, "event_type", eventType, "deduplicated", !inserted)
	return nil
}

// backoffDuration computes an equal-jitter delay for the given retry
// attempt (1-indexed): half fixed at the exponentially-grown, capped
// delay, half uniform random on top -- smooths reconnect storms without
// either a thundering herd (no jitter) or unbounded waits (no cap).
func backoffDuration(attempt int, base, maxDelay time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if base <= 0 {
		base = time.Second
	}
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}

	shift := min(attempt-1, 30) // guards the 1<<shift multiply below from overflowing
	delay := base * time.Duration(1<<shift)
	if delay <= 0 || delay > maxDelay {
		delay = maxDelay
	}

	half := delay / 2
	return half + time.Duration(rand.Int64N(int64(half)+1))
}
