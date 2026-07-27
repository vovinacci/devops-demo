# Exercise: What the flat network reaches (hardening)

Two compose defaults decide who can talk to your databases, and neither
announces itself. `"5432:5432"` publishes a port on **every** interface, not
just loopback. A single shared network puts every container one DNS lookup
away from every other one, including three databases that have no business
knowing about each other.

This stack ships with both defaults corrected. This exercise undoes each one
on purpose -- the same way exercise 03 breaks the event stream on purpose --
watches what becomes reachable, and puts it back. The lesson is not "add
`127.0.0.1`"; it is that the insecure version looks and behaves *identically*
from the application's point of view, which is exactly why it survives to
production.

## Objective

Show, with a connection you can watch succeed and then fail, that (a) a
published port defaults to all interfaces, and (b) a flat compose network lets
any compromised container read any other tier's data. Then confirm that
segmenting the networks costs the application nothing -- every legitimate path
still works.

## Prerequisites

```shell
make up-full
make seed        # 20 rows in the core DB, so a cross-tier read returns real data
```

Confirm the hardened baseline before breaking it:

```shell
docker ps --filter "name=devops-demo" --format '{{.Names}}\t{{.Ports}}' \
  | grep -E 'db|postgres'
```

All three databases should appear, each bound to loopback:

```text
devops-demo-db-1                  127.0.0.1:5432->5432/tcp
devops-demo-postgres-analytics-1  127.0.0.1:5433->5432/tcp
devops-demo-postgres-reports-1    127.0.0.1:5434->5432/tcp
```

The host port and container port differ for two of them -- they are all
Postgres on `5432` inside, remapped on the host to avoid collisions. What
matters is the `127.0.0.1:` prefix. If you see `0.0.0.0`, you are on an older
revision, and the rest of this exercise assumes the hardened baseline.

## Part 1 -- who can reach a published port

Find your machine's LAN address (not loopback):

```shell
ipconfig getifaddr en0            # macOS
hostname -I | awk '{print $1}'    # Linux
```

Call the API on loopback, then on that LAN address:

```shell
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8000/healthz
curl -sS -o /dev/null -w '%{http_code}\n' --max-time 5 http://<LAN-IP>:8000/healthz
```

The first returns `200`. The second fails to connect -- the port is bound to
loopback only.

Now undo the hardening for one service. In
`deploy/compose/docker-compose.yml`, find the `api` service and drop the
`127.0.0.1:` prefix from its first published port:

```yaml
    ports:
      - "8000:8000"          # was "127.0.0.1:8000:8000"
```

Apply it and repeat both calls:

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . up -d api
curl -sS -o /dev/null -w '%{http_code}\n' --max-time 5 http://<LAN-IP>:8000/healthz
```

The LAN call now returns `200`. Nothing in the application changed. Anyone
who can route to your machine -- a coffee-shop network, a colleague on the
office VLAN, a container on the same host -- just gained the API.

**Restore the `127.0.0.1:` prefix and re-apply before continuing.**

## Part 2 -- what one compromised container can read

The stack runs three independent databases. Ask the analytics-tier database
for the *core* application's rows:

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . \
  exec -e PGPASSWORD=app postgres-analytics \
  psql -h db -U app -d appdb -c "select id, name from items limit 3;"
```

It fails, and note *how* it fails:

```text
psql: error: could not translate host name "db" to address: Name does not resolve
```

Not "connection refused" -- the name does not resolve at all. `db` lives on
`dbnet-core`; `postgres-analytics` is on `dbnet-analytics` and has no way to
even name it.

Now flatten the network again. Give `postgres-analytics` a foot in the core
data tier -- which is what the single shared network granted it implicitly:

```yaml
    networks:
      - dbnet-analytics
      - dbnet-core         # <- what the old flat network effectively gave it
```

(Adding `devnet` here would *not* reproduce the old behaviour: `db` is on
`dbnet-core` only, so it is not reachable from the mesh either. That is the
segmentation working -- the core database is not on the network that every
service shares.)

Re-apply and repeat the same query:

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . up -d postgres-analytics
docker compose -f deploy/compose/docker-compose.yml --project-directory . \
  exec -e PGPASSWORD=app postgres-analytics \
  psql -h db -U app -d appdb -c "select id, name from items limit 3;"
```

Three rows of core application data, read by a container that exists only to
store analytics. No exploit, no privilege escalation -- just the default
network and a password that the compose file hands out in plain text.

**Restore `postgres-analytics` to `dbnet-analytics` only, and re-apply.**

## Expected observations

| Probe | Flat / published everywhere | Segmented / loopback |
| ----- | --------------------------- | -------------------- |
| `curl http://<LAN-IP>:8000/healthz` | `200` | connection fails |
| `docker ps` port column | `0.0.0.0:5432->5432` and `[::]:5432->5432` | `127.0.0.1:5432->5432` |
| `postgres-analytics` -> `db` | returns core `items` rows | `Name does not resolve` |
| `postgres-reports` -> `postgres-analytics` | returns the analytics schema | `Name does not resolve` |
| `api` / `analytics` / `reports` `/readyz` | `200` | `200` (unchanged) |
| Prometheus `postgres` target | `up` | `up` (unchanged) |

The last two rows carry the argument. Segmentation is not a tradeoff here:
every legitimate path -- each service to its own database, and Prometheus to
`postgres_exporter` across two networks -- behaves exactly as before. The only
thing removed is reachability nobody was using.

Read the `networks:` blocks and sort the dual-homed services into two kinds.
Most are **owners**: `api`, `analytics`, and `reports` each sit on `devnet`
plus their own tier, because a service has to reach its own database. Two are
**non-owners**, reaching into a tier they do not own:

- `postgres_exporter` -- on `dbnet-core` to query `db`, on `devnet` so
  Prometheus can scrape it.
- `grafana` -- on `dbnet-analytics` to serve the Analytics History dashboard,
  connecting as the read-only `grafana_ro` role (SELECT on three tables).

That is the shape a real boundary takes. Not "nothing crosses", but "every
crossing is named, justified, and as narrow as the job allows".

## Cleanup

```shell
git diff deploy/compose/docker-compose.yml    # confirm both edits are reverted
make down
```

If `git diff` shows anything, you left the stack insecure -- revert it.

## Discussion questions

1. `docker ps` showed both `0.0.0.0:5432->5432` and `[::]:5432->5432` before
   the fix. What is the second one, and why would a firewall rule written
   only against the IPv4 address leave the port reachable anyway?
2. The cross-tier read in Part 2 needed a password, and the compose file
   supplies it (`POSTGRES_PASSWORD:-app`). Suppose the passwords were strong
   and unique per tier. Argue whether the flat network would then be
   acceptable -- and what an attacker who has read-only access to one
   container's environment or filesystem could still do.
3. `postgres_exporter` is the one service that crosses a tier boundary. (a)
   What would you have to change to remove even that crossing, and (b) is the
   result actually more secure, or has the exposure just moved somewhere less
   visible? Consider where the metrics have to end up either way.
4. Binding to `127.0.0.1` protects the host's other interfaces, but every
   container on `devnet` can still reach every published service by its
   compose DNS name. Which of the two changes in this exercise would have
   stopped the Part 2 data read on its own, and why does that make port
   binding the weaker of the two controls?
5. This stack runs on one machine, so a single loopback bind is enough. On
   Kubernetes (RFC-0003) there is no `127.0.0.1` to bind to and no compose
   network to segment. Which Kubernetes object plays the role of
   `dbnet-analytics`, and what is the equivalent mistake -- the thing that is
   on by default and quietly makes every pod reachable from every other?
   Then the sting: Kind's default CNI (`kindnetd`) does not enforce that
   object at all, so a correct policy applied there changes nothing you can
   observe. What does that imply about testing a security control on a
   platform whose defaults differ from production?
