# Kubernetes packaging (Helm)

The RFC-0001/0002 stack on Kubernetes, per RFC-0003: Helm charts that package
it (DK2, ADR-0015) and a local Kind cluster to run them on (DK1, DK10). Two
independent things live here, and they stay independent on purpose:

- **`make lint-k8s`** -- offline. Renders every profile with `helm template`
  and schema-validates it with `kubeconform`. No cluster, no `kubectl`; this
  is the per-PR CI gate.
- **`make kind-up` / `make kind-deploy` / `make kind-down`** -- the real
  thing. A multi-node Kind cluster, Envoy Gateway at the edge, the charts
  installed on top.

Observability is Kubernetes-native: the Prometheus Operator discovers targets
from ServiceMonitors instead of a static scrape file, and Grafana loads
dashboards from labelled ConfigMaps instead of a mounted directory.

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
        _config.tpl          common.configmap/secret/fileconfigmap/envFrom
    backend/ frontend/ analytics/ reports/ reports-ui/ canary/
    blackbox/ loadgen/ mailpit/ loki/ postgres-exporter/
                            thin per-service charts (import common)
    alloy/                  log-shipping DaemonSet (its own config, a rewrite)
    postgres/               ONE StatefulSet chart, aliased 3x by the umbrella
    platform/               umbrella chart (depends on the per-service charts)
      templates/gateway.yaml     the edge: Gateway + HTTPRoutes + GRPCRoute
      templates/monitoring.yaml  PrometheusRules + the blackbox Probe
      templates/dashboards.yaml  Grafana dashboards as labelled ConfigMaps
      files/rules/               copies of the compose rule files
      files/dashboards/          copies of the compose dashboards
      values.yaml            base = the `core` profile
      values-<profile>.yaml  additive overlays (analytics/reports/...)
  kind/
    cluster.yaml            multi-node cluster, digest-pinned node image
    gatewayclass.yaml       cluster-scoped GatewayClass for Envoy Gateway
    envoyproxy.yaml         Kind-only: pins the edge Service to NodePort 30080
    referencegrant.yaml     lets the platform's route reach Grafana's namespace
    kube-prometheus-stack-values.yaml   the observability stack, trimmed for Kind
  schemas/
    monitoring.coreos.com/servicemonitor_v1.json     vendored CRD schemas
    gateway.networking.k8s.io/{gateway,httproute,grpcroute}_v1.json
  scripts/
    validate.sh             the offline lint + render + kubeconform gate
    kind-up.sh              cluster + Envoy Gateway + kube-prometheus-stack
    kind-deploy.sh          build + `kind load` + `helm upgrade --install`
    kind-seed.sh            items + analytics history (schema is a hook Job)
    kind-down.sh            delete the cluster
```

Two charts carry a `files/` directory (`loki`, `blackbox`): a copy of the
config file compose bind-mounts, which becomes a ConfigMap here. Helm cannot
read outside a chart, so the copy is physical -- and `validate.sh` diffs it
against the original so it cannot drift.

Build artifacts (`charts/*/charts/`, `Chart.lock`) are gitignored -- they are
rebuilt by `helm dependency build` (the gate does this for you).

## Which services are charted, and which come from the stack

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

- **The log shipper** (DaemonSet): `alloy`, one collector per node.

**Deliberately NOT charted here:** `prometheus`, `grafana`, `alertmanager`.
They come from **kube-prometheus-stack**, installed by `make kind-up` as a
cluster prerequisite -- charting them would package the wrong (static-config)
shape, and a remote chart in the umbrella would make the offline CI gate
reach the network. `cadvisor` is retired outright on Kubernetes (the kubelet
already exposes cAdvisor container metrics, DK6).

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

## Configuration and credentials (DK8)

Compose passes everything as plain `environment:` entries. Kubernetes splits
them by **kind of data**, and that split is the lesson:

| | Object | Holds |
| --- | --- | --- |
| `config:` | ConfigMap `<release>-<svc>-config` | upstream URLs, tuning knobs, `ENVIRONMENT`, `JAVA_TOOL_OPTIONS` |
| `secret:` | Secret `<release>-<svc>-secret` | anything carrying a password -- including the DSNs that embed one |
| `configFiles:` | ConfigMap `<release>-<svc>-files` | whole config files for services that take `--config.file` (loki, blackbox) |

Both env-shaped objects are consumed with `envFrom`, so the container sees the
same variable names it sees under compose and no service code changes.

Which values are credentials is decided by content, not by name. Four DSNs
embed the demo password and are therefore Secrets -- `DATABASE_URL` and
`ALEMBIC_DATABASE_URL` (backend), `DATA_SOURCE_NAME` (postgres-exporter),
`ANALYTICS_DATABASE_URL` (analytics) -- while `reports` is the one service
whose JDBC URL keeps the user and password in separate variables, so only
`REPORTS_DATABASE_PASSWORD` is a Secret and the URL stays a plain setting.

Values are rendered through `tpl`, so an in-cluster address is written against
the release rather than hardcoded:

```yaml
config:
  REPORTS_BACKEND_URL: "http://{{ .Release.Name }}-backend:8000"
```

Compose could hardcode `http://api:8000` because the compose service name *is*
the DNS name; Helm names objects `<release>-<chart>`, so the reference has to
be computed. The one image that had a peer's hostname baked in -- nginx's
`proxy_pass http://api:8000` in the `frontend` -- now reads it from
`FRONTEND_BACKEND_URL` through nginx's own envsubst template mechanism, with
the compose name as the image default so compose behaviour is unchanged.

**The posture is demo-grade and says so (ADR-0011).** A Secret is
base64-encoded, *not* encrypted, and these values are committed to this
repository. That is honest for a teaching stack whose passwords are
`app`/`analytics`/`reports`, and it is not how a real system stores
credentials -- a real one sources them from Vault, a cloud KMS, the Secrets
Store CSI driver or External Secrets. Separating them from the ConfigMap is
what makes that later swap a change to *one object* rather than an audit of
every manifest.

Addresses that reach the observability stack are namespace-qualified --
analytics' `GRAFANA_URL`, loadgen's `K6_PROMETHEUS_RW_SERVER_URL` -- because
that stack is a separate release in its own namespace and a bare Service name
resolves only within the caller's.

**A ConfigMap or Secret edit does not restart the pods reading it.** With
`envFrom` the container reads its environment once, at start, so an upgrade
that changes only configuration reports success, leaves the pods running, and
the new value never takes effect. The library stamps a `checksum/config`
annotation built from each service's config, secret and config files onto the
pod template, so changing configuration changes the template and Kubernetes
rolls the pods.

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

## Run it on Kind

```sh
make kind-up                      # cluster + Envoy Gateway + kube-prometheus-stack
make kind-deploy                  # build, load, install -- the core profile
make kind-deploy PROFILE=full     # or any profile from the table above
make kind-seed                    # items + analytics history (dashboards need data)
make kind-down                    # delete the cluster, data included
```

`kind-up` is the cluster and rarely changes; `kind-deploy` is the application
and changes constantly. They are separate targets so a redeploy never rebuilds
the world.

### The three Kind-isms

Kind is not a cloud, and the places where that shows are called out rather
than hidden:

1. **No registry.** The charts pin `tag: dev` with `imagePullPolicy:
   IfNotPresent`, so an image that was never loaded is an `ErrImagePull` the
   kubelet cannot resolve. `kind-deploy` builds each service and runs
   `kind load docker-image` to copy it into the node containers. A real
   cluster pulls from a registry instead.
2. **No LoadBalancer.** Envoy Gateway creates one Service per Gateway and
   defaults it to `type: LoadBalancer`, which on Kind stays `<pending>`
   forever. `deploy/k8s/kind/envoyproxy.yaml` pins it to a NodePort on a fixed
   port, and `cluster.yaml` maps that port to `localhost:8080`. This object is
   applied by `kind-deploy` and referenced through a value that is only set on
   Kind -- the charts themselves render standard Gateway API objects and stay
   portable.
3. **Node-local volumes.** The local-path provisioner binds a ReadWriteOnce
   claim to whichever node the pod first landed on, so a Deployment holding
   one uses `strategy: Recreate` (`reports`, `loki`). The default rolling
   update would deadlock: the surge pod is scheduled before the old one
   releases the volume, and on any other node it never attaches.

### Reaching the services

Routing is **by hostname**, not by path prefix (ADR-0016), so every service
keeps its own origin and nothing has to be rewritten to live under a
sub-path. Everything goes through the Gateway on `localhost:8080`:

| URL | Service |
| --- | --- |
| `http://frontend.devops-demo.localhost:8080/` | frontend (nginx SPA) |
| `http://reports-ui.devops-demo.localhost:8080/` | reports-ui (Caddy SPA, `reports-ui` profile) |
| `http://api.devops-demo.localhost:8080/` | backend REST |
| `grpc.devops-demo.localhost:8080` | backend gRPC (GRPCRoute) |
| `http://grafana.devops-demo.localhost:8080/` | Grafana (admin/admin) |

No `/etc/hosts` editing: RFC 6761 reserves the whole `.localhost` tree for
loopback, and macOS and systemd-resolved both resolve `*.localhost` to
127.0.0.1 out of the box. On a resolver that does not (some glibc setups
without systemd-resolved), add one line:

```text
127.0.0.1 frontend.devops-demo.localhost reports-ui.devops-demo.localhost api.devops-demo.localhost grpc.devops-demo.localhost
```

Grafana's route is the one that crosses a namespace: it belongs to the
kube-prometheus-stack release, so it needs a ReferenceGrant (below).

### Migrations run themselves

Under compose, `make up` leaves the backend database unmigrated until someone
remembers to run `make seed`. On Kubernetes the deploy owns that step: the
backend chart ships a **Helm hook Job** that runs `alembic upgrade head` on
every install and upgrade (`charts/backend/templates/migration-job.yaml`).

It is a `post-install,post-upgrade` hook rather than a `pre-*` one because on
a first install there is no Postgres yet when the pre-hooks run, while
`--wait` has the StatefulSet ready before the post-hooks. A successful Job
deletes itself; a **failed one is kept** so there is something to read:

```sh
kubectl -n devops-demo logs job/platform-backend-migrate
```

Seed *data* is still a manual step -- the equivalent of `make seed --count 20`
has no Kubernetes counterpart yet.

### Observability, the Operator way

The stack -- Prometheus Operator, Prometheus, Alertmanager, Grafana,
kube-state-metrics, node-exporter -- is installed by `make kind-up` into the
`monitoring` namespace. The platform contributes the custom resources that say
WHAT to watch:

| Compose | Kubernetes | Owner |
| --- | --- | --- |
| a `scrape_configs` job per service | `ServiceMonitor` | the service's own chart |
| `rule_files` | `PrometheusRule` | the umbrella |
| the `blackbox_http` job + `relabel_configs` | `Probe` | the umbrella |
| a mounted dashboards directory | labelled `ConfigMap` | the umbrella |
| an Alloy container reading the Docker socket | a `DaemonSet` per node | its own chart |

That is the whole difference. Under compose these are sections of one
`prometheus.yml` that Prometheus must restart to reload; here each is an
object owned by the thing it describes, added by creating a resource. The
static scrape jobs are not ported at all -- their equivalent is the
ServiceMonitor each D6 chart already renders.

**The selector trap.** kube-prometheus-stack defaults every selector to "only
objects labelled `release=<my release>`". Our CRs come from a different
release, so Prometheus comes up perfectly healthy and silently scrapes
nothing of ours. The four `*SelectorNilUsesHelmValues: false` lines in
`kind/kube-prometheus-stack-values.yaml` are what make it select cluster-wide.
A green Prometheus with no targets is the failure mode to recognise.

**Two Kind-isms in that file.** The control plane runs as static pods on
127.0.0.1, so the kube-controller-manager, kube-scheduler, kube-proxy and etcd
scrapes can never succeed -- they and their alert rules are off, otherwise a
fresh cluster alerts on its own unreachable internals. And Grafana is pinned
to the image compose runs, with plugin preinstallation disabled and a
startupProbe: the chart's newer default spends over a minute building a search
index at boot, against probes with a 1s timeout and no startup gate, so the
kubelet kills it mid-build and it restarts into the same build.

**Logs.** Alloy runs one pod per node and reads pod logs through the
Kubernetes API -- no hostPath mount, no privilege, just declared RBAC. It is
scoped to its own node with a field selector on `spec.nodeName` from the
downward API; without that every collector tails every pod and each line
ships once per node.

**The cross-namespace route.** Grafana belongs to the stack's release, so the
platform's HTTPRoute for it points into another namespace. Gateway API refuses
that unless the target namespace publishes a `ReferenceGrant`
(`kind/referencegrant.yaml`) -- consent given by the owner of the referenced
thing rather than taken by the referrer, the deliberate difference from
ingress-style routing. A missing grant shows up as `ResolvedRefs=False` on the
route, not as a silent failure.

### When a deploy fails halfway

`kind-deploy` installs with `--wait`, so a pod that never becomes ready fails
the release and leaves it in `pending-install`. The next run then reports
`another operation (install/upgrade/rollback) is in progress`. Clear it before
retrying:

```sh
helm -n devops-demo uninstall platform
kubectl -n devops-demo get pods          # and look at what did not start
```

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
`kubeconform` at the vendored CRD schemas -- the Prometheus Operator
**ServiceMonitor** and the Gateway API **Gateway**, **HTTPRoute** and
**GRPCRoute** -- so those CRs are **actually validated**. Without it a CR is an
*error* (`could not find schema`), not silently skipped, which is exactly why
this gate does **not** use `--ignore-missing-schemas` (the DK9 trap: a green
gate that validates nothing).

### Refreshing the vendored schemas

Each file is the `openAPIV3Schema` of one CRD version -- the whole resource
(`apiVersion`, `kind`, `metadata`, `spec`), which is what `kubeconform`
validates a manifest against.

```sh
# ServiceMonitor, from a prometheus-operator release
TAG=v0.93.1
curl -fsSL "https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/${TAG}/example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml" \
  | yq -o=json '.spec.versions[] | select(.name == "v1") | .schema.openAPIV3Schema' \
  > deploy/k8s/schemas/monitoring.coreos.com/servicemonitor_v1.json

# Gateway API, from a gateway-api release (standard channel)
GWAPI=v1.6.1
curl -fsSL -o /tmp/gwapi.yaml \
  "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GWAPI}/standard-install.yaml"
for pair in gateways:gateway httproutes:httproute grpcroutes:grpcroute; do
  crd="${pair%%:*}"; out="${pair##*:}"
  yq -o=json "select(.kind == \"CustomResourceDefinition\" and .metadata.name == \"${crd}.gateway.networking.k8s.io\") | .spec.versions[] | select(.name == \"v1\") | .schema.openAPIV3Schema" \
    /tmp/gwapi.yaml > "deploy/k8s/schemas/gateway.networking.k8s.io/${out}_v1.json"
done
```

The Gateway API version to match is the bundle Envoy Gateway ships, since that
is what actually gets installed:

```sh
helm template eg oci://docker.io/envoyproxy/gateway-helm --version 1.9.0 --include-crds \
  | grep -o 'gateway.networking.k8s.io/bundle-version: v[0-9.]*' | sort -u
```

**The stack version and these schemas move together.** `kind-up.sh` pins
kube-prometheus-stack `88.5.4`, whose appVersion is operator `v0.93.1`, which
is the tag the files above are cut from. Bump the chart and re-cut the
schemas in the same change, or the gate validates CRs against a shape the
cluster no longer has.

## The toolchain (DK9)

`.mise.toml` pins all four Kubernetes tools: `helm`, `kubeconform`, `kind`,
`kubectl`. `kubectl` is the only one with a second version consumer -- the
Kind node image in `kind/cluster.yaml` *is* a Kubernetes version, and kubectl
tolerates only one minor of skew from the API server -- so
`scripts/check-toolchain-drift.sh` compares the two and refuses a node-image
bump that outruns the client. Bump the pin and the image together.
