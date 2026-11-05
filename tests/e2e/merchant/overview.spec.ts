import { fileURLToPath } from 'node:url'
import { expect, login, noOverflow, test } from './fixtures'

test.use({ timezoneId: 'Asia/Shanghai' })
const day = 86400000

test('shows real scoped stats, a fixed seven-day chart and the latest six orders', async ({ page, account, request }, testInfo) => {
  const now = new Date()
  const date = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).format(now)
  const today = Date.parse(`${date}T00:00:00+08:00`)
  await page.clock.setFixedTime(now)
  const prefix = `VP${date.replaceAll('-', '')}${crypto.randomUUID().slice(0, 8)}`
  function row(index: number, daysAgo: number, hour: number, status: string, channel: string | null, original: number, paidDaysAgo = daysAgo) {
    return { trade_no: `${prefix}-${index}`, name: `会员方案 ${index}`, channel, status, original_cents: original, actual_cents: channel === null ? null : original + 1, created_at: new Date(today - daysAgo * day + hour * 3600000).toISOString(), paid_at: status === 'paid' || status === 'refunded' ? new Date(today - paidDaysAgo * day + hour * 3600000).toISOString() : null }
  }
  const orders = [
    row(0, 0, 6, 'paid', 'alipay', 120000), row(1, 0, 5, 'pending', null, 2500),
    row(2, 0, 4, 'expired', 'wxpay', 3000), row(3, 0, 3, 'refunded', 'qqpay', 9900),
    // Created yesterday but paid today: the amount and order-count bases differ.
    row(4, 1, 5, 'paid', 'wxpay', 80000, 0), row(5, 6, 4, 'paid', 'qqpay', 100000),
    row(6, 7, 4, 'paid', 'alipay', 50000), row(7, 29, 4, 'paid', 'wxpay', 150000),
    row(8, 30, 4, 'paid', 'qqpay', 200000), row(9, 1, 2, 'paid', 'alipay', 100000),
  ]
  expect((await request.post('/__e2e/orders', { data: { pid: account.pid, orders } })).status()).toBe(204)
  expect((await request.post('/__e2e/device', { data: { pid: account.pid, state: 'online', name: '收银台安卓机' } })).status()).toBe(204)
  const statsRequests: URL[] = []
  page.on('request', (incoming) => { if (new URL(incoming.url()).pathname === '/api/v1/merchant/stats') statsRequests.push(new URL(incoming.url())) })
  await page.goto('/login')
  await login(page, account)
  await expect(page.getByRole('heading', { name: new RegExp(account.username) })).toBeVisible()
  const cases = [
    { label: '今日', receipts: '¥2,000.00', count: '3', paid: 1, rate: '50.0%', change: '+100.0%' },
    { label: '近 7 天', receipts: '¥4,000.00', count: '6', paid: 4, rate: '80.0%', change: '+700.0%' },
    { label: '近 30 天', receipts: '¥6,000.00', count: '8', paid: 6, rate: '85.7%', change: '+200.0%' },
  ]
  for (const value of cases) {
    await page.getByRole('button', { name: value.label, exact: true }).click()
    await expect(page.getByRole('button', { name: value.label, exact: true })).toHaveAttribute('aria-pressed', 'true')
    const receipts = page.getByRole('region', { name: '收款指标', exact: true }).getByRole('region', { name: `${value.label}收款`, exact: true })
    await expect(receipts).toContainText(value.receipts)
    await expect(receipts).toContainText(`较上一周期 ${value.change}`)
    const count = page.getByRole('region', { name: `${value.label}订单`, exact: true })
    await expect(count.locator('p')).toHaveText(value.count)
    await expect(count).toContainText(`已支付 ${value.paid} 笔`)
    await expect(page.getByRole('region', { name: '支付成功率', exact: true })).toContainText(value.rate)
    await expect(page.getByRole('region', { name: '支付成功率', exact: true })).toContainText('不含待支付 1 笔')
    await expect(page.getByRole('region', { name: '待支付', exact: true }).locator('p')).toHaveText('1')
    const bars = page.getByRole('img', { name: '近 7 天收款', exact: true })
    await expect(bars.locator('span')).toHaveText(['1.0', '0.0', '0.0', '0.0', '0.0', '1.0', '2.0'])
    await expect(bars.locator('[data-date]').last()).toHaveCSS('height', '130px')
    await expect(bars.locator('[data-date]').last()).toHaveClass(/bg-accent/)
  }
  const weekStart = new Date(today - 6 * day).toISOString()
  expect(statsRequests.filter((url) => url.searchParams.get('start') === weekStart)).toHaveLength(1)
  expect(statsRequests.every((url) => url.searchParams.get('timezone') === 'Asia/Shanghai')).toBe(true)
  const table = page.getByRole('table').filter({ has: page.getByRole('columnheader', { name: '订单号' }) })
  const rows = table.locator('tbody tr')
  await expect(rows).toHaveCount(6)
  for (const [position, index, channel, amount, status] of [
    [0, 0, '支付宝', '¥1,200.01', '已支付'], [1, 1, '未分配', '¥25.00', '待支付'],
    [2, 2, '微信支付', '¥30.01', '已过期'], [3, 3, 'QQ 钱包', '¥99.01', '已退款'],
    [4, 4, '微信支付', '¥800.01', '已支付'], [5, 9, '支付宝', '¥1,000.01', '已支付'],
  ] as const) {
    await expect(rows.nth(position).locator('td').first()).toHaveText(`${prefix}-${index}`)
    await expect(rows.nth(position)).toContainText(`会员方案 ${index}`)
    await expect(rows.nth(position)).toContainText(channel)
    await expect(rows.nth(position)).toContainText(amount)
    await expect(rows.nth(position)).toContainText(status)
  }
  await expect(page.getByText('收银台安卓机', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '今日', exact: true }).click()
  await expect(page.getByRole('region', { name: '今日收款', exact: true })).toContainText('¥2,000.00')
  await page.evaluate(() => document.fonts.ready)
  for (const theme of ['light', 'dark'] as const) {
    await page.getByRole('button', { name: theme === 'light' ? '浅色' : '深色', exact: true }).click()
    await noOverflow(page)
    await page.screenshot({ path: fileURLToPath(new URL(`../../../.artifacts/overview-shots/dashboard-${theme}-${testInfo.project.name}.png`, import.meta.url)), fullPage: true, animations: 'disabled' })
  }
})

test('renders unbound, online and offline devices from the real backend', async ({ page, account, request }) => {
  await page.goto('/login')
  await login(page, account)
  const card = page.getByRole('region', { name: '监控设备', exact: true })
  await expect(card).toContainText('尚未绑定监控设备')
  await expect(page.getByText('暂无订单', { exact: true })).toBeVisible()
  await expect(page.getByText('上一周期无收款', { exact: true })).toBeVisible()
  await expect(page.getByText('暂无收款', { exact: true })).toBeVisible()
  for (const state of ['online', 'offline'] as const) {
    expect((await request.post('/__e2e/device', { data: { pid: account.pid, state, name: `Fixture ${state}` } })).status()).toBe(204)
    await page.reload()
    await expect(card).toContainText(`Fixture ${state}`)
    await expect(card.getByText(state === 'online' ? '在线' : '离线', { exact: true })).toBeVisible()
    await expect(card).toContainText(state === 'online' ? '心跳' : '最后在线')
    await noOverflow(page)
  }
})
