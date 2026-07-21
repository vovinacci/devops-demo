// Package httpserver implements the uniform service contract HTTP
// surface (RFC-0001 D6): /healthz, /readyz, /metrics, wrapped in OTel
// HTTP instrumentation.
package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Pinger is the one database operation readyz depends on. An interface
// (rather than a concrete *pgxpool.Pool) so handler tests can use a fake
// instead of a real Postgres connection.
type Pinger interface {
	Ping(ctx context.Context) error
}

// dbUp mirrors the last readyz outcome as a whitebox metric alongside the
// liveness/readiness endpoints themselves (RFC-0001 D6).
var dbUp = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "analytics_db_up",
	Help: "1 if the last readiness check reached Postgres, 0 otherwise. " +
		"Stream health (PR-B) is a separate metric -- ADR-0005 readyz is DB-only.",
})

// Server builds the analytics HTTP handler.
type Server struct {
	pinger Pinger
	mux    *http.ServeMux
}

// New wires the routes. pinger is checked by /readyz only.
func New(pinger Pinger) *Server {
	s := &Server{pinger: pinger, mux: http.NewServeMux()}
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/readyz", s.handleReadyz)
	s.mux.Handle("/metrics", promhttp.Handler())
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
// liveness is a Phase-B ingest concern (canary pipeline-lag, gRPC stream
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
