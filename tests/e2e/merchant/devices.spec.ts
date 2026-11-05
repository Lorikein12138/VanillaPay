import { generateKeyPairSync } from 'node:crypto'
import { fileURLToPath } from 'node:url'
import QRCode from 'qrcode'
import type { APIRequestContext, Page } from '@playwright/test'
import { expect, login, noOverflow, test } from './fixtures'

async function shot(page: Page, name: string) {
  const width = page.viewportSize()?.width
  if (width !== 360 && width !== 1280) return
  const theme = width === 360 ? 'dark' : 'light'
  await page.evaluate(() => document.fonts.ready)
  await noOverflow(page)
  await page.screenshot({ path: fileURLToPath(new URL(`../../../.artifacts/merchant-devices-shots/${name}-${theme}.png`, import.meta.url)), animations: 'disabled', fullPage: name === 'codes', mask: [page.getByTestId('one-time-key'), page.getByTestId('binding-code')], maskColor: '#73726c' })
}
async function signIn(page: Page, account: Parameters<typeof login>[1], path: string) {
  await page.goto(`/login?redirect=${encodeURIComponent(path)}`); await login(page, account)
  await expect(page).toHaveURL(new RegExp(`${path}$`))
  await page.getByRole('button', { name: page.viewportSize()?.width === 360 ? '深色' : '浅色', exact: true }).click()
}
async function mailbox(page: Page, request: APIRequestContext, username: string) {
  const sending = page.waitForResponse((response) => response.url().endsWith('/auth/email-codes'))
  await page.getByRole('button', { name: '获取邮箱验证码' }).click()
  const response = await sending
  expect(response.status()).toBe(200)
  const headers = await response.request().allHeaders()
  expect(headers['x-csrf-token']).toBeTruthy()
  expect(headers.origin).toBe(new URL(page.url()).origin)
  const message = await (await request.post('/__e2e/mail', { data: { email: `${username}@example.test` } })).json() as { code: string }
  // Avoid echoing any actual verification code on failure.
  expect(/^\d{6}$/.test(message.code)).toBe(true)
  await page.getByLabel('邮箱验证码', { exact: true }).fill(message.code)
}

test('uploads and replaces collection codes, toggles availability and saves bounded collection settings', async ({ page, account }) => {
  await signIn(page, account, '/codes')
  const slot = page.getByRole('article', { name: 'alipay收款码' })
  const png = await QRCode.toBuffer(`https://qr.alipay.com/${crypto.randomUUID()}`, { width: 300 })
  for (let attempt = 0; attempt < 2; attempt++) {
    await slot.locator('input[type=file]').setInputFiles({ name: 'collection.png', mimeType: 'image/png', buffer: png })
    const uploaded = page.waitForResponse((response) => response.url().endsWith('/collection/codes/alipay') && response.request().method() === 'PUT')
    await page.getByRole('button', { name: '确认保存', exact: true }).click()
    const response = await uploaded
    expect(response.status()).toBe(201)
    const headers = await response.request().allHeaders()
    expect(headers.origin).toBe(new URL(page.url()).origin)
    expect(headers['x-csrf-token']).toBeTruthy()
    expect(headers['content-type']).toContain('multipart/form-data; boundary=')
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await expect(slot.getByRole('img')).toBeVisible()
  }
  await shot(page, 'codes')
  await slot.getByRole('button', { name: '停用', exact: true }).click()
  await page.getByRole('button', { name: '确认保存', exact: true }).click()
  await expect(slot.getByText('已停用', { exact: true })).toBeVisible()
  await slot.getByRole('button', { name: '启用', exact: true }).click()
  await page.getByRole('button', { name: '确认保存', exact: true }).click()
  await expect(slot.getByText('已启用', { exact: true })).toBeVisible()
  await page.getByLabel('订单有效期（秒）', { exact: true }).fill('120')
  await page.getByLabel('浮动上限（元）', { exact: true }).fill('0.29')
  await page.getByLabel('浮动方向', { exact: true }).selectOption('down')
  await page.getByRole('button', { name: '保存收款设置' }).click()
  await page.getByRole('button', { name: '确认保存', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: '收款设置已保存' })).toBeVisible()
  await expect(page.getByText(/当前生效：有效期 120 秒，浮动上限 0.29 元，向下浮动/)).toBeVisible()
})

test('redeems a real binding code, reports receipts, opens a matched order and revokes the device', async ({ page, account, request }) => {
  await signIn(page, account, '/devices')
  await page.getByRole('button', { name: '绑定设备', exact: true }).click()
  const issued = page.waitForResponse((response) => response.url().endsWith('/device/bindings') && response.request().method() === 'POST')
  await page.getByRole('button', { name: '生成绑定码' }).click()
  const issuedResponse = await issued
  expect(issuedResponse.status()).toBe(201)
  const value = await issuedResponse.json() as { data: { code: string; binding_id: string; qr_content: string } }
  expect(value.data.qr_content.startsWith('vanillapay://bind?')).toBe(true)
  await expect(page.getByRole('img', { name: '设备绑定二维码' })).toBeVisible()
  await shot(page, 'binding')
  const redeem = await request.post('/api/v1/device/bind', { data: { code: value.data.code, device_name: '收款监控测试端', app_version: 'e2e-1' } })
  expect(redeem.status()).toBe(201)
  const credential = await redeem.json() as { data: { device_token: string } }
  const deviceHeaders = { Authorization: `Bearer ${credential.data.device_token}` }
  expect((await request.post('/api/v1/device/heartbeat', { headers: deviceHeaders, data: { app_version: 'e2e-1' } })).status()).toBe(200)
  await expect(page.getByRole('status')).toContainText('设备已绑定')
  await page.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(page.getByText('在线 · 已绑定')).toBeVisible()
  const trade = `VP-${crypto.randomUUID()}`
  expect((await request.post('/__e2e/orders', { data: { pid: account.pid, orders: [{ trade_no: trade, name: '设备收款测试订单', status: 'pending', channel: 'alipay', original_cents: 12345, actual_cents: 12345, created_at: new Date(Date.now() - 2000).toISOString() }] } })).status()).toBe(204)
  const raw = '<img src=x onerror="window.receiptXSS=true"> 原始收款通知'
  const receiptResponse = await request.post('/api/v1/device/receipts', { headers: deviceHeaders, data: { event_id: crypto.randomUUID(), channel: 'alipay', amount_cents: 12345, occurred_at: new Date().toISOString(), raw_text: raw } })
  expect(receiptResponse.status()).toBe(201)
  const receipt = await receiptResponse.json() as { data: { receipt_id: string; match_state: string } }
  expect(receipt.data.match_state).toBe('matched')
  await page.reload()
  await page.getByRole('button', { name: receipt.data.receipt_id, exact: true }).click()
  await expect(page.getByRole('dialog').locator('pre')).toHaveText(raw)
  await expect(page.getByRole('dialog').locator('img')).toHaveCount(0)
  await page.getByRole('dialog').getByRole('link', { name: trade, exact: true }).click()
  await expect(page).toHaveURL(new RegExp(`/orders/${trade}$`))
  await expect(page.getByText('设备收款测试订单', { exact: true })).toBeVisible()
  await page.goto('/devices')
  await page.getByRole('button', { name: '解绑设备', exact: true }).click()
  await expect(page.getByRole('button', { name: '确认解绑' })).toBeDisabled()
  await page.getByRole('checkbox', { name: '我确认解绑当前设备' }).check()
  await page.getByRole('button', { name: '确认解绑' }).click()
  await expect(page.getByText('未绑定', { exact: true })).toBeVisible()
  expect((await request.post('/api/v1/device/heartbeat', { headers: deviceHeaders, data: { app_version: 'e2e-1' } })).status()).toBe(401)
  await noOverflow(page)
})

test('obtains a security email code, resets the key once and saves an IP whitelist', async ({ page, account, request }) => {
  await signIn(page, account, '/integration')
  await page.getByRole('button', { name: '重置 Key', exact: true }).click()
  await mailbox(page, request, account.username)
  const reset = page.waitForResponse((response) => response.url().endsWith('/integration/reset-key'))
  await page.getByRole('button', { name: '确认重置 Key' }).click()
  expect((await reset).status()).toBe(200)
  await expect(page.getByTestId('one-time-key')).toBeVisible()
  await shot(page, 'one-time-key')
  await page.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(page.getByTestId('one-time-key')).toHaveCount(0)
  await expect(page.getByText('2', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '重置 Key', exact: true }).click()
  await expect(page.getByTestId('one-time-key')).toHaveCount(0)
  await page.getByRole('button', { name: '关闭', exact: true }).click()
  await page.getByRole('checkbox', { name: '启用 IP 白名单' }).check()
  await page.getByLabel('允许的 IP 或 CIDR').fill('192.0.2.1\n2001:db8::/32')
  await page.getByRole('button', { name: '保存白名单', exact: true }).click()
  const saved = page.waitForResponse((response) => response.url().endsWith('/integration/ip-whitelist') && response.request().method() === 'PUT')
  await page.getByRole('button', { name: '确认保存白名单' }).click()
  expect((await saved).status()).toBe(200)
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.reload()
  await expect(page.getByRole('checkbox', { name: '启用 IP 白名单' })).toBeChecked()
  await expect(page.getByLabel('允许的 IP 或 CIDR')).toHaveValue('192.0.2.1/32\n2001:db8::/32')
})

test('saves an RSA public key using a real email security proof', async ({ page, account, request }) => {
  const pair = generateKeyPairSync('rsa', { modulusLength: 2048, publicKeyEncoding: { type: 'spki', format: 'pem' }, privateKeyEncoding: { type: 'pkcs8', format: 'pem' } })
  await signIn(page, account, '/integration')
  await page.getByRole('button', { name: '保存商户公钥', exact: true }).click()
  await page.getByLabel('RSA 公钥 PEM', { exact: true }).fill(pair.publicKey)
  await mailbox(page, request, account.username)
  const saved = page.waitForResponse((response) => response.url().endsWith('/integration/rsa-public-key') && response.request().method() === 'PUT')
  await page.getByRole('button', { name: '确认保存公钥' }).click()
  expect((await saved).status()).toBe(200)
  await expect(page.getByRole('status')).toContainText('RSA 公钥已保存')
  await page.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(page.locator('pre').first()).toContainText('BEGIN PUBLIC KEY')
  await noOverflow(page)
})
