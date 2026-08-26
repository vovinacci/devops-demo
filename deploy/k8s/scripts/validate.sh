#!/usr/bin/env bash
# Offline Helm chart gate (RFC-0003 DK2, DK9): helm lint every chart, then
# `helm template <umbrella> | kubeconform -strict` for every profile overlay.
# No cluster required -- this is a pure templating + schema-validation pass.
#
# kubeconform is given an explicit -schema-location for every CRD the charts
# render -- the Prometheus Operator ServiceMonitor and the Gateway API
# Gateway/HTTPRoute/GRPCRoute -- vendored under deploy/k8s/schemas/, so those
# CRs are ACTUALLY validated, never skipped. --ignore-missing-schemas is
# deliberately NOT used (it would silently pass exactly the CRs worth
# checking, the DK9 trap).
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
compose_file="deploy/compose/docker-compose.yml"
# kubeconform schema-location template resolving the vendored CRD JSON schemas
# (lowercased kind), e.g. monitoring.coreos.com/servicemonitor_v1.json and
# gateway.networking.k8s.io/httproute_v1.json.
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
# The rule files and dashboards are rendered into PrometheusRule CRs and
# Grafana dashboard ConfigMaps, so they must stay identical to what compose
# feeds Prometheus and Grafana -- otherwise the two stacks silently alert and
# chart on different definitions.
check_copy observability/prometheus_slo_rules.yml "$umbrella/files/rules/prometheus_slo_rules.yml"
check_copy observability/prometheus_alerts.yml "$umbrella/files/rules/prometheus_alerts.yml"
for dashboard in observability/grafana/dashboards/*.json; do
  check_copy "$dashboard" "$umbrella/files/dashboards/$(basename "$dashboard")"
done
if [ "$copy_drift" -gt 0 ]; then
  echo "config file drift: ${copy_drift} copy/copies disagree with the compose original." >&2
  exit 1
fi

# Third-party images are pinned twice -- once in compose, once in the chart --
# and nothing imports one from the other. This is what catches a bump applied
# to only one of them (RFC-0003 DK2; the same gate construction as above).
echo "==> third-party image pins match the compose originals"
image_drift=0
# The image of ONE named compose service, not "anywhere in the file": a chart
# pointed at some other service's image would otherwise still match.
compose_image() {
  awk -v svc="  $1:" '
    $0 == svc { inblock = 1; next }
    inblock && /^  [^ ]/ { exit }
    inblock && $1 == "image:" { print $2; exit }
  ' "$compose_file"
}

check_image() {
  local chart="$1" service="$2"
  local values="$charts_dir/$chart/values.yaml"
  local repo tag ref want
  repo="$(sed -n 's/^  repository: \(.*\)$/\1/p' "$values" | head -n1)"
  tag="$(sed -n 's/^  tag: \(.*\)$/\1/p' "$values" | head -n1)"
  if [ -z "$repo" ] || [ -z "$tag" ]; then
    echo "DRIFT $chart: cannot read image repository/tag from $values" >&2
    image_drift=$((image_drift + 1))
    return
  fi
  ref="${repo}:${tag}"
  want="$(compose_image "$service")"
  if [ -z "$want" ]; then
    echo "DRIFT $chart: no image found for compose service '$service'" >&2
    image_drift=$((image_drift + 1))
  elif [ "$ref" = "$want" ]; then
    echo "ok    $chart: $ref"
  else
    echo "DRIFT $chart: $ref (compose service '$service' pins $want)" >&2
    image_drift=$((image_drift + 1))
  fi
}

# chart -> the compose service it mirrors. The three Postgres aliases share one
# chart and one image, so `db` stands for all of them.
check_image postgres db
check_image postgres-exporter postgres_exporter
check_image mailpit mailpit
check_image loki loki
check_image blackbox blackbox
if [ "$image_drift" -gt 0 ]; then
  echo "image pin drift: ${image_drift} chart(s) disagree with compose." >&2
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
  echo "==> profile '$name': helm template | kubeconform -strict (ServiceMonitor + Gateway API CRs validated)"
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
