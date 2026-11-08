import { fileURLToPath } from 'node:url'
import type { Page } from '@playwright/test'
import type { components } from '../../../web/src/api/schema'
import { expect, login, noOverflow, test, clickControl, openMenu, polishShot } from './fixtures'

test.use({ timezoneId: 'Asia/Shanghai' })
function fixtureOrder(status = 'pending', index = 0) {
  const trade = `VP-${crypto.randomUUID()}`
  return { trade_no: trade, out_trade_no: `M-${trade}`, name: `云服务 · 月度方案 ${index + 1}`, channel: status === 'pending' ? null : 'alipay', status, original_cents: 12345, actual_cents: status === 'pending' ? null : 12346, created_at: new Date(Date.now() - index * 1000).toISOString(), paid_at: status === 'paid' || status === 'refunded' ? new Date().toISOString() : null }
}
function rows(page: Page) {
  return (page.viewportSize()?.width ?? 1280) < 768 ? page.getByRole('list', { name: '订单卡片' }).locator(':scope > li') : page.getByRole('table', { name: '订单', exact: true }).locator('tbody tr')
}
async function screenshot(page: Page, name: string) {
  const width = page.viewportSize()?.width
  if (width !== 360 && width !== 1280) return
  const theme = width === 360 ? 'dark' : 'light'
  await page.evaluate(() => document.fonts.ready)
  await noOverflow(page)
  await polishShot(page, name === 'list' ? 'orders' : name === 'detail' ? 'order-detail' : 'order-action-dialog')
  await page.screenshot({ path: fileURLToPath(new URL(`../../../.artifacts/orders-shots/${name}-${theme}-${width}.png`, import.meta.url)), fullPage: name !== 'confirmation', animations: 'disabled' })
}

test('filters orders, restores shared URLs, loads cursors and exports only the current merchant and filters', async ({ page, account, request }) => {
  const prefix = `MER-${crypto.randomUUID()}`
  const orders = Array.from({ length: 23 }, (_, index) => ({ ...fixtureOrder(index === 0 ? 'pending' : index === 1 ? 'expired' : 'paid', index), out_trade_no: `${prefix}-${index}` }))
  expect((await request.post('/__e2e/orders', { data: { pid: account.pid, orders } })).status()).toBe(204)
  const other = await (await request.post('/__e2e/merchant', { data: {} })).json() as { pid: string }
  const otherOrder = { ...fixtureOrder('paid'), name: 'Other merchant private order', out_trade_no: `${prefix}-other` }
  expect((await request.post('/__e2e/orders', { data: { pid: other.pid, orders: [otherOrder] } })).status()).toBe(204)
  await page.goto('/login'); await login(page, account)
  await openMenu(page); await page.getByRole('link', { name: '订单', exact: true }).click()
  await expect(rows(page)).toHaveCount(20)
  await page.getByRole('button', { name: '加载更多' }).click()
  await expect(rows(page)).toHaveCount(23)
  await page.getByLabel('状态', { exact: true }).selectOption('paid')
  await page.getByLabel('通道', { exact: true }).selectOption('alipay')
  await page.getByLabel('开始时间', { exact: true }).fill('2000-01-01T00:00')
  await page.getByLabel('结束时间', { exact: true }).fill('2099-01-01T00:00')
  await page.getByRole('button', { name: '应用筛选' }).click()
  await expect(page).toHaveURL(/status=paid/)
  await expect(rows(page)).toHaveCount(20)
  const shared = page.url()
  expect(new URL(shared).searchParams.get('start')).toBe('1999-12-31T16:00:00.000Z')
  await page.reload()
  await expect(page.getByLabel('状态', { exact: true })).toHaveValue('paid')
  await expect(page.getByLabel('通道', { exact: true })).toHaveValue('alipay')
  await expect(page.getByLabel('搜索订单')).toHaveValue('')
  await expect(page.getByLabel('开始时间', { exact: true })).toHaveValue('2000-01-01T00:00')
  await page.getByRole('button', { name: '加载更多' }).click()
  await expect(rows(page)).toHaveCount(21)
  const downloadPromise = page.waitForEvent('download')
  await page.getByRole('button', { name: '导出 CSV' }).click()
  const download = await downloadPromise
  expect(download.suggestedFilename()).toBe('orders.csv')
  expect(await download.failure()).toBeNull()
  const stream = await download.createReadStream()
  stream.setEncoding('utf8')
  const parts: string[] = []
  for await (const part of stream) {
    if (typeof part !== 'string') throw new Error('download did not decode as UTF-8')
    parts.push(part)
  }
  const csv = parts.join('')
  expect(csv).toContain(orders[2]!.trade_no)
  expect(csv).not.toContain(orders[0]!.trade_no)
  expect(csv).not.toContain(otherOrder.trade_no)
  expect(csv.trim().split('\n')).toHaveLength(22)
  await page.getByLabel('搜索订单').fill(orders[2]!.out_trade_no)
  await page.getByRole('button', { name: '应用筛选' }).click()
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page).first()).toContainText(orders[2]!.trade_no)
  await page.reload()
  await expect(page.getByLabel('搜索订单')).toHaveValue(orders[2]!.out_trade_no)
  await expect(rows(page)).toHaveCount(1)
  const filteredDownload = page.waitForEvent('download')
  await page.getByRole('button', { name: '导出 CSV' }).click()
  const filteredStream = await (await filteredDownload).createReadStream()
  filteredStream.setEncoding('utf8')
  const filteredParts: string[] = []
  for await (const part of filteredStream) {
    if (typeof part !== 'string') throw new Error('download did not decode as UTF-8')
    filteredParts.push(part)
  }
  const filteredCSV = filteredParts.join('')
  expect(filteredCSV).toContain(orders[2]!.trade_no)
  expect(filteredCSV).not.toContain(orders[3]!.trade_no)
  await noOverflow(page)
})

test('shows order details, supplements a payment and records a confirmed offline refund', async ({ page, account, request }) => {
  const pending = { ...fixtureOrder(), allowed_channels_snapshot: ['alipay'] }
  expect((await request.post('/__e2e/orders', { data: { pid: account.pid, orders: [pending, fixtureOrder('paid', 1), fixtureOrder('refunded', 2)] } })).status()).toBe(204)
  await page.goto('/login'); await login(page, account)
  await expect(page).toHaveURL(/\/dashboard$/)
  await clickControl(page, page.viewportSize()?.width === 360 ? '深色' : '浅色')
  await openMenu(page); await page.getByRole('link', { name: '订单', exact: true }).click()
  await expect(rows(page)).toHaveCount(3)
  await screenshot(page, 'list')
  await page.getByRole('link', { name: pending.trade_no, exact: true }).filter({ visible: true }).click()
  await expect(page.getByRole('heading', { name: '订单详情', exact: true })).toBeVisible()
  await expect(page.getByText('¥123.45', { exact: true })).toBeVisible()
  await expect(page.getByText(pending.out_trade_no, { exact: true })).toBeVisible()
  await screenshot(page, 'detail')
  await page.getByRole('button', { name: '补单', exact: true }).click()
  await expect(page.getByLabel('补单通道').locator('option[value="wxpay"]')).toBeDisabled()
  await expect(page.getByLabel('补单通道').locator('option[value="alipay"]')).toBeEnabled()
  await polishShot(page, 'supplement-channels-dialog', false, 'contract-backend-fixes-shots')
  await page.getByLabel('补单通道').selectOption('alipay')
  await page.getByLabel('原因', { exact: true }).fill('已核对银行收款记录')
  await screenshot(page, 'confirmation')
  const supplementResponse = page.waitForResponse((response) => response.url().endsWith('/supplement') && response.request().method() === 'POST')
  await page.getByRole('button', { name: '确认补单' }).click()
  expect((await supplementResponse).status()).toBe(200)
  await expect(page.getByRole('status')).toContainText('补单成功')
  await expect(page.getByText('已核对银行收款记录', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '补单', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '标记退款' }).click()
  await polishShot(page, 'refund-order-dialog', false)
  await expect(page.getByRole('dialog')).toContainText('不会自动退回资金')
  await page.getByLabel('原因', { exact: true }).fill('已线下原路全额退回')
  await expect(page.getByRole('button', { name: '确认退款' })).toBeDisabled()
  await page.getByRole('checkbox').check()
  const refundResponse = page.waitForResponse((response) => response.url().endsWith('/refund') && response.request().method() === 'POST')
  await page.getByRole('button', { name: '确认退款' }).click()
  expect((await refundResponse).status()).toBe(200)
  await expect(page.getByRole('status')).toContainText('已标记退款')
  await expect(page.getByText('已退款', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '标记退款' })).toHaveCount(0)
  await noOverflow(page)
})

test('shows real callback attempts and queues a replay with CSRF, Origin and idempotency headers', async ({ page, account, request }) => {
  // The production outbound client rejects this fixture target before dialing.
  const paid = { ...fixtureOrder('paid'), notify_url: 'https://127.0.0.1/notify', return_url: 'https://shop.example.test/return', callback_state: 'failed' }
  const capped = { ...paid, ...fixtureOrder('paid', 1), callback_generation: 2147483647 }
  expect((await request.post('/__e2e/orders', { data: { pid: account.pid, orders: [paid, capped] } })).status()).toBe(204)
  await page.goto(`/login?redirect=${encodeURIComponent(`/orders/${paid.trade_no}`)}`); await login(page, account)
  await expect(page).toHaveURL(new RegExp(`/orders/${paid.trade_no}$`))
  await clickControl(page, page.viewportSize()?.width === 360 ? '深色' : '浅色')
  await expect(page.getByText('fixture callback failed', { exact: true })).toBeVisible()
  await expect(page.getByText('CALLBACK_NOT_CONFIRMED', { exact: true })).toBeVisible()
  await expect(page.getByText(paid.return_url, { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '重发回调', exact: true }).click()
  await polishShot(page, 'callback-order-dialog', false)
  const replayResponse = page.waitForResponse((response) => response.url().endsWith('/callback-replay'))
  await page.getByRole('button', { name: '确认重发回调' }).click()
  const response = await replayResponse
  expect(response.status()).toBe(202)
  const headers = await response.request().allHeaders()
  expect(headers['idempotency-key']).toMatch(/^[0-9a-f-]{36}$/)
  expect(headers['x-csrf-token']).toBeTruthy()
  expect(headers.origin).toBe(new URL(page.url()).origin)
  await expect(page.getByRole('status')).toContainText('回调已加入重发队列')
  await noOverflow(page)
  await page.goto(`/orders/${capped.trade_no}`)
  await expect(page.getByRole('button', { name: '重发回调', exact: true })).toBeDisabled()
  await expect(page.getByText('已达重发上限（2147483646/2147483646）')).toBeVisible()
  await polishShot(page, 'callback-replay-limit', true, 'contract-backend-fixes-shots')
})

test('rejects and resolves pending refund requests through the real backend', async ({ page, account, request }) => {
  const rejected = { ...fixtureOrder('paid'), refund_pending: true }
  const resolved = { ...fixtureOrder('paid', 1), refund_pending: true }
  const stale = { ...fixtureOrder('paid', 2), refund_pending: true }
  expect((await request.post('/__e2e/orders', { data: { pid: account.pid, orders: [rejected, resolved, stale] } })).status()).toBe(204)
  await page.goto('/login'); await login(page, account)
  await expect(page).toHaveURL(/\/dashboard$/)
  await clickControl(page, page.viewportSize()?.width === 360 ? '深色' : '浅色')
  await page.goto(`/orders/${rejected.trade_no}`)
  await page.getByRole('button', { name: '拒绝退款', exact: true }).click()
  await polishShot(page, 'reject-order-dialog', false)
  await page.getByLabel('原因', { exact: true }).fill('申请信息待核实')
  await page.getByRole('button', { name: '确认拒绝退款' }).click()
  await expect(page.getByRole('status')).toContainText('退款申请已拒绝')
  await expect(page.getByText('已拒绝', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: '拒绝退款', exact: true })).toHaveCount(0)
  await page.goto(`/orders/${resolved.trade_no}`)
  await page.getByRole('button', { name: '处理退款', exact: true }).click()
  await polishShot(page, 'resolve-order-dialog', false)
  await page.getByLabel('原因', { exact: true }).fill('已核实并线下全额退款')
  await page.getByRole('checkbox').check()
  await page.getByRole('button', { name: '确认处理退款' }).click()
  await expect(page.getByRole('status')).toContainText('退款申请已处理')
  await expect(page.getByText('已完成', { exact: true })).toBeVisible()
  await expect(page.getByText('已退款', { exact: true })).toBeVisible()

  await page.goto(`/orders/${stale.trade_no}`)
  await page.getByRole('button', { name: '处理退款', exact: true }).click()
  await page.getByLabel('原因', { exact: true }).fill('已核实退款申请')
  await page.getByRole('checkbox').check()
  const identity = await page.request.get('/api/v1/merchant/me')
  const { data } = await identity.json() as { data: components['schemas']['SessionIdentity'] }
  const reject = await page.request.post(`/api/v1/merchant/refunds/${stale.trade_no}-refund/resolve`, {
    headers: { Origin: new URL(page.url()).origin, 'X-CSRF-Token': data.csrf_token, 'Idempotency-Key': crypto.randomUUID() },
    data: { action: 'reject', reason: '申请信息待核实', confirm: false },
  })
  expect(reject.status()).toBe(200)
  await page.getByRole('button', { name: '确认处理退款' }).click()
  await expect(page.getByRole('alert')).toContainText('该退款申请已被处理')
  await polishShot(page, 'refund-already-handled-dialog', false, 'contract-backend-fixes-shots')
  await page.getByRole('button', { name: '取消', exact: true }).click()
  await expect(page.getByText('已拒绝', { exact: true })).toBeVisible()
})

test('guards unauthenticated access and hides missing and other-merchant orders behind 404', async ({ page, account, request }) => {
  expect((await page.request.get('/api/v1/merchant/orders')).status()).toBe(401)
  await page.goto('/orders/not-an-order')
  await expect(page).toHaveURL(/\/login\?redirect=/)
  await login(page, account)
  await expect(page.getByRole('heading', { name: '订单不存在' })).toBeVisible()
  const other = await (await request.post('/__e2e/merchant', { data: {} })).json() as { pid: string }
  const hidden = fixtureOrder('paid')
  expect((await request.post('/__e2e/orders', { data: { pid: other.pid, orders: [hidden] } })).status()).toBe(204)
  const hiddenResponse = page.waitForResponse((response) => response.url().endsWith(`/api/v1/merchant/orders/${hidden.trade_no}`))
  await page.goto(`/orders/${hidden.trade_no}`)
  expect((await hiddenResponse).status()).toBe(404)
  await expect(page.getByRole('heading', { name: '订单不存在' })).toBeVisible()
  await page.getByRole('link', { name: '返回订单列表' }).click()
  await expect(page.getByText('没有符合筛选条件的订单')).toBeVisible()
})
