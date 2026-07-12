# Default target - displays help
.DEFAULT_GOAL := help

# Variable for target name output (DRY principle)
PRINT_TARGET = @echo "▶ make → $@"

# Compose entrypoint: file lives in deploy/compose/, --project-directory keeps
# relative paths and the project name resolving from the repo root
COMPOSE = docker compose -f deploy/compose/docker-compose.yml --project-directory .

# Git hooks runner: prek preferred, classic pre-commit is the documented
# fallback (RFC-0001 D14); both read .pre-commit-config.yaml
PREK = $(shell command -v prek 2>/dev/null || echo pre-commit)

##@ Help

.PHONY: help
help: ## Display this help message
	@echo "Available commands:"
	@echo ""
	@awk 'BEGIN {FS = ":.*?## "}; /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }; /^[a-zA-Z_-]+:.*?## / { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Development

.PHONY: up
up: ## Start all services
	$(PRINT_TARGET)
	$(COMPOSE) up -d --build

.PHONY: down
down: ## Stop all services
	$(PRINT_TARGET)
	$(COMPOSE) down

.PHONY: clean
clean: down ## Complete project cleanup (containers, images, volumes, networks, local artifacts)
	$(PRINT_TARGET)
	@echo "Stopping and removing containers..."
	-$(COMPOSE) down -v --remove-orphans
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

##@ Testing

.PHONY: test
test: test-backend test-frontend ## Run all tests (backend + frontend)

.PHONY: test-backend
test-backend: ## Run backend tests locally via virtualenv
	$(PRINT_TARGET)
	@echo "Checking database availability..."
	DB_WAS_RUNNING=$$($(COMPOSE) ps db 2>/dev/null | grep -q "Up" && echo "yes" || echo "no"); \
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
	cd services/backend && DATABASE_URL="postgresql+asyncpg://app:app@localhost:5432/appdb" \
		ALEMBIC_DATABASE_URL="postgresql+psycopg://app:app@localhost:5432/appdb" \
		python -m alembic -c alembic.ini upgrade head || true; \
	echo "Running backend tests via virtualenv..."; \
	cd services/backend && DATABASE_URL="postgresql+asyncpg://app:app@localhost:5432/appdb" \
		ALEMBIC_DATABASE_URL="postgresql+psycopg://app:app@localhost:5432/appdb" \
		pytest -q; \
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
	if ! $(COMPOSE) ps db 2>/dev/null | grep -q "Up"; then \
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
		-v "$(PWD)/services/backend/tests:/app/tests:ro" \
		api sh -c "pip install --user -e '.[dev]' && python -m alembic -c /app/alembic.ini upgrade head && pytest tests -q -v"

.PHONY: test-frontend
test-frontend: ## Run frontend unit tests
	$(PRINT_TARGET)
	cd services/frontend && ([ -d node_modules ] && [ -f node_modules/.bin/vitest ] || npm install)
	cd services/frontend && npm run test

##@ Code Quality

.PHONY: lint
lint: lint-infra lint-backend type-check lint-frontend ## Code quality check for entire project (backend + frontend + infra)

.PHONY: lint-backend
lint-backend: ## Backend code quality check via ruff
	$(PRINT_TARGET)
	cd services/backend && ruff check .

.PHONY: lint-frontend
lint-frontend: ## Frontend code quality check via eslint
	$(PRINT_TARGET)
	cd services/frontend && ([ -d node_modules ] && [ -f node_modules/.bin/eslint ] || npm install)
	cd services/frontend && npm run lint

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
	docker build -t devops-demo-backend:latest ./services/backend
	docker build -t devops-demo-frontend:latest ./services/frontend

.PHONY: image-sizes
image-sizes: build-images ## Display Docker image sizes
	$(PRINT_TARGET)
	docker images devops-demo-backend:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
	docker images devops-demo-frontend:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"

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
