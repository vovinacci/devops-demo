# ============================================================================
# Multi-stage Dockerfile for the Kotlin reports service (RFC-0001 D2, D6)
# ============================================================================

# ----------------------------------------------------------------------------
# Stage 1: builder -- compile a bootable jar with the committed Gradle wrapper
# (JDK version pinned in .mise.toml, enforced by scripts/check-toolchain-drift.sh)
# ----------------------------------------------------------------------------
FROM eclipse-temurin:21-jdk-jammy@sha256:9d8dcf999b0bce2453e913823595a5ff2a4e8e9e5d5241b45280d0ff069818ec AS builder
WORKDIR /build

# Dependency layer: copy only the build definition + wrapper first, resolve
# dependencies, then copy sources -- a source-only edit reuses the cached
# dependency download instead of re-fetching the JVM ecosystem every build.
COPY gradlew ./
COPY gradle ./gradle
COPY settings.gradle.kts build.gradle.kts gradle.lockfile ./
RUN ./gradlew --no-daemon dependencies >/dev/null 2>&1 || true

COPY src ./src
RUN ./gradlew --no-daemon clean bootJar

# ----------------------------------------------------------------------------
# Stage 2: runtime -- slim JRE (RFC-0001 D6)
# ----------------------------------------------------------------------------
FROM eclipse-temurin:21-jre-jammy@sha256:d63bd8d9b171999cbed8576f2c76e874dd4856791a358536e5c4d407e77edc13 AS runtime

LABEL maintainer="DevOps Demo Team" \
      version="0.1.0" \
      description="DevOps Demo Kotlin reports service" \
      org.opencontainers.image.title="devops-demo-reports" \
      org.opencontainers.image.version="0.1.0" \
      org.opencontainers.image.description="Kotlin/Spring Boot reports service (RFC-0001 D2 JVM showcase)" \
      org.opencontainers.image.source="https://github.com/vovinacci/devops-demo"

# wget for the container healthcheck below, same choice as the canary's
# debian-slim runtime: a JRE image ships no HTTP client, and a shell-exec
# probe against /healthz keeps the healthcheck consistent with the other
# compose services (analytics is the exception -- its distroless image
# carries a `healthcheck` subcommand instead).
RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd -r appuser && \
    useradd -g appuser -u 1000 appuser

# Artifact volume mount target (RFC-0001 D2): PR-2 writes generated reports
# here; created and owned by the runtime user now so the mount is writable.
RUN mkdir -p /var/lib/reports/artifacts && chown -R appuser:appuser /var/lib/reports

COPY --from=builder --chown=appuser:appuser /build/build/libs/reports-0.1.0.jar /app/reports.jar

USER appuser

# Container-aware heap sizing (RFC-0001 D2): the JVM reads cgroup limits and
# caps the heap at 75% of the container memory limit -- the deeper GC/heap
# tuning exhibit lands with the PR-2 dashboard.
ENV JAVA_OPTS="-XX:MaxRAMPercentage=75.0"
ENV REPORTS_ARTIFACT_DIR="/var/lib/reports/artifacts"

HEALTHCHECK --interval=10s --timeout=5s --start-period=40s --retries=3 \
    CMD wget -q -O /dev/null http://localhost:8083/healthz || exit 1

EXPOSE 8083

ENTRYPOINT ["sh", "-c", "exec java $JAVA_OPTS -jar /app/reports.jar"]
