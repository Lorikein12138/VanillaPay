import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, beforeEach, expect, vi } from 'vitest'

// jsdom has no layout, so it does not implement scrolling; the router's scroll
// restoration calls it on every navigation.
Object.defineProperty(window, 'scrollTo', { configurable: true, writable: true, value: () => undefined })

// Any console.warn / console.error not explicitly consumed by a test fails it.
const unexpected: string[] = []

beforeEach(() => {
  unexpected.length = 0
  localStorage.clear()
  document.documentElement.removeAttribute('data-theme')
  document.documentElement.removeAttribute('lang')
  for (const level of ['warn', 'error'] as const) {
    vi.spyOn(console, level).mockImplementation((...args: unknown[]) => {
      unexpected.push(`console.${level}: ${args.map(String).join(' ')}`)
    })
  }
})

afterEach(() => {
  cleanup()
  const logged = unexpected.splice(0)
  vi.restoreAllMocks()
  expect(logged, 'unexpected console output').toEqual([])
})
