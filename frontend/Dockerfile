# ============================================================================
# Multi-stage Dockerfile for React + Vite frontend
# ============================================================================

# ----------------------------------------------------------------------------
# Stage 1: Builder - build React application
# ----------------------------------------------------------------------------
FROM node:24-alpine AS builder

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
FROM nginx:alpine AS runtime

# Copy built application from builder stage
COPY --from=builder /build/dist /usr/share/nginx/html

# Copy custom nginx configuration
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Expose port
EXPOSE 80

# Healthcheck
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost/ || exit 1

# nginx starts automatically
CMD ["nginx", "-g", "daemon off;"]
