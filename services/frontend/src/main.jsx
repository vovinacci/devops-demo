import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { onCLS, onINP, onFCP, onLCP, onTTFB } from 'web-vitals'
import './index.css'
import App from './App.jsx'

// Function to send metrics to backend
function sendToAnalytics(metric) {
  const getApiUrl = () => {
    if (import.meta.env.VITE_API_URL) {
      return import.meta.env.VITE_API_URL
    }
    if (window.location.hostname === 'localhost' && window.location.port === '5173') {
      return 'http://localhost:8000'
    }
    return '/api/' // Production via nginx proxy (with trailing slash)
  }

  const API_URL = getApiUrl()

  // Send metric to backend. keepalive lets the request outlive the page:
  // LCP and CLS flush on tab hide/unload, exactly when a plain fetch may
  // be dropped by the browser.
  fetch(`${API_URL}/metrics/frontend`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      name: metric.name,
      value: metric.value,
    }),
    keepalive: true,
  }).catch(err => {
    console.warn('Failed to send metric:', metric.name, err)
  })
}

// Register all Web Vitals metrics
onCLS(sendToAnalytics) // Cumulative Layout Shift
onINP(sendToAnalytics) // Interaction to Next Paint (replaces FID in v5+)
onFCP(sendToAnalytics) // First Contentful Paint
onLCP(sendToAnalytics) // Largest Contentful Paint
onTTFB(sendToAnalytics) // Time to First Byte

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <App />
  </StrictMode>
)
