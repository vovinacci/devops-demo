# Self-contained service Makefile (RFC-0001 D6/D8): identical target set
# across every service -- build, test, lint, run. Root Makefile delegates
# here via lint-canary/test-canary.
.DEFAULT_GOAL := help

PRINT_TARGET = @echo "> make -> $@"

##@ Help

.PHONY: help
help: ## Display this help message
	@awk 'BEGIN {FS = ":.*?## "}; /^[a-zA-Z_-]+:.*?## / { printf "  %-15s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Canary

.PHONY: build
build: ## Compile the release binary
	$(PRINT_TARGET)
	cargo build --release

.PHONY: test
test: ## Run unit + integration tests (journey logic against a mock backend)
	$(PRINT_TARGET)
	cargo test

.PHONY: lint
lint: ## Format check + clippy (deny warnings)
	$(PRINT_TARGET)
	cargo fmt --check
	cargo clippy --all-targets -- -D warnings

.PHONY: format
format: ## Auto-format sources
	$(PRINT_TARGET)
	cargo fmt

.PHONY: run
run: ## Run the canary locally (talks to CANARY_BACKEND_URL, default http://localhost:8000)
	$(PRINT_TARGET)
	cargo run
