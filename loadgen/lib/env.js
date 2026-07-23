// Loadgen environment contract (loadgen/README.md "Environment variables").
// Parsed once at k6 init time -- k6 caches module evaluation, so every
// scenario file importing `env` gets the same parsed object, not a
// re-parse per VU/iteration.
//
// profile.json is loaded here (not per-scenario) for the same reason:
// one parse, shared by every scenario and by scenarios/incident.js.

function num(name, fallback) {
  const raw = __ENV[name];
  if (raw === undefined || raw === "") return fallback;
  const n = Number(raw);
  if (!Number.isFinite(n)) {
    throw new Error(`${name} must be a number, got "${raw}"`);
  }
  return n;
}

// refUnixSeconds is the RFC-0001 D5 evaluation reference (loadprofile/README.md):
// shape.js requires it explicitly and refuses to default it. loadgen/entrypoint.sh
// sets LOADGEN_REF_UNIX = `date +%s` once per container start, before k6 runs --
// this module never calls Date.now() itself (see lib/schedule.js for why a
// moving reference would silently defeat DEMO_TIME_SCALE).
const refUnixSeconds = num("LOADGEN_REF_UNIX", undefined);
if (refUnixSeconds === undefined) {
  throw new Error(
    "LOADGEN_REF_UNIX is required (RFC-0001 D5 reference time) -- " +
      "normally set by loadgen/entrypoint.sh at container start; running " +
      "k6 directly needs LOADGEN_REF_UNIX=$(date +%s) exported first."
  );
}

// Reject values that would produce zero/negative stages or a k6
// options-validation error deep inside the executor instead of a clear
// message here at init.
function positive(name, fallback) {
  const n = num(name, fallback);
  if (n <= 0) throw new Error(`${name} must be > 0, got ${n}`);
  return n;
}

function positiveInt(name, fallback) {
  const n = positive(name, fallback);
  if (!Number.isInteger(n)) throw new Error(`${name} must be an integer, got ${n}`);
  return n;
}

const preAllocatedVUs = positiveInt("LOADGEN_PREALLOCATED_VUS", 10);
const maxVUs = positiveInt("LOADGEN_MAX_VUS", 50);
if (maxVUs < preAllocatedVUs) {
  throw new Error(`LOADGEN_MAX_VUS (${maxVUs}) must be >= LOADGEN_PREALLOCATED_VUS (${preAllocatedVUs})`);
}

export const env = {
  refUnixSeconds,
  scale: positive("DEMO_TIME_SCALE", 1),
  durationHours: positive("LOADGEN_DURATION_HOURS", 24),
  stageMinutes: positive("LOADGEN_STAGE_MINUTES", 15),
  preAllocatedVUs,
  maxVUs,
  backendUrl: __ENV.LOADGEN_BACKEND_URL || "http://api:8000",
  webUrl: __ENV.LOADGEN_WEB_URL || "http://web:80",
  grpcAddr: __ENV.LOADGEN_GRPC_ADDR || "api:50051",
  protoImportPath: __ENV.LOADGEN_PROTO_DIR || "/home/k6/proto",
  // RFC-0001 Phase 5 PR-2 (D5 enforcement): setup()'s scale guard
  // (lib/scaleguard.js) fetches this service's /api/v1/seed-marker.
  // `analytics` is a separate opt-in compose profile from `load` --
  // absent entirely is an expected, tolerated state (D10), not an error.
  analyticsUrl: __ENV.LOADGEN_ANALYTICS_URL || "http://analytics:8082",
  // RFC-0001 Phase 6 PR-3 (D12): the `reports` JVM profile is EXCLUDED from
  // the per-PR e2e gate and exercised nightly/full instead. Unlike
  // analyticsUrl this has NO default URL on purpose -- its presence IS the
  // enable-signal (reportsEnabled) for the heavy `report` scenario in
  // scenarios/main.js and scenarios/smoke.js; defaulting it to a URL would
  // schedule the report load unconditionally, including in per-PR smoke.
  reportsUrl: __ENV.LOADGEN_REPORTS_URL || undefined,
  reportsEnabled: Boolean(__ENV.LOADGEN_REPORTS_URL),
};

// loadprofile/profile.json is a sibling of lib/ under the image's
// /home/k6 root (loadgen/Dockerfile COPYs both there) -- open() resolves
// relative to this file, not the importing scenario, so this path is
// stable regardless of which scenario imports `env`.
export const profile = JSON.parse(open("../loadprofile/profile.json"));
