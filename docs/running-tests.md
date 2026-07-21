# Running Tests

## Prerequisites

Environment setup is covered in [Local setup](local-setup.md). Run `make doctor` to verify
Python and Node versions are correct. DB credentials default to `app/app/appdb`; override via
`.env` (see `.env.example`).

## Quick Start

| Goal | Command |
| ------ | --------- |
| All tests | `make test` |
| Backend only | `make test-backend` |
| Frontend only | `make test-frontend` |
| Inside Docker | `make test-docker` |
| Full CI check | `make ci` |

`make test-backend` starts the DB container if absent, runs migrations, runs pytest, and stops
the DB if it started it. `make ci` runs exactly what CI runs -- see [CI docs](ci.md) for
pipeline details. Lint/format specifics are in [Contributing](contributing.md).

## Backend Tests

Tests live in `services/backend/tests/` and use pytest + pytest-asyncio.

Install the backend editable in any virtualenv:

```shell
pip install -e "services/backend[dev]"
```

The backend imports its generated gRPC stubs at module load time (`app.main`
-> `app.grpc_server`), so they must exist before pytest can even collect
tests -- generate them once from the repo root:

```shell
make generate
```

Run via make (recommended -- `make test-backend` already depends on `generate`):

```shell
make test-backend
```

Or invoke pytest directly from an activated virtualenv (after `make generate`):

```shell
# run all
pytest -v services/backend

# run one file
pytest services/backend/tests/test_items.py -v

# run one test
pytest services/backend/tests/test_items.py::test_items_crud -v

# with markers / keyword filter
pytest services/backend -k "test_items" -v

# with coverage
pytest services/backend --cov=app --cov-report=term --cov-report=html
# open services/backend/htmlcov/index.html in a browser

# retry only failures from last run
pytest services/backend --lf
```

### Test Database

Tests connect to `postgresql+asyncpg://app:app@localhost:5432/appdb` by default (override via
`.env`). Fixtures reset state before each test, apply migrations, seed 10 items, then remove
only items created by that test after it finishes.

### Test Files

| File | What it covers |
| ------ | ---------------- |
| `tests/test_health.py` | `/health` endpoint, DB connectivity |
| `tests/test_items.py` | CRUD, validation, uniqueness |
| `tests/test_seeded_items.py` | Scenarios against pre-seeded data |
| `tests/test_cleanup.py` | Verifies tests leave no residue |
| `tests/conftest.py` | Fixtures, async loop config, DB setup |

### Docker Execution

```shell
make test-docker
```

Builds a container, installs deps, runs migrations, runs pytest, cleans up. Use for
production-like validation or when a local virtualenv is unavailable.

## Frontend Tests

Tests live next to source files (`*.test.jsx`) and use Vitest + React Testing Library.

```shell
make test-frontend

# or directly
cd services/frontend && npm run test
```

### Modes

By default tests mock `fetch`. To run against a live API:

```shell
export VITE_TEST_API_URL="http://localhost:8000"
make test-frontend
```

### Useful Vitest Flags

```shell
cd services/frontend

npm run test:watch          # re-run on file change
npm run test:ui             # interactive browser UI
npm run test -- --coverage  # coverage report in coverage/
npm run test -- src/App.test.jsx  # single file
```

### Frontend Test Files

| File | What it covers |
| ------ | ---------------- |
| `src/App.test.jsx` | CRUD via UI, user interactions, error handling |
| `src/main.test.jsx` | Web Vitals metrics (LCP, INP, CLS, FCP, TTFB) |
| `src/test/setup.js` | Vitest env setup, mocks, jest-dom matchers |

## Integration Tests

Run backend and frontend together against live services:

```shell
# start full stack
make up

# backend tests (uses real DB already up)
make test-backend

# frontend tests against real API
export VITE_TEST_API_URL="http://localhost:8000"
make test-frontend
```

If you need to start only the DB and API via compose:

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . up -d db api
```

## Troubleshooting

**DB connection errors** -- `ConnectionError`, `OperationalError`:

```shell
docker compose -f deploy/compose/docker-compose.yml --project-directory . ps db
docker compose -f deploy/compose/docker-compose.yml --project-directory . exec db pg_isready -U app -d appdb
```

`make test-backend` starts the DB automatically, so prefer that over manual compose commands.

**Import errors** -- `ModuleNotFoundError`:

```shell
# verify package is installed editable
python -c "import app; print(app.__file__)"

# reinstall if needed
pip install -e "services/backend[dev]"
```

**Event loop errors** -- `RuntimeError: Event loop is closed`:
Fixtures handle this automatically. If errors persist, check that `asyncio_mode = "auto"` is
set in `services/backend/pyproject.toml` and upgrade pytest-asyncio.

**Frontend module errors** -- `Cannot find module`:

```shell
cd services/frontend
rm -rf node_modules
npm install
```

**Frontend API errors** (real API mode) -- `NetworkError`, `Failed to fetch`:

```shell
curl http://localhost:8000/health
echo $VITE_TEST_API_URL
```

**Timeout errors**:

- Backend: `pytest --timeout=30`
- Frontend: pass `{ timeout: 10000 }` as third arg to `test()`
- Or switch to mocked API mode (no `VITE_TEST_API_URL` set)

## Reference

- [pytest](https://docs.pytest.org/)
- [pytest-asyncio](https://pytest-asyncio.readthedocs.io/)
- [Vitest](https://vitest.dev/)
- [React Testing Library](https://testing-library.com/docs/react-testing-library/intro/)
