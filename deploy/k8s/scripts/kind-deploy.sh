#!/usr/bin/env bash
# Build the service images, load them into Kind, and install the umbrella
# chart (RFC-0003 DK10). Profile-selectable, mirroring `--profile` under
# compose (RFC-0001 D10):
#
#   make kind-deploy                  # core
#   make kind-deploy PROFILE=full     # everything
#   make kind-deploy PROFILE="analytics load"   # combine, as compose does
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

# Space-separated, so profiles COMBINE the way compose's repeated --profile
# flags do: PROFILE="analytics load" is `--profile analytics --profile load`.
# The k6 smoke gate needs exactly that pair, and a single-value PROFILE keeps
# working unchanged.
profiles="${PROFILE:-core}"
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
images="$core_images"
# Additive overlays layer on the core base, one -f each, in the order given.
helm_values=(-f "$umbrella/values.yaml")

for profile in $profiles; do
  case "$profile" in
    core) continue ;;
    analytics) images="$images analytics" ;;
    reports) images="$images reports" ;;
    reports-ui) images="$images reports-ui" ;;
    synthetic) images="$images canary" ;;
    load) images="$images loadgen" ;;
    full) images="$images analytics reports reports-ui canary loadgen" ;;
    *)
      echo "unknown profile '$profile' in PROFILE='$profiles' (want: core|analytics|reports|reports-ui|synthetic|load|full)" >&2
      exit 1
      ;;
  esac
  helm_values+=(-f "$umbrella/values-${profile}.yaml")
done

# The same service can be named by two profiles (full plus a specific one);
# build each image once.
images="$(printf '%s' "$images" | tr ' ' '\n' | sort -u | tr '\n' ' ')"

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

# Workshop mode (RFC-0001 D5): one profile-day compressed into an hour of
# wall-clock time. Under compose this is an environment variable on the whole
# stack; here it is a values override on the two charts that read it. The
# seeder cannot then disagree about scale -- `kubectl exec` runs inside the
# analytics pod and inherits whatever this set.
demo_time_scale="${DEMO_TIME_SCALE:-}"
if [ -n "$demo_time_scale" ] && [ "$demo_time_scale" != "1" ]; then
  echo "==> workshop mode: DEMO_TIME_SCALE=${demo_time_scale}"
  # --set-string: --set types 24 as an int64, and a ConfigMap value must be a
  # string (the library coerces too, but this keeps the rendered YAML honest).
  helm_values+=(--set-string "analytics.config.DEMO_TIME_SCALE=${demo_time_scale}")
  helm_values+=(--set-string "loadgen.config.DEMO_TIME_SCALE=${demo_time_scale}")
fi

echo "==> helm upgrade --install platform (profiles '$profiles', namespace '$namespace')"
helm upgrade --install platform "$umbrella" \
  --kube-context "kind-${cluster_name}" \
  --namespace "$namespace" --create-namespace \
  "${helm_values[@]}" \
  --set gateway.envoyProxyRef=kind-nodeport \
  --wait --timeout 10m

echo
kubectl --context "kind-${cluster_name}" -n "$namespace" get pods
echo
echo "deployed profiles '$profiles'. Reachable through the Gateway on localhost:8080:"
kubectl --context "kind-${cluster_name}" -n "$namespace" get httproute,grpcroute \
  -o custom-columns='KIND:.kind,NAME:.metadata.name,HOSTNAMES:.spec.hostnames'
echo
echo "e.g. curl http://frontend.devops-demo.localhost:8080/"
