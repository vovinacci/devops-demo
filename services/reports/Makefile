# Self-contained service Makefile (RFC-0001 D6): identical target set
# across every service -- build, test, lint, run. Root Makefile delegates
# here via lint-reports/test-reports. The committed Gradle wrapper drives
# every target; the JDK comes from .mise.toml (ADR-0012), on PATH via mise.
.DEFAULT_GOAL := help

PRINT_TARGET = @echo "> make -> $@"

##@ Help

.PHONY: help
help: ## Display this help message
	@awk 'BEGIN {FS = ":.*?## "}; /^[a-zA-Z_-]+:.*?## / { printf "  %-15s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Reports

.PHONY: build
build: ## Compile the bootable jar
	$(PRINT_TARGET)
	./gradlew --no-daemon clean bootJar

.PHONY: test
test: ## Run tests (integration tests spin up a throwaway Postgres via Testcontainers -- needs Docker)
	$(PRINT_TARGET)
	./gradlew --no-daemon test

.PHONY: lint
lint: ## Format check via ktlint (deny warnings)
	$(PRINT_TARGET)
	./gradlew --no-daemon ktlintCheck

.PHONY: format
format: ## Auto-format sources via ktlint
	$(PRINT_TARGET)
	./gradlew --no-daemon ktlintFormat

.PHONY: run
run: ## Run the reports service locally (REPORTS_* env vars, see README/application.yml)
	$(PRINT_TARGET)
	./gradlew --no-daemon bootRun
