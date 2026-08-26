#!/usr/bin/env bash
# Build the service images, load them into Kind, and install the umbrella
# chart (RFC-0003 DK10). Profile-selectable, mirroring `--profile` under
# compose (RFC-0001 D10):
#
#   make kind-deploy                  # core
#   make kind-deploy PROFILE=full     # everything
#
# Kind runs no image registry. The charts pin `tag: dev` with the default
# imagePullPolicy of IfNotPresent, so an image that was never loaded is an
# ErrImagePull the kubelet cannot resolve -- `kind load docker-image` is what
# puts the locally built image inside the node containers. This is the
# Kind-ism the runbook calls out (RFC-0003 Section 9): a real cluster pulls
# from a registry instead.
set -euo pipefail

if ! command -v kind >/dev/null 2>&1 || ! command -v kubectl >/dev/null 2>&1 || ! command -v helm >/dev/null 2>&1; then
  if command -v mise >/dev/null 2>&1; then
    exec mise exec -- "$0" "$@"
  fi
  echo "kind, kubectl and helm are required (pinned in .mise.toml; run 'mise install')" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$repo_root"

profile="${PROFILE:-core}"
namespace="${NAMESPACE:-devops-demo}"
umbrella="deploy/k8s/charts/platform"
cluster_name="$(sed -n 's/^name: \(.*\)$/\1/p' deploy/k8s/kind/cluster.yaml)"

if ! kind get clusters 2>/dev/null | grep -qx "$cluster_name"; then
  echo "cluster '$cluster_name' does not exist -- run 'make kind-up' first" >&2
  exit 1
fi

# Images per profile: exactly the built services the profile turns on. Building
# all seven for a core-only deploy would spend minutes on a JVM image the
# release will not reference.
core_images="backend frontend"
case "$profile" in
  core) images="$core_images" ;;
  analytics) images="$core_images analytics" ;;
  reports) images="$core_images reports" ;;
  reports-ui) images="$core_images reports-ui" ;;
  synthetic) images="$core_images canary" ;;
  load) images="$core_images loadgen" ;;
  full) images="$core_images analytics reports reports-ui canary loadgen" ;;
  *)
    echo "unknown PROFILE '$profile' (want: core|analytics|reports|reports-ui|synthetic|load|full)" >&2
    exit 1
    ;;
esac

# Build context per image, mirroring deploy/compose/docker-compose.yml. The
# named extra contexts (proto/, loadprofile/) are the same ones compose passes:
# the gRPC stubs and the load profile are generated or read at build time and
# never committed (RFC-0001 D8, ADR-0002).
build_image() {
  local name="$1"
  local tag="devops-demo/${name}:dev"
  echo "==> building $tag"
  case "$name" in
    backend)
      docker build -t "$tag" --target runtime --build-context proto=./proto ./services/backend
      ;;
    frontend)
      docker build -t "$tag" --target runtime ./services/frontend
      ;;
    analytics)
      docker build -t "$tag" --build-context proto=./proto --build-context loadprofile=./loadprofile ./services/analytics
      ;;
    reports)
      docker build -t "$tag" ./services/reports
      ;;
    reports-ui)
      docker build -t "$tag" ./services/reports-ui
      ;;
    canary)
      docker build -t "$tag" ./services/canary
      ;;
    loadgen)
      docker build -t "$tag" --build-context loadprofile=./loadprofile --build-context proto=./proto ./loadgen
      ;;
    *)
      echo "no build recipe for '$name'" >&2
      exit 1
      ;;
  esac
}

for image in $images; do
  build_image "$image"
done

echo "==> loading images into Kind"
for image in $images; do
  kind load docker-image "devops-demo/${image}:dev" --name "$cluster_name"
done

echo "==> helm dependency build"
helm dependency build "$umbrella" >/dev/null

# Additive profiles LAYER on the core base with a second -f, the same way the
# offline gate renders them (deploy/k8s/scripts/validate.sh).
helm_values=(-f "$umbrella/values.yaml")
if [ "$profile" != "core" ]; then
  helm_values+=(-f "$umbrella/values-${profile}.yaml")
fi

# The EnvoyProxy is Kind-only edge plumbing and has to exist before the
# Gateway references it, so it is applied here rather than rendered by the
# chart -- which keeps the umbrella free of any Kind-specific object
# (deploy/k8s/kind/envoyproxy.yaml explains what it pins and why).
echo "==> applying the Kind NodePort EnvoyProxy"
kubectl --context "kind-${cluster_name}" create namespace "$namespace" \
  --dry-run=client -o yaml | kubectl --context "kind-${cluster_name}" apply -f -
kubectl --context "kind-${cluster_name}" -n "$namespace" apply -f deploy/k8s/kind/envoyproxy.yaml

# Grafana belongs to the kube-prometheus-stack release in `monitoring`, so the
# platform's HTTPRoute for it crosses a namespace boundary and the target
# namespace has to consent (deploy/k8s/kind/referencegrant.yaml).
# Skipped rather than fatal when the observability stack is absent: the grant
# protects a namespace that only exists once kind-up has installed the stack,
# and a platform deploy without it is a legitimate state (the Grafana route
# simply reports ResolvedRefs=False).
if kubectl --context "kind-${cluster_name}" get namespace monitoring >/dev/null 2>&1; then
  echo "==> applying the cross-namespace ReferenceGrant for Grafana"
  sed "s/__PLATFORM_NAMESPACE__/${namespace}/" deploy/k8s/kind/referencegrant.yaml |
    kubectl --context "kind-${cluster_name}" -n monitoring apply -f -
else
  echo "==> monitoring namespace absent -- skipping the Grafana ReferenceGrant"
fi

echo "==> helm upgrade --install platform (profile '$profile', namespace '$namespace')"
helm upgrade --install platform "$umbrella" \
  --kube-context "kind-${cluster_name}" \
  --namespace "$namespace" --create-namespace \
  "${helm_values[@]}" \
  --set gateway.envoyProxyRef=kind-nodeport \
  --wait --timeout 10m

echo
kubectl --context "kind-${cluster_name}" -n "$namespace" get pods
echo
echo "deployed profile '$profile'. Reachable through the Gateway on localhost:8080:"
kubectl --context "kind-${cluster_name}" -n "$namespace" get httproute,grpcroute \
  -o custom-columns='KIND:.kind,NAME:.metadata.name,HOSTNAMES:.spec.hostnames'
echo
echo "e.g. curl http://frontend.devops-demo.localhost:8080/"
