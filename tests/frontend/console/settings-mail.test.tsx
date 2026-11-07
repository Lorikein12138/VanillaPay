import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { consoleApi } from '@/api/client'
import { useMailTest } from '@/features/console/settings/useMailTest'

const recipient = 'ops@example.test'
const queued = { delivery_id: 'test-delivery', state: 'queued' } as const
const sent = { id: queued.delivery_id, state: 'sent', error_code: null } as const
type Status = Awaited<ReturnType<typeof consoleApi.mailTestStatus>>

describe('settings test email lifecycle', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('aborts a hanging status request at the overall deadline and ignores its late reply', async () => {
    let resolve!: (value: Status) => void
    const pending = new Promise<Status>((done) => {
      resolve = done
    })
    const post = vi.spyOn(consoleApi, 'testMail').mockResolvedValue(queued)
    const poll = vi.spyOn(consoleApi, 'mailTestStatus').mockReturnValueOnce(pending).mockResolvedValue(sent)
    const { result } = renderHook(useMailTest)
    act(() => {
      void result.current.send(recipient)
    })
    await act(async () => vi.advanceTimersByTimeAsync(2000))
    const firstSignal = post.mock.calls[0]?.[1]
    expect(poll).toHaveBeenCalledExactlyOnceWith(queued.delivery_id, firstSignal)
    await act(async () => vi.advanceTimersByTimeAsync(58000))
    expect(result.current.state).toBe('timeout')
    expect(firstSignal?.aborted).toBe(true)
    expect(vi.getTimerCount()).toBe(0)

    act(() => {
      void result.current.send(recipient)
    })
    const secondSignal = post.mock.calls[1]?.[1]
    expect(secondSignal).not.toBe(firstSignal)
    expect(result.current.state).toBe('queued')
    await act(async () => {
      resolve({ ...sent, state: 'failed', error_code: 'unknown' })
      await pending
    })
    expect(result.current.state).toBe('queued')
    await act(async () => vi.advanceTimersByTimeAsync(2000))
    expect(result.current.state).toBe('sent')
    expect(secondSignal?.aborted).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('aborts and clears both timers when unmounted while waiting to poll', async () => {
    const post = vi.spyOn(consoleApi, 'testMail').mockResolvedValue(queued)
    const poll = vi.spyOn(consoleApi, 'mailTestStatus').mockResolvedValue(sent)
    const { result, unmount } = renderHook(useMailTest)
    await act(async () => {
      void result.current.send(recipient)
      await vi.advanceTimersByTimeAsync(1000)
    })
    expect(vi.getTimerCount()).toBe(2)
    unmount()
    expect(post.mock.calls[0]?.[1]?.aborted).toBe(true)
    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(60000)
    expect(poll).not.toHaveBeenCalled()
  })
})
