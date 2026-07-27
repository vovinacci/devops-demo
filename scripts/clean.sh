#!/usr/bin/env bash
# Full local teardown (ADR-0019). Invoked by `make clean`, which runs `down`
# first.
#
# Every step is best-effort on purpose: cleanup runs when things are already
# broken or half-removed, so "nothing matched" must not abort the rest. The
# `|| true` on each line is what the Makefile's `-` prefix did before.
# Note that `xargs docker rmi` with no input is itself an error on both BSD
# and GNU xargs, which is the common case here once a resource is gone.
#
# COMPOSE is passed in by the Makefile so the compose invocation is defined
# in exactly one place.

set -uo pipefail

COMPOSE="${COMPOSE:?COMPOSE must be set by the caller (see Makefile)}"

echo "Stopping and removing containers..."
$COMPOSE --profile "*" down -v --remove-orphans || true

echo "Removing project images..."
docker images --filter "reference=devops-demo*" -q | xargs docker rmi -f || true
docker images --filter "reference=*devops-demo*" -q | xargs docker rmi -f || true

echo "Removing project volumes..."
for vol in devops-demo python-devops-demo dbdata loki-data; do
  docker volume ls --filter "name=${vol}" -q | xargs docker volume rm || true
done

echo "Removing project networks..."
for net in devops-demo python-devops-demo devnet; do
  docker network ls --filter "name=${net}" -q | xargs docker network rm || true
done

echo "Cleaning local artifacts..."
for dir in __pycache__ .pytest_cache .ruff_cache .mypy_cache "*.egg-info" htmlcov node_modules dist; do
  find . -type d -name "$dir" -exec rm -rf {} + || true
done
for file in "*.pyc" "*.pyo" .coverage; do
  find . -type f -name "$file" -delete || true
done

echo "Cleanup completed!"
