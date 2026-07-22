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

- `services/<name>/` -- self-contained services (own Dockerfile, README,
  tests). Nested `AGENTS.md` files there add module specifics.
- `proto/` -- buf-managed Protobuf contracts.
- `loadprofile/` -- shared load-shape definition + parity test (RFC-0001 D5, ADR-0003).
- `loadgen/` -- k6 scenarios + incident mode (RFC-0001 D4, ADR-0006).
- `observability/` -- Prometheus, Grafana, Loki, Alloy, blackbox_exporter,
  Alertmanager + Mailpit configs and dashboards, all provisioned from files.
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
- Full stack when relevant: `make up` (core), `make up-full` (all
  profiles), `make seed`, `make test`.
- `make seed-history` -- historical seeder (RFC-0001 Phase 5 D5): needs
  the `analytics` profile already up -- run `make up-full` first (or
  `docker compose -f deploy/compose/docker-compose.yml
  --project-directory . --profile analytics up -d --build` for the
  analytics profile alone); `SEED_DAYS`/`SEED_SEED` override the
  `--days 90`/`--seed 42` defaults. Re-running on top of existing data
  is idempotent-ish, not exact (see `services/analytics/README.md`'s
  Historical seed section); clean re-seed = drop the analytics volume
  first: `docker compose -f deploy/compose/docker-compose.yml
  --project-directory . --profile analytics down -v`.
- Per-service targets where the service ships its own Makefile:
  `make -C services/canary build test lint run`.
- `make generate` -- regenerates gRPC/protobuf Python stubs into
  `services/backend/app/proto_gen` (buf + grpcio-tools; never committed,
  RFC-0001 D8, ADR-0002). Run once after clone/proto change for IDE
  completion; CI regenerates in its own build.
- `make incident` / `make heal` -- one-shot k6 incident overlay (10x
  spike or error storm) and its kill switch (RFC-0001 D4, ADR-0006);
  verify recovery afterward on the load dashboard and in Mailpit.
- `make smoke` -- the e2e CI stage (`.github/workflows/e2e.yml`), runnable
  locally: brings up `core + analytics + load`, runs the k6 smoke gate,
  asserts service wiring (RFC-0001 D12, PR-4). CI-only otherwise: not
  part of `make ci` (it needs Docker and takes longer than a lint/test
  pass), but the local == CI command is the same one either way.
  `make smoke-full` is the nightly workflow's equivalent (all current
  profiles, longer run).

## Rejected Findings

Review findings evaluated and deliberately not applied. Read before
producing review output; do not re-raise these.

- `services/backend/tests/test_health.py`: "del app.dependency_overrides
  in finally can raise KeyError; use .pop()" -- rejected: the assignment
  is unconditional immediately before the try, KeyError is unreachable.
- `proto/devopsdemo/items/v1/items.proto`: "WatchItemEvents should
  return a WatchItemEventsResponse message (buf RPC_RESPONSE_STANDARD_NAME)"
  -- rejected: the stream deliberately carries the domain event message
  (ItemEvent); a wrapper adds noise. Recorded as a scoped `except:` in
  `proto/buf.yaml`.

## When unsure

- Design intent: `docs/rfc/0001-*.md` (plan), `docs/rfc/0000-*.md` (baseline).
- A specific decision: `docs/adr/`.
- Process/conventions: `docs/engineering-principles.md`.
- If a task conflicts with a Hard rule or an accepted decision: say so and
  propose an ADR instead of silently diverging.
