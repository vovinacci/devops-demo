# Default target - displays help
.DEFAULT_GOAL := help

# Variable for target name output (DRY principle)
PRINT_TARGET = @echo "▶ make → $@"

# Compose entrypoint: file lives in deploy/compose/, --project-directory keeps
# relative paths and the project name resolving from the repo root
COMPOSE = docker compose -f deploy/compose/docker-compose.yml --project-directory .

# Git hooks runner: prek preferred, classic pre-commit is the documented
# fallback (ADR-0012); both read .pre-commit-config.yaml
PREK = $(shell command -v prek 2>/dev/null || echo pre-commit)

##@ Help

.PHONY: help
help: ## Display this help message
	@echo "Available commands:"
	@echo ""
	@awk 'BEGIN {FS = ":.*?## "}; /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }; /^[a-zA-Z_-]+:.*?## / { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Development

.PHONY: doctor
doctor: ## Verify local toolchain (versions, Docker, RAM) -- run this first
	$(PRINT_TARGET)
	@bash scripts/doctor.sh

.PHONY: venv-install
venv-install: ## Create backend virtualenv (services/backend/.venv) with dev deps; idempotent
	$(PRINT_TARGET)
	python3 -m venv services/backend/.venv
	services/backend/.venv/bin/pip install --quiet --upgrade pip setuptools wheel
	services/backend/.venv/bin/pip install --quiet -e "services/backend[dev]"
	@echo "Activate with: source services/backend/.venv/bin/activate"

.PHONY: up
up: ## Start all services
	$(PRINT_TARGET)
	$(COMPOSE) up -d --build

.PHONY: up-full
up-full: ## Start all services incl. optional profiles
	$(PRINT_TARGET)
	$(COMPOSE) --profile synthetic --profile analytics --profile reports --profile reports-ui --profile load up -d --build

.PHONY: up-workshop
up-workshop: ## Start all profiles at DEMO_TIME_SCALE=24 (RFC-0001 D5 workshop mode: one profile-day compresses to 1 wall-clock hour). Seed at the SAME scale afterward -- `DEMO_TIME_SCALE=24 make seed-history` -- or loadgen's scale guard refuses to start against the mismatched seed marker (no marker / analytics absent: it continues; docs/exercises/05-find-the-seeded-anomalies.md)
	$(PRINT_TARGET)
	DEMO_TIME_SCALE=24 $(COMPOSE) --profile synthetic --profile analytics --profile reports --profile reports-ui --profile load up -d --build

.PHONY: down
down: ## Stop all services (all profiles; plain "compose down" skips profile-scoped ones)
	$(PRINT_TARGET)
	$(COMPOSE) --profile "*" down

.PHONY: clean
clean: down ## Complete project cleanup (containers, images, volumes, networks, local artifacts)
	$(PRINT_TARGET)
	@echo "Stopping and removing containers..."
	-$(COMPOSE) --profile "*" down -v --remove-orphans
	@echo "Removing project images..."
	-docker images --filter "reference=devops-demo*" -q | xargs docker rmi -f
	-docker images --filter "reference=*devops-demo*" -q | xargs docker rmi -f
	@echo "Removing project volumes..."
	-docker volume ls --filter "name=devops-demo" -q | xargs docker volume rm
	-docker volume ls --filter "name=python-devops-demo" -q | xargs docker volume rm
	-docker volume ls --filter "name=dbdata" -q | xargs docker volume rm
	-docker volume ls --filter "name=loki-data" -q | xargs docker volume rm
	@echo "Removing project networks..."
	-docker network ls --filter "name=devops-demo" -q | xargs docker network rm
	-docker network ls --filter "name=python-devops-demo" -q | xargs docker network rm
	-docker network ls --filter "name=devnet" -q | xargs docker network rm
	@echo "Cleaning local artifacts..."
	-find . -type d -name "__pycache__" -exec rm -rf {} +
	-find . -type d -name ".pytest_cache" -exec rm -rf {} +
	-find . -type d -name ".ruff_cache" -exec rm -rf {} +
	-find . -type d -name ".mypy_cache" -exec rm -rf {} +
	-find . -type d -name "*.egg-info" -exec rm -rf {} +
	-find . -type d -name "htmlcov" -exec rm -rf {} +
	-find . -type f -name "*.pyc" -delete
	-find . -type f -name "*.pyo" -delete
	-find . -type f -name ".coverage" -delete
	-find . -type d -name "node_modules" -exec rm -rf {} +
	-find . -type d -name "dist" -exec rm -rf {} +
	@echo "Cleanup completed!"

.PHONY: logs
logs: ## View API logs (follow mode)
	$(PRINT_TARGET)
	$(COMPOSE) logs -f api

.PHONY: seed
seed: ## Add seed data (20 items)
	$(PRINT_TARGET)
	$(COMPOSE) run --rm api sh -c "python -m alembic -c /app/alembic.ini upgrade head && python -m app.seed --count 20"

.PHONY: seed-reset
seed-reset: ## Clear and add seed data
	$(PRINT_TARGET)
	$(COMPOSE) run --rm api sh -c "python -m alembic -c /app/alembic.ini upgrade head && python -m app.seed --only-reset"

.PHONY: seed-dry
seed-dry: ## Dry run seed (show what will be created)
	$(PRINT_TARGET)
	$(COMPOSE) run --rm api sh -c "python -m app.seed --dry-run --count 10"

.PHONY: seed-history
seed-history: seed ## Seed analytics historical data (RFC-0001 Phase 5 D5): run AFTER `make up`/`make up-full`/`make up-workshop` with the analytics profile already up (postgres-analytics + api reachable) -- --no-deps below means this does NOT start them for you. SEED_DAYS/SEED_SEED override the defaults (90/42); SEED_DAYS=3 for a quick local check. After `make up-workshop`, run as `DEMO_TIME_SCALE=24 make seed-history` -- the compose `run` below reads DEMO_TIME_SCALE from this shell's environment same as any other `docker compose` invocation, so it must match the stack's scale or loadgen's scale guard refuses to start against the mismatched marker (D5 coherence; missing marker or absent analytics: loadgen continues).
	$(PRINT_TARGET)
	$(COMPOSE) --profile analytics run --rm --no-deps analytics \
		seed --days $${SEED_DAYS:-90} --seed $${SEED_SEED:-42}

.PHONY: generate
generate: generate-backend generate-analytics ## Generate gRPC/protobuf stubs (Python + Go, never committed -- RFC-0001 D8, ADR-0002)

# Split per language so a job that needs only one toolchain installs only
# that toolchain: generate-backend needs grpcio-tools (bundled protoc),
# generate-analytics needs buf. `generate` above stays the local
# regenerate-everything entry point.
.PHONY: generate-backend
generate-backend: ## Generate Python gRPC/protobuf stubs only (grpcio-tools; no buf needed)
	$(PRINT_TARGET)
	rm -rf services/backend/app/proto_gen
	mkdir -p services/backend/app/proto_gen
	python -m grpc_tools.protoc -I proto \
		--python_out=services/backend/app/proto_gen \
		--pyi_out=services/backend/app/proto_gen \
		--grpc_python_out=services/backend/app/proto_gen \
		proto/devopsdemo/items/v1/items.proto

.PHONY: generate-analytics
generate-analytics: ## Generate Go gRPC/protobuf stubs only (buf)
	$(PRINT_TARGET)
	rm -rf services/analytics/internal/pb
	$(MAKE) -C services/analytics generate

##@ Load generation

.PHONY: incident
incident: ## One-shot k6 incident overlay (INCIDENT_MODE=spike|errors, INCIDENT_MINUTES numeric, default 5); `make heal` stops it early
	$(PRINT_TARGET)
	$(COMPOSE) --profile load run --rm --no-deps --name loadgen-incident \
		-e INCIDENT_MODE=$${INCIDENT_MODE:-spike} \
		-e INCIDENT_MINUTES=$${INCIDENT_MINUTES:-5} \
		-e INCIDENT_SPIKE_MULTIPLIER=$${INCIDENT_SPIKE_MULTIPLIER:-10} \
		-e INCIDENT_ERROR_RATE_PER_S=$${INCIDENT_ERROR_RATE_PER_S:-5} \
		-e K6_PROMETHEUS_RW_SERVER_URL=http://prometheus:9090/api/v1/write \
		-e "K6_PROMETHEUS_RW_TREND_STATS=p(95),p(99),avg" \
		loadgen run -o experimental-prometheus-rw /home/k6/scenarios/incident.js

.PHONY: heal
heal: ## Stop a running `make incident` overlay early (safe to run even if nothing is running)
	$(PRINT_TARGET)
	-docker rm -f loadgen-incident

##@ E2E

.PHONY: smoke
smoke: ## e2e CI stage, runnable locally: bring up core+analytics+load, run the k6 smoke gate, assert wiring (RFC-0001 D12; .github/workflows/e2e.yml runs exactly this)
	$(PRINT_TARGET)
	$(COMPOSE) --profile analytics --profile load up -d --build --wait --wait-timeout 180
	@# --no-deps on every one-shot `compose run loadgen` below: without it,
	@# compose re-evaluates loadgen's depends_on and recreates a running
	@# api on a build-context hash mismatch -- MID-RUN, killing in-flight
	@# requests (observed as a connection-refused burst failing the very
	@# thresholds this gate exists to enforce). The stack is already up
	@# from the line above; the one-shot needs nothing started for it.
	$(COMPOSE) --profile load run --rm --no-deps \
		-e K6_PROMETHEUS_RW_SERVER_URL=http://prometheus:9090/api/v1/write \
		-e "K6_PROMETHEUS_RW_TREND_STATS=p(95),p(99),avg" \
		loadgen run -o experimental-prometheus-rw /home/k6/scenarios/smoke.js
	bash scripts/e2e-smoke.sh
	@echo "Stack left running -- tear down with: make down"

.PHONY: smoke-full
smoke-full: ## nightly CI stage, runnable locally: all profiles incl. reports/JVM and reports-ui/Caddy (core+analytics+synthetic+reports+reports-ui+load), a longer k6 smoke run WITH the report scenario, canary/blackbox assertions (RFC-0001 D12; .github/workflows/nightly.yml runs exactly this)
	$(PRINT_TARGET)
	@# --profile reports (RFC-0001 D12) + --profile reports-ui (RFC-0002): the
	@# JVM and the Caddy static frontend over it stay OUT of the per-PR `smoke`
	@# target and are exercised here nightly/full instead. Longer --wait-timeout
	@# than `smoke`: reports has the slowest start of any service (JVM boot +
	@# Flyway; healthcheck start_period 40s).
	$(COMPOSE) --profile analytics --profile synthetic --profile reports --profile reports-ui --profile load up -d --build --wait --wait-timeout 300
	@# LOADGEN_REPORTS_URL is the enable-signal (loadgen/lib/env.js): setting
	@# it here -- and ONLY here, never in `smoke` -- is what schedules the
	@# `report` scenario in smoke.js. Unset in the per-PR gate, absent from
	@# options.scenarios, JVM untouched.
	$(COMPOSE) --profile load run --rm --no-deps \
		-e SMOKE_DURATION_SECONDS=$${SMOKE_DURATION_SECONDS:-300} \
		-e LOADGEN_REPORTS_URL=http://reports:8083 \
		-e K6_PROMETHEUS_RW_SERVER_URL=http://prometheus:9090/api/v1/write \
		-e "K6_PROMETHEUS_RW_TREND_STATS=p(95),p(99),avg" \
		loadgen run -o experimental-prometheus-rw /home/k6/scenarios/smoke.js
	NIGHTLY=1 bash scripts/e2e-smoke.sh
	@echo "Stack left running -- tear down with: make down"

##@ Testing

.PHONY: test
test: test-backend test-frontend test-canary test-analytics test-reports test-reports-ui ## Run all tests (backend + frontend + canary + analytics + reports + reports-ui)

.PHONY: test-backend
test-backend: generate-backend ## Run backend tests locally via virtualenv
	$(PRINT_TARGET)
	@echo "Checking database availability..."
	DB_WAS_RUNNING=$$($(COMPOSE) ps db 2>/dev/null | grep -qiE "up|running" && echo "yes" || echo "no"); \
	if [ "$$DB_WAS_RUNNING" = "no" ]; then \
		echo ""; \
		echo "⚠️  Database is not running. Starting DB only..."; \
		$(COMPOSE) up -d db || true; \
		echo "Waiting for DB readiness..."; \
		timeout=30; \
		while [ $$timeout -gt 0 ]; do \
			if $(COMPOSE) exec -T db pg_isready -U app -d appdb; then \
				break; \
			fi; \
			sleep 1; \
			timeout=$$((timeout - 1)); \
		done; \
	fi; \
	echo "Running DB migrations..."; \
	( cd services/backend && DATABASE_URL="postgresql+asyncpg://app:app@localhost:5432/appdb" \
		ALEMBIC_DATABASE_URL="postgresql+psycopg://app:app@localhost:5432/appdb" \
		python -m alembic -c alembic.ini upgrade head ) || true; \
	echo "Running backend tests via virtualenv..."; \
	( cd services/backend && DATABASE_URL="postgresql+asyncpg://app:app@localhost:5432/appdb" \
		ALEMBIC_DATABASE_URL="postgresql+psycopg://app:app@localhost:5432/appdb" \
		pytest -q ); \
	TEST_EXIT_CODE=$$?; \
	if [ "$$DB_WAS_RUNNING" = "no" ]; then \
		echo ""; \
		echo "Stopping database started for tests..."; \
		$(COMPOSE) stop db || true; \
	fi; \
	exit $$TEST_EXIT_CODE

.PHONY: test-docker
test-docker: ## Run tests in Docker container (for CI/CD)
	$(PRINT_TARGET)
	@echo "Checking database availability..."
	if ! $(COMPOSE) ps db 2>/dev/null | grep -qiE "up|running"; then \
		echo "⚠️  Database is not running. Starting DB..."; \
		$(COMPOSE) up -d db || true; \
		echo "Waiting for DB readiness..."; \
		timeout=30; \
		while [ $$timeout -gt 0 ]; do \
			if $(COMPOSE) exec -T db pg_isready -U app -d appdb; then \
				break; \
			fi; \
			sleep 1; \
			timeout=$$((timeout - 1)); \
		done; \
	fi
	@echo "Running tests in Docker container..."
	$(COMPOSE) run --rm \
		-v "$(CURDIR)/services/backend/tests:/app/tests:ro" \
		api sh -c "pip install --user -e '.[dev]' && python -m alembic -c /app/alembic.ini upgrade head && pytest tests -q -v"

.PHONY: test-frontend
test-frontend: ## Run frontend unit tests
	$(PRINT_TARGET)
	cd services/frontend && ([ -d node_modules ] && [ -f node_modules/.bin/vitest ] || npm install)
	cd services/frontend && npm run test

.PHONY: test-canary
test-canary: ## Run canary tests (Rust toolchain from mise -- see .mise.toml)
	$(PRINT_TARGET)
	$(MAKE) -C services/canary test

.PHONY: test-analytics
test-analytics: generate ## Run analytics tests (Go toolchain from mise -- see .mise.toml)
	$(PRINT_TARGET)
	$(MAKE) -C services/analytics test

.PHONY: test-reports
test-reports: ## Run reports tests (JDK/Gradle from mise -- see .mise.toml; Testcontainers needs Docker)
	$(PRINT_TARGET)
	$(MAKE) -C services/reports test

.PHONY: test-reports-ui
test-reports-ui: ## Validate the reports-ui Caddyfile + container smoke (/healthz, /metrics; needs Docker)
	$(PRINT_TARGET)
	$(MAKE) -C services/reports-ui test

##@ Code Quality

.PHONY: lint
lint: lint-infra lint-backend type-check lint-frontend lint-canary lint-analytics lint-reports lint-reports-ui lint-proto parity ## Code quality check for entire project (backend + frontend + canary + analytics + reports + reports-ui + proto + infra + load profile parity)

.PHONY: lint-backend
lint-backend: ## Backend code quality check via ruff
	$(PRINT_TARGET)
	cd services/backend && ruff check .

.PHONY: lint-frontend
lint-frontend: ## Frontend code quality check via eslint
	$(PRINT_TARGET)
	cd services/frontend && ([ -d node_modules ] && [ -f node_modules/.bin/eslint ] || npm install)
	cd services/frontend && npm run lint

.PHONY: lint-canary
lint-canary: ## Canary code quality check via cargo fmt + clippy (Rust toolchain from mise)
	$(PRINT_TARGET)
	$(MAKE) -C services/canary lint

.PHONY: lint-analytics
lint-analytics: generate ## Analytics code quality check via gofmt + go vet + golangci-lint (Go toolchain from mise)
	$(PRINT_TARGET)
	$(MAKE) -C services/analytics lint

.PHONY: lint-reports
lint-reports: ## Reports code quality check via ktlint (JDK/Gradle from mise)
	$(PRINT_TARGET)
	$(MAKE) -C services/reports lint

.PHONY: lint-reports-ui
lint-reports-ui: ## Reports UI check: caddy fmt + validate (via the caddy image) + ASCII-only assets
	$(PRINT_TARGET)
	$(MAKE) -C services/reports-ui lint

.PHONY: lint-proto
lint-proto: ## proto/ code quality check via buf lint + buf format --diff
	$(PRINT_TARGET)
	cd proto && buf lint
	cd proto && buf format --diff --exit-code

.PHONY: parity
parity: ## Load profile parity: JS (loadprofile/shape.js) + Go (loadshape) vs goldens (Hard rule 8)
	$(PRINT_TARGET)
	node loadprofile/parity/compare.mjs
	$(MAKE) -C services/analytics test-loadshape

.PHONY: lint-infra
lint-infra: ## CI: Infrastructure validation via prek hooks (single source of truth)
	$(PRINT_TARGET)
	command -v $(PREK) >/dev/null 2>&1 || { echo "prek is not installed. Install: see docs/ci.md (fallback: pip install pre-commit)"; exit 1; }
	$(PREK) run --all-files

.PHONY: format
format: format-backend format-frontend ## Format code for entire project (backend + frontend + infra)

.PHONY: format-backend
format-backend: ## Backend code formatting via ruff (format + auto-fix linting)
	$(PRINT_TARGET)
	cd services/backend && ruff format .
	cd services/backend && ruff check . --fix || true

.PHONY: format-frontend
format-frontend: ## Frontend code formatting via prettier
	$(PRINT_TARGET)
	cd services/frontend && ([ -d node_modules ] && [ -f node_modules/.bin/prettier ] || npm install)
	cd services/frontend && npm run format

.PHONY: type-check
type-check: ## Type checking via mypy
	$(PRINT_TARGET)
	cd services/backend && mypy app

##@ CI

.PHONY: ci
ci: ## Run exactly what CI runs (hooks + lint + tests); divergence from CI is a bug
	$(PRINT_TARGET)
	$(MAKE) lint
	$(MAKE) test

##@ Git hooks

.PHONY: pre-commit-install
pre-commit-install: ## Install git hooks (prek or pre-commit)
	$(PRINT_TARGET)
	$(PREK) install

.PHONY: pre-commit-run
pre-commit-run: ## Run hooks on all files
	$(PRINT_TARGET)
	$(PREK) run --all-files

.PHONY: pre-commit-update
pre-commit-update: ## Update hook versions in .pre-commit-config.yaml
	$(PRINT_TARGET)
	$(PREK) autoupdate

##@ Container images

.PHONY: build-images
build-images: ## Build Docker images
	$(PRINT_TARGET)
	docker build --build-context proto=proto -t devops-demo-backend:latest ./services/backend
	docker build -t devops-demo-frontend:latest ./services/frontend
	docker build -t devops-demo-canary:latest ./services/canary
	docker build --build-context proto=proto -t devops-demo-analytics:latest ./services/analytics
	docker build -t devops-demo-reports:latest ./services/reports
	docker build -t devops-demo-reports-ui:latest ./services/reports-ui

.PHONY: image-sizes
image-sizes: build-images ## Display Docker image sizes (the canary/analytics vs the rest is the D9 "costs less" exhibit)
	$(PRINT_TARGET)
	docker images devops-demo-backend:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
	docker images devops-demo-frontend:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
	docker images devops-demo-canary:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
	docker images devops-demo-analytics:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
	docker images devops-demo-reports:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
	docker images devops-demo-reports-ui:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"

##@ Frontend

.PHONY: frontend-dev
frontend-dev: ## Run frontend in development mode
	$(PRINT_TARGET)
	cd services/frontend && npm install && npm run dev

.PHONY: frontend-build
frontend-build: ## Build frontend for production
	$(PRINT_TARGET)
	cd services/frontend && npm install && npm run build

.PHONY: frontend-lint
frontend-lint: ## Frontend code quality check (eslint)
	$(PRINT_TARGET)
	@echo "Checking npm dependencies..."
	@cd services/frontend && ([ -d node_modules ] && [ -f node_modules/.bin/eslint ] || npm install)
	@cd services/frontend && npm run lint

.PHONY: frontend-lint-fix
frontend-lint-fix: ## Fix eslint errors in frontend
	$(PRINT_TARGET)
	@echo "Checking npm dependencies..."
	@cd services/frontend && ([ -d node_modules ] && [ -f node_modules/.bin/eslint ] || npm install)
	@cd services/frontend && npm run lint:fix

.PHONY: frontend-format
frontend-format: ## Frontend code formatting (prettier)
	$(PRINT_TARGET)
	@echo "Checking npm dependencies..."
	@cd services/frontend && ([ -d node_modules ] && [ -f node_modules/.bin/prettier ] || npm install)
	@cd services/frontend && npm run format

.PHONY: frontend-format-check
frontend-format-check: ## Check frontend formatting (no changes)
	$(PRINT_TARGET)
	@echo "Checking npm dependencies..."
	@cd services/frontend && ([ -d node_modules ] && [ -f node_modules/.bin/prettier ] || npm install)
	@cd services/frontend && npm run format:check
