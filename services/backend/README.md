# Backend

FastAPI + SQLAlchemy (async) REST API, the source of truth for `Item` rows.
Also serves the cross-service gRPC contract (RFC-0001 D3, ADR-0002):
backend serves, analytics dials -- this process never makes outbound gRPC
connections and has no awareness of connected consumers.

## REST (`:8000`)

- `/healthz` -- liveness, always `200` once the process is up.
- `/readyz` -- readiness; `503` if the database is unreachable.
- `/items` -- `GET` (list), `POST` (create).
- `/items/{item_id}` -- `DELETE`.
- `/metrics` -- Prometheus exposition format (HTTP + gRPC metrics both
  ride this one endpoint; nothing new to scrape for gRPC).

## gRPC (`:50051`)

`devopsdemo.items.v1.ItemService` (`proto/devopsdemo/items/v1/items.proto`),
served via `grpc.aio` in the same process as FastAPI, started/stopped from
the FastAPI lifespan (`app.grpc_server`, `app.main`).

Plaintext, no auth -- by design, not an oversight: this is a local compose
network only (RFC-0001 non-goal; TLS/auth for cross-network gRPC is out of
scope for this phase).

- `ListItems` / `GetItemStats` -- unary, bulk/backfill pull.
- `WatchItemEvents` -- server-streaming: `created` / `deleted` events
  (`updated` is in the contract ahead of any producer -- no PUT endpoint
  exists yet).

### Event semantics: emit-after-commit, at-most-once

`app.events.ItemEventBroadcaster` fans out item events to zero or more
`WatchItemEvents` streams, entirely in-process:

- Events are published only **after** the DB commit succeeds (`app.crud`),
  never before.
- Delivery is **at-most-once, no durability**. If nobody is subscribed,
  the event is simply dropped -- the `core` compose profile runs the gRPC
  server with zero clients at no cost (no buffering, no queueing).
- Each subscriber owns a bounded queue (64 events). `publish()` is
  synchronous and never blocks -- it runs on the HTTP request path right
  after a commit, so a slow gRPC consumer must never hold up a write. A
  subscriber whose queue fills up is disconnected (its queue receives a
  sentinel, ending that `WatchItemEvents` stream) rather than the event
  being awaited into place. The analytics client is expected to reconnect
  and reconcile state via `ListItems` (ADR-0002's snapshot-then-stream
  sequence) -- the same recovery path as a network disconnect.
- Known gap, accepted by design: a committed item whose event was never
  emitted (crash between commit and publish), and events emitted while a
  consumer is disconnected or too slow, are unrecoverable. The
  transactional-outbox fix is out of scope for this phase (RFC-0001
  Section 10, NATS capstone).

### Health

The gRPC Health Checking Protocol (`grpc_health.v1`) reports `SERVING` for
both the overall server and `devopsdemo.items.v1.ItemService` as soon as
the process is up -- kept simple, not wired to DB state the way HTTP
`/readyz` is. The container healthcheck (`app.healthcheck`, `python -m
app.healthcheck`) checks HTTP `/healthz` **and** the gRPC health service
together; both must pass.

### Metrics (RED)

| Metric | Type | Labels | Meaning |
| ------ | ---- | ------ | ------- |
| `grpc_server_handled_total` | counter | `grpc_method`, `grpc_code` | Calls completed, by method and status |
| `grpc_server_handling_seconds` | histogram | `grpc_method` | Call duration; for `WatchItemEvents` this is the whole subscribe-to-disconnect stream lifetime, not per-message |
| `grpc_server_stream_messages_total` | counter | `grpc_method` | Individual messages pushed on a streaming RPC |

`grpc_method` is one of the contract's fixed RPC names (bounded
cardinality by construction).

## Tracing (RFC-0001 D11, ADR-0010)

Both the HTTP API and the gRPC server are instrumented with the
OpenTelemetry SDK (`app/otel.py`): every request gets a real span with a
real, random W3C trace ID -- `opentelemetry-instrumentation-fastapi`
extracts an incoming `traceparent` HTTP header, and a gRPC server
interceptor (`opentelemetry-instrumentation-grpc`'s `aio_server_interceptor`,
composed explicitly with the RED metrics interceptor rather than via that
package's monkeypatch-based `GrpcAioInstrumentorServer`) does the same for
gRPC metadata. The canary already sends `traceparent` on every journey
call, so a canary trace ID and the backend log lines for that same
request now match end to end.

No exporter is configured -- the trace **backend** (Tempo) is deferred
(Section 10 of RFC-0001), same as the canary. Spans are still created and
recorded, just never shipped anywhere.

Until Tempo lands, log-trace correlation is manual: every backend log
line carries `[trace_id=<32 hex chars>]` (`app.otel.TraceIdFilter`,
installed on the root logger), all zeros when the line wasn't logged
inside a span (e.g. the excluded routes below). In Loki, once shipped:

```logql
{container="devops-demo-api-1"} |= "trace_id=b4f151ede3df15ebd770309cc1146b98"
```

`/healthz`, `/readyz`, `/health`, and `/metrics` are excluded from span
creation (`excluded_urls`) -- healthcheck and scrape traffic has no
diagnostic value and would dominate whatever Tempo eventually samples.
Their own log lines still exist (the metrics/logging middleware isn't
excluded), just with a zero trace_id.

Logs are still the plain `basicConfig` text format, not JSON -- converting
to structured JSON logs is separate debt (D6), not done in this PR.

## Generated code (`app/proto_gen/`)

Never committed (gitignored, RFC-0001 D8 / ADR-0002 / AGENTS.md Hard rule
1). Regenerate with:

```shell
make generate-backend   # from the repo root
```

This runs a single `python -m grpc_tools.protoc` invocation producing
message classes, `.pyi` stubs, and the gRPC service stubs together;
`grpcio-tools` bundles its own protoc, so nothing else needs to be
installed. The Dockerfile's build stage runs the identical command before
installing runtime dependencies, so the shipped image contains generated
code that the repository itself never does.

Import path: `app.grpc_server` adds `app/proto_gen` itself to `sys.path`
(not `app.proto_gen.devopsdemo...`) before importing `from
devopsdemo.items.v1 import items_pb2, items_pb2_grpc`. `grpcio-tools` has
no option to nest its generated absolute import
(`from devopsdemo.items.v1 import items_pb2`, hard-coded by the plugin)
under `app`; importing the same file under two different module paths
would register its descriptor twice and fail with a duplicate-file
descriptor-pool error. Making `proto_gen` the `sys.path` entry keeps
exactly one import path for generated code.

## Single-worker constraint

The container runs uvicorn with `--workers 1`. The event broadcaster
above is per-process state, and so is the `grpc.aio` server it feeds --
with more than one worker, each forked process would run its own
broadcaster and its own gRPC server sharing `:50051` via `SO_REUSEPORT`. A
`WatchItemEvents` subscriber would then only see commits made by whichever
worker it happened to land on, silently dropping most events instead of
the documented at-most-once-when-disconnected gap.

## Development

```shell
make venv-install        # from the repo root: creates services/backend/.venv
source services/backend/.venv/bin/activate
make generate-backend     # from the repo root: regenerate gRPC stubs
make test-backend         # from the repo root
make lint-backend type-check
```

## Docker

Multi-stage build. The builder stage reaches the repo-root `proto/`
module as a named additional build context (`proto`), not by widening
this service's own build context (`services/backend`) -- `docker build
--build-context proto=proto services/backend`, or via compose's
`additional_contexts` (`deploy/compose/docker-compose.yml`). Dev
dependencies (including `grpcio-tools`) are installed only to generate
code in the builder stage and never reach the runtime image.
