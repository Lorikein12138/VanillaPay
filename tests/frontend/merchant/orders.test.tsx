import { emptyNotifications } from './notification-fixture'
import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
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
  return { order: order(status, channel), notify_url: 'https://callback.example.test/notify', return_url: 'https://shop.example.test/return', clientip: null, key_version: null, receipt_id: null, callback_attempts: [], refund: null }
}
const success = <T,>(data: T) => HttpResponse.json({ data, error: null, request_id: crypto.randomUUID() })
const failure = (status: number, code: Schema['ErrorCode']) => HttpResponse.json({ error: { code, fields: {}, details: {}, message: code }, request_id: crypto.randomUUID() }, { status })
const server = setupServer(emptyNotifications,
  http.get('*/api/v1/merchant/me', () => success(identity)),
  http.get('*/api/v1/merchant/orders', () => success({ items: [], total: 0, next_cursor: null })),
)
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => { server.resetHandlers(); vi.restoreAllMocks() })
afterAll(() => server.close())
function mount(path = '/orders') {
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) })
  const client = createQueryClient()
  render(<AppProviders i18n={createI18n()} queryClient={client}><RouterProvider router={router} /></AppProviders>)
  return { router, client }
}

describe('order filters and cursor pagination', () => {
  it('clears unsubmitted fields when reset keeps the same empty URL', async () => {
    const { router } = mount()
    const actor = userEvent.setup()
    await actor.type(await screen.findByLabelText('搜索订单'), 'unsubmitted')
    await actor.selectOptions(screen.getByLabelText('状态'), 'paid')
    await actor.click(screen.getByRole('button', { name: '重置' }))
    expect(screen.getByLabelText('搜索订单')).toHaveValue('')
    expect(screen.getByLabelText('状态')).toHaveValue('')
    expect(router.state.location.search).toEqual({})
  })
  it('restores shared URL filters and writes changed filters back as RFC3339', async () => {
    const requests: URL[] = []
    server.use(http.get('*/api/v1/merchant/orders', ({ request }) => { requests.push(new URL(request.url)); return success({ items: [], total: 0, next_cursor: null }) }))
    const { router } = mount('/orders?status=paid&channel=alipay&query=shared&start=2026-11-01T00%3A00%3A00Z&end=2026-11-04T00%3A00%3A00Z')
    expect(await screen.findByLabelText('状态')).toHaveValue('paid')
    expect(screen.getByLabelText('通道')).toHaveValue('alipay')
    expect(screen.getByLabelText('搜索订单')).toHaveValue('shared')
    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]!.searchParams.get('start')).toBe('2026-11-01T00:00:00Z')
    fireEvent.change(screen.getByLabelText('开始时间'), { target: { value: '2026-11-02T10:30' } })
    fireEvent.change(screen.getByLabelText('结束时间'), { target: { value: '2026-11-03T11:30' } })
    const actor = userEvent.setup()
    await actor.selectOptions(screen.getByLabelText('状态'), 'expired')
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
    await screen.findAllByText(first.trade_no)
    const actor = userEvent.setup()
    await actor.click(screen.getByRole('button', { name: '加载更多' }))
    await screen.findAllByText(second.trade_no)
    expect(within(screen.getByRole('table')).getAllByRole('row')).toHaveLength(3)
    expect(urls.at(-1)!.searchParams.get('cursor')).toBe('cursor-fixture')
    await actor.selectOptions(screen.getByLabelText('状态'), 'paid')
    await actor.click(screen.getByRole('button', { name: '应用筛选' }))
    await waitFor(() => expect(urls.at(-1)!.searchParams.get('status')).toBe('paid'))
    expect(urls.at(-1)!.searchParams.has('cursor')).toBe(false)
    await actor.click(screen.getByRole('button', { name: '重置' }))
    await waitFor(() => expect(urls.at(-1)!.searchParams.has('status')).toBe(false))
    expect(urls.at(-1)!.searchParams.has('cursor')).toBe(false)
    expect(within(screen.getByRole('table')).getAllByRole('row')).toHaveLength(2)
  })

  it('shows initial loading, an error with retry, and the empty result independently', async () => {
    let release: () => void = () => {}
    const gate = new Promise<void>((resolve) => { release = resolve })
    server.use(http.get('*/api/v1/merchant/orders', async () => { await gate; return failure(500, 'INTERNAL_ERROR') }))
    mount()
    await waitFor(() => expect(screen.getByRole('region', { name: '订单列表' })).toHaveAttribute('aria-busy', 'true'))
    expect(screen.queryByText('没有符合筛选条件的订单')).not.toBeInTheDocument()
    release()
    expect(await screen.findByRole('alert')).toHaveTextContent('服务暂时不可用')
    server.use(http.get('*/api/v1/merchant/orders', () => success({ items: [], total: 0, next_cursor: null })))
    await userEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText('没有符合筛选条件的订单')).toBeVisible()
  })
})

describe('order actions', () => {
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
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['merchant', identity.user.username, 'order', value.order.trade_no] })
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
