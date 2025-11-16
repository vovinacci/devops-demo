# Running Tests

This document contains detailed instructions for running tests in the DevOps Demo project. It covers unit tests, integration tests, backend and frontend tests, as well as code coverage setup and troubleshooting common issues.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Backend Tests](#backend-tests)
- [Frontend Tests](#frontend-tests)
- [Integration Tests](#integration-tests)
- [CI/CD Tests](#cicd-tests)
- [Code Coverage](#code-coverage)
- [Troubleshooting](#troubleshooting)
- [Reference](#reference)

## Prerequisites

Before running tests, you need to install all dependencies and set up the development environment. Detailed instructions are available in [Local setup](local-setup.md).

- **Minimum requirements:**

  - Python 3.12 with backend dependencies installed
  - Node.js >= 24 with frontend dependencies installed
  - PostgreSQL database (can be run via Docker)
  - Docker and Docker Compose (for integration tests)

## Quick Start

### Running All Tests

The easiest way to run all project tests:

```shell
# Run all tests (backend + frontend)
make test
```

This command automatically:

- Checks if database is running, starts it if needed
- Runs backend tests via virtualenv
- Runs frontend tests via npm
- Shows summary information about test results

### Running Only Backend Tests

```shell
# Locally via virtualenv (recommended for development)
make test-backend

# Or in Docker container (for CI/CD or production-like environments)
make test-docker
```

### Running Only Frontend Tests

```shell
make test-frontend
```

## Backend Tests

Backend tests are written using pytest and pytest-asyncio for async code support. All tests are located in the `backend/tests/` directory.

### Local Execution (Virtualenv)

For local development, using virtualenv is recommended:

```shell
# Make sure database is running
docker compose up -d db

# Wait for DB readiness
docker compose exec db pg_isready -U app -d appdb

# Run all backend tests
make test-backend

# Or manually
cd backend
source .venv/bin/activate
pytest -v
```

**Advantages of local execution:**

- Faster startup (no Docker overhead)
- Easier debugging with IDE
- Ability to use breakpoints
- Faster feedback loop

### Docker Execution

For testing in production-like environments or in CI/CD:

```shell
# Run tests in Docker container
make test-docker
```

This command:

- Checks if database is running, starts it if needed
- Creates temporary container with backend code
- Installs dependencies in container
- Applies database migrations
- Runs tests via pytest
- Automatically cleans up container after execution

**When to use Docker tests:**

- CI/CD pipelines
- Testing in production-like environments
- When local virtualenv is unavailable
- For checking compatibility with production image

### Test Options

**Running specific test file:**

```shell
cd backend
.venv/bin/pytest tests/test_items.py -v
```

**Running specific test function:**

```shell
cd backend
.venv/bin/pytest tests/test_items.py::test_items_crud -v
```

**Running tests with detailed output:**

```shell
cd backend
.venv/bin/pytest -v  # Verbose mode
.venv/bin/pytest -vv  # Even more detailed output
```

**Running tests with code coverage:**

```shell
cd backend
.venv/bin/pytest --cov=app --cov-report=html --cov-report=term
```

**Running tests in parallel mode (if pytest-xdist is installed):**

```shell
cd backend
.venv/bin/pytest -n auto  # Automatic worker count detection
.venv/bin/pytest -n 4  # Use 4 workers
```

**Running only failed tests from previous run:**

```shell
cd backend
.venv/bin/pytest --lf  # Last failed
```

**Running tests with name filtering:**

```shell
cd backend
.venv/bin/pytest -k "test_items"  # Only tests containing "test_items" in name
```

### Test Structure

Tests are organized by functionality:

**`tests/test_health.py`**

- Health check endpoint tests (`/health`)
- Service status and database connection verification
- Tests for various scenarios (normal operation, DB issues)

**`tests/test_items.py`**

- CRUD operation tests for items
- Creating, reading, deleting items
- Data validation and error handling
- Field uniqueness verification

**`tests/test_seeded_items.py`**

- Integration tests with seed data
- Tests working with pre-populated database
- Complex scenario verification with real data

**`tests/test_cleanup.py`**

- Database cleanup verification tests
- Verification that tests don't leave "garbage" after execution
- Database state validation before and after tests

**`tests/conftest.py`**

- Pytest fixtures and configuration
- Test database setup
- Fixtures for creating test data
- Async event loop configuration

### Test Database

Tests use a separate test database. Test fixtures automatically:

**Before each test:**

- Reset database state
- Apply all migrations
- Create deterministic test data (10 items by default)

**After each test:**

- Remove only items created during the test
- Leave seed data untouched
- Clean up all changes made by the test

**Important:** Tests automatically clean up after themselves, removing only items created during test execution. Seed data remains for subsequent tests.

**Test database configuration:**

- Connection string: `postgresql+asyncpg://app:app@localhost:5432/appdb`
- Tests use the same database as development (but with automatic cleanup)
- For isolation, you can use a separate test database via environment variable

## Frontend Tests

Frontend tests are written using Vitest and React Testing Library. All tests are located next to components (`.test.jsx` files).

- **Basic run:**

  Via `npm`

  ```shell
  cd frontend
  npm run test
  ```

  Or via Make

  ```shell
  make test-frontend
  ```

### Modes

Frontend tests support two operation modes:

- **Mocked API (default)**
  - Uses mocked `fetch` for fast execution
  - Doesn't require running backend
  - Faster execution
  - Ideal for component unit tests

- **Real API**
  - Uses real backend API
  - Requires running backend service
  - More realistic testing
  - Ideal for integration tests

**Switching to Real API mode:**

```shell
# Set environment variable
export VITE_TEST_API_URL="http://localhost:8000"
# or
export VITEST_TEST_API_URL="http://localhost:8000"

# Run tests
npm run test
```

**Note:** When using Real API mode, tests automatically clean up created items after execution.

### Options

**Running tests in watch mode:**

```shell
cd frontend
npm run test:watch
```

Watch mode automatically restarts tests on file changes, which is very convenient during development.

**Running tests with UI:**

```shell
cd frontend
npm run test:ui
```

UI mode opens an interactive interface for viewing and running tests in the browser.

**Running specific test file:**

```shell
cd frontend
npm run test -- src/App.test.jsx
```

**Running tests with coverage:**

```shell
cd frontend
npm run test -- --coverage
```

**Running tests in parallel mode:**

```shell
cd frontend
npm run test -- --threads  # Use threads
npm run test -- --no-threads  # Sequential execution
```

**Running only failed tests:**

```shell
cd frontend
npm run test -- --reporter=verbose --reporter=json
```

### Structure

**`src/App.test.jsx`**

- Main App component tests
- CRUD operations via UI
- User interaction testing
- Data display verification
- Error handling tests

**`src/main.test.jsx`**

- Web Vitals metrics integration tests
- Metrics sending to backend verification
- Tests for different metric types (LCP, INP, CLS, FCP, TTFB)

**`src/test/setup.js`**

- Test environment setup
- Vitest configuration
- Mock and global fixture setup
- Import additional matchers (@testing-library/jest-dom)

## Integration Tests

Integration tests verify interaction between backend and frontend components.

### Running Integration Tests

**Full stack (Backend + Frontend):**

```shell
# Start all services
make up

# Wait for all services to be ready
sleep 10  # Or check health checks

# Run backend integration tests
make test-backend

# Run frontend tests against real API
export VITE_TEST_API_URL="http://localhost:8000"
make test-frontend
```

**Backend integration tests only:**

```shell
# Start only DB and API
docker compose up -d db api

# Wait for readiness
docker compose exec api python -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')"

# Run tests
make test-backend
```

**Frontend integration tests only:**

```shell
# Start backend
docker compose up -d db api

# Configure frontend to use real API
export VITE_TEST_API_URL="http://localhost:8000"

# Run frontend tests
make test-frontend
```

### Types of Integration Tests

**Backend integration tests:**

- Tests using real database
- Tests with seed data (`test_seeded_items.py`)
- Cleanup verification tests (`test_cleanup.py`)
- Tests verifying full CRUD operation cycle

**Frontend integration tests:**

- Tests using real API (with `VITE_TEST_API_URL`)
- Web Vitals metrics sending tests
- Full user interaction cycle with API tests

## CI/CD Tests

Tests automatically run in GitHub Actions when:

- Creating Pull Request to `main`, `master`, or `develop`
- Pushing to these branches
- Manual workflow trigger

### CI/CD Configuration

CI/CD configuration is located in `.github/workflows/pr.yml` and includes:

**Backend tests:**

- Python 3.12 installation
- Dependency installation
- Running linting (ruff, mypy)
- Running tests via pytest in Docker

**Frontend tests:**

- Node.js installation
- npm dependency installation
- Running linting (ESLint)
- Running tests via Vitest

**Infrastructure tests:**

- YAML file validation
- Docker Compose configuration check
- Dockerfile linting

### Local Check Before Commit

Before creating a Pull Request, it's recommended to run all checks locally:

```shell
# Run all linting checks
make lint

# Run all tests
make test

# Run pre-commit hooks
make pre-commit-run
```

This will help avoid failed CI/CD checks and save time.

## Code Coverage

### Backend Coverage

**Generating coverage report:**

```shell
cd backend

# Install pytest-cov if not already installed
.venv/bin/pip install pytest-cov

# Run tests with coverage
.venv/bin/pytest --cov=app --cov-report=html --cov-report=term

# View HTML report
open htmlcov/index.html  # macOS
# or
xdg-open htmlcov/index.html  # Linux
# or
start htmlcov/index.html  # Windows
```

**Report types:**

- `--cov-report=term` - terminal output
- `--cov-report=html` - HTML report in `htmlcov/`
- `--cov-report=xml` - XML report for CI/CD integration
- `--cov-report=json` - JSON report for programmatic processing

**Setting minimum coverage:**

```shell
# Set minimum coverage (e.g., 80%)
.venv/bin/pytest --cov=app --cov-fail-under=80
```

### Frontend Coverage

**Generating coverage report:**

```shell
cd frontend

# Run tests with coverage
npm run test -- --coverage

# View report (usually opens automatically)
# Or find in coverage/ directory
```

**Coverage configuration:**
Coverage is configured in `vite.config.js` or `vitest.config.js` via `coverage` options.

## Troubleshooting

### Database Connection Errors (Backend Tests)

**Symptoms:** `ConnectionError`, `OperationalError`, or other database connection errors.

**Solution:**

```shell
# Make sure DB is running
docker compose ps db

# Start DB if not running
docker compose up -d db

# Wait for DB readiness
docker compose exec db pg_isready -U app -d appdb

# Check environment variables
echo $DATABASE_URL
echo $ALEMBIC_DATABASE_URL
```

**Connection check:**

```shell
# Try connecting to DB manually
docker compose exec db psql -U app -d appdb -c "SELECT 1;"
```

### Import Errors (Backend Tests)

**Symptoms:** `ModuleNotFoundError`, `ImportError`.

**Solution:**

```shell
# Make sure virtualenv is activated
source backend/.venv/bin/activate

# Check that package is installed
cd backend
.venv/bin/python -c "import app; print(app.__file__)"

# Reinstall dependencies if needed
pip install -e ".[dev]"
```

**PYTHONPATH check:**

```shell
# Make sure current directory is in PYTHONPATH
cd backend
export PYTHONPATH="${PYTHONPATH}:$(pwd)"
```

### Event Loop Errors (Backend Tests)

**Symptoms:** `RuntimeError: Event loop is closed`, `There is no current event loop`.

**Solution:**
These errors are usually resolved automatically through test fixtures. If issues persist:

```shell
# Check pytest-asyncio configuration in pyproject.toml
# Make sure it's set: asyncio_mode = "auto"

# Reinstall pytest-asyncio
cd backend
.venv/bin/pip install --upgrade pytest-asyncio
```

### Module Errors (Frontend Tests)

**Symptoms:** `Cannot find module`, `Module not found`.

**Solution:**

```shell
cd frontend

# Remove node_modules and reinstall
rm -rf node_modules package-lock.json
npm install

# Check that all dependencies are installed
npm list --depth=0
```

### API Connection Errors (Frontend Tests, Real API Mode)

**Symptoms:** `NetworkError`, `Failed to fetch`, timeout errors.

**Solution:**

```shell
# Make sure backend is running
make up

# Check API health check
curl http://localhost:8000/health

# Check environment variable
echo $VITE_TEST_API_URL

# Set correct URL if needed
export VITE_TEST_API_URL="http://localhost:8000"
```

### Timeout Errors

**Symptoms:** Tests fail due to timeout.

**Solution:**

- Increase timeout in test configuration
- Use mocked API mode for faster execution
- Check database and API performance

**For Vitest:**

```javascript
// In test file or configuration
test('my test', async () => {
  // ...
}, { timeout: 10000 })  // 10 seconds
```

**For pytest:**

```shell
# Set timeout via pytest-timeout
.venv/bin/pytest --timeout=30  # 30 seconds per test
```

## Reference

### Useful Links

**Backend testing:**

- [pytest Documentation](https://docs.pytest.org/) - Complete pytest documentation
- [pytest-asyncio Documentation](https://pytest-asyncio.readthedocs.io/) - Async test documentation
- [httpx Documentation](https://www.python-httpx.org/) - HTTP client for testing

**Frontend testing:**

- [Vitest Documentation](https://vitest.dev/) - Vitest documentation
- [React Testing Library Documentation](https://testing-library.com/docs/react-testing-library/intro/) - React Testing Library documentation
- [Testing Library User Event](https://testing-library.com/docs/user-event/intro/) - User interaction simulation

**General:**

- [Test-Driven Development](https://en.wikipedia.org/wiki/Test-driven_development) - TDD methodology
- [Testing Best Practices](https://kentcdodds.com/blog/common-mistakes-with-react-testing-library) - Best practices for React testing

### Additional Resources

Detailed information about development environment setup is available in [Local setup](local-setup.md).

Information about writing tests and code quality standards is available in [Contributing](contributing.md).
