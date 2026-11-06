import { expect, test } from '@playwright/test'
import type { components } from '../../../web/src/api/schema'

type Checkout = components['schemas']['PaymentCheckoutDTO']

test('a hanging merchant return receives only one document navigation after a signed URL refresh', async ({ page }) => {
  const endpoint = '**/api/v1/checkout/return-once'
  const first = 'https://shop.example/return?timestamp=1&sign=one%2Bvalue'
  const latest = 'https://shop.example/return?timestamp=2&sign=two%2Bvalue'
  let paid = false, refreshed = false, postStarted = false, gets = 0
  let releasePost!: () => void
  const slowPost = new Promise<void>((resolve) => { releasePost = resolve })
  const releaseReturns: (() => void)[] = []
  const navigations: string[] = []
  const initial: Checkout = {
    trade_no: 'return-once', name: '月度会员', product_id: '', merchant: { username: '示例小店', id: '1' },
    status: 'pending', reason: 'no_channel', original_cents: 3000, actual_cents: null, channel: null,
    available_channels: ['alipay', 'wxpay'], qr_image_url: null, expires_at: new Date(Date.now() + 300000).toISOString(),
    server_time: new Date().toISOString(), device_online: true, can_pay: false, return_url_signed: null, requires_exact_amount: true,
  }
  page.on('request', (request) => {
    if (request.isNavigationRequest() && request.resourceType() === 'document' && request.url().startsWith('https://shop.example/return?')) navigations.push(request.url())
  })
  await page.route('https://shop.example/return?**', async (route) => {
    // Keep the original checkout document alive while merchant navigation hangs.
    await new Promise<void>((resolve) => { releaseReturns.push(resolve) })
    await route.abort().catch(() => {})
  })
  await page.route(endpoint, (route) => {
    gets++
    return route.fulfill({ json: { data: paid ? { ...initial, status: 'paid', reason: null, channel: 'alipay', actual_cents: 2999, return_url_signed: refreshed ? latest : first } : initial } })
  })
  await page.route(endpoint + '/channel', async (route) => {
    postStarted = true
    await slowPost
    refreshed = true
    await route.fulfill({ json: { data: initial } })
  })
  try {
    await page.goto('/pay/return-once')
    await page.getByRole('button', { name: '支付宝', exact: true }).click()
    await expect.poll(() => postStarted).toBe(true)
    paid = true
    await expect(page.getByRole('heading', { name: '支付成功' })).toBeVisible()
    await page.getByRole('button', { name: '返回商户', exact: true }).click({ noWaitAfter: true })
    await expect.poll(() => navigations.length).toBe(1)
    const fresh = page.waitForResponse((response) => response.url().endsWith('/api/v1/checkout/return-once') && response.request().method() === 'GET')
    releasePost()
    const response = await fresh
    // Chromium can defer response.body() while a document navigation is pending.
    // The route above supplies the new signature; wait for its GET response headers.
    expect(response.status()).toBe(200)
    expect(gets).toBe(3)
    // Cross the original and any incorrectly restarted three-second deadlines.
    await page.waitForTimeout(3500)
    expect(navigations).toEqual([first])
  } finally {
    releasePost()
    for (const release of releaseReturns) release()
    await page.unrouteAll({ behavior: 'wait' })
  }
})
