# ADR-0011: Security baseline now, supply-chain depth deferred

- Status: Accepted
- Date: 2026-07-12
- Extracted from: RFC-0001 D13
- Amended: 2026-08-27 -- terminology only. The deferred work below was called
  a "security capstone"; it is named supply-chain depth. The capstone is
  RFC-0004, authentication and authorization. No decision changed.

## Context

Security gates should teach, not cargo-cult. Some failures are instantly
understandable to students; provenance and attestation only make sense
after builds, releases, and dependencies have been seen breaking in
ordinary ways.

## Decision

In scope now, in every pipeline: gitleaks secret scanning (in git hooks and
CI -- "you committed a secret" needs no security background) and Trivy
image scanning failing on critical CVEs ("your image ships a known
vulnerability" is core DevOps literacy).

**Scan everything that runs; gate only what we build.** Every image in the
compose stack is scanned, including the ones we pull (Prometheus, Grafana,
Loki, Postgres, the exporters): an unscanned Grafana is an unscanned service,
and pretending otherwise would make the security stage a decoration. But the
two halves are gated differently, because the available response differs. A
critical CVE in an image we build has an action attached -- bump the base
image, rebuild -- so it fails the pipeline. A CVE in an upstream image does
not: we cannot patch Grafana, and the honest options are to wait for a release
or to stop using it. Failing the build on something no one in the room can fix
does not teach diligence, it teaches people to click through red. So pulled
images are reported (`exit-code: 0`) at a wider severity net, and the
self-built matrix keeps the hard CRITICAL gate.

That asymmetry is deliberate and stated rather than emergent: the previous
shape -- scanning only self-built images -- was silently narrower than "we
scan our images" implied to anyone reading the badge. The image list is
derived from the compose file at scan time, so it cannot drift out of sync
with what the stack actually runs.

Deferred to supply-chain depth, a separate future RFC:
SBOM generation (syft) on releases, CodeQL static analysis (covers
Python/JS/Go/Kotlin -- notably not Rust, documented honestly), and image
signing/verification (cosign).

## Alternatives

- Full supply-chain stack now: rejected -- conceptually heavy before the
  rest of the pipeline is understood, and cosign verification is weak in a
  compose-only world; do not cargo-cult it in.
- No security gates until that later RFC: rejected -- both baseline gates are
  near-free and produce teachable red builds.

## Consequences

- Easier: every pipeline has a security stage from Phase 0; a red gate
  carries a real CVE identifier, a teachable moment. Coverage now matches the
  claim -- every image the stack runs is scanned, not only the ones we build.
- Harder: the deferred depth is a documented, visible gap until that later
  RFC lands. The report-only lane needs someone to actually read it; a
  findings list nobody opens is worth about as much as no scan at all, and
  nothing in CI forces that habit.
