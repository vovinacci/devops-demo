# ADR-0016: Gateway API via Envoy Gateway for the edge

- Status: Accepted
- Date: 2026-07-24
- Extracted from: RFC-0003 DK3

## Context

On Kubernetes the stack needs an external entry point to replace compose's
host-port publishing. One request path crosses frontend -> backend (REST and
gRPC) -> analytics -> reports, so the edge must route HTTP by host and also
route gRPC (the L7 lesson RFC-0001 flagged as out of scope under compose,
RFC-0001 Section 11). The edge is also where the future authentication capstone
(RFC-0004) attaches identity, so the choice must not preclude edge OIDC.

## Decision

External access is the **Gateway API** implemented by **Envoy Gateway**: one
`Gateway` resource plus `HTTPRoute`s for `frontend`, `reports-ui`, the backend
REST API, and Grafana, and a `GRPCRoute` for the backend's gRPC `ItemService`.
The Gateway API is the modern, role-oriented successor to Ingress (infra-owned
`Gateway` vs app-owned routes) and is superseding Ingress across the ecosystem.
Envoy Gateway is a CNCF, Envoy-based implementation whose `SecurityPolicy`
provides edge OIDC directly -- the exact hook RFC-0004 needs -- and whose
`GRPCRoute` support finally exercises the "gRPC through an L7 edge" lesson.

## Alternatives

- ingress-nginx / raw Ingress: rejected -- the older API, whose annotation-driven
  feature sprawl is the fragmentation the Gateway API was designed to replace,
  with no first-class gRPC route type; choosing it teaches the legacy shape.
- NodePort / port-forward only: rejected -- reaches a pod but teaches none of the
  edge routing, host-based virtual routing, or OIDC-attachment point that is
  half the reason to stand up an edge.

## Consequences

- Easier: one edge routes HTTP and gRPC by host; the OIDC-attachment point for
  RFC-0004 exists from day one; the routing model is the current standard.
- Harder: the Gateway API and Envoy Gateway are newer and less familiar than
  Ingress; the CRDs (`Gateway`, `HTTPRoute`, `GRPCRoute`) are additional
  concepts to teach, and Kind has no cloud LoadBalancer so the Gateway is
  reached via a documented local method (RFC-0003 Section 9).
