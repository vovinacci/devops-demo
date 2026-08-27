#!/usr/bin/env bash
# Backend tests against a local Postgres (ADR-0019: logic lives in scripts/,
# the Makefile stays an index of entry points). Invoked by `make test-backend`.
#
# Borrows the database: if the compose `db` service is already up this leaves
# it alone, and if it is not, it starts it and stops it again afterwards. The
# pytest exit code is preserved through that cleanup -- a failing test must
# still fail the target.
#
# COMPOSE is passed in by the Makefile so the compose invocation is defined
# in exactly one place.

set -uo pipefail

COMPOSE="${COMPOSE:?COMPOSE must be set by the caller (see Makefile)}"

DB_URL_ASYNC="postgresql+asyncpg://app:app@localhost:5432/appdb"
DB_URL_SYNC="postgresql+psycopg://app:app@localhost:5432/appdb"
READY_TIMEOUT=30

echo "Checking database availability..."
if $COMPOSE ps db 2>/dev/null | grep -qiE "up|running"; then
  db_was_running=yes
else
  db_was_running=no
fi

# Invoked by the EXIT trap below, which shellcheck does not trace.
# shellcheck disable=SC2329
cleanup() {
  if [ "$db_was_running" = "no" ]; then
    echo
    echo "Stopping database started for tests..."
    $COMPOSE stop db || true
  fi
}
trap cleanup EXIT

if [ "$db_was_running" = "no" ]; then
  echo
  echo "Database is not running. Starting DB only..."
  $COMPOSE up -d db || true
  echo "Waiting for DB readiness..."
  # -h 127.0.0.1 forces a TCP probe. Without it pg_isready checks the unix
  # socket, and on a fresh volume the postgres entrypoint runs initdb behind a
  # temporary socket-only server -- so a socket probe reports ready while the
  # port the tests connect on is still closed.
  ready=no
  timeout=$READY_TIMEOUT
  while [ "$timeout" -gt 0 ]; do
    if $COMPOSE exec -T db pg_isready -h 127.0.0.1 -U app -d appdb; then
      ready=yes
      break
    fi
    sleep 1
    timeout=$((timeout - 1))
  done
  if [ "$ready" = "no" ]; then
    echo "Database did not become ready within ${READY_TIMEOUT}s." >&2
    exit 1
  fi
fi

# Fail here rather than letting the tests report it. An unmigrated database
# turns one legible migration error into a wall of "relation does not exist"
# failures that say nothing about the cause.
echo "Running DB migrations..."
(
  cd services/backend || exit 1
  DATABASE_URL="$DB_URL_ASYNC" ALEMBIC_DATABASE_URL="$DB_URL_SYNC" \
    python -m alembic -c alembic.ini upgrade head
) || {
  echo "Database migrations failed; not running tests." >&2
  exit 1
}

echo "Running backend tests via virtualenv..."
(
  cd services/backend || exit 1
  DATABASE_URL="$DB_URL_ASYNC" ALEMBIC_DATABASE_URL="$DB_URL_SYNC" \
    pytest -q
)
test_exit=$?

exit "$test_exit"
