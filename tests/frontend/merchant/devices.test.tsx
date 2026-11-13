import { chooseSelect } from '../portal-controls'
import { emptyNotifications } from './notification-fixture'
import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { publicConfigHandler } from '../public-config-fixture'
import { useState, type ReactNode } from 'react'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { APIError, api, onUnauthorized } from '@/api/client'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import { BindingDialog } from '@/features/merchant/DevicesPage'
import { IntegrationPage, SecurityDialog } from '@/features/merchant/IntegrationPage'
import { centsToYuan, collectionInput, invalidateCollection, receiptSearch, uploadError, validWhitelist, yuanToCents, type Schema } from '@/features/merchant/collection-data'
import { createI18n } from '@/lib/i18n'

vi.mock('qrcode', () => ({ default: { toDataURL: () => Promise.resolve('data:image/png;base64,AA==') } }))
const user: Schema['AuthUserDTO'] = { id: '1', role: 'merchant', username: 'device_shop', email: 'device_shop@example.test', pid: '9000000000000001', banned: false, banned_at: null, ban_reason: null, totp_enabled: false, locale: 'zh-CN', theme: 'system', created_at: new Date().toISOString() }
const identity: Schema['SessionIdentity'] = { user, csrf_token: crypto.randomUUID(), expires_at: new Date(Date.now() + 3600000).toISOString() }
const device: Schema['DeviceDeviceDTO'] = { id: null, name: null, app_version: null, bound: false, online: false, last_seen_at: null, offline_at: null, heartbeat_interval_seconds: 30, offline_threshold_seconds: 90 }
const settings: Schema['SettingsCollectionSettings'] = { version: 1, order_ttl_seconds: null, float_limit_cents: null, float_direction: null, bounds: { ttl_max_seconds: 600, float_min_cents: 1, float_max_cents: 50 }, effective: { order_ttl_seconds: 300, float_limit_cents: 10, float_direction: 'up' } }
const success = <T,>(data: T) => HttpResponse.json({ data, error: null, request_id: crypto.randomUUID() })
const server = setupServer(
  emptyNotifications, publicConfigHandler,
  http.get('*/downloads/latest.json', () => new HttpResponse(null, { status: 404 })),
  http.get('*/api/v1/merchant/me', () => success(identity)),
  http.get('*/api/v1/merchant/device', () => success(device)),
  http.get('*/api/v1/merchant/receipts', () => success({ items: [], total: 0, next_cursor: null })),
  http.get('*/api/v1/merchant/collection', () => success({ codes: [], settings, device })),
  http.get('*/api/v1/merchant/collection/codes', () => success({ codes: [] })),
)
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => { server.resetHandlers(); vi.useRealTimers() })
afterAll(() => server.close())
function wrapper(children: ReactNode, client = createQueryClient()) {
  const rendered = render(<AppProviders i18n={createI18n()} queryClient={client}>{children}</AppProviders>)
  return { client, ...rendered }
}
function mount(path: string) {
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) })
  const client = createQueryClient()
  return { router, ...wrapper(<RouterProvider router={router} context={{ queryClient: client }} />, client) }
}
function binding(expires = 300): Schema['DeviceBindingDTO'] { return { binding_id: crypto.randomUUID(), code: crypto.randomUUID(), qr_content: 'vanillapay://bind?code=fixture', expires_at: new Date(Date.now() + expires * 1000).toISOString() } }

describe('monitor installer', () => {
  const manifest = {
    version: '1.0.0', file: 'vanillapay-monitor-1.0.0.apk', sha256: 'a'.repeat(64),
    size: 2_048, published_at: '2026-11-09T00:00:00Z',
  }
  it('shows the versioned same-origin download, file size and expandable checksum', async () => {
    server.use(http.get('*/downloads/latest.json', () => HttpResponse.json(manifest)))
    mount('/devices')
    const link = await screen.findByRole('link', { name: '下载 VanillaPay Monitor 1.0.0' })
    expect(link).toHaveAttribute('href', '/downloads/vanillapay-monitor-1.0.0.apk')
      expect(screen.getByText('装到收款手机上，打开后点上方「绑定设备」扫码。')).toBeVisible()
    expect(screen.getByText(/2,048/)).toBeVisible()
    await userEvent.click(screen.getByText('查看 SHA-256'))
    expect(screen.getByText(manifest.sha256)).toBeVisible()
  })
  it.each([null, { ...manifest, file: 'https://elsewhere.test/download.apk' }])('treats a missing or invalid installer as unpublished', async (value) => {
    if (value) server.use(http.get('*/downloads/latest.json', () => HttpResponse.json(value)))
    mount('/devices')
    expect(await screen.findByText('安装包暂未发布')).toBeVisible()
    expect(screen.queryByRole('link', { name: /下载 VanillaPay/ })).not.toBeInTheDocument()
  })
})

describe('binding lifecycle', () => {
  it('invalidates a late binding creation once without polling after unmount', async () => {
    let release!: (value: Schema['DeviceBindingDTO']) => void
    const create = vi.spyOn(api, 'createBinding').mockImplementation(() => new Promise((resolve) => { release = resolve }))
    const poll = vi.spyOn(api, 'binding').mockResolvedValue({ state: 'pending', device: null, expires_at: binding().expires_at })
    const changed = vi.fn().mockResolvedValue(undefined)
    const { unmount } = wrapper(<BindingDialog replace={false} onClose={() => {}} onChanged={changed} />)
    fireEvent.click(screen.getByRole('button', { name: '生成绑定码' }))
    expect(create).toHaveBeenCalledOnce()
    unmount()
    await act(() => { release(binding()); return Promise.resolve() })
    expect(changed).toHaveBeenCalledOnce()
    expect(poll).not.toHaveBeenCalled()
  })

  it.each(['close', 'expire and regenerate'] as const)('ignores a late consumed response after %s', async (exit) => {
    vi.useFakeTimers()
    const old = binding(1), next = binding()
    const changed = vi.fn().mockResolvedValue(undefined)
    let release!: (value: Awaited<ReturnType<typeof api.binding>>) => void
    vi.spyOn(api, 'createBinding').mockResolvedValueOnce(old).mockResolvedValueOnce(next)
    const poll = vi.spyOn(api, 'binding')
      .mockImplementationOnce(() => new Promise((resolve) => { release = resolve }))
      .mockResolvedValue({ state: 'pending', device: null, expires_at: next.expires_at })
    function Host() {
      const [open, setOpen] = useState(true)
      return open ? <BindingDialog replace={false} onClose={() => setOpen(false)} onChanged={changed} /> : null
    }
    wrapper(<Host />)
    await act(() => { fireEvent.click(screen.getByRole('button', { name: '生成绑定码' })); return Promise.resolve() })
    expect(screen.getByTestId('binding-code')).toHaveTextContent(old.code)
    const signal = poll.mock.calls[0]?.[1]
    expect(signal?.aborted).toBe(false)
    if (exit === 'close') {
      fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    } else {
      await act(() => vi.advanceTimersByTimeAsync(1000))
      await act(() => { fireEvent.click(screen.getByRole('button', { name: '重新生成' })); return Promise.resolve() })
      expect(screen.getByTestId('binding-code')).toHaveTextContent(next.code)
    }
    expect(signal?.aborted).toBe(true)
    await act(() => { release({ state: 'consumed', device: null, expires_at: old.expires_at }); return Promise.resolve() })
    expect(changed).toHaveBeenCalledTimes(exit === 'close' ? 1 : 2)
    expect(screen.queryByText('设备已绑定')).not.toBeInTheDocument()
    if (exit === 'close') expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    else expect(screen.getByTestId('binding-code')).toHaveTextContent(next.code)
  })

  it.each(['consumed', 'expired'])('polls pending then %s, refreshes on consumption and stops', async (state) => {
    const value = binding(), changed = vi.fn().mockResolvedValue(undefined)
    vi.spyOn(api, 'createBinding').mockResolvedValue(value)
    const poll = vi.spyOn(api, 'binding').mockResolvedValueOnce({ state: 'pending', device: null, expires_at: value.expires_at }).mockResolvedValue({ state, device: null, expires_at: value.expires_at })
    wrapper(<BindingDialog replace={false} onClose={() => {}} onChanged={changed} />)
    fireEvent.click(screen.getByRole('button', { name: '生成绑定码' }))
    await screen.findByText(/等待绑定/)
    await waitFor(() => expect(poll).toHaveBeenCalledTimes(1))
    await screen.findByText(state === 'consumed' ? '设备已绑定' : '绑定码已过期', {}, { timeout: 4000 })
    expect(changed).toHaveBeenCalledTimes(state === 'consumed' ? 2 : 1)
    const count = poll.mock.calls.length
    vi.useFakeTimers(); await act(() => vi.advanceTimersByTimeAsync(6000))
    expect(poll).toHaveBeenCalledTimes(count)
    if (state === 'expired') { fireEvent.click(screen.getByRole('button', { name: '重新生成' })); await act(() => Promise.resolve()); expect(api.createBinding).toHaveBeenCalledTimes(2) }
  })
  it('aborts an in-flight request and does not schedule another poll after unmount', async () => {
    const value = binding()
    vi.spyOn(api, 'createBinding').mockResolvedValue(value)
    let signal: AbortSignal | undefined
    const poll = vi.spyOn(api, 'binding').mockImplementation((_id, passed) => { signal = passed; return new Promise(() => {}) })
    const { unmount } = wrapper(<BindingDialog replace={false} onClose={() => {}} onChanged={() => Promise.resolve()} />)
    fireEvent.click(screen.getByRole('button', { name: '生成绑定码' }))
    await screen.findByText(/等待绑定/)
    unmount(); expect(signal?.aborted).toBe(true)
    vi.useFakeTimers(); await act(() => vi.advanceTimersByTimeAsync(6000))
    expect(poll).toHaveBeenCalledTimes(1)
  })
  it('expires locally even when the status request is stalled', async () => {
    vi.spyOn(api, 'createBinding').mockResolvedValue(binding(1))
    let signal: AbortSignal | undefined
    vi.spyOn(api, 'binding').mockImplementation((_id, passed) => { signal = passed; return new Promise(() => {}) })
    wrapper(<BindingDialog replace={false} onClose={() => {}} onChanged={() => Promise.resolve()} />)
    fireEvent.click(screen.getByRole('button', { name: '生成绑定码' }))
    await screen.findByText(/等待绑定/)
    await screen.findByText('绑定码已过期', {}, { timeout: 2500 })
    expect(signal?.aborted).toBe(true)
  })
  it('requires explicit replacement confirmation', async () => {
    const create = vi.spyOn(api, 'createBinding').mockResolvedValue(binding())
    vi.spyOn(api, 'binding').mockImplementation(() => new Promise(() => {}))
    wrapper(<BindingDialog replace onClose={() => {}} onChanged={() => Promise.resolve()} />)
    expect(screen.getByRole('button', { name: '生成绑定码' })).toBeDisabled()
    await userEvent.click(screen.getByRole('checkbox')); await userEvent.click(screen.getByRole('button', { name: '生成绑定码' }))
    expect(create).toHaveBeenCalledWith(true)
  })
})

describe('security proof and one-time key', () => {
  const integration: Schema['Integration'] = {
    pid: user.pid!, key: 'fixture-current-key', key_version: 1, base_url: 'https://example.test',
    v1: { submit: '', mapi: '', query: '', refund: '', info: '', orders: '' }, v2: { submit: '', create: '', query: '', refund: '', info: '', orders: '', close: '', refundquery: '' },
    merchant_public_key: null, platform_public_keys: [], ip_whitelist: { enabled: false, entries: [] },
  }

  it.each(['open and close reset', 'leave and return'] as const)('does not display or cache a late key read after %s', async (exit) => {
    let release!: (value: Schema['Integration']) => void
    const lateKey = crypto.randomUUID()
    const get = vi.spyOn(api, 'integration').mockResolvedValue(integration)
      .mockImplementationOnce(() => Promise.resolve(integration))
      .mockImplementationOnce(() => new Promise((resolve) => { release = resolve }))
    function Host() {
      const [open, setOpen] = useState(true)
      return <><button onClick={() => setOpen(!open)}>{open ? '离开' : '返回'}</button>{open && <IntegrationPage user={user} />}</>
    }
    const { client } = wrapper(<Host />)
    fireEvent.click(await screen.findByRole('button', { name: '显示 Key' }))
    expect(get).toHaveBeenCalledTimes(2)
    if (exit === 'open and close reset') {
      fireEvent.click(screen.getByRole('button', { name: /^重置 Key$/ }))
      fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    } else {
      fireEvent.click(screen.getByRole('button', { name: '离开' }))
      fireEvent.click(screen.getByRole('button', { name: '返回' }))
    }
    await act(() => { release({ ...integration, key: lateKey }); return Promise.resolve() })
    expect(await screen.findByRole('button', { name: '显示 Key' })).toBeEnabled()
    expect(screen.queryByText(lateKey)).not.toBeInTheDocument()
    expect(JSON.stringify(client.getQueryCache().getAll())).not.toContain(lateKey)
    expect(JSON.stringify(client.getMutationCache().getAll())).not.toContain(lateKey)
  })

  it('invalidates a late key reset once without exposing the key or proof in a rebuilt dialog', async () => {
    const key = crypto.randomUUID(), changed = vi.fn().mockResolvedValue(undefined)
    let release!: (value: Awaited<ReturnType<typeof api.resetKey>>) => void
    const reset = vi.spyOn(api, 'resetKey').mockImplementation(() => new Promise((resolve) => { release = resolve }))
    function Host({ generation }: { generation: number }) {
      return <SecurityDialog key={generation} mode="reset" user={user} onClose={() => {}} onChanged={changed} />
    }
    const client = createQueryClient(), i18n = createI18n()
    const view = (generation: number) => <AppProviders i18n={i18n} queryClient={client}><Host generation={generation} /></AppProviders>
    const { rerender } = render(view(1))
    fireEvent.change(screen.getByLabelText('邮箱验证码'), { target: { value: '123456' } })
    fireEvent.click(screen.getByRole('button', { name: '确认重置 Key' }))
    expect(reset).toHaveBeenCalledOnce()
    rerender(view(2))
    await act(() => { release({ key, pid: user.pid!, key_version: 2 }); return Promise.resolve() })
    expect(changed).toHaveBeenCalledOnce()
    expect(screen.getByLabelText('邮箱验证码')).toHaveValue('')
    expect(screen.queryByTestId('one-time-key')).not.toBeInTheDocument()
    expect(JSON.stringify(client.getQueryCache().getAll())).not.toContain(key)
    expect(JSON.stringify(client.getMutationCache().getAll())).not.toContain(key)
  })

  it('keeps the returned key out of caches/storage and destroys the display when closed', async () => {
    const secret = crypto.randomUUID(), changed = vi.fn().mockResolvedValue(undefined)
    vi.spyOn(api, 'resetKey').mockResolvedValue({ key: secret, pid: user.pid!, key_version: 2 })
    function Host() {
      const [open, setOpen] = useState(true)
      return <><button onClick={() => setOpen(true)}>打开</button>{open && <SecurityDialog mode="reset" user={user} onClose={() => setOpen(false)} onChanged={changed} />}</>
    }
    const { client } = wrapper(<Host />)
    await userEvent.type(screen.getByLabelText('邮箱验证码'), '123456')
    await userEvent.click(screen.getByRole('button', { name: '确认重置 Key' }))
    expect(await screen.findByTestId('one-time-key')).toHaveTextContent(secret)
    expect(JSON.stringify(client.getQueryCache().getAll())).not.toContain(secret)
    expect(client.getMutationCache().getAll()).toHaveLength(0)
    expect(JSON.stringify(localStorage)).not.toContain(secret)
    await userEvent.click(screen.getByRole('button', { name: '关闭' }))
    expect(screen.queryByText(secret)).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '打开' }))
    expect(screen.queryByTestId('one-time-key')).not.toBeInTheDocument()
    expect(screen.getByLabelText('邮箱验证码')).toHaveValue('')
  })
  it('discards the integration endpoint key before caching and only reveals on demand', async () => {
    const secret = crypto.randomUUID()
    const data: Schema['Integration'] = { pid: user.pid!, key: secret, key_version: 1, base_url: 'https://example.test', v1: { submit: '', mapi: '', query: '', refund: '', info: '', orders: '' }, v2: { submit: '', create: '', query: '', refund: '', info: '', orders: '', close: '', refundquery: '' }, merchant_public_key: null, platform_public_keys: [], ip_whitelist: { enabled: false, entries: [] } }
    const get = vi.spyOn(api, 'integration').mockResolvedValue(data)
    const { client } = wrapper(<IntegrationPage user={user} />)
    await screen.findByText('商户凭据')
    expect(JSON.stringify(client.getQueryData(['merchant', user.username, 'integration']))).not.toContain(secret)
    expect(screen.queryByText(secret)).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '显示 Key' }))
    expect(await screen.findByText(secret)).toBeVisible(); expect(get).toHaveBeenCalledTimes(2)
    await userEvent.click(screen.getByRole('button', { name: '隐藏 Key' }))
    expect(screen.queryByText(secret)).not.toBeInTheDocument()
  })
  it('uses the current merchant email/reset_key purpose, requires TOTP and honors retry_after_seconds', async () => {
    const send = vi.spyOn(api, 'emailCode').mockResolvedValue({ sent: true, retry_after_seconds: 3 })
    const reset = vi.spyOn(api, 'resetKey').mockResolvedValue({ key: crypto.randomUUID(), pid: user.pid!, key_version: 2 })
    wrapper(<SecurityDialog mode="reset" user={{ ...user, totp_enabled: true }} onClose={() => {}} onChanged={() => Promise.resolve()} />)
    await userEvent.click(screen.getByRole('button', { name: '获取邮箱验证码' }))
    expect(send).toHaveBeenCalledWith({ email: user.email, purpose: 'reset_key' })
    expect(await screen.findByRole('button', { name: /^0:0[1-3]$/ })).toBeDisabled()
    await userEvent.type(screen.getByLabelText('邮箱验证码'), '123456')
    expect(screen.getByRole('button', { name: '确认重置 Key' })).toBeDisabled()
    await userEvent.type(screen.getByLabelText('动态验证码'), '654321')
    await userEvent.click(screen.getByRole('button', { name: '确认重置 Key' }))
    expect(reset).toHaveBeenCalledWith({ email_code: '123456', totp_code: '654321' })
  })
  it('handles 429 cooldown and allows sending again after it expires', async () => {
    vi.spyOn(api, 'emailCode').mockRejectedValue(new APIError(429, { code: 'EMAIL_SEND_TOO_SOON', message: '', fields: {}, details: { retry_after_seconds: 1 } }))
    wrapper(<SecurityDialog mode="rsa" user={user} onClose={() => {}} onChanged={() => Promise.resolve()} />)
    await userEvent.click(screen.getByRole('button', { name: '获取邮箱验证码' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('操作过于频繁')
    expect(screen.getByRole('button', { name: /^0:0[1-3]$/ })).toBeDisabled()
    await waitFor(() => expect(screen.getByRole('button', { name: '获取邮箱验证码' })).toBeEnabled(), { timeout: 2500 })
  })
  it('does not log out the current session for an invalid second factor', async () => {
    const unauthorized = vi.fn(); onUnauthorized(unauthorized)
    await api.me()
    server.use(http.post('*/api/v1/merchant/integration/reset-key', () => HttpResponse.json({ error: { code: 'INVALID_2FA', fields: {}, details: {}, message: '' } }, { status: 401 })))
    await expect(api.resetKey({ email_code: '123456', totp_code: '123456' })).rejects.toMatchObject({ status: 401 })
    expect(unauthorized).not.toHaveBeenCalled()
  })
})

describe('collection bounds and cache invalidation', () => {
  it('sends all four order writes with the caller idempotency keys and session credentials', async () => {
    await api.me()
    const fetchOptions = vi.spyOn(globalThis, 'fetch').mockImplementation(() => Promise.resolve(success(null)))
    const keys = ['supplement-key', 'refund-key', 'resolve-key', 'replay-key']
    await api.supplement('trade-1', { reason: 'fixture' }, keys[0]!)
    await api.refund('trade-1', { reason: 'fixture', confirm: true }, keys[1]!)
    await api.resolveRefund('refund-1', { reason: 'fixture', confirm: false, action: 'reject' }, keys[2]!)
    await api.replayCallback('trade-1', keys[3]!)
    expect(fetchOptions.mock.calls.map(([url, options]) => [
      url, options?.method, options?.credentials,
      new Headers(options?.headers).get('Idempotency-Key'),
    ])).toEqual([
      ['/api/v1/merchant/orders/trade-1/supplement', 'POST', 'include', keys[0]],
      ['/api/v1/merchant/orders/trade-1/refund', 'POST', 'include', keys[1]],
      ['/api/v1/merchant/refunds/refund-1/resolve', 'POST', 'include', keys[2]],
      ['/api/v1/merchant/orders/trade-1/callback-replay', 'POST', 'include', keys[3]],
    ])
  })

  it('lets fetch set the multipart content type and retains CSRF for PUT, PATCH and DELETE', async () => {
    await api.me()
    const fetchOptions = vi.spyOn(globalThis, 'fetch').mockImplementation(() => Promise.resolve(success(null)))
    const file = new File(['fixture'], 'code.png', { type: 'image/png' })
    await api.uploadQR('alipay', file)
    await api.setQREnabled('alipay', true)
    await api.unbindDevice()
    expect(fetchOptions.mock.calls.map(([, options]) => [
      options?.method, new Headers(options?.headers).get('X-CSRF-Token'),
    ])).toEqual(['PUT', 'PATCH', 'DELETE'].map((method) => [method, identity.csrf_token]))
    const multipart = fetchOptions.mock.calls[0]?.[1]
    expect(new Headers(multipart?.headers).has('Content-Type')).toBe(false)
    expect(multipart?.body).toBeInstanceOf(FormData)
    expect((multipart?.body as FormData).get('file')).toBe(file)
  })

  it('notifies global logout once for a protected UNAUTHENTICATED response', async () => {
    await api.me()
    const unauthorized = vi.fn()
    onUnauthorized(unauthorized)
    server.use(http.get('*/api/v1/merchant/device', () => HttpResponse.json({
      error: { code: 'UNAUTHENTICATED', message: '' },
    }, { status: 401 })))
    try {
      await expect(api.device()).rejects.toMatchObject({ status: 401, problem: { code: 'UNAUTHENTICATED' } })
      expect(unauthorized).toHaveBeenCalledOnce()
    } finally {
      onUnauthorized(() => {})
    }
  })

  it('converts yuan/cents exactly and validates nullable settings against response bounds', () => {
    expect(yuanToCents('0.29')).toBe(29); expect(yuanToCents('1.01')).toBe(101); expect(centsToYuan(29)).toBe('0.29')
    for (const value of ['1.001', '-1', '1e2', 'NaN', '9007199254740992']) expect(yuanToCents(value)).toBeNull()
    expect(collectionInput('', '', '', settings.bounds)).toEqual({ order_ttl_seconds: null, float_limit_cents: null, float_direction: null })
    expect(collectionInput('600', '0.50', 'down', settings.bounds)).toEqual({ order_ttl_seconds: 600, float_limit_cents: 50, float_direction: 'down' })
    for (const [ttl, amount, direction] of [['59', '0.10', 'up'], ['601', '0.10', 'up'], ['60.5', '0.10', 'up'], ['60', '0.00', 'up'], ['60', '0.51', 'up'], ['60', '0.1', 'sideways']]) expect(collectionInput(ttl!, amount!, direction!, settings.bounds)).toBeNull()
  })
  it('limits upload type/size and rejects empty enabled whitelists', () => {
    expect(uploadError(new File(['x'], 'code.svg', { type: 'image/svg+xml' }))).not.toBeNull()
    expect(uploadError(new File([new Uint8Array(5 * 1024 * 1024 + 1)], 'code.png', { type: 'image/png' }))).not.toBeNull()
    expect(validWhitelist([], true)).toBe(false); expect(validWhitelist([], false)).toBe(true)
    expect(validWhitelist(['192.0.2.1', '2001:db8::/32', '::ffff:192.0.2.1/128'], true)).toBe(true)
    expect(validWhitelist(['999.0.0.1'], true)).toBe(false)
  })
  it('invalidates every related merchant prefix and every stats range without affecting another merchant', async () => {
    const client = createQueryClient()
    const parts = ['collection', 'codes', 'device', 'receipts', 'orders', 'order', 'integration', 'stats']
    for (const part of parts) for (const range of [1, 7, 30]) client.setQueryData(['merchant', user.username, part, range], {})
    client.setQueryData(['merchant', 'other', 'stats', 1], {})
    await invalidateCollection(client, user.username)
    for (const part of parts) for (const range of [1, 7, 30]) expect(client.getQueryState(['merchant', user.username, part, range])?.isInvalidated).toBe(true)
    expect(client.getQueryState(['merchant', 'other', 'stats', 1])?.isInvalidated).toBe(false)
  })
  it('sends collection settings through PUT with CSRF and invalidates stats on success', async () => {
    let payload: unknown, csrf: string | null = null
    server.use(http.put('*/api/v1/merchant/collection/settings', async ({ request }) => { payload = await request.json(); csrf = request.headers.get('X-CSRF-Token'); return success(settings) }))
    const { client } = mount('/codes')
    await screen.findByLabelText('浮动上限（元）')
    client.setQueryData(['merchant', user.username, 'stats', { days: 7 }], {})
    await userEvent.type(screen.getByLabelText('浮动上限（元）'), '0.29')
    await userEvent.click(screen.getByRole('button', { name: '保存收款设置' }))
    await userEvent.click(screen.getByRole('button', { name: '确认保存' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(payload).toEqual({ expected_version: settings.version, order_ttl_seconds: null, float_limit_cents: 29, float_direction: null })
    expect(csrf).toBe(identity.csrf_token)
    expect(client.getQueryState(['merchant', user.username, 'stats', { days: 7 }])?.isInvalidated).toBe(true)
  })
  it('reloads changed settings after 409 and keeps the conflict notice visible', async () => {
    const versions: number[] = []
    server.use(http.get('*/api/v1/merchant/collection', () => success({ codes: [], device, settings: versions.length ? { ...settings, version: 2, float_limit_cents: 15 } : settings })), http.put('*/api/v1/merchant/collection/settings', async ({ request }) => {
      const body = await request.json() as Schema['SettingsSaveCollectionRequest']
      versions.push(body.expected_version)
      if (versions.length > 1) return success({ ...settings, version: 3, float_limit_cents: 15 })
      return HttpResponse.json({ error: { code: 'VERSION_CONFLICT', fields: {}, details: {}, message: '' } }, { status: 409 })
    }))
    mount('/codes')
    await userEvent.type(await screen.findByLabelText('浮动上限（元）'), '0.29')
    await userEvent.click(screen.getByRole('button', { name: '保存收款设置' }))
    await userEvent.click(screen.getByRole('button', { name: '确认保存' }))
    await waitFor(() => expect(screen.getByLabelText('浮动上限（元）')).toHaveValue('0.15'))
    expect(screen.getByText('设置已被其他会话修改，已载入最新值，请确认后再保存。')).toBeVisible()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(versions).toEqual([settings.version])
    await userEvent.click(screen.getByRole('button', { name: '保存收款设置' }))
    await userEvent.click(screen.getByRole('button', { name: '确认保存' }))
    await waitFor(() => expect(versions).toEqual([settings.version, 2]))
  })
  it.each(['toggle', 'unbind', 'binding', 'reset', 'rsa', 'whitelist'] as const)('invalidates merchant stats after the actual %s write flow', async (kind) => {
    const integration: Schema['Integration'] = { pid: user.pid!, key: crypto.randomUUID(), key_version: 1, base_url: 'https://example.test', v1: { submit: '', mapi: '', query: '', refund: '', info: '', orders: '' }, v2: { submit: '', create: '', query: '', refund: '', info: '', orders: '', close: '', refundquery: '' }, merchant_public_key: null, platform_public_keys: [], ip_whitelist: { enabled: false, entries: [] } }
    vi.spyOn(api, 'integration').mockResolvedValue(integration)
    vi.spyOn(api, 'resetKey').mockResolvedValue({ pid: user.pid!, key: crypto.randomUUID(), key_version: 2 })
    vi.spyOn(api, 'saveRSA').mockResolvedValue({ fingerprint: 'fixture', updated_at: new Date().toISOString() })
    vi.spyOn(api, 'saveWhitelist').mockResolvedValue({ enabled: false, entries: [] })
    vi.spyOn(api, 'unbindDevice').mockResolvedValue(null)
    vi.spyOn(api, 'createBinding').mockResolvedValue(binding())
    vi.spyOn(api, 'binding').mockImplementation(() => new Promise(() => {}))
    vi.spyOn(api, 'setQREnabled').mockResolvedValue({ channel: 'alipay', asset_id: 'asset', image_url: null, updated_at: null, enabled: false })
    server.use(http.get('*/api/v1/merchant/device', () => success({ ...device, bound: kind === 'unbind' })), http.get('*/api/v1/merchant/collection/codes', () => success({ codes: [{ channel: 'alipay', asset_id: 'asset', image_url: null, enabled: true, updated_at: null }] })))
    const path = kind === 'toggle' ? '/codes' : kind === 'unbind' || kind === 'binding' ? '/devices' : '/integration'
    const { client } = mount(path)
    await screen.findByRole('heading', { name: path === '/codes' ? '收款设置' : path === '/devices' ? '收款监控端' : '商户凭据' })
    const key = ['merchant', user.username, 'stats', { range: 'previous' }]
    client.setQueryData(key, {})
    const actor = userEvent.setup()
    if (kind === 'toggle') { await actor.click(screen.getByRole('button', { name: '停用' })); await actor.click(screen.getByRole('button', { name: '确认保存' })) }
    if (kind === 'unbind') { await actor.click(await screen.findByRole('button', { name: '解绑设备' })); await actor.click(screen.getByRole('checkbox')); await actor.click(screen.getByRole('button', { name: '确认解绑' })) }
    if (kind === 'binding') { await actor.click(await screen.findByRole('button', { name: '绑定设备' })); await actor.click(screen.getByRole('button', { name: '生成绑定码' })) }
    if (kind === 'reset' || kind === 'rsa') {
      await actor.click(screen.getByRole('button', { name: kind === 'reset' ? '重置 Key' : '保存商户公钥' }))
      if (kind === 'rsa') await actor.type(screen.getByLabelText('RSA 公钥 PEM'), '-----BEGIN PUBLIC KEY-----\nYWJj\n-----END PUBLIC KEY-----')
      await actor.type(screen.getByLabelText('邮箱验证码'), '123456')
      await actor.click(screen.getByRole('button', { name: kind === 'reset' ? '确认重置 Key' : '确认保存公钥' }))
    }
    if (kind === 'whitelist') { await actor.click(screen.getByRole('button', { name: '保存白名单' })); await actor.click(screen.getByRole('button', { name: '确认保存白名单' })) }
    await waitFor(() => expect(client.getQueryState(key)?.isInvalidated).toBe(true))
  })
})

describe('receipt URL filters and pagination', () => {
  it('appends cursor pages, resets on filters and renders raw receipt text without HTML', async () => {
    const row: Schema['PortalReceiptDTO'] = { id: 'receipt-1', event_id: crypto.randomUUID(), device_id: 'device', channel: 'alipay', amount_cents: 123, occurred_at: new Date().toISOString(), received_at: new Date().toISOString(), provider_trade_no: null, payer_hint: null, match_state: 'unmatched', unmatched_reason: 'no_matching_order', trade_no: null }
    const urls: URL[] = []
    const raw = '<script>alert(1)</script><img src=x onerror=alert(1)>'
    server.use(http.get('*/api/v1/merchant/receipts', ({ request }) => { const url = new URL(request.url); urls.push(url); return success({ items: [{ ...row, id: url.searchParams.has('cursor') ? 'receipt-2' : row.id }], total: 2, next_cursor: url.searchParams.has('cursor') ? null : 'next-fixture' }) }), http.get('*/api/v1/merchant/receipts/:id', () => success({ ...row, raw_text: raw })))
    mount('/devices')
    await screen.findAllByRole('button', { name: 'receipt-1' })
    await userEvent.click(screen.getByRole('button', { name: '加载更多' }))
    await screen.findAllByRole('button', { name: 'receipt-2' })
    expect(urls.at(-1)!.searchParams.get('cursor')).toBe('next-fixture')
    await chooseSelect(userEvent.setup(), screen.getByLabelText('匹配状态'), 'unmatched')
    await userEvent.click(screen.getByRole('button', { name: '应用筛选' }))
    await waitFor(() => expect(urls.at(-1)!.searchParams.get('match_state')).toBe('unmatched'))
    expect(urls.at(-1)!.searchParams.has('cursor')).toBe(false)
    expect(screen.queryAllByRole('button', { name: 'receipt-2' })).toHaveLength(0)
    await userEvent.click(screen.getAllByRole('button', { name: 'receipt-1' })[0]!)
    await screen.findByText(raw)
    expect(screen.getByRole('dialog').querySelectorAll('img,script')).toHaveLength(0)
  })
  it('normalizes search fields and synchronizes shared filters and changes to the URL', async () => {
    expect(receiptSearch({ channel: ' alipay ', match_state: 'matched', ignored: 1 })).toEqual({ channel: 'alipay', match_state: 'matched' })
    const requests: URL[] = []
    server.use(http.get('*/api/v1/merchant/receipts', ({ request }) => { requests.push(new URL(request.url)); return success({ items: [], total: 0, next_cursor: null }) }))
    const { router } = mount('/devices?channel=alipay&match_state=matched&start=2026-11-01T00%3A00%3A00Z')
    expect(await screen.findByLabelText('匹配状态')).toHaveAttribute('data-value', 'matched')
    expect(screen.getByLabelText('收款渠道')).toHaveAttribute('data-value', 'alipay')
    await chooseSelect(userEvent.setup(), screen.getByLabelText('匹配状态'), 'unmatched')
    fireEvent.change(screen.getByLabelText('结束时间'), { target: { value: '2026-11-04T10:30' } })
    await userEvent.click(screen.getByRole('button', { name: '应用筛选' }))
    await waitFor(() => expect(router.state.location.search.match_state).toBe('unmatched'))
    expect(router.state.location.search.end).toBe(new Date('2026-11-04T10:30').toISOString())
    await waitFor(() => expect(requests.at(-1)!.searchParams.get('match_state')).toBe('unmatched'))
    expect(requests.at(-1)!.searchParams.has('cursor')).toBe(false)
    await userEvent.click(screen.getByRole('button', { name: '重置' }))
    await waitFor(() => expect(router.state.location.search).toEqual({}))
  })
})
