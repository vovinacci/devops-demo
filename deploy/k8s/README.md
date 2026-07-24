# Kubernetes packaging (Helm)

Helm charts that package the RFC-0001/0002 stack for Kubernetes, per RFC-0003
(DK2, ADR-0015). **This is PR-2: the packaging only -- a pure, offline
templating exercise.** There is no cluster here: the charts render with `helm
template` and validate with `kubeconform` against the Kubernetes API schemas,
no `kubectl` and no Kind. The cluster itself (Kind, Envoy Gateway, the
`make kind-*` targets) is **PR-3**; the Kubernetes-native observability
(kube-prometheus-stack Operator, Grafana/Loki-Alloy) is **PR-4**.

## Why a library chart

RFC-0001 D6 (ADR-0004) requires every service to ship the *same shape*:
`/healthz` + `/readyz` health endpoints, `/metrics`, structured logs. On
Kubernetes that shape becomes a Deployment, a Service, probes, a resource
block, and a ServiceMonitor. Rather than copy that into five runtimes and
watch them drift, the shape lives **once** in a Helm **library chart**
(`charts/common`); each per-service chart is a few values that instantiate it.
The library chart is the D6 uniform contract made executable -- uniformity
enforced by templating, not by copies staying in sync.

`/healthz` -> `livenessProbe`, `/readyz` -> `readinessProbe`, `/metrics` ->
`ServiceMonitor`. That is the whole payoff of D6, mechanical and per-service
special-casing-free.

## Layout

```text
deploy/k8s/
  charts/
    common/                 library chart (type: library) -- the D6 shape
      templates/
        _helpers.tpl         names, labels, selectorLabels, image ref
        _ports.tpl           container + Service ports
        _probes.tpl          liveness/readiness/startup (the opt-out mechanism)
        _workload.tpl        common.deployment, common.statefulset
        _networking.tpl      common.serviceaccount/service/servicemonitor/pvc
    backend/ frontend/ analytics/ reports/ reports-ui/ canary/
    blackbox/ loadgen/ mailpit/ loki/ postgres-exporter/
                            thin per-service charts (import common)
    postgres/               ONE StatefulSet chart, aliased 3x by the umbrella
    platform/               umbrella chart (depends on the per-service charts)
      values.yaml            base = the `core` profile
      values-<profile>.yaml  additive overlays (analytics/reports/...)
  schemas/
    monitoring.coreos.com/servicemonitor_v1.json   vendored CRD schema
  scripts/validate.sh       the offline lint + render + kubeconform gate
```

Build artifacts (`charts/*/charts/`, `Chart.lock`) are gitignored -- they are
rebuilt by `helm dependency build` (the gate does this for you).

## Which services are charted (the PR-2/PR-4 boundary)

Charted here (application + supporting services, RFC-0003 Section 6):

- **App services** (Deployment + Service + ServiceMonitor): `backend`
  (REST :8000 + gRPC :50051), `analytics`, `reports`, `reports-ui`, `canary`.
- **The D6 exception** (Deployment + Service, no ServiceMonitor): `frontend`
  (see below).
- **Supporting infra** (third-party images, digest-pinned from compose):
  `mailpit`, `loki`, `blackbox`, `postgres-exporter` (this one keeps its
  ServiceMonitor), and `loadgen` (a Deployment only -- no listener).
- **Stateful** (StatefulSet + PVC + headless Service, RFC-0003 DK7, ADR-0018):
  the three Postgres instances `postgres-backend` / `postgres-analytics` /
  `postgres-reports`, and the `reports` artifact PVC.

**Deliberately NOT charted here:** `prometheus`, `grafana`, `alertmanager`,
`alloy`. RFC-0003 Section 6 sources those from the **kube-prometheus-stack**
Operator and a DaemonSet in **PR-4** -- charting them now would package the
wrong (static-config) shape. `cadvisor` is retired outright on Kubernetes (the
kubelet already exposes cAdvisor container metrics, DK6).

## The D6 opt-out mechanism (DK2) and the frontend exception (DK4)

The library's probes and ServiceMonitor are **per-service opt-out, default
on**. Each probe (liveness / readiness / startup) is a value with a `type`:
`httpGet` (default, D6) | `tcpSocket` | `grpc` | `exec` | `none`; the
ServiceMonitor is a boolean.

The **nginx `frontend` is the one documented D6 exception** (ADR-0013). Its SPA
fallback (`try_files ... /index.html`) answers *any* path with `index.html`, so
an `httpGet /healthz` probe would return 200 with the app shell and pass
without signal -- a vacuous probe worse than none. So the frontend chart sets:

```yaml
d6:
  liveness:  {type: tcpSocket, port: http}   # honest "the static server answers"
  readiness: {type: none}
  serviceMonitor:
    enabled: false                            # exposes no /metrics
```

and renders **no `httpGet /healthz` and no ServiceMonitor**. The Caddy
`reports-ui`, by contrast, *meets* D6 and takes the defaults -- the same
nginx-vs-Caddy contrast RFC-0002/ADR-0013 teaches, now visible in Kubernetes.
`postgres-exporter` reuses the same opt-out (tcpSocket liveness, it has no HTTP
health endpoint) while *keeping* its ServiceMonitor -- proof the two flags are
independent. `loadgen` opts out of everything (no Service, no probes, no
ServiceMonitor). The `backend` keeps the D6 HTTP pair and adds a **startup
probe of type `grpc`** against the gRPC Health Checking Protocol on `:50051`
(k8s allows one handler per probe, so the gRPC check rides the startupProbe).

## The JVM exhibit (DK5)

The `reports` chart carries the container-limit-vs-heap lesson as a first-class
exhibit: a memory *limit* alone does not bound the JVM heap, so its values set
`JAVA_TOOL_OPTIONS: -XX:MaxRAMPercentage=75.0` beside `resources.limits.memory`
(reports is the largest single consumer, ~400 MiB). See `charts/reports/values.yaml`.

## Profiles -> overlays (D10)

The umbrella's values overlays mirror the compose profiles one-to-one, so
`helm template` selects exactly the services `--profile` would. Additive
profiles **layer** on the base with a second `-f`:

| Compose | Helm overlay | Adds |
| --- | --- | --- |
| core (`make up`) | `values.yaml` | backend, frontend, postgres-backend, postgres-exporter, mailpit, loki |
| `--profile analytics` | `-f values.yaml -f values-analytics.yaml` | analytics, postgres-analytics |
| `--profile reports` | `... -f values-reports.yaml` | reports (+ artifact PVC), postgres-reports |
| `--profile reports-ui` | `... -f values-reports-ui.yaml` | reports-ui |
| `--profile synthetic` | `... -f values-synthetic.yaml` | blackbox, canary |
| `--profile load` | `... -f values-load.yaml` | loadgen |
| `make up-full` | `-f values.yaml -f values-full.yaml` | every additive profile |

Each additive profile is a `<name>.enabled` condition on the umbrella
dependency; the base turns the `core` set on and the rest off.

## Validate locally (== CI)

Everything below is offline -- no cluster. `helm` and `kubeconform` are pinned
in `.mise.toml`; `mise install` gets them.

```sh
make lint-k8s          # the whole gate: helm lint + template | kubeconform, all profiles
```

`make lint-k8s` is also part of `make lint` / `make ci`, and the k8s-lint CI
workflow (`.github/workflows/k8s-lint.yml`) runs the very same
`deploy/k8s/scripts/validate.sh`. To run pieces by hand:

```sh
helm lint deploy/k8s/charts/common deploy/k8s/charts/backend       # any chart
helm dependency build deploy/k8s/charts/platform                    # vendor subcharts
helm template platform deploy/k8s/charts/platform \
  -f deploy/k8s/charts/platform/values.yaml \
  -f deploy/k8s/charts/platform/values-full.yaml \
  | kubeconform -strict -summary \
      -schema-location default \
      -schema-location 'deploy/k8s/schemas/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'
```

The second `-schema-location` is the load-bearing one (DK9): it points
`kubeconform` at the vendored **ServiceMonitor** CRD schema so the Prometheus
Operator CR is **actually validated**. Without it the CR is an *error*
(`could not find schema`), not silently skipped -- which is exactly why this
gate does **not** use `--ignore-missing-schemas` (the DK9 trap: a green gate
that validates nothing).

## Deliberate deviation from the RFC's toolchain line

RFC-0003 DK9 lists `kind`, `kubectl`, `helm`, `kubeconform` as the new pinned
toolchains. PR-2 adds **only `helm` + `kubeconform`** to `.mise.toml` -- the
two this offline packaging PR genuinely needs. `kind` + `kubectl` wait for
PR-3, which stands up the cluster. Neither `helm` nor `kubeconform` has a
second version consumer (no Dockerfile `FROM`, no wrapper), so there is nothing
for `scripts/check-toolchain-drift.sh` to compare against and it is left
untouched.
