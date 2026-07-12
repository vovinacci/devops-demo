# Database and data seeding

## Database Structure

The project uses PostgreSQL 16 with the following structure:

- **Table `items`**
  - `id` (SERIAL PRIMARY KEY) - unique identifier
  - `name` (VARCHAR(100) UNIQUE NOT NULL) - item name

## Migrations

Database migrations are managed through Alembic and executed automatically on API service startup.

- **Migration structure:**

  ```text
  services/backend/alembic/
  ├── env.py              # Alembic configuration
  └── versions/           # Migration files
      └── 20251026_0001_init_items.py
  ```

- **Creating a new migration:**

  ```shell
  cd services/backend
  alembic revision --autogenerate -m "change description"
  ```

- **Applying migrations manually:**

  ```shell
  docker compose exec api alembic upgrade head
  ```

## Seed Data

For testing and development, commands are available to add test data:

```shell
# Add 20 test items (default)
make seed

# Clear all data and add new seed data
make seed-reset

# Dry run - show what will be created without actual write
make seed-dry
```

**Seed script parameters:**

- `--count N` - number of items to create (default: 20)
- `--only-reset` - clear all data and add new
- `--dry-run` - show what will be created without writing
