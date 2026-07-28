# ADR-0014: Kubernetes on Kind as the local deployment target

- Status: Accepted
- Date: 2026-07-24
- Extracted from: RFC-0003 DK1

## Context

The platform (RFC-0001, RFC-0002) is deployed with Docker Compose. Compose
teaches process supervision and a private network but not the Kubernetes
primitives students operate on the job -- Deployments and rollouts, readiness
gating traffic, Services and cluster DNS, StatefulSets and PVCs, the Gateway
edge, and an operator reconciling state. A second, production-like deployment
target is needed, and it must run locally, in CI, and on a student laptop.

## Decision

Run the stack on a local **Kind** (Kubernetes-in-Docker) cluster: a pinned
`kind` binary and a digest-pinned node image, configured **multi-node** (one
control-plane, two workers) so scheduling and node placement are real rather
than degenerate. Kind runs upstream Kubernetes components in Docker
containers, making it the closest local approximation to a real cluster, the
substrate Kubernetes uses for its own conformance runs, and the most
CI-friendly option (`kind create cluster` in a plain Actions job, no nested
virtualization). Compose is retained as the fast local path; Kind is the
production-like path (see RFC-0003 DK10).

## Alternatives

- minikube: rejected -- heavier, VM-oriented by default, and its driver matrix
  is an extra variable; slower and fiddlier in CI than Kind's Docker
  containers.
- k3d / k3s: rejected as the default -- k3s swaps upstream components (embedded
  datastore, Traefik, servicelb, Klipper) for lighter ones, so it teaches k3s's
  substitutions rather than upstream Kubernetes. Kept as a documented
  lighter-weight fallback for the most RAM-constrained laptops.

## Consequences

- Easier: a real, throwaway, conformance-grade cluster locally and in CI; the
  same manifests run in both places; teardown is a single `kind delete`.
- Harder: Kind's RAM footprint is far above compose's (RFC-0003 Section 9), and
  Kind-isms (no cloud LoadBalancer, explicit local image loading) must be
  taught as local specifics, not universal Kubernetes behavior.
