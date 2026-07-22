// CI/nightly e2e smoke gate (RFC-0001 D4/D12, PR-4). One-shot, short
// counterpart to the long-running scenarios/main.js: same exec functions,
// same threshold contract (loadgen/lib/thresholds.js), a small fraction
// of the run time -- D4's "same scripts double as CI gates" means this
// script must genuinely reuse main.js's scenario bodies rather than
// approximate them, so a threshold regression in main.js is caught here
// too.
//
// `constant-arrival-rate`, not the shared diurnal profile: a 60-90s
// window is too short for loadprofile/shape.js's diurnal shape to mean
// anything, and this script only needs a steady, modest, deterministic
// rate long enough for k6's percentile thresholds to have enough samples
// to be meaningful. Deliberately excludes `expensive`: its burst
// modulation (EXPENSIVE_BURST_CYCLE in main.js) is stage-index-driven and
// has no analogue in a single flat stage -- its threshold key stays
// imported from THRESHOLDS anyway, and a threshold with zero matching
// samples is simply not evaluated by k6, not a failure (see
// loadgen/lib/thresholds.js).
import { env } from "../lib/env.js";
import { THRESHOLDS } from "../lib/thresholds.js";
// Re-exported, not called from here: k6 resolves a scenario's `exec` name
// against this script's own export table, so main.js's scenario bodies
// must be re-exported by name to be reusable as exec targets in options
// below (main.js's own module-level code, e.g. its stage precomputation,
// still runs once on import -- harmless, just unused output here).
export { browse, crud, abuse, grpcScenario } from "./main.js";

function positiveInt(name, fallback) {
  const raw = __ENV[name];
  if (raw === undefined || raw === "") return fallback;
  const n = Number(raw);
  if (!Number.isFinite(n) || n <= 0 || !Number.isInteger(n)) {
    throw new Error(`${name} must be a positive integer, got "${raw}"`);
  }
  return n;
}

// 75s default sits in the middle of the ~60-90s window this script is
// designed for; nightly.yml overrides it to a longer run (SMOKE_DURATION_SECONDS=300)
// via the same env var, no separate script needed for the "full" stage.
const DURATION_SECONDS = positiveInt("SMOKE_DURATION_SECONDS", 75);
const DURATION = `${DURATION_SECONDS}s`;

// Fixed, modest per-scenario rates -- not derived from loadprofile/shape.js
// (see module comment above). Weighted roughly like main.js's iteration
// split (browse > crud > abuse > grpc) but the absolute numbers matter
// less here than "enough iterations in DURATION_SECONDS for the p(95)
// thresholds to be meaningful, without hammering a CI runner".
export const options = {
  scenarios: {
    browse: {
      executor: "constant-arrival-rate",
      exec: "browse",
      rate: 10,
      timeUnit: "1s",
      duration: DURATION,
      preAllocatedVUs: env.preAllocatedVUs,
      maxVUs: env.maxVUs,
    },
    crud: {
      executor: "constant-arrival-rate",
      exec: "crud",
      rate: 4,
      timeUnit: "1s",
      duration: DURATION,
      preAllocatedVUs: env.preAllocatedVUs,
      maxVUs: env.maxVUs,
    },
    abuse: {
      executor: "constant-arrival-rate",
      exec: "abuse",
      rate: 2,
      timeUnit: "1s",
      duration: DURATION,
      preAllocatedVUs: env.preAllocatedVUs,
      maxVUs: env.maxVUs,
    },
    grpc: {
      executor: "constant-arrival-rate",
      exec: "grpcScenario",
      rate: 1,
      timeUnit: "1s",
      duration: DURATION,
      preAllocatedVUs: env.preAllocatedVUs,
      maxVUs: env.maxVUs,
    },
  },
  thresholds: THRESHOLDS,
};
