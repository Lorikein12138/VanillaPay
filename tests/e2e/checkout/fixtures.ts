import { createHash } from 'node:crypto'
import QRCode from 'qrcode'
import { expect, type APIRequestContext } from '@playwright/test'

type ChannelName = 'alipay' | 'wxpay'

export async function prepareCheckout(request: APIRequestContext) {
  const qrURLs = {
    alipay: 'https://qr.alipay.com/checkout-example',
    wxpay: 'wxp://checkout-example',
  } satisfies Record<ChannelName, string>
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
