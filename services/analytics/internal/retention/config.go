package retention

import (
	"os"
	"strconv"
	"time"
)

// Config is the retention job's runtime configuration, read once at
// startup from the environment (uniform service contract, RFC-0001 D6).
type Config struct {
	// Interval paces the ticker loop. The job also always runs once
	// immediately at startup regardless of Interval (see Runner.Run) --
	// an interval-only loop on a service restarted daily would otherwise
	// never fire.
	Interval time.Duration
	// RetentionDays is the lateness horizon (ADR-0005 D7): raw
	// item_events rows whose event_time (never received_at -- Hard rule
	// 7) is older than this many days are deleted every run.
	RetentionDays int
}

// ConfigFromEnv reads Config from ANALYTICS_RETENTION_INTERVAL and
// ANALYTICS_RETENTION_DAYS.
func ConfigFromEnv() Config {
	return Config{
		Interval:      envDuration("ANALYTICS_RETENTION_INTERVAL", time.Hour),
		RetentionDays: envInt("ANALYTICS_RETENTION_DAYS", 7),
	}
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

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
