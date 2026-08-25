# ============================================================================
# Multi-stage Dockerfile for production
# Optimized for minimal size and maximum security
# ============================================================================

# ----------------------------------------------------------------------------
# Stage 1: Builder - generate gRPC stubs, install dependencies and compile
# ----------------------------------------------------------------------------
FROM python:3.12.13-slim@sha256:57cd7c3a7a273101a6485ba99423ee568157882804b1124b4dd04266317710de AS builder

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

# proto/ lives at the repo root, one level above this service's own build
# context (`services/backend`) -- reached here as a named additional build
# context (compose: additional_contexts; plain `docker build`:
# --build-context proto=proto), not by widening the primary context.
# DL3022 expects --from to reference a FROM alias in this Dockerfile;
# "proto" is an externally supplied named build context instead (hadolint
# has no way to see that from static analysis of this file alone).
# hadolint ignore=DL3022
COPY --from=proto . /build/proto

# 1. Install with dev extras (grpcio-tools, for codegen below) -- this
#    layer is discarded with the builder stage, dev tooling never reaches
#    the runtime image.
# 2. Generate gRPC/protobuf Python stubs (RFC-0001 D8, ADR-0002; RFC text:
#    "Python via grpcio-tools at build") -- the exact command `make
#    generate` runs at the repo root, just rooted at /build instead.
#    Never committed -- proto/ is the only source of truth. grpcio-tools
#    bundles its own protoc, so this needs no separate protoc/buf install
#    in this stage (buf itself only governs lint/format/breaking in CI,
#    not Python codegen -- an earlier buf-generate-based recipe for
#    python_out/pyi_out looked like it worked locally but only because a
#    system protoc happened to be on PATH; it fails in a clean container
#    like this one, which is how this got caught).
# 3. Re-install runtime-only dependencies (no dev extras, no grpcio-tools)
#    as the --user install actually copied into the runtime stage below.
#    --ignore-installed: without it pip sees every dependency already
#    satisfied by the `.[dev]` install in step 1 (global site-packages)
#    and installs nothing into the user site, leaving the runtime image
#    missing grpc, fastapi, etc.
RUN pip install --upgrade pip setuptools wheel && \
    pip install --no-warn-script-location ".[dev]" && \
    mkdir -p app/proto_gen && \
    python -m grpc_tools.protoc -I proto \
        --python_out=app/proto_gen --pyi_out=app/proto_gen --grpc_python_out=app/proto_gen \
        proto/devopsdemo/items/v1/items.proto && \
    pip install --user --ignore-installed --no-warn-script-location .

# ----------------------------------------------------------------------------
# Stage 2: Runtime - minimal production image
# ----------------------------------------------------------------------------
FROM python:3.12.13-slim@sha256:57cd7c3a7a273101a6485ba99423ee568157882804b1124b4dd04266317710de AS runtime

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

# Copy application code -- from the builder stage, not the build context:
# only the builder has app/proto_gen (generated there, never committed --
# RFC-0001 D8, ADR-0002)
COPY --from=builder --chown=appuser:appuser /build/app /app/app
COPY --chown=appuser:appuser alembic.ini /app/alembic.ini
COPY --chown=appuser:appuser alembic /app/alembic
# Copy pyproject.toml for ability to install dev dependencies (for tests)
COPY --chown=appuser:appuser pyproject.toml /app/pyproject.toml

# Set PATH for non-root user (to find installed packages)
ENV PATH=/home/appuser/.local/bin:$PATH

# Switch to non-root user (security best practice)
USER 1000

# Health check for Docker/Kubernetes: HTTP /healthz liveness AND the gRPC
# Health Checking Protocol on :50051 -- both must pass (RFC-0001 D6)
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD ["python", "-m", "app.healthcheck"]

# Expose ports: HTTP API + gRPC ItemService (RFC-0001 D3)
EXPOSE 8000 50051

# --workers 1: the item-event broadcaster (app.events) and the grpc.aio
# server it feeds are per-process state (RFC-0001 D3 emit-after-commit).
# With more than one uvicorn worker, each forked process would run its own
# broadcaster and its own gRPC server sharing :50051 via SO_REUSEPORT --
# a WatchItemEvents subscriber would only ever see the commits made by
# whichever worker it happened to land on, silently dropping most events
# instead of the documented at-most-once-when-disconnected gap.
# --access-log for request logging
# --log-level info for production logging
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "1", "--access-log", "--log-level", "info"]
