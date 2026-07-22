#!/usr/bin/env bash
# Toolchain drift gate: .mise.toml is the single source of truth for
# runtime versions. Some consumers cannot import a value from another
# file (Dockerfile FROM, devcontainer image strings, .nvmrc), so the
# duplication is physical -- this gate makes it un-driftable, the same
# construction as the load-profile parity test (ADR-0003).
# Runs as a prek hook locally and in CI (same config).

set -euo pipefail

canon_python="$(sed -n 's/^python = "\(.*\)"/\1/p' .mise.toml)"
canon_node="$(sed -n 's/^node = "\(.*\)"/\1/p' .mise.toml)"
canon_rust="$(sed -n 's/^rust = "\(.*\)"/\1/p' .mise.toml)"
canon_go="$(sed -n 's/^go = "\(.*\)"/\1/p' .mise.toml)"
canon_k6="$(sed -n 's/^k6 = "\(.*\)"/\1/p' .mise.toml)"

if [ -z "$canon_python" ] || [ -z "$canon_node" ] || [ -z "$canon_rust" ] || [ -z "$canon_go" ] || [ -z "$canon_k6" ]; then
  echo "cannot read python/node/rust/go/k6 pins from .mise.toml" >&2
  exit 1
fi

drift=0

# actual must equal the canonical value or refine it (3.12.13 vs 3.12)
expect() {
  local label="$1" actual="$2" want="$3"
  if [ -z "$actual" ]; then
    printf 'DRIFT %s: value not found (canon %s)\n' "$label" "$want"
    drift=$((drift + 1))
    return
  fi
  case "$actual" in
    "$want" | "$want".*)
      printf 'ok    %s: %s\n' "$label" "$actual"
      ;;
    *)
      printf 'DRIFT %s: %s (canon %s)\n' "$label" "$actual" "$want"
      drift=$((drift + 1))
      ;;
  esac
}

expect ".nvmrc" \
  "$(tr -d '[:space:]' <services/frontend/.nvmrc)" "$canon_node"

expect "devcontainer image (python)" \
  "$(sed -n 's/.*devcontainers\/python:1-\([0-9.]*\)-.*/\1/p' .devcontainer/devcontainer.json)" \
  "$canon_python"

expect "devcontainer node feature" \
  "$(python3 -c 'import json; print(json.load(open(".devcontainer/devcontainer.json"))["features"]["ghcr.io/devcontainers/features/node:1"]["version"])' 2>/dev/null)" \
  "$canon_node"

expect "backend Dockerfile FROM" \
  "$(sed -n 's/^FROM python:\([0-9][0-9.]*\).*/\1/p' services/backend/Dockerfile | head -n1)" \
  "$canon_python"

expect "frontend Dockerfile FROM" \
  "$(sed -n 's/^FROM node:\([0-9][0-9.]*\).*/\1/p' services/frontend/Dockerfile | head -n1)" \
  "$canon_node"

expect "canary Dockerfile FROM" \
  "$(sed -n 's/^FROM rust:\([0-9][0-9.]*\).*/\1/p' services/canary/Dockerfile | head -n1)" \
  "$canon_rust"

expect "analytics Dockerfile FROM" \
  "$(sed -n 's/^FROM golang:\([0-9][0-9.]*\).*/\1/p' services/analytics/Dockerfile | head -n1)" \
  "$canon_go"

expect "pyproject requires-python floor" \
  "$(sed -n 's/^requires-python = ">=\(.*\)"/\1/p' services/backend/pyproject.toml)" \
  "$canon_python"

expect "loadgen Dockerfile FROM" \
  "$(sed -n 's/^FROM grafana\/k6:\([0-9][0-9.]*\).*/\1/p' loadgen/Dockerfile | head -n1)" \
  "$canon_k6"

if [ "$drift" -gt 0 ]; then
  echo
  echo "toolchain drift: ${drift} consumer(s) disagree with .mise.toml."
  echo "Bump .mise.toml first, then align every file listed above."
  exit 1
fi
