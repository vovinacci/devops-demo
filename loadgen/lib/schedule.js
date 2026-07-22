// Builds k6 `ramping-arrival-rate` stage arrays from the shared load
// profile (RFC-0001 D5, ADR-0003). Stages are computed once, at k6 init
// time (module load, before any VU runs) -- k6 options must be static by
// the time the test starts, so the whole future schedule for the run's
// duration is precomputed here rather than evaluated live per request.
//
// No Date.now() calls anywhere in this file: every timestamp fed to
// rate() is refUnixSeconds + i*stepSeconds, both plain numbers. This
// matters beyond style -- shape.js's `scale` compresses profile time
// *around a fixed refUnixSeconds* (loadprofile/README.md): if the
// reference moved with each evaluation (e.g. calling Date.now() again
// per stage) it would always equal the timestamp being evaluated,
// collapsing `(unixSeconds - ref) * scale` to zero and silently
// defeating DEMO_TIME_SCALE. refUnixSeconds is captured exactly once,
// by loadgen/entrypoint.sh, before k6 even starts.
import { rate } from "../loadprofile/shape.js";

// buildStages evaluates the shared profile at a fixed step across
// `totalSeconds`, scales it by this scenario's share of total traffic
// (`weight`), and returns k6 stage objects ({ target, duration }).
// `modulate(target, stepIndex, unixSeconds)` lets a scenario layer its
// own pattern (e.g. periodic bursts) on top of the shared diurnal shape.
export function buildStages({
  profile,
  refUnixSeconds,
  scale,
  totalSeconds,
  stepSeconds,
  weight,
  minTarget = 1,
  modulate,
}) {
  const steps = Math.max(1, Math.ceil(totalSeconds / stepSeconds));
  const stages = [];
  for (let i = 0; i < steps; i++) {
    const unixSeconds = refUnixSeconds + i * stepSeconds;
    const base = rate(unixSeconds, profile, { scale, refUnixSeconds });
    let target = base * weight;
    if (modulate) target = modulate(target, i, unixSeconds);
    // ramping-arrival-rate's `target` is requests/timeUnit and must be a
    // whole number (k6 rejects a fractional target with a JSON-unmarshal
    // error into its int64 field) -- sub-request-per-second precision
    // isn't meaningful for this demo's traffic volumes anyway.
    target = Math.max(minTarget, Math.round(target));
    stages.push({ target, duration: `${stepSeconds}s` });
  }
  return stages;
}
