package ingest

import (
	"os"
	"time"
)

// Config is the ingest client's runtime configuration, read once at
// startup from the environment (uniform service contract, RFC-0001 D6).
type Config struct {
	// BackendAddr is the backend gRPC target analytics dials (ADR-0002:
	// analytics dials, backend never does). Default matches a local
	// `make run` against a backend also run locally; compose sets this
	// to the in-network "api:50051" explicitly (same pattern as
	// ANALYTICS_DATABASE_URL's own local-vs-compose default split).
	BackendAddr string
	// BackoffBase and BackoffMax bound the reconnect loop's exponential
	// backoff (ADR-0002 reconnect sequence): first retry waits around
	// BackoffBase, growing exponentially and capped at BackoffMax.
	BackoffBase time.Duration
	BackoffMax  time.Duration
}

// ConfigFromEnv reads Config from ANALYTICS_BACKEND_GRPC_ADDR,
// ANALYTICS_INGEST_BACKOFF_BASE, and ANALYTICS_INGEST_BACKOFF_MAX.
func ConfigFromEnv() Config {
	return Config{
		BackendAddr: envString("ANALYTICS_BACKEND_GRPC_ADDR", "localhost:50051"),
		BackoffBase: envDuration("ANALYTICS_INGEST_BACKOFF_BASE", time.Second),
		BackoffMax:  envDuration("ANALYTICS_INGEST_BACKOFF_MAX", 30*time.Second),
	}
}

func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
