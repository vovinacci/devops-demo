# Runbook: Kubernetes bring-up

## Meaning

How to stand the platform up on Kubernetes, what it costs, what breaks, and
how to tell the difference between a broken deploy and a working one that
looks broken. The compose stack is unaffected by everything here -- it stays
the fast local path (`deploy/compose/README.md`).

For what the charts contain and why, read `deploy/k8s/README.md`. This file is
for when you are running it.

## Bring it up

```shell
make kind-up                     # cluster + Envoy Gateway + kube-prometheus-stack
make kind-deploy PROFILE=full    # build, load into Kind, helm install
make kind-seed                   # items + analytics history
```

`make kind-smoke` does all of that, drives it with k6, asserts the wiring and
tears the cluster down -- it is what the nightly CI gate runs. Add
`KEEP_CLUSTER=1` to keep the cluster for inspection when it fails.

Profiles combine, exactly as compose's repeated `--profile` flags do:

```shell
make kind-deploy PROFILE="analytics load"
```

## What it costs

Measured across the three Kind nodes, with the observability stack running:

| Deployed | Memory |
| --- | --- |
| core + analytics + load | ~3.4 GiB |
| every profile (`full`) | ~4.2 GiB |

Compare with ~1.6 GiB for the whole compose stack. The difference is the
cluster itself: a control plane, three nodes, Envoy, and the Prometheus
Operator with Prometheus, Alertmanager, Grafana, kube-state-metrics and
node-exporter beside the services.

RFC-0003 Section 9 estimated 6-10 GB and treated it as the arc's principal
risk. Measurement puts it at roughly half that, which is why the nightly gate
runs every profile with the Operator rather than dropping either to fit.

If your machine is tight, deploy fewer profiles rather than fewer nodes --
`PROFILE=core` is about 2.4 GiB, and the multi-node topology is what makes
scheduling and volume affinity visible at all.

## The JVM exhibit (DK5)

`reports` is the largest single consumer, and deliberately so. Its container
memory *limit* does not bound the JVM heap by itself -- the limit bounds total
RSS, while heap, metaspace, thread stacks, code cache and direct buffers all
live inside it. The chart sets `JAVA_TOOL_OPTIONS: -XX:MaxRAMPercentage=75.0`
beside `resources.limits.memory` so the heap tracks the limit rather than the
node.

Watch it:

```shell
kubectl -n devops-demo top pod -l app.kubernetes.io/name=reports   # needs metrics-server
kubectl -n devops-demo logs -l app.kubernetes.io/name=reports | head -30
```

The Reports JVM dashboard in Grafana shows the heap sawtooth against the
limit. If the pod is OOMKilled, the limit is too low for the heap percentage,
not the other way round.

## Failures worth recognising

**Everything is Running and Prometheus has no targets.** The stack's selectors
default to "only objects labelled with my own release". Our ServiceMonitors
come from a different release. Fixed in
`deploy/k8s/kind/kube-prometheus-stack-values.yaml` by four
`*SelectorNilUsesHelmValues: false` lines -- if you fork those values, keep
them.

**A dashboard is empty but the service is up.** Check the job label:

```shell
kubectl -n devops-demo get servicemonitor platform-reports -o jsonpath='{.spec.jobLabel}'
```

Without `jobLabel: app.kubernetes.io/name`, Prometheus names the job after the
Service (`platform-reports`), and every query written against the compose
stack matches nothing while looking perfectly healthy.

**A config change deploys successfully and does nothing.** Editing a ConfigMap
or Secret does not restart the pods reading it; with `envFrom` the environment
is read once, at start. The library stamps a `checksum/config` annotation on
the pod template so a config change rolls the pods. If you add a chart that
bypasses the library, it will have this bug.

**`ErrImagePull` on a service you just changed.** Kind has no registry. The
image has to be loaded into the nodes:

```shell
kind load docker-image devops-demo/backend:dev --name devops-demo
```

`make kind-deploy` does this for you; a hand-rolled `helm upgrade` does not.

**`ImagePullBackOff` on a third-party image, with TLS handshake timeouts.**
That is Docker Hub being unreachable or rate-limiting, not a platform fault.
Retry; the pull is per-node, so a partial failure is normal.

**A rollout that never completes on `reports` or `loki`.** Both hold a
ReadWriteOnce claim, which attaches to one node at a time. They use
`strategy: Recreate` for that reason. A rolling update would schedule the new
pod before the old one released the volume, and it would never attach.

**`another operation (install/upgrade/rollback) is in progress`.** A previous
`helm --wait` failed and left the release pending. Clear it before retrying:

```shell
helm -n devops-demo uninstall platform
kubectl -n devops-demo get pods        # and look at what did not start
```

**reports-ui says the reports service is unavailable.** Check whether it is
telling the truth:

```shell
curl -s http://reports-ui.devops-demo.localhost:8080/metrics | grep upstreams_healthy
```

`0` means Caddy cannot reach its `/api` upstream. That gauge exists precisely
so a dead upstream cannot read as green (RFC-0002 D10) -- it has been right
and the address wrong before now.

## Tear down

```shell
make kind-down     # delete the cluster; PVC data goes with it
make clean-k8s     # the above, plus the Helm build artifacts
```

`kind-down` is a factory reset, not a restart: the local-path provisioner
keeps volume data inside the node containers, so nothing survives to reattach.

## Also see

- [`deploy/k8s/README.md`](../../deploy/k8s/README.md) -- the charts, the
  Gateway, the Operator model, the offline gate
- [Observability](../observability.md) -- what the dashboards show
- [Exercise 09](../exercises/09-the-servicemonitor-that-scrapes-nothing.md)
  and [Exercise 10](../exercises/10-the-config-change-that-did-nothing.md)
