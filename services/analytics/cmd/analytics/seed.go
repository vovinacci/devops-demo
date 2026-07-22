package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/vovinacci/devops-demo/services/analytics/internal/grafana"
	"github.com/vovinacci/devops-demo/services/analytics/internal/loadshape"
	"github.com/vovinacci/devops-demo/services/analytics/internal/logging"
	itemsv1 "github.com/vovinacci/devops-demo/services/analytics/internal/pb/devopsdemo/items/v1"
	"github.com/vovinacci/devops-demo/services/analytics/internal/retention"
	"github.com/vovinacci/devops-demo/services/analytics/internal/seeder"
	"github.com/vovinacci/devops-demo/services/analytics/internal/store"
)

// seed implements the `analytics seed` subcommand (RFC-0001 Phase 5 D5,
// .omc/handoffs/team-plan.md binding decision): strict per-event
// historical seeding through the live ingestion path
// (internal/store.IngestEventsBatch), never direct aggregate writes.
// Flags mirror the team-plan's documented CLI (--days default 90, --seed
// default 42); DEMO_TIME_SCALE and the ANALYTICS_* connection env vars
// are reused from the serve subcommand (uniform service contract,
// RFC-0001 D6) rather than reintroduced as seed-specific flags.
func seed(args []string) int {
	logger := logging.New()
	slog.SetDefault(logger)

	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	days := fs.Int("days", 90, "days of history to generate, counting back from now")
	seedValue := fs.Int64("seed", 42, "PRNG seed for deterministic event generation")
	profilePath := fs.String("profile",
		envString("ANALYTICS_LOADPROFILE_PATH", "../../loadprofile/profile.json"),
		"path to loadprofile/profile.json")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	scale := envFloat("DEMO_TIME_SCALE", 1.0)
	// Captured ONCE, before any generation happens: loadprofile/README.md's
	// refUnixSeconds contract requires one fixed reference point for the
	// whole run, not wall-clock re-read per hour (which would make each
	// hour's rate a function of how long generation has already taken).
	refTime := time.Now().UTC()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	profileBytes, err := os.ReadFile(*profilePath)
	if err != nil {
		logger.Error("read load profile failed", "path", *profilePath, "error", err)
		return 1
	}
	profile, err := loadshape.ParseProfile(profileBytes)
	if err != nil {
		logger.Error("parse load profile failed", "path", *profilePath, "error", err)
		return 1
	}

	databaseURL := envString("ANALYTICS_DATABASE_URL",
		"postgres://analytics:analytics@localhost:5433/analytics?sslmode=disable")
	pool, err := store.Connect(ctx, databaseURL, 10, 2*time.Second)
	if err != nil {
		logger.Error("connect to postgres failed", "error", err)
		return 1
	}
	defer pool.Close()
	if migrateErr := store.Migrate(ctx, pool); migrateErr != nil {
		logger.Error("migrate failed", "error", migrateErr)
		return 1
	}
	db := store.New(pool)

	backendAddr := envString("ANALYTICS_BACKEND_GRPC_ADDR", "localhost:50051")
	items, err := fetchBackendItems(ctx, backendAddr)
	if err != nil {
		logger.Error("fetch backend items failed", "addr", backendAddr, "error", err)
		return 1
	}
	if len(items) == 0 {
		fmt.Fprintln(os.Stderr,
			"no backend items found -- run `make seed` first (backend items are the referents synthetic events reference)")
		return 1
	}

	logger.Info("seed starting",
		"days", *days, "seed", *seedValue, "scale", scale, "ref_time", refTime, "items", len(items))

	result, runErr := seeder.Run(ctx, seeder.Config{
		Days:      *days,
		SeedValue: *seedValue,
		Scale:     scale,
		RefTime:   refTime,
	}, profile, items, db)
	if runErr != nil {
		logger.Error("seed generation failed", "error", runErr, "events_written", result.EventsWritten)
		return 1
	}

	logger.Info("seed generation complete, running closing retention pass",
		"events_written", result.EventsWritten)
	// The SAME retention code the live service runs (RFC-0001 D5's own
	// "same aggregation/bucketing/retention code" clause, ADR-0005 D7):
	// trims raw item_events past the retention horizon so the end state
	// is canonical regardless of --days, not a seeder-specific rebuild
	// of the retention logic.
	retention.New(retention.ConfigFromEnv(), db).RunOnce(ctx)

	// Grafana annotations for the 3 story anomalies (RFC-0001 Phase 5
	// D5/Section 7, Phase 5 PR-2): best-effort, D10 spirit -- Grafana is a
	// profile-independent process, so a write failure here (absent,
	// unreachable, wrong creds) is logged and the seed run still succeeds
	// and still writes its marker below. Idempotent: re-running replaces
	// the previous run's annotations by tag instead of accumulating them.
	gc := grafana.NewClient(grafana.ConfigFromEnv())
	anns := grafana.ComputeAnnotations(profile, refTime, scale)
	if annErr := gc.ReplaceAnomalyAnnotations(ctx, anns); annErr != nil {
		logger.Warn("grafana annotation write failed, continuing without them", "error", annErr)
	} else {
		logger.Info("grafana annotations written", "count", len(anns))
	}

	// Written LAST, success only: GET /api/v1/seed-marker existing means
	// "a seed run completed", the contract loadgen's scale guard (Phase 5
	// PR-2) depends on -- a partial/failed run above returns before this
	// point and leaves no marker. Note RunOnce (above) logs but does not
	// return a retention failure, so the marker is written even if the
	// closing retention pass itself failed -- accepted for a demo seeder.
	if err := db.UpsertSeedMarker(ctx, store.SeedMarker{
		Scale:         scale,
		RefUnix:       refTime.Unix(),
		Seed:          *seedValue,
		Days:          *days,
		SeededAt:      time.Now().UTC(),
		EventsWritten: result.EventsWritten,
	}); err != nil {
		logger.Error("write seed marker failed", "error", err)
		return 1
	}

	logger.Info("seed complete", "events_written", result.EventsWritten, "days", *days, "seed", *seedValue)
	return 0
}

// fetchBackendItems pulls the ADR-0002 ListItems snapshot once. Unlike
// internal/ingest.Client, the seeder is a one-shot batch job, not a
// long-lived reconnecting stream consumer -- a single dial with no
// backoff loop is deliberate: an operator running `make seed-history`
// interactively benefits from a fast, clear failure over an open-ended
// reconnect wait.
func fetchBackendItems(ctx context.Context, backendAddr string) ([]seeder.Item, error) {
	conn, err := grpc.NewClient(backendAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", backendAddr, err)
	}
	defer func() { _ = conn.Close() }()

	client := itemsv1.NewItemServiceClient(conn)
	resp, err := client.ListItems(ctx, &itemsv1.ListItemsRequest{})
	if err != nil {
		return nil, fmt.Errorf("ListItems: %w", err)
	}

	protoItems := resp.GetItems()
	items := make([]seeder.Item, len(protoItems))
	for i, it := range protoItems {
		items[i] = seeder.Item{ID: it.GetId(), Name: it.GetName()}
	}
	// Deterministic seeding draws items by index: pin the order here
	// instead of trusting the backend's response ordering forever.
	slices.SortFunc(items, func(a, b seeder.Item) int { return cmp.Compare(a.ID, b.ID) })
	return items, nil
}

func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}
