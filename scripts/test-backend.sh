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

if [ "$db_was_running" = "no" ]; then
  echo
  echo "Database is not running. Starting DB only..."
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

# Tolerated on failure, as before: a migration error should surface through
# the tests that then fail, not by masking them with a different exit code.
echo "Running DB migrations..."
(
  cd services/backend || exit 1
  DATABASE_URL="$DB_URL_ASYNC" ALEMBIC_DATABASE_URL="$DB_URL_SYNC" \
    python -m alembic -c alembic.ini upgrade head
) || true

echo "Running backend tests via virtualenv..."
(
  cd services/backend || exit 1
  DATABASE_URL="$DB_URL_ASYNC" ALEMBIC_DATABASE_URL="$DB_URL_SYNC" \
    pytest -q
)
test_exit=$?

if [ "$db_was_running" = "no" ]; then
  echo
  echo "Stopping database started for tests..."
  $COMPOSE stop db || true
fi

exit "$test_exit"
