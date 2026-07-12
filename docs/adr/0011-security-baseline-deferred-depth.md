# ADR-0011: Security baseline now, supply-chain depth deferred

- Status: Accepted
- Date: 2026-07-12
- Extracted from: RFC-0001 D13

## Context

Security gates should teach, not cargo-cult. Some failures are instantly
understandable to students; provenance and attestation only make sense
after builds, releases, and dependencies have been seen breaking in
ordinary ways.

## Decision

In scope now, in every pipeline: gitleaks secret scanning (in git hooks and
CI -- "you committed a secret" needs no security background) and Trivy
image scanning failing on critical CVEs ("your image ships a known
vulnerability" is core DevOps literacy). Deferred to a security capstone:
SBOM generation (syft) on releases, CodeQL static analysis (covers
Python/JS/Go/Kotlin -- notably not Rust, documented honestly), and image
signing/verification (cosign).

## Alternatives

- Full supply-chain stack now: rejected -- conceptually heavy before the
  rest of the pipeline is understood, and cosign verification is weak in a
  compose-only world; do not cargo-cult it in.
- No security gates until the capstone: rejected -- both baseline gates are
  near-free and produce teachable red builds.

## Consequences

- Easier: every pipeline has a security stage from Phase 0; a red gate
  carries a real CVE identifier, a teachable moment.
- Harder: the deferred depth is a documented, visible gap until the
  capstone phase.
