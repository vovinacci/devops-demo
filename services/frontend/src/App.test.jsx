import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import App from './App'

/**
 * Tests support two modes:
 * 1. With mocks (default) - fast unit tests without real API
 * 2. With real API - integration tests that clean up the database after themselves
 *
 * To use real API, set environment variable:
 *   VITE_TEST_API_URL=http://localhost:8000 npm run test
 *
 * Tests automatically track created items and delete them after each test.
 */
const USE_REAL_API = !!(
  process.env.VITE_TEST_API_URL ||
  import.meta.env.VITE_TEST_API_URL ||
  process.env.VITEST_TEST_API_URL
)
const API_URL =
  process.env.VITE_TEST_API_URL ||
  import.meta.env.VITE_TEST_API_URL ||
  process.env.VITEST_TEST_API_URL ||
  'http://localhost:8000'

// Store original fetch
const originalFetch = globalThis.fetch || global.fetch

// Track created items for cleanup
let createdItemIds = []

// Wrapper for fetch that tracks created items
const trackedFetch = async (url, options = {}) => {
  const response = await originalFetch(url, options)

  // If this is a POST request to /items and it's successful, save the ID
  if (options.method === 'POST' && url.includes('/items') && response.ok) {
    try {
      const clonedResponse = response.clone()
      const data = await clonedResponse.json()
      if (data && data.id) {
        createdItemIds.push(data.id)
      }
    } catch {
      // Ignore parsing errors
    }
  }

  return response
}

// Set up fetch
if (USE_REAL_API) {
  // Use real fetch with tracking
  global.fetch = trackedFetch
} else {
  // Use mocks for unit tests
  global.fetch = vi.fn()
}

describe('App Component', () => {
  beforeEach(() => {
    // Clear list of created items
    createdItemIds = []

    if (!USE_REAL_API) {
      // Clear mocks before each test
      vi.clearAllMocks()
    }
    // Reset window.confirm
    window.confirm = vi.fn(() => true)
  })

  afterEach(async () => {
    // Clean up created items from database
    if (USE_REAL_API && createdItemIds.length > 0) {
      for (const itemId of createdItemIds) {
        try {
          await originalFetch(`${API_URL}/items/${itemId}`, {
            method: 'DELETE',
          })
        } catch (err) {
          // Ignore errors during cleanup
          console.warn(`Failed to cleanup item ${itemId}:`, err.message || err)
        }
      }
      createdItemIds = []
    }

    if (!USE_REAL_API) {
      vi.restoreAllMocks()
    }
  })

  describe('Initial render', () => {
    it('should render the app header', async () => {
      if (!USE_REAL_API) {
        global.fetch.mockResolvedValueOnce({
          ok: true,
          json: async () => [],
        })
      }

      await act(async () => {
        render(<App />)
      })

      // Wait for async operations to complete
      await waitFor(() => {
        expect(screen.getByText('DevOps Demo - Items Manager')).toBeInTheDocument()
      })
    })

    it('should fetch and display items on mount', async () => {
      if (!USE_REAL_API) {
        const mockItems = [
          { id: 1, name: 'Item 1' },
          { id: 2, name: 'Item 2' },
        ]

        global.fetch.mockResolvedValueOnce({
          ok: true,
          json: async () => mockItems,
        })
      }

      await act(async () => {
        render(<App />)
      })

      await waitFor(
        () => {
          expect(screen.getByText(/Items \(\d+\)/)).toBeInTheDocument()
        },
        { timeout: 5000 }
      )

      if (!USE_REAL_API) {
        expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining('/items'))
      }
    })

    it('should display loading state while fetching', async () => {
      if (!USE_REAL_API) {
        global.fetch.mockImplementationOnce(
          () =>
            new Promise(resolve => {
              setTimeout(() => {
                resolve({
                  ok: true,
                  json: async () => [],
                })
              }, 100)
            })
        )
      }

      await act(async () => {
        render(<App />)
      })

      // Loading state can be very fast with real API
      if (!USE_REAL_API) {
        await waitFor(() => {
          expect(screen.getByText('Loading items...')).toBeInTheDocument()
        })

        await waitFor(() => {
          expect(screen.queryByText('Loading items...')).not.toBeInTheDocument()
        })
      } else {
        // With real API, just verify that component loaded
        await waitFor(() => {
          expect(screen.getByPlaceholderText('Enter item name')).toBeInTheDocument()
        })
      }
    })

    it('should display empty state when no items', async () => {
      if (!USE_REAL_API) {
        global.fetch.mockResolvedValueOnce({
          ok: true,
          json: async () => [],
        })
      }

      await act(async () => {
        render(<App />)
      })

      await waitFor(
        () => {
          expect(screen.getByText('No items yet. Add one above!')).toBeInTheDocument()
          expect(screen.getByText('Items (0)')).toBeInTheDocument()
        },
        { timeout: 5000 }
      )
    })
  })

  describe('Error handling', () => {
    it('should display error when fetch fails', async () => {
      if (!USE_REAL_API) {
        global.fetch.mockRejectedValueOnce(new Error('Network error'))

        await act(async () => {
          render(<App />)
        })

        await waitFor(() => {
          expect(screen.getByText(/Network error/i)).toBeInTheDocument()
        })
      } else {
        // With real API, skip this test since we can't simulate network error
        // without complex manipulations
      }
    })

    it('should display error when API returns non-ok status', async () => {
      if (!USE_REAL_API) {
        global.fetch.mockResolvedValueOnce({
          ok: false,
          status: 500,
        })

        await act(async () => {
          render(<App />)
        })

        await waitFor(() => {
          expect(screen.getByText(/HTTP error! status: 500/i)).toBeInTheDocument()
        })
      } else {
        // With real API, skip this test
      }
    })
  })

  describe('Create item', () => {
    it('should create a new item when form is submitted', async () => {
      const user = userEvent.setup()

      if (USE_REAL_API) {
        // Use real API
        // First clear the items list
        await originalFetch(`${API_URL}/items`, { method: 'GET' })
      } else {
        // Mock initial fetch
        global.fetch.mockResolvedValueOnce({
          ok: true,
          json: async () => [],
        })

        // Mock create request
        const newItem = { id: 1, name: 'New Item' }
        global.fetch.mockResolvedValueOnce({
          ok: true,
          json: async () => newItem,
        })
      }

      await act(async () => {
        render(<App />)
      })

      await waitFor(() => {
        expect(screen.getByPlaceholderText('Enter item name')).toBeInTheDocument()
      })

      const input = screen.getByPlaceholderText('Enter item name')
      const submitButton = screen.getByRole('button', { name: /add item/i })

      await user.type(input, 'New Item')
      await user.click(submitButton)

      await waitFor(
        () => {
          expect(screen.getByText('New Item')).toBeInTheDocument()
          expect(screen.getByText('Items (1)')).toBeInTheDocument()
        },
        { timeout: 5000 }
      )

      if (!USE_REAL_API) {
        expect(global.fetch).toHaveBeenCalledWith(
          expect.stringContaining('/items'),
          expect.objectContaining({
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({ name: 'New Item' }),
          })
        )
      }
    })

    it('should not create item with empty name', async () => {
      const user = userEvent.setup()

      if (!USE_REAL_API) {
        global.fetch.mockResolvedValueOnce({
          ok: true,
          json: async () => [],
        })
      }

      await act(async () => {
        render(<App />)
      })

      await waitFor(() => {
        expect(screen.getByPlaceholderText('Enter item name')).toBeInTheDocument()
      })

      const submitButton = screen.getByRole('button', { name: /add item/i })
      expect(submitButton).toBeDisabled()

      await user.type(screen.getByPlaceholderText('Enter item name'), '   ')
      expect(submitButton).toBeDisabled()

      await user.type(screen.getByPlaceholderText('Enter item name'), 'Test')
      expect(submitButton).not.toBeDisabled()
    })

    it('should display error when create fails', async () => {
      const user = userEvent.setup()

      if (!USE_REAL_API) {
        global.fetch.mockResolvedValueOnce({
          ok: true,
          json: async () => [],
        })

        global.fetch.mockResolvedValueOnce({
          ok: false,
          status: 409,
          json: async () => ({ detail: 'Item already exists' }),
        })
      } else {
        // With real API, first create item, then try to create duplicate
        // This will be automatically tracked and cleaned up in afterEach
      }

      await act(async () => {
        render(<App />)
      })

      await waitFor(() => {
        expect(screen.getByPlaceholderText('Enter item name')).toBeInTheDocument()
      })

      const input = screen.getByPlaceholderText('Enter item name')
      await user.type(input, 'Duplicate Item')
      await user.click(screen.getByRole('button', { name: /add item/i }))

      if (USE_REAL_API) {
        // With real API, first create item
        await waitFor(
          () => {
            expect(screen.getByText('Duplicate Item')).toBeInTheDocument()
          },
          { timeout: 5000 }
        )

        // Then try to create duplicate
        await user.type(input, 'Duplicate Item')
        await user.click(screen.getByRole('button', { name: /add item/i }))
      }

      await waitFor(
        () => {
          expect(screen.getByText(/Item already exists/i)).toBeInTheDocument()
        },
        { timeout: 5000 }
      )
    })

    it('should clear input after successful creation', async () => {
      const user = userEvent.setup()

      if (!USE_REAL_API) {
        global.fetch.mockResolvedValueOnce({
          ok: true,
          json: async () => [],
        })

        global.fetch.mockResolvedValueOnce({
          ok: true,
          json: async () => ({ id: 1, name: 'New Item' }),
        })
      }

      await act(async () => {
        render(<App />)
      })

      await waitFor(() => {
        expect(screen.getByPlaceholderText('Enter item name')).toBeInTheDocument()
      })

      const input = screen.getByPlaceholderText('Enter item name')
      await user.type(input, 'New Item')
      await user.click(screen.getByRole('button', { name: /add item/i }))

      await waitFor(
        () => {
          expect(input).toHaveValue('')
        },
        { timeout: 5000 }
      )
    })
  })

  describe('Delete item', () => {
    it('should delete an item when delete button is clicked', async () => {
      const user = userEvent.setup()

      const mockItems = [
        { id: 1, name: 'Item 1' },
        { id: 2, name: 'Item 2' },
      ]

      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockItems,
      })

      global.fetch.mockResolvedValueOnce({
        ok: true,
      })

      await act(async () => {
        render(<App />)
      })

      await waitFor(() => {
        expect(screen.getByText('Item 1')).toBeInTheDocument()
      })

      const deleteButton = screen.getByRole('button', { name: /delete item 1/i })
      await user.click(deleteButton)

      await waitFor(() => {
        expect(screen.queryByText('Item 1')).not.toBeInTheDocument()
        expect(screen.getByText('Item 2')).toBeInTheDocument()
        expect(screen.getByText('Items (1)')).toBeInTheDocument()
      })

      expect(global.fetch).toHaveBeenCalledWith(
        expect.stringContaining('/items/1'),
        expect.objectContaining({
          method: 'DELETE',
        })
      )
    })

    it('should not delete item if user cancels confirmation', async () => {
      const user = userEvent.setup()

      window.confirm = vi.fn(() => false)

      const mockItems = [{ id: 1, name: 'Item 1' }]

      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockItems,
      })

      await act(async () => {
        render(<App />)
      })

      await waitFor(() => {
        expect(screen.getByText('Item 1')).toBeInTheDocument()
      })

      const deleteButton = screen.getByRole('button', { name: /delete item 1/i })
      await user.click(deleteButton)

      expect(window.confirm).toHaveBeenCalledWith('Delete "Item 1"?')
      expect(screen.getByText('Item 1')).toBeInTheDocument()
      expect(global.fetch).toHaveBeenCalledTimes(1) // Only initial fetch
    })

    it('should display error when delete fails', async () => {
      const user = userEvent.setup()

      const mockItems = [{ id: 1, name: 'Item 1' }]

      global.fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockItems,
      })

      global.fetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
      })

      await act(async () => {
        render(<App />)
      })

      await waitFor(() => {
        expect(screen.getByText('Item 1')).toBeInTheDocument()
      })

      const deleteButton = screen.getByRole('button', { name: /delete item 1/i })
      await user.click(deleteButton)

      await waitFor(
        () => {
          expect(screen.getByText(/HTTP error! status: 404/i)).toBeInTheDocument()
        },
        { timeout: 2000 }
      )

      // Item should still be in the list since deletion failed
      expect(screen.getByText('Item 1')).toBeInTheDocument()
    })
  })

  describe('Form validation', () => {
    it('should disable submit button when input is empty', async () => {
      if (!USE_REAL_API) {
        global.fetch.mockResolvedValueOnce({
          ok: true,
          json: async () => [],
        })
      }

      await act(async () => {
        render(<App />)
      })

      await waitFor(() => {
        const submitButton = screen.getByRole('button', { name: /add item/i })
        expect(submitButton).toBeDisabled()
      })
    })

    it('should enable submit button when input has value', async () => {
      const user = userEvent.setup()

      if (!USE_REAL_API) {
        global.fetch.mockResolvedValueOnce({
          ok: true,
          json: async () => [],
        })
      }

      await act(async () => {
        render(<App />)
      })

      await waitFor(() => {
        expect(screen.getByPlaceholderText('Enter item name')).toBeInTheDocument()
      })

      const input = screen.getByPlaceholderText('Enter item name')
      const submitButton = screen.getByRole('button', { name: /add item/i })

      expect(submitButton).toBeDisabled()

      await user.type(input, 'Test Item')
      expect(submitButton).not.toBeDisabled()
    })
  })
})
