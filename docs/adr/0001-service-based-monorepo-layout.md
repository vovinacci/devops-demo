# ADR-0001: Service-based monorepo layout

- Status: Accepted
- Date: 2026-07-12
- Extracted from: RFC-0001 D8

## Context

The repository grows from two modules to six (backend, frontend, analytics,
reports, canary, loadgen) across five language ecosystems. `backend/` and
`frontend/` at the repo root do not scale, and cross-cutting contracts
(proto, load profile) need one home. The demo's value is the whole system in
one clone.

## Decision

One monorepo with a service/module-based top level: each `services/<name>/`
is self-contained (own Dockerfile, Makefile implementing the uniform target
set, README, tests); cross-service sources of truth live at the root
(`proto/`, `loadprofile/`); deployment config lives in `deploy/`; the root
Makefile orchestrates and delegates.

## Alternatives

- Per-service repositories (polyrepo): rejected -- cross-cutting contracts
  become expensive, and "clone one repo, see the whole system" is the
  product.
- Language-based top level (`go/`, `python/`): rejected -- a directory named
  after a language says nothing about responsibility.

## Consequences

- Easier: path-filtered CI per module, per-service ownership, nested
  `AGENTS.md` for module specifics, one-command bring-up.
- Harder: root tooling (Makefile, CI filters) must scale with module count.
- Accepted: mechanical migration cost (git mv preserves history; moves and
  logic changes never mix).
