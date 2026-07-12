# Troubleshooting

## First Step: Run make doctor

Before digging into specific symptoms, run:

```shell
make doctor
```

This checks tool presence, versions against `.mise.toml` pins, Docker daemon, Compose v2,
and available RAM/disk. It prints actionable fixes for most common problems.

---

## Toolchain Version Problems

**Symptoms:** Wrong Python or Node version; `mise` commands fail.

**Fix:** Install pinned versions via mise and activate in your shell.

```shell
mise install
```

Add to `~/.zshrc` if not already present:

```shell
eval "$(mise activate zsh)"
```

Then open a new shell. Version pins are in `.mise.toml` -- do not install specific
versions by hand.

See [docs/local-setup.md](./local-setup.md) for full setup steps.

---

## Docker Issues

### Docker daemon not running

**Symptoms:** `docker ps` fails, `make up` errors immediately.

**Fix:** Start Docker Desktop (macOS) or the Docker service (Linux), then retry.

### Docker Compose v1 used instead of v2

**Symptoms:** `docker-compose` command not found, or compose behavior differs.

**Fix:** This project requires Compose v2 (the `docker compose` plugin). Do not use
`docker-compose` (v1). Verify:

```shell
docker compose version
```

### Raw compose commands

Bare `docker compose <cmd>` from repo root does NOT pick up the correct file.
Use make targets:

```shell
make up        # start all services
make down      # stop all services
make logs      # tail API logs
make clean     # remove containers and volumes (deletes data)
```

For raw compose commands, specify the file explicitly:

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . <cmd>
```

### Services crash or won't start

```shell
make down
make up

# If still broken, full cleanup:
make clean
make up
```

### Volume or network conflicts

```shell
# Check volumes
docker volume ls

# Remove a specific volume (deletes data)
docker volume rm <volume_name>

# Check networks
docker network ls

# Remove a conflicting network
docker network rm <network_name>

make up
```

---

## Port Conflicts

**Ports used:**

| Service           | Port |
|-------------------|------|
| Frontend          | 8080 |
| API               | 8000 |
| Grafana           | 3000 |
| Prometheus        | 9090 |
| Loki              | 3100 |
| PostgreSQL        | 5432 |
| postgres-exporter | 9187 |
| cAdvisor          | 8081 |

**Symptoms:** "port already in use" or "address already in use".

**Find the occupying process:**

```shell
lsof -i :8000
lsof -i :8080
# etc.
```

**Fix:** Kill the conflicting process:

```shell
kill -9 <PID>
```

Or change the port in `deploy/compose/docker-compose.yml`.

---

## Database Connection Issues

**Symptoms:** API returns connection errors, health check fails, or
`services/backend` reports DB unavailable.

**Check DB status:**

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . ps db
docker compose -f deploy/compose/docker-compose.yml --project-directory . exec db pg_isready -U app -d appdb
docker compose -f deploy/compose/docker-compose.yml --project-directory . logs db
curl http://localhost:8000/health
```

**Check environment variables** (defaults work with `.env.example`):

```shell
echo $DATABASE_URL
echo $ALEMBIC_DATABASE_URL
```

Expected defaults: `postgresql+asyncpg://app:app@localhost:5432/appdb` and
`postgresql+psycopg://app:app@localhost:5432/appdb`. Override via `.env`; see `.env.example`.

**Test connection:**

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . exec db psql -U app -d appdb -c "SELECT 1;"
```

**Common causes:**

- DB container not running or still starting (give it a few seconds)
- Credentials mismatch between `.env` and DB init
- Network conflict between containers

---

## Migration Issues

**Symptoms:** Errors applying migrations, schema mismatch.

**Check state:**

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . exec api alembic current
docker compose -f deploy/compose/docker-compose.yml --project-directory . exec api alembic history
```

**Apply migrations:**

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . exec api alembic upgrade head
```

**Locally (venv active):**

```shell
cd services/backend
source .venv/bin/activate
alembic -c alembic.ini upgrade head
```

**Rollback:**

```shell
# Last migration
docker compose -f deploy/compose/docker-compose.yml --project-directory . exec api alembic downgrade -1

# Specific revision
docker compose -f deploy/compose/docker-compose.yml --project-directory . exec api alembic downgrade <revision>
```

---

## Backend Virtualenv Issues

**Symptoms:** `venv` creation or activation errors in `services/backend`.

**Recreate venv:**

```shell
rm -rf services/backend/.venv
make venv-install
```

**Verify activation:**

```shell
which python   # -> services/backend/.venv/bin/python
```

---

## Frontend node_modules Issues

**Symptoms:** Errors installing or running frontend in `services/frontend`.

**Reinstall dependencies:**

```shell
cd services/frontend
rm -rf node_modules package-lock.json
npm cache clean --force
npm install
```

**npm registry (corporate proxy):**

```shell
npm config get registry
npm config set registry https://registry.npmjs.org/
```

---

## Logs

```shell
make logs                  # API logs (make target)

# Or via compose directly:
docker compose -f deploy/compose/docker-compose.yml --project-directory . logs -f api
docker compose -f deploy/compose/docker-compose.yml --project-directory . logs -f db
docker compose -f deploy/compose/docker-compose.yml --project-directory . logs -f web
docker compose -f deploy/compose/docker-compose.yml --project-directory . logs -f
```

**Via Grafana (Loki):**

1. Open `http://localhost:3000`
2. Go to Explore
3. Select Loki datasource
4. Use LogQL queries for filtering

---

## Metrics / Dashboard Issues

**Symptoms:** Grafana shows empty dashboards; metrics missing.

```shell
curl http://localhost:9090/-/healthy
curl http://localhost:8000/metrics
docker compose -f deploy/compose/docker-compose.yml --project-directory . exec prometheus promtool check config /etc/prometheus/prometheus.yml
docker compose -f deploy/compose/docker-compose.yml --project-directory . restart grafana
```

---

## Still Stuck

- Run `make doctor` again after changes -- it catches most toolchain issues.
- Check full logs: `make logs` or per-service compose logs.
- See [docs/architecture.md](./architecture.md#additional-resources) for tool docs.
- See [docs/running-tests.md](./running-tests.md) for test-specific failures.
- Open an issue in the project repository with the problem description and relevant logs.
