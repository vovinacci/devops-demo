#!/usr/bin/env bash
# Delete the local Kind cluster (RFC-0003 DK10). Idempotent.
#
# This deletes the whole cluster, PersistentVolumes included: the local-path
# provisioner keeps its data inside the node containers, so there is nothing
# left to reattach afterwards. `make kind-down && make kind-up` is a factory
# reset, not a restart.
set -euo pipefail

if ! command -v kind >/dev/null 2>&1; then
  if command -v mise >/dev/null 2>&1; then
    exec mise exec -- "$0" "$@"
  fi
  echo "kind is required (pinned in .mise.toml; run 'mise install')" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$repo_root"

cluster_name="$(sed -n 's/^name: \(.*\)$/\1/p' deploy/k8s/kind/cluster.yaml)"

if kind get clusters 2>/dev/null | grep -qx "$cluster_name"; then
  echo "==> deleting Kind cluster '$cluster_name'"
  kind delete cluster --name "$cluster_name"
else
  echo "cluster '$cluster_name' does not exist (nothing to delete)"
fi
