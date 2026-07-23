// Shared k6 threshold contract (RFC-0001 D4 CI-gate contract, ADR-0006).
// scenarios/main.js (the long-running compose service) and
// scenarios/smoke.js (the CI/nightly one-shot e2e gate, PR-4) enforce the
// same thresholds by importing this one object instead of each carrying
// its own copy -- a duplicated literal would silently drift the moment
// either script's thresholds changed without the other. Per-threshold
// rationale (why 300ms/800ms/1%, why expensive is looser, why abuse has
// no http_req_failed entry) lives as comments in scenarios/main.js next
// to the scenario that produces each metric; this module only holds the
// values both scripts must agree on.
//
// A threshold whose metric receives zero samples in a given run (e.g.
// smoke.js does not exercise "expensive") is simply not evaluated by k6,
// not a failure -- see https://github.com/grafana/k6/issues/1346 -- so
// reusing the whole object unmodified in a script that runs a subset of
// scenarios is safe.
export const THRESHOLDS = {
  "http_req_duration{scenario:browse}": ["p(95)<300"],
  "http_req_duration{scenario:crud}": ["p(95)<300"],
  "http_req_duration{scenario:expensive}": ["p(95)<800"],
  "http_req_failed{scenario:browse}": ["rate<0.01"],
  "http_req_failed{scenario:crud}": ["rate<0.01"],
  "http_req_failed{scenario:expensive}": ["rate<0.01"],
  "grpc_req_duration{scenario:grpc}": ["p(95)<300"],
  loadgen_grpc_call_failed: ["rate<0.01"],
  loadgen_abuse_unexpected_status: ["rate<0.01"],
  // report scenario (RFC-0001 Phase 6 PR-3, D12), scenarios/main.js report().
  // 5%, not the 1% of the pure-correctness gates above: a report job SHOULD
  // succeed, but it is an async POI/PDF render on a bounded-concurrency JVM
  // under nightly load, with best-effort analytics enrichment (D10) and a
  // bounded client-side poll -- eventual-consistency slack that warrants a
  // little more tolerance than the synchronous 1% signals. Safe to keep here
  // unconditionally even though per-PR smoke never schedules `report`: a
  // threshold whose metric gets zero samples is simply not evaluated by k6
  // (see the module note above), so the unset-URL per-PR run is unaffected.
  loadgen_report_job_failed: ["rate<0.05"],
};
