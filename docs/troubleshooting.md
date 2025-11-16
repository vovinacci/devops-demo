# Troubleshooting

## Service Restart

If services are not working correctly, try a full restart:

```shell
# Stop and remove containers
make down

# Start again
make up

# Or full restart with volume cleanup (careful - will delete data!)
make clean
make up
```

## Viewing Logs

**Logs for specific service:**

```shell
make logs                    # API logs
docker compose logs -f api  # API logs (more detailed)
docker compose logs -f db    # Database logs
docker compose logs -f web   # Frontend logs
```

**Logs for all services:**

```shell
docker compose logs -f       # Logs for all services
```

**Logs via Grafana:**

1. Open http://localhost:3000
2. Go to Explore
3. Select Loki datasource
4. Use LogQL queries for filtering

## Metrics or Dashboard Issues

**Symptoms:** Metrics are not displayed in Grafana or dashboards are empty.

**Solution:**

```shell
# Check Prometheus availability
curl http://localhost:9090/-/healthy

# Check metrics API
curl http://localhost:8000/metrics

# Check Prometheus configuration
docker compose exec prometheus promtool check config /etc/prometheus/prometheus.yml

# Restart Grafana to update dashboards
docker compose restart grafana
```

## Starting services

### Python Version Issues

**Symptoms:** Errors when installing dependencies or version incompatibility.

**Solution:**

```shell
# Check installed Python version
python3.12 --version

# Check Python path
which python3.12

# If Python 3.12 is not installed, install it:
# macOS: brew install python@3.12
# Ubuntu/Debian: sudo apt-get install python3.12 python3.12-venv
# Windows: Download from python.org
```

**Note:** Make sure you're using exactly Python 3.12, not another version.

### Database Connection Issues

**Symptoms:** API returns connection errors, health check fails, or connection errors when starting backend locally.

**Check database status:**

```shell
# Check if DB is running
docker compose ps db

# Start DB if not running
docker compose up -d db

# Wait for DB readiness (may take a few seconds)
docker compose exec db pg_isready -U app -d appdb

# Check DB logs if there are issues
docker compose logs db

# Check API health check
curl http://localhost:8000/health

# Check DB accessibility from container
docker compose exec api python -c "import asyncio; from app.db import engine; asyncio.run(engine.connect())"
```

**Check environment variables:**

```shell
# Check if environment variables are set
echo $DATABASE_URL
echo $ALEMBIC_DATABASE_URL

# If not set, set them:
export DATABASE_URL="postgresql+asyncpg://app:app@localhost:5432/appdb"
export ALEMBIC_DATABASE_URL="postgresql+psycopg://app:app@localhost:5432/appdb"
```

**Test connection:**

```shell
# Try connecting to DB via psql
docker compose exec db psql -U app -d appdb -c "SELECT 1;"
```

**Common causes:**

- DB container is not running or restarting
- Incorrect credentials in environment variables
- Network issues between containers
- DB is not ready yet (check health check)

### Port Issues

**Symptoms:** "port already in use" or "address already in use" errors.

**Check occupied ports:**

```shell
# Check which processes are using ports
# macOS/Linux:
lsof -i :8000   # API port
lsof -i :5432   # PostgreSQL port
lsof -i :8080   # Frontend port
lsof -i :5173   # Vite dev server port
lsof -i :3000   # Grafana
lsof -i :9090   # Prometheus

# Windows:
netstat -ano | findstr :8000
netstat -ano | findstr :5432
```

**Solution:**

1. **Change ports in configuration:**
    - API (8000): Change in `docker-compose.yml` or use `--port` flag for uvicorn
    - PostgreSQL (5432): Change in `docker-compose.yml`
    - Frontend (8080): Change in `docker-compose.yml`
    - Vite dev server (5173): Change in `vite.config.js`

2. **Stop conflicting processes:**

   ```shell
   # Find process PID
   lsof -i :8000

   # Stop process (replace PID with actual)
   kill -9 <PID>
   ```

### Virtualenv Issues

**Symptoms:** Errors when creating virtualenv or activation.

**Full virtualenv recreation:**

```shell
# Remove existing virtualenv
rm -rf backend/.venv

# Recreate and install dependencies
make venv-install
```

**Activation check:**

```shell
# After activation check:
which python  # Should point to backend/.venv/bin/python
python --version  # Should be 3.12.x
```

**Permission issues:**

```shell
# Make sure execution permissions exist
chmod +x backend/.venv/bin/python
chmod +x backend/.venv/bin/pip
```

### npm Issues

**Symptoms:** Errors when installing frontend dependencies.

**Full reinstallation:**

```shell
cd frontend

# Remove node_modules and lock file
rm -rf node_modules package-lock.json

# Clear npm cache
npm cache clean --force

# Reinstall dependencies
npm install
```

**Node.js version check:**

```shell
# Make sure Node.js >= 20 is used
node --version

# If version is outdated, update Node.js:
# macOS: brew upgrade node
# Linux: Use nvm or update via package manager
# Windows: Download from nodejs.org
```

**npm registry issues:**

```shell
# Check registry settings
npm config get registry

# If you need to change (e.g., for corporate proxy):
npm config set registry https://registry.npmjs.org/
```

### Docker Issues

**Symptoms:** Services don't start or crash.

**Check Docker status:**

```shell
# Check if Docker is running
docker ps

# Check logs for all services
docker compose logs

# Check logs for specific service
docker compose logs api
docker compose logs db
```

**Full restart:**

```shell
# Stop and remove all containers
make down

# Start again
make up

# If issues persist, full cleanup:
make clean
make up
```

**Docker volumes issues:**

```shell
# Check volumes
docker volume ls

# Remove problematic volume (careful - will delete data!)
docker volume rm <volume_name>

# Restart services to create new volume
make up
```

**Docker networks issues:**

```shell
# Check networks
docker network ls

# Remove problematic network
docker network rm <network_name>

# Restart services
make up
```

### Migration Issues

**Symptoms:** Errors when applying migrations or database schema mismatch.

**Check migration state:**

```shell
# Check current migration version
docker compose exec api alembic current

# Check migration history
docker compose exec api alembic history
```

**Apply migrations manually:**

```shell
# Apply all migrations
docker compose exec api alembic upgrade head

# Or locally (if backend is running without Docker)
cd backend
source .venv/bin/activate
alembic -c alembic.ini upgrade head
```

**Rollback migrations (careful!):**

```shell
# Rollback last migration
docker compose exec api alembic downgrade -1

# Rollback to specific version
docker compose exec api alembic downgrade <revision>
```

### If nothing helps

If issues remain unresolved:

- Check service logs: `docker compose logs [service_name]`
- Check tool documentation in section [Additional Resources](./architecture.md#additional-resources)
- Create an issue in the project repository with detailed problem description and logs
