package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect dials Postgres with bounded retry. Compose starts
// postgres-analytics and analytics together (RFC-0001 D1); the pool must
// tolerate the database not accepting connections yet for the first few
// seconds. The container healthcheck (readyz), not this retry loop, is
// the authoritative signal to orchestration -- this only keeps the
// process from dying on a startup race.
func Connect(ctx context.Context, databaseURL string, attempts int, delay time.Duration) (*pgxpool.Pool, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		pool, err := pgxpool.New(ctx, databaseURL)
		if err != nil {
			lastErr = err
		} else if err := pool.Ping(ctx); err != nil {
			lastErr = err
			pool.Close()
		} else {
			return pool, nil
		}

		if i == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, fmt.Errorf("connect to postgres after %d attempts: %w", attempts, lastErr)
}
