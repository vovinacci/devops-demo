// One-shot incident overlay (RFC-0001 D4, `make incident` / `make heal`).
// Run via `docker compose run --rm --name loadgen-incident loadgen run
// scenarios/incident.js` (see Makefile) -- deliberately a *separate*
// script from scenarios/main.js, not a mode switch inside it: the
// long-running loadgen service keeps ramping the normal shared-profile
// mix throughout, untouched; this is a second, temporary k6 process
// layered on top, which `make heal` kills by its fixed container name.
//
// Unlike main.js, this script has NO thresholds and does NOT mark any
// response as expected: the whole point is to make http_req_failed and
// latency genuinely spike so the SLO burn-rate alerts (RFC-0001 Section 7)
// have something real to fire on.
import http from "k6/http";
import { check, sleep } from "k6";

import { env, profile } from "../lib/env.js";
import { rate } from "../loadprofile/shape.js";

function num(name, fallback) {
  const raw = __ENV[name];
  if (raw === undefined || raw === "") return fallback;
  const n = Number(raw);
  if (!Number.isFinite(n)) throw new Error(`${name} must be a number, got "${raw}"`);
  return n;
}

// INCIDENT_MODE=spike (default): flat arrival rate at
// INCIDENT_SPIKE_MULTIPLIER-x the current shared-profile rate, hammering
// the cheapest real endpoint -- an offered-load spike, not error traffic.
// INCIDENT_MODE=errors: moderate, steady arrival rate of intentionally
// invalid requests -- an error-rate spike, not a volume spike.
const MODE = __ENV.INCIDENT_MODE === undefined || __ENV.INCIDENT_MODE === "" ? "spike" : __ENV.INCIDENT_MODE;
if (MODE !== "spike" && MODE !== "errors") {
  throw new Error(`INCIDENT_MODE must be "spike" or "errors", got "${MODE}"`);
}

function positive(name, fallback) {
  const n = num(name, fallback);
  if (n <= 0) throw new Error(`${name} must be > 0, got ${n}`);
  return n;
}

const MINUTES = positive("INCIDENT_MINUTES", 5);
const SPIKE_MULTIPLIER = positive("INCIDENT_SPIKE_MULTIPLIER", 10);

// Single evaluation at env.refUnixSeconds itself (offset 0) -- this
// script's own container start, same reference-time contract as
// scenarios/main.js (loadgen/README.md).
const currentRate = rate(env.refUnixSeconds, profile, {
  scale: env.scale,
  refUnixSeconds: env.refUnixSeconds,
});

// constant-arrival-rate's `rate` is an int64 field, like
// ramping-arrival-rate's `target` (loadgen/lib/schedule.js) -- rounded
// to a whole number for the same reason.
const spikeRate = Math.max(1, Math.round(currentRate * SPIKE_MULTIPLIER));
const errorRate = Math.max(1, Math.round(positive("INCIDENT_ERROR_RATE_PER_S", 5)));

export const options = {
  scenarios: {
    incident: {
      executor: "constant-arrival-rate",
      exec: MODE === "errors" ? "errors" : "spike",
      rate: MODE === "errors" ? errorRate : spikeRate,
      timeUnit: "1s",
      duration: `${MINUTES}m`,
      preAllocatedVUs: env.preAllocatedVUs,
      maxVUs: env.maxVUs,
    },
  },
  // No thresholds: this script's purpose is to fail loudly, not to gate
  // anything -- `make heal` is the only way it's meant to stop early.
};

export function spike() {
  const res = http.get(`${env.backendUrl}/items`, { tags: { name: "incident_spike" } });
  check(res, { "incident spike: items 200": (r) => r.status === 200 });
}

// Error storm: rotate through requests that are *unintended* mistakes as
// far as k6/Prometheus is concerned (no responseCallback) -- a
// duplicate-name conflict (409), malformed JSON (422, FastAPI's own
// Pydantic body validation -- see loadgen/scenarios/main.js's abuse()
// for why it's 422 and not the backend's hand-written 400), and a
// delete of a nonexistent id (404) all count against http_req_failed
// here, on purpose.
export function errors() {
  const which = __ITER % 3;

  if (which === 0) {
    // Same name every call -> the second and later calls hit the
    // backend's uniqueness constraint (409).
    http.post(`${env.backendUrl}/items`, JSON.stringify({ name: "loadgen-incident-duplicate" }), {
      headers: { "Content-Type": "application/json" },
      tags: { name: "incident_conflict" },
    });
  } else if (which === 1) {
    http.post(`${env.backendUrl}/items`, "{not valid json", {
      headers: { "Content-Type": "application/json" },
      tags: { name: "incident_invalid_json" },
    });
  } else {
    http.del(`${env.backendUrl}/items/999999999`, null, { tags: { name: "incident_delete_missing" } });
  }

  sleep(0.05);
}
