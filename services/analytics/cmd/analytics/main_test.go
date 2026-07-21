package main

import "testing"

func TestSeedExitsNonZeroUntilPhase5(t *testing.T) {
	if code := run([]string{"seed"}); code != 2 {
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
