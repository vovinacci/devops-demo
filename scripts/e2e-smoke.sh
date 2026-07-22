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
PROM_URL="${PROM_URL:-http://localhost:9090}"

compose() {
  docker compose -f "$COMPOSE_FILE" --project-directory . "$@"
}

fail() {
  local service="$1" reason="$2"
  echo "::error::e2e-smoke: ${reason}" >&2
  echo "--- compose ps ---" >&2
  compose ps >&2 || true
  echo "--- ${service} logs (tail 200) ---" >&2
  compose logs --tail=200 "$service" >&2 || true
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
check_http_200 "backend /healthz" "http://localhost:8000/healthz" api

echo "== analytics /readyz =="
check_http_200 "analytics /readyz" "http://localhost:8082/readyz" analytics

echo "== analytics stream_connected =="
stream_connected=$(curl -sS http://localhost:8082/metrics 2>/dev/null | awk '/^analytics_stream_connected /{print $2}' || true)
if [ "${stream_connected:-}" != "1" ]; then
  fail analytics "analytics_stream_connected == '${stream_connected:-<absent>}', expected 1"
fi
echo "OK: analytics_stream_connected == 1"

echo "== prometheus targets api + analytics up =="
for job in api analytics; do
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
fi

echo "All e2e smoke assertions passed."
