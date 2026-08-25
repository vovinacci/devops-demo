#!/usr/bin/env bash
# Create the local Kind cluster and install the cluster-scoped prerequisites
# the charts assume exist (RFC-0003 DK1, DK10). Idempotent: safe to re-run.
#
# `make kind-up` is the cluster; `make kind-deploy` is the application on top
# of it. The split is deliberate -- the cluster is slow and rarely changes, the
# app is fast and changes constantly, so a redeploy must not rebuild the world.
set -euo pipefail

# Ensure the mise-pinned kind/kubectl are on PATH (self-bootstrap under mise
# once if they are not), so a bare `bash kind-up.sh` works locally too.
if ! command -v kind >/dev/null 2>&1 || ! command -v kubectl >/dev/null 2>&1; then
  if command -v mise >/dev/null 2>&1; then
    exec mise exec -- "$0" "$@"
  fi
  echo "kind and kubectl are required (pinned in .mise.toml; run 'mise install')" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$repo_root"

cluster_config="deploy/k8s/kind/cluster.yaml"
cluster_name="$(sed -n 's/^name: \(.*\)$/\1/p' "$cluster_config")"

# The Prometheus Operator itself is PR-4, but its ServiceMonitor CRD has to
# exist NOW: every D6 service chart renders a ServiceMonitor unconditionally,
# so `helm install` fails on an unknown kind without it. The alternative --
# guarding the template with `.Capabilities.APIVersions.Has` -- is a trap:
# `helm template` has no cluster capabilities, so the guard would silently stop
# rendering ServiceMonitors and the offline kubeconform gate (RFC-0003 DK9,
# the whole point of PR-2) would validate nothing.
#
# This tag pins the CRD *install* only. Which prometheus-operator ships with
# the platform is still PR-4's choice (see deploy/k8s/README.md); PR-4 must
# reconcile this tag, the vendored schema, and its kube-prometheus-stack pick.
PROMETHEUS_OPERATOR_TAG="v0.93.1"
servicemonitor_crd="https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/${PROMETHEUS_OPERATOR_TAG}/example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml"

if kind get clusters 2>/dev/null | grep -qx "$cluster_name"; then
  echo "==> cluster '$cluster_name' already exists (skipping create)"
else
  echo "==> creating Kind cluster '$cluster_name' (1 control-plane + 2 workers)"
  kind create cluster --config "$cluster_config" --wait 120s
fi

# Every subsequent command in this script and in kind-deploy.sh is explicit
# about its context: a developer with several clusters in their kubeconfig
# must never have `make kind-deploy` land somewhere else.
kubectl config use-context "kind-${cluster_name}"

echo "==> waiting for nodes to be Ready"
kubectl wait --for=condition=Ready nodes --all --timeout=120s

# --server-side: the CRD's embedded openAPIV3Schema is far larger than the
# 256 KiB last-applied-configuration annotation a client-side apply would try
# to write, which fails outright on CRDs this size.
echo "==> installing the ServiceMonitor CRD (prometheus-operator ${PROMETHEUS_OPERATOR_TAG})"
kubectl apply --server-side -f "$servicemonitor_crd"

echo
echo "cluster '$cluster_name' is up. Next: make kind-deploy"
