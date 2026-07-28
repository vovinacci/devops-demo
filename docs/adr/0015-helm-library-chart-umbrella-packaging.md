# ADR-0015: Helm library-chart + umbrella packaging

- Status: Accepted
- Date: 2026-07-24
- Extracted from: RFC-0003 DK2

## Context

RFC-0001 D6 (ADR-0004) requires every service to ship the same shape:
health/readiness endpoints, `/metrics`, structured logs, a Dockerfile, and Make
targets. On Kubernetes that shape becomes a Deployment, a Service, probes, a
resource block, and a ServiceMonitor. Five runtimes must express that identical
shape without five copies drifting apart -- the same uniformity D6 exists to
enforce, now in manifests.

## Decision

Package the stack with **Helm** as a **library chart** plus thin per-service
application charts plus one umbrella chart. The library chart encodes the D6
service shape once (Deployment + Service + probes wired from `/healthz` and
`/readyz` + resources + ServiceMonitor); each per-service chart is a few values
(image, ports, env, resource numbers) that instantiate the library template.
The D6 pieces (each probe, the ServiceMonitor) are **default-on but
per-service opt-out** values: the nginx `frontend` is the sole documented
exception (ADR-0013 -- no `/healthz`/`/readyz`/`/metrics`), so its chart opts
out of the HTTP probes (an honest `tcpSocket` liveness instead) and out of the
ServiceMonitor, without special-casing the library template.
The umbrella chart pulls the per-service charts as dependencies, and per-profile
**values overlays** (`core`, `+analytics`, `+reports`, `+reports-ui`,
`+synthetic`, `+load`) mirror the compose profiles (RFC-0001 D10, ADR-0008)
one-to-one. The library chart is the Kubernetes expression of the D6 uniform
contract: the shape is enforced by templating, not by copies staying in sync.

## Alternatives

- Raw manifests + Kustomize: rejected -- Kustomize overlays variants well but
  has no equivalent of a library chart's parameterized template reuse, so the
  D6 shape would be duplicated or patched across services, the DRY loss the
  library chart exists to prevent.
- Per-service copy-pasted manifests: rejected -- the anti-pattern D6 was written
  to kill; near-identical Deployment specs drift the moment one is edited alone.

## Consequences

- Easier: a new service is a handful of values against a reviewed template;
  the profile mental model is shared with compose; `helm lint` +
  `helm template | kubeconform` gate every change (RFC-0003 DK9).
- Harder: three chart layers (library, per-service, umbrella) are more moving
  parts than a flat manifest directory, and Helm templating is its own skill to
  teach.
