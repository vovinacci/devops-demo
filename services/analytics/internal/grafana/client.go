package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// requestTimeout bounds every individual HTTP call this client makes.
// Grafana may simply be absent (D10 spirit: a profile-independent
// process the seed run must not hang waiting on) -- a bounded per-call
// timeout, not an unbounded default http.Client, keeps a hung/unreachable
// Grafana from stalling `analytics seed` indefinitely.
const requestTimeout = 5 * time.Second

// Client writes and replaces seed-anomaly annotations via the Grafana
// HTTP annotation API (basic auth, RFC-0001 Phase 5 PR-2).
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient builds a Client against cfg. Every method call is
// independently timeout-bounded (requestTimeout); ctx cancellation (e.g.
// SIGINT during `analytics seed`) still takes precedence.
func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: requestTimeout}}
}

type apiAnnotation struct {
	ID      int64    `json:"id"`
	Time    int64    `json:"time"`
	TimeEnd int64    `json:"timeEnd"`
	Tags    []string `json:"tags"`
	Text    string   `json:"text"`
}

// ReplaceAnomalyAnnotations idempotently replaces every existing
// AnomalyTag-tagged annotation with anns: GET the existing set by tag,
// DELETE each by id (Grafana's annotation API has no bulk or tag-scoped
// delete, only delete-by-id), then POST the fresh set -- so re-running
// `analytics seed` (e.g. a repeated `make seed-history`) replaces stale
// anomaly windows instead of accumulating duplicates every run. The
// delete loop is bounded by the length of the just-fetched existing set,
// never open-ended.
//
// Every error here is returned to the caller, which (per RFC-0001 D10
// spirit) logs a warning and continues rather than failing the seed run:
// Grafana is a profile-independent process, and a seed run's success
// must not depend on it being reachable.
func (c *Client) ReplaceAnomalyAnnotations(ctx context.Context, anns []Annotation) error {
	existing, err := c.listByTag(ctx, AnomalyTag)
	if err != nil {
		return fmt.Errorf("list existing %s annotations: %w", AnomalyTag, err)
	}
	for _, a := range existing {
		if err := c.delete(ctx, a.ID); err != nil {
			return fmt.Errorf("delete stale annotation %d: %w", a.ID, err)
		}
	}
	for _, a := range anns {
		if err := c.create(ctx, a); err != nil {
			return fmt.Errorf("create annotation %q: %w", a.Text, err)
		}
	}
	return nil
}

func (c *Client) listByTag(ctx context.Context, tag string) ([]apiAnnotation, error) {
	// Explicit limit: Grafana defaults the list to 100, which would
	// orphan older annotations if the tag ever accumulated past that
	// (normal usage writes 3 and replaces them each seed run).
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.cfg.URL+"/api/annotations?limit=1000&tags="+tag, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.cfg.User, c.cfg.Password)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus(resp)
	}

	var out []apiAnnotation
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode annotations list: %w", err)
	}
	return out, nil
}

func (c *Client) delete(ctx context.Context, id int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/api/annotations/%d", c.cfg.URL, id), nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.cfg.User, c.cfg.Password)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return unexpectedStatus(resp)
	}
	return nil
}

func (c *Client) create(ctx context.Context, a Annotation) error {
	body, err := json.Marshal(apiAnnotation{
		Time:    a.TimeUnixMilli,
		TimeEnd: a.TimeEndUnixMilli,
		Tags:    a.Tags,
		Text:    a.Text,
	})
	if err != nil {
		return fmt.Errorf("marshal annotation: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.URL+"/api/annotations", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.cfg.User, c.cfg.Password)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	// Grafana's create-annotation endpoint returns 200, not 201.
	if resp.StatusCode != http.StatusOK {
		return unexpectedStatus(resp)
	}
	return nil
}

func unexpectedStatus(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(b))
}
