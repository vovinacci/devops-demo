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
FROM nginx:alpine@sha256:54f2a904c251d5a34adf545a72d32515a15e08418dae0266e23be2e18c66fefa AS runtime

# Copy built application from builder stage
COPY --from=builder /build/dist /usr/share/nginx/html

# Copy custom nginx configuration
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Expose port
EXPOSE 80

# Healthcheck; 127.0.0.1 explicitly -- busybox wget resolves localhost to
# ::1 first and nginx listens on IPv4 only
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1/ || exit 1

# nginx starts automatically
CMD ["nginx", "-g", "daemon off;"]
