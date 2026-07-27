# Exercise: Round-trip a dashboard edit (operability)

Grafana here is provisioned from files: seven dashboards in
`observability/grafana/dashboards/`, read by the file provider on every start
and re-read every ten seconds. The provider also runs with
`allowUiUpdates: true`, so you can build a panel by clicking instead of
hand-writing JSON -- which is how dashboards are actually authored, and why the
setting is on.

The gap this exercise closes is what happens after you click Save. The edit
goes into Grafana's own database, inside a Docker volume; the repo never hears
about it, and nothing on screen tells you so. This exercise makes that
divergence visible, then closes the loop the way a dashboard change is supposed
to ship -- as a diff somebody can review.

## Objective

Edit a panel in Grafana, watch the save succeed, and prove two things the UI
does not tell you: the repo is unchanged, and the edit does not survive a
restart. Then make the same change properly and watch it survive.

## Prerequisites

```shell
make up
export GRAFANA_AUTH="admin:admin"   # the local default, per the README table
```

Confirm the seven dashboards provisioned, each with the uid its file pins:

```shell
curl -sS -u "$GRAFANA_AUTH" 'http://localhost:3000/api/search?type=dash-db' \
  | jq -r '.[] | "\(.uid) \(.title)"' | sort
```

```text
analytics-history Analytics History
analytics-ingest Analytics Ingest
devops-demo DevOps Demo
load-k6 Load (k6)
monitoring-layers Monitoring Layers
reports-jvm Reports JVM
reports-ui Reports UI (Caddy)
```

Those uids come from the files, which is why every dashboard has a readable,
stable URL: http://localhost:3000/d/devops-demo/.

## Part 1 -- save in the UI, and look for the warning that never comes

First, ask Grafana what it thinks this dashboard is, *before* touching it:

```shell
curl -sS -u "$GRAFANA_AUTH" http://localhost:3000/api/dashboards/uid/devops-demo \
  | jq '{provisioned: .meta.provisioned, canSave: .meta.canSave, file: .meta.provisionedExternalId}'
```

```json
{
  "provisioned": false,
  "canSave": true,
  "file": "devops-demo-dashboard.json"
}
```

Read that carefully. Grafana knows exactly which file the dashboard came from,
and still reports it as not provisioned and freely saveable. That combination is
what `allowUiUpdates: true` buys: editing behaves like editing anything else,
and nothing on the way in warns you.

Now open http://localhost:3000/d/devops-demo/ (admin/admin). On the
**SLO: Availability (99.95% target)** panel choose **Edit panel**, change the
title to `SLO: Availability (EDITED IN UI)`, go back, then **Save dashboard**.

Grafana confirms the save. Ask the repo about it:

```shell
git status --short
grep -c "EDITED IN UI" observability/grafana/dashboards/devops-demo-dashboard.json
```

`git status` prints nothing and the count is `0`. The dashboard you are looking
at exists in no file.

Ask Grafana the same question as before:

```shell
curl -sS -u "$GRAFANA_AUTH" http://localhost:3000/api/dashboards/uid/devops-demo \
  | jq '{provisioned: .meta.provisioned, canSave: .meta.canSave, file: .meta.provisionedExternalId}'
```

```json
{
  "provisioned": false,
  "canSave": true,
  "file": ""
}
```

The file is gone from the answer. Saving did not just write your change
somewhere else -- it cut the dashboard's link to the file it was provisioned
from. Grafana no longer associates what you are looking at with anything in the
repo, and it told you this only if you went looking.

## Part 2 -- restart, and lose it

```shell
make down
make up
```

Wait for Grafana, then look at the panel title again:

```shell
curl -sS -u "$GRAFANA_AUTH" http://localhost:3000/api/dashboards/uid/devops-demo \
  | jq -r '.dashboard.panels[0].title'
```

```text
SLO: Availability (99.95% target)
```

The edit is gone. It was never lost loudly -- no error, no conflict, no prompt.
The file provider simply reclaimed the dashboard it owns, and the file won.

This is the correct outcome, and it is worth being precise about why. The
dashboard file pins `"uid": "devops-demo"`, so the provider recognises the
dashboard as the one it provisioned and overwrites it. Without a pinned uid the
provider would not recognise it, and you would end up with two dashboards named
"DevOps Demo" instead -- your edited one, orphaned, beside a freshly provisioned
copy.

## Part 3 -- close the loop

Make the same change where it belongs. In
`observability/grafana/dashboards/devops-demo-dashboard.json`, change the first
panel's title to `SLO: Availability (EDITED IN UI)`:

```shell
grep -n '"SLO: Availability (99.95% target)"' \
  observability/grafana/dashboards/devops-demo-dashboard.json
```

Edit that line, then look at what you produced:

```shell
git diff --stat observability/grafana/dashboards/devops-demo-dashboard.json
```

One line changed. That diff is the entire point: a dashboard change is now a
reviewable change, like any other.

The provider re-reads every ten seconds, so you do not even need a restart --
but restart anyway, to prove the edit survives the thing that ate the last one:

```shell
make down
make up
curl -sS -u "$GRAFANA_AUTH" http://localhost:3000/api/dashboards/uid/devops-demo \
  | jq -r '.dashboard.panels[0].title'
```

```text
SLO: Availability (EDITED IN UI)
```

For a one-line change, editing the file directly is the honest round trip. For
a dashboard you actually built by clicking, use **Export** -> **Export as JSON**
-> **Save JSON to file** and write the result over the matching file. Note that
Grafana's export is pretty-printed while these files are stored compactly, so
the first such export reformats the whole file and produces a large diff;
after that, exports diff cleanly against each other.

## Expected observations

| Probe | After the UI save | After `make down` + `make up` |
| ----- | ----------------- | ----------------------------- |
| Grafana panel title | your edited title | the file's title |
| `git status --short` | empty | empty |
| dashboard count | 7 | 7 |
| `.meta.provisioned` | `false` | `false` |
| `.meta.provisionedExternalId` | `""` | `devops-demo-dashboard.json` |

The `git status` row is the one that matters. Every check a careful person would
run after saving -- the dashboard renders, the panel is right, nothing errors --
passes, and the change still does not exist anywhere durable.

The last row is the mechanism underneath it. Saving detached the dashboard from
its file; restarting let the provider find it again by uid and overwrite it. On
a dashboard whose file pinned no uid, the provider would not have found it, and
you would be looking at two dashboards named "DevOps Demo" instead of one.

## Cleanup

```shell
git checkout -- observability/grafana/dashboards/devops-demo-dashboard.json
make down
```

## Discussion questions

1. The file provider could instead run with `allowUiUpdates: false`, which makes
   Grafana refuse the save and show the dashboard as read-only. That trades a
   silent divergence for an upfront refusal. Which is the better default for
   this repo, which for a production instance, and what does each cost the
   person trying to build a panel?
2. `make seed-history` writes its anomaly annotations through Grafana's HTTP API
   (`POST /api/annotations`, `services/analytics/internal/grafana/client.go`), so
   they exist only in Grafana's database and have no file anywhere in this repo.
   If the dashboards' source of truth is the repo, what is the Grafana volume
   the source of truth for -- and what would you have to do to survive
   `make clean` with the seeded anomalies of
   [exercise 05](05-find-the-seeded-anomalies.md) intact?
3. Suppose you start a new dashboard by copying an existing file and forget to
   change its `uid`. Predict what Grafana does with two files claiming one uid,
   then check yourself against `scripts/check-dashboard-uids.sh`. Why is a gate
   a better answer here than a sentence in the documentation?
