# ============================================================================
# Multi-stage Dockerfile for the Go analytics service (RFC-0001 D6, D8)
# ============================================================================

# ----------------------------------------------------------------------------
# Stage 1: builder -- generate gRPC stubs (buf, RFC-0001 D8/ADR-0002), then
# compile a static binary
# ----------------------------------------------------------------------------
FROM golang:1.26.5-bookworm@sha256:1ecb7edf62a0408027bd5729dfd6b1b8766e578e8df93995b225dfd0944eb651 AS builder

# buf is pinned to the same version as .mise.toml. The Go protoc
# plugins are NOT installed here: buf.gen.yaml invokes them via
# `go tool`, so their versions come from this module's go.mod/go.sum
# (Renovate-managed, MVS-locked to the protobuf runtime) -- a drift
# between generator and runtime is structurally impossible.
RUN go install github.com/bufbuild/buf/cmd/buf@v1.72.0
ENV PATH=/root/go/bin:${PATH}

# proto/ lives at the repo root, one level above this service's own build
# context (`services/analytics`) -- reached here as a named additional
# build context (compose: additional_contexts; plain `docker build`:
# --build-context proto=proto), the same pattern as
# services/backend/Dockerfile. The repo-relative layout (proto/ and
# services/analytics/ as siblings) is recreated under /repo so
# proto/buf.gen.yaml's relative `out:` path resolves identically to a
# local `cd proto && buf generate` run.
# hadolint ignore=DL3022
COPY --from=proto . /repo/proto
COPY . /repo/services/analytics

WORKDIR /repo/proto
RUN buf generate

WORKDIR /repo/services/analytics
# CGO_ENABLED=0: a static binary with no libc dependency, required for the
# distroless/static runtime stage below.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/analytics ./cmd/analytics

# ----------------------------------------------------------------------------
# Stage 2: runtime -- distroless static (RFC-0001 D6)
# ----------------------------------------------------------------------------
FROM gcr.io/distroless/static:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6 AS runtime

LABEL maintainer="DevOps Demo Team" \
      version="0.1.0" \
      description="DevOps Demo Go analytics service" \
      org.opencontainers.image.title="devops-demo-analytics" \
      org.opencontainers.image.version="0.1.0" \
      org.opencontainers.image.description="Go analytics service: event ingestion, aggregation, retention (RFC-0001 D1)" \
      org.opencontainers.image.source="https://github.com/vovinacci/devops-demo"

COPY --from=builder /out/analytics /usr/local/bin/analytics

# loadprofile/ reached as a named build context (compose:
# additional_contexts; plain `docker build`: --build-context
# loadprofile=./loadprofile), the same pattern loadgen/Dockerfile uses
# for the same directory (RFC-0001 D8, ADR-0002). Copied straight into
# the runtime stage, not the builder: `analytics seed` (RFC-0001 Phase 5
# D5) reads profile.json at container RUNTIME, not at build time -- no
# codegen or compilation step touches it. ANALYTICS_LOADPROFILE_PATH
# (compose) points here; the binary's own default
# (../../loadprofile/profile.json) is for `go run ./cmd/analytics seed`
# outside a container, the same local-vs-compose split every other
# ANALYTICS_* default follows.
# hadolint ignore=DL3022
COPY --from=loadprofile . /etc/analytics/loadprofile

# distroless/static ships no shell, wget, or curl -- unlike the canary's
# debian-slim runtime, a container HEALTHCHECK here cannot exec a probe
# tool. The binary carries its own `healthcheck` subcommand instead (hits
# its own /healthz + /readyz over loopback), which is what makes the
# smaller, shell-less runtime image possible at all.
HEALTHCHECK --interval=10s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/analytics", "healthcheck"]

# distroless/static:nonroot already runs as uid 65532 (non-root) by default.
EXPOSE 8082

ENTRYPOINT ["/usr/local/bin/analytics"]
CMD ["serve"]
