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
if ! command -v kind >/dev/null 2>&1 || ! command -v kubectl >/dev/null 2>&1 || ! command -v helm >/dev/null 2>&1; then
  if command -v mise >/dev/null 2>&1; then
    exec mise exec -- "$0" "$@"
  fi
  echo "kind, kubectl and helm are required (pinned in .mise.toml; run 'mise install')" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$repo_root"

cluster_config="deploy/k8s/kind/cluster.yaml"
cluster_name="$(sed -n 's/^name: \(.*\)$/\1/p' "$cluster_config")"

# Kubernetes-native observability (RFC-0003 DK6, ADR-0017). This chart brings
# the Prometheus Operator, Prometheus, Alertmanager, Grafana, kube-state-metrics
# and node-exporter -- and, importantly, every monitoring.coreos.com CRD, so the
# standalone ServiceMonitor CRD install PR-3 needed is gone.
#
# Pinned deliberately: chart 88.5.4 ships operator v0.93.1, which is the version
# deploy/k8s/schemas/monitoring.coreos.com/ is vendored from. Those two move
# together -- see deploy/k8s/README.md.
KUBE_PROMETHEUS_STACK_VERSION="88.5.4"
ENVOY_GATEWAY_VERSION="1.9.1"

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

echo "==> installing kube-prometheus-stack ${KUBE_PROMETHEUS_STACK_VERSION} (Operator, Prometheus, Alertmanager, Grafana)"
# --force-update: `helm repo add` fails when the name already exists with a
# different URL, which would abort an otherwise idempotent kind-up.
helm repo add --force-update prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null
helm repo update prometheus-community >/dev/null
helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  --version "$KUBE_PROMETHEUS_STACK_VERSION" \
  --kube-context "kind-${cluster_name}" \
  --namespace monitoring --create-namespace \
  -f deploy/k8s/kind/kube-prometheus-stack-values.yaml \
  --wait --timeout 10m

echo "==> installing Envoy Gateway ${ENVOY_GATEWAY_VERSION} (Gateway API CRDs included)"
helm upgrade --install eg oci://docker.io/envoyproxy/gateway-helm \
  --version "$ENVOY_GATEWAY_VERSION" \
  --kube-context "kind-${cluster_name}" \
  --namespace envoy-gateway-system --create-namespace \
  --wait --timeout 5m

# Cluster-scoped and shared by every release, so it is installed with the
# controller rather than rendered by the umbrella chart.
echo "==> installing the GatewayClass"
kubectl apply -f deploy/k8s/kind/gatewayclass.yaml

echo
echo "cluster '$cluster_name' is up. Next: make kind-deploy"
