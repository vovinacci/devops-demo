# ============================================================================
# Multi-stage Dockerfile for production
# Optimized for minimal size and maximum security
# ============================================================================

# ----------------------------------------------------------------------------
# Stage 1: Builder - install dependencies and compile
# ----------------------------------------------------------------------------
FROM python:3.12.13-slim@sha256:423ed6ab25b1921a477529254bfeeabf5855151dc2c3141699a1bfc852199fbf AS builder

# Install system dependencies for compiling Python packages
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    gcc \
    && rm -rf /var/lib/apt/lists/*

# Python configuration for builder
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DEFAULT_TIMEOUT=100

WORKDIR /build

# Copy dependency files (for better Docker layer caching)
# pip install . requires all project files, so we copy everything
# But this is still better than in runtime stage, because build tools are here
COPY pyproject.toml /build/
COPY app /build/app/
COPY alembic.ini /build/alembic.ini
COPY alembic /build/alembic/

# Install pip and project dependencies (production only, without dev dependencies)
# pip install . by default does not install optional-dependencies
# Use --user to install in home directory
RUN pip install --upgrade pip setuptools wheel && \
    pip install --user --no-warn-script-location .

# ----------------------------------------------------------------------------
# Stage 2: Runtime - minimal production image
# ----------------------------------------------------------------------------
FROM python:3.12.13-slim@sha256:423ed6ab25b1921a477529254bfeeabf5855151dc2c3141699a1bfc852199fbf AS runtime

# Image metadata (for production tracking)
LABEL maintainer="DevOps Demo Team" \
      version="0.1.0" \
      description="DevOps Demo Backend API" \
      org.opencontainers.image.title="devops-demo-backend" \
      org.opencontainers.image.version="0.1.0" \
      org.opencontainers.image.description="FastAPI backend for DevOps demonstration" \
      org.opencontainers.image.source="https://github.com/your-org/python-devops-demo"

# Python configuration for production
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONPATH=/app \
    PYTHONHASHSEED=random \
    PIP_NO_CACHE_DIR=1

# Create non-root user for security (security best practice)
RUN groupadd -r appuser && \
    useradd -r -g appuser -u 1000 appuser && \
    mkdir -p /app /app/data && \
    chown -R appuser:appuser /app

WORKDIR /app

# Copy installed dependencies from builder stage
# Use --chown for correct permissions
COPY --from=builder --chown=appuser:appuser /root/.local /home/appuser/.local

# Copy application code (separately for better caching)
COPY --chown=appuser:appuser app /app/app
COPY --chown=appuser:appuser alembic.ini /app/alembic.ini
COPY --chown=appuser:appuser alembic /app/alembic
# Copy pyproject.toml for ability to install dev dependencies (for tests)
COPY --chown=appuser:appuser pyproject.toml /app/pyproject.toml

# Set PATH for non-root user (to find installed packages)
ENV PATH=/home/appuser/.local/bin:$PATH

# Switch to non-root user (security best practice)
USER appuser

# Health check for Docker/Kubernetes
# Checks availability of /health endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')" || exit 1

# Expose port
EXPOSE 8000

# Production command with uvicorn workers
# --workers 4 to use all CPU cores
# --access-log for request logging
# --log-level info for production logging
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "4", "--access-log", "--log-level", "info"]
