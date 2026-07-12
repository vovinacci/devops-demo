#!/usr/bin/env bash
# postCreate.sh -- runs once after the devcontainer is built.
# Idempotent: safe to re-run.

set -euo pipefail

echo "== Installing backend (editable) with dev extras..."
python -m pip install --upgrade pip setuptools wheel
pip install -e "./services/backend[dev]"

echo "== Installing git hooks runner and project hooks..."
pip install pre-commit
pre-commit install --install-hooks || echo "  (skipped: not a git repo or hooks already installed)"

echo "== Installing frontend dependencies..."
(
  cd services/frontend
  if ! npm ci; then npm install; fi
)

echo "== Doctor..."
make doctor || true

cat <<'EOF'

devops-demo container is ready.

Try:
  make doctor    # verify the toolchain
  make up        # bring up the whole stack
  make seed      # add 20 demo items
  make test      # run backend + frontend tests

Forwarded ports:
  Frontend   -> 8080
  API        -> 8000   (Swagger at /docs)
  Grafana    -> 3000   (admin/admin)
  Prometheus -> 9090
EOF
