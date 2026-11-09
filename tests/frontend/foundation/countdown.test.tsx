import { act, renderHook } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { useCountdown } from '@/components/ui/countdown'

afterEach(() => vi.useRealTimers())

it('synchronizes deadline changes immediately, clamps overdue values and clears the stopped interval', async () => {
  vi.useFakeTimers()
  const start = Date.now()
  const view = renderHook<number, { deadline: number | null }>(({ deadline }) => useCountdown(deadline), {
    initialProps: { deadline: start + 5000 },
  })
  expect(view.result.current).toBe(5)
  await act(() => vi.advanceTimersByTimeAsync(1000))
  expect(view.result.current).toBe(4)
  vi.setSystemTime(start + 1750)
  view.rerender({ deadline: start + 2800 })
  expect(view.result.current).toBe(2)
  view.rerender({ deadline: start - 1000 })
  expect(view.result.current).toBe(0)
  view.rerender({ deadline: null })
  expect(view.result.current).toBe(0)
  expect(vi.getTimerCount()).toBe(0)
  view.rerender({ deadline: Date.now() + 2000 })
  expect(view.result.current).toBe(2)
  view.unmount()
  expect(vi.getTimerCount()).toBe(0)
})
