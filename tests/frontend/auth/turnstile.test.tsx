import { fireEvent, render, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Turnstile } from '@/features/auth/Turnstile'

afterEach(() => {
  delete window.turnstile
  document.querySelectorAll('script[src*="challenges.cloudflare.com"]').forEach((script) => script.remove())
  vi.unstubAllGlobals()
})

describe('Turnstile lifecycle without external requests', () => {
  it('forwards the configured action/token, clears expired tokens and removes the widget', async () => {
    const disconnected = vi.fn()
    vi.stubGlobal('ResizeObserver', class { observe = vi.fn(); disconnect = disconnected })
    let options: Parameters<NonNullable<Window['turnstile']>['render']>[1] | undefined
    const remove = vi.fn()
    const widget = crypto.randomUUID()
    window.turnstile = { render: (_node, value) => { options = value; return widget }, remove }
    const onToken = vi.fn()
    const onError = vi.fn()
    const view = render(<Turnstile siteKey={crypto.randomUUID()} action="login" onToken={onToken} onError={onError} />)
    await waitFor(() => expect(options?.action).toBe('login'))
    const token = crypto.randomUUID()
    options?.callback(token)
    expect(onToken).toHaveBeenLastCalledWith(token)
    options?.['expired-callback']()
    expect(onToken).toHaveBeenLastCalledWith('')
    expect(onError).toHaveBeenCalledOnce()
    view.unmount()
    expect(remove).toHaveBeenCalledWith(widget)
    expect(disconnected).toHaveBeenCalledOnce()
  })

  it('reports a script failure and permits a new script request when retried', async () => {
    const onError = vi.fn()
    const first = render(<Turnstile siteKey={crypto.randomUUID()} action="login" onToken={vi.fn()} onError={onError} />)
    const script = document.querySelector('script[src*="challenges.cloudflare.com"]')
    expect(script).toHaveAttribute('src', 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit')
    if (!script) throw new Error('missing Turnstile script')
    fireEvent.error(script)
    await waitFor(() => expect(onError).toHaveBeenCalledOnce())
    expect(script.isConnected).toBe(false)
    first.unmount()
    const second = render(<Turnstile siteKey={crypto.randomUUID()} action="login" onToken={vi.fn()} onError={onError} />)
    const retried = document.querySelector('script[src*="challenges.cloudflare.com"]')
    expect(retried).not.toBe(script)
    if (!retried) throw new Error('missing retried script')
    fireEvent.error(retried)
    await waitFor(() => expect(onError).toHaveBeenCalledTimes(2))
    second.unmount()
  })
})
