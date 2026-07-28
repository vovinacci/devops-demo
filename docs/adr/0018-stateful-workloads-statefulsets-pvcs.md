# ADR-0018: Stateful workloads as StatefulSets with PVCs on Kubernetes

- Status: Accepted
- Date: 2026-07-24
- Extracted from: RFC-0003 DK7

## Context

RFC-0003 runs the RFC-0001/0002 stack on a local Kubernetes cluster (Kind).
The stack carries persistent state that compose held in named volumes: three
independent Postgres instances (backend, analytics, reports) and the reports
service's generated-artifact volume. Kubernetes offers more than one way to
run a stateful pod, and the choice is a teaching decision -- the naive one
quietly loses data. "Service owns its store" (RFC-0001 D1) already gives each
database a single owner, so the three databases map to three separate
workloads rather than one shared store.

## Decision

The three Postgres instances run as **StatefulSets**, each with its own
**PersistentVolumeClaim** and a **headless Service** for stable network
identity. The reports service's artifact volume becomes a `ReadWriteOnce`
**PVC**.

StatefulSets are chosen because stable, sticky per-replica identity (a
predictable pod name and DNS entry) plus durable per-replica storage that
survives pod recreation is precisely their use case. "Survives" is scoped to
the owning node: the volume is node-local, so a recreated pod reattaches to it
only where the node is still available. Local volumes do not follow a pod to
another node, and recovery from node loss is out of scope for this platform. Modelling each database
as its own StatefulSet keeps the D1 ownership boundary visible in the cluster
topology exactly as it is in compose: three stores, three owners, no shared
volume. The reports artifact PVC teaches the volume-lifecycle-vs-pod-lifecycle
distinction directly -- the artifacts outlive any single reports pod, and (per
RFC-0001 D2) they are files on a volume, never BLOBs in Postgres.

## Alternatives

- **Deployments with `emptyDir`**: rejected -- `emptyDir` dies with the pod, so
  a database on it loses all data on any reschedule. A database on ephemeral
  storage is a teaching anti-pattern, not a shortcut.
- **An in-cluster Postgres operator** (CloudNativePG, Zalando): rejected for
  this RFC -- excellent in production, but it introduces an operator and its
  CRDs that are orthogonal to the StatefulSet/PVC primitives this platform
  exists to teach, and it costs RAM the Kind budget cannot spare. Noted as
  future work (RFC-0003 Section 10).

## Consequences

- Easier: each database keeps a durable identity and volume across pod
  recreation; the D1 "service owns its store" boundary stays legible in the
  cluster; the reports artifact PVC makes volume lifecycle a first-class
  exhibit.
- Harder: PVCs on Kind are backed by local-path storage (`ReadWriteOnce`,
  node-local), so a rescheduled stateful pod stays pinned to its node's volume
  -- a documented Kind-ism, not a portable multi-node storage story. Real
  replicated/HA Postgres remains out of scope, deferred to the operator path
  above.
