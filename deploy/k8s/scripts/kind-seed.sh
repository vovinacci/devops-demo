#!/usr/bin/env bash
# Seed the deployed stack (RFC-0003 DK10), the Kubernetes counterpart of
# `make seed` and `make seed-history`.
#
# The SCHEMA is not this script's job -- the backend chart's Helm hook Job
# runs `alembic upgrade head` on every install and upgrade. This adds DATA:
# items in the backend, and analytics' seeded history when that profile is
# deployed.
#
# One thing is easier here than under compose. `make seed-history` has to be
# run with the same DEMO_TIME_SCALE the stack was started with, by hand, or
# loadgen's scale guard refuses to start against a mismatched seed marker
# (RFC-0001 D5). `kubectl exec` runs INSIDE the analytics pod, which already
# carries DEMO_TIME_SCALE from its ConfigMap -- the scales cannot disagree.
set -euo pipefail

if ! command -v kubectl >/dev/null 2>&1; then
  if command -v mise >/dev/null 2>&1; then
    exec mise exec -- "$0" "$@"
  fi
  echo "kubectl is required (pinned in .mise.toml; run 'mise install')" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$repo_root"

namespace="${NAMESPACE:-devops-demo}"
cluster_name="$(sed -n 's/^name: \(.*\)$/\1/p' deploy/k8s/kind/cluster.yaml)"
context="kind-${cluster_name}"
kc=(kubectl --context "$context" --namespace "$namespace")

if ! "${kc[@]}" get deployment platform-backend >/dev/null 2>&1; then
  echo "backend is not deployed in namespace '$namespace' -- run 'make kind-deploy' first" >&2
  exit 1
fi

echo "==> waiting for the backend to be ready"
"${kc[@]}" rollout status deployment/platform-backend --timeout=120s

echo "==> seeding items (${SEED_COUNT:-20})"
"${kc[@]}" exec deployment/platform-backend -- \
  python -m app.seed --count "${SEED_COUNT:-20}"

# analytics is an opt-in profile; seeding history is a no-op when it is not
# deployed, the same graceful degradation the rest of the stack uses
# (RFC-0001 D10).
if "${kc[@]}" get deployment platform-analytics >/dev/null 2>&1; then
  echo "==> waiting for analytics to be ready"
  "${kc[@]}" rollout status deployment/platform-analytics --timeout=120s

  echo "==> seeding analytics history (${SEED_DAYS:-90} days, seed ${SEED_SEED:-42})"
  "${kc[@]}" exec deployment/platform-analytics -- \
    /usr/local/bin/analytics seed --days "${SEED_DAYS:-90}" --seed "${SEED_SEED:-42}"
else
  echo "==> analytics is not deployed (profile off) -- skipping history seed"
fi

echo
echo "seeded. Dashboards: http://grafana.devops-demo.localhost:8080/"
