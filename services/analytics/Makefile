# Self-contained service Makefile (RFC-0001 D6/D8): identical target set
# across every service -- build, test, lint, run. Root Makefile delegates
# here via lint-analytics/test-analytics.
.DEFAULT_GOAL := help

PRINT_TARGET = @echo "> make -> $@"

TEST_PG_CONTAINER = analytics-test-pg
TEST_PG_PORT = 55432
TEST_DATABASE_URL = postgres://analytics:analytics@localhost:$(TEST_PG_PORT)/analytics_test?sslmode=disable

##@ Help

.PHONY: help
help: ## Display this help message
	@awk 'BEGIN {FS = ":.*?## "}; /^[a-zA-Z_-]+:.*?## / { printf "  %-15s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Analytics

.PHONY: build
build: ## Compile the binary
	$(PRINT_TARGET)
	go build -o bin/analytics ./cmd/analytics

.PHONY: test
test: ## Run unit + integration tests; migration tests need real Postgres, so a throwaway container is started/stopped around the run
	$(PRINT_TARGET)
	@docker rm -f $(TEST_PG_CONTAINER) >/dev/null 2>&1 || true
	docker run -d --name $(TEST_PG_CONTAINER) -p $(TEST_PG_PORT):5432 \
		-e POSTGRES_DB=analytics_test -e POSTGRES_USER=analytics -e POSTGRES_PASSWORD=analytics \
		postgres:16-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777 >/dev/null
	@echo "waiting for test postgres..."
	@timeout=30; \
	until docker exec $(TEST_PG_CONTAINER) pg_isready -U analytics -d analytics_test >/dev/null 2>&1; do \
		timeout=$$((timeout - 1)); \
		if [ $$timeout -le 0 ]; then echo "test postgres did not become ready"; docker rm -f $(TEST_PG_CONTAINER) >/dev/null; exit 1; fi; \
		sleep 1; \
	done
	@# -p 1: internal/store and internal/ingest (RFC-0001 Phase 3 PR-B) both
	@# exercise real Postgres against this one throwaway container and both
	@# tear down with a DROP TABLE cleanup; `go test`'s default cross-package
	@# parallelism runs their test binaries concurrently, so one package's
	@# cleanup can race another package's still-running test against the
	@# same tables. Serializing packages avoids the race (within a package,
	@# tests already run sequentially unless they opt into t.Parallel()).
	@ANALYTICS_TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -p 1 ./... || (docker rm -f $(TEST_PG_CONTAINER) >/dev/null; exit 1)
	@docker rm -f $(TEST_PG_CONTAINER) >/dev/null

.PHONY: lint
lint: ## Format check + go vet + golangci-lint (deny warnings)
	$(PRINT_TARGET)
	test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...
	golangci-lint run ./...

.PHONY: format
format: ## Auto-format sources
	$(PRINT_TARGET)
	gofmt -w .

.PHONY: run
run: ## Run the analytics service locally in serve mode (D6 uniform contract)
	$(PRINT_TARGET)
	ANALYTICS_HTTP_ADDR=$${ANALYTICS_HTTP_ADDR:-:8082} \
	ANALYTICS_DATABASE_URL=$${ANALYTICS_DATABASE_URL:-postgres://analytics:analytics@localhost:5433/analytics?sslmode=disable} \
	go run ./cmd/analytics serve
