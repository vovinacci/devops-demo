#!/usr/bin/env bash
# Backend tests inside the api container (ADR-0019). Invoked by
# `make test-docker`. Unlike test-backend.sh this does not stop a database it
# started -- the container run is the throwaway part, the stack is not.
#
# COMPOSE is passed in by the Makefile so the compose invocation is defined
# in exactly one place.

set -uo pipefail

COMPOSE="${COMPOSE:?COMPOSE must be set by the caller (see Makefile)}"
CURDIR="${CURDIR:-$(pwd)}"
READY_TIMEOUT=30

echo "Checking database availability..."
if ! $COMPOSE ps db 2>/dev/null | grep -qiE "up|running"; then
  echo "Database is not running. Starting DB..."
  $COMPOSE up -d db || true
  echo "Waiting for DB readiness..."
  timeout=$READY_TIMEOUT
  while [ "$timeout" -gt 0 ]; do
    if $COMPOSE exec -T db pg_isready -U app -d appdb; then
      break
    fi
    sleep 1
    timeout=$((timeout - 1))
  done
fi

echo "Running tests in Docker container..."
$COMPOSE run --rm \
  -v "${CURDIR}/services/backend/tests:/app/tests:ro" \
  api sh -c "pip install --user -e '.[dev]' && python -m alembic -c /app/alembic.ini upgrade head && pytest tests -q -v"
