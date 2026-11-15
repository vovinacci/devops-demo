# ============================================================================
# Multi-stage Dockerfile for React + Vite frontend
# ============================================================================

# ----------------------------------------------------------------------------
# Stage 1: Builder - build React application
# ----------------------------------------------------------------------------
FROM node:24-alpine@sha256:a0b9bf06e4e6193cf7a0f58816cc935ff8c2a908f81e6f1a95432d679c54fbfd AS builder

WORKDIR /build

# Copy package files for better caching
COPY package*.json ./

# Install dependencies
RUN npm ci

# Copy source code
COPY . .

# Build production build
RUN npm run build

# ----------------------------------------------------------------------------
# Stage 2: Runtime - nginx for static hosting
# ----------------------------------------------------------------------------
FROM nginx:alpine@sha256:785835ea65db4bba6f021acc0f3be91b0fef76a53fe41f8588c6698d2aed0606 AS runtime

# CVE-2026-111497 (brotli-libs, CRITICAL): the Alpine repos ship the fix
# (1.2.1-r0) but the upstream image has not been rebuilt with it yet, so the
# Trivy gate (ADR-0011) fails on the pinned digest alone. Drop this line once
# a Renovate digest bump carries brotli-libs >= 1.2.1-r0.
RUN apk upgrade --no-cache brotli-libs

# Copy built application from builder stage
COPY --from=builder /build/dist /usr/share/nginx/html

# nginx renders /etc/nginx/templates/*.template through envsubst into
# conf.d/ at start-up. The default keeps the compose service name, so an
# image run with no environment behaves exactly as before; Kubernetes
# overrides it with the Service DNS name (deploy/k8s/charts/frontend).
ENV FRONTEND_BACKEND_URL=http://api:8000
COPY nginx.conf /etc/nginx/templates/default.conf.template

# Expose port
EXPOSE 80

# Healthcheck; 127.0.0.1 explicitly -- busybox wget resolves localhost to
# ::1 first and nginx listens on IPv4 only
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["sh", "-c", "wget --no-verbose --tries=1 --spider http://127.0.0.1/ || exit 1"]

# nginx starts automatically
CMD ["nginx", "-g", "daemon off;"]
