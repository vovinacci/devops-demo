#!/usr/bin/env bash
# Post-smoke assertions for the e2e/nightly CI stages (RFC-0001 D12, PR-4).
# Called by `make smoke` (core + analytics + load) and `make smoke-full`
# (adds the synthetic profile -- set NIGHTLY=1 to also run the
# canary/blackbox checks that profile enables). Cheap, high-value checks
# on top of the k6 threshold gate: the stack is not just "up", the pieces
# are actually talking to each other.
#
# On any failure this prints `compose ps` plus the failing service's log
# tail before exiting non-zero -- CI's own logs-on-failure step (dumping
# every service) is a second, coarser net for anything this misses.
set -euo pipefail

COMPOSE_FILE="deploy/compose/docker-compose.yml"

# Every address is a variable defaulting to the compose stack's published
# port, so `make smoke` behaves exactly as before and the SAME assertions can
# run against Kubernetes, where the services sit behind the Gateway on one
# port and are addressed by hostname. Two stacks, one definition of "working".
SMOKE_TARGET="${SMOKE_TARGET:-compose}"
PROM_URL="${PROM_URL:-http://localhost:9090}"
BACKEND_URL="${BACKEND_URL:-http://localhost:8000}"
ANALYTICS_URL="${ANALYTICS_URL:-http://localhost:8082}"
REPORTS_URL="${REPORTS_URL:-http://localhost:8083}"
REPORTS_UI_URL="${REPORTS_UI_URL:-http://localhost:8084}"
K8S_NAMESPACE="${K8S_NAMESPACE:-devops-demo}"

compose() {
  docker compose -f "$COMPOSE_FILE" --project-directory . "$@"
}

# fail() is called with COMPOSE service names; Kubernetes names two of them
# differently (api -> backend, web -> frontend), so the diagnostic dump
# translates rather than making every call site aware of the target.
k8s_workload() {
  case "$1" in
    api) echo backend ;;
    web) echo frontend ;;
    *) echo "$1" ;;
  esac
}

diagnose() {
  local service="$1"
  case "$SMOKE_TARGET" in
    k8s)
      echo "--- pods ---" >&2
      kubectl -n "$K8S_NAMESPACE" get pods >&2 || true
      echo "--- ${service} logs (tail 200) ---" >&2
      kubectl -n "$K8S_NAMESPACE" logs -l "app.kubernetes.io/name=$(k8s_workload "$service")" \
        --tail=200 >&2 || true
      ;;
    *)
      echo "--- compose ps ---" >&2
      compose ps >&2 || true
      echo "--- ${service} logs (tail 200) ---" >&2
      compose logs --tail=200 "$service" >&2 || true
      ;;
  esac
}

fail() {
  local service="$1" reason="$2"
  echo "::error::e2e-smoke: ${reason}" >&2
  diagnose "$service"
  exit 1
}

check_http_200() {
  local label="$1" url="$2" service="$3" code
  # curl itself writes "000" via -w on a connection failure, but only
  # after a completed attempt to write the format string -- an `||
  # echo "000"` fallback on top of that would double up to "000000" on
  # a connect refusal (curl also exits non-zero), so this only supplies
  # the fallback when curl produced no output at all.
  # `|| true`: under set -euo pipefail a connect-refused curl (non-zero
  # exit) inside the substitution would kill the script here, before
  # fail() gets to print its diagnostics.
  code=$(curl -sS -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)
  code="${code:-000}"
  if [ "$code" != "200" ]; then
    fail "$service" "$label returned $code (expected 200), url=$url"
  fi
  echo "OK: $label -> 200"
}

echo "== backend /healthz =="
check_http_200 "backend /healthz" "${BACKEND_URL}/healthz" api

echo "== analytics /readyz =="
check_http_200 "analytics /readyz" "${ANALYTICS_URL}/readyz" analytics

echo "== analytics stream_connected =="
stream_connected=$(curl -sS "${ANALYTICS_URL}/metrics" 2>/dev/null | awk '/^analytics_stream_connected /{print $2}' || true)
if [ "${stream_connected:-}" != "1" ]; then
  fail analytics "analytics_stream_connected == '${stream_connected:-<absent>}', expected 1"
fi
echo "OK: analytics_stream_connected == 1"

# Job names differ by stack: compose names its scrape jobs in prometheus.yml
# (`api`), while Kubernetes derives them from each Service's own name via the
# ServiceMonitor's jobLabel (`backend`). Same two services either way.
PROM_JOBS="${PROM_JOBS:-api analytics}"
echo "== prometheus targets up: ${PROM_JOBS} =="
for job in $PROM_JOBS; do
  health=$(curl -sS "${PROM_URL}/api/v1/targets" 2>/dev/null | jq -r --arg job "$job" \
    '.data.activeTargets[] | select(.labels.job==$job) | .health' | head -n1 || true)
  if [ "${health:-}" != "up" ]; then
    fail prometheus "target job=$job health='${health:-<absent>}', expected up"
  fi
  echo "OK: prometheus target job=$job up"
done

if [ "${NIGHTLY:-0}" = "1" ]; then
  echo "== canary journey success (nightly only) =="
  canary_success=$(curl -sS "${PROM_URL}/api/v1/query" \
    --data-urlencode 'query=sum(canary_journey_total{result="success"})' 2>/dev/null |
    jq -r '.data.result[0].value[1] // "0"' || true)
  if ! awk -v v="$canary_success" 'BEGIN { exit !(v > 0) }'; then
    fail canary "sum(canary_journey_total{result=\"success\"}) == $canary_success, expected > 0"
  fi
  echo "OK: canary journey success total = $canary_success"

  echo "== blackbox probe_success all 1 (nightly only) =="
  down_count=$(curl -sS "${PROM_URL}/api/v1/query" --data-urlencode 'query=probe_success == 0' 2>/dev/null |
    jq -r '.data.result | length' || true)
  if [ "${down_count:-1}" != "0" ]; then
    fail blackbox "probe_success == 0 for $down_count target(s), expected none"
  fi
  echo "OK: all blackbox probes succeeding"

  # reports (the JVM profile, RFC-0001 D2/D12) is up only in the nightly/full
  # path; the k6 `report` scenario already drives it, this proves the async
  # job flow end to end (POST -> poll -> download) as a cheap, targeted check.
  echo "== reports /readyz (nightly only) =="
  check_http_200 "reports /readyz" "${REPORTS_URL}/readyz" reports

  echo "== reports job POST -> poll -> download (nightly only) =="
  # csv is the lightest format (no POI/PDF library), so this assertion stays
  # fast and does not compete with the k6 report scenario's heavier renders.
  submit_headers=$(curl -sS -D - -o /dev/null \
    -X POST "${REPORTS_URL}/reports" \
    -H 'Content-Type: application/json' \
    -d '{"type":"items-summary","format":"csv"}' 2>/dev/null || true)
  # Spring emits a capital-L `Location: /reports/{id}` over HTTP/1.1 -- match
  # the header name case-insensitively and strip the trailing CR.
  location=$(printf '%s' "$submit_headers" | awk 'tolower($1) == "location:" { print $2 }' | tr -d '\r')
  if [ -z "${location:-}" ]; then
    fail reports "POST /reports returned no Location header (submit failed)"
  fi
  echo "OK: reports job accepted -> ${location}"

  status=""
  for _ in $(seq 1 30); do
    sleep 1
    status=$(curl -sS "${REPORTS_URL}${location}" 2>/dev/null | jq -r '.status // ""' || true)
    if [ "$status" = "SUCCEEDED" ] || [ "$status" = "FAILED" ]; then
      break
    fi
  done
  if [ "$status" != "SUCCEEDED" ]; then
    fail reports "report job ${location} status='${status:-<none>}', expected SUCCEEDED"
  fi
  echo "OK: report job ${location} SUCCEEDED"

  check_http_200 "reports download" "${REPORTS_URL}${location}/download" reports

  # reports-ui (RFC-0002, the Caddy static frontend + reverse proxy) rides the
  # same nightly/full stack as reports; like reports it stays OUT of the per-PR
  # gate. Prove its D6 endpoints answer and that it exposes Caddy's native
  # Prometheus metrics on the site listener (the `metrics` handler re-exposing
  # the admin-API metrics -- the ADR-0013 win over the React frontend's nginx).
  echo "== reports-ui /healthz (nightly only) =="
  check_http_200 "reports-ui /healthz" "${REPORTS_UI_URL}/healthz" reports-ui

  echo "== reports-ui /metrics exposes caddy_ series (nightly only) =="
  # Status first, body second. Grepping the body alone cannot tell "Caddy
  # answered without the series" from "something else answered instead" -- and
  # through a Gateway the something else is an error page, which contains no
  # caddy_ lines either. The failure then names the wrong cause.
  metrics_body="$(mktemp)"
  metrics_err="$(mktemp)"
  # The transfer's own exit status matters separately from the HTTP status: a
  # truncated response is a 200 that stopped early, and if the bytes that did
  # arrive happen to contain a caddy_ line, discarding curl's status would let
  # it pass. Keep curl's stderr too -- it is the only place the reason appears.
  if metrics_code=$(curl -sS -o "$metrics_body" -w '%{http_code}' "${REPORTS_UI_URL}/metrics" 2>"$metrics_err"); then
    curl_status=0
  else
    curl_status=$?
  fi
  if [ "$curl_status" -ne 0 ]; then
    echo "--- curl stderr ---" >&2
    cat "$metrics_err" >&2 || true
    rm -f "$metrics_body" "$metrics_err"
    fail reports-ui "GET ${REPORTS_UI_URL}/metrics: curl exited $curl_status (transfer failed or incomplete)"
  fi
  rm -f "$metrics_err"
  metrics_code="${metrics_code:-000}"
  if [ "$metrics_code" != "200" ]; then
    echo "--- first 10 lines of the response ---" >&2
    head -10 "$metrics_body" >&2 || true
    rm -f "$metrics_body"
    fail reports-ui "GET ${REPORTS_UI_URL}/metrics returned $metrics_code (expected 200)"
  fi
  if ! grep -q '^caddy_' "$metrics_body"; then
    echo "--- first 10 lines of the 200 response ---" >&2
    head -10 "$metrics_body" >&2 || true
    rm -f "$metrics_body"
    fail reports-ui "GET ${REPORTS_UI_URL}/metrics returned 200 with no caddy_ series (metrics handler not exposing Caddy metrics)"
  fi
  rm -f "$metrics_body"
  echo "OK: reports-ui /metrics exposes caddy_ series"

  # The one path that crosses a reverse proxy configured INSIDE an image
  # rather than by a manifest: reports-ui's Caddyfile proxies /api to the
  # reports service. A wrong upstream there cannot be caught by rendering
  # manifests or by probing either service directly -- both are healthy while
  # every /api call fails. It shipped exactly that way to Kubernetes once.
  echo "== reports-ui /api reaches reports (nightly only) =="
  check_http_200 "reports-ui /api/reports" "${REPORTS_UI_URL}/api/reports" reports-ui

  echo "== reports-ui upstream is healthy, not just reachable (nightly only) =="
  upstreams_healthy=$(curl -sS "${REPORTS_UI_URL}/metrics" 2>/dev/null |
    awk '/^caddy_reverse_proxy_upstreams_healthy/{print $2; exit}' || true)
  if [ "${upstreams_healthy:-0}" != "1" ]; then
    fail reports-ui "caddy_reverse_proxy_upstreams_healthy == '${upstreams_healthy:-<absent>}', expected 1 (the /api upstream is down or misaddressed)"
  fi
  echo "OK: caddy_reverse_proxy_upstreams_healthy == 1"
  echo "OK: reports-ui /metrics exposes caddy_ series"
fi

echo "All e2e smoke assertions passed."
