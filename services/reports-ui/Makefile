# Self-contained service Makefile (RFC-0001 D6): identical target set across
# every service -- build, test, lint, run. Root Makefile delegates here via
# lint-reports-ui/test-reports-ui. reports-ui has no host toolchain (RFC-0002
# D3: no build step); the Caddy image itself runs fmt/validate, so no `caddy`
# binary is needed on PATH.
.DEFAULT_GOAL := help

PRINT_TARGET = @echo "> make -> $@"

# Digest-pinned Caddy image (matches the Dockerfile FROM). Every fmt/validate
# runs through it -- no host caddy install required.
CADDY_IMAGE = caddy:2-alpine@sha256:5f5c8640aae01df9654968d946d8f1a56c497f1dd5c5cda4cf95ab7c14d58648
CADDY = docker run --rm -v "$(CURDIR):/work" -w /work $(CADDY_IMAGE) caddy

IMAGE = devops-demo-reports-ui:dev

##@ Help

.PHONY: help
help: ## Display this help message
	@awk 'BEGIN {FS = ":.*?## "}; /^[a-zA-Z_-]+:.*?## / { printf "  %-15s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Reports UI

.PHONY: build
build: ## Build the container image (caddy validate runs in the build)
	$(PRINT_TARGET)
	docker build --pull -t $(IMAGE) .

.PHONY: test
test: ## Validate the Caddyfile, then a container smoke check (/healthz, /metrics)
	$(PRINT_TARGET)
	$(CADDY) validate --config Caddyfile
	docker build --pull -t $(IMAGE) .
	@echo "Smoke: starting container, probing /healthz and /metrics..."
	@cid=$$(docker run -d -p 18084:8084 $(IMAGE)); \
	ok=1; \
	for i in $$(seq 1 20); do \
		if curl -fsS http://localhost:18084/healthz >/dev/null 2>&1; then break; fi; \
		sleep 0.5; \
	done; \
	curl -fsS http://localhost:18084/healthz >/dev/null 2>&1 && echo "  /healthz OK" || { echo "  /healthz FAILED"; ok=0; }; \
	curl -fsS http://localhost:18084/metrics 2>/dev/null | grep -q '^caddy_' && echo "  /metrics OK (caddy_* present)" || { echo "  /metrics FAILED"; ok=0; }; \
	docker rm -f $$cid >/dev/null 2>&1; \
	[ $$ok -eq 1 ]

.PHONY: lint
lint: ## caddy fmt (no-diff) + caddy validate + ASCII-only site assets
	$(PRINT_TARGET)
	@echo "caddy fmt (must be a no-op)..."
	@$(CADDY) fmt Caddyfile > .Caddyfile.fmt; \
	diff Caddyfile .Caddyfile.fmt; rc=$$?; rm -f .Caddyfile.fmt; \
	[ $$rc -eq 0 ] || { echo "Caddyfile not formatted -- run 'make format'"; exit 1; }
	$(CADDY) validate --config Caddyfile
	@echo "ASCII-only check on site assets + Caddyfile..."
	@if LC_ALL=C grep -RPln '[^\x00-\x7f]' Caddyfile site/ 2>/dev/null | grep .; then \
		echo "non-ASCII bytes found above"; exit 1; \
	else echo "  ASCII OK"; fi

.PHONY: format
format: ## Auto-format the Caddyfile in place
	$(PRINT_TARGET)
	$(CADDY) fmt --overwrite Caddyfile

.PHONY: run
run: ## Run the service locally on :8084 (compose profile reports-ui)
	$(PRINT_TARGET)
	docker compose -f ../../deploy/compose/docker-compose.yml --project-directory ../.. \
		--profile reports-ui up --build reports-ui
