import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { useState, type ReactNode } from 'react'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { api } from '@/api/client'
import type { components } from '@/api/schema'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import { PurchaseDialog, SubscriptionPage } from '@/features/merchant/SubscriptionPage'
import { PopupAnnouncements, UnreadBadge } from '@/features/merchant/NotificationsPage'
import { invalidateNotifications, notificationTarget, useNotificationSummary } from '@/features/merchant/notification-data'
import { invalidateSubscription } from '@/features/merchant/subscription-data'
import { auditSearch } from '@/features/merchant/audit-data'
import { createI18n } from '@/lib/i18n'
import { emptyNotifications } from './notification-fixture'

type Schema = components['schemas']
const plan: Schema['SubscriptionPlanDTO'] = { id: '2', revision: 3, name: { zh_CN: '基础套餐', en: 'Basic' }, price_cents: 1200, duration_days: 30, channels: ['alipay'], purchase_limit: 2, purchased_count: 0, remaining_count: 2, deleted: false }
const current: Schema['SubscriptionSubscriptionDTO'] = { plan: null, active: false, channels: [], expires_at: null, source: 'none', revision: 0 }
const user: Schema['AuthUserDTO'] = { id: '1', username: 'subscriber', email: 'subscriber@example.test', pid: '9000000000000001', role: 'merchant', banned: false, banned_at: null, ban_reason: null, totp_enabled: false, locale: 'zh-CN', theme: 'light', created_at: new Date().toISOString() }
const session: Schema['SessionIdentity'] = { user, csrf_token: crypto.randomUUID(), expires_at: new Date(Date.now() + 3600000).toISOString() }
const purchase: Schema['SubscriptionPurchaseDTO'] = { id: 'purchase-id', plan_snapshot: plan, purchase_kind: 'new', state: 'pending', price_cents: 1200, trade_no: 'trade-123', cashier_url: '/cashier/exact-url?opaque=1', created_at: new Date().toISOString(), paid_at: null, expires_at: new Date(Date.now() + 300000).toISOString(), fulfillment_at: null }
const overview: Schema['SubscriptionOverview'] = { current, plans: [plan], pending_purchase: null, billing_ready: true }
const announcement: Schema['PortalAnnouncementDTO'] = { id: 'announcement', revision: 1, title: { zh_CN: '系统公告', en: 'Announcement' }, body: { zh_CN: '<script>alert(1)</script>\n<b>原始文本</b>', en: 'Notice' }, popup: true, published_at: new Date().toISOString(), read: false }
const success = <T,>(data: T) => HttpResponse.json({ data, error: null, request_id: crypto.randomUUID() })
const failure = (code: Schema['ErrorCode'], status = 409) => HttpResponse.json({ error: { code, message: code, fields: {}, details: {} }, request_id: crypto.randomUUID() }, { status })
const server = setupServer(emptyNotifications, http.get('*/api/v1/merchant/me', () => success(session)), http.get('*/api/v1/merchant/subscription', () => success(overview)))
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => { server.resetHandlers(); vi.useRealTimers() })
afterAll(() => server.close())
function wrap(children: ReactNode) {
  const client = createQueryClient()
  return { client, ...render(<AppProviders queryClient={client} i18n={createI18n()}>{children}</AppProviders>) }
}
function mount(path: string) {
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) })
  return { router, ...wrap(<RouterProvider router={router} />) }
}

describe('subscription confirmations', () => {
  it.each(['new', 'renew', 'switch'] as const)('shows %s semantics and sends the captured revisions with CSRF and one idempotency key', async (kind) => {
    await api.me()
    const value = kind === 'new' ? current : { ...current, revision: 7, active: true, plan: kind === 'renew' ? plan : { ...plan, id: 'other' }, expires_at: new Date(Date.now() + 86400000).toISOString() }
    const requests: Request[] = [], bodies: unknown[] = []
    server.use(http.post('*/api/v1/merchant/subscription/purchases', async ({ request }) => { requests.push(request); bodies.push(await request.json()); return success(purchase) }))
    const changed = vi.fn().mockResolvedValue(undefined)
    wrap(<PurchaseDialog plan={plan} current={value} onClose={() => {}} onChanged={changed} />)
    expect(screen.getByRole('dialog')).toHaveAccessibleName(kind === 'new' ? '确认购买套餐' : kind === 'renew' ? '确认续费套餐' : '确认更换套餐')
    const submit = screen.getByRole('button', { name: '确认并创建订单' })
    if (kind === 'switch') {
      expect(screen.getByText('当前套餐剩余时间将作废')).toBeVisible(); expect(submit).toBeDisabled()
      fireEvent.click(submit); expect(requests).toHaveLength(0)
      await userEvent.click(screen.getByRole('checkbox'))
    }
    if (kind === 'renew') expect(screen.getByText('支付完成后，从当前套餐到期时间顺延。')).toBeVisible()
    await userEvent.click(submit)
    await waitFor(() => expect(changed).toHaveBeenCalledTimes(1))
    expect(bodies).toEqual([{ plan_id: plan.id, plan_revision: 3, expected_subscription_revision: value.revision, confirm_replace: kind === 'switch' }])
    expect(requests[0]!.headers.get('X-CSRF-Token')).toBe(session.csrf_token)
    expect(requests[0]!.headers.get('Idempotency-Key')).toMatch(/^[\da-f-]{36}$/)
  })
  it('reuses a UUID on retry but rotates it after closing and reopening', async () => {
    const keys: string[] = []
    server.use(http.post('*/api/v1/merchant/subscription/purchases', ({ request }) => { keys.push(request.headers.get('Idempotency-Key')!); return failure('INTERNAL_ERROR', 503) }))
    function Host() { const [open, setOpen] = useState(true); return <><button onClick={() => setOpen(true)}>重开</button>{open && <PurchaseDialog plan={plan} current={current} onClose={() => setOpen(false)} onChanged={async () => {}} />}</> }
    wrap(<Host />)
    await userEvent.click(screen.getByRole('button', { name: '确认并创建订单' })); await screen.findByRole('alert')
    await userEvent.click(screen.getByRole('button', { name: '确认并创建订单' })); await waitFor(() => expect(keys).toHaveLength(2))
    await screen.findByRole('alert'); await userEvent.click(screen.getByRole('button', { name: '关闭' })); await userEvent.click(screen.getByText('重开'))
    await userEvent.click(screen.getByRole('button', { name: '确认并创建订单' })); await screen.findByRole('alert')
    expect(keys[0]).toBe(keys[1]); expect(keys[2]).not.toBe(keys[0])
  })
  it.each([
    ['BILLING_CREDENTIAL_STALE', '平台收款配置已变更，请稍后重试。'],
    ['PLAN_UNAVAILABLE', '套餐已变更或下架，请重新选择。'],
    ['VERSION_CONFLICT', '当前订阅已变更，请重新确认。'],
    ['PURCHASE_PENDING', '已有待支付订单，请先支付或取消。'],
    ['PURCHASE_LIMIT_REACHED', '该套餐已达限购次数。'],
  ] as const)('explains %s and refreshes subscription data', async (code, text) => {
    let reads = 0
    server.use(http.get('*/api/v1/merchant/subscription', () => { reads++; return success(overview) }), http.post('*/api/v1/merchant/subscription/purchases', () => failure(code)))
    wrap(<SubscriptionPage name={user.username} />)
    await userEvent.click(await screen.findByRole('button', { name: '购买套餐' }))
    await userEvent.click(screen.getByRole('button', { name: '确认并创建订单' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(text)
    expect(reads).toBeGreaterThanOrEqual(2)
    expect(screen.getByRole('button', { name: '确认并创建订单' })).toBeDisabled()
  })
  it.each(['billing', 'pending', 'deleted', 'limit'] as const)('disables unavailable purchases: %s', async (reason) => {
    server.use(http.get('*/api/v1/merchant/subscription', () => success({ ...overview, billing_ready: reason !== 'billing', pending_purchase: reason === 'pending' ? { ...purchase, cashier_url: null } : null, plans: [{ ...plan, deleted: reason === 'deleted', remaining_count: reason === 'limit' ? 0 : 2 }] })), http.get('*/api/v1/merchant/subscription/purchases/:id', () => success(purchase)))
    wrap(<SubscriptionPage name={user.username} />)
    expect(await screen.findByRole('button', { name: '购买套餐' })).toBeDisabled()
    if (reason === 'billing') expect(screen.getByRole('status')).toHaveTextContent('平台暂未开放在线购买')
    if (reason === 'pending') expect(await screen.findByRole('link', { name: '去支付' })).toHaveAttribute('href', purchase.cashier_url)
  })
  it('confirms cancellation, refreshes pending state and never constructs cashier_url', async () => {
    let pending = true, canceled = 0
    server.use(http.get('*/api/v1/merchant/subscription', () => success({ ...overview, pending_purchase: pending ? { ...purchase, cashier_url: null } : null })), http.get('*/api/v1/merchant/subscription/purchases/:id', () => success(purchase)), http.post('*/api/v1/merchant/subscription/purchases/:id/cancel', () => { canceled++; pending = false; return success({ ...purchase, state: 'cancelled' }) }))
    wrap(<SubscriptionPage name={user.username} />)
    expect(await screen.findByRole('link', { name: '去支付' })).toHaveAttribute('href', purchase.cashier_url)
    await userEvent.click(screen.getByRole('button', { name: '取消订单' })); expect(canceled).toBe(0)
    await userEvent.click(screen.getByRole('button', { name: '确认取消订单' }))
    await waitFor(() => expect(screen.queryByRole('region', { name: '待支付订单' })).not.toBeInTheDocument())
    expect(canceled).toBe(1); expect(screen.getByRole('button', { name: '购买套餐' })).toBeEnabled()
  })
  it('refreshes a conflicting cancellation while keeping its error visible', async () => {
    let reads = 0
    server.use(http.get('*/api/v1/merchant/subscription', () => { reads++; return success({ ...overview, pending_purchase: purchase }) }), http.get('*/api/v1/merchant/subscription/purchases/:id', () => success(purchase)), http.post('*/api/v1/merchant/subscription/purchases/:id/cancel', () => failure('VERSION_CONFLICT')))
    wrap(<SubscriptionPage name={user.username} />)
    await userEvent.click(await screen.findByRole('button', { name: '取消订单' }))
    await userEvent.click(screen.getByRole('button', { name: '确认取消订单' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('当前订阅已变更，请重新确认。')
    expect(reads).toBeGreaterThanOrEqual(2)
  })
  it.each([[422, 'BILLING_NOT_READY', '平台暂未开放在线购买'], [503, 'INTERNAL_ERROR', '服务暂时不可用，请稍后重试。']] as const)('explains HTTP %s without losing the confirmation', async (status, code, message) => {
    server.use(http.post('*/api/v1/merchant/subscription/purchases', () => failure(code, status)))
    wrap(<PurchaseDialog plan={plan} current={current} onClose={() => {}} onChanged={async () => {}} />)
    await userEvent.click(screen.getByRole('button', { name: '确认并创建订单' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(message)
    expect(screen.getByRole('button', { name: '确认并创建订单' })).toBeEnabled()
  })
  it('invalidates subscription, audit logs, orders and stats under the merchant prefix', async () => {
    const client = createQueryClient()
    for (const name of ['subscription', 'audit-logs', 'stats', 'orders', 'collection']) client.setQueryData(['merchant', user.username, name, 'nested'], {})
    client.setQueryData(['merchant', 'other', 'stats'], {})
    await invalidateSubscription(client, user.username)
    for (const name of ['subscription', 'audit-logs', 'stats', 'orders', 'collection']) expect(client.getQueryState(['merchant', user.username, name, 'nested'])?.isInvalidated).toBe(true)
    expect(client.getQueryState(['merchant', 'other', 'stats'])?.isInvalidated).toBe(false)
  })
})

describe('notifications and announcements', () => {
  it('maps only known actions and encodes the complete entity id', () => {
    expect(notificationTarget(null)).toBeNull()
    expect(notificationTarget({ page: 'https://evil.example', entity_id: 'x' })).toBeNull()
    expect(notificationTarget({ page: '/dashboard/collection', entity_id: '' })).toBe('/devices')
    expect(notificationTarget({ page: '/dashboard/orders', entity_id: 'a/b?c#d' })).toBe('/orders/a%2Fb%3Fc%23d')
    expect(notificationTarget({ page: '/dashboard/orders', entity_id: '' })).toBeNull()
  })
  it('caps the badge visually and preserves the full accessible count', () => {
    const { rerender } = wrap(<UnreadBadge count={123} />)
    expect(screen.getByLabelText('123 条未读通知')).toHaveTextContent('99+')
    rerender(<AppProviders queryClient={createQueryClient()} i18n={createI18n()}><UnreadBadge count={0} /></AppProviders>)
    expect(screen.queryByText('99+')).not.toBeInTheDocument()
  })
  it('renders HTML and scripts as text and skips a deleted announcement on 404', async () => {
    server.use(http.post('*/api/v1/merchant/announcements/:id/read', () => failure('NOT_FOUND', 404)))
    wrap(<PopupAnnouncements name={user.username} items={[announcement]} />)
    const dialog = screen.getByRole('dialog')
    expect(dialog).toHaveTextContent('<script>alert(1)</script>'); expect(dialog.querySelector('script,b')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: '知道了' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })
  it('refetches on announcement version conflict and shows the new revision once', async () => {
    let revision = 1, reads = 0
    server.use(http.get('*/api/v1/merchant/notifications', () => { reads++; return success({ items: [], total: 0, unread_count: 0, next_cursor: null, popup_announcements: [{ ...announcement, revision, title: { zh_CN: `公告版本 ${revision}`, en: 'Updated' } }] }) }), http.post('*/api/v1/merchant/announcements/:id/read', async ({ request }) => { expect(await request.json()).toEqual({ revision: 1 }); revision = 2; return failure('VERSION_CONFLICT') }))
    function Host() { const query = useNotificationSummary(user.username); return <PopupAnnouncements name={user.username} items={query.data?.popup_announcements ?? []} /> }
    wrap(<Host />)
    await screen.findByRole('dialog', { name: '公告版本 1' }); await userEvent.click(screen.getByRole('button', { name: '知道了' }))
    await screen.findByRole('dialog', { name: '公告版本 2' }); expect(reads).toBe(2)
    await userEvent.click(screen.getByRole('button', { name: '关闭' })); expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
  it('invalidates all notification pages and the summary for this merchant only', async () => {
    const client = createQueryClient()
    for (const part of ['summary', 'list']) client.setQueryData(['merchant', user.username, 'notifications', part], {})
    client.setQueryData(['merchant', 'other', 'notifications', 'summary'], {})
    await invalidateNotifications(client, user.username)
    for (const part of ['summary', 'list']) expect(client.getQueryState(['merchant', user.username, 'notifications', part])?.isInvalidated).toBe(true)
    expect(client.getQueryState(['merchant', 'other', 'notifications', 'summary'])?.isInvalidated).toBe(false)
  })
  it('marks a notification before navigating and refreshes the layout badge', async () => {
    let unread = 1, marked = false
    server.use(http.get('*/api/v1/merchant/notifications', () => success({ items: [{ id: 'n1', kind: 'device_offline', source_id: 'd1', title: { zh_CN: '设备消息', en: 'Device alert' }, summary: { zh_CN: '<b>纯文本</b>', en: 'Text' }, created_at: new Date().toISOString(), read_at: unread ? null : new Date().toISOString(), action: { page: '/dashboard/collection', entity_id: '' } }], unread_count: unread, total: 1, next_cursor: null, popup_announcements: [] })), http.post('*/api/v1/merchant/notifications/read', async ({ request }) => { expect(await request.json()).toEqual({ ids: ['n1'] }); marked = true; unread = 0; return success({ unread_count: 0 }) }), http.get('*/api/v1/merchant/device', () => { expect(marked).toBe(true); return success({ bound: false, heartbeat_interval_seconds: 30, offline_threshold_seconds: 90 }) }), http.get('*/api/v1/merchant/receipts', () => success({ items: [], next_cursor: null, total: 0 })))
    const { router } = mount('/notifications')
    await screen.findByText('<b>纯文本</b>')
    expect(screen.getAllByLabelText('1 条未读通知').length).toBeGreaterThan(0)
    await userEvent.click(screen.getByRole('link', { name: '查看详情' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/devices'))
    await waitFor(() => expect(screen.queryByLabelText('1 条未读通知')).not.toBeInTheDocument())
  })
  it('stops its summary polling when unmounted', async () => {
    const load = vi.spyOn(api, 'notifications').mockResolvedValue({ items: [], total: 0, next_cursor: null, unread_count: 0, popup_announcements: [] })
    function Host() { useNotificationSummary(user.username); return null }
    const { unmount } = wrap(<Host />)
    await waitFor(() => expect(load).toHaveBeenCalledTimes(1))
    vi.useFakeTimers(); unmount(); await act(() => vi.advanceTimersByTimeAsync(120000))
    expect(load).toHaveBeenCalledTimes(1)
  })
  it('filters unread notifications, paginates and sends the exclusive all-read body', async () => {
    const parameters: URLSearchParams[] = []
    let count = 2
    server.use(http.get('*/api/v1/merchant/notifications', ({ request }) => {
      const params = new URL(request.url).searchParams
      parameters.push(params)
      const id = params.has('cursor') ? 'n2' : 'n1'
      return success({ items: params.get('unread_only') === 'true' && count === 0 ? [] : [{ id, kind: 'announcement', source_id: id, title: { zh_CN: id, en: id }, summary: { zh_CN: '文本', en: 'Text' }, created_at: new Date().toISOString(), read_at: count ? null : new Date().toISOString(), action: { page: 'https://evil.example', entity_id: '' } }], total: count, unread_count: count, next_cursor: params.has('cursor') || count === 0 ? null : 'next-notice', popup_announcements: [] })
    }), http.post('*/api/v1/merchant/notifications/read', async ({ request }) => { expect(await request.json()).toEqual({ all: true }); count = 0; return success({ unread_count: 0 }) }))
    mount('/notifications')
    await screen.findByRole('heading', { name: 'n1' })
    expect(screen.queryByRole('link', { name: '查看详情' })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '未读' }))
    await waitFor(() => expect(parameters.some((value) => value.get('unread_only') === 'true')).toBe(true))
    await userEvent.click(screen.getByRole('button', { name: '加载更多' }))
    await screen.findByRole('heading', { name: 'n2' })
    expect(parameters.at(-1)?.get('cursor')).toBe('next-notice')
    await userEvent.click(screen.getByRole('button', { name: '全部标为已读' }))
    await screen.findByText('暂无通知')
    expect(screen.getByRole('button', { name: '全部标为已读' })).toBeDisabled()
  })
})

describe('audit logs', () => {
  it('synchronizes filters to the URL and sends cursor pagination without merchant parameters', async () => {
    const queries: URLSearchParams[] = []
    server.use(http.get('*/api/v1/merchant/audit-logs', ({ request }) => {
      const query = new URL(request.url).searchParams; queries.push(query)
      return success({ items: [{ id: query.has('cursor') ? '2' : '1', actor_role: 'merchant', actor_id: '1', merchant_id: '1', action: 'future.unknown', target_type: 'subscription', target_id: 'p1', result: 'success', ip: '127.0.0.1', created_at: new Date().toISOString(), request_id: query.has('cursor') ? 'request-two' : 'request-one', safe_metadata: { text: '<script>raw</script>' } }], total: 2, next_cursor: query.has('cursor') ? null : 'opaque-cursor' })
    }))
    const { router } = mount('/account/logs')
    await screen.findByText('request-one')
    await userEvent.selectOptions(screen.getByLabelText('操作类型'), 'auth.login')
    await userEvent.click(screen.getByRole('button', { name: '重置' }))
    expect(screen.getByLabelText('操作类型')).toHaveValue('')
    await userEvent.selectOptions(screen.getByLabelText('操作类型'), 'subscription.purchase')
    await userEvent.selectOptions(screen.getByLabelText('结果'), 'success')
    fireEvent.change(screen.getByLabelText('开始时间'), { target: { value: '2026-11-01T00:00:00' } })
    fireEvent.change(screen.getByLabelText('结束时间'), { target: { value: '2026-11-02T00:00:00' } })
    await userEvent.click(screen.getByRole('button', { name: '应用筛选' }))
    await waitFor(() => expect(router.state.location.search).toMatchObject({ action: 'subscription.purchase', result: 'success' }))
    await waitFor(() => expect(queries.at(-1)?.get('start')).toBe(new Date('2026-11-01T00:00:00').toISOString()))
    await userEvent.click(screen.getByRole('button', { name: '加载更多' }))
    await screen.findByText('request-two')
    expect(queries.at(-1)?.get('cursor')).toBe('opaque-cursor'); expect(queries.at(-1)?.has('merchant_id')).toBe(false)
    expect(screen.getAllByText('future.unknown')).toHaveLength(2)
    await userEvent.click(screen.getAllByRole('button', { name: '详情' })[0]!)
    expect(within(screen.getByRole('dialog')).getByText(/<script>raw<\/script>/)).toBeVisible()
    expect(screen.getByRole('dialog').querySelector('script')).toBeNull()
    expect(auditSearch({ result: 'success', action: ' auth.login ', merchant_id: 'other', cursor: 'bad' })).toEqual({ result: 'success', action: 'auth.login' })
  })
})
