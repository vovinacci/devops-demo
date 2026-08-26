#!/usr/bin/env bash
# Stop a running `make kind-incident` overlay early (RFC-0003 DK10), the Kind
# counterpart of `make heal`. Safe to run when nothing is running, exactly as
# the compose target is.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$repo_root"

namespace="${NAMESPACE:-devops-demo}"
cluster_name="$(sed -n 's/^name: \(.*\)$/\1/p' deploy/k8s/kind/cluster.yaml)"

if ! kind get clusters 2>/dev/null | grep -qx "$cluster_name"; then
  echo "cluster '$cluster_name' does not exist (nothing to heal)"
  exit 0
fi

kubectl --context "kind-${cluster_name}" -n "$namespace" \
  delete job platform-incident --ignore-not-found
