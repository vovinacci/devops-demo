# Exercise: Call, then break, the gRPC contract (Phase 2)

RFC-0001 Phase 2 adds a contract-first gRPC surface between the backend
and (eventually) analytics: `devopsdemo.items.v1.ItemService` (ADR-0002).
This exercise explores the live contract with `grpcurl`, then deliberately
breaks it to watch `buf breaking` catch the mistake before it ever reaches
`main`.

## Objective

Call all three RPCs (`ListItems`, `GetItemStats`, `WatchItemEvents`)
against the running backend, watch a `WatchItemEvents` stream update in
real time as items are created and deleted, then make an incompatible
proto change and see CI's breaking-change gate reject it.

## Prerequisites

```shell
make up   # core profile: backend + full observability
```

`grpcurl` is optional (see [prerequisites.md](../prerequisites.md)); a
small Python script using the generated stubs works identically and is
shown as a fallback for every step below. Check what you have:

```shell
command -v grpcurl && echo "grpcurl available" || echo "using the Python fallback"
```

If you are using the Python fallback, generate the stubs first (they are
never committed -- RFC-0001 D8, ADR-0002):

```shell
make generate-backend
```

## Part 1: the live contract

1. List the service and its RPCs (server reflection is not enabled, so
   pass the `.proto` sources directly):

   ```shell
   grpcurl -plaintext -import-path proto -proto devopsdemo/items/v1/items.proto \
     localhost:50051 list devopsdemo.items.v1.ItemService
   ```

   Python fallback:

   ```shell
   python -c "
   import sys; sys.path.insert(0, 'services/backend/app/proto_gen')
   from devopsdemo.items.v1 import items_pb2_grpc
   print([m for m in dir(items_pb2_grpc.ItemServiceStub) if not m.startswith('_')])
   "
   ```

2. Call `ListItems` and `GetItemStats`:

   ```shell
   grpcurl -plaintext -import-path proto -proto devopsdemo/items/v1/items.proto \
     localhost:50051 devopsdemo.items.v1.ItemService/ListItems
   grpcurl -plaintext -import-path proto -proto devopsdemo/items/v1/items.proto \
     localhost:50051 devopsdemo.items.v1.ItemService/GetItemStats
   ```

   Confirm `total_items` matches `curl -sS http://localhost:8000/items | jq length`.

3. Check the gRPC Health Checking Protocol:

   ```shell
   grpcurl -plaintext -import-path proto -proto devopsdemo/items/v1/items.proto \
     -d '{"service": "devopsdemo.items.v1.ItemService"}' \
     localhost:50051 grpc.health.v1.Health/Check
   ```

4. Open a `WatchItemEvents` stream in one terminal:

   ```shell
   grpcurl -plaintext -import-path proto -proto devopsdemo/items/v1/items.proto \
     localhost:50051 devopsdemo.items.v1.ItemService/WatchItemEvents
   ```

   In another terminal (or the frontend UI at http://localhost:8080),
   create and then delete an item:

   ```shell
   curl -sS -X POST http://localhost:8000/items -H "Content-Type: application/json" \
     -d '{"name": "grpc-exercise-item"}' | tee /tmp/item.json
   ID=$(python3 -c "import json; print(json.load(open('/tmp/item.json'))['id'])")
   curl -sS -X DELETE "http://localhost:8000/items/$ID" -w '%{http_code}\n'
   ```

   Watch the stream terminal: you should see an `EVENT_TYPE_CREATED` event
   and then an `EVENT_TYPE_DELETED` event, each carrying the full item
   (including `name` on the delete event -- proto3 field-presence
   discipline, see `items.proto` comments). Confirm in Grafana/Prometheus
   that `grpc_server_stream_messages_total{grpc_method="WatchItemEvents"}`
   incremented by 2.

   Python fallback for step 4 is a short `asyncio` script; see
   `services/backend/tests/test_grpc.py::test_watch_item_events_created_then_deleted`
   for the exact call shapes (`stub.WatchItemEvents(...)`, then
   `await call.read()`).

## Part 2: break the contract on purpose

1. Make an incompatible edit to `proto/devopsdemo/items/v1/items.proto` --
   for example, rename `total_items` to `count` in `GetItemStatsResponse`,
   or change `Item.id` from `int64` to `int32`.

2. Run the same checks CI runs:

   ```shell
   make lint-proto
   cd proto && buf breaking --against '../.git#branch=main,subdir=proto'
   ```

   `buf breaking` should fail, naming the exact incompatible change.
   `proto/buf.yaml` sets `breaking.use: [FILE]`, so buf enforces
   *source/generated-code* compatibility, which is stricter than raw wire
   compatibility: a field rename (`total_items` -> `count`) keeps the same
   field number and is wire-compatible, and `int32` -> `int64` is a
   varint-compatible wire change -- but both break the generated API the
   consumer compiled against, so the FILE policy rejects them. That is the
   point: consumers depend on names and types, not just wire tags.

3. Revert the edit and confirm `buf breaking` passes again:

   ```shell
   git checkout -- proto/devopsdemo/items/v1/items.proto
   ```

## Expected observations

- `ListItems`/`GetItemStats` are unary pulls; nothing changes between
  calls unless the backend's data changes underneath them (ADR-0002's
  snapshot semantics -- exactly what an analytics client calls on
  reconnect to reconcile state).
- `WatchItemEvents` only shows events emitted **after** you opened the
  stream (emit-after-commit, at-most-once, RFC-0001 D3) -- items that
  already existed before you connected only appear via `ListItems`, never
  as a backfilled stream of historical events.
- `buf breaking` fails on wire-incompatible changes (renumbering,
  retyping, renaming a field that changes its wire tag) but would *not*
  fail on purely additive changes (a new field, a new RPC) -- try adding a
  harmless new field to confirm the gate is precise, not just strict.

## Cleanup

```shell
make down
```

Confirm no exercise item was left behind:

```shell
curl -sS http://localhost:8000/items | jq '[.[] | select(.name == "grpc-exercise-item")]'
```

## Discussion questions

1. `WatchItemEvents` never resends events emitted before you connected,
   and `ListItems` never tells you *when* an item was created. Why is
   pairing snapshot + stream (ADR-0002) still enough to build a correct
   analytics ingestion pipeline, given that limitation?
2. The backend runs a single uvicorn worker specifically because of the
   in-process event broadcaster (see `services/backend/README.md`). What
   would have to change -- architecturally, not just configuration -- to
   run the backend with multiple workers or replicas without losing
   events?
3. `buf breaking` compares against the `main` branch, not against the
   previous commit on your branch. Construct a sequence of two commits on
   the same branch where the *first* is a breaking change but
   `buf breaking` (as configured here) would not catch it until later.
