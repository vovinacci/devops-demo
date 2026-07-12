# Contributing

This document contains detailed guidelines for contributing to the DevOps Demo project. It covers the development process, code standards, testing, code review process, and best practices for maintaining high code quality.

## Table of Contents

- [Getting Started](#getting-started)
- [Linting and Testing](#linting-and-testing)
- [Backend Linting](#backend-linting)
- [Frontend Linting](#frontend-linting)
- [Infrastructure Linting](#infrastructure-linting)
- [Git Hooks Setup](#git-hooks-setup)
- [Code Style](#code-style)
- [Testing](#testing)
- [CI/CD](#cicd)
- [Reference](#reference)

## Getting Started

### Installing Required Software

Install all necessary tools and set up the development environment. Detailed instructions are available in [Local setup](local-setup.md).

Toolchain versions are pinned in `.mise.toml` (the single source of truth);
verify your environment with:

```bash
make doctor
```

### Making Changes

After setting up the environment, you can start making changes to the code. Make sure you understand the project structure and follow established code standards.

**Main directories:**

- `services/backend/app/` - Backend code (FastAPI)
- `services/frontend/src/` - Frontend code (React)
- `services/backend/tests/` - Backend tests
- `services/frontend/src/*.test.jsx` - Frontend tests
- `observability/` - Observability stack configuration

### Commits

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):
`<type>(<scope>): <subject>`, e.g. `feat(backend): add item tags`.
Common scopes here: `backend`, `frontend`, `api`, `db`, `infra`.

**The PR title matters most**: this is a squash-merge repository, so the PR
title becomes the commit message on `main` -- it is linted by CI and drives
automated releases (see [CI/CD architecture](ci.md)). Branch commits are
work-in-progress and disappear at merge.

Why this workflow: [engineering principles](engineering-principles.md)
Section 4.

### Submitting Pull Request

Create a Pull Request and fill in the template: *why* (with a link to the
RFC/ADR/issue it serves) and, for non-trivial changes, the **Change model**
section -- affected components, touched Hard rules, assumptions, plan.
Reviewers read the model before the diff.

### Code Review

Review culture and the escalation path (write a proposal, don't argue) are
described in [engineering principles](engineering-principles.md) Section 4.
Practical note: add new commits instead of amending and force-pushing --
it keeps review threads readable; everything is squashed at merge anyway.

## Linting and Testing

All code quality checks run automatically via git hooks (prek) and can be run manually via Make commands.

### Quick Commands

```bash
# Run all linting checks
make lint

# Run all tests
make test

# Format code
make format

# Run pre-commit hooks manually
make pre-commit-run
```

**It's recommended to run these commands before each commit to avoid failed CI/CD checks.**

## Backend Linting

Python code is checked via [ruff] for linting and [mypy] for type checking.

### Using Ruff

**Linting check:**

```bash
# Check code for errors
cd services/backend
.venv/bin/ruff check .

# Automatically fix possible errors
.venv/bin/ruff check . --fix

# Check only specific file
.venv/bin/ruff check app/main.py
```

**Code formatting:**

```bash
# Format code
cd services/backend
.venv/bin/ruff format .

# Check formatting without changes
.venv/bin/ruff format --check .
```

**Via Make:**

```bash
make lint-backend      # Linting check
make format-backend    # Code formatting
```

**Ruff configuration:**

- Configuration is located in `services/backend/.ruff.toml`
- Ruff automatically ignores `.venv/`, `__pycache__/`, and `alembic/versions/` directories
- Maximum line length: 160 characters
- Use double quotes for strings

### Using Mypy

**Type checking:**

```bash
cd services/backend
.venv/bin/mypy app
```

**Via Make:**

```bash
make type-check
```

**Mypy configuration:**

- Configuration is located in `services/backend/pyproject.toml` in `[tool.mypy]` section
- Mypy checks only code in `app/` directory
- Strict mode is used for better type checking

**Common errors and solutions:**

- `Missing type annotation` - Add type hints to functions and variables
- `Incompatible types` - Check argument and return value types
- `Unused "type: ignore" comment` - Remove unnecessary ignore comments

## Frontend Linting

Frontend code is checked via [ESLint] for linting and [Prettier] for formatting.

### Using ESLint

**Linting check:**

```bash
cd services/frontend
npm run lint

# Automatically fix possible errors
npm run lint:fix
```

**Via Make:**

```bash
make lint-frontend
```

**ESLint configuration:**

- Configuration is located in `services/frontend/eslint.config.js`
- Recommended rules for React are used
- Additional rules for better code quality

### Using Prettier

**Code formatting:**

```bash
cd services/frontend
npm run format

# Check formatting without changes
npm run format:check
```

**Via Make:**

```bash
make format-frontend
```

**Prettier configuration:**

- Configuration is located in `services/frontend/.prettierrc.json`
- Prettier is integrated with ESLint via `eslint-config-prettier`

## Infrastructure Linting

Infrastructure files (YAML, Dockerfiles, GitHub Actions workflows) are checked via [yamllint] and [hadolint].

### Running Infrastructure Checks

```bash
# Check all infrastructure files
make lint-infra
```

This command checks:

- YAML files (docker-compose, observability configs)
- GitHub Actions workflows (`.github/workflows/*.yml`)
- Renovate configuration (`.github/renovate.json5`)
- Docker Compose validation
- Dockerfiles via hadolint

**Individual checks:**

**YAML linting:**

```bash
# Install yamllint if not already installed
pip install yamllint

# Check specific file
yamllint -c .yamllint.yml deploy/compose/docker-compose.yml
```

**Docker Compose validation:**

```bash
# Check deploy/compose/docker-compose.yml syntax
docker compose -f deploy/compose/docker-compose.yml --project-directory . config --quiet
```

**Dockerfile linting:**

```bash
# Install hadolint if not already installed
# macOS: brew install hadolint
# Linux: https://github.com/hadolint/hadolint#install

# Check Dockerfile
hadolint --config .hadolint.yaml services/backend/Dockerfile
```

## Git Hooks Setup

Git hooks run linting and formatting checks before each commit. The runner
is [prek](https://github.com/j178/prek) (single binary, pre-commit
compatible); classic [pre-commit](https://pre-commit.com/) works as a
fallback with the same config. CI runs the identical tool and config --
hooks cannot drift between local and CI (see [CI/CD architecture](ci.md)).

### Installation

```bash
# Install git hooks (uses prek when available)
make pre-commit-install
```

### Manual Run

```bash
# Run on all files
make pre-commit-run
```

### Configured Hooks

The authoritative list is [.pre-commit-config.yaml](../.pre-commit-config.yaml)
-- a list copied here would only drift. Highlights: whitespace/format
fixers, YAML/JSON/TOML checks, compose validation, hadolint, markdownlint,
ASCII-only docs linter, shellcheck/shfmt, gitleaks secret scanning, and the
toolchain drift gate.

### Updating Hooks

Hook versions are dependencies: Renovate updates them automatically
(grouped as `git-hooks`). Manual bump if needed:

```bash
make pre-commit-update
```

## Code Style

### Python

**Main principles:**

- Follow [PEP 8](https://pep8.org/) style guide
- Use type hints where possible
- Maximum line length: 160 characters (configured in ruff)
- Use double quotes for strings
- Follow naming conventions (snake_case for functions and variables, PascalCase for classes)

**Examples:**

```python
# Good
def get_user_by_id(user_id: int) -> User | None:
    """Get user by ID."""
    return db.query(User).filter(User.id == user_id).first()

# Bad
def getUser(id):
    return db.query(User).filter(User.id==id).first()
```

**Type hints:**

```python
# Required type hints for functions
async def create_item(item_data: ItemCreate, db: AsyncSession) -> Item:
    """Create new item."""
    db_item = Item(**item_data.model_dump())
    db.add(db_item)
    await db.commit()
    await db.refresh(db_item)
    return db_item
```

**Docstrings:**

```python
def complex_function(param1: str, param2: int) -> bool:
    """
    Short function description.

    Detailed description of what the function does and what parameters it takes.

    Args:
        param1: Parameter 1 description
        param2: Parameter 2 description

    Returns:
        Return value description

    Raises:
        ValueError: When parameters are invalid
    """
    pass
```

### JavaScript/React

**Main principles:**

- Follow ESLint recommended rules
- Use functional components with hooks
- Prefer named exports
- Use Prettier for formatting
- Follow naming conventions (camelCase for functions and variables, PascalCase for components)

**Examples:**

```javascript
// Good
import { useState, useEffect } from 'react';

export function ItemList() {
  const [items, setItems] = useState([]);

  useEffect(() => {
    fetchItems().then(setItems);
  }, []);

  return <div>{/* ... */}</div>;
}

// Bad
import React from 'react';

export default class ItemList extends React.Component {
  // ...
}
```

**Hooks:**

```javascript
// Use hooks instead of class components
function MyComponent() {
  const [state, setState] = useState(initialValue);

  useEffect(() => {
    // Side effects
    return () => {
      // Cleanup
    };
  }, [dependencies]);

  const handleClick = useCallback(() => {
    // Event handler
  }, [dependencies]);

  return <div onClick={handleClick}>Content</div>;
}
```

### YAML

**Main principles:**

- Use 2 spaces for indentation
- Follow yamllint configuration
- Use quotes for values containing special characters
- Add comments to explain complex configurations

**Examples:**

```yaml
# Good
services:
  api:
    image: python:3.12
    environment:
      - DATABASE_URL=postgresql://localhost/db
    ports:
      - "8000:8000"

# Bad
services:
api:
image:python:3.12
environment:-DATABASE_URL=postgresql://localhost/db
```

## Testing

### Writing Tests

**Backend tests:**

- Place tests in `services/backend/tests/` directory
- Use pytest fixtures for test environment setup
- Tests should be independent and not depend on execution order
- Use descriptive test names
- Tests should clean up after themselves

**Frontend tests:**

- Place tests next to components (`.test.jsx` files)
- Use React Testing Library for component testing
- Test behavior, not implementation
- Use user-event for simulating user interaction

**Examples:**

**Backend:**

```python
async def test_create_item(db: AsyncSession):
    """Test creating new item."""
    item_data = ItemCreate(name="Test Item")
    item = await create_item(item_data, db)

    assert item.id is not None
    assert item.name == "Test Item"
```

**Frontend:**

```javascript
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App } from './App';

test('creates new item', async () => {
  render(<App />);

  const input = screen.getByLabelText(/item name/i);
  await userEvent.type(input, 'New Item');

  const button = screen.getByRole('button', { name: /create/i });
  await userEvent.click(button);

  expect(screen.getByText('New Item')).toBeInTheDocument();
});
```

### Test Requirements

- All new features should include tests
- Tests should be independent and not depend on execution order
- Use descriptive test names
- Tests should clean up after themselves (don't leave "garbage" in database)
- Code coverage should be sufficient for critical functionality

Detailed information about running tests is available in [Running tests](running-tests.md).

## CI/CD

Pipeline topology, gates, releases, and branch protection are documented in
[CI/CD architecture](ci.md). The contract that matters while contributing:
`make ci` runs locally exactly what the pipeline runs -- green locally
means green in CI.

## Reference

### Configuration Files

- [pre-commit](../.pre-commit-config.yaml) - Pre-commit hooks configuration
- [ruff](../services/backend/.ruff.toml) - Ruff configuration
- [mypy](../services/backend/pyproject.toml#tool.mypy) - Mypy configuration
- [ESLint](../services/frontend/eslint.config.js) - ESLint configuration
- [Prettier](../services/frontend/.prettierrc.json) - Prettier configuration
- [yamllint](../.yamllint.yml) - yamllint configuration
- [hadolint](../.hadolint.yaml) - hadolint configuration

### Useful Links

- [Conventional Commits](https://www.conventionalcommits.org/) - Conventional Commits specification
- [pre-commit](https://pre-commit.com/) - pre-commit documentation
- [Ruff](https://pypi.org/project/ruff/) - Ruff documentation
- [Mypy](https://mypy.readthedocs.io/) - Mypy documentation
- [ESLint](https://eslint.org/) - ESLint documentation
- [Prettier](https://prettier.io/) - Prettier documentation
- [yamllint](https://yamllint.readthedocs.io/) - yamllint documentation
- [hadolint](https://github.com/hadolint/hadolint) - hadolint documentation

### Additional Resources

- [Local setup](local-setup.md) - Local environment setup
- [Running tests](running-tests.md) - Running tests
- [Prerequisites](prerequisites.md) - Required knowledge and skills
