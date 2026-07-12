# Architecture overview

The project is built on a microservices architecture with clear separation of responsibilities between components.

## System Components

- **Frontend** ([readme](../services/frontend/README.md))
  - React 18 with Vite as build tool
  - Web Vitals metrics for performance monitoring
  - Nginx for static hosting in production
  - Responsive UI with CRUD operations

- **Backend**
  - FastAPI (Python 3.12) with async support
  - SQLAlchemy with async driver (asyncpg)
  - Pydantic for data validation and serialization
  - Alembic for database migration management
  - Prometheus client for metrics collection

- **Database**
  - PostgreSQL 16 (Alpine-based image)
  - Automatic migrations on service startup
  - Health checks for state monitoring

- **Observability Stack**
  - **Prometheus** - Metrics collection and storage
  - **Grafana** - Metrics visualization and dashboards
  - **Loki** - Centralized log storage
  - **Grafana Alloy** - Log collection from containers (replacement for Promtail)
  - **Postgres Exporter** - PostgreSQL metrics

- **Infrastructure**
  - Docker and Docker Compose for orchestration
  - Multi-stage Dockerfile for image size optimization
  - Health checks for all critical services
  - Network isolation through Docker networks

## Technology Stack

| Component          | Technology    |
|--------------------|---------------|
| Backend Runtime    | Python        |
| Web Framework      | FastAPI       |
| ORM                | SQLAlchemy    |
| Database           | PostgreSQL    |
| Frontend Framework | React         |
| Build Tool         | Vite          |
| Metrics            | Prometheus    |
| Visualization      | Grafana       |
| Logs               | Loki          |
| Log Collector      | Grafana Alloy |

### Additional resources

- **Backend:**
  - [FastAPI Documentation](https://fastapi.tiangolo.com/) - Official FastAPI documentation
  - [SQLAlchemy Documentation](https://docs.sqlalchemy.org/) - SQLAlchemy ORM documentation
  - [Alembic Documentation](https://alembic.sqlalchemy.org/) - Database migration documentation
  - [Pydantic Documentation](https://docs.pydantic.dev/) - Data validation documentation

- **Frontend:**
  - [React Documentation](https://react.dev/) - Official React documentation
  - [Vite Documentation](https://vitejs.dev/) - Vite build tool documentation

- **Observability:**
  - [Prometheus Documentation](https://prometheus.io/docs/) - Prometheus documentation
  - [Grafana Documentation](https://grafana.com/docs/) - Grafana documentation
  - [Loki Documentation](https://grafana.com/docs/loki/latest/) - Loki documentation
  - [Grafana Alloy Documentation](https://grafana.com/docs/alloy/latest/) - Grafana Alloy documentation

- **Infrastructure:**
  - [Docker Documentation](https://docs.docker.com/) - Docker documentation
  - [Docker Compose Documentation](https://docs.docker.com/compose/) - Docker Compose documentation
  - [PostgreSQL Documentation](https://www.postgresql.org/docs/) - PostgreSQL documentation
