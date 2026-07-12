# DevOps Demo

A complete DevOps project example: from code to a working service with tests, containerization, deployment, and basic observability.
The project demonstrates modern development and DevOps practices, including test automation, monitoring, logging, and metrics visualization.

The project structure is as follows:

```text
devops-demo/
├── docs/                        # Documentation
│
├── services/                    # Self-contained services
│   ├── backend/                 # Backend service
│   │   ├── app/                 # FastAPI application
│   │   ├── alembic/             # Database migrations
│   │   ├── tests/               # Backend tests
│   │   ├── Dockerfile           # Production Dockerfile
│   │   └── pyproject.toml       # Python dependencies and configuration
│   │
│   └── frontend/                # Frontend application
│       ├── src/                 # React components
│       ├── public/              # Static files
│       ├── Dockerfile           # Production Dockerfile
│       ├── package.json         # Node.js dependencies
│       └── vite.config.js       # Vite configuration
│
├── observability/               # Observability configuration
│   ├── prometheus.yml           # Prometheus configuration
│   ├── prometheus_slo_rules.yml # SLO rules
│   ├── grafana/                 # Grafana configuration
│   │   ├── dashboards/          # JSON dashboards
│   │   ├── provisioning/        # Automatic configuration
│   │   └── grafana.ini          # Grafana settings
│   ├── loki/                    # Loki configuration
│   └── alloy/                   # Grafana Alloy configuration
│
├── deploy/                      # Deployment configuration
│   └── compose/
│       └── docker-compose.yml   # Main Docker Compose configuration
│
├── Makefile                     # Convenience commands
└── README.md                    # This file
```

The project contains detailed documentation for various aspects of development and usage:

- [Prerequisites](docs/prerequisites.md) - Required knowledge and skills for working with the project, recommended learning resources
- [Local setup](docs/local-setup.md) - Local development environment setup, dependency installation, Docker configuration, and troubleshooting common issues
- [Running tests](docs/running-tests.md) - Detailed guide on running tests, including unit and integration tests for backend and frontend
- [Contributing](docs/contributing.md) - Contribution guidelines, code standards, code review process, and best practices

## Quick Start

### System Requirements

- Docker version 24 or newer
- Docker Compose version 2.24 or newer
- Make version 3.81 or newer
- Minimum 2GB free RAM
- Minimum 3GB free disk space

### Running the Project

- Start everything

  ```shell
  make up
  ```

- Seed initial data (could be done multiple times)

  ```shell
  make seed
  ```

After successful startup, all services will be available at the following URLs:

| Service               | URL                         | Credentials | Description                                              |
|-----------------------|-----------------------------|-------------|----------------------------------------------------------|
| **Frontend**          | http://localhost:8080       | -           | React application with CRUD interface for managing items |
| **API**               | http://localhost:8000       | -           | FastAPI REST API server                                  |
| **API Documentation** | http://localhost:8000/docs  | -           | Swagger UI with interactive API documentation            |
| **ReDoc**             | http://localhost:8000/redoc | -           | Alternative API documentation in ReDoc format            |
| **Grafana**           | http://localhost:3000       | admin/admin | Dashboards for metrics and logs visualization            |
| **Prometheus**        | http://localhost:9090       | -           | UI for viewing and querying metrics                      |
| **Loki**              | http://localhost:3100       | -           | API for accessing logs                                   |
| **Postgres Exporter** | http://localhost:9187       | -           | PostgreSQL metrics in Prometheus format                  |
| **cAdvisor**          | http://localhost:8081       | -           | Container and resource metrics                           |

**Notes**:

- On first login to Grafana, the system will prompt you to change the administrator password. For development environments, this can be skipped.
- Detailed interactive API documentation is available via Swagger UI at http://localhost:8000/docs or via ReDoc at http://localhost:8000/redoc.
- For a complete list of available convenience make targets, run:

  ```shell
  make help
  ```

For more detailed information, please refer to [local setup](./docs/local-setup.md).

## Architecture

See the [architecture overview](./docs/architecture.md) for detailed information.

## Observability

See the [observability overview](./docs/observability.md) for detailed information.

## Database and data seeding

See the [data overview](./docs/data.md) for detailed information.

## Troubleshooting

See the [troubleshooting guide](./docs/troubleshooting.md) for common issues and solutions.

### Usage in Educational Process

The project can be used for:

- Demonstrating DevOps practices with practical examples
- Practical classes on containerization and orchestration
- Learning observability and monitoring
- Practice with CI/CD and automation
- Understanding production-ready code and architecture

See [homework assignments](docs/homework.md) with various difficulty levels for students.

## License

This project is intended for educational purposes and licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
