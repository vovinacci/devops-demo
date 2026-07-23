package grafana_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vovinacci/devops-demo/services/analytics/internal/grafana"
	"github.com/vovinacci/devops-demo/services/analytics/internal/loadshape"
)

func testProfile() loadshape.Profile {
	return loadshape.Profile{
		BaseRatePerS: 8.0,
		Anomalies: []loadshape.Anomaly{
			{Name: "traffic-spike", Type: "spike", OffsetDays: -10, DurationHours: 6, Multiplier: 4.0},
			{Name: "ingestion-outage", Type: "outage", OffsetDays: -6, DurationHours: 4, Multiplier: 0.0},
			{Name: "gradual-degradation", Type: "degradation", OffsetDays: -2, DurationHours: 48, Multiplier: 0.3},
		},
	}
}

func TestComputeAnnotationsOnePerAnomaly(t *testing.T) {
	ref := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	anns := grafana.ComputeAnnotations(testProfile(), ref, 1)
	if len(anns) != 3 {
		t.Fatalf("want 3 annotations (one per anomaly), got %d", len(anns))
	}
	for i, a := range anns {
		if a.TimeEndUnixMilli <= a.TimeUnixMilli {
			t.Fatalf("annotation %d: timeEnd (%d) must be after time (%d)", i, a.TimeEndUnixMilli, a.TimeUnixMilli)
		}
		if len(a.Tags) < 2 || a.Tags[0] != grafana.AnomalyTag {
			t.Fatalf("annotation %d: want first tag %q, got %v", i, grafana.AnomalyTag, a.Tags)
		}
	}
	// Exact bounds at scale 1: the spike (offset -10d, 6h) window is
	// plain wall-clock offset arithmetic.
	spikeStart := ref.Add(-10 * 24 * time.Hour).UnixMilli()
	spikeEnd := ref.Add(-10*24*time.Hour + 6*time.Hour).UnixMilli()
	if anns[0].TimeUnixMilli != spikeStart || anns[0].TimeEndUnixMilli != spikeEnd {
		t.Fatalf("spike window at scale 1: want [%d, %d], got [%d, %d]",
			spikeStart, spikeEnd, anns[0].TimeUnixMilli, anns[0].TimeEndUnixMilli)
	}
}

func TestComputeAnnotationsScale24CompressesTowardRef(t *testing.T) {
	ref := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	anns := grafana.ComputeAnnotations(testProfile(), ref, 24)
	if len(anns) != 3 {
		t.Fatalf("want 3 annotations, got %d", len(anns))
	}
	// At scale 24 the wall-clock window is the profile-time window
	// divided by the scale: the spike's profile window [-10d, -10d+6h]
	// lands at [-10h, -9h45m] real time (AnomalyRealWindow inversion).
	spikeStart := ref.Add(-10 * time.Hour).UnixMilli()
	spikeEnd := ref.Add(-10*time.Hour + 15*time.Minute).UnixMilli()
	if anns[0].TimeUnixMilli != spikeStart || anns[0].TimeEndUnixMilli != spikeEnd {
		t.Fatalf("spike window at scale 24: want [%d, %d], got [%d, %d]",
			spikeStart, spikeEnd, anns[0].TimeUnixMilli, anns[0].TimeEndUnixMilli)
	}
}

// fakeGrafana is an in-memory stand-in for the annotation API surface
// ReplaceAnomalyAnnotations depends on: GET by tag, DELETE by id, POST
// create -- no real Grafana in unit tests.
type fakeGrafana struct {
	nextID  int64
	rows    map[int64]map[string]any
	deletes []int64
	creates []map[string]any
}

func newFakeGrafana(seed ...map[string]any) *fakeGrafana {
	f := &fakeGrafana{rows: map[int64]map[string]any{}}
	for _, row := range seed {
		f.nextID++
		row["id"] = f.nextID
		f.rows[f.nextID] = row
	}
	return f
}

func (f *fakeGrafana) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "admin" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/annotations":
			// Reject requests missing the production filters: without
			// tags= the client would fetch (and then delete) unrelated
			// annotations, and without an explicit limit Grafana's
			// default 100 could orphan older entries -- this fake fails
			// the test if either regresses.
			q := r.URL.Query()
			if q.Get("tags") != "seed-anomaly" || q.Get("limit") != "1000" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var out []map[string]any
			for _, row := range f.rows {
				out = append(out, row)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodDelete:
			id := parseTrailingID(r.URL.Path)
			f.deletes = append(f.deletes, id)
			delete(f.rows, id)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/api/annotations":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.creates = append(f.creates, body)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// parseTrailingID extracts the trailing /api/annotations/{id} path
// segment -- avoids pulling in a router for one test double.
func parseTrailingID(path string) int64 {
	var n, mul int64 = 0, 1
	for i := len(path) - 1; i >= 0 && path[i] >= '0' && path[i] <= '9'; i-- {
		n += int64(path[i]-'0') * mul
		mul *= 10
	}
	return n
}

func testConfig(url string) grafana.Config {
	return grafana.Config{URL: url, User: "admin", Password: "admin"}
}

func TestReplaceAnomalyAnnotationsWritesAllOnEmptyStart(t *testing.T) {
	fake := newFakeGrafana()
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	client := grafana.NewClient(testConfig(srv.URL))
	anns := grafana.ComputeAnnotations(testProfile(), time.Now(), 1)

	if err := client.ReplaceAnomalyAnnotations(context.Background(), anns); err != nil {
		t.Fatalf("ReplaceAnomalyAnnotations: %v", err)
	}
	if len(fake.deletes) != 0 {
		t.Fatalf("want no deletes against an empty annotation set, got %v", fake.deletes)
	}
	if len(fake.creates) != len(anns) {
		t.Fatalf("want %d creates, got %d", len(anns), len(fake.creates))
	}
}

func TestReplaceAnomalyAnnotationsDeletesStaleThenCreates(t *testing.T) {
	fake := newFakeGrafana(
		map[string]any{"tags": []string{grafana.AnomalyTag, "spike"}},
		map[string]any{"tags": []string{grafana.AnomalyTag, "outage"}},
	)
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	client := grafana.NewClient(testConfig(srv.URL))
	anns := grafana.ComputeAnnotations(testProfile(), time.Now(), 1)

	if err := client.ReplaceAnomalyAnnotations(context.Background(), anns); err != nil {
		t.Fatalf("ReplaceAnomalyAnnotations: %v", err)
	}
	if len(fake.deletes) != 2 {
		t.Fatalf("want the 2 pre-existing rows deleted, got %v", fake.deletes)
	}
	if len(fake.rows) != 0 {
		t.Fatalf("want 0 rows left over from the stale set, got %d", len(fake.rows))
	}
	if len(fake.creates) != len(anns) {
		t.Fatalf("want %d fresh creates replacing the stale set, got %d", len(anns), len(fake.creates))
	}
}

func TestReplaceAnomalyAnnotationsAbsentGrafanaReturnsError(t *testing.T) {
	fake := newFakeGrafana()
	srv := httptest.NewServer(fake.handler())
	srv.Close() // closed before any request -- simulates Grafana absent (D10 spirit)

	client := grafana.NewClient(testConfig(srv.URL))
	anns := grafana.ComputeAnnotations(testProfile(), time.Now(), 1)

	err := client.ReplaceAnomalyAnnotations(context.Background(), anns)
	if err == nil {
		t.Fatal("want an error against an unreachable Grafana, got nil -- caller (cmd/analytics) relies on this to log-and-continue")
	}
}
