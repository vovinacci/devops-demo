# Agent instructions -- devops-demo

Universal instructions for AI coding agents (Claude Code, Cursor, Copilot,
Codex, Gemini, and others). This file (`AGENTS.md`, repo root) is the only
instruction file -- agents read the open standard natively; no tool-specific
copies or pointers exist. Rules are stated once, in the Hard rules list;
other sections reference them by number.

## What this repository is

A teaching platform: "same operational requirements, five runtimes"
(Python/FastAPI backend, React frontend, Go analytics, Kotlin reports,
Rust canary) wired to one observability stack, with shaped k6 load,
seeded history, and gRPC contracts. The *process* is part of the product:
decisions live in `docs/rfc/` and `docs/adr/`; conventions in
`docs/engineering-principles.md`. Read the governing document before
structural changes.

## Repository map

- `services/<name>/` -- self-contained services (own Dockerfile, Makefile,
  README, tests). Nested `AGENTS.md` files there add module specifics.
- `proto/` -- buf-managed Protobuf contracts.
- `loadprofile/` -- shared load-shape definition + cross-language parity test.
- `loadgen/` -- k6 scenarios.
- `observability/` -- Prometheus, Grafana, Loki, Alloy, Alertmanager,
  blackbox_exporter configs and dashboards (all provisioned from files).
- `deploy/compose/` -- Docker Compose (profiles: core + additive).
- `docs/` -- rfc/, adr/, runbooks/, exercises/, ci.md, engineering-principles.md.

## Hard rules

The single list of rules and invariants. Rationale and detail live in the
referenced documents; the rule itself is authoritative as written here.

1. **Never edit generated code.** gRPC stubs are generate-in-build
   (`make generate`); changes go in `proto/` sources. (RFC-0001 D8)
2. **Docs are ASCII-only**; `--`, `->`, `x`, "Section N"; Mermaid for
   diagrams; box-drawing allowed only in directory trees.
   (engineering-principles.md Section 1)
3. **Conventional Commits everywhere, PR titles included** (squash-merge
   repo; titles drive release-please version math). (RFC-0001 D12)
4. **Contract changes require a decision document.** `proto/`,
   `loadprofile/profile.json`, and the uniform service contract
   (healthz/readyz, /metrics, JSON logs, Make targets) change only with an
   ADR or RFC amendment. proto changes must pass `buf breaking`.
   (RFC-0001 D3, D6)
5. **Accepted RFCs/ADRs are immutable** -- supersede, never rewrite.
   (engineering-principles.md Section 3)
6. **No secrets in the repo**; gitleaks runs in hooks and CI; do not weaken
   or bypass it. (RFC-0001 D13)
7. **Analytics buckets on event time, never arrival time.** (RFC-0001 D1)
8. **Seam invariant:** seeded history and live traffic share one load-shape
   function and one time scale; do not touch either side alone.
   (RFC-0001 D5)
9. **Profile degradation:** services tolerate absent optional dependencies
   (e.g. canary skips its analytics step when analytics is down) rather
   than failing. (RFC-0001 D10)
10. **Mechanical and logic changes never mix** in one PR.
    (engineering-principles.md Section 4)
11. **Tests, docs, and dashboards ship with the change** -- Definition of
    Done. (engineering-principles.md Section 5)

## Reasoning protocol (Model-First Reasoning)

Before writing code, build an explicit model of the change; reason against
the model, not against the first file you found. Scale by change class:

- **Trivial** (typo, comment, single-file non-behavioral): skip.
- **Standard** (behavior change within one module): state briefly --
  affected files/components; which Hard rules (by number) the change
  touches; assumptions; plan.
- **Structural** (contracts, cross-module, new dependencies, schema/proto):
  the above, plus the governing RFC/ADR by name, and an explicit conflict
  declaration if your plan diverges from it.

The model is an artifact, not a thought: put it in the PR description under
the "Change model" section of the PR template. Reviewers read the model
first, the diff second. Applies to human authors equally.

## Verification (run before proposing changes as done)

- `make doctor` -- toolchain sanity (versions pinned in `.mise.toml`).
- `make ci` -- exactly what CI runs (includes `prek run --all-files`).
  Local/CI divergence is a bug (engineering-principles.md Section 6).
- Per-service: `make -C services/<name> build test lint`.
- Full stack when relevant: `make up` (core), `make up-full`, `make seed`,
  `make incident` / `make heal`.

## When unsure

- Design intent: `docs/rfc/0001-*.md` (plan), `docs/rfc/0000-*.md` (baseline).
- A specific decision: `docs/adr/`.
- Process/conventions: `docs/engineering-principles.md`.
- If a task conflicts with a Hard rule or an accepted decision: say so and
  propose an ADR instead of silently diverging.
