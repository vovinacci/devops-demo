// Package httpserver implements the uniform service contract HTTP
// surface (RFC-0001 D6): /healthz, /readyz, /metrics, wrapped in OTel
// HTTP instrumentation, plus the ingest read API (RFC-0001 Phase 3 PR-B):
// /api/v1/items/{item_id} and /api/v1/stats.
package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/vovinacci/devops-demo/services/analytics/internal/store"
)

// Pinger is the one database operation readyz depends on. An interface
// (rather than a concrete *pgxpool.Pool) so handler tests can use a fake
// instead of a real Postgres connection.
type Pinger interface {
	Ping(ctx context.Context) error
}

// ItemsStore is the read surface the ingest read API depends on,
// satisfied by *store.Store. A row becomes known only via stream
// ingestion or snapshot reconcile (internal/ingest) -- never synthesized
// here -- so GetCurrentItem genuinely reflects gRPC ingestion progress,
// which is what the canary v2 pipeline-lag step polls it for.
type ItemsStore interface {
	GetCurrentItem(ctx context.Context, itemID int64) (store.CurrentItem, bool, error)
	StatsLast24h(ctx context.Context) ([]store.BucketStat, error)
	// GetSeedMarker backs GET /api/v1/seed-marker (RFC-0001 Phase 5 D5):
	// found=false means no `analytics seed` run has ever completed --
	// distinct from an error, so loadgen's scale guard (Phase 5 PR-2) can
	// warn-and-continue on a fresh, never-seeded stack instead of failing.
	GetSeedMarker(ctx context.Context) (store.SeedMarker, bool, error)
}

// dbUp mirrors the last readyz outcome as a whitebox metric alongside the
// liveness/readiness endpoints themselves (RFC-0001 D6).
var dbUp = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "analytics_db_up",
	Help: "1 if the last readiness check reached Postgres, 0 otherwise. " +
		"Stream health is a separate metric (internal/ingest) -- ADR-0005 readyz is DB-only.",
})

// Server builds the analytics HTTP handler.
type Server struct {
	pinger Pinger
	items  ItemsStore
	mux    *http.ServeMux
}

// New wires the routes. pinger is checked by /readyz only; items backs
// the ingest read API.
func New(pinger Pinger, items ItemsStore) *Server {
	s := &Server{pinger: pinger, items: items, mux: http.NewServeMux()}
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/readyz", s.handleReadyz)
	s.mux.Handle("/metrics", promhttp.Handler())
	s.mux.HandleFunc("GET /api/v1/items/{item_id}", s.handleGetItem)
	s.mux.HandleFunc("GET /api/v1/stats", s.handleStats)
	s.mux.HandleFunc("GET /api/v1/seed-marker", s.handleSeedMarker)
	return s
}

// Handler wraps the mux with OTel HTTP instrumentation (RFC-0001 D11).
// healthz/readyz/metrics are excluded: they are polled every few seconds
// by the container healthcheck and Prometheus, and a span per poll would
// be noise, never a request worth tracing.
func (s *Server) Handler() http.Handler {
	filter := otelhttp.WithFilter(func(r *http.Request) bool {
		switch r.URL.Path {
		case "/healthz", "/readyz", "/metrics":
			return false
		default:
			return true
		}
	})
	return otelhttp.NewHandler(s.mux, "analytics", filter)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handleReadyz checks the database only (ADR-0005): stream-connection
// liveness is an ingest concern (canary pipeline-lag, gRPC stream
// metrics), not readiness -- a disconnected upstream event stream must
// not take the HTTP surface out of rotation, matching the canary's own
// readyz discipline (RFC-0001 D10 graceful degradation).
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.pinger.Ping(ctx); err != nil {
		dbUp.Set(0)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	dbUp.Set(1)
	w.WriteHeader(http.StatusOK)
}

type itemResponse struct {
	ItemID     int64     `json:"item_id"`
	Name       string    `json:"name"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	Tombstoned bool      `json:"tombstoned"`
}

// handleGetItem backs the canary v2 pipeline-lag step: a row exists here
// only once it has been observed via WatchItemEvents stream ingestion or
// a ListItems snapshot reconcile (internal/ingest), never synthesized by
// this handler -- so "known" genuinely measures gRPC ingestion progress.
// 404 means never seen; a deleted-but-once-seen item still returns 200
// with tombstoned=true, distinct from unknown.
func (s *Server) handleGetItem(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseInt(r.PathValue("item_id"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	item, found, err := s.items.GetCurrentItem(r.Context(), itemID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, itemResponse{
		ItemID:     item.ItemID,
		Name:       item.Name,
		FirstSeen:  item.FirstSeen,
		LastSeen:   item.LastSeen,
		Tombstoned: item.Tombstoned,
	})
}

type statEntry struct {
	EventType string `json:"event_type"`
	Count     int64  `json:"count"`
}

// handleStats is a cheap aggregate read (last 24h bucket totals by
// event_type), not a report -- real reporting is the Kotlin reports
// service, a later phase.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.items.StatsLast24h(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	entries := make([]statEntry, len(stats))
	for i, stat := range stats {
		entries[i] = statEntry{EventType: stat.EventType, Count: stat.Count}
	}
	writeJSON(w, http.StatusOK, entries)
}

type seedMarkerResponse struct {
	Scale         float64   `json:"scale"`
	RefUnix       int64     `json:"ref_unix"`
	Seed          int64     `json:"seed"`
	Days          int       `json:"days"`
	SeededAt      time.Time `json:"seeded_at"`
	EventsWritten int64     `json:"events_written"`
}

// handleSeedMarker backs the Phase 5 seam contract: loadgen (Phase 5
// PR-2) fetches this at startup to compare its own DEMO_TIME_SCALE
// against the scale history was seeded at (Hard rule 8 -- a mismatch
// silently breaks the seam invariant). 404 means never seeded (fresh
// stack, `make up` with no `make seed-history` yet) -- distinct from an
// error, so the caller can warn-and-continue instead of refusing to
// start.
func (s *Server) handleSeedMarker(w http.ResponseWriter, r *http.Request) {
	marker, found, err := s.items.GetSeedMarker(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, seedMarkerResponse{
		Scale:         marker.Scale,
		RefUnix:       marker.RefUnix,
		Seed:          marker.Seed,
		Days:          marker.Days,
		SeededAt:      marker.SeededAt,
		EventsWritten: marker.EventsWritten,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
