import { chooseSelect } from '../portal-controls'
import { emptyNotifications } from './notification-fixture'
import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { publicConfigHandler } from '../public-config-fixture'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import { APIError } from '@/api/client'
import type { components } from '@/api/schema'
import { availableOrderActions, orderErrorKey, validReason } from '@/features/merchant/orders-data'
import { previousRange, statsRange } from '@/features/merchant/overview-data'
import { createI18n } from '@/lib/i18n'
import { order } from './fixtures'

type Schema = components['schemas']
const identity: Schema['SessionIdentity'] = { user: { id: '1', role: 'merchant', username: 'orders_shop', pid: '9000000000000001', banned: false, banned_at: null, ban_reason: null, totp_enabled: false, locale: 'zh-CN', theme: 'system', created_at: new Date().toISOString() }, csrf_token: crypto.randomUUID(), expires_at: new Date(Date.now() + 3600000).toISOString() }
function detail(status = 'pending', channel: string | null = 'alipay'): Schema['OrderDetail'] {
  return { order: order(status, channel), notify_url: 'https://callback.example.test/notify', return_url: 'https://shop.example.test/return', clientip: null, key_version: null, receipt_id: null, callback_attempts: [], refund: null,
    allowed_channels_snapshot: ['alipay', 'wxpay'], callback_replays_used: 0, callback_replay_limit: 2147483646 }
}
const success = <T,>(data: T) => HttpResponse.json({ data, error: null, request_id: crypto.randomUUID() })
const failure = (status: number, code: Schema['ErrorCode']) => HttpResponse.json({ error: { code, fields: {}, details: {}, message: code }, request_id: crypto.randomUUID() }, { status })
const server = setupServer(emptyNotifications, publicConfigHandler,
  http.get('*/api/v1/merchant/me', () => success(identity)),
  http.get('*/api/v1/merchant/orders', () => success({ items: [], total: 0, next_cursor: null })),
)
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => { server.resetHandlers(); vi.restoreAllMocks() })
afterAll(() => server.close())
function mount(path = '/orders') {
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) })
  const client = createQueryClient()
  render(<AppProviders i18n={createI18n()} queryClient={client}><RouterProvider router={router} context={{ queryClient: client }} /></AppProviders>)
  return { router, client }
}

describe('order filters and cursor pagination', () => {
  it('keeps an unsafe integer as text in a direct order search, list and CSV export', async () => {
    const query = '9007199254740993'
    const lists: URL[] = [], exports: URL[] = []
    server.use(http.get('*/api/v1/merchant/orders', ({ request }) => {
      lists.push(new URL(request.url))
      return success({ items: [], total: 0, next_cursor: null })
    }), http.get('*/api/v1/merchant/orders/export.csv', ({ request }) => {
      exports.push(new URL(request.url))
      return failure(409, 'VERSION_CONFLICT')
    }))
    mount('/orders?query=' + query)
    expect(await screen.findByLabelText('搜索订单')).toHaveValue(query)
    await waitFor(() => expect(lists[0]?.searchParams.get('query')).toBe(query))
    await userEvent.click(screen.getByRole('button', { name: '导出 CSV' }))
    await screen.findByRole('alert')
    expect(exports[0]?.searchParams.get('query')).toBe(query)
  })

  it('clears unsubmitted fields when reset keeps the same empty URL', async () => {
    const { router } = mount()
    const actor = userEvent.setup()
    await actor.type(await screen.findByLabelText('搜索订单'), 'unsubmitted')
    await chooseSelect(actor, screen.getByLabelText('状态'), 'paid')
    await actor.click(screen.getByRole('button', { name: '重置' }))
    expect(screen.getByLabelText('搜索订单')).toHaveValue('')
    expect(screen.getByLabelText('状态')).toHaveAttribute('data-value', '')
    expect(router.state.location.search).toEqual({})
  })
  it('restores shared URL filters and writes changed filters back as RFC3339', async () => {
    const requests: URL[] = []
    server.use(http.get('*/api/v1/merchant/orders', ({ request }) => { requests.push(new URL(request.url)); return success({ items: [], total: 0, next_cursor: null }) }))
    const { router } = mount('/orders?status=paid&channel=alipay&query=shared&start=2026-11-01T00%3A00%3A00Z&end=2026-11-04T00%3A00%3A00Z')
    expect(await screen.findByLabelText('状态')).toHaveAttribute('data-value', 'paid')
    expect(screen.getByLabelText('通道')).toHaveAttribute('data-value', 'alipay')
    expect(screen.getByLabelText('搜索订单')).toHaveValue('shared')
    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]!.searchParams.get('start')).toBe('2026-11-01T00:00:00Z')
    fireEvent.change(screen.getByLabelText('开始时间'), { target: { value: '2026-11-02T10:30' } })
    fireEvent.change(screen.getByLabelText('结束时间'), { target: { value: '2026-11-03T11:30' } })
    const actor = userEvent.setup()
    await chooseSelect(actor, screen.getByLabelText('状态'), 'expired')
    await actor.click(screen.getByRole('button', { name: '应用筛选' }))
    await waitFor(() => expect(router.state.location.search.status).toBe('expired'))
    expect(router.state.location.search.start).toBe(new Date('2026-11-02T10:30').toISOString())
    await actor.click(screen.getByRole('button', { name: '重置' }))
    await waitFor(() => expect(router.state.location.search).toEqual({}))
  })

  it('appends cursor pages and resets the cursor when changing or returning to a filter', async () => {
    const first = order('pending', null), second = order('pending', 'alipay')
    const urls: URL[] = []
    server.use(http.get('*/api/v1/merchant/orders', ({ request }) => {
      const url = new URL(request.url); urls.push(url)
      return success({ items: url.searchParams.has('cursor') ? [second] : [first], total: 2, next_cursor: url.searchParams.has('cursor') ? null : 'cursor-fixture' })
    }))
    mount()
    await screen.findAllByRole('link', { name: first.trade_no })
    const actor = userEvent.setup()
    await actor.click(screen.getByRole('button', { name: '加载更多' }))
    await screen.findAllByRole('link', { name: second.trade_no })
    expect(within(screen.getByRole('table')).getAllByRole('row')).toHaveLength(3)
    expect(urls.at(-1)!.searchParams.get('cursor')).toBe('cursor-fixture')
    await chooseSelect(actor, screen.getByLabelText('状态'), 'paid')
    await actor.click(screen.getByRole('button', { name: '应用筛选' }))
    await waitFor(() => expect(urls.at(-1)!.searchParams.get('status')).toBe('paid'))
    expect(urls.at(-1)!.searchParams.has('cursor')).toBe(false)
    const requestCount = urls.length
    await actor.click(screen.getByRole('button', { name: '重置' }))
    await waitFor(() => expect(within(screen.getByRole('table')).getAllByRole('row')).toHaveLength(2))
    expect(urls).toHaveLength(requestCount)
    await actor.click(screen.getByRole('button', { name: '加载更多' }))
    await screen.findAllByRole('link', { name: second.trade_no })
    expect(urls.at(-1)!.searchParams.has('status')).toBe(false)
    expect(urls.at(-1)!.searchParams.get('cursor')).toBe('cursor-fixture')
  })

  it('keeps the previous rows visible while the next filter is loading', async () => {
    const first = order('pending', null), next = order('paid', 'alipay')
    let release!: () => void
    const gate = new Promise<void>((resolve) => { release = resolve })
    server.use(http.get('*/api/v1/merchant/orders', async ({ request }) => {
      const filtered = new URL(request.url).searchParams.has('status')
      if (filtered) await gate
      return success({ items: [filtered ? next : first], total: 1, next_cursor: null })
    }))
    const { router } = mount()
    const actor = userEvent.setup()
    const list = () => within(screen.getByRole('table'))
    await screen.findAllByRole('link', { name: first.trade_no })
    try {
      await chooseSelect(actor, screen.getByLabelText('状态'), 'paid')
      await actor.click(screen.getByRole('button', { name: '应用筛选' }))
      await waitFor(() => expect(router.state.location.search.status).toBe('paid'))
      expect(list().getByRole('link', { name: first.trade_no })).toBeVisible()
      expect(screen.queryByText('没有符合筛选条件的订单')).not.toBeInTheDocument()
    } finally {
      release()
    }
    expect(await list().findByRole('link', { name: next.trade_no })).toBeVisible()
  })

  it('shows initial loading, an error with retry, and the empty result independently', async () => {
    let release: () => void = () => {}
    const gate = new Promise<void>((resolve) => { release = resolve })
    server.use(http.get('*/api/v1/merchant/orders', async () => { await gate; return failure(500, 'INTERNAL_ERROR') }))
    mount()
    expect(await screen.findByRole('status', { name: '正在加载…' })).toBeVisible()
    expect(screen.queryByText('没有符合筛选条件的订单')).not.toBeInTheDocument()
    release()
    expect(await screen.findByRole('alert')).toHaveTextContent('服务暂时不可用')
    server.use(http.get('*/api/v1/merchant/orders', () => success({ items: [], total: 0, next_cursor: null })))
    await userEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText('没有符合筛选条件的订单')).toBeVisible()
  })
})

describe('order actions', () => {
  it.each(['table', 'list'] as const)('opens supplement from the merchant %s with fresh detail and preserves the detail link', async (layout) => {
    const initial = detail('pending', null)
    const locked = { ...initial, order: { ...initial.order, channel: 'wxpay', actual_cents: 12992 } }
    let latest = locked
    const writes: unknown[] = []
    let reads = 0
    server.use(
      http.get('*/api/v1/merchant/orders', () => success({ items: [writes.length ? latest.order : initial.order], total: 1, next_cursor: null })),
      http.get('*/api/v1/merchant/orders/:trade', () => { reads++; return success(latest) }),
      http.post('*/api/v1/merchant/orders/:trade/supplement', async ({ request }) => {
        writes.push(await request.json())
        latest = { ...locked, order: { ...locked.order, status: 'paid' } }
        return success(latest.order)
      }),
    )
    const { client } = mount('/orders?status=pending')
    const detailKey = ['merchant', identity.user.username, 'order', initial.order.trade_no]
    const statsKey = ['merchant', identity.user.username, 'stats', { days: 7 }]
    client.setQueryData(detailKey, initial)
    client.setQueryData(statsKey, {})
    const rows = within(layout === 'table' ? await screen.findByRole('table') : await screen.findByRole('list', { name: '订单卡片' }))
    expect(rows.getByRole('link', { name: initial.order.trade_no })).toHaveAttribute('href', `/orders/${initial.order.trade_no}?status=pending`)
    await userEvent.click(rows.getByRole('button', { name: '补单' }))
    const dialog = within(await screen.findByRole('dialog', { name: '补单' }))
    expect(reads).toBe(1)
    expect(dialog.queryByRole('combobox')).not.toBeInTheDocument()
    expect(dialog.getByText('微信支付')).toBeVisible()
    expect(dialog.getByText(/129\.92/)).toBeVisible()
    expect(client.getQueryData(detailKey)).toEqual(locked)
    if (layout === 'list') return
    await userEvent.type(dialog.getByLabelText('原因', { exact: true }), 'Matched payment')
    await userEvent.click(dialog.getByRole('button', { name: '确认补单' }))
    expect(await screen.findByText('补单成功，订单已更新。')).toBeVisible()
    expect(reads).toBe(2)
    expect(writes).toEqual([{ channel: 'wxpay', reason: 'Matched payment', expected_amount_cents: 12992 }])
    expect(rows.queryByRole('button', { name: '补单' })).not.toBeInTheDocument()
    expect(client.getQueryState(detailKey)?.isInvalidated).toBe(true)
    expect(client.getQueryState(statsKey)?.isInvalidated).toBe(true)
  })

  it('keeps eligible orders read-only in the management list and detail', async () => {
    const value = detail()
    server.use(
      http.get('*/api/v1/console/me', () => success({ ...identity, user: { ...identity.user, role: 'admin' } })),
      http.get('*/api/v1/console/orders', () => success({ items: [value.order], total: 1, next_cursor: null })),
      http.get('*/api/v1/console/orders/:trade', () => success(value)),
    )
    mount('/console/orders')
    const rows = within(await screen.findByRole('table'))
    expect(screen.queryByRole('button', { name: '补单' })).not.toBeInTheDocument()
    await userEvent.click(rows.getByRole('link', { name: value.order.trade_no }))
    expect(await screen.findByRole('heading', { name: '订单详情' })).toBeVisible()
    expect(screen.queryByRole('button', { name: '补单' })).not.toBeInTheDocument()
  })

  it('checks the detail entry again and refreshes a now-ineligible order without opening the dialog', async () => {
    const cached = detail()
    let latest = cached
    let reads = 0
    server.use(http.get('*/api/v1/merchant/orders/:trade', () => { reads++; return success(latest) }))
    mount(`/orders/${cached.order.trade_no}`)
    const button = await screen.findByRole('button', { name: '补单' })
    latest = { ...cached, order: { ...cached.order, status: 'refunded' } }
    await userEvent.click(button)
    expect(await screen.findByText(/当前订单已不能补单/)).toBeVisible()
    expect(await screen.findByText('已退款', { exact: true })).toBeVisible()
    expect(reads).toBe(2)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('reports a failed open and prevents duplicate detail reads while opening', async () => {
    const value = detail()
    let reads = 0
    let release!: () => void
    const gate = new Promise<void>((resolve) => { release = resolve })
    server.use(
      http.get('*/api/v1/merchant/orders', () => success({ items: [value.order], total: 1, next_cursor: null })),
      http.get('*/api/v1/merchant/orders/:trade', async () => { reads++; await gate; return HttpResponse.error() }),
    )
    mount()
    const btn = within(await screen.findByRole('table')).getByRole('button', { name: '补单' })
    try {
      fireEvent.click(btn)
      fireEvent.click(btn)
      expect(btn).toBeDisabled()
      await waitFor(() => expect(reads).toBe(1))
    } finally { release() }
    expect(await screen.findByRole('alert')).toHaveTextContent(/网络/)
    expect(btn).toBeEnabled()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('requires confirmation of the latest amount and channel when they change before the first POST', async () => {
    const initial = detail('pending', null)
    let latest = initial
    const writes: unknown[] = []
    server.use(
      http.get('*/api/v1/merchant/orders/:trade', () => success(latest)),
      http.post('*/api/v1/merchant/orders/:trade/supplement', async ({ request }) => {
        writes.push(await request.json())
        return success(latest.order)
      }),
    )
    const { client } = mount(`/orders/${initial.order.trade_no}`)
    const keys = [
      ['merchant', identity.user.username, 'order', initial.order.trade_no],
      ['merchant', identity.user.username, 'orders', 'list', {}],
      ['merchant', identity.user.username, 'stats', { days: 7 }],
    ]
    for (const key of keys.slice(1)) client.setQueryData(key, {})
    const actor = userEvent.setup()
    await actor.click(await screen.findByRole('button', { name: '补单' }))
    const dialog = within(await screen.findByRole('dialog'))
    await chooseSelect(actor, dialog.getByLabelText('补单通道'), 'alipay')
    await actor.type(dialog.getByLabelText('原因', { exact: true }), 'Matched payment')
    latest = { ...initial, order: { ...initial.order, channel: 'wxpay', actual_cents: 12992 } }
    await actor.click(dialog.getByRole('button', { name: '确认补单' }))
    expect(await dialog.findByRole('alert')).toHaveTextContent(/核对最新金额和通道/)
    expect(writes).toEqual([])
    expect(dialog.queryByRole('combobox')).not.toBeInTheDocument()
    expect(dialog.getByText('微信支付')).toBeVisible()
    expect(dialog.getByText(/129\.92/)).toBeVisible()
    expect(keys.every((key) => client.getQueryState(key)?.isInvalidated)).toBe(true)
    await actor.click(dialog.getByRole('button', { name: '确认补单' }))
    await screen.findByText('补单成功，订单已更新。')
    expect(writes).toEqual([{ channel: 'wxpay', reason: 'Matched payment', expected_amount_cents: 12992 }])
  })

  it.each(['CHANNEL_LOCKED', 'ORDER_NOT_PAYABLE'] as const)('refreshes the order after a POST %s race', async (code) => {
    const initial = detail('pending', null)
    let latest = initial
    let writes = 0
    server.use(
      http.get('*/api/v1/merchant/orders/:trade', () => success(latest)),
      http.post('*/api/v1/merchant/orders/:trade/supplement', () => {
        writes++
        latest = { ...initial, order: { ...initial.order, channel: 'wxpay', status: code === 'CHANNEL_LOCKED' ? 'pending' : 'paid' } }
        return failure(409, code)
      }),
    )
    mount(`/orders/${initial.order.trade_no}`)
    const actor = userEvent.setup()
    await actor.click(await screen.findByRole('button', { name: '补单' }))
    const dialog = within(await screen.findByRole('dialog'))
    await chooseSelect(actor, dialog.getByLabelText('补单通道'), 'alipay')
    await actor.type(dialog.getByLabelText('原因', { exact: true }), 'Matched payment')
    await actor.click(dialog.getByRole('button', { name: '确认补单' }))
    expect(await dialog.findByRole('alert')).toHaveTextContent(code === 'CHANNEL_LOCKED' ? /核对最新金额和通道/ : /当前订单已不能补单/)
    expect(dialog.getByText('微信支付')).toBeVisible()
    expect(writes).toBe(1)
    if (code === 'ORDER_NOT_PAYABLE') expect(dialog.getByRole('button', { name: '确认补单' })).toBeDisabled()
  })

  it('refreshes the displayed amount and requires a second confirmation after an amount conflict', async () => {
    const initial = detail('pending', null)
    const locked = { ...initial, order: { ...initial.order, channel: 'alipay', actual_cents: 12346 } }
    let latest = initial
    const writes: unknown[] = []
    server.use(
      http.get('*/api/v1/merchant/orders/:trade', () => success(latest)),
      http.post('*/api/v1/merchant/orders/:trade/supplement', async ({ request }) => {
        writes.push(await request.json())
        latest = locked
        return writes.length === 1 ? failure(409, 'ORDER_AMOUNT_CHANGED') : success(locked.order)
      }),
    )
    const { client } = mount(`/orders/${initial.order.trade_no}`)
    const detailKey = ['merchant', identity.user.username, 'order', initial.order.trade_no]
    const otherDetailKey = ['merchant', identity.user.username, 'order', 'another-order']
    client.setQueryData(otherDetailKey, initial)
    const actor = userEvent.setup()
    await actor.click(await screen.findByRole('button', { name: '补单' }))
    const dialog = within(await screen.findByRole('dialog'))
    await chooseSelect(actor, dialog.getByLabelText('补单通道'), 'alipay')
    await actor.type(dialog.getByLabelText('原因', { exact: true }), 'Matched payment')
    await actor.click(dialog.getByRole('button', { name: '确认补单' }))
    expect(await dialog.findByRole('alert')).toHaveTextContent(/订单金额已变化/)
    expect(dialog.getByText(/123\.46/)).toBeVisible()
    expect(writes).toEqual([{ channel: 'alipay', reason: 'Matched payment', expected_amount_cents: 12345 }])
    expect(client.getQueryState(detailKey)?.isInvalidated).toBe(true)
    expect(client.getQueryState(otherDetailKey)?.isInvalidated).toBe(false)
    await actor.click(dialog.getByRole('button', { name: '确认补单' }))
    await screen.findByText('补单成功，订单已更新。')
    expect(writes[1]).toEqual({ channel: 'alipay', reason: 'Matched payment', expected_amount_cents: 12346 })
  })

  it.each([false, true])('invalidates detail even when the conflict GET fails (active refetch fails: %s)', async (refetchFails) => {
    const initial = detail()
    const paid = { ...initial, order: { ...initial.order, status: 'paid' } }
    let conflicted = false
    let conflictReads = 0
    server.use(
      http.get('*/api/v1/merchant/orders/:trade', () => {
        if (!conflicted) return success(initial)
        conflictReads++
        return conflictReads === 1 || refetchFails ? HttpResponse.error() : success(paid)
      }),
      http.post('*/api/v1/merchant/orders/:trade/supplement', () => {
        conflicted = true
        return failure(409, 'ORDER_NOT_PAYABLE')
      }),
    )
    const { client } = mount(`/orders/${initial.order.trade_no}`)
    const detailKey = ['merchant', identity.user.username, 'order', initial.order.trade_no]
    const listKey = ['merchant', identity.user.username, 'orders', 'list', {}]
    const statsKey = ['merchant', identity.user.username, 'stats', { days: 7 }]
    client.setQueryData(listKey, {})
    client.setQueryData(statsKey, {})
    const actor = userEvent.setup()
    await actor.click(await screen.findByRole('button', { name: '补单' }))
    const dialog = within(await screen.findByRole('dialog'))
    await actor.type(dialog.getByLabelText('原因', { exact: true }), 'Matched payment')
    await actor.click(dialog.getByRole('button', { name: '确认补单' }))
    expect(await dialog.findByRole('alert')).toHaveTextContent(/暂时无法获取最新信息/)
    await waitFor(() => expect(dialog.getByRole('button', { name: '取消' })).toBeEnabled())
    expect(dialog.getByRole('button', { name: '确认补单' })).toBeDisabled()
    await actor.click(dialog.getByRole('button', { name: '取消' }))
    expect(conflictReads).toBe(2)
    expect(screen.queryByRole('button', { name: '补单' })).not.toBeInTheDocument()
    expect(client.getQueryState(listKey)?.isInvalidated).toBe(true)
    expect(client.getQueryState(statsKey)?.isInvalidated).toBe(true)
    if (refetchFails) {
      expect(client.getQueryState(detailKey)?.isInvalidated).toBe(true)
      expect(screen.getByRole('alert')).toHaveTextContent(/网络/)
    } else {
      expect(screen.getByText('已支付', { exact: true })).toBeVisible()
    }
  })

  it('retries an unknown POST outcome with the same payload and key even after the order is paid', async () => {
    const initial = detail('pending', null)
    const paid = { ...initial, order: { ...initial.order, status: 'paid', channel: 'alipay' } }
    const writes: { key: string | null; body: unknown }[] = []
    let reads = 0
    let readsAtRetry = -1
    server.use(
      http.get('*/api/v1/merchant/orders/:trade', () => { reads++; return success(writes.length ? paid : initial) }),
      http.post('*/api/v1/merchant/orders/:trade/supplement', async ({ request }) => {
        writes.push({ key: request.headers.get('Idempotency-Key'), body: await request.json() })
        if (writes.length === 1) return HttpResponse.error()
        readsAtRetry = reads
        return success(paid.order)
      }),
    )
    mount(`/orders/${initial.order.trade_no}`)
    const actor = userEvent.setup()
    await actor.click(await screen.findByRole('button', { name: '补单' }))
    const dialog = within(await screen.findByRole('dialog'))
    const reason = dialog.getByLabelText('原因', { exact: true })
    const channel = dialog.getByLabelText('补单通道')
    await chooseSelect(actor, channel, 'alipay')
    await actor.type(reason, 'Matched payment')
    await actor.click(dialog.getByRole('button', { name: '确认补单' }))
    expect(await dialog.findByRole('alert')).toHaveTextContent(/网络/)
    expect(reason).toHaveAttribute('readonly')
    expect(channel).toBeDisabled()
    expect(reads).toBe(3)
    await actor.click(dialog.getByRole('button', { name: '确认补单' }))
    await screen.findByText('补单成功，订单已更新。')
    expect(readsAtRetry).toBe(3)
    expect(writes).toHaveLength(2)
    expect(writes[0]?.key).toMatch(/^[0-9a-f-]{36}$/)
    expect(writes[0]?.body).toEqual({ reason: 'Matched payment', channel: 'alipay', expected_amount_cents: 12345 })
    expect(writes[1]).toEqual(writes[0])
  })

  it('only allows supplement channels present in the order snapshot', async () => {
    const value = { ...detail('pending', null), allowed_channels_snapshot: ['alipay'] }
    server.use(http.get('*/api/v1/merchant/orders/:trade', () => success(value)))
    mount(`/orders/${value.order.trade_no}`)
    const actor = userEvent.setup()
    await actor.click(await screen.findByRole('button', { name: '补单' }))
    screen.getByLabelText('补单通道').focus()
    await actor.keyboard('{Enter}')
    const channel = within(await screen.findByRole('listbox'))
    expect(channel.getByRole('option', { name: '支付宝' })).not.toHaveAttribute('aria-disabled', 'true')
    expect(channel.getByRole('option', { name: '微信支付' })).toHaveAttribute('aria-disabled', 'true')
  })

  it('marks test orders in the list and detail only', async () => {
    const test = { ...order('pending', null), name: '测试支付', is_test: true }, normal = { ...order('paid', 'alipay'), name: '会员' }
    server.use(
      http.get('*/api/v1/merchant/orders', () => success({ items: [test, normal], total: 2, next_cursor: null })),
      http.get('*/api/v1/merchant/orders/:trade', () => success({ ...detail(), order: test, notify_url: null })),
    )
    const { router } = mount()
    const tests = await screen.findAllByText('测试', { exact: true })
    for (const badge of tests) expect(badge.closest('tr, li, article')?.textContent).toContain('测试支付')
    expect(screen.getAllByText('会员').every((cell) => !within(cell.closest('tr, li, article') ?? cell).queryByText('测试', { exact: true }))).toBe(true)
    await router.navigate({ to: '/orders/$tradeNo', params: { tradeNo: test.trade_no } })
    await screen.findByRole('heading', { name: '订单详情' })
    expect(screen.getByText('测试', { exact: true })).toBeVisible()
  })

  it('keeps replay visible but disabled at the generation limit', async () => {
    const paid = detail('paid')
    paid.order.callback_status = 'failed'
    const value = { ...paid, callback_replays_used: paid.callback_replay_limit }
    server.use(http.get('*/api/v1/merchant/orders/:trade', () => success(value)))
    mount(`/orders/${value.order.trade_no}`)
    expect(await screen.findByRole('button', { name: '重发回调' })).toBeDisabled()
    expect(screen.getByText('已达重发上限（2147483646/2147483646）')).toBeVisible()
  })

  it('shows an already handled refund conflict and refreshes detail', async () => {
    const value = detail('paid')
    const refund: NonNullable<Schema['OrderDetail']['refund']> = { id: crypto.randomUUID(), trade_no: value.order.trade_no,
      out_refund_no: null, amount_cents: 12345, status: 'pending', reason: null, requested_at: new Date().toISOString(), handled_at: null, handled_by: null }
    let rejected = false
    server.use(
      http.get('*/api/v1/merchant/orders/:trade', () => success({ ...value, refund: { ...refund, status: rejected ? 'rejected' : 'pending' } })),
      http.post('*/api/v1/merchant/refunds/:id/resolve', () => { rejected = true; return failure(409, 'REFUND_ALREADY_HANDLED') }),
    )
    mount(`/orders/${value.order.trade_no}`)
    const actor = userEvent.setup()
    await actor.click(await screen.findByRole('button', { name: '处理退款' }))
    await actor.type(screen.getByLabelText('原因', { exact: true }), 'Verified refund')
    await actor.click(screen.getByRole('checkbox'))
    await actor.click(screen.getByRole('button', { name: '确认处理退款' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('该退款申请已被处理')
    expect(screen.getByRole('button', { name: '确认处理退款' })).toBeDisabled()
    await actor.click(screen.getByRole('button', { name: '取消' }))
    expect(await screen.findByText('已拒绝')).toBeVisible()
    expect(screen.queryByRole('button', { name: '处理退款' })).not.toBeInTheDocument()
  })

  it.each(['supplement', 'refund', 'resolve'] as const)('invalidates every cached statistics range for this merchant after %s', async (action) => {
    const value = detail(action === 'supplement' ? 'pending' : 'paid')
    value.refund = action === 'resolve' ? { id: crypto.randomUUID(), trade_no: value.order.trade_no, out_refund_no: null, amount_cents: 12345, status: 'pending', reason: null, requested_at: new Date().toISOString(), handled_at: null, handled_by: null } : null
    const endpoint = action === 'resolve' ? `/api/v1/merchant/refunds/${value.refund!.id}/resolve` : `/api/v1/merchant/orders/${value.order.trade_no}/${action}`
    server.use(http.get('*/api/v1/merchant/orders/:trade', () => success(value)), http.post(`*${endpoint}`, () => success(action === 'resolve' ? value.refund : value.order)))
    const { client } = mount(`/orders/${value.order.trade_no}`)
    const ranges = ([1, 7, 30] as const).flatMap((days) => {
      const range = statsRange(new Date(), days, 'Asia/Shanghai')
      return [range, previousRange(range)]
    })
    const keys = ranges.map((range) => ['merchant', identity.user.username, 'stats', range])
    for (const key of keys) client.setQueryData(key, { total_amount_cents: 12345 })
    const otherMerchant = ['merchant', 'another_shop', 'stats', ranges[0]]
    client.setQueryData(otherMerchant, { total_amount_cents: 12345 })
    expect(keys.every((key) => client.getQueryState(key)?.isInvalidated === false)).toBe(true)
    const actor = userEvent.setup()
    await actor.click(await screen.findByRole('button', { name: action === 'supplement' ? '补单' : action === 'refund' ? '标记退款' : '处理退款' }))
    await actor.type(screen.getByLabelText('原因', { exact: true }), 'Verified offline transaction')
    if (action !== 'supplement') await actor.click(screen.getByRole('checkbox'))
    await actor.click(screen.getByRole('button', { name: action === 'supplement' ? '确认补单' : action === 'refund' ? '确认退款' : '确认处理退款' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(keys.every((key) => client.getQueryState(key)?.isInvalidated === true)).toBe(true)
    expect(client.getQueryState(otherMerchant)?.isInvalidated).toBe(false)
  })
  it.each(['pending', 'expired', 'paid', 'refunded'])('limits actions for status %s', (status) => {
    const value = detail(status); value.order.callback_status = 'failed'
    expect(availableOrderActions(value)).toEqual(status === 'paid' ? ['refund', 'replay'] : status === 'refunded' ? [] : ['supplement'])
  })
  it.each(['pending', 'running', 'none', 'internal_fulfillment'])('does not replay callback status %s', (state) => {
    const value = detail('paid'); value.order.callback_status = state
    expect(availableOrderActions(value)).not.toContain('replay')
  })
  it('checks subscription, missing notify URL and pending refund restrictions', () => {
    const value = detail('paid'); value.order.callback_status = 'success'; value.order.purpose = 'subscription'
    expect(availableOrderActions(value)).toEqual(['refund'])
    value.order.purpose = 'external'; value.notify_url = null
    expect(availableOrderActions(value)).toEqual(['refund'])
    value.refund = { id: crypto.randomUUID(), trade_no: value.order.trade_no, out_refund_no: null, amount_cents: 12345, status: 'pending', reason: null, requested_at: new Date().toISOString(), handled_at: null, handled_by: null }
    expect(availableOrderActions(value)).toEqual(['refund', 'resolve', 'reject'])
  })
  it('keeps one idempotency key and payload for retries, discards it on close, and invalidates order caches on success', async () => {
    const value = detail()
    const seen: { key: string | null; csrf: string | null; body: unknown }[] = []
    server.use(http.get('*/api/v1/merchant/orders/:trade', () => success(value)), http.post('*/api/v1/merchant/orders/:trade/supplement', async ({ request }) => {
      seen.push({ key: request.headers.get('Idempotency-Key'), csrf: request.headers.get('X-CSRF-Token'), body: await request.json() })
      if (seen.length < 3) return failure(500, 'INTERNAL_ERROR')
      value.order.status = 'paid'; value.order.is_manual = true
      return success(value.order)
    }))
    const { client } = mount(`/orders/${value.order.trade_no}`)
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    const actor = userEvent.setup()
    await actor.click(await screen.findByRole('button', { name: '补单' }))
    await actor.type(screen.getByLabelText('原因', { exact: true }), 'Verified payment')
    await actor.click(screen.getByRole('button', { name: '确认补单' }))
    await screen.findByRole('alert')
    expect(screen.getByLabelText('原因', { exact: true })).toHaveAttribute('readonly')
    await actor.click(screen.getByRole('button', { name: '确认补单' }))
    await waitFor(() => expect(seen).toHaveLength(2))
    expect(seen[0]!.key).toMatch(/^[0-9a-f-]{36}$/)
    expect(seen[1]).toEqual(seen[0])
    expect(seen[0]!.csrf).toBe(identity.csrf_token)
    await actor.click(screen.getByRole('button', { name: '取消' }))
    await actor.click(screen.getByRole('button', { name: '补单' }))
    await actor.type(screen.getByLabelText('原因', { exact: true }), 'Verified payment')
    await actor.click(screen.getByRole('button', { name: '确认补单' }))
    await screen.findByText('补单成功，订单已更新。')
    expect(seen[2]!.key).not.toBe(seen[0]!.key)
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['merchant', identity.user.username, 'order', value.order.trade_no], exact: true, refetchType: 'active' })
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['merchant', identity.user.username, 'orders'] })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
  it('requires refund confirmation and blocks duplicate submission and dismissal while pending', async () => {
    const value = detail('paid')
    let submissions = 0, release: () => void = () => {}
    const gate = new Promise<void>((resolve) => { release = resolve })
    server.use(http.get('*/api/v1/merchant/orders/:trade', () => success(value)), http.post('*/api/v1/merchant/orders/:trade/refund', async ({ request }) => {
      submissions++
      expect(await request.json()).toEqual({ confirm: true, reason: 'Offline refund verified' })
      await gate; return failure(409, 'ALREADY_REFUNDED')
    }))
    mount(`/orders/${value.order.trade_no}`)
    const actor = userEvent.setup()
    await actor.click(await screen.findByRole('button', { name: '标记退款' }))
    await actor.type(screen.getByLabelText('原因', { exact: true }), 'Offline refund verified')
    expect(screen.getByRole('button', { name: '确认退款' })).toBeDisabled()
    await actor.click(screen.getByRole('checkbox'))
    await actor.dblClick(screen.getByRole('button', { name: '确认退款' }))
    expect(screen.getByRole('button', { name: '提交中…' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '取消' })).toBeDisabled()
    await actor.keyboard('{Escape}')
    expect(screen.getByRole('dialog')).toBeVisible()
    expect(submissions).toBe(1)
    release()
    expect(await screen.findByRole('alert')).toHaveTextContent('订单状态已变化')
  })
  it('validates byte limits for refund reasons while allowing supplement rune limits', () => {
    expect(validReason(' ', 'refund')).toBe(false)
    expect(validReason('原'.repeat(167), 'refund')).toBe(false)
    expect(validReason('原'.repeat(500), 'supplement')).toBe(true)
  })
})

describe('order error mapping', () => {
  it.each([
    [400, 'VALIDATION_ERROR', 'merchant.orders.errors.validation'], [409, 'ORDER_NOT_PAID', 'merchant.orders.errors.conflict'], [409, 'ORDER_NOT_PAYABLE', 'merchant.orders.errors.conflict'], [409, 'ALREADY_REFUNDED', 'merchant.orders.errors.conflict'], [409, 'CALLBACK_PENDING', 'merchant.orders.errors.callbackPending'], [409, 'CALLBACK_UNAVAILABLE', 'merchant.orders.errors.callbackUnavailable'], [409, 'IDEMPOTENCY_CONFLICT', 'merchant.orders.errors.idempotency'], [403, 'CSRF_FAILED', 'merchant.orders.errors.csrf'], [403, 'CHANNEL_NOT_ALLOWED', 'merchant.orders.errors.channel'], [409, 'CHANNEL_LOCKED', 'merchant.orders.errors.channelLocked'], [404, 'NOT_FOUND', 'merchant.orders.notFound'], [500, 'INTERNAL_ERROR', 'merchant.orders.errors.unavailable'],
  ] as const)('maps %s/%s to a useful message', (status, code, expected) => {
    expect(orderErrorKey(new APIError(status, { code, message: code, fields: {}, details: {} }))).toBe(expected)
  })
})
