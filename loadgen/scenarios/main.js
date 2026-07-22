// Long-running loadgen (RFC-0001 D4, ADR-0006): the compose `loadgen`
// service's default CMD. Five ramping-arrival-rate scenarios, all
// derived from the one shared load profile (loadprofile/, RFC-0001 D5),
// so the mix breathes with the same diurnal/weekly shape as everything
// else in the demo. Scenario weights sum to 1.0 (report-trigger,
// RFC-0001 D4's fifth original slice, is deferred to Phase 6 per the
// team-plan handoff -- reports doesn't exist yet).
import http from "k6/http";
import grpc from "k6/net/grpc";
import { check, sleep } from "k6";
import { Rate } from "k6/metrics";

import { env, profile } from "../lib/env.js";
import { buildStages } from "../lib/schedule.js";

// Custom correctness signals, separate from k6's built-in http_req_failed
// (RFC-0001 D4 CI-gate contract, see Thresholds below):
// - abuse requests are marked EXPECTED via http.expectedStatuses() below,
//   so http_req_failed never counts them; this Rate instead tracks
//   "did the abuse request return the status we designed it to return".
// - grpc has no http_req_failed equivalent at all (that metric is
//   HTTP-only), so this Rate is the gRPC scenario's own failure signal.
const abuseUnexpectedStatus = new Rate("loadgen_abuse_unexpected_status");
const grpcCallFailed = new Rate("loadgen_grpc_call_failed");

const totalSeconds = env.durationHours * 3600;
const stepSeconds = env.stageMinutes * 60;

// Scenario weights apportion the shared profile's arrival rate across
// scenario ITERATIONS (RFC-0001 D4 spirit: browse 60%, CRUD 20%, abuse
// 10%, gRPC 5%). Iterations are not single requests -- browse and crud
// issue 2 HTTP calls each, grpc 2 invokes, expensive a batch -- so the
// request-level mix deliberately differs from the iteration split; the
// iteration mix is the contract (documented in loadgen/README.md).
// "expensive" is a distinct, low base weight (5%) modulated by its own
// on/off burst pattern below, not a slice of the other four -- see
// WEIGHTS.expensiveBase.
const WEIGHTS = {
  browse: 0.6,
  crud: 0.2,
  abuse: 0.1,
  grpc: 0.05,
  expensiveBase: 0.05,
};

// "expensive" burst cadence, expressed in stage counts (not minutes) so
// it stays well-defined regardless of LOADGEN_STAGE_MINUTES: one stage
// in every EXPENSIVE_BURST_CYCLE is a burst (BURST_MULTIPLIER-x the base
// rate), the rest sit at a low idle fraction. Honest limitation
// (loadgen/README.md): the backend has no artificially slow/unindexed
// query, so "expensive" means a burst of concurrent ordinary GET /items
// calls per iteration (see expensive() below), not a fabricated one.
const EXPENSIVE_BURST_CYCLE = 4;
const EXPENSIVE_BURST_MULTIPLIER = 8;
const EXPENSIVE_IDLE_FRACTION = 0.2;
const EXPENSIVE_BURST_FANOUT = 5;

function expensiveModulate(target, stepIndex) {
  const inBurst = stepIndex % EXPENSIVE_BURST_CYCLE === 0;
  return inBurst ? target * EXPENSIVE_BURST_MULTIPLIER : target * EXPENSIVE_IDLE_FRACTION;
}

const stagesBrowse = buildStages({
  profile,
  refUnixSeconds: env.refUnixSeconds,
  scale: env.scale,
  totalSeconds,
  stepSeconds,
  weight: WEIGHTS.browse,
  minTarget: 1,
});
const stagesCrud = buildStages({
  profile,
  refUnixSeconds: env.refUnixSeconds,
  scale: env.scale,
  totalSeconds,
  stepSeconds,
  weight: WEIGHTS.crud,
  minTarget: 1,
});
const stagesAbuse = buildStages({
  profile,
  refUnixSeconds: env.refUnixSeconds,
  scale: env.scale,
  totalSeconds,
  stepSeconds,
  weight: WEIGHTS.abuse,
  minTarget: 1,
});
const stagesGrpc = buildStages({
  profile,
  refUnixSeconds: env.refUnixSeconds,
  scale: env.scale,
  totalSeconds,
  stepSeconds,
  weight: WEIGHTS.grpc,
  minTarget: 1,
});
const stagesExpensive = buildStages({
  profile,
  refUnixSeconds: env.refUnixSeconds,
  scale: env.scale,
  totalSeconds,
  stepSeconds,
  weight: WEIGHTS.expensiveBase,
  minTarget: 1,
  modulate: expensiveModulate,
});

function scenarioConfig(exec, stages) {
  return {
    executor: "ramping-arrival-rate",
    exec,
    startRate: stages[0].target,
    timeUnit: "1s",
    preAllocatedVUs: env.preAllocatedVUs,
    maxVUs: env.maxVUs,
    stages,
  };
}

export const options = {
  scenarios: {
    browse: scenarioConfig("browse", stagesBrowse),
    crud: scenarioConfig("crud", stagesCrud),
    abuse: scenarioConfig("abuse", stagesAbuse),
    grpc: scenarioConfig("grpcScenario", stagesGrpc),
    expensive: scenarioConfig("expensive", stagesExpensive),
  },
  // RFC-0001 D4 CI-gate contract (reused as-is by the future e2e workflow,
  // PR-4): per-scenario submetrics via k6's automatic `scenario` tag, not
  // one blanket threshold -- abuse's 4xx responses are marked EXPECTED via
  // http.expectedStatuses() in abuse() below, so they never enter
  // http_req_failed in the first place; there is deliberately no
  // http_req_failed{scenario:abuse} threshold here, its correctness
  // signal is loadgen_abuse_unexpected_status instead. "expensive" gets a
  // looser latency budget: it exists specifically to produce a real p99
  // tail, so gating it at the same 300ms as everything else would be
  // gating the exhibit itself.
  thresholds: {
    "http_req_duration{scenario:browse}": ["p(95)<300"],
    "http_req_duration{scenario:crud}": ["p(95)<300"],
    "http_req_duration{scenario:expensive}": ["p(95)<800"],
    "http_req_failed{scenario:browse}": ["rate<0.01"],
    "http_req_failed{scenario:crud}": ["rate<0.01"],
    "http_req_failed{scenario:expensive}": ["rate<0.01"],
    "grpc_req_duration{scenario:grpc}": ["p(95)<300"],
    loadgen_grpc_call_failed: ["rate<0.01"],
    loadgen_abuse_unexpected_status: ["rate<0.01"],
  },
};

// --- browse (~60%): read-only traffic against backend + frontend -----

export function browse() {
  const itemsRes = http.get(`${env.backendUrl}/items`, { tags: { name: "items_list" } });
  check(itemsRes, { "browse: items 200": (r) => r.status === 200 });

  sleep(Math.random() * 0.5);

  const webRes = http.get(`${env.webUrl}/`, { tags: { name: "web_root" } });
  check(webRes, { "browse: web 200": (r) => r.status === 200 });
}

// --- crud (~20%): create -> delete lifecycle, loadgen- prefixed -------
//
// The prefix (distinct from the canary's `canary-` prefix, RFC-0001 D9)
// keeps synthetic writes identifiable and filterable; the same-iteration
// delete keeps the item count from growing unbounded across a long run
// (verified live in this PR's e2e evidence).

export function crud() {
  const name = `loadgen-${env.refUnixSeconds}-${__VU}-${__ITER}-${Math.floor(Math.random() * 1e6)}`;
  const createRes = http.post(`${env.backendUrl}/items`, JSON.stringify({ name }), {
    headers: { "Content-Type": "application/json" },
    tags: { name: "items_create" },
  });
  const created = check(createRes, { "crud: create 201": (r) => r.status === 201 });
  if (!created) return;

  sleep(0.1);

  const id = createRes.json("id");
  const delRes = http.del(`${env.backendUrl}/items/${id}`, null, {
    tags: { name: "items_delete" },
  });
  check(delRes, { "crud: delete 204": (r) => r.status === 204 });
}

// --- abuse (~10%): three intentionally-invalid request shapes ---------
//
// Every response here is EXPECTED to be a 4xx: http.expectedStatuses()
// tells k6 not to count it in http_req_failed (that metric answers "did
// something go wrong", and nothing did -- the backend correctly rejected
// bad input). loadgen_abuse_unexpected_status is the real correctness
// check: it fires only when the backend's actual status diverges from
// what this scenario is designed to provoke.

const ABUSE_EXPECTED = http.expectedStatuses(400, 404, 422);

export function abuse() {
  const which = __ITER % 3;

  if (which === 0) {
    // Malformed JSON body to POST /items -- FastAPI binds the request
    // body straight into the `ItemCreate` Pydantic model before the
    // handler ever runs, so a parse failure here is FastAPI's own
    // automatic request-validation error (422), not the backend's
    // hand-written 400 (that one only exists on /metrics/frontend
    // below, which parses the body itself). Verified live: without this
    // case k6 reported every one of these as an unexpected status.
    const res = http.post(`${env.backendUrl}/items`, "{not valid json", {
      headers: { "Content-Type": "application/json" },
      tags: { name: "abuse_invalid_json" },
      responseCallback: ABUSE_EXPECTED,
    });
    abuseUnexpectedStatus.add(!check(res, { "abuse: invalid json -> 422": (r) => r.status === 422 }));
  } else if (which === 1) {
    // Unknown web-vital name -- rejected by ALLOWED_WEB_VITALS
    // (services/backend/app/main.py).
    const res = http.post(
      `${env.backendUrl}/metrics/frontend`,
      JSON.stringify({ name: "NOT_A_REAL_VITAL", value: 1 }),
      {
        headers: { "Content-Type": "application/json" },
        tags: { name: "abuse_unknown_vital" },
        responseCallback: ABUSE_EXPECTED,
      }
    );
    abuseUnexpectedStatus.add(!check(res, { "abuse: unknown vital -> 400": (r) => r.status === 400 }));
  } else {
    // Delete an id that (almost certainly) never existed.
    const res = http.del(`${env.backendUrl}/items/999999999`, null, {
      tags: { name: "abuse_delete_missing" },
      responseCallback: ABUSE_EXPECTED,
    });
    abuseUnexpectedStatus.add(!check(res, { "abuse: delete missing -> 404": (r) => r.status === 404 }));
  }
}

// --- grpc (~5%): unary ItemService calls, proto loaded at runtime -----

const grpcClient = new grpc.Client();
grpcClient.load([env.protoImportPath], "devopsdemo/items/v1/items.proto");

export function grpcScenario() {
  // connect() throws (it never returns a status) -- without the catch a
  // gRPC-endpoint outage would surface as dropped iterations instead of
  // the loadgen_grpc_call_failed rate this scenario advertises as its
  // failure signal.
  try {
    grpcClient.connect(env.grpcAddr, { plaintext: true });
  } catch (err) {
    grpcCallFailed.add(true);
    return;
  }

  const listRes = grpcClient.invoke("devopsdemo.items.v1.ItemService/ListItems", {});
  grpcCallFailed.add(
    !check(listRes, { "grpc: ListItems OK": (r) => r && r.status === grpc.StatusOK })
  );

  const statsRes = grpcClient.invoke("devopsdemo.items.v1.ItemService/GetItemStats", {});
  grpcCallFailed.add(
    !check(statsRes, { "grpc: GetItemStats OK": (r) => r && r.status === grpc.StatusOK })
  );

  grpcClient.close();
}

// --- expensive (occasional bursts): concurrent GET /items fan-out -----
//
// Honest limitation (loadgen/README.md): the backend has no unindexed
// search or oversized-page endpoint to hammer, so this scenario
// approximates RFC-0001 D4's "expensive query" tail by firing several
// concurrent ordinary requests per iteration -- real queueing/connection
// pressure, not a fabricated slow query.

export function expensive() {
  // http.batch fires all requests concurrently from this one VU/iteration
  // -- a sequential loop of http.get would just be more browse traffic
  // with extra steps, not the concurrency pressure this scenario exists
  // to create.
  const requests = Array.from({ length: EXPENSIVE_BURST_FANOUT }, () => [
    "GET",
    `${env.backendUrl}/items`,
    null,
    { tags: { name: "items_list_burst" } },
  ]);
  const responses = http.batch(requests);
  for (const r of responses) {
    check(r, { "expensive: items 200": (res) => res.status === 200 });
  }
}
