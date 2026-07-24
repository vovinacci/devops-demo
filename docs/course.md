# Course: operating a polyglot platform

This repository is a semester's worth of teaching material disguised as a
working system. It is a five-runtime platform (Python, JavaScript, Go, Rust,
Kotlin/JVM) with contract-first gRPC, shaped load, seamless historical data,
and three-layer monitoring -- all under one observability roof and one uniform
service contract.

The thesis: **done means operable, not `200 OK`.** Every module makes one
operational difference visible on a dashboard, then asks you to break it and
watch which layer notices. The *process* -- the RFCs that argued each decision,
the ADRs that froze it, the runbooks that operate it, and the exercises that
stress it -- is the material, not a wrapper around it.

The semester is nine modules over roughly fourteen weeks: the seven build
phases of [RFC-0001](rfc/0001-polyglot-platform.md), the sibling
[RFC-0002](rfc/0002-reports-ui.md) integration exhibit, and two upcoming
modules (Kubernetes on Kind, then an authentication capstone) that are in
design.

## How to use this course

The model is **hybrid: run the present, read the past.**

- **Run current `main`.** Every module's guided path is the live system on
  `main`. Start with `make doctor` (toolchain, Docker, RAM), then bring up the
  profile the module needs -- `make up` for the core stack, `make up-full` for
  every optional profile, or a specific `--profile` selection. Then do the
  module's exercise against the running stack.
- **Read how it grew.** Each build phase left an annotated git tag. Browse a
  phase's exact state with `git checkout phase-3`, or see precisely what a
  phase added with `git diff phase-2..phase-3`. Read the phase alongside its
  RFC-0001 decision records and the ADRs that landed with it.
- **Operate it.** The runbooks in [`docs/runbooks/`](runbooks/) are the ops
  half of every module: an alert fires, you open the runbook, you triage.

`main` is the single source of truth. Bug fixes, doc corrections, and new
exercises always land on `main` -- the tags are **read-only checkpoints** that
capture what a phase looked like when it shipped, never patched in place. When
`main` and a tag disagree, `main` is right; the tag is history.

## Course map

| Module                              | Phase / RFC | What lands                        | Read (RFC + ADR)          | Run                              | Do    | Operate (runbooks)                              | Tag        |
| ----------------------------------- | ----------- | --------------------------------- | ------------------------- | -------------------------------- | ----- | ----------------------------------------------- | ---------- |
| 0. Baseline and operability         | Phase 0     | Repo layout, CI, toolchain        | D6, D8, D14; ADR 1/4/9/12 | `make up`                        | ex 00 | --                                              | `phase-0`  |
| 1. Three-layer monitoring           | Phase 1     | blackbox + canary v1              | D6, D9, D10; ADR 7/8      | `make up-full`                   | ex 01 | probe-down, canary-journey-failing              | `phase-1`  |
| 2. Contract-first gRPC              | Phase 2     | proto/ + gRPC server + buf gate   | D3, D11; ADR 2/10         | `make up`                        | ex 02 | --                                              | `phase-2`  |
| 3. Analytics and the event stream   | Phase 3     | Go analytics + canary v2          | D1, D3, D7; ADR 5/2       | `make up-full`                   | ex 03 | analytics-stream-down, canary-pipeline-lag-high | `phase-3`  |
| 4. Load and incident-to-inbox       | Phase 4     | loadgen + Alertmanager + e2e gate | D4, D5; ADR 3/6/7/8       | `make up-full` + `make incident` | ex 04 | all four (triage)                               | `phase-4`  |
| 5. Historical data and seams        | Phase 5     | seeder + history dashboards       | D5, D7; ADR 5/3           | `make up-workshop` + seed        | ex 05 | --                                              | `phase-5`  |
| 6. The JVM showcase                 | Phase 6     | Kotlin reports + canary v3        | D2, D6, D10, D12; ADR 4   | `make up-full`                   | ex 06 | --                                              | `phase-6`  |
| 7. A UI over the API                | RFC-0002    | reports-ui Caddy SPA (`:8084`)    | RFC-0002; ADR 13          | `make up-full`                   | --    | --                                              | `rfc-0002` |
| 8. Into Kind (UPCOMING)             | RFC-0003    | K8s/Kind, Gateway API, Helm       | RFC-0003 (in design)      | --                               | --    | --                                              | `rfc-0003` |
| 9. Capstone: authN/authZ (UPCOMING) | RFC-0004    | OIDC across the polyglot mesh     | RFC-0004 (in design)      | --                               | --    | --                                              | `rfc-0004` |

`ex NN` is [`docs/exercises/NN-*.md`](exercises/00-baseline.md); `D`n is the
decision record of that number in [RFC-0001 Section 6-12](rfc/0001-polyglot-platform.md);
`ADR n` is [`docs/adr/000n-*.md`](adr/0001-service-based-monorepo-layout.md).

## Modules

### Module 0 -- Baseline and operability

The lesson: a service that returns `200` is not done -- it is done when it is
operable. Read [RFC-0000](rfc/0000-baseline-retrospective.md) and RFC-0001
decisions D6 (uniform contract), D8 (monorepo layout), D14 (toolchain); ADRs
[0001](adr/0001-service-based-monorepo-layout.md),
[0004](adr/0004-uniform-service-contract.md),
[0009](adr/0009-cicd-architecture-releases.md),
[0012](adr/0012-reproducible-developer-toolchain.md). Bring up the core stack
with `make up` and do [exercise 00](exercises/00-baseline.md). Discussion: what
would you need to see before you would page someone at 3am?

### Module 1 -- Three-layer monitoring

The lesson: whitebox, blackbox, and synthetic layers each notice different
failures. Read RFC-0001 D6, D9, D10; ADRs
[0007](adr/0007-three-layer-monitoring.md),
[0008](adr/0008-compose-profiles-degradation.md). `make up-full` adds the
`synthetic` profile (blackbox_exporter + Rust canary v1). Do
[exercise 01](exercises/01-monitoring-layers.md); operate with
[probe-down](runbooks/probe-down.md) and
[canary-journey-failing](runbooks/canary-journey-failing.md). Discussion: which
layer fires first, and why is that the wrong one to alert on alone?

### Module 2 -- Contract-first gRPC and tracing

The lesson: the contract is the artifact; code is downstream of it. Read
RFC-0001 D3, D11; ADRs [0002](adr/0002-grpc-direction-streaming-buf.md),
[0010](adr/0010-opentelemetry-day-one.md). The core stack (`make up`) already
serves the gRPC port. Do [exercise 02](exercises/02-grpc-contract.md) -- call
the contract, then break it and watch the `buf` CI gate reject it. Discussion:
what does "contract-first" buy you that a shared client library does not?

### Module 3 -- Analytics and the event stream

The lesson: a service that owns its own store, and the durability gap that
choice exposes. Read RFC-0001 D1, D3, D7; ADRs
[0005](adr/0005-analytics-data-ownership-retention.md),
[0002](adr/0002-grpc-direction-streaming-buf.md). `make up-full` (or add
`--profile analytics`) brings up Go analytics and its own Postgres; canary v2
now measures pipeline lag. Do
[exercise 03](exercises/03-break-the-event-stream.md); operate with
[analytics-stream-down](runbooks/analytics-stream-down.md) and
[canary-pipeline-lag-high](runbooks/canary-pipeline-lag-high.md). Discussion:
break the stream -- which monitoring layer fires first?

### Module 4 -- Load and incident-to-inbox

The lesson: dashboards without traffic are screenshots; an incident should end
in someone's inbox. Read RFC-0001 D4, D5; ADRs
[0003](adr/0003-shared-load-profile-stitching.md),
[0006](adr/0006-k6-load-generator-ci-gate.md),
[0007](adr/0007-three-layer-monitoring.md),
[0008](adr/0008-compose-profiles-degradation.md). `make up-full` brings up
shaped k6 load, Alertmanager, and Mailpit; `make incident` overlays a fault.
Do [exercise 04](exercises/04-incident-to-inbox.md) and use all four runbooks
as a triage drill. Discussion: trace one alert from metric to inbox -- where
could it silently drop?

### Module 5 -- Historical data and seams

The lesson: seeded history must stitch into live traffic with no visible seam.
Read RFC-0001 D5, D7; ADRs
[0005](adr/0005-analytics-data-ownership-retention.md),
[0003](adr/0003-shared-load-profile-stitching.md). Use workshop mode:
`make up-workshop` (hardcodes `DEMO_TIME_SCALE=24`, so one profile-day is one
wall-clock hour), then `DEMO_TIME_SCALE=24 SEED_DAYS=3 make seed-history` at
the matching scale. Do [exercise 05](exercises/05-find-the-seeded-anomalies.md)
-- find the three seeded anomalies. Discussion: where is the seam, and how
would you prove there is not one?

### Module 6 -- The JVM showcase

The lesson: same operational requirements, a very different runtime. Read
RFC-0001 D2, D6, D10, D12; ADR
[0004](adr/0004-uniform-service-contract.md). `make up-full` adds the `reports`
profile -- Kotlin/Spring Boot on the JVM (`:8083`), canary v3's report step,
and a report k6 scenario. Do
[exercise 06](exercises/06-watch-the-gc-sawtooth.md) -- watch the GC sawtooth
under report load. Discussion: what does the JVM make visible that the Go and
Rust services do not?

### Module 7 -- A UI over the API (RFC-0002)

The lesson: a static SPA is still an operable service -- health, metrics, and a
reverse proxy included. Read [RFC-0002](rfc/0002-reports-ui.md); ADR
[0013](adr/0013-caddy-static-server-reverse-proxy.md). `make up-full` adds the
`reports-ui` profile: a Caddy-served SPA on `:8084` that proxies the reports
API and re-exposes its own `/healthz` and `/metrics`. No dedicated exercise yet
-- explore the UI and read the reverse-proxy and metrics story. Discussion:
what makes a "just static files" service page-worthy?

### Module 8 -- Place it all in Kind (RFC-0003, UPCOMING)

The deployment and operability platform. Kubernetes on Kind, Gateway API via
Envoy, Helm charts, the D6 health endpoints wired to probes, the measured
per-service footprint turned into resource requests and limits, and the
Prometheus Operator with ServiceMonitors. This is where the whole polyglot
stack becomes production-like. Currently in design; the `rfc-0003` tag will
mark it.

### Module 9 -- Capstone: security, authN/authZ (RFC-0004, UPCOMING)

The platform today has zero auth -- everything is open with demo credentials
(ADR-0011 deferred it; RFC-0001 Section 10 names a security capstone). This
capstone adds authentication and authorization across the polyglot mesh, riding
on the Kind/Kubernetes platform from Module 8: OIDC login on the SPAs
(Authorization Code + PKCE), then JWT/JWKS validation and role/scope authz in
every service -- each language its own way, a cross-language exhibit like the D6
contract -- then edge OIDC at the Envoy Gateway (SecurityPolicy) and
service-to-service auth (client-credentials / mTLS). It is provider-agnostic:
everything integrates against the standard OIDC discovery/JWKS contract, so any
compliant provider drops in. The repo ships three interchangeable provider
deployments -- Keycloak, Zitadel, Dex -- and **students choose one**; swapping
providers is itself the lesson, the same "two implementations of one contract"
pedagogy as the nginx/Caddy contrast. Currently in design; the `rfc-0004` tag
will mark it. Discussion: where does each language draw the line between
authentication and authorization, and what breaks when you swap the provider?

## Instructor notes

- **Pacing.** Roughly one to two weeks per module fills a ~14-week semester:
  seven RFC-0001 phases, the RFC-0002 sibling, and the two upcoming modules.
  Front-load Modules 0-1; Modules 3-4 carry the most operational depth.
- **Compressed sessions.** For in-class demos use workshop mode
  (`make up-workshop`, `DEMO_TIME_SCALE=24`): a 90-day history plays out in
  hours. Seed at the same scale or loadgen's guard refuses to start against a
  mismatched marker.
- **Hardware.** Point students at `make doctor` first -- it checks Docker and
  RAM before anything starts. The core stack (`make up`) runs ~10 containers;
  the full profile set (`make up-full`) runs ~18, and the Kind module in
  Module 8 will need noticeably more headroom.
- **Assessment.** Each exercise ends in a *Discussion questions* section --
  those double as assignments, quiz prompts, or lab write-ups. `make ci` is the
  same gate the platform holds itself to, so "make CI green" is a fair grading
  bar.

## How the platform was built

Every build phase is an annotated tag and a matching RFC-0001 decision trail.
To read the construction history end to end: walk the tags in order
(`git log --oneline phase-0..phase-6`), diff adjacent phases to see exactly
what each one added (`git diff phase-1..phase-2`), and read each phase's
section of [RFC-0001](rfc/0001-polyglot-platform.md) beside the
[ADRs](adr/) that froze its decisions. The
[engineering principles](engineering-principles.md) doc explains why the
RFC-then-ADR lifecycle exists at all.
