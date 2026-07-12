# Database and data seeding

## Schema

PostgreSQL, single table:

- **`items`**
  - `id` (SERIAL PRIMARY KEY)
  - `name` (VARCHAR(100) UNIQUE NOT NULL)

Credentials default to `app`/`app`/`appdb`, overridable via `.env`
(see `.env.example`).

## Migrations

Managed by Alembic; applied automatically when the api service starts.

- Migration sources: `services/backend/alembic/versions/`
- Create a new migration (backend venv, db running):

  ```shell
  cd services/backend
  alembic revision --autogenerate -m "change description"
  ```

- Apply manually inside the running container:

  ```shell
  docker compose -f deploy/compose/docker-compose.yml --project-directory . \
    exec api python -m alembic -c /app/alembic.ini upgrade head
  ```

## Seed data

```shell
make seed        # add 20 test items
make seed-reset  # clear all data, then reseed
make seed-dry    # show what would be created, no writes
```

Seed script flags (`python -m app.seed`): `--count N`, `--only-reset`,
`--dry-run`.
