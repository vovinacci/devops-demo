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

	for _, version := range []string{"0001_init", "0002_current_items", "0003_seed_marker", "0004_grafana_readonly"} {
		var got string
		if err := pool.QueryRow(ctx,
			`SELECT version FROM schema_migrations WHERE version = $1`, version,
		).Scan(&got); err != nil {
			t.Fatalf("expected %s recorded in schema_migrations: %v", version, err)
		}
	}

	// 0004's role contract: grafana_ro can log in, is not superuser, and
	// holds exactly the least-privilege surface the datasource needs --
	// USAGE plus SELECT on the three dashboard tables, and nothing on
	// item_events (raw events are not a dashboard table; a new grant
	// there must be a visible, deliberate migration change).
	var canLogin, super bool
	if err := pool.QueryRow(ctx,
		`SELECT rolcanlogin, rolsuper FROM pg_roles WHERE rolname = 'grafana_ro'`,
	).Scan(&canLogin, &super); err != nil {
		t.Fatalf("grafana_ro role missing: %v", err)
	}
	if !canLogin || super {
		t.Fatalf("grafana_ro: want login=true superuser=false, got login=%v superuser=%v", canLogin, super)
	}
	var usage bool
	if err := pool.QueryRow(ctx,
		`SELECT has_schema_privilege('grafana_ro', 'public', 'USAGE')`,
	).Scan(&usage); err != nil || !usage {
		t.Fatalf("grafana_ro schema USAGE: got %v err %v", usage, err)
	}
	for _, table := range []string{"event_buckets", "current_items", "seed_marker"} {
		var sel bool
		if err := pool.QueryRow(ctx,
			`SELECT has_table_privilege('grafana_ro', $1, 'SELECT')`, table,
		).Scan(&sel); err != nil || !sel {
			t.Fatalf("grafana_ro SELECT on %s: got %v err %v", table, sel, err)
		}
	}
	var rawSel bool
	if err := pool.QueryRow(ctx,
		`SELECT has_table_privilege('grafana_ro', 'item_events', 'SELECT')`,
	).Scan(&rawSel); err != nil {
		t.Fatalf("check item_events privilege: %v", err)
	}
	if rawSel {
		t.Fatal("grafana_ro must NOT have SELECT on item_events (least privilege)")
	}
}
