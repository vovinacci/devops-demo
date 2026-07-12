# ADR-0012: Reproducible developer toolchain

- Status: Accepted
- Date: 2026-07-12
- Extracted from: RFC-0001 D14 (as implemented during Phase 0)

## Context

Five ecosystems mean "works with my Go 1.22" drift multiplies. Toolchain
versions appear in places that cannot import a shared value (Dockerfile
FROM lines, devcontainer image strings, .nvmrc, workflow YAML), so
duplication is physical and must be governed.

## Decision

`.mise.toml` is the single source of truth for runtime versions (mise
installs them; asdf-compatible). `make doctor` verifies the environment --
required tools, Docker daemon, versions against the pins, hooks runner,
RAM/disk -- and is the first command in every setup doc. A toolchain drift
gate (`scripts/check-toolchain-drift.sh`, run as a hook locally and in CI)
compares every consumer (.nvmrc, devcontainer, Dockerfile FROM tags,
requires-python floor) against the canon: duplication remains, drift
cannot be silent -- the same construction as the load-profile parity test
(ADR-0003). Git hooks run under prek (single binary, pre-commit
compatible); CI runs the same config via prek-action, and hook
environments are managed (no system binaries), so tool versions are
identical everywhere. A devcontainer provides the browser/Codespaces path.

## Alternatives

- Documented versions in README: rejected -- documentation is not a gate.
- asdf: rejected for speed and single-binary UX; mise reads the same
  format.
- Classic pre-commit as primary runner: kept as the documented fallback
  (same config); prek chosen for the single-binary UX. prek is young --
  the fallback is the mitigation.

## Consequences

- Easier: version bumps are one edit plus a gate that lists every other
  file to touch; local and CI hooks cannot diverge.
- Harder: one more tool (mise) in the onboarding path -- softened by
  `make doctor` hints (including shell activation); bare `mise use <tool>`
  rewrites pins to `latest` (warned in the file, caught by the gate).
