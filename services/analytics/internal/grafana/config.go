// Package grafana writes seed-time annotations for the loadprofile's
// story anomalies (RFC-0001 Phase 5 D5/Section 7: "Grafana annotations
// for seeded anomalies"). Best-effort by design (D10 spirit): Grafana is
// a profile-independent process the seeder must not depend on to
// succeed -- every exported entry point returns an error for the caller
// to log and continue past, never a reason to fail the seed run.
package grafana

import "os"

// Config is the seed-time annotation writer's connection to Grafana,
// read from the environment (uniform service contract, RFC-0001 D6).
// Compose wires these onto the analytics service alongside its other
// GRAFANA_* env; defaults match the compose grafana service's own
// admin/admin (documented as demo-grade in services/analytics/README.md
// -- not a credential worth protecting in this repo).
type Config struct {
	URL      string
	User     string
	Password string
}

// ConfigFromEnv reads GRAFANA_URL/GRAFANA_USER/GRAFANA_PASSWORD.
func ConfigFromEnv() Config {
	return Config{
		URL:      envString("GRAFANA_URL", "http://grafana:3000"),
		User:     envString("GRAFANA_USER", "admin"),
		Password: envString("GRAFANA_PASSWORD", "admin"),
	}
}

func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
