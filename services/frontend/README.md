# Frontend - React + Vite

Minimal React frontend for the DevOps Demo project with Web Vitals metrics. The frontend implements a CRUD interface for managing items and automatically tracks
web application performance metrics.

## Table of Contents

- [Quick Start](#quick-start)
- [Functionality](#functionality)
- [Configuration](#configuration)
- [Docker](#docker)
- [Web Vitals Metrics](#web-vitals-metrics)
- [Styling](#styling)
- [Development](#development)
- [Troubleshooting](#troubleshooting)

## Quick Start

### System Requirements

- Node.js version 24 or newer
- npm (installed with Node.js) or yarn
- Backend API should be running on `http://localhost:8000` (for full functionality)

### Installation and Running

**Install dependencies:**

```shell
npm install
```

**Run development server:**

```shell
npm run dev
```

Frontend will be available at http://localhost:5173 (default Vite port).

**Build for production:**

```shell
npm run build
```

This will create an optimized production build in the `dist/` directory.

**Preview production build:**

```shell
npm run preview
```

This will start a local server to preview the production build.

## Functionality

### CRUD Operations

The frontend implements a complete set of CRUD operations for managing items:

- **Create**
  - Form for adding new items
  - Client-side input validation
  - POST request to `/items` endpoint
  - List update after successful creation

- **Read**
  - Display list of all items
  - Automatic update on page load
  - Error handling when fetching data

- **Delete**
  - Delete button for each item
  - Deletion confirmation via confirm dialog
  - DELETE request to `/items/{item_id}` endpoint
  - List update after deletion

- **Update**
  - Ability to edit existing items (if implemented)
  - Validation of updated data
  - PATCH or PUT request

### Web Vitals Metrics

The frontend automatically tracks and sends the following performance metrics to the backend:

- **LCP (Largest Contentful Paint)**
  - Measures time to load the largest content element
  - Sent automatically on page load
  - Target value: < 2.5 seconds

- **INP (Interaction to Next Paint)**
  - Measures response time to user interaction
  - Replaces FID (First Input Delay) in web-vitals v5+
  - Sent on user interaction with interface
  - Target value: < 200 milliseconds

- **CLS (Cumulative Layout Shift)**
  - Measures layout stability during loading
  - Sent when layout shifts are detected
  - Target value: < 0.1

- **FCP (First Contentful Paint)**
  - Measures time to first content display
  - Sent automatically on page load
  - Target value: < 1.8 seconds

- **TTFB (Time to First Byte)**
  - Measures time to receive first byte from server
  - Sent automatically on page load
  - Target value: < 800 milliseconds

Metrics are sent to the `/metrics/frontend` backend endpoint via POST requests with JSON payload.

## Configuration

### Environment Variables

Create a `.env` file in the root of the `services/frontend/` directory (or use `.env.example` as a base):

```env
VITE_API_URL=http://localhost:8000
```

**Important:** Environment variables for Vite must have the `VITE_` prefix to be accessible in code.

**Access variables in code:**

```javascript
const apiUrl = import.meta.env.VITE_API_URL || 'http://localhost:8000';
```

**Note:** In production via nginx proxy, the `/api` path is used, which automatically redirects to the backend API.

### Vite Configuration

Main configuration is located in `vite.config.js`:

- **Development server:** Configured on port 5173
- **Proxy:** API requests are proxied to backend (if configured)
- **Build:** Optimizations for production build
- **Hot Module Replacement (HMR):** Enabled for fast development

## Docker

### Development

For local development with Docker:

```shell
# Build image
docker build -t devops-demo-frontend ./services/frontend

# Run container
docker run -p 8080:80 devops-demo-frontend
```

Frontend will be available at http://localhost:8080.

### Production (via docker-compose)

Frontend is automatically built and started via `deploy/compose/docker-compose.yml`:

```shell
# Start only frontend service
docker compose up -d web

# Or start all services
docker compose up -d
```

**Multi-stage build:**

Dockerfile uses multi-stage build for optimization:

1. **Build stage:** Install dependencies and build project
2. **Runtime stage:** Nginx for serving static files

This allows creating a minimal production image without Node.js and build tools.

## Web Vitals

Metrics are automatically sent to the backend on page load and user interaction. All metrics are available in Prometheus and can be visualized in Grafana.

### Checking Metrics

**1. Via Browser DevTools:**

- Open DevTools (F12)
- Go to Network tab
- Load page
- Find POST requests to `/metrics/frontend`
- Check request payloads

**2. Via Prometheus:**

- Open Prometheus UI: http://localhost:9090
- Use query: `fe_web_vital{name="LCP"}` or other metrics
- Check metric values

**3. Via Grafana:**

- Open Grafana: http://localhost:3000
- Find dashboard "DevOps Demo Dashboard"
- Check panels with Web Vitals metrics

### Configuring Metrics

Metrics are configured in `src/main.jsx` via the `web-vitals` library:

```javascript
import {onCLS, onINP, onLCP, onFCP, onTTFB} from 'web-vitals';

// Send metrics to backend
function sendToAnalytics(metric) {
    fetch('/metrics/frontend', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
            name: metric.name,
            value: metric.value,
            url: window.location.pathname
        })
    });
}

// Subscribe to metrics
onCLS(sendToAnalytics);
onINP(sendToAnalytics);
onLCP(sendToAnalytics);
onFCP(sendToAnalytics);
onTTFB(sendToAnalytics);
```

## Styling

Simple CSS without frameworks is used for minimalism and fast loading. The design is responsive and works on mobile devices.

**Main Styles:**

- `index.css` - global styles, CSS reset, base utilities
- `App.css` - styles for App component and its child elements

**Responsive Design:**

- Flexbox used for layout
- Media queries for adaptation to different screen sizes
- Mobile version optimized for touch interaction

## Development Workflow

### Adding New Components

```shell
# Create new component
touch src/components/NewComponent.jsx
```

**Component Structure:**

```javascript
import {useState} from 'react';
import './NewComponent.css';

export function NewComponent() {
    const [state, setState] = useState('');

    return (
        <div className="new-component">
            {/* Component content */}
        </div>
    );
}
```

### Linting

**Code check:**

```shell
npm run lint
```

**Auto-fix:**

```shell
npm run lint:fix
```

**Via Make:**

```shell
make lint-frontend
```

### Code Formatting

**Formatting via Prettier:**

```shell
npm run format
```

**Check formatting:**

```shell
npm run format:check
```

**Via Make:**

```shell
make format-frontend
```

### Testing

**Run tests:**

```shell
npm run test
```

**Run tests in watch mode:**

```shell
npm run test:watch
```

**Run tests with UI:**

```shell
npm run test:ui
```

**Via Make:**

```shell
make test-frontend
```

## Troubleshooting

### API Connection Issues

**Symptoms:** Errors when fetching or sending data, CORS errors.

**Solution:**

1. **Verify backend is running:**

   ```shell
   curl http://localhost:8000/health
   # Should return: {"status": "ok", "database": "connected"}
   ```

2. **Check CORS settings on backend:**
    - Backend should allow requests from `http://localhost:5173` (development)
    - Or from `http://localhost:8080` (production via nginx)

3. **Check `VITE_API_URL` in `.env` file:**

   ```shell
   cat services/frontend/.env
   # Should contain: VITE_API_URL=http://localhost:8000
   ```

4. **Restart dev server after changing `.env`:**

   ```shell
   # Stop dev server (Ctrl+C)
   # Start again
   npm run dev
   ```

### Metrics Not Sending

**Symptoms:** Web Vitals metrics don't appear in Prometheus or Grafana.

**Solution:**

1. **Check browser console for errors:**
    - Open DevTools (F12)
    - Go to Console tab
    - Check for JavaScript errors

2. **Verify `/metrics/frontend` endpoint is accessible:**

   ```shell
   curl -X POST http://localhost:8000/metrics/frontend \
     -H "Content-Type: application/json" \
     -d '{"name":"LCP","value":1.5}'
   ```

3. **Check Network tab in DevTools:**
    - Open DevTools → Network
    - Load page
    - Find POST requests to `/metrics/frontend`
    - Check response status (should be 200 or 204)

4. **Verify web-vitals library is installed:**

   ```shell
   npm list web-vitals
   ```

### Docker Build Issues

**Symptoms:** Errors when building Docker image, large image size.

**Solution:**

1. **Ensure Node.js version matches in Dockerfile:**

   ```dockerfile
   FROM node:20-alpine AS builder
   ```

   Verify version matches local Node.js version.

2. **Check `.dockerignore` file:**

    - Ensure `node_modules`, `.git`, and other unnecessary files are excluded
    - This will reduce build context size

3. **Check build logs:**

   ```shell
   docker build -t devops-demo-frontend ./services/frontend --progress=plain
   ```

4. **Check container logs:**

   ```shell
   docker compose logs web
   ```

5. **Clear Docker cache if needed:**

   ```shell
   docker builder prune
   ```

### Hot Module Replacement (HMR) Issues

**Symptoms:** Code changes don't display automatically, need to manually reload page.

**Solution:**

1. **Verify dev server is running:**

   ```shell
   npm run dev
   ```

2. **Check port 5173 is not occupied:**

   ```shell
   lsof -i :5173
   ```

3. **Restart dev server:**

   ```shell
   # Stop (Ctrl+C) and start again
   npm run dev
   ```

4. **Clear browser cache:**
    - Open DevTools (F12)
    - Right-click on refresh button
    - Select "Empty Cache and Hard Reload"

### Production Build Issues

**Symptoms:** Production build doesn't work, errors on run.

**Solution:**

1. **Verify build completed successfully:**

   ```shell
   npm run build
   # Verify dist/ directory is created
   ```

2. **Check nginx configuration:**

   ```shell
   docker compose exec web nginx -t
   # Should show: nginx: configuration file /etc/nginx/nginx.conf test is successful
   ```

3. **Check nginx logs:**

   ```shell
   docker compose logs web
   ```

4. **Verify static files are accessible:**

   ```shell
   curl http://localhost:8080/
   # Should return HTML page
   ```

---

**Additional Information:** Detailed documentation about local environment configuration is available in [Local setup](../docs/local-setup.md).
