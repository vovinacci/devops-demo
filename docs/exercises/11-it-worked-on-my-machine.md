# Exercise: It worked on my machine (Kubernetes)

`make kind-deploy` works. It has worked for weeks. Then someone clones the
repository, runs the same command, and it fails immediately:

```text
Error: template: platform/charts/reports/templates/manifests.yaml:2:3:
executing "..." at <include "common.serviceaccount" .>:
error calling include: template: no template "common.serviceaccount"
associated with template "gotpl"
```

Nothing changed. The chart is identical. The difference is entirely in what
was already lying around on the working machine -- and this exercise
manufactures that difference on purpose so you can watch a passing command
start failing without a single file changing.

## Objective

Show that a build step can depend on **state left behind by a different
command**, that this dependency is invisible on any machine where the other
command has run, and that the only reliable test is a clean checkout -- which
is exactly what CI is.

## Prerequisites

A working repository. No cluster needed for steps 1 to 3.

## Steps

### 1. Look at the state nobody thinks about

The per-service charts import a shared library chart. Helm requires that
library to be *vendored* into each chart before it can be rendered:

```shell
ls deploy/k8s/charts/reports/charts/
cat deploy/k8s/charts/reports/Chart.yaml | grep -A3 dependencies
```

You will see `common-0.1.0.tgz` -- a build artifact. Now note two things:

```shell
cat deploy/k8s/.gitignore
git status --short deploy/k8s/charts/
```

It is **gitignored**, so it is not in a fresh clone; and it is absent from
`git status`, so nothing reminds you it exists.

### 2. Find out what put it there

```shell
grep -rn "dependency" deploy/k8s/scripts/*.sh
```

`validate.sh` -- which is `make lint-k8s` -- vendors every chart. That is the
command that has been quietly preparing the ground for every deploy you have
ever run.

### 3. Simulate a fresh clone, without cloning

Remove exactly what a clone would not have:

```shell
find deploy/k8s/charts -mindepth 2 -maxdepth 2 \
  \( -name charts -type d -o -name Chart.lock -type f \) -exec rm -rf {} +
```

Now render the umbrella the way a deploy does:

```shell
helm template platform deploy/k8s/charts/platform \
  -f deploy/k8s/charts/platform/values.yaml
```

It fails, on a repository you have not modified.

### 4. Watch the wrong fix work

```shell
make lint-k8s
helm template platform deploy/k8s/charts/platform -f deploy/k8s/charts/platform/values.yaml >/dev/null && echo OK
```

Green again. This is the trap: running the linter "fixes" the deploy, so the
obvious conclusion -- "you have to lint first" -- is both true and wrong. It
is true of the symptom and wrong about the cause, and it leaves the failure in
place for anyone who does not know the folklore.

### 5. Fix the cause

`kind-deploy.sh` now vendors every chart itself rather than assuming another
command did:

```shell
grep -A8 "dependency update" deploy/k8s/scripts/kind-deploy.sh
```

Note it *derives* the chart list from the directory instead of repeating it,
so a chart added tomorrow is covered without anyone remembering to update a
list. Prove it from a clean state again:

```shell
find deploy/k8s/charts -mindepth 2 -maxdepth 2 \
  \( -name charts -type d -o -name Chart.lock -type f \) -exec rm -rf {} +
make kind-up
make kind-deploy PROFILE=core
```

## What to take away

- **Gitignored build artifacts are invisible dependencies.** They do not show
  up in `git status`, they are absent from a clone, and any command that needs
  one without creating it works perfectly for everybody who happens to have it.
- A command that only works after some *other* command has run has an
  undeclared dependency. "Run `make lint-k8s` first" is folklore, not a build
  system.
- **A clean checkout is a test, and CI is the only place that runs it every
  time.** Your machine accumulates state; the CI runner does not. That is not
  a limitation of CI, it is the whole value of it.
- Derive lists rather than repeating them. The fix could have hardcoded the
  eleven chart names -- and would have been wrong the first time somebody
  added a twelfth.

## Going further

This one was found by the nightly Kind gate on its first honest run, on a
branch where the artifacts happened to have been cleared. It had been latent
since the deploy script was written: every local run passed, because
`make lint-k8s` runs constantly during development.

Two questions:

1. What else in this repository would fail on a clean checkout? Try
   `git clone` into a temporary directory and run `make ci` in it.
2. `Chart.lock` is also gitignored here. Look up what a lockfile is normally
   for, and work out why committing it would *not* have prevented this --
   and what it would have changed instead.
