// Loadgen scale guard (RFC-0001 Phase 5 PR-2, D5 enforcement): runs once
// in k6's setup() phase, before any VU/scenario starts, so a scale
// mismatch aborts the whole test instead of running an hour on the wrong
// shape. Fetches analytics' seed-marker (`GET /api/v1/seed-marker`,
// services/analytics/internal/httpserver) and reasons over three
// distinct outcomes:
//
//   - 200 + marker.scale !== DEMO_TIME_SCALE -- refuse: the seeder and
//     this run disagree on the shared load-shape scale (ADR-0003 Hard
//     rule 8), which silently breaks the seam invariant (history at one
//     diurnal frequency, live traffic at another). setup() throwing here
//     is what makes k6 abort the run.
//   - 404 -- analytics has never completed a `analytics seed` run on
//     this stack (a fresh `make up` with no `make seed-history` yet).
//     Not an error: info log, continue.
//   - connection error / timeout / any other status -- `analytics` is a
//     separate opt-in compose profile from `load` (D10 graceful
//     degradation): its absence must not brick `make up` with no
//     analytics profile at all. Warn, continue.
import http from "k6/http";

import { env } from "./env.js";

// Short by design: setup() runs once per k6 start and blocks every
// scenario from beginning until it returns, so this must not meaningfully
// delay loadgen's startup even when analytics is unreachable.
const SEED_MARKER_TIMEOUT = "2s";

export function checkSeedMarker() {
  const res = http.get(`${env.analyticsUrl}/api/v1/seed-marker`, {
    timeout: SEED_MARKER_TIMEOUT,
    tags: { name: "seed_marker_check" },
  });

  // k6 does not throw on network-level failures (DNS, connection
  // refused, timeout): it returns status 0 with `error` describing the
  // failure, rather than an exception -- checked first since it is not a
  // real HTTP response at all.
  if (res.status === 0) {
    console.warn(
      `seed-marker check: analytics unreachable at ${env.analyticsUrl} (${res.error}) -- continuing without the scale guard`
    );
    return;
  }

  if (res.status === 404) {
    console.info("seed-marker check: no seed marker found (analytics never seeded) -- continuing");
    return;
  }

  if (res.status !== 200) {
    console.warn(`seed-marker check: unexpected status ${res.status} from ${env.analyticsUrl} -- continuing`);
    return;
  }

  let marker;
  try {
    marker = res.json();
  } catch (err) {
    console.warn(`seed-marker check: failed to parse response body (${err}) -- continuing`);
    return;
  }

  if (marker.scale !== env.scale) {
    throw new Error(
      `seed scale ${marker.scale} != DEMO_TIME_SCALE ${env.scale} -- re-run \`make seed-history\` or match the env`
    );
  }
}
