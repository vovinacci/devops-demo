package httpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vovinacci/devops-demo/services/analytics/internal/httpserver"
	"github.com/vovinacci/devops-demo/services/analytics/internal/store"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(_ context.Context) error { return f.err }

type fakeItemsStore struct {
	items     map[int64]store.CurrentItem
	statsErr  error
	statsData []store.BucketStat
}

func (f fakeItemsStore) GetCurrentItem(_ context.Context, itemID int64) (store.CurrentItem, bool, error) {
	item, found := f.items[itemID]
	return item, found, nil
}

func (f fakeItemsStore) StatsLast24h(_ context.Context) ([]store.BucketStat, error) {
	return f.statsData, f.statsErr
}

func doRequest(t *testing.T, pinger httpserver.Pinger, items httpserver.ItemsStore, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	srv := httpserver.New(pinger, items)
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestHealthzAlwaysOK(t *testing.T) {
	rec := doRequest(t, fakePinger{err: errors.New("db down")}, fakeItemsStore{}, http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestReadyzOKWhenDBUp(t *testing.T) {
	rec := doRequest(t, fakePinger{}, fakeItemsStore{}, http.MethodGet, "/readyz")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestReadyzUnavailableWhenDBDown(t *testing.T) {
	rec := doRequest(t, fakePinger{err: errors.New("db down")}, fakeItemsStore{}, http.MethodGet, "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
}

func TestMetricsExposesPrometheusFormat(t *testing.T) {
	rec := doRequest(t, fakePinger{}, fakeItemsStore{}, http.MethodGet, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Fatalf("expected go_goroutines in exposition, got: %s", rec.Body.String())
	}
}

func TestGetItemKnownReturnsOK(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	items := fakeItemsStore{items: map[int64]store.CurrentItem{
		42: {ItemID: 42, Name: "widget", FirstSeen: now, LastSeen: now, Tombstoned: false},
	}}
	rec := doRequest(t, fakePinger{}, items, http.MethodGet, "/api/v1/items/42")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		ItemID     int64  `json:"item_id"`
		Name       string `json:"name"`
		Tombstoned bool   `json:"tombstoned"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ItemID != 42 || body.Name != "widget" || body.Tombstoned {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestGetItemTombstonedReturnsOKWithFlag(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	items := fakeItemsStore{items: map[int64]store.CurrentItem{
		7: {ItemID: 7, Name: "gone", FirstSeen: now, LastSeen: now, Tombstoned: true},
	}}
	rec := doRequest(t, fakePinger{}, items, http.MethodGet, "/api/v1/items/7")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	var body struct {
		Tombstoned bool `json:"tombstoned"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Tombstoned {
		t.Fatalf("expected tombstoned=true")
	}
}

func TestGetItemUnknownReturns404(t *testing.T) {
	rec := doRequest(t, fakePinger{}, fakeItemsStore{}, http.MethodGet, "/api/v1/items/999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestGetItemBadIDReturns400(t *testing.T) {
	rec := doRequest(t, fakePinger{}, fakeItemsStore{}, http.MethodGet, "/api/v1/items/not-a-number")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestStatsReturnsShape(t *testing.T) {
	items := fakeItemsStore{statsData: []store.BucketStat{
		{EventType: "created", Count: 3},
		{EventType: "deleted", Count: 1},
	}}
	rec := doRequest(t, fakePinger{}, items, http.MethodGet, "/api/v1/stats")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	var body []struct {
		EventType string `json:"event_type"`
		Count     int64  `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body) != 2 || body[0].EventType != "created" || body[0].Count != 3 {
		t.Fatalf("unexpected body: %+v", body)
	}
}
