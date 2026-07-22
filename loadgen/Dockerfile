# ============================================================================
# loadgen image: grafana/k6 (RFC-0001 D4/D6, ADR-0006) + repo scenarios
# ============================================================================
# k6 v2.1.0's stock binary already bundles everything this loadgen needs:
# HTTP (k6/http), gRPC (k6/net/grpc), and the Prometheus remote-write
# output (`-o experimental-prometheus-rw`, internal/output/prometheusrw --
# still "experimental" in its flag name, but built into core k6, not an
# xk6 extension). No custom xk6 build stage is needed here, unlike the
# upstream `with-browser` image variant.
FROM grafana/k6:2.1.0@sha256:65c920dc067d5e2e00befbf982af6ad6ad0117034e8b1c65817c7975c52d4669

LABEL maintainer="DevOps Demo Team" \
      version="0.1.0" \
      description="DevOps Demo k6 load generator" \
      org.opencontainers.image.title="devops-demo-loadgen" \
      org.opencontainers.image.version="0.1.0" \
      org.opencontainers.image.description="k6 shaped load + incident mode (RFC-0001 D4)" \
      org.opencontainers.image.source="https://github.com/vovinacci/devops-demo"

WORKDIR /home/k6

# loadprofile/ and proto/ are named build contexts (compose:
# additional_contexts), the same pattern services/backend and
# services/analytics use for proto/ (RFC-0001 D8, ADR-0002): k6 imports
# shape.js/profile.json directly (loadgen/lib/env.js, lib/schedule.js)
# and loads items.proto at runtime (scenarios/main.js grpc.Client#load).
COPY scenarios ./scenarios
COPY lib ./lib
# hadolint ignore=DL3022
COPY --from=loadprofile . ./loadprofile
# hadolint ignore=DL3022
COPY --from=proto . ./proto
COPY --chmod=755 entrypoint.sh /home/k6/entrypoint.sh

# The base image's own ENTRYPOINT is ["k6"] (see upstream grafana/k6
# Dockerfile) -- this wrapper replaces it so LOADGEN_REF_UNIX can be set
# once before k6 starts (see entrypoint.sh), then execs the same `k6`
# binary the base image ships.
ENTRYPOINT ["/home/k6/entrypoint.sh"]
# -o experimental-prometheus-rw (RFC-0001 D4/Section 7, ADR-0006): pushes
# every k6 metric to Prometheus (K6_PROMETHEUS_RW_SERVER_URL,
# K6_PROMETHEUS_RW_TREND_STATS set in compose -- Prometheus itself needs
# --web.enable-remote-write-receiver, set on the prometheus service).
CMD ["run", "-o", "experimental-prometheus-rw", "/home/k6/scenarios/main.js"]
