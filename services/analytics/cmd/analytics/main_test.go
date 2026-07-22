package main

import "testing"

// TestSeedBadFlagExits2 exercises seed's flag parsing without needing a
// real Postgres/backend: an unknown flag fails fast at flag.Parse,
// before any connection attempt. The full seed run (RFC-0001 Phase 5) is
// exercised by internal/seeder's own tests (deterministic generation,
// no infra needed) and services/analytics/README.md's documented
// `make seed-history` e2e run (needs Postgres + backend, not a unit test).
func TestSeedBadFlagExits2(t *testing.T) {
	if code := run([]string{"seed", "--bogus-flag"}); code != 2 {
		t.Fatalf("want exit 2, got %d", code)
	}
}

func TestUnknownSubcommandExits2(t *testing.T) {
	if code := run([]string{"bogus"}); code != 2 {
		t.Fatalf("want exit 2, got %d", code)
	}
}

func TestHealthcheckPortDefaultsTo8082(t *testing.T) {
	t.Setenv("ANALYTICS_HTTP_ADDR", "")
	if port := healthcheckPort(); port != "8082" {
		t.Fatalf("want 8082, got %s", port)
	}
}

func TestHealthcheckPortParsesHostPort(t *testing.T) {
	t.Setenv("ANALYTICS_HTTP_ADDR", "0.0.0.0:9999")
	if port := healthcheckPort(); port != "9999" {
		t.Fatalf("want 9999, got %s", port)
	}
}
