#!/usr/bin/env bash
# The Kubernetes e2e gate (RFC-0003 PR-5): create a cluster, deploy, seed, run
# the k6 smoke scenario against it, assert the wiring, tear it down.
#
# This is the Kind counterpart of `make smoke`, and deliberately runs the SAME
# assertions -- scripts/e2e-smoke.sh with its addresses pointed at the Gateway
# instead of compose's published ports. Two stacks, one definition of
# "working": an assertion that holds on one and not the other is a real
# difference, not a difference in how each is tested.
#
# Runs the FULL profile set, which the compose per-PR gate cannot afford. Two
# reasons, both measured rather than assumed:
#
#   core + analytics + load + observability stack   3.36 GiB
#   full + observability stack                      4.22 GiB
#
# across the three Kind nodes, against the 7 GB standard runner the CI budget
# assumes (RFC-0001 D12). RFC-0003 Section 9 estimated 6-10 GB for this and
# treated it as the arc's main risk; it is not one.
#
# The second reason is coverage. The reports-ui assertions -- including the
# /api call through its in-image reverse proxy -- only run with the reports
# and reports-ui profiles up. A leaner profile set would skip precisely the
# check that exists because that proxy shipped misaddressed to Kubernetes.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$repo_root"

profiles="${PROFILE:-full}"
namespace="${NAMESPACE:-devops-demo}"
domain="${GATEWAY_DOMAIN:-devops-demo.localhost}"
gateway_port="${GATEWAY_PORT:-8080}"
keep="${KEEP_CLUSTER:-0}"
cluster_name="$(sed -n 's/^name: \(.*\)$/\1/p' deploy/k8s/kind/cluster.yaml)"

# PROFILE is overridable, but this gate is not profile-agnostic: it runs a k6
# Job from the loadgen image and asserts against analytics, reports and
# reports-ui. A narrower set fails late and confusingly -- an ErrImagePull on
# an image nobody built, or an assertion against a service nobody deployed --
# so it is rejected up front.
for required in analytics reports reports-ui load; do
  case " $profiles " in
    *" full "* | *" $required "*) ;;
    *)
      echo "PROFILE='$profiles' is missing '$required', which this gate needs" >&2
      echo "use PROFILE=full (the default), or add every one of: analytics reports reports-ui load" >&2
      exit 1
      ;;
  esac
done

# Tear down on ANY exit unless asked not to: a CI run that leaves a cluster
# behind wedges the next one, and a developer debugging locally wants the
# opposite. Failure paths dump state first (see below), so keeping the cluster
# is a convenience, not the only way to find out what broke.
cleanup() {
  local status=$?
  kill "${port_forward_pid:-}" 2>/dev/null || true
  if [ "$status" -ne 0 ]; then
    echo "::group::cluster state at failure"
    kubectl --context "kind-${cluster_name}" get pods -A 2>&1 || true
    kubectl --context "kind-${cluster_name}" -n "$namespace" describe pods 2>&1 | tail -100 || true
    echo "::endgroup::"
  fi
  if [ "$keep" = "1" ]; then
    echo "KEEP_CLUSTER=1 -- leaving '$cluster_name' up (make kind-down to remove)"
  else
    bash deploy/k8s/scripts/kind-down.sh || true
  fi
  exit "$status"
}
trap cleanup EXIT

echo "==> bringing up the cluster"
bash deploy/k8s/scripts/kind-up.sh

echo "==> deploying profiles '$profiles'"
PROFILE="$profiles" bash deploy/k8s/scripts/kind-deploy.sh

echo "==> seeding"
SEED_DAYS="${SEED_DAYS:-3}" bash deploy/k8s/scripts/kind-seed.sh

# k6 runs INSIDE the cluster as a Job, not from the runner: the scenario
# addresses services by their in-cluster names, and running it outside would
# test the Gateway rather than the platform. The image is already loaded by
# kind-deploy under the `load` profile.
echo "==> running the k6 smoke scenario as a Job"
kubectl --context "kind-${cluster_name}" -n "$namespace" delete job platform-smoke --ignore-not-found >/dev/null
kubectl --context "kind-${cluster_name}" -n "$namespace" apply -f - <<JOB
apiVersion: batch/v1
kind: Job
metadata:
  name: platform-smoke
spec:
  backoffLimit: 0
  template:
    metadata:
      labels:
        app.kubernetes.io/name: smoke
    spec:
      restartPolicy: Never
      containers:
        - name: k6
          image: devops-demo/loadgen:dev
          imagePullPolicy: IfNotPresent
          args: ["run", "/home/k6/scenarios/smoke.js"]
          envFrom:
            - configMapRef:
                name: platform-loadgen-config
          env:
            - name: SMOKE_DURATION_SECONDS
              value: "${SMOKE_DURATION_SECONDS:-60}"
JOB

if ! kubectl --context "kind-${cluster_name}" -n "$namespace" \
  wait --for=condition=complete job/platform-smoke --timeout=600s; then
  echo "::error::k6 smoke job did not complete"
  kubectl --context "kind-${cluster_name}" -n "$namespace" logs job/platform-smoke --tail=200 || true
  exit 1
fi
kubectl --context "kind-${cluster_name}" -n "$namespace" logs job/platform-smoke --tail=40

# Prometheus has no Gateway route -- it is cluster infrastructure, not a
# published surface -- so the assertions reach it through a port-forward that
# lives only as long as they do.
echo "==> port-forwarding Prometheus for the assertions"
kubectl --context "kind-${cluster_name}" -n monitoring \
  port-forward svc/kube-prometheus-stack-prometheus 9090:9090 >/dev/null 2>&1 &
port_forward_pid=$!
for _ in $(seq 1 30); do
  curl -sf --max-time 2 http://localhost:9090/-/healthy >/dev/null 2>&1 && break
  sleep 2
done

echo "==> asserting the wiring (scripts/e2e-smoke.sh against the Gateway)"
SMOKE_TARGET=k8s \
  K8S_NAMESPACE="$namespace" \
  BACKEND_URL="http://api.${domain}:${gateway_port}" \
  ANALYTICS_URL="http://analytics.${domain}:${gateway_port}" \
  REPORTS_URL="http://reports.${domain}:${gateway_port}" \
  REPORTS_UI_URL="http://reports-ui.${domain}:${gateway_port}" \
  PROM_URL="${PROM_URL:-http://localhost:9090}" \
  PROM_JOBS="${PROM_JOBS:-backend analytics}" \
  NIGHTLY="${NIGHTLY:-1}" \
  bash scripts/e2e-smoke.sh

kill "$port_forward_pid" 2>/dev/null || true

echo
echo "kind smoke passed."
