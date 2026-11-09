import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { focusManager } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { publicConfigHandler } from '../public-config-fixture'
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
import { AuditDetails } from '@/features/merchant/AuditLogsPage'
import { formatOrderTime } from '@/features/merchant/OrderParts'
import { SegmentedControl } from '@/components/ui/segmented-control'
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
const server = setupServer(
  emptyNotifications, publicConfigHandler,
  http.get('*/api/v1/merchant/me', () => success(session)),
  http.get('*/api/v1/merchant/subscription', () => success(overview)),
)
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
  it('keeps the price and revisions shown when a confirmation was opened during a background refresh', async () => {
    let latest = overview
    const buy = vi.spyOn(api, 'purchase').mockResolvedValue(purchase)
    server.use(http.get('*/api/v1/merchant/subscription', () => success(latest)))
    const { client } = wrap(<SubscriptionPage name={user.username} />)
    await userEvent.click(await screen.findByRole('button', { name: '购买套餐' }))
    latest = { ...overview, current: { ...current, revision: 8 }, plans: [{ ...plan, revision: 9, price_cents: 8800 }] }
    await act(() => client.refetchQueries({ queryKey: ['merchant', user.username, 'subscription'] }))
    expect(await screen.findByText('¥88.00')).toBeInTheDocument()
    expect(within(screen.getByRole('dialog')).getByText('¥12.00')).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: '确认并创建订单' }))
    expect(buy).toHaveBeenCalledExactlyOnceWith({
      plan_id: plan.id, plan_revision: plan.revision, expected_subscription_revision: current.revision, confirm_replace: false,
    }, expect.any(String))
  })

  it('polls every 15 seconds while pending and stops when the pending purchase disappears', async () => {
    vi.useFakeTimers()
    const load = vi.spyOn(api, 'subscription')
      .mockResolvedValueOnce({ ...overview, pending_purchase: purchase })
      .mockResolvedValueOnce({ ...overview, pending_purchase: purchase })
      .mockResolvedValue(overview)
    vi.spyOn(api, 'purchaseDetail').mockResolvedValue(purchase)
    wrap(<SubscriptionPage name={user.username} />)
    await act(() => vi.advanceTimersByTimeAsync(0))
    expect(screen.getByRole('region', { name: '待支付订单' })).toBeVisible()
    await act(() => vi.advanceTimersByTimeAsync(14999))
    expect(load).toHaveBeenCalledTimes(1)
    await act(() => vi.advanceTimersByTimeAsync(1))
    expect(load).toHaveBeenCalledTimes(2)
    await act(() => vi.advanceTimersByTimeAsync(15001))
    expect(load).toHaveBeenCalledTimes(3)
    expect(screen.queryByRole('region', { name: '待支付订单' })).not.toBeInTheDocument()
    await act(() => vi.advanceTimersByTimeAsync(45000))
    expect(load).toHaveBeenCalledTimes(3)
  })

  it.each(['new', 'renew', 'switch'] as const)('shows %s semantics and sends the captured revisions with CSRF and one idempotency key', async (kind) => {
    await api.me()
    const value = kind === 'new' ? current : { ...current, revision: 7, active: true, plan: kind === 'renew' ? plan : { ...plan, id: 'other', name: { zh_CN: '原有套餐', en: 'Previous plan' } }, expires_at: '2026-12-01T12:00:00Z' }
    const requests: Request[] = [], bodies: unknown[] = []
    server.use(http.post('*/api/v1/merchant/subscription/purchases', async ({ request }) => { requests.push(request); bodies.push(await request.json()); return success(purchase) }))
    const changed = vi.fn().mockResolvedValue(undefined)
    wrap(<PurchaseDialog plan={plan} current={value} onClose={() => {}} onChanged={changed} />)
    expect(screen.getByRole('dialog')).toHaveAccessibleName(kind === 'new' ? '确认购买套餐' : kind === 'renew' ? '确认续费套餐' : '确认更换套餐')
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByText('套餐', { selector: 'dt' })).toBeVisible()
    expect(within(dialog).getByText('金额', { selector: 'dt' })).toBeVisible()
    expect(within(dialog).getByText('时长', { selector: 'dt' })).toBeVisible()
    expect(within(dialog).getByText('30 天')).toBeVisible()
    expect(within(dialog).queryByText('到期时间', { selector: 'dt' })).not.toBeInTheDocument()
    const submit = screen.getByRole('button', { name: '确认并创建订单' })
    if (kind === 'switch') {
      expect(dialog).toHaveAccessibleDescription('支付完成后立即生效，新周期 30 天从生效时起算。')
      expect(within(dialog).getByText('当前套餐', { selector: 'dt' })).toBeVisible()
      expect(screen.getByRole('note')).toHaveTextContent(`当前套餐（原有套餐，${formatOrderTime(value.expires_at, 'zh-CN')} 到期）的剩余时间将在支付完成后作废。`)
      expect(screen.getByRole('checkbox', { name: '我已了解当前套餐剩余时间将作废' })).not.toHaveAccessibleDescription()
      expect(submit).toBeDisabled()
      fireEvent.click(submit); expect(requests).toHaveLength(0)
      await userEvent.click(screen.getByRole('checkbox'))
    }
    if (kind === 'renew') {
      expect(dialog).toHaveAccessibleDescription('支付完成后，在当前到期时间基础上顺延 30 天。')
      expect(within(dialog).getByText('当前到期', { selector: 'dt' }).nextElementSibling?.querySelector('time')).toHaveAttribute('datetime', value.expires_at)
    }
    if (kind === 'new') {
      expect(dialog).toHaveAccessibleDescription('支付完成后开通所选套餐。')
      expect(dialog.querySelector('time')).toBeNull()
      expect(within(dialog).queryByText('当前套餐', { selector: 'dt' })).not.toBeInTheDocument()
      expect(within(dialog).queryByText('当前到期')).not.toBeInTheDocument()
    }
    expect(dialog.querySelectorAll('time')).toHaveLength(kind === 'new' ? 0 : 1)
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
    expect(within(screen.getByRole('region', { name: '可选套餐' })).getByRole('status')).toHaveTextContent('已有待支付订单')
    expect(within(screen.getByRole('region', { name: '待支付订单' })).getByText('trade-123')).toBeVisible()
    expect(screen.getByRole('button', { name: '复制订单号' })).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: '取消订单' })); expect(canceled).toBe(0)
    await userEvent.click(screen.getByRole('button', { name: '暂不取消' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument(); expect(canceled).toBe(0)
    await userEvent.click(screen.getByRole('button', { name: '取消订单' }))
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
  it('refreshes the unread summary every 60 seconds and on focus, then stops after unmount', async () => {
    vi.useFakeTimers()
    const load = vi.spyOn(api, 'notifications').mockResolvedValue({ items: [], total: 0, next_cursor: null, unread_count: 0, popup_announcements: [] })
    function Host() { useNotificationSummary(user.username); return null }
    const { unmount } = wrap(<Host />)
    try {
      await act(() => vi.advanceTimersByTimeAsync(59999))
      expect(load).toHaveBeenCalledTimes(1)
      await act(() => vi.advanceTimersByTimeAsync(1))
      expect(load).toHaveBeenCalledTimes(2)
      focusManager.setFocused(false)
      await act(() => { focusManager.setFocused(true); return Promise.resolve() })
      expect(load).toHaveBeenCalledTimes(3)
      unmount()
      focusManager.setFocused(false)
      await act(() => { focusManager.setFocused(true); return Promise.resolve() })
      await act(() => vi.advanceTimersByTimeAsync(120000))
      expect(load).toHaveBeenCalledTimes(3)
    } finally {
      focusManager.setFocused(undefined)
    }
  })

  it('keeps the notification unread and stays on the list when marking it fails', async () => {
    const notice = {
      id: 'n1', kind: 'device_offline', source_id: 'd1', title: { zh_CN: '设备消息', en: 'Device alert' },
      summary: { zh_CN: '请检查设备', en: 'Check device' }, created_at: new Date().toISOString(), read_at: null,
      action: { page: '/dashboard/collection', entity_id: '' },
    }
    let attempts = 0
    server.use(
      http.get('*/api/v1/merchant/notifications', () => success({
        items: [notice], unread_count: 1, total: 1, next_cursor: null, popup_announcements: [],
      })),
      http.post('*/api/v1/merchant/notifications/read', () => { attempts++; return failure('INTERNAL_ERROR', 503) }),
    )
    const { router } = mount('/notifications')
    await userEvent.click(await screen.findByRole('link', { name: '查看详情' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('服务暂时不可用')
    expect(router.state.location.pathname).toBe('/notifications')
    expect(screen.getAllByLabelText('1 条未读通知').length).toBeGreaterThan(0)
    expect(attempts).toBe(1)
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
    await userEvent.click(screen.getByRole('radio', { name: '未读' }))
    await waitFor(() => expect(parameters.some((value) => value.get('unread_only') === 'true')).toBe(true))
    await userEvent.click(screen.getByRole('button', { name: '加载更多' }))
    await screen.findByRole('heading', { name: 'n2' })
    expect(parameters.at(-1)?.get('cursor')).toBe('next-notice')
    await userEvent.click(screen.getByRole('button', { name: '全部标为已读' }))
    await screen.findByText('暂无未读通知')
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
    await screen.findAllByText('request-one')
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
    await screen.findAllByText('request-two')
    expect(queries.at(-1)?.get('cursor')).toBe('opaque-cursor'); expect(queries.at(-1)?.has('merchant_id')).toBe(false)
    expect(within(screen.getByRole('table')).getAllByText('future.unknown')).toHaveLength(2)
    const mobile = screen.getByRole('list', { name: '操作记录卡片' })
    expect(within(mobile).getAllByRole('button', { name: '详情' })).toHaveLength(2)
    await userEvent.click(screen.getAllByRole('button', { name: '详情' })[0]!)
    expect(within(screen.getByRole('dialog')).getByText(/<script>raw<\/script>/)).toBeVisible()
    expect(screen.getByRole('dialog').querySelector('script')).toBeNull()
    expect(auditSearch({ result: 'success', action: ' auth.login ', merchant_id: 'other', cursor: 'bad' })).toEqual({ result: 'success', action: 'auth.login' })
  })
})

describe('visual revision interactions', () => {
  it('uses roving focus, wrapping arrows, Home, End and Space in the shared segmented control', async () => {
    function Host() {
      const [value, setValue] = useState('all')
      return <SegmentedControl label="筛选" value={value} options={[{ value: 'all', label: '全部' }, { value: 'disabled', label: '禁用', disabled: true }, { value: 'unread', label: '未读' }]} onValueChange={setValue} />
    }
    wrap(<Host />)
    const controls = within(screen.getByRole('radiogroup', { name: '筛选' }))
    const all = controls.getByRole('radio', { name: '全部' }), unread = controls.getByRole('radio', { name: '未读' })
    await userEvent.tab(); expect(all).toHaveFocus()
    await userEvent.keyboard('{ArrowRight}'); expect(unread).toHaveFocus(); expect(unread).toHaveAttribute('aria-checked', 'true')
    expect(all).toHaveAttribute('tabindex', '-1'); expect(controls.getByRole('radio', { name: '禁用' })).toBeDisabled()
    await userEvent.keyboard('{ArrowDown}'); expect(all).toHaveFocus()
    await userEvent.keyboard('{ArrowLeft}'); expect(unread).toHaveFocus()
    await userEvent.keyboard('{Home}'); expect(all).toHaveFocus()
    await userEvent.keyboard('{End}'); expect(unread).toHaveFocus()
    await userEvent.keyboard('{ArrowUp}'); expect(all).toHaveFocus()
    await userEvent.keyboard(' '); expect(all).toHaveAttribute('aria-checked', 'true')
  })

  it('distinguishes all and unread empty states and keeps the compact row actions readable', async () => {
    let empty = true
    server.use(http.get('*/api/v1/merchant/notifications', () => success({ items: empty ? [] : [{ id: 'compact', kind: 'device_offline', source_id: 'd', title: { zh_CN: '设备离线', en: 'Device offline' }, summary: { zh_CN: '请检查设备网络', en: 'Check the network' }, created_at: new Date().toISOString(), read_at: null, action: { page: '/dashboard/collection', entity_id: '' } }], total: empty ? 0 : 1, unread_count: empty ? 0 : 1, next_cursor: null, popup_announcements: [] })))
    const { client } = mount('/notifications')
    await screen.findByText('暂无通知')
    await userEvent.click(screen.getByRole('radio', { name: '未读' }))
    await screen.findByText('暂无未读通知')
    empty = false
    await act(() => invalidateNotifications(client, user.username))
    const row = await screen.findByRole('listitem', { name: '设备离线' })
    expect(within(row).getByRole('heading', { name: '设备离线' })).toBeVisible()
    expect(within(row).getByRole('button', { name: '标为已读' })).toBeVisible()
    expect(within(row).getByRole('link', { name: '查看详情' })).toHaveAttribute('href', '/devices')
  })

  const audit: Schema['PortalAuditDTO'] = { id: '1', actor_id: 'internal-10', actor_role: 'merchant', merchant_id: '1', action: 'subscription.purchase', target_type: 'subscription', target_id: 'purchase-1', result: 'success', created_at: '2026-11-06T12:00:00Z', ip: null, request_id: 'request-detail', safe_metadata: {} }
  it.each([['merchant', '商户'], ['admin', '管理员'], ['system', '系统']] as const)('localizes the %s actor without its internal id', (role, label) => {
    wrap(<AuditDetails item={{ ...audit, actor_role: role }} onClose={() => {}} />)
    const dialog = screen.getByRole('dialog', { name: '操作详情' })
    expect(within(dialog).getByText('操作者', { selector: 'dt' }).nextElementSibling).toHaveTextContent(label)
    expect(dialog).not.toHaveTextContent('internal-10')
    expect(within(dialog).getByText('操作对象', { selector: 'dt' }).nextElementSibling).toHaveTextContent('订阅套餐 · purchase-1')
    expect(within(dialog).queryByText('安全元数据')).not.toBeInTheDocument()
  })
  it('hides null metadata, preserves unknown targets and copies an unboxed request id', async () => {
    const user = userEvent.setup()
    // Defensive handling of null responses without weakening the generated DTO.
    const item = { ...audit, target_type: 'future_target', safe_metadata: null } as unknown as Schema['PortalAuditDTO']
    wrap(<AuditDetails item={item} onClose={() => {}} />)
    expect(screen.queryByText('安全元数据')).not.toBeInTheDocument()
    expect(screen.getByText('操作对象', { selector: 'dt' }).nextElementSibling).toHaveTextContent('future_target · purchase-1')
    await user.click(screen.getByRole('button', { name: '复制请求 ID' }))
    expect(await navigator.clipboard.readText()).toBe('request-detail')
  })
})
