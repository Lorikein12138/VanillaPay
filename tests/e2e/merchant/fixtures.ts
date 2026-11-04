import { createHmac } from 'node:crypto'
import { expect, test as base, type Page } from '@playwright/test'

type Account = { username: string; password: string; pid: string; totpSeed: string; turnstileToken: string }
export const test = base.extend<{ account: Account; twoFactor: boolean; banned: boolean }>({
  twoFactor: [false, { option: true }],
  banned: [false, { option: true }],
  account: async ({ request, page, twoFactor, banned }, runTest) => {
    const response = await request.post('/__e2e/merchant', { data: { totp: twoFactor, banned } })
    expect(response.ok(), 'create isolated, runtime-random merchant fixture').toBe(true)
    const account = await response.json() as Account
    await page.route('https://challenges.cloudflare.com/turnstile/v0/api.js*', (route) => route.fulfill({
      contentType: 'application/javascript',
      body: `window.turnstile = {
        render(node, options) {
          const id = 'widget-' + crypto.randomUUID(); node.dataset.widgetId = id;
          const button = document.createElement('button'); button.type = 'button';
          button.textContent = '完成验证'; button.style.minHeight = '44px';
          button.dataset.action = options.action;
          button.onclick = () => options.callback(${JSON.stringify(account.turnstileToken)});
          node.append(button); return id;
        },
        remove(id) { const node = document.querySelector('[data-widget-id="' + id + '"]'); if (node) node.replaceChildren(); }
      };`,
    }))
    await runTest(account)
  },
})
export { expect }

export async function login(page: Page, account: Account) {
  await page.getByLabel('用户名 / 邮箱').fill(account.username)
  await page.getByLabel('密码', { exact: true }).fill(account.password)
  const verify = page.getByRole('button', { name: '完成验证' })
  await expect(verify).toHaveAttribute('data-action', 'login')
  await verify.click()
  await page.getByRole('button', { name: '登录', exact: true }).click()
}

// Compute RFC 6238 SHA-1 TOTP from the runtime seed; no credentials in source.
export function totp(seed: string, time = Date.now()) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const char of seed.replace(/=+$/, '')) bits += alphabet.indexOf(char).toString(2).padStart(5, '0')
  const bytes: number[] = []
  for (let offset = 0; offset + 8 <= bits.length; offset += 8) bytes.push(Number.parseInt(bits.slice(offset, offset + 8), 2))
  const counter = Buffer.alloc(8)
  counter.writeBigUInt64BE(BigInt(Math.floor(time / 30000)))
  const digest = createHmac('sha1', Buffer.from(bytes)).update(counter).digest()
  const offset = (digest[digest.length - 1] ?? 0) & 15
  return ((digest.readUInt32BE(offset) & 0x7fffffff) % 1000000).toString().padStart(6, '0')
}

export async function noOverflow(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0)
}
