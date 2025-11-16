# Prerequisites

This document describes the knowledge and skills necessary to understand and work with the DevOps Demo project. It serves as a guide for assessing readiness to work with the project and provides links to learning resources.

## Table of Contents

- [Backend](#backend)
- [Frontend](#frontend)
- [Infrastructure](#infrastructure)
- [Recommended Learning Resources](#recommended-learning-resources)

## Backend

### Backend Programming Languages and Versions

- **Python 3.12**
  - Understanding Python 3.12 syntax
  - Type hints and annotations for static typing
  - Async/await syntax for asynchronous programming
  - Context managers for resource management
  - Decorators for extending function and class functionality
  - List comprehensions and generator expressions
  - Exception handling and error handling patterns

- **Key concepts:**
  - Object-oriented programming (classes, inheritance, polymorphism)
  - Functional programming (lambda, map, filter, reduce)
  - Modules and packages (import, `__init__.py`)
  - Virtual environments (venv, virtualenv)

### Backend Frameworks and Libraries

- **FastAPI**
  - Creating REST API endpoints
  - Dependency injection via Depends()
  - Request/Response models with Pydantic
  - Middleware for request processing
  - Automatic API documentation (Swagger/OpenAPI)
  - Background tasks and WebSockets (if needed)
  - CORS configuration
  - Security features (OAuth2, JWT tokens)

- **SQLAlchemy (async extension)**
  - AsyncSession and async operations for asynchronous database work
  - Declarative models for defining data models
  - Query building via SQLAlchemy Core and ORM
  - Relationships and foreign keys for table connections
  - Connection pooling for connection optimization
  - Transactions and session management
  - Eager loading and lazy loading strategies

- **Alembic**
  - Creating migrations via autogenerate
  - Applying migrations (upgrade/downgrade)
  - Database schema versioning
  - Manual migration writing for complex changes
  - Data migrations for data migration
  - Migration conflicts and their resolution

- **Pydantic**
  - Schema definition for data validation
  - Data validation and type coercion
  - JSON serialization/deserialization
  - Custom validators and field validators
  - Model configuration and advanced features

- **asyncpg**
  - Asynchronous driver for PostgreSQL
  - Connection management and connection pooling
  - Query execution and prepared statements
  - Transactions and error handling
  - Performance optimization

- **psycopg**
  - Synchronous driver for PostgreSQL (used by Alembic)
  - Connection management
  - Query execution
  - Transactions

### Backend Testing

- **pytest**
  - Test structure and test discovery
  - Fixtures for test environment setup
  - Parametrization for running tests with different parameters
  - Assertions and assertion rewriting
  - Markers for test categorization
  - Plugins and extensions

- **pytest-asyncio**
  - Async test support
  - Async fixtures for asynchronous setup
  - Event loop management
  - Async test functions and async context managers

- **httpx**
  - HTTP client for API testing
  - ASGITransport for testing FastAPI applications
  - AsyncClient for asynchronous HTTP requests
  - TestClient for synchronous testing
  - Mocking and fixtures for HTTP client

### Backend Code Quality

- **Ruff**
  - Fast linter and formatter for Python
  - Linting rules (pycodestyle, pyflakes, isort, etc.)
  - Code formatting in Black style
  - Import sorting and organization
  - Auto-fixing possible issues
  - Configuration via TOML files

- **Mypy**
  - Static type checker for Python
  - Type checking and type inference
  - Type annotations and generics
  - Configuration via pyproject.toml
  - Strict mode and gradual typing
  - Type stubs for third-party libraries

- **pre-commit**
  - Git hooks for automatic checks
  - Hook configuration via YAML
  - Pre-commit hooks setup and management
  - Integration with various tools

### Database

- **PostgreSQL 16**
  - SQL syntax and DDL commands
  - Transactions and ACID properties
  - Constraints (UNIQUE, PRIMARY KEY, FOREIGN KEY, CHECK)
  - Indexes for query optimization
  - Connection strings and URL formats
  - Database design and normalization
  - Query optimization and EXPLAIN ANALYZE

- **Key concepts:**
  - Relational database model
  - Tables, columns, rows
  - Joins (INNER, LEFT, RIGHT, FULL)
  - Aggregations (GROUP BY, HAVING)
  - Subqueries and CTEs (Common Table Expressions)
  - Views and materialized views

### Other Backend Concepts

- **REST API**
  - Architectural style for APIs
  - HTTP methods (GET, POST, PUT, PATCH, DELETE)
  - Status codes and their meanings
  - Resource naming conventions
  - HATEOAS (Hypermedia as the Engine of Application State)
  - API versioning strategies

- **Async programming**
  - Asynchronous programming in Python
  - Event loops and asyncio
  - Coroutines and async/await
  - Async context managers
  - Async generators
  - Concurrent execution and tasks

- **Dependency Injection**
  - Design pattern for dependency management
  - FastAPI Depends() for dependency injection
  - Dependency chains and sub-dependencies
  - Overriding dependencies for testing

- **Environment variables**
  - Configuration management via environment variables
  - Environment-specific settings (development, staging, production)
  - .env files and their usage
  - Security best practices for secrets

## Frontend

### Frontend Programming Languages

- **JavaScript (ES2020+)**
  - ES6+ features (arrow functions, destructuring, spread operator)
  - Async/await for asynchronous programming
  - Promises and Promise handling
  - Modules (import/export) for modular architecture
  - Template literals for strings
  - Optional chaining and nullish coalescing
  - Array methods (map, filter, reduce, etc.)

- **JSX**
  - Syntax for React components
  - JSX expressions and embedding JavaScript
  - Conditional rendering
  - Lists and keys
  - Event handling in JSX
  - JSX vs HTML differences

### Frontend Frameworks and Libraries

- **React 19**
  - Functional components instead of class components
  - Hooks (useState, useEffect, useCallback, useMemo, useContext, useRef)
  - Component lifecycle and side effects
  - Props and state management
  - Event handling and synthetic events
  - Component composition and reusability
  - Performance optimization (memo, useMemo, useCallback)
  - Context API for global state
  - Error boundaries for error handling

- **Vite**
  - Build tool and dev server for fast development
  - Development server with Hot Module Replacement (HMR)
  - Build configuration and optimization
  - Environment variables (VITE_* prefix)
  - Plugin system and ecosystem
  - Production builds and code splitting

### Frontend Testing

- **Vitest**
  - Test framework based on Vite
  - Test structure and organization
  - Assertions and matchers
  - Mocking (vi.mock, vi.fn, vi.spyOn)
  - Test configuration via vite.config.js
  - Watch mode for fast development
  - Coverage reports

- **React Testing Library**
  - Utilities for testing React components
  - render() for rendering components
  - screen queries (getByText, getByRole, getByLabelText, etc.)
  - user-event for simulating user interaction
  - waitFor for asynchronous operations
  - act() for wrapping state updates
  - Testing best practices (testing behavior, not implementation)

- **@testing-library/jest-dom**
  - Additional matchers for DOM testing
  - toBeInTheDocument(), toHaveClass(), toHaveTextContent(), etc.
  - Custom matchers for better testing

- **jsdom**
  - DOM implementation for Node.js (for tests)
  - Browser environment emulation
  - Configuration and limitations

### Frontend Code Quality

- **ESLint**
  - Linter for JavaScript/React
  - ESLint rules and configuration
  - React-specific rules (eslint-plugin-react)
  - Integration with Prettier
  - Custom rules and plugins

- **Prettier**
  - Code formatter for JavaScript/React
  - Formatting rules and configuration
  - Integration with ESLint
  - Editor integration

### Frontend Web APIs

- **Fetch API**
  - HTTP requests via fetch()
  - GET, POST, DELETE requests
  - Request options (headers, body, method)
  - Response handling and error handling
  - Async/await with fetch
  - Request/Response objects

- **Web Vitals API**
  - Web application performance metrics
  - onCLS (Cumulative Layout Shift)
  - onINP (Interaction to Next Paint)
  - onLCP (Largest Contentful Paint)
  - onFCP (First Contentful Paint)
  - onTTFB (Time to First Byte)
  - Performance measurement and reporting

### Other Frontend Concepts

- **HTTP**
  - Protocol for communication between client and server
  - Request/Response cycle
  - HTTP methods and their semantics
  - Status codes and their meanings
  - Headers and their usage
  - CORS (Cross-Origin Resource Sharing)

- **REST API**
  - Architectural style for APIs
  - API endpoints and resource naming
  - CRUD operations (Create, Read, Update, Delete)
  - API design best practices

- **CSS**
  - Web application styling
  - CSS selectors and specificity
  - Flexbox and Grid for layout
  - Responsive design and media queries
  - CSS variables (custom properties)
  - CSS modules and scoped styles

- **npm**
  - Package manager for Node.js
  - package.json and package-lock.json
  - npm install and dependency management
  - npm scripts for automation
  - Semantic versioning
  - npm registry and packages

## Infrastructure

### Containerization

- **Docker**
  - Platform for application containerization
  - Dockerfile syntax and best practices
  - Multi-stage builds for image optimization
  - Layer caching for faster builds
  - Build context and .dockerignore
  - Image optimization and size reduction
  - Security best practices for Docker images

- **Docker Compose**
  - Container orchestration for local development
  - docker-compose.yml syntax and structure
  - Service definition and configuration
  - Networks for communication between containers
  - Volumes for data storage
  - Environment variables and secrets
  - Health checks for service state monitoring
  - Dependencies between services

### Observability

- **Prometheus**
  - Monitoring and metrics collection system
  - Prometheus metrics format (text-based)
  - Metric types (Counter, Gauge, Histogram, Summary)
  - Scrape configuration and service discovery
  - PromQL (Prometheus Query Language) for queries
  - Alerting rules and alertmanager
  - Recording rules for query optimization

- **Grafana**
  - Platform for metrics and logs visualization
  - Dashboards and panels
  - Data sources (Prometheus, Loki, etc.)
  - Visualizations (graphs, tables, gauges, etc.)
  - Provisioning for automatic configuration
  - Alerting and notifications
  - Dashboard sharing and templating

- **Loki**
  - Log storage system (Grafana Loki)
  - Log aggregation and storage
  - LogQL (Loki Query Language) for queries
  - Label-based indexing for fast search
  - Retention policies and data management
  - Integration with Prometheus and Grafana

- **Grafana Alloy**
  - Agent for log and metrics collection (replacement for Promtail)
  - Log collection from Docker containers
  - Label extraction and transformation
  - Ship configuration for sending to Loki
  - Metrics collection and forwarding
  - Service discovery for automatic service detection

- **Postgres Exporter**
  - PostgreSQL metrics exporter for Prometheus
  - Database metrics collection
  - Prometheus format for metrics
  - Configuration and connection settings
  - Integration with Prometheus

### CI/CD

- **GitHub Actions**
  - Platform for CI/CD automation
  - Workflow syntax (YAML)
  - Jobs and steps for pipeline definition
  - Actions and reusable workflows
  - Environment variables and secrets management
  - Matrix strategies for testing on different versions
  - Service containers for testing with dependencies
  - Caching for build optimization
  - Artifacts for storing build results

- **Dependabot**
  - Automatic dependency updates
  - Configuration via YAML
  - Package ecosystems (pip, npm, docker, github-actions)
  - Update schedules and frequency
  - Security updates and vulnerability scanning
  - Pull request management

### Code Quality (Infrastructure)

- **yamllint**
  - Linter for YAML files
  - YAML syntax validation
  - Style checking (indentation, line length, etc.)
  - Configuration via .yamllint.yml
  - Integration with pre-commit

- **hadolint**
  - Linter for Dockerfiles
  - Dockerfile best practices checking
  - Security checks for Docker images
  - Configuration via .hadolint.yaml
  - Integration with CI/CD

- **pre-commit**
  - Git hooks for automatic checks
  - Hook configuration for infrastructure files
  - Local hooks for quick checks
  - Integration with various tools

### Build Tools

- **Make**
  - Utility for task automation
  - Makefile syntax and structure
  - Targets and dependencies
  - Variables and functions
  - Phony targets for non-file goals
  - Pattern rules for reuse
  - Conditional execution

### Other Concepts

- **YAML**
  - Data serialization format
  - YAML syntax and structure
  - Indentation and formatting
  - Lists and dictionaries
  - Multi-line strings and anchors
  - YAML best practices

- **nginx**
  - Web server and reverse proxy
  - Configuration files and directives
  - Static file serving
  - Reverse proxy setup for API
  - Location blocks and routing
  - SSL/TLS configuration

- **Git**
  - Version control system
  - Basic Git commands (add, commit, push, pull)
  - Branches and branch management
  - Pull requests and code review
  - Conventional commits for structured commits
  - Git workflows (GitFlow, GitHub Flow)

- **Environment Variables**
  - Configuration management via environment variables
  - Docker Compose environment variables
  - CI/CD environment variables
  - Security best practices for secrets
  - .env files and their usage

- **Health Checks**
  - Service health checking
  - HTTP health endpoints
  - Docker health checks
  - Database health checks (pg_isready)
  - Health check best practices

- **Networking**
  - Network concepts for containers
  - Ports and port mapping
  - Docker networks for isolation
  - Service discovery in Docker
  - DNS resolution in Docker
  - Network policies and security

- **Volumes**
  - Data storage in Docker
  - Docker volumes for persistent storage
  - Named volumes vs anonymous volumes
  - Volume mounting and bind mounts
  - Volume management and cleanup

## Recommended Learning Resources

- **Backend**
  - **Python and AsyncIO:**
    - [Python 3.12 Documentation](https://docs.python.org/3.12/) - Official Python documentation
    - [Python AsyncIO Documentation](https://docs.python.org/3/library/asyncio.html) - asyncio documentation
    - [Real Python](https://realpython.com/) - Python learning materials
  - **FastAPI:**
    - [FastAPI Documentation](https://fastapi.tiangolo.com/) - Official FastAPI documentation
    - [FastAPI Tutorial](https://fastapi.tiangolo.com/tutorial/) - Step-by-step tutorial
  - **SQLAlchemy:**
    - [SQLAlchemy Documentation](https://docs.sqlalchemy.org/) - Official SQLAlchemy documentation
    - [SQLAlchemy Async Documentation](https://docs.sqlalchemy.org/en/20/orm/extensions/asyncio.html) - Async extension
  - **Alembic:**
    - [Alembic Documentation](https://alembic.sqlalchemy.org/) - Official Alembic documentation
  - **Testing:**
    - [pytest Documentation](https://docs.pytest.org/) - Official pytest documentation
    - [pytest-asyncio Documentation](https://pytest-asyncio.readthedocs.io/) - Async testing
  - **Code Quality:**
    - [Ruff Documentation](https://docs.astral.sh/ruff/) - Official Ruff documentation
    - [Mypy Documentation](https://mypy.readthedocs.io/) - Official Mypy documentation

- **Frontend**
  - **React:**
    - [React Documentation](https://react.dev/) - Official React documentation
    - [React Beta Documentation](https://beta.react.dev/) - New React documentation
  - **Vite:**
    - [Vite Documentation](https://vitejs.dev/) - Official Vite documentation
  - **Testing:**
    - [Vitest Documentation](https://vitest.dev/) - Official Vitest documentation
    - [React Testing Library Documentation](https://testing-library.com/docs/react-testing-library/intro/) - React Testing Library documentation
  - **Web Vitals:**
    - [Web Vitals](https://web.dev/vitals/) - Web Vitals metrics documentation

- **Infrastructure**
  - **Docker:**
    - [Docker Documentation](https://docs.docker.com/) - Official Docker documentation
    - [Docker Compose Documentation](https://docs.docker.com/compose/) - Docker Compose documentation
    - [Docker Best Practices](https://docs.docker.com/develop/dev-best-practices/) - Best practices
  - **Observability:**
    - [Prometheus Documentation](https://prometheus.io/docs/) - Official Prometheus documentation
    - [Grafana Documentation](https://grafana.com/docs/) - Official Grafana documentation
    - [Loki Documentation](https://grafana.com/docs/loki/latest/) - Loki documentation
    - [Grafana Alloy Documentation](https://grafana.com/docs/alloy/latest/) - Grafana Alloy documentation
  - **CI/CD:**
    - [GitHub Actions Documentation](https://docs.github.com/en/actions) - Official GitHub Actions documentation
    - [Dependabot Documentation](https://docs.github.com/en/code-security/dependabot) - Dependabot documentation
  - **PostgreSQL:**
    - [PostgreSQL Documentation](https://www.postgresql.org/docs/) - Official PostgreSQL documentation

- **General Resources**
  - **DevOps:**
    - [DevOps Roadmap](https://roadmap.sh/devops) - Roadmap for DevOps engineers
    - [Awesome DevOps](https://github.com/bregman-arie/awesome-devops) - DevOps resources collection
  - **Best Practices:**
    - [12 Factor App](https://12factor.net/) - Methodology for SaaS applications
    - [Clean Code](https://www.amazon.com/Clean-Code-Handbook-Software-Craftsmanship/dp/0132350882) - Book about clean code
  - **Git:**
    - [Pro Git Book](https://git-scm.com/book) - Book about Git
    - [GitHub Guides](https://guides.github.com/) - GitHub guides
