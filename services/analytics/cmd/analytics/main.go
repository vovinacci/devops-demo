// Command analytics is the Go analytics service (RFC-0001 D1). Subcommand
// dispatch via stdlib os.Args -- no flag/cobra dependency for two verbs.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vovinacci/devops-demo/services/analytics/internal/httpserver"
	"github.com/vovinacci/devops-demo/services/analytics/internal/ingest"
	"github.com/vovinacci/devops-demo/services/analytics/internal/logging"
	"github.com/vovinacci/devops-demo/services/analytics/internal/otelsetup"
	"github.com/vovinacci/devops-demo/services/analytics/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "serve":
		return serve()
	case "seed":
		return seed()
	case "healthcheck":
		return healthcheck()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q (want: serve, seed, healthcheck)\n", cmd)
		return 2
	}
}

// seed is a stub: the historical seeder is RFC-0001 Phase 5 (shared load
// profile + stitching), out of scope for this scaffold.
func seed() int {
	fmt.Fprintln(os.Stderr, "seed arrives with RFC-0001 Phase 5")
	return 2
}

func serve() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := logging.New()
	slog.SetDefault(logger)

	tp := otelsetup.Init(ctx)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			logger.Error("otel shutdown failed", "error", err)
		}
	}()

	databaseURL := envString("ANALYTICS_DATABASE_URL",
		"postgres://analytics:analytics@localhost:5433/analytics?sslmode=disable")
	pool, err := store.Connect(ctx, databaseURL, 10, 2*time.Second)
	if err != nil {
		logger.Error("connect to postgres failed", "error", err)
		return 1
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool); err != nil {
		logger.Error("migrate failed", "error", err)
		return 1
	}
	db := store.New(pool)

	// Ingest runs on its own cancelable context so an unexpected HTTP
	// server failure (the errCh branch below) can stop it too, not just
	// a shutdown signal -- ctx alone is only canceled by SIGINT/SIGTERM.
	ingestCtx, cancelIngest := context.WithCancel(ctx)
	defer cancelIngest()
	ingestClient := ingest.New(ingest.ConfigFromEnv(), db)
	ingestDone := make(chan struct{})
	go func() {
		defer close(ingestDone)
		ingestClient.Run(ingestCtx)
	}()

	addr := envString("ANALYTICS_HTTP_ADDR", ":8082")
	srv := httpserver.New(db, db)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("analytics listening", "addr", addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections")
	case err := <-errCh:
		logger.Error("http server failed", "error", err)
		cancelIngest()
		<-ingestDone
		return 1
	}

	// Stop ingest before closing the pool (deferred above, LIFO-last):
	// stream health is an alerting concern, not readyz's (ADR-0005), but
	// the ingest goroutine still holds a *pgxpool.Pool it must stop using
	// before shutdown proceeds.
	cancelIngest()
	<-ingestDone

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown failed", "error", err)
		return 1
	}
	return 0
}

// healthcheck lets the Dockerfile use a distroless runtime image (no
// shell, no wget/curl) by exec-ing the binary itself instead of a shell
// command -- it hits the process's own /healthz and /readyz over loopback.
func healthcheck() int {
	client := &http.Client{Timeout: 3 * time.Second}
	base := "http://localhost:" + healthcheckPort()

	for _, path := range []string{"/healthz", "/readyz"} {
		resp, err := client.Get(base + path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck %s: %v\n", path, err)
			return 1
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "healthcheck %s: status %d\n", path, resp.StatusCode)
			return 1
		}
	}
	return 0
}

func healthcheckPort() string {
	_, port, err := net.SplitHostPort(envString("ANALYTICS_HTTP_ADDR", ":8082"))
	if err != nil {
		return "8082"
	}
	return port
}

func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
