import { createHash } from 'node:crypto'
import QRCode from 'qrcode'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
import type { components } from '../../../web/src/api/schema'
import { polishShot } from '../merchant/fixtures'

type Checkout = components['schemas']['PaymentCheckoutDTO']
const channelNames = { alipay: '支付宝', wxpay: '微信支付', qqpay: 'QQ 钱包' } as const

async function prepare(request: APIRequestContext) {
  const qrURLs = {
    alipay: 'https://qr.alipay.com/checkout-example',
    wxpay: 'wxp://checkout-example',
    qqpay: 'https://i.qianbao.qq.com/checkout-example',
  } satisfies Record<keyof typeof channelNames, string>
  const codes = Object.fromEntries(await Promise.all(Object.entries(qrURLs).map(async ([channel, url]) => {
    const png = await QRCode.toBuffer(url, { width: 300 })
    return [channel, { url, png: png.toString('base64') }] as const
  })))
  const response = await request.post('/__e2e/checkout', { data: { codes } })
  expect(response.status()).toBe(200)
  const fixture = await response.json() as { pid: string; v1_key: string; device_token: string; created_at: string }
  const form: Record<string, string> = { pid: fixture.pid, name: '月度会员', money: '30.00', out_trade_no: crypto.randomUUID(), clientip: '127.0.0.1', notify_url: 'https://merchant.example.test/notify' }
  const canonical = Object.keys(form).sort().map((key) => `${key}=${form[key]}`).join('&')
  form.sign = createHash('md5').update(canonical + fixture.v1_key).digest('hex')
  const created = await request.post('/mapi.php', { form })
  expect(created.status()).toBe(200)
  const result = await created.json() as { code: number; trade_no: string; error_code?: string }
  expect(result.code, result.error_code).toBe(1)
  return { trade: result.trade_no, deviceToken: fixture.device_token }
}
async function checkout(request: APIRequestContext, trade: string) {
  const response = await request.get(`/api/v1/checkout/${trade}`)
  expect(response.status()).toBe(200)
  return (await response.json() as { data: Checkout }).data
}
async function screenshots(page: Page, name: string) {
  for (const [theme, label] of [['light', '浅色'], ['dark', '深色']] as const) {
    await page.getByRole('button', { name: label, exact: true }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    await page.locator('img').evaluateAll(async (images) => {
      await Promise.all(images.map((image) => (image as HTMLImageElement).decode()))
      await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())))
    })
    await polishShot(page, name, true, 'checkout-shots')
    const shop = await page.locator('header').getByText('示例小店').boundingBox()
    const controls = await page.getByRole('button', { name: '浅色', exact: true }).boundingBox()
    expect(Math.abs((shop!.y + shop!.height / 2) - (controls!.y + controls!.height / 2))).toBeLessThan(2)
  }
}

async function expectChannelLayout(page: Page) {
  const buttons = Object.values(channelNames).map((name) => page.getByRole('button', { name, exact: true }))
  for (const button of buttons) await expect(button).toBeVisible()
  const boxes = (await Promise.all(buttons.map((button) => button.boundingBox()))).map((box) => box!)
  // Three channels share one row from the `sm` breakpoint and stack as full-width rows below it.
  const row = page.viewportSize()!.width >= 640
  for (const [previous, next] of boxes.slice(1).map((box, index) => [boxes[index]!, box] as const)) {
    expect(next.width).toBeCloseTo(previous.width)
    if (row) {
      expect(next.y).toBe(previous.y)
      expect(next.x).toBeGreaterThan(previous.x + previous.width)
    } else {
      expect(next.x).toBe(previous.x)
      expect(next.y).toBeGreaterThan(previous.y + previous.height)
    }
  }
}

test('public v1 checkout locks a channel, serves its signed QR and receives a real device payment', async ({ page, request }) => {
  const { trade, deviceToken } = await prepare(request)
  await page.goto(`/pay/${trade}`)
  await expect(page.getByRole('heading', { name: '月度会员' })).toBeVisible()
  const initial = await checkout(request, trade)
  expect(initial.channel).toBeNull()
  expect(initial.available_channels).toEqual(Object.keys(channelNames))
  await expectChannelLayout(page)
  await screenshots(page, 'unselected')
  const post = page.waitForResponse((response) => response.url().endsWith(`/checkout/${trade}/channel`) && response.request().method() === 'POST')
  await page.getByRole('button', { name: '支付宝', exact: true }).click()
  const selected = await post
  expect(selected.status()).toBe(200)
  expect((await selected.request().allHeaders()).origin).toBe(new URL(page.url()).origin)
  const data = await checkout(request, trade)
  expect(data.actual_cents).toBe(2999)
  const qr = page.getByRole('img', { name: '收款二维码' })
  await expect(qr).toHaveAttribute('src', data.qr_image_url!)
  await expect(qr).toBeVisible()
  await expect.poll(() => qr.evaluate((element) => (element as HTMLImageElement).naturalWidth)).toBe(300)
  await expect(page.getByRole('note')).toContainText('请务必支付 ¥29.99')
  await expect(page.getByRole('button', { name: '微信支付', exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'QQ 钱包', exact: true })).toHaveCount(0)
  await screenshots(page, 'locked')
  const receipt = await request.post('/api/v1/device/receipts', {
    headers: { Authorization: `Bearer ${deviceToken}` },
    data: { event_id: crypto.randomUUID(), channel: 'alipay', amount_cents: data.actual_cents, occurred_at: new Date().toISOString(), raw_text: '支付宝收款 29.99 元，月度会员' },
  })
  expect(receipt.status()).toBe(201)
  expect((await receipt.json() as { data: { match_state: string } }).data.match_state).toBe('matched')
  await expect(page.getByRole('heading', { name: '支付成功', exact: true })).toBeVisible({ timeout: 10000 })
  await expect(page.getByText('已支付 ¥29.99 · 支付宝')).toBeVisible()
  await expect(page.getByText('可以关闭本页')).toBeVisible()
  await screenshots(page, 'paid')
  expect((await checkout(request, trade)).status).toBe('paid')
})

// State coverage below uses the real GET expiry transition.
for (const state of ['expired', 'offline'] as const) {
  test(`public checkout explains ${state}`, async ({ page, request }) => {
    const { trade } = await prepare(request)
    expect((await request.post('/__e2e/checkout', { data: { trade_no: trade, state } })).status()).toBe(204)
    await page.goto(`/pay/${trade}`)
    if (state === 'expired') await expect(page.getByRole('heading', { name: '订单已过期' })).toBeVisible()
    else {
      await expect(page.getByRole('note')).toContainText('收款设备离线')
      await expectChannelLayout(page)
    }
    await expect(page.getByRole('img', { name: '收款二维码' })).toHaveCount(0)
    await screenshots(page, state)
  })
}
