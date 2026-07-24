# Exercise: Watch the GC sawtooth (Phase 6)

RFC-0001 Section 9 makes one student exercise per phase part of the
Definition of Done. Phase 6 adds the Kotlin/Spring Boot reports service --
the JVM showcase -- whose entire reason for existing is to make garbage
collection *visible* on a dashboard (D2). This exercise puts a student in
front of the `Reports JVM` dashboard, has them drive real report jobs, and
asks them to see the sawtooth and read the report-job meters that go with
it.

The contrast is the point: the Rust canary and Go analytics services next
door have no GC pauses to show. The JVM does, and that is neither a bug nor
a secret -- it is a property you operate around (heap sizing against a
container limit, readiness while the pool warms, pause time under load).

## Objective

Bring up the reports stack with a real backend to report against, submit
report jobs in each format, and on the `Reports JVM` dashboard:

1. see the heap **sawtooth** -- `Heap Used` climbing as a job allocates and
   dropping on each GC -- and the GC pause-rate/time panels rise with it;
2. read the **report-job row** -- jobs by status, duration p95, in-flight,
   artifact throughput -- and connect each to the jobs you just ran;
3. observe the **D10 degradation**: kill analytics, run a report, and watch
   the job still succeed with an "unavailable" analytics section.

## Prerequisites

The reports service needs the backend (source of truth for items) and,
optionally, analytics (enrichment). Bring up core + analytics + reports:

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . \
  --profile analytics --profile reports up -d --build
```

Seed a few items so the report has content:

```shell
make seed
```

Grafana is at http://localhost:3000 (admin/admin); open the **Reports JVM**
dashboard. Give the JVM ~40s to finish starting (its start-up time relative
to the Go/Rust services is itself part of the lesson).

## Part 1 -- drive report jobs and watch the sawtooth

Submit a report and poll it to completion:

```shell
# Submit an XLSX report; capture the job id from the Location header.
id=$(curl -s -D - -o /dev/null -X POST http://localhost:8083/reports \
  -H 'content-type: application/json' \
  -d '{"type":"items-summary","format":"xlsx"}' \
  | awk -F'/' 'tolower($0) ~ /^location:/ {print $3}' | tr -d '\r')
echo "job: $id"

# Poll until SUCCEEDED.
curl -s http://localhost:8083/reports/$id | jq .

# Download the artifact.
curl -s -o report.xlsx http://localhost:8083/reports/$id/download
```

Now generate load in a burst so the allocation is visible -- fire a batch of
jobs across all three formats:

```shell
for i in $(seq 1 30); do
  for fmt in xlsx pdf csv; do
    curl -s -o /dev/null -X POST http://localhost:8083/reports \
      -H 'content-type: application/json' \
      -d "{\"type\":\"items-summary\",\"format\":\"$fmt\"}"
  done
done
```

On the dashboard, watch:

- **JVM Heap: Used vs Committed** -- the `used` line should sawtooth: up
  while POI/OpenPDF allocate, down on each collection.
- **GC Pause Rate by Action** and **GC Time Spent by Action** -- both rise
  while the batch runs.
- **Report Jobs by Status** -- a `succeeded` line; **Report Job Duration
  p95**, **In-flight Report Jobs** (capped at `reports.job-concurrency`),
  and **Artifact Throughput** all light up.

## Part 2 -- the D10 degradation

Stop analytics and run one more report:

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . \
  stop analytics

id=$(curl -s -D - -o /dev/null -X POST http://localhost:8083/reports \
  -H 'content-type: application/json' \
  -d '{"type":"items-summary","format":"csv"}' \
  | awk -F'/' 'tolower($0) ~ /^location:/ {print $3}' | tr -d '\r')
sleep 2
curl -s http://localhost:8083/reports/$id | jq .status   # SUCCEEDED
curl -s http://localhost:8083/reports/$id/download        # CSV with an
                                                          # "unavailable" note
```

The job **succeeds** on backend data alone; the analytics section is marked
unavailable rather than failing the report (RFC-0001 D10, Hard rule 9).

Now stop the backend too and submit another report -- this one **fails**,
because the backend is the source of truth, not optional:

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . stop api
id=$(curl -sS -D - -o /dev/null -X POST http://localhost:8083/reports \
  -H 'content-type: application/json' \
  -d '{"type":"items-summary","format":"csv"}' \
  | awk -F'/' 'tolower($0) ~ /^location:/ {print $3}' | tr -d '\r')
# poll until terminal, then read the failure and the download status
curl -sS "http://localhost:8083/reports/${id}" | jq '{status, error}'   # FAILED + error
curl -sS -o /dev/null -w '%{http_code}\n' "http://localhost:8083/reports/${id}/download"  # 409
```

## Expected observations

- The **sawtooth** is generational GC doing its job: short-lived POI/PDF
  objects fill the young generation, a minor GC reclaims them, `used` drops.
  A monitoring component with no GC (the Rust canary) cannot show this --
  the adjacency of the two dashboards is the exhibit (RFC-0001 Section 7).
- **In-flight Report Jobs** plateaus at `reports.job-concurrency` (default
  2, `REPORTS_JOB_CONCURRENCY`), not at 90: the fixed dispatcher caps how
  many reports allocate at once, which is what keeps the allocation burst a
  *sawtooth* instead of an OOM.
- **`committed` barely moves while `used` sawtooths**: `used` is live heap
  right now; `committed` is what the JVM has reserved from the OS and grows
  only when sustained live-set pressure forces it -- a bursty allocate-then-
  collect workload rarely does.
- **Graceful degradation** distinguishes a *required* upstream (backend)
  from an *optional* one (analytics): the same job outcome (succeed vs fail)
  encodes which is which, and the report says so in its output rather than
  hiding the gap (engineering-principles.md: "a visible limitation over a
  hidden hack").

## Cleanup

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . \
  --profile analytics --profile reports down -v
```

## Discussion questions

1. `In-flight Report Jobs` plateaus at 2 rather than at the 90 you
   submitted, because the dispatcher is bounded on purpose
   (`reports.job-concurrency`). What does the heap panel look like if you
   raise `REPORTS_JOB_CONCURRENCY` far higher -- at what point does the
   *sawtooth* stop being a sawtooth, and what fails first, the dashboard's
   legibility or the container's memory limit?
2. The `committed` line barely moves while `used` sawtooths hard. Who
   decides `committed`, and how does the JVM's heap sizing interact with the
   reports container's memory limit? Sketch what you would watch (and what
   you would tune) to keep this service from being OOMKilled under a
   sustained, not bursty, report load.
3. Part 2 encodes "backend required, analytics optional" in the *job
   outcome* -- succeed-with-a-note vs fail -- and puts the limitation in the
   artifact rather than firing an alert. Contrast that with the alerting-
   side degradation in exercises 03 and 04. When is "make the gap visible in
   the output" the right call, and when would you want a metric or alert on
   the degraded-section rate instead?
