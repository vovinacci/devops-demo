# Local Setup

This document contains detailed instructions for setting up the local development environment for the DevOps Demo project. It covers installation of all necessary dependencies, configuration of backend and frontend components, Docker setup, and troubleshooting common issues.

## Table of Contents

- [System Requirements](#system-requirements)
- [Installing Dependencies](#installing-dependencies)
- [Backend Setup](#backend-setup)
- [Frontend Setup](#frontend-setup)
- [Docker Setup](#docker-setup)
- [Environment Variables](#environment-variables)
- [Running Services Locally](#running-services-locally)
- [Pre-commit Setup](#pre-commit-setup)
- [Troubleshooting](#troubleshooting)

## System Requirements

### Required Dependencies

- **Python 3.12**
  - Official website: https://www.python.org/
  - Version check: `python3.12 --version`

- **Node.js version 24 or newer**
  - Official website: https://nodejs.org/
  - Version check: `node --version`
  - npm is installed together with Node.js

- **Docker and Docker Compose**
  - Docker Engine: https://docs.docker.com/engine/install/
  - On macOS, consider using [OrbStack](https://orbstack.dev/)
  - Minimum Docker version: 24 or newer
  - Minimum Docker Compose version: 2.24 or newer
  - Version check:

    ```shell
    docker --version
    docker compose version
    ```

- **Make**
  - Official website: https://www.gnu.org/software/make/
  - On macOS install via Xcode Command Line Tools: `xcode-select --install`
  - On Linux: `sudo apt-get install build-essential` (Ubuntu/Debian) or `sudo yum groupinstall "Development Tools"` (RHEL/CentOS)
  - Check: `make --version`

- **Python Virtualenv**
  - Used for isolating Python dependencies
  - Official website: https://virtualenv.pypa.io/en/latest/
  - Installed automatically with Python 3.12 (module `venv`)
  - Alternative: `pip install virtualenv`

- **Pre-commit**
  - Automatic code quality checks before commit
  - Official website: https://pre-commit.com/
  - Install via: `pip install pre-commit`
  - More details in section [Pre-commit Setup](#pre-commit-setup)

## Installing Dependencies

### Checking System Requirements

The first command after cloning -- verifies every tool, version, the
Docker daemon, and available resources, with actionable errors:

```shell
make doctor
```

Toolchain versions are pinned in `.mise.toml`; with
[mise](https://mise.jdx.dev) installed, `mise install` provides the
pinned Python and Node automatically. Manual check, if you prefer:

```shell
# Check Python
python3.12 --version

# Check Node.js and npm
node --version
npm --version

# Check Docker
docker --version
docker compose version

# Check Make (optional)
make --version
```

## Backend Setup

### Using Make (Recommended)

The easiest way to set up the backend environment:

```shell
# Create virtualenv and install all dependencies
make venv-install
```

This command performs the following actions:

- Creates Python 3.12 virtualenv in `services/backend/.venv` (idempotent operation - does not recreate if already exists)
- Updates pip, setuptools, and wheel to latest versions
- Installs all production and development dependencies from `pyproject.toml`
- Includes development dependencies (pytest, ruff, mypy, etc.)

### Manual Setup

If you don't use Make or need more control over the process:

```shell
# Navigate to backend directory
cd services/backend

# Create Python virtualenv
python3.12 -m venv .venv

# Activate virtualenv
# On Linux/macOS:
source .venv/bin/activate

# On Windows (Command Prompt):
.venv\Scripts\activate.bat

# On Windows (PowerShell):
.venv\Scripts\Activate.ps1

# Update pip and build tools
pip install --upgrade pip setuptools wheel

# Install dependencies (production + development)
pip install -e ".[dev]"
```

**Note:** The `-e` flag installs the package in editable mode, allowing changes without reinstallation. `[dev]` indicates installation of additional development dependencies.

### Verification

After installation, verify that all tools are available:

```shell
# Navigate to backend directory
cd services/backend

# Check Python version (should be 3.12.x)
.venv/bin/python --version

# Check development tool versions
.venv/bin/ruff --version
.venv/bin/mypy --version
.venv/bin/pytest --version

# Verify package is installed correctly
.venv/bin/python -c "import app; print(app.__file__)"
```

## Frontend Setup

- Installing Dependencies

  ```shell
  # Navigate to frontend directory
  cd services/frontend

  # Install all npm dependencies
  npm install
  ```

  This command:

  - Reads `package.json` and `package-lock.json`
  - Installs all dependencies in `node_modules/`
  - Creates or updates `package-lock.json` to ensure reproducibility

- Verification

  ```shell
  # Check tool versions
  cd services/frontend

  # Check ESLint
  npm run lint --version  # or npx eslint --version

  # Check Vitest
  npm run test --version  # or npx vitest --version

  # Check Vite
  npm run dev --version  # or npx vite --version
  ```

- Frontend Dependency Structure
  - **Production dependencies:**
    - React 19 - UI library
    - Vite - build tool and dev server
  - **Development dependencies:**
    - Vitest - test framework
    - React Testing Library - utilities for testing React
    - ESLint - linter for JavaScript/React
    - Prettier - code formatter
    - `@testing-library/jest-dom` - additional matchers for tests

## Docker Setup

### Running Only Database

For local backend development without Docker, you can run only PostgreSQL:

```shell
# Start only database
docker compose up -d db

# Check status
docker compose ps db

# Check health check
docker compose exec db pg_isready -U app -d appdb
```

### Running All Services

For a complete local environment with all services:

```shell
# Start all services (development mode)
make up

# Or directly via Docker Compose
docker compose up -d --build
```

This will start:

- PostgreSQL database
- FastAPI backend
- React frontend
- Prometheus for metrics
- Grafana for visualization
- Loki for logs
- Grafana Alloy for log collection
- Postgres Exporter for database metrics
- cAdvisor for container metrics

## Environment Variables

### For Local Backend Development (without Docker)

When developing backend locally (without Docker container), you need to set the following environment variables:

```shell
# PostgreSQL connection string for async operations (SQLAlchemy + asyncpg)
export DATABASE_URL="postgresql+asyncpg://app:app@localhost:5432/appdb"

# PostgreSQL connection string for synchronous operations (Alembic + psycopg)
export ALEMBIC_DATABASE_URL="postgresql+psycopg://app:app@localhost:5432/appdb"
```

**Note:** These variables are automatically set when using Docker Compose via `deploy/compose/docker-compose.yml`.

### Creating .env File (Optional)

For convenience, you can create a `.env` file in the project root:

```shell
# .env
DATABASE_URL=postgresql+asyncpg://app:app@localhost:5432/appdb
ALEMBIC_DATABASE_URL=postgresql+psycopg://app:app@localhost:5432/appdb
```

**Warning:** `.env` files should not be committed to Git. Make sure `.env` is added to `.gitignore`.

### Environment Variables for Frontend

Frontend uses environment variables with `VITE_` prefix:

```shell
# Example (if you need to change API URL)
export VITE_API_URL="http://localhost:8000"
```

Environment variables are available in code via `import.meta.env.VITE_API_URL`.

## Running Services Locally

### Backend (without Docker)

For backend development without Docker container:

```shell
# Activate virtualenv
source services/backend/.venv/bin/activate  # Linux/macOS
# or
backend\.venv\Scripts\activate  # Windows

# Navigate to backend directory
cd services/backend

# Apply database migrations
alembic -c alembic.ini upgrade head

# Start API server with hot reload
uvicorn app.main:app --reload --port 8000
```

**Requirements:**

- PostgreSQL database must be running
- Environment variables `DATABASE_URL` and `ALEMBIC_DATABASE_URL` must be set

**Starting database:**

```shell
# Start only DB via Docker
docker compose up -d db

# Wait for DB readiness
docker compose exec db pg_isready -U app -d appdb
```

**API Access:**

- API will be available at: http://localhost:8000
- Swagger UI: http://localhost:8000/docs
- ReDoc: http://localhost:8000/redoc

### Frontend (without Docker)

For frontend development without Docker container:

```shell
# Navigate to frontend directory
cd services/frontend

# Start development server
npm run dev
```

**Frontend Access:**

- Frontend will be available at: http://localhost:5173 (default Vite)
- Hot Module Replacement (HMR) is enabled automatically
- API requests are proxied to `http://localhost:8000` (configured in `vite.config.js`)

**Note:** Frontend requires a running backend API on port 8000.

### Combined Startup

You can run backend locally and frontend via Docker, or vice versa:

```shell
# Option 1: Backend locally, Frontend via Docker
docker compose up -d db web  # Only DB and frontend
source services/backend/.venv/bin/activate
cd services/backend
uvicorn app.main:app --reload --port 8000

# Option 2: Backend via Docker, Frontend locally
docker compose up -d db api  # Only DB and backend
cd services/frontend
npm run dev
```

## Pre-commit Setup

Pre-commit hooks automatically run code quality checks before each Git commit. This helps maintain high code quality standards and avoid errors at early stages.

### Installation

**Using Make:**

```shell
make pre-commit-install
```

**Manually:**

```shell
# Install pre-commit (if not already installed)
cd services/backend
.venv/bin/pip install pre-commit

# Install Git hooks
.venv/bin/pre-commit install
```

This will install hooks in `.git/hooks/pre-commit`, which will automatically run before each commit.

### Running Pre-commit Manually

To check all files without committing:

**Using Make:**

```shell
make pre-commit-run
```

**Manually:**

```shell
cd services/backend
.venv/bin/pre-commit run --all-files
```

### Configured Hooks

The project uses the following pre-commit hooks (see `.pre-commit-config.yaml`):

**Backend:**

- `ruff` - Python linting and automatic error fixing
- `ruff-format` - Python code formatting
- `mypy` - Static type checking
- `bandit` - Code security checking
- `detect-secrets` - Secret detection in code

**Frontend:**

- `eslint` - JavaScript/React code linting
- `prettier` - Code formatting

**Infrastructure:**

- `yamllint` - YAML file checking
- `hadolint` - Dockerfile checking
- `docker-compose-validate` - docker-compose.yml validation

**General:**

- `trailing-whitespace` - Remove trailing whitespace at end of lines
- `end-of-file-fixer` - Add newline at end of files
- `check-yaml` - YAML syntax validation
- `check-json` - JSON syntax validation
- `check-toml` - TOML syntax validation

### Updating Hooks

To update hooks to latest versions:

```shell
make pre-commit-update
```

Or manually:

```shell
cd services/backend
.venv/bin/pre-commit autoupdate
```

## Troubleshooting

See [troubleshooting guide](./troubleshooting.md#starting-services)

### Additional Resources

If issues remain unresolved, check [if nothing helps](./troubleshooting.md#if-nothing-helps) section of the troubleshooting guide.
