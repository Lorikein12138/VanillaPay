import type { Response } from '@playwright/test'
import type { components } from '@/api/schema'
import { expect, login, test } from './fixtures'

test.use({ timezoneId: 'Asia/Shanghai' })

for (const action of ['supplement', 'refund', 'resolve'] as const) {
  test(`refreshes cached overview statistics after ${action} through SPA navigation`, async ({ page, account, request }) => {
    const trade = `VP-${crypto.randomUUID()}`
    const pending = action === 'supplement'
    const timestamp = new Date().toISOString()
    expect((await request.post('/__e2e/orders', { data: { pid: account.pid, orders: [{ trade_no: trade, name: '统计缓存回归订单', channel: pending ? null : 'alipay', status: pending ? 'pending' : 'paid', original_cents: 12345, actual_cents: pending ? null : 12345, created_at: timestamp, paid_at: pending ? null : timestamp, refund_pending: action === 'resolve' }] } })).status()).toBe(204)
    const responses: Response[] = []
    page.on('response', (response) => { if (new URL(response.url()).pathname === '/api/v1/merchant/stats') responses.push(response) })
    await page.goto('/login')
    await login(page, account)
    const receipts = page.getByRole('region', { name: '今日收款', exact: true })
    const count = page.getByRole('region', { name: '今日订单', exact: true })
    const unpaid = page.getByRole('region', { name: '待支付', exact: true }).locator('p')
    const trend = page.getByRole('img', { name: '近 7 天收款', exact: true }).locator('span')
    await expect(receipts).toContainText(pending ? '¥0.00' : '¥123.45')
    await expect(count).toContainText(`已支付 ${pending ? 0 : 1} 笔`)
    await expect(unpaid).toHaveText(pending ? '1' : '0')
    await expect(trend).toHaveText(['0.0', '0.0', '0.0', '0.0', '0.0', '0.0', pending ? '0.0' : '0.1'])
    const daily = responses.find((response) => {
      const params = new URL(response.url()).searchParams
      const start = Date.parse(params.get('start') ?? '')
      const end = Date.parse(params.get('end') ?? '')
      return end - start === 86400000 && end > Date.now()
    })
    expect(daily).toBeDefined()
    // Keep the same document and QueryClient: only use links after the initial login.
    const documentMarker = crypto.randomUUID()
    await page.evaluate((marker) => { document.documentElement.dataset.cacheRegression = marker }, documentMarker)
    await page.getByRole('link', { name: '订单', exact: true }).click()
    await page.getByRole('link', { name: trade, exact: true }).filter({ visible: true }).click()
    await page.getByRole('button', { name: action === 'supplement' ? '补单' : action === 'refund' ? '标记退款' : '处理退款', exact: true }).click()
    if (pending) await page.getByLabel('补单通道').selectOption('alipay')
    await page.getByLabel('原因', { exact: true }).fill('已核实线下收付款')
    if (!pending) await page.getByRole('checkbox').check()
    await page.getByRole('button', { name: pending ? '确认补单' : action === 'refund' ? '确认退款' : '确认处理退款', exact: true }).click()
    await expect(page.getByRole('status')).toContainText(pending ? '补单成功' : action === 'refund' ? '已标记退款' : '退款申请已处理')
    await expect(page.getByText(pending ? '已支付' : '已退款', { exact: true })).toBeVisible()
    await page.getByRole('link', { name: '概览', exact: true }).click()
    await expect(receipts).toContainText(pending ? '¥123.45' : '¥0.00')
    await expect(count).toContainText(`已支付 ${pending ? 1 : 0} 笔`)
    await expect(count.locator('p')).toHaveText(pending ? '1' : '0')
    await expect(unpaid).toHaveText('0')
    await expect(page.getByRole('region', { name: '支付成功率', exact: true })).toContainText('不含待支付 0 笔')
    await expect(trend).toHaveText(['0.0', '0.0', '0.0', '0.0', '0.0', '0.0', pending ? '0.1' : '0.0'])
    await expect(page.locator('html')).toHaveAttribute('data-cache-regression', documentMarker)
    const dailyResponses = responses.filter((response) => response.url() === daily!.url())
    expect(dailyResponses.length).toBeGreaterThanOrEqual(2)
    const envelope = await dailyResponses.at(-1)!.json() as { data: components['schemas']['PortalStatsDTO'] }
    // The existing stats API excludes refunded orders, including from total_order_count.
    expect(envelope.data.status_counts).toEqual({ paid: pending ? 1 : 0, pending: 0, expired: 0 })
    expect(envelope.data.total_order_count).toBe(pending ? 1 : 0)
    expect(envelope.data.total_amount_cents).toBe(pending ? 12345 : 0)
    const recent = page.getByRole('region', { name: '最近订单', exact: true }).getByRole('row').filter({ has: page.getByText(trade, { exact: true }) })
    await expect(recent).toContainText(pending ? '已支付' : '已退款')
  })
}
