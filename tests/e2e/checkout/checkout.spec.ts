import { prepareCheckout, selectCheckoutScreenshotTheme } from './fixtures'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
import type { components } from '../../../web/src/api/schema'
import { polishShot } from '../merchant/fixtures'

type Checkout = components['schemas']['PaymentCheckoutDTO']
const channelNames = { alipay: '支付宝', wxpay: '微信支付' } as const

async function checkout(request: APIRequestContext, trade: string) {
  const response = await request.get(`/api/v1/checkout/${trade}`)
  expect(response.status()).toBe(200)
  return (await response.json() as { data: Checkout }).data
}
async function screenshots(page: Page, name: string) {
  const viewport = page.viewportSize()!
  if (viewport.width === 360 || viewport.width === 1280) {
    await page.setViewportSize({ width: viewport.width, height: viewport.width === 360 ? 640 : 720 })
  }
  for (const theme of ['light', 'dark'] as const) {
    await selectCheckoutScreenshotTheme(page, theme)
    await page.locator('img').evaluateAll(async (images) => {
      await Promise.all(images.map((image) => (image as HTMLImageElement).decode()))
      await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())))
    })
    await page.evaluate(() => document.fonts.ready)
    const dimensions = await page.evaluate(() => ({
      width: document.documentElement.scrollWidth,
      height: document.documentElement.scrollHeight,
    }))
    const frame = page.viewportSize()!
    expect(dimensions.width).toBeLessThanOrEqual(frame.width)
    expect(dimensions.height).toBeLessThanOrEqual(frame.height)
    await polishShot(page, name, true, 'checkout-shots')
    const receipt = page.getByRole('main')
    await expect(receipt).toHaveCount(1)
    await expect(receipt.getByText('示例小店')).toBeVisible()
    await expect(receipt.getByRole('button', { name: '复制订单号' })).toBeVisible()
    await expect(receipt.getByText('由 VanillaPay 提供收款服务')).toBeVisible()
  }
}

async function expectChannelLayout(page: Page) {
  const buttons = Object.values(channelNames).map((name) => page.getByRole('button', { name, exact: true }))
  for (const button of buttons) await expect(button).toBeVisible()
  const boxes = (await Promise.all(buttons.map((button) => button.boundingBox()))).map((box) => box!)
  const [first, second] = boxes
  expect(second!.width).toBeCloseTo(first!.width)
  expect(second!.x).toBeCloseTo(first!.x)
  expect(Math.abs(second!.y - (first!.y + first!.height))).toBeLessThanOrEqual(1)
  await expect(buttons[0]!).toHaveCSS('border-bottom-width', '1px')
}

test('public v1 checkout locks a channel, serves its signed QR and receives a real device payment', async ({ page, request }) => {
  const { trade, deviceToken } = await prepareCheckout(request)
  await page.goto(`/pay/${trade}`)
  await expect(page.getByRole('heading', { name: '月度会员', level: 1, exact: true })).toBeVisible()
  const initial = await checkout(request, trade)
  expect(initial.channel).toBeNull()
  expect(initial.available_channels).toEqual(Object.keys(channelNames))
  await expectChannelLayout(page)
  await screenshots(page, 'unselected')
  const copyOrder = page.getByRole('button', { name: '复制订单号' })
  const language = page.getByRole('button', { name: '当前为简体中文，切换到英语', exact: true })
  const theme = page.getByRole('button', { name: '当前外观：深色，切换到跟随系统', exact: true })
  for (const control of [language, theme]) {
    await expect(control).toHaveCSS('width', '44px')
    await expect(control).toHaveCSS('height', '44px')
  }
  // Screenshot controls leave focus in the header. Reload for a fresh keyboard visit.
  await page.reload()
  const receiptMain = page.getByRole('main')
  const firstChannel = receiptMain.getByRole('button', { name: '支付宝', exact: true })
  await expect(firstChannel).toBeVisible()
  await page.keyboard.press('Tab')
  const skip = page.getByRole('link', { name: '跳到主要内容', exact: true })
  await expect(skip).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(receiptMain).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(firstChannel).toBeFocused()
  const post = page.waitForResponse((response) => response.url().endsWith(`/checkout/${trade}/channel`) && response.request().method() === 'POST')
  await page.keyboard.press('Enter')
  const selected = await post
  expect(selected.status()).toBe(200)
  expect((await selected.request().allHeaders()).origin).toBe(new URL(page.url()).origin)
  const data = await checkout(request, trade)
  expect(data.actual_cents).toBe(2999)
  const qr = page.getByRole('img', { name: '收款二维码' })
  await expect(qr).toHaveAttribute('src', data.qr_image_url!)
  await expect(qr).toBeVisible()
  await expect(qr.locator('..')).toHaveCSS('background-color', 'rgb(255, 255, 255)')
  await expect(page.getByRole('heading', { name: '收款二维码' })).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(copyOrder).toBeFocused()
  await expect.poll(() => qr.evaluate((element) => (element as HTMLImageElement).naturalWidth)).toBe(300)
  const reminder = page.getByRole('note')
  await expect(reminder).toContainText('请按实付金额付款，金额不符将无法自动到账')
  await expect(reminder).not.toContainText('¥29.99')
  await expect(receiptMain.getByText('¥29.99', { exact: true })).toHaveCount(1)
  await expect(page.getByRole('button', { name: '微信支付', exact: true })).toHaveCount(0)
  await screenshots(page, 'locked')
  const receipt = await request.post('/api/v1/device/receipts', {
    headers: { Authorization: `Bearer ${deviceToken}` },
    data: { event_id: crypto.randomUUID(), channel: 'alipay', amount_cents: data.actual_cents, occurred_at: new Date().toISOString(), raw_text: '支付宝收款 29.99 元，月度会员' },
  })
  expect(receipt.status()).toBe(201)
  expect((await receipt.json() as { data: { match_state: string } }).data.match_state).toBe('matched')
  await expect(page.getByRole('heading', { name: '支付成功', exact: true })).toBeVisible({ timeout: 10000 })
  await expect(receiptMain.getByText('已支付', { exact: true })).toBeVisible()
  await expect(receiptMain.getByText('¥29.99', { exact: true })).toBeVisible()
  await expect(receiptMain.getByText('支付宝', { exact: true })).toBeVisible()
  await expect(page.getByText('可以关闭本页')).toBeVisible()
  await screenshots(page, 'paid')
  expect((await checkout(request, trade)).status).toBe('paid')
})

test('public checkout reveals the receipt after keyboard verification', async ({ page, request }) => {
  const { trade } = await prepareCheckout(request)
  const initial = await checkout(request, trade)
  let verified = false
  const endpoint = `**/api/v1/checkout/${trade}`
  await page.route('https://challenges.cloudflare.com/turnstile/v0/api.js*', (route) => route.fulfill({
    contentType: 'application/javascript',
    body: `window.turnstile = {
      render(node, options) {
        const button = document.createElement('button');
        button.type = 'button';
        button.textContent = '完成本地验证';
        button.onclick = () => options.callback('checkout-fixture-token');
        node.append(button);
        return 'checkout-fixture';
      },
      remove() {},
    };`,
  }))
  await page.route(endpoint, (route) => route.fulfill({ json: {
    data: verified ? initial : { ...initial, verification: { required: true, site_key: 'checkout-fixture-key' } },
  } }))
  await page.route(endpoint + '/verify', (route) => {
    expect(route.request().postDataJSON()).toEqual({ turnstile_token: 'checkout-fixture-token' })
    verified = true
    return route.fulfill({ status: 204 })
  })

  await page.goto(`/pay/${trade}`)
  const receipt = page.getByRole('main')
  const widget = receipt.getByRole('button', { name: '完成本地验证' })
  await expect(receipt.getByRole('heading', { name: '收银台人机验证', level: 1 })).toBeVisible()
  await expect(receipt.getByRole('heading', { name: '月度会员', level: 1, exact: true })).toHaveCount(0)
  await expect(receipt.getByText('示例小店')).toHaveCount(0)
  await expect(receipt.getByText(trade, { exact: true })).toHaveCount(0)
  await expect(receipt.getByText('¥30.00', { exact: true })).toHaveCount(0)
  await expect(receipt.getByRole('button', { name: '复制订单号' })).toHaveCount(0)
  await expect(receipt.getByRole('button', { name: '支付宝', exact: true })).toHaveCount(0)
  await expect(receipt.getByRole('img', { name: '收款二维码' })).toHaveCount(0)
  await widget.focus()
  await page.keyboard.press('Enter')
  await page.keyboard.press('Tab')
  await expect(receipt.getByRole('button', { name: '继续付款' })).toBeFocused()
  await page.keyboard.press('Enter')
  const channel = receipt.getByRole('button', { name: '支付宝', exact: true })
  await expect(channel).toBeVisible()
  await expect(receipt.getByRole('heading', { name: '月度会员', level: 1, exact: true })).toBeVisible()
  await expect(receipt.getByText(trade, { exact: true })).toBeVisible()
  await expect(receipt.getByText('¥30.00', { exact: true })).toBeVisible()
  await page.keyboard.press('Tab')
  await expect(channel).toBeFocused()
})

// State coverage below uses the real GET expiry transition.
for (const state of ['expired', 'offline'] as const) {
  test(`public checkout explains ${state}`, async ({ page, request }) => {
    const { trade } = await prepareCheckout(request)
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
