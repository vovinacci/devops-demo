import { expect, afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'
import * as matchers from '@testing-library/jest-dom/matchers'

// Extend expect with matchers from jest-dom
expect.extend(matchers)

// Clean up after each test
afterEach(() => {
  cleanup()
})
