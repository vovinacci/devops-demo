# ADR-0009: CI/CD architecture and release engineering

- Status: Accepted
- Date: 2026-07-12
- Extracted from: RFC-0001 D12 (as amended during Phase 0)

## Context

Five ecosystems in one repository need per-module pipelines that scale with
the system, cross-cutting contract gates, automated releases, and automated
dependency updates -- built phase by phase, not as a hardening task later.

## Decision

Path-filtered per-module workflows plus always-on cross-cutting gates
(hooks via prek-action, PR title commitlint), image build + Trivy scan, and
(from Phase 4) one e2e stage on `core + analytics + load` with k6
thresholds; the JVM service is exercised nightly, not per-PR (7 GB runner
limit). `make ci` runs locally exactly what CI runs. Releases: squash-merge
only, PR titles linted as Conventional Commits, release-please maintains a
repo-level single-version release PR; merging it tags and (in a later
phase) publishes pinned images to GHCR -- never `latest`. Dependency
updates: Renovate, self-hosted in GitHub Actions (weekly cron + manual
dispatch), grouped per ecosystem, digest pinning for actions and base
images; runtime policy allows patch-only updates for Python/Go and blocks
Node majors. Branch protection requires only the always-on cross-cutting
checks.

## Alternatives

- One monolithic workflow: rejected -- runs everything on every change and
  hides per-module ownership.
- Merge commits / rebase-merge: rejected -- release-please version math is
  only as good as the linear title history it reads.
- Dependabot: GitHub-native and near zero-config, but loses on the deciding
  criterion -- five ecosystems plus hook versions, mise pins, and a digest
  policy need Renovate's managers, grouping, and monorepo awareness.
  Documented fallback if self-hosting becomes a burden.
- Hosted Mend Renovate app: rejected -- self-hosting keeps the automation
  in the repository as code, at the cost of owning the token and schedule.
- Requiring path-filtered checks in branch protection: rejected -- a
  workflow skipped by its path filter leaves required checks pending
  forever.

## Consequences

- Easier: green-locally means green-in-CI; releases assemble themselves;
  dependency PRs arrive grouped and pinned.
- Harder: a PAT with workflow scope is required (Renovate updates workflow
  files; release PRs must trigger CI); per-module checks rely on reviewer
  discipline rather than branch protection.
