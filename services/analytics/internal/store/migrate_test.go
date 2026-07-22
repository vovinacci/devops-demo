package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vovinacci/devops-demo/services/analytics/internal/store"
)

// Needs a real Postgres: services/analytics/Makefile's test target starts
// a throwaway container and sets ANALYTICS_TEST_DATABASE_URL; CI runs it
// against a service-container Postgres instead. Skipped, not failed, when
// absent -- e.g. plain `go test ./...` outside either harness.
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

func TestMigrateAppliesCleanlyAndIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS item_events, event_buckets, current_items, seed_marker, schema_migrations`)
	})

	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	// Idempotency matters here: ADR-0005's mutable-upsert semantics assume
	// migrations (and the process restarts that re-run them) never fail
	// or double-apply on an already-migrated database.
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("second migrate (idempotency): %v", err)
	}

	var tableCount int
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_name IN ('item_events', 'event_buckets', 'current_items', 'seed_marker')
	`).Scan(&tableCount)
	if err != nil {
		t.Fatalf("check tables: %v", err)
	}
	if tableCount != 4 {
		t.Fatalf("expected 4 tables (item_events, event_buckets, current_items, seed_marker), got %d", tableCount)
	}

	for _, version := range []string{"0001_init", "0002_current_items", "0003_seed_marker"} {
		var got string
		if err := pool.QueryRow(ctx,
			`SELECT version FROM schema_migrations WHERE version = $1`, version,
		).Scan(&got); err != nil {
			t.Fatalf("expected %s recorded in schema_migrations: %v", version, err)
		}
	}
}
