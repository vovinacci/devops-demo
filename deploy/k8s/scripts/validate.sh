#!/usr/bin/env bash
# Offline Helm chart gate (RFC-0003 DK2, DK9): helm lint every chart, then
# `helm template <umbrella> | kubeconform -strict` for every profile overlay.
# No cluster required -- this is a pure templating + schema-validation pass.
#
# kubeconform is given an explicit -schema-location for the Prometheus Operator
# ServiceMonitor CRD (vendored under deploy/k8s/schemas/), so the CR is
# ACTUALLY validated, never skipped. --ignore-missing-schemas is deliberately
# NOT used (it would silently pass exactly the CRs worth checking, the DK9
# trap).
#
# `make lint-k8s` and the k8s-lint CI workflow both run this file -- make == CI
# (RFC-0001 D12).
set -euo pipefail

# Ensure the mise-pinned helm + kubeconform are on PATH (self-bootstrap under
# mise once if they are not), so a bare `bash validate.sh` works locally too.
if ! command -v helm >/dev/null 2>&1 || ! command -v kubeconform >/dev/null 2>&1; then
  if command -v mise >/dev/null 2>&1; then
    exec mise exec -- "$0" "$@"
  fi
  echo "helm and kubeconform are required (pinned in .mise.toml; run 'mise install')" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$repo_root"

charts_dir="deploy/k8s/charts"
umbrella="$charts_dir/platform"
schema_dir="deploy/k8s/schemas"
# kubeconform schema-location template resolving the vendored CRD JSON schemas
# (lowercased kind), e.g. monitoring.coreos.com/servicemonitor_v1.json.
crd_schema="$schema_dir/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json"

# Per-service charts (consume the common library) + the library itself.
services=(backend frontend analytics reports reports-ui canary blackbox
  loadgen mailpit loki postgres-exporter postgres)

# Config files that exist twice on purpose: compose bind-mounts the original,
# Helm can only read files inside a chart, so the chart carries a copy that
# becomes a ConfigMap (RFC-0003 DK8). Physical duplication + an un-driftable
# gate is the same construction as scripts/check-toolchain-drift.sh.
echo "==> config file copies match the compose originals"
copy_drift=0
check_copy() {
  local original="$1" copy="$2"
  if cmp -s "$original" "$copy"; then
    echo "ok    $copy == $original"
  else
    echo "DRIFT $copy differs from $original" >&2
    diff -u "$original" "$copy" >&2 || true
    copy_drift=$((copy_drift + 1))
  fi
}
check_copy observability/loki/config.yml "$charts_dir/loki/files/config.yml"
check_copy observability/blackbox/blackbox.yml "$charts_dir/blackbox/files/blackbox.yml"
if [ "$copy_drift" -gt 0 ]; then
  echo "config file drift: ${copy_drift} copy/copies disagree with the compose original." >&2
  exit 1
fi

echo "==> helm dependency build (vendor the common library into each chart)"
for s in "${services[@]}"; do
  helm dependency build "$charts_dir/$s" >/dev/null
done
helm dependency build "$umbrella" >/dev/null

echo "==> helm lint"
helm lint "$charts_dir/common" "${services[@]/#/$charts_dir/}" "$umbrella"

# Profile overlays -- the k8s expression of the compose profiles (RFC-0001 D10).
# Additive profiles layer on top of the core base with a second -f.
run_profile() {
  local name="$1"
  shift
  echo "==> profile '$name': helm template | kubeconform -strict (ServiceMonitor CRD validated)"
  helm template platform "$umbrella" "$@" |
    kubeconform -strict -summary \
      -schema-location default \
      -schema-location "$crd_schema"
}

run_profile core -f "$umbrella/values.yaml"
run_profile analytics -f "$umbrella/values.yaml" -f "$umbrella/values-analytics.yaml"
run_profile reports -f "$umbrella/values.yaml" -f "$umbrella/values-reports.yaml"
run_profile reports-ui -f "$umbrella/values.yaml" -f "$umbrella/values-reports-ui.yaml"
run_profile synthetic -f "$umbrella/values.yaml" -f "$umbrella/values-synthetic.yaml"
run_profile load -f "$umbrella/values.yaml" -f "$umbrella/values-load.yaml"
run_profile full -f "$umbrella/values.yaml" -f "$umbrella/values-full.yaml"

echo "OK: all charts lint clean; every profile renders and passes kubeconform -strict."
