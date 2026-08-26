#!/usr/bin/env bash
# One-shot k6 incident overlay on the deployed stack (RFC-0003 DK10), the Kind
# counterpart of `make incident`.
#
# Compose runs this as `compose run --rm --name loadgen-incident`; here it is a
# Job, which is the same idea with a name Kubernetes already understands --
# run to completion, keep the record, delete it to stop early (`make
# kind-heal`). The long-running loadgen Deployment is untouched either way:
# the overlay is additional traffic beside it, not a replacement.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$repo_root"

namespace="${NAMESPACE:-devops-demo}"
cluster_name="$(sed -n 's/^name: \(.*\)$/\1/p' deploy/k8s/kind/cluster.yaml)"
kc=(kubectl --context "kind-${cluster_name}" --namespace "$namespace")

mode="${INCIDENT_MODE:-spike}"
minutes="${INCIDENT_MINUTES:-5}"

case "$mode" in
  spike | errors) ;;
  *)
    echo "unknown INCIDENT_MODE '$mode' (want: spike|errors)" >&2
    exit 1
    ;;
esac

if ! "${kc[@]}" get deployment platform-loadgen >/dev/null 2>&1; then
  echo "loadgen is not deployed -- run 'make kind-deploy PROFILE=load' (or full) first" >&2
  exit 1
fi

echo "==> starting the '$mode' incident for ${minutes}m"
"${kc[@]}" delete job platform-incident --ignore-not-found >/dev/null
"${kc[@]}" apply -f - <<JOB
apiVersion: batch/v1
kind: Job
metadata:
  name: platform-incident
  labels:
    app.kubernetes.io/part-of: devops-demo
spec:
  backoffLimit: 0
  template:
    metadata:
      labels:
        app.kubernetes.io/name: incident
    spec:
      restartPolicy: Never
      containers:
        - name: k6
          image: devops-demo/loadgen:dev
          imagePullPolicy: IfNotPresent
          args: ["run", "-o", "experimental-prometheus-rw", "/home/k6/scenarios/incident.js"]
          envFrom:
            - configMapRef:
                name: platform-loadgen-config
          env:
            - name: INCIDENT_MODE
              value: "${mode}"
            - name: INCIDENT_MINUTES
              value: "${minutes}"
            - name: INCIDENT_SPIKE_MULTIPLIER
              value: "${INCIDENT_SPIKE_MULTIPLIER:-10}"
            - name: INCIDENT_ERROR_RATE_PER_S
              value: "${INCIDENT_ERROR_RATE_PER_S:-5}"
            - name: K6_PROMETHEUS_RW_TREND_STATS
              value: "p(95),p(99),avg"
JOB

echo
echo "incident running. Watch it:"
echo "  kubectl -n ${namespace} logs -f job/platform-incident"
echo "  http://grafana.devops-demo.localhost:8080/"
echo "Stop early: make kind-heal"
