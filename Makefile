# Default target - displays help
.DEFAULT_GOAL := help

# Variable for target name output (DRY principle)
PRINT_TARGET = @echo "▶ make → $@"

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
	docker compose up -d --build

.PHONY: down
down: ## Stop all services
	$(PRINT_TARGET)
	docker compose down

.PHONY: clean
clean: down ## Complete project cleanup (containers, images, volumes, networks, local artifacts)
	$(PRINT_TARGET)
	@echo "Stopping and removing containers..."
	-docker compose down -v --remove-orphans
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
	docker compose logs -f api

.PHONY: seed
seed: ## Add seed data (20 items)
	$(PRINT_TARGET)
	docker compose run --rm api sh -c "python -m alembic -c /app/alembic.ini upgrade head && python -m app.seed --count 20"

.PHONY: seed-reset
seed-reset: ## Clear and add seed data
	$(PRINT_TARGET)
	docker compose run --rm api sh -c "python -m alembic -c /app/alembic.ini upgrade head && python -m app.seed --only-reset"

.PHONY: seed-dry
seed-dry: ## Dry run seed (show what will be created)
	$(PRINT_TARGET)
	docker compose run --rm api sh -c "python -m app.seed --dry-run --count 10"

##@ Testing

.PHONY: test
test: test-backend test-frontend ## Run all tests (backend + frontend)

.PHONY: test-backend
test-backend: ## Run backend tests locally via virtualenv
	$(PRINT_TARGET)
	@echo "Checking database availability..."
	DB_WAS_RUNNING=$$(docker compose ps db 2>/dev/null | grep -q "Up" && echo "yes" || echo "no"); \
	if [ "$$DB_WAS_RUNNING" = "no" ]; then \
		echo ""; \
		echo "⚠️  Database is not running. Starting DB only..."; \
		docker compose up -d db || true; \
		echo "Waiting for DB readiness..."; \
		timeout=30; \
		while [ $$timeout -gt 0 ]; do \
			if docker compose exec -T db pg_isready -U app -d appdb; then \
				break; \
			fi; \
			sleep 1; \
			timeout=$$((timeout - 1)); \
		done; \
	fi; \
	echo "Running DB migrations..."; \
	cd backend && DATABASE_URL="postgresql+asyncpg://app:app@localhost:5432/appdb" \
		ALEMBIC_DATABASE_URL="postgresql+psycopg://app:app@localhost:5432/appdb" \
		python -m alembic -c alembic.ini upgrade head || true; \
	echo "Running backend tests via virtualenv..."; \
	cd backend && DATABASE_URL="postgresql+asyncpg://app:app@localhost:5432/appdb" \
		ALEMBIC_DATABASE_URL="postgresql+psycopg://app:app@localhost:5432/appdb" \
		pytest -q; \
	TEST_EXIT_CODE=$$?; \
	if [ "$$DB_WAS_RUNNING" = "no" ]; then \
		echo ""; \
		echo "Stopping database started for tests..."; \
		docker compose stop db || true; \
	fi; \
	exit $$TEST_EXIT_CODE

.PHONY: test-docker
test-docker: ## Run tests in Docker container (for CI/CD)
	$(PRINT_TARGET)
	@echo "Checking database availability..."
	if ! docker compose ps db 2>/dev/null | grep -q "Up"; then \
		echo "⚠️  Database is not running. Starting DB..."; \
		docker compose up -d db || true; \
		echo "Waiting for DB readiness..."; \
		timeout=30; \
		while [ $$timeout -gt 0 ]; do \
			if docker compose exec -T db pg_isready -U app -d appdb; then \
				break; \
			fi; \
			sleep 1; \
			timeout=$$((timeout - 1)); \
		done; \
	fi
	@echo "Running tests in Docker container..."
	docker compose run --rm \
		-v "$(PWD)/backend/tests:/app/tests:ro" \
		api sh -c "pip install --user -e '.[dev]' && python -m alembic -c /app/alembic.ini upgrade head && pytest tests -q -v"

.PHONY: test-frontend
test-frontend: ## Run frontend unit tests
	$(PRINT_TARGET)
	cd frontend && ([ -d node_modules ] && [ -f node_modules/.bin/vitest ] || npm install)
	cd frontend && npm run test

##@ Code Quality

.PHONY: lint
lint: lint-infra lint-backend type-check lint-frontend ## Code quality check for entire project (backend + frontend + infra)

.PHONY: lint-backend
lint-backend: ## Backend code quality check via ruff
	$(PRINT_TARGET)
	cd backend && ruff check .

.PHONY: lint-frontend
lint-frontend: ## Frontend code quality check via eslint
	$(PRINT_TARGET)
	cd frontend && ([ -d node_modules ] && [ -f node_modules/.bin/eslint ] || npm install)
	cd frontend && npm run lint

.PHONY: lint-infra
lint-infra: ## CI: Infrastructure validation via pre-commit (single source of truth)
	$(PRINT_TARGET)
	command -v pre-commit >/dev/null 2>&1 || { echo "pre-commit is not installed. Install: pip install pre-commit"; exit 1; }
	pre-commit run --all-files

.PHONY: format
format: format-backend format-frontend ## Format code for entire project (backend + frontend + infra)

.PHONY: format-backend
format-backend: ## Backend code formatting via ruff (format + auto-fix linting)
	$(PRINT_TARGET)
	cd backend && ruff format .
	cd backend && ruff check . --fix || true

.PHONY: format-frontend
format-frontend: ## Frontend code formatting via prettier
	$(PRINT_TARGET)
	cd frontend && ([ -d node_modules ] && [ -f node_modules/.bin/prettier ] || npm install)
	cd frontend && npm run format

.PHONY: type-check
type-check: ## Type checking via mypy
	$(PRINT_TARGET)
	cd backend && mypy app

##@ Pre-commit

.PHONY: pre-commit-install
pre-commit-install: ## Install pre-commit hooks
	$(PRINT_TARGET)
	pre-commit install

.PHONY: pre-commit-run
pre-commit-run: ## Run pre-commit on all files
	$(PRINT_TARGET)
	pre-commit run --all-files

.PHONY: pre-commit-update
pre-commit-update: ## Update pre-commit hooks to latest versions
	$(PRINT_TARGET)
	pre-commit autoupdate

##@ Container images

.PHONY: build-images
build-images: ## Build Docker images
	$(PRINT_TARGET)
	docker build -t devops-demo-backend:latest ./backend
	docker build -t devops-demo-frontend:latest ./frontend

.PHONY: image-sizes
image-sizes: build-images ## Display Docker image sizes
	$(PRINT_TARGET)
	docker images devops-demo-backend:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
	docker images devops-demo-frontend:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"

##@ Frontend

.PHONY: frontend-dev
frontend-dev: ## Run frontend in development mode
	$(PRINT_TARGET)
	cd frontend && npm install && npm run dev

.PHONY: frontend-build
frontend-build: ## Build frontend for production
	$(PRINT_TARGET)
	cd frontend && npm install && npm run build

.PHONY: frontend-lint
frontend-lint: ## Frontend code quality check (eslint)
	$(PRINT_TARGET)
	@echo "Checking npm dependencies..."
	@cd frontend && ([ -d node_modules ] && [ -f node_modules/.bin/eslint ] || npm install)
	@cd frontend && npm run lint

.PHONY: frontend-lint-fix
frontend-lint-fix: ## Fix eslint errors in frontend
	$(PRINT_TARGET)
	@echo "Checking npm dependencies..."
	@cd frontend && ([ -d node_modules ] && [ -f node_modules/.bin/eslint ] || npm install)
	@cd frontend && npm run lint:fix

.PHONY: frontend-format
frontend-format: ## Frontend code formatting (prettier)
	$(PRINT_TARGET)
	@echo "Checking npm dependencies..."
	@cd frontend && ([ -d node_modules ] && [ -f node_modules/.bin/prettier ] || npm install)
	@cd frontend && npm run format

.PHONY: frontend-format-check
frontend-format-check: ## Check frontend formatting (no changes)
	$(PRINT_TARGET)
	@echo "Checking npm dependencies..."
	@cd frontend && ([ -d node_modules ] && [ -f node_modules/.bin/prettier ] || npm install)
	@cd frontend && npm run format:check
