# Exercise: The ServiceMonitor that scrapes nothing (Kubernetes)

Prometheus is green. Every target is up. Every pod is Ready. And the Reports
JVM dashboard is empty.

Nothing here is broken in a way any health check can see, which is the point.
This exercise breaks the same thing on purpose, watches a healthy-looking
cluster tell you nothing, and then finds it with the only tool that works:
asking what a query actually matched.

## Objective

Show that a metrics pipeline can be *structurally* correct -- CR created,
target discovered, scrape succeeding -- and still deliver nothing to the
dashboards that read it, because a label the operator generates does not
match the label the queries were written against.

## Prerequisites

The Kubernetes stack, with reports running:

```shell
make kind-up
make kind-deploy PROFILE=full
make kind-seed
```

Grafana is at `http://grafana.devops-demo.localhost:8080/` (admin/admin).

## Steps

### 1. Confirm it works

Open the **Reports JVM** dashboard. Panels have data.

Ask Prometheus what backs them:

```shell
kubectl -n monitoring port-forward svc/kube-prometheus-stack-prometheus 9090:9090 &
curl -s 'http://localhost:9090/api/v1/query?query=up%7Bjob%3D%22reports%22%7D' | jq '.data.result'
```

One result, value `1`. Note the `job` label: `reports`.

### 2. Break it, without breaking anything

Remove the one line that decides that label:

```shell
kubectl -n devops-demo patch servicemonitor platform-reports \
  --type json -p '[{"op":"remove","path":"/spec/jobLabel"}]'
```

Wait about thirty seconds for the operator to regenerate the scrape config.

### 3. Observe a healthy cluster with an empty dashboard

```shell
kubectl -n devops-demo get pods -l app.kubernetes.io/name=reports   # Running, 1/1
curl -s 'http://localhost:9090/api/v1/query?query=up%7Bjob%3D%22reports%22%7D' | jq '.data.result'
```

The pod is fine. The query returns `[]`.

Now ask what Prometheus *does* have:

```shell
curl -s 'http://localhost:9090/api/v1/query?query=up' | jq -r '[.data.result[].metric.job] | unique[]'
```

`platform-reports` is there. The service is being scraped, successfully, under
a different name -- the Service's name rather than the workload's.

Reload the Reports JVM dashboard: empty. Every panel queries `job="reports"`.

**Nothing reported an error.** Not the pod, not the ServiceMonitor, not
Prometheus, not Grafana. A dashboard with no data looks exactly like a service
with no traffic.

### 4. Put it back

```shell
kubectl -n devops-demo patch servicemonitor platform-reports \
  --type merge -p '{"spec":{"jobLabel":"app.kubernetes.io/name"}}'
```

Or redeploy: `make kind-deploy PROFILE=full`.

Within a minute the query matches again and the dashboard fills.

## What to take away

- `jobLabel` names a label **on the Service** whose *value* becomes the `job`
  label on every series scraped from it. Without it you get the Service's own
  name, which on Helm is `<release>-<chart>` and differs from anything a
  compose-era query was written against.
- This is why the charts set it in the library rather than per service: one
  place, every D6 service, portable queries across both stacks.
- The general shape: **a green pipeline proves delivery, not correctness.**
  Something can be scraped perfectly and still be unusable, because usefulness
  depends on agreement between producer and consumer that no health check
  models.
- The only reliable check is to ask what a query matched, not whether the
  parts are up. `count(up{job="..."})` is a better dashboard test than any
  number of Ready pods.

## Going further

This shipped for real. RFC-0003 PR-4 landed with no `jobLabel`, and the
Reports JVM (16 panels), Reports UI (12) and Analytics History (1) dashboards
were all silently empty on Kubernetes while every pod was Ready and Prometheus
was green. It was found by a reviewer reading the alert rule, not by anything
running. Ask yourself what in this repo would have caught it -- and what you
would add.
