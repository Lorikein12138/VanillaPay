import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createI18n } from '@/lib/i18n'
import { Overview } from '@/features/merchant/Overview'
import { DeviceCard, RecentOrders, TrendChart } from '@/features/merchant/OverviewCards'
import { statsRange } from '@/features/merchant/overview-data'
import { emptyStats, order, unboundDevice } from './fixtures'

const success = <T,>(data: T) => HttpResponse.json({ data, error: null, request_id: crypto.randomUUID() })
const server = setupServer(
  http.get('*/api/v1/merchant/stats', () => success({ ...emptyStats(), total_amount_cents: 12345, total_order_count: 4, status_counts: { paid: 2, pending: 1, expired: 1 } })),
  http.get('*/api/v1/merchant/device', () => success(unboundDevice)),
  http.get('*/api/v1/merchant/orders', () => success({ items: [], total: 0, next_cursor: null })),
)
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())
function mount(children: React.ReactNode, language: 'zh-CN' | 'en' = 'zh-CN') {
  return render(<AppProviders i18n={createI18n(language)} queryClient={createQueryClient()}>{children}</AppProviders>)
}

describe('overview sections', () => {
  it('renders all four metric values and their hints from the stats response', async () => {
    mount(<Overview name="fixture_shop" />)
    expect(await screen.findByText('¥123.45')).toBeVisible()
    const receipts = within(screen.getByRole('region', { name: '今日收款' }))
    expect(await receipts.findByText('较上一周期 +0.0%')).toBeVisible()
    const orders = within(screen.getByRole('region', { name: '今日订单' }))
    expect(orders.getByText('4', { exact: true })).toBeVisible()
    expect(orders.getByText('已支付 2 笔')).toBeVisible()
    const rate = within(screen.getByRole('region', { name: '支付成功率' }))
    expect(rate.getByText('66.7%')).toBeVisible()
    expect(rate.getByText('不含待支付 1 笔')).toBeVisible()
    expect(within(screen.getByRole('region', { name: '待支付' })).getByText('1', { exact: true })).toHaveClass('text-accent-text')
  })
  it('reserves section space with skeletons until each request resolves', async () => {
    let release: () => void = () => { throw new Error('missing loading gate') }
    const gate = new Promise<void>((resolve) => { release = resolve })
    server.use(
      http.get('*/api/v1/merchant/stats', async () => { await gate; return success(emptyStats()) }),
      http.get('*/api/v1/merchant/device', async () => { await gate; return success(unboundDevice) }),
      http.get('*/api/v1/merchant/orders', async () => { await gate; return success({ items: [], total: 0, next_cursor: null }) }),
    )
    const { container } = mount(<Overview name="fixture_shop" />)
    try {
      expect(container.querySelectorAll('[aria-busy="true"]')).toHaveLength(4)
      expect(container.querySelectorAll('.animate-pulse').length).toBeGreaterThanOrEqual(7)
      expect(screen.queryByText('¥0.00')).not.toBeInTheDocument()
      expect(screen.queryByText('暂无订单')).not.toBeInTheDocument()
    } finally { release() }
    expect(await screen.findByText('¥0.00')).toBeVisible()
    await waitFor(() => expect(container.querySelector('[aria-busy="true"]')).not.toBeInTheDocument())
  })
  it.each(['unbound', 'online', 'offline'] as const)('renders the %s device with a textual status', (state) => {
    mount(<DeviceCard device={{ ...unboundDevice, bound: state !== 'unbound', online: state === 'online', last_seen_at: '2026-11-05T01:00:00Z', offline_at: state === 'offline' ? '2026-11-05T00:50:00Z' : null }} now={new Date('2026-11-05T01:00:12Z')} />)
    if (state === 'unbound') expect(screen.getByText('尚未绑定监控设备')).toBeVisible()
    else {
      expect(screen.getByText('未命名设备')).toBeVisible()
      expect(screen.getByText(state === 'online' ? '在线' : '离线', { exact: true })).toBeVisible()
      expect(screen.getByText(state === 'online' ? '心跳 12秒钟前' : '最后在线 10分钟前')).toBeVisible()
    }
  })
  it('uses the last heartbeat when an offline device has no offline_at', () => {
    mount(<DeviceCard device={{ ...unboundDevice, bound: true, name: 'Fixture device', last_seen_at: '2026-11-05T01:00:00Z' }} now={new Date('2026-11-05T01:00:12Z')} />)
    expect(screen.getByText('Fixture device')).toBeVisible()
    expect(screen.getByText('最后在线 12秒钟前')).toBeVisible()
  })
  it('renders all order states, null channel and the actual amount with original fallback', () => {
    const orders = [order('paid', 'alipay'), order('pending', null), order('expired', 'wxpay'), order('refunded', 'alipay')]
    mount(<RecentOrders orders={orders} />)
    for (const label of ['已支付', '待支付', '已过期', '已退款', '未分配', '支付宝', '微信支付']) expect(screen.getAllByText(label, { exact: true })[0]).toBeVisible()
    const rows = screen.getAllByRole('row').slice(1)
    expect(within(rows[0]!).getByText('¥123.46')).toBeVisible()
    expect(within(rows[1]!).getByText('¥123.45')).toBeVisible()
  })
  it('keeps zero chart bars at zero and exposes an accessible data table', () => {
    const points = Array.from({ length: 7 }, (_, index) => ({ date: `2026-11-0${index + 1}`, amount_cents: 0, paid_count: 0, pending_count: 0, expired_count: 0 }))
    const { container } = mount(<TrendChart points={points} format={() => '0.0'} />)
    expect(screen.getByText('暂无收款')).toBeVisible()
    expect(container.querySelectorAll('[data-date]')).toHaveLength(7)
    for (const bar of container.querySelectorAll('[data-date]')) expect(bar).toHaveStyle({ height: '0px' })
    expect(within(screen.getByRole('table')).getAllByRole('row')).toHaveLength(8)
  })
  it.each(['device', 'orders', 'stats'] as const)('isolates a failed %s request and recovers on retry', async (section) => {
    let fail = true
    server.use(http.get(`*/api/v1/merchant/${section}`, () => fail ? HttpResponse.json({ error: { code: 'INTERNAL_ERROR' } }, { status: 500 }) : success(section === 'device' ? unboundDevice : section === 'orders' ? { items: [], total: 0, next_cursor: null } : emptyStats())))
    mount(<Overview name="fixture_shop" />)
    await screen.findAllByRole('alert')
    if (section !== 'device') expect(await screen.findByText('尚未绑定监控设备')).toBeVisible()
    if (section !== 'orders') expect(await screen.findByText('暂无订单')).toBeVisible()
    if (section !== 'stats') expect(await screen.findByText('¥123.45')).toBeVisible()
    fail = false
    for (const button of screen.getAllByRole('button', { name: '重试' })) await userEvent.click(button)
    if (section === 'stats') {
      await screen.findByText('¥0.00')
      // The previous-period error becomes visible only after current stats recover.
      for (const button of screen.queryAllByRole('button', { name: '重试' })) await userEvent.click(button)
    }
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())
  })
  it('reuses the seven-day query and sends the browser IANA timezone and limit=6', async () => {
    const calls: URL[] = []
    server.use(http.get('*/api/v1/merchant/stats', ({ request }) => { calls.push(new URL(request.url)); return success(emptyStats()) }))
    let limit = ''
    server.use(http.get('*/api/v1/merchant/orders', ({ request }) => { limit = new URL(request.url).searchParams.get('limit') ?? ''; return success({ items: [], total: 0, next_cursor: null }) }))
    mount(<Overview name="fixture_shop" />)
    await waitFor(() => expect(calls).toHaveLength(3))
    await userEvent.click(screen.getByRole('button', { name: '近 7 天' }))
    await waitFor(() => expect(calls).toHaveLength(4))
    expect(calls.every((url) => url.searchParams.get('timezone') === Intl.DateTimeFormat().resolvedOptions().timeZone)).toBe(true)
    expect(new Set(calls.map((url) => url.search)).size).toBe(4)
    expect(limit).toBe('6')
    expect(screen.getByRole('button', { name: '近 7 天' })).toHaveAttribute('aria-pressed', 'true')
    expect(await screen.findByText('上一周期无收款')).toBeVisible()
    expect(screen.getByText('—', { exact: true })).toBeVisible()
  })
  it.each(['previous', 'trend'] as const)('isolates a failed %s query from current metrics', async (section) => {
    let fail = true
    const current = statsRange(new Date(), 1, Intl.DateTimeFormat().resolvedOptions().timeZone)
    server.use(http.get('*/api/v1/merchant/stats', ({ request }) => {
      const params = new URL(request.url).searchParams
      const target = section === 'previous' ? params.get('end') === current.start : Date.parse(params.get('end')!) - Date.parse(params.get('start')!) > 2 * 86400000
      return fail && target ? HttpResponse.json({ error: { code: 'INTERNAL_ERROR' } }, { status: 500 }) : success({ ...emptyStats(), total_amount_cents: 12345 })
    }))
    mount(<Overview name="fixture_shop" />)
    await screen.findByRole('alert')
    expect(await screen.findByText('¥123.45')).toBeVisible()
    expect(await screen.findByText('尚未绑定监控设备')).toBeVisible()
    expect(await screen.findByText('暂无订单')).toBeVisible()
    fail = false
    await userEvent.click(screen.getByRole('button', { name: '重试' }))
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())
  })
  it('renders English copy and localized weekday without placeholder translation keys', async () => {
    mount(<Overview name="fixture_shop" />, 'en')
    expect(await screen.findByRole('heading', { name: /Good .*fixture_shop/ })).toBeVisible()
    expect(await screen.findByText('No monitoring device bound')).toBeVisible()
    expect(screen.getByText(/Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday/)).toBeVisible()
    expect(screen.queryByText(/merchant\.overview\./)).not.toBeInTheDocument()
  })
})
