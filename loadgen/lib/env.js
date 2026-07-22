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

export const env = {
  refUnixSeconds,
  scale: num("DEMO_TIME_SCALE", 1),
  durationHours: num("LOADGEN_DURATION_HOURS", 24),
  stageMinutes: num("LOADGEN_STAGE_MINUTES", 15),
  preAllocatedVUs: num("LOADGEN_PREALLOCATED_VUS", 10),
  maxVUs: num("LOADGEN_MAX_VUS", 50),
  backendUrl: __ENV.LOADGEN_BACKEND_URL || "http://api:8000",
  webUrl: __ENV.LOADGEN_WEB_URL || "http://web:80",
  grpcAddr: __ENV.LOADGEN_GRPC_ADDR || "api:50051",
  protoImportPath: __ENV.LOADGEN_PROTO_DIR || "/home/k6/proto",
};

// loadprofile/profile.json is a sibling of lib/ under the image's
// /home/k6 root (loadgen/Dockerfile COPYs both there) -- open() resolves
// relative to this file, not the importing scenario, so this path is
// stable regardless of which scenario imports `env`.
export const profile = JSON.parse(open("../loadprofile/profile.json"));
