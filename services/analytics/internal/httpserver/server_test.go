package httpserver_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vovinacci/devops-demo/services/analytics/internal/httpserver"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(_ context.Context) error { return f.err }

func doRequest(t *testing.T, pinger httpserver.Pinger, path string) *httptest.ResponseRecorder {
	t.Helper()
	srv := httpserver.New(pinger)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestHealthzAlwaysOK(t *testing.T) {
	rec := doRequest(t, fakePinger{err: errors.New("db down")}, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestReadyzOKWhenDBUp(t *testing.T) {
	rec := doRequest(t, fakePinger{}, "/readyz")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestReadyzUnavailableWhenDBDown(t *testing.T) {
	rec := doRequest(t, fakePinger{err: errors.New("db down")}, "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
}

func TestMetricsExposesPrometheusFormat(t *testing.T) {
	rec := doRequest(t, fakePinger{}, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Fatalf("expected go_goroutines in exposition, got: %s", rec.Body.String())
	}
}
