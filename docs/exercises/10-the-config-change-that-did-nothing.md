# Exercise: The config change that did nothing (Kubernetes)

You change a value. You deploy. Helm says the release succeeded. Every pod
stays Ready. And the old value is still in effect.

This is the most dangerous failure shape in this repository, because it is
indistinguishable from success at every layer that reports anything.

## Objective

Show that editing a ConfigMap does not restart the pods consuming it, that a
`helm upgrade` changing only configuration therefore has no effect, and that
the fix is to make the change visible where Kubernetes actually looks: the pod
template.

## Prerequisites

```shell
make kind-up
make kind-deploy PROFILE=full
```

## Steps

### 1. See what a pod is running with

```shell
kubectl -n devops-demo get cm platform-canary-config -o json | jq '.data'
kubectl -n devops-demo get pod -l app.kubernetes.io/name=canary \
  -o json | jq '.items[0].spec.containers[0].envFrom'
```

The container takes its whole environment from that ConfigMap -- once, at
start.

### 2. Change the ConfigMap directly, as an operator would

```shell
kubectl -n devops-demo patch cm platform-canary-config \
  --type merge -p '{"data":{"CANARY_BACKEND_URL":"http://not-a-real-service:8000"}}'
```

Point it at a service that does not exist. Then wait, and watch nothing
happen:

```shell
kubectl -n devops-demo get cm platform-canary-config -o jsonpath='{.data.CANARY_BACKEND_URL}{"\n"}'
kubectl -n devops-demo get pods -l app.kubernetes.io/name=canary
```

The ConfigMap holds the new value. The pod has not restarted, its age is
unchanged, and it is still succeeding against the *old* address. Check the
canary's own metrics -- journeys are still passing.

**The cluster is in a state where the declared configuration and the running
configuration disagree, and nothing anywhere reports it.**

### 3. Prove the pod never re-read it

```shell
kubectl -n devops-demo exec deploy/platform-canary -- env 2>/dev/null | grep CANARY_BACKEND_URL
```

Still the old value. `envFrom` is resolved when the container starts; the
ConfigMap is not a live view.

### 4. Make the change visible

Roll it by hand and watch the new value take effect -- and the canary start
failing, because now it really is pointed at nothing:

```shell
kubectl -n devops-demo rollout restart deploy/platform-canary
kubectl -n devops-demo rollout status deploy/platform-canary
kubectl -n devops-demo exec deploy/platform-canary -- env 2>/dev/null | grep CANARY_BACKEND_URL
```

### 5. See what the chart does about it

```shell
kubectl -n devops-demo get deploy platform-canary \
  -o json | jq '.spec.template.metadata.annotations'
```

There is a `checksum/config` annotation: a hash of this service's config,
secret and config files, stamped on the **pod template**. Change any of them
through the chart and the template changes; a changed template is what makes
Kubernetes roll the pods.

Restore everything:

```shell
make kind-deploy PROFILE=full
```

## What to take away

- A ConfigMap is not a live view. `envFrom` and `env` are resolved at
  container start; mounted ConfigMap *files* do update in place, eventually,
  which makes the inconsistency worse rather than better -- two mechanisms,
  different behaviour, same object.
- Kubernetes rolls pods when the **pod template** changes. Nothing else about
  a referenced object triggers it. That is a deliberate design decision, not
  an oversight.
- Therefore a config-only `helm upgrade` is a silent no-op unless something
  puts the config *into* the template. A checksum annotation is the standard
  answer, and the library does it for every service so no chart can forget.
- Generalise it: **"the deploy succeeded" is a statement about the API server
  accepting objects, not about anything running differently.** Ask what
  observable behaviour should have changed, and check that instead.

## Going further

This one also shipped. During RFC-0003 PR-4, analytics was given the address
of the newly-deployed Grafana; the values were correct, `helm upgrade`
reported success, and the annotation write kept failing against the old
address. The checksum annotation exists because of that hour.

Two questions worth answering for yourself: which of this repository's gates
could have caught it, and what would you have to assert to catch the next one?
