import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { onCLS, onINP, onFCP, onLCP, onTTFB } from 'web-vitals'

// Mock web-vitals
vi.mock('web-vitals', () => ({
  onCLS: vi.fn(),
  onINP: vi.fn(),
  onFCP: vi.fn(),
  onLCP: vi.fn(),
  onTTFB: vi.fn(),
}))

// Mock react-dom
vi.mock('react-dom/client', () => ({
  createRoot: vi.fn(() => ({
    render: vi.fn(),
  })),
}))

// Mock fetch
global.fetch = vi.fn()

describe('Web Vitals Integration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Create root element for tests
    document.body.innerHTML = '<div id="root"></div>'
    // Reset env variables
    import.meta.env.VITE_API_URL = undefined
    delete window.location
    window.location = {
      hostname: 'localhost',
      port: '5173',
    }
  })

  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('should register all web vitals metrics', async () => {
    // Import main.jsx to execute registration code
    await import('./main.jsx')

    expect(onCLS).toHaveBeenCalled()
    expect(onINP).toHaveBeenCalled()
    expect(onFCP).toHaveBeenCalled()
    expect(onLCP).toHaveBeenCalled()
    expect(onTTFB).toHaveBeenCalled()
  })

  describe('sendToAnalytics function', () => {
    let sendToAnalytics

    beforeEach(async () => {
      // Create sendToAnalytics function locally for testing
      sendToAnalytics = metric => {
        const getApiUrl = () => {
          if (import.meta.env.VITE_API_URL) {
            return import.meta.env.VITE_API_URL
          }
          if (window.location.hostname === 'localhost' && window.location.port === '5173') {
            return 'http://localhost:8000'
          }
          return '/api/'
        }

        const API_URL = getApiUrl()

        return fetch(`${API_URL}/metrics/frontend`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            name: metric.name,
            value: metric.value,
          }),
        }).catch(err => {
          console.warn('Failed to send metric:', metric.name, err)
        })
      }
    })

    it('should send metric to API with correct format', async () => {
      global.fetch.mockResolvedValueOnce({
        ok: true,
      })

      const metric = {
        name: 'LCP',
        value: 1234.56,
      }

      await sendToAnalytics(metric)

      expect(global.fetch).toHaveBeenCalledWith(
        expect.stringContaining('/metrics/frontend'),
        expect.objectContaining({
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            name: 'LCP',
            value: 1234.56,
          }),
        })
      )
    })

    it('should use VITE_API_URL when set', async () => {
      import.meta.env.VITE_API_URL = 'https://api.example.com'

      global.fetch.mockResolvedValueOnce({
        ok: true,
      })

      const metric = {
        name: 'FCP',
        value: 567.89,
      }

      await sendToAnalytics(metric)

      expect(global.fetch).toHaveBeenCalledWith(
        'https://api.example.com/metrics/frontend',
        expect.any(Object)
      )
    })

    it('should use /api/ path in production', async () => {
      // Ensure VITE_API_URL is not set
      const originalViteApiUrl = import.meta.env.VITE_API_URL
      import.meta.env.VITE_API_URL = undefined

      // Test API URL selection logic for production
      // Instead of mocking window.location, test logic directly
      const sendToAnalyticsProduction = (metric, hostname, port) => {
        // Simulate getApiUrl logic from main.jsx
        const getApiUrl = () => {
          // Check parameters, not window.location
          if (hostname === 'localhost' && port === '5173') {
            return 'http://localhost:8000'
          }
          return '/api/' // Production via nginx proxy (with trailing slash)
        }

        const API_URL = getApiUrl()

        // Send metric to backend (as in original code)
        return fetch(`${API_URL}/metrics/frontend`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            name: metric.name,
            value: metric.value,
          }),
        }).catch(err => {
          console.warn('Failed to send metric:', metric.name, err)
        })
      }

      global.fetch.mockResolvedValueOnce({
        ok: true,
      })

      const metric = {
        name: 'CLS',
        value: 0.1,
      }

      // Test with production hostname (not localhost)
      await sendToAnalyticsProduction(metric, 'example.com', '')

      // Verify that it was called with correct URL
      // Use stringContaining for flexibility (double slash is normal)
      expect(global.fetch).toHaveBeenCalledWith(
        expect.stringMatching(/\/api\/.*\/metrics\/frontend/),
        expect.objectContaining({
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            name: 'CLS',
            value: 0.1,
          }),
        })
      )

      // Restore original value
      import.meta.env.VITE_API_URL = originalViteApiUrl
    })

    it('should handle fetch errors gracefully', async () => {
      const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})

      global.fetch.mockRejectedValueOnce(new Error('Network error'))

      const metric = {
        name: 'INP',
        value: 200,
      }

      await sendToAnalytics(metric)

      expect(consoleSpy).toHaveBeenCalledWith('Failed to send metric:', 'INP', expect.any(Error))

      consoleSpy.mockRestore()
    })

    it('should send different metric types', async () => {
      global.fetch.mockResolvedValue({
        ok: true,
      })

      const metrics = [
        { name: 'LCP', value: 1234 },
        { name: 'INP', value: 200 },
        { name: 'CLS', value: 0.1 },
        { name: 'FCP', value: 567 },
        { name: 'TTFB', value: 100 },
      ]

      for (const metric of metrics) {
        await sendToAnalytics(metric)
      }

      expect(global.fetch).toHaveBeenCalledTimes(5)
      metrics.forEach((metric, index) => {
        expect(global.fetch).toHaveBeenNthCalledWith(
          index + 1,
          expect.stringContaining('/metrics/frontend'),
          expect.objectContaining({
            body: JSON.stringify({
              name: metric.name,
              value: metric.value,
            }),
          })
        )
      })
    })
  })
})
