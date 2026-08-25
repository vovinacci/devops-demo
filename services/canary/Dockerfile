# ============================================================================
# Multi-stage Dockerfile for the Rust canary (RFC-0001 D6, D9)
# cargo-chef layer caching: dependencies are compiled in their own layer
# and only invalidated when Cargo.toml/Cargo.lock actually change, not on
# every source edit.
# ============================================================================

# ----------------------------------------------------------------------------
# Stage 1: chef -- shared base for the planner and builder stages
# ----------------------------------------------------------------------------
FROM rust:1.97-slim-bookworm@sha256:99e09cb2284e2ddbb73a995deee3e91783fd04d177602ccf6eab326d778ee777 AS chef
WORKDIR /build
RUN cargo install cargo-chef --locked

# ----------------------------------------------------------------------------
# Stage 2: planner -- compute the dependency recipe from Cargo.toml/lock
# ----------------------------------------------------------------------------
FROM chef AS planner
COPY . .
RUN cargo chef prepare --recipe-path recipe.json

# ----------------------------------------------------------------------------
# Stage 3: builder -- cook (cache) dependencies, then build the binary
# ----------------------------------------------------------------------------
FROM chef AS builder
COPY --from=planner /build/recipe.json recipe.json
# Dependency layer: rebuilt only when the recipe (Cargo.toml/lock) changes.
RUN cargo chef cook --release --recipe-path recipe.json
COPY . .
RUN cargo build --release --bin canary

# ----------------------------------------------------------------------------
# Stage 4: runtime -- minimal image, no toolchain
# ----------------------------------------------------------------------------
FROM debian:bookworm-slim@sha256:7b140f374b289a7c2befc338f42ebe6441b7ea838a042bbd5acbfca6ec875818 AS runtime

LABEL maintainer="DevOps Demo Team" \
      version="0.1.0" \
      description="DevOps Demo synthetic canary (Rust)" \
      org.opencontainers.image.title="devops-demo-canary" \
      org.opencontainers.image.version="0.1.0" \
      org.opencontainers.image.description="Rust synthetic-journey canary for the three-layer monitoring stack" \
      org.opencontainers.image.source="https://github.com/vovinacci/devops-demo"

# wget: debian-slim runtime chosen over distroless specifically so the
# container healthcheck below (and the compose healthcheck) can exec it --
# a distroless image ships no shell and no HTTP client (see the loki
# service comment in deploy/compose/docker-compose.yml for the tradeoff
# this avoids).
RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd -r appuser && \
    useradd -r -g appuser -u 1000 appuser

COPY --from=builder --chown=appuser:appuser /build/target/release/canary /usr/local/bin/canary

USER 1000

HEALTHCHECK --interval=10s --timeout=5s --start-period=10s --retries=3 \
    CMD ["sh", "-c", "wget -q -O /dev/null http://localhost:8080/healthz || exit 1"]

EXPOSE 8080

CMD ["/usr/local/bin/canary"]
