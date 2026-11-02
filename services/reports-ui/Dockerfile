# ============================================================================
# reports-ui (RFC-0002, ADR-0013): Caddy static server + reverse proxy.
# Single-stage, digest-pinned (repo convention) -- there is no build step
# (RFC-0002 D3): the site is hand-written HTML/CSS/JS copied in verbatim.
# ============================================================================
FROM caddy:2-alpine@sha256:e70b164da81ef698b3ad19af799ea6086ce6d9e48ed4079eb9fe26f6fbf14ee5

# CVE-2026-111497 (brotli-libs, CRITICAL): the Alpine repos ship the fix
# (1.2.1-r0) but the upstream image has not been rebuilt with it yet, so the
# Trivy gate (ADR-0011) fails on the pinned digest alone. Drop this line once
# a Renovate digest bump carries brotli-libs >= 1.2.1-r0.
RUN apk upgrade --no-cache brotli-libs

# Default upstream for the /api reverse proxy: the compose service name, so
# the image is unchanged out of the box. Kubernetes overrides it through the
# chart's ConfigMap (deploy/k8s/charts/reports-ui).
ENV REPORTS_UI_REPORTS_URL=reports:8083
COPY Caddyfile /etc/caddy/Caddyfile
COPY site/ /srv/

# Fail the build on a malformed Caddyfile (D6 hygiene): a bad config never
# reaches a running container.
RUN caddy validate --config /etc/caddy/Caddyfile

# Non-root runtime (D6 hygiene, same posture as canary/reports). Caddy writes
# autosave/state under the XDG dirs (/config, /data), so they are chowned to
# the runtime user; :8084 is unprivileged so no cap is needed to bind it.
RUN addgroup -S caddy && adduser -S -G caddy -u 1000 caddy \
    && chown -R caddy:caddy /config /data /srv
USER 1000

HEALTHCHECK --interval=10s --timeout=5s --start-period=5s --retries=3 \
    CMD ["sh", "-c", "wget -q -O /dev/null http://localhost:8084/healthz || exit 1"]

EXPOSE 8084

CMD ["caddy", "run", "--config", "/etc/caddy/Caddyfile"]
