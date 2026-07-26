# ADR-0017: Kubernetes-native observability via the Prometheus Operator

- Status: Accepted
- Date: 2026-07-24
- Extracted from: RFC-0003 DK6

## Context

The compose stack scrapes metrics from a static `observability/prometheus.yml`
and provisions Grafana dashboards from files, with Alloy tailing logs via the
mounted Docker socket. On Kubernetes those static, file-driven mechanisms are
the wrong model: discovery should be label-driven and monitoring should be
reconciled by an operator, which is how observability is actually run on
Kubernetes. Moving to Kubernetes without moving the observability model would
run Prometheus on Kubernetes while teaching none of it.

## Decision

Adopt the **Prometheus Operator** via the **kube-prometheus-stack** chart. It
runs Prometheus, Alertmanager, and Grafana; **per-service ServiceMonitors**
select each `/metrics` endpoint, retiring the static `prometheus.yml` scrape
jobs (discovery is now label-driven). **Grafana dashboards ship as ConfigMaps**
(the operator's sidecar loads any ConfigMap carrying the dashboard label),
replacing file provisioning while reusing the same dashboard JSON. **Loki**
stays and **Alloy runs as a DaemonSet**, each pod tailing only its OWN node's
pod logs via node-local discovery (the Kubernetes SD `node` meta-label pinned
to the pod's `spec.nodeName`) instead of the Docker socket. Node-local scoping
is required, not incidental: a DaemonSet where every Alloy replica discovered
pods cluster-wide would ship each log line once per node, duplicating
everything in Loki. The standalone
`cadvisor` container is retired -- the kubelet already exposes cAdvisor metrics
and kube-prometheus-stack ships node-exporter -- while `postgres_exporter`
stays with its own ServiceMonitor.

## Alternatives

- Lift the static `prometheus.yml` into a ConfigMap verbatim: rejected -- runs
  Prometheus on Kubernetes while teaching none of the Kubernetes-native
  monitoring model; a missed lesson dressed up as a port.
- A standalone Prometheus without the Operator: kept only as an optional
  lighter-weight local path for RAM-constrained laptops (RFC-0003 Section 9),
  not the default -- it forgoes the ServiceMonitor/reconciliation exhibit.

## Consequences

- Easier: adding a service to monitoring is a ServiceMonitor label, not a
  scrape-config edit; dashboards and alerts are reconciled by the operator; the
  static-vs-discovery contrast against compose is itself an exhibit.
- Harder: the Operator and its CRDs (ServiceMonitor, Probe, Prometheus,
  Alertmanager) are more to learn and a meaningful share of the Kind RAM budget
  (RFC-0003 Section 9), which is why an optional-Operator local path is kept.
