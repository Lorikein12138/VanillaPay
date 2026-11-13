import { generateKeyPairSync } from 'node:crypto'
import { fileURLToPath } from 'node:url'
import QRCode from 'qrcode'
import type { APIRequestContext, Page } from '@playwright/test'
import type { components } from '../../../web/src/api/schema'
import { chooseSelect, expect, login, merchantVisualShots, noOverflow, test, clickControl, polishShot } from './fixtures'

async function shot(page: Page, name: string) {
  const width = page.viewportSize()?.width
  if (width !== 360 && width !== 1280) return
  const theme = width === 360 ? 'dark' : 'light'
  await page.evaluate(() => document.fonts.ready)
  await noOverflow(page)
  await polishShot(page, name === 'binding' || name === 'one-time-key' || name === 'totp-setup' || name === 'recovery-codes' ? name+'-dialog' : name)
  await page.screenshot({ path: fileURLToPath(new URL(`../../../.artifacts/merchant-devices-shots/${name}-${theme}.png`, import.meta.url)), animations: 'disabled', fullPage: name === 'codes', mask: [page.getByTestId('one-time-key'), page.getByTestId('binding-code')], maskColor: '#73726c' })
}
async function signIn(page: Page, account: Parameters<typeof login>[1], path: string) {
  await page.goto(`/login?redirect=${encodeURIComponent(path)}`); await login(page, account)
  await expect(page).toHaveURL(new RegExp(`${path}$`))
  await clickControl(page, page.viewportSize()?.width === 360 ? '深色' : '浅色')
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
  const slot = page.getByRole('region', { name: 'alipay收款码' })
  const png = await QRCode.toBuffer(`https://qr.alipay.com/${crypto.randomUUID()}`, { width: 300 })
  for (let attempt = 0; attempt < 2; attempt++) {
    await slot.locator('input[type=file]').setInputFiles({ name: 'collection.png', mimeType: 'image/png', buffer: png })
    await polishShot(page, 'upload-code-dialog', false)
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
  await polishShot(page, 'toggle-code-dialog', false)
  await page.getByRole('button', { name: '确认保存', exact: true }).click()
  await expect(slot.getByText('已停用', { exact: true })).toBeVisible()
  await slot.getByRole('button', { name: '启用', exact: true }).click()
  await page.getByRole('button', { name: '确认保存', exact: true }).click()
  await expect(slot.getByText('已启用', { exact: true })).toBeVisible()
  await page.getByLabel('订单有效期（秒）', { exact: true }).fill('120')
  await page.getByLabel('浮动上限（元）', { exact: true }).fill('0.29')
  await chooseSelect(page, page.getByLabel('浮动方向', { exact: true }), 'down')
  await page.getByRole('button', { name: '保存收款设置' }).click()
  await polishShot(page, 'collection-settings-dialog', false)
  await page.getByRole('button', { name: '确认保存', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: '收款设置已保存' })).toBeVisible()
  const effective = page.getByRole('heading', { name: '收款设置', exact: true }).locator('..').locator('..').locator('..').locator('dl')
  await expect(effective).toContainText('120 秒'); await expect(effective).toContainText('0.29'); await expect(effective).toContainText('向下浮动')

  const identity = await page.request.get('/api/v1/merchant/me')
  const { data: { csrf_token } } = await identity.json() as { data: components['schemas']['SessionIdentity'] }
  const collection = await page.request.get('/api/v1/merchant/collection')
  const { data: { settings } } = await collection.json() as { data: components['schemas']['Collection'] }
  const updated = await page.request.put('/api/v1/merchant/collection/settings', {
    headers: { Origin: new URL(page.url()).origin, 'X-CSRF-Token': csrf_token },
    data: { expected_version: settings.version, order_ttl_seconds: 180, float_limit_cents: 29, float_direction: 'down' },
  })
  expect(updated.status()).toBe(200)
  await page.getByLabel('订单有效期（秒）', { exact: true }).fill('240')
  await page.getByRole('button', { name: '保存收款设置' }).click()
  await page.getByRole('button', { name: '确认保存', exact: true }).click()
  const conflict = page.getByText('设置已被其他会话修改，已载入最新值，请确认后再保存。')
  await expect(conflict).toBeVisible()
  await expect(page.getByLabel('订单有效期（秒）', { exact: true })).toHaveValue('180')
  await polishShot(page, 'collection-version-conflict', true, 'contract-backend-fixes-shots')
})

test('redeems a real binding code, reports receipts, opens a matched order and revokes the device', async ({ page, account, request }) => {
  await signIn(page, account, '/devices')
  await polishShot(page, 'devices')
  await page.getByRole('button', { name: '绑定设备', exact: true }).click()
  await polishShot(page, 'binding-create-dialog', false)
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
  await expect(page.getByRole('link', { name: trade, exact: true })).toBeVisible()
  await merchantVisualShots(page, async () => {
    await polishShot(page, 'receipt-filters', true, 'web-visual-polish-shots')
  })
  await page.getByRole('button', { name: receipt.data.receipt_id, exact: true }).click()
  await expect(page.getByRole('dialog').locator('pre')).toHaveText(raw)
  await polishShot(page, 'receipt-detail-dialog', false)
  await expect(page.getByRole('dialog').locator('img')).toHaveCount(0)
  await page.getByRole('dialog').getByRole('link', { name: trade, exact: true }).click()
  await expect(page).toHaveURL(new RegExp(`/orders/${trade}$`))
  await expect(page.getByText('设备收款测试订单', { exact: true })).toBeVisible()
  await page.goto('/devices')
  await page.getByRole('button', { name: '解绑设备', exact: true }).click()
  await expect(page.getByRole('button', { name: '确认解绑' })).toBeDisabled()
  await polishShot(page, 'unbind-dialog', false)
  await page.getByRole('checkbox', { name: '我确认解绑当前设备' }).check()
  await page.getByRole('button', { name: '确认解绑' }).click()
  await expect(page.getByText('未绑定', { exact: true })).toBeVisible()
  expect((await request.post('/api/v1/device/heartbeat', { headers: deviceHeaders, data: { app_version: 'e2e-1' } })).status()).toBe(401)
  await noOverflow(page)
})

test('links to the published Android installer and serves it as an attachment', async ({ page, account, request }) => {
  await page.goto('/login?redirect=%2Fdevices')
  await login(page, account)
  await expect(page).toHaveURL(/\/devices$/)
  const path = '/downloads/vanillapay-monitor-1.0.0.apk'
  await expect(page.getByRole('link', { name: '下载 VanillaPay Monitor 1.0.0' })).toHaveAttribute('href', path)
  const apk = await request.head(path)
  expect(apk.ok()).toBe(true)
  expect(apk.headers()['content-type']).toBe('application/vnd.android.package-archive')
  expect(apk.headers()['content-disposition']).toBe('attachment; filename="vanillapay-monitor-1.0.0.apk"')
  expect(apk.headers()['x-content-type-options']).toBe('nosniff')
  await merchantVisualShots(page, async () => { await polishShot(page, 'devices-download', true, 'android-monitor-shots') })
})

test('obtains a security email code, resets the key once and saves an IP whitelist', async ({ page, account, request }) => {
  await signIn(page, account, '/integration')
  for (const path of ['/api.php?act=query', '/api.php?act=orders', '/api/merchant/info', '/api/merchant/orders', '/api/pay/close', '/api/pay/refundquery']) {
    await expect(page.getByText(new URL(path, page.url()).href, { exact: true })).toBeVisible()
  }
  await merchantVisualShots(page, async () => { await polishShot(page, 'integration', true, 'payment-protocol-compat-shots') })
  await page.getByRole('button', { name: '重置 Key', exact: true }).click()
  await polishShot(page, 'reset-key-dialog', false)
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
  await polishShot(page, 'whitelist-dialog', false)
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
  await polishShot(page, 'rsa-dialog', false)
  await mailbox(page, request, account.username)
  const saved = page.waitForResponse((response) => response.url().endsWith('/integration/rsa-public-key') && response.request().method() === 'PUT')
  await page.getByRole('button', { name: '确认保存公钥' }).click()
  expect((await saved).status()).toBe(200)
  await expect(page.getByRole('status')).toContainText('RSA 公钥已保存')
  await page.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(page.getByText(pair.publicKey.trim(), { exact: true })).toContainText('BEGIN PUBLIC KEY')
  await noOverflow(page)
})
