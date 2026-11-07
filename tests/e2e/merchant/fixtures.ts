import { fileURLToPath } from 'node:url'
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
          button.onclick = () => options.callback(${JSON.stringify(account.turnstileToken)} + ':' + options.action + ':' + crypto.randomUUID());
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

/** Open the mobile navigation only when a requested control is inside it. */
export async function openMenu(page: Page) {
  const menu = page.getByRole('button', { name: /打开菜单|Open menu/ })
  if (page.viewportSize()!.width < 1024) {
    await expect(page).toHaveURL(/\/(dashboard|orders|codes|devices|integration|account|banned|subscription|notifications)([/?#]|$)/)
    await expect(menu).toBeVisible()
    if (!(await page.getByRole('dialog').isVisible())) await menu.click()
  }
}
export async function closeMenu(page: Page) {
  const drawer = page.getByRole('dialog', { name: /商户导航|Merchant navigation/ })
  if (await drawer.isVisible()) await page.keyboard.press('Escape')
}
export async function clickControl(page: Page, name: string) {
  const control = page.getByRole('button', { name, exact: true })
  if (page.viewportSize()!.width < 1024) {
    const menu = page.getByRole('button', { name: /打开菜单|Open menu/ })
    await expect(control.or(menu).filter({ visible: true }).first()).toBeVisible()
    if (!(await control.isVisible())) await openMenu(page)
  }
  await control.click()
  await closeMenu(page)
}
export async function polishShot(
  page: Page,
  name: string,
  fullPage = !name.includes('dialog') && name !== 'drawer',
  directory = 'web-ui-polish-shots',
  passwords: 'mask' | 'assert-empty' = 'mask',
) {
  const width = page.viewportSize()?.width
  if (width !== 360 && width !== 1280) return
  await page.evaluate(() => document.fonts.ready)
  await page.waitForLoadState('networkidle')
  await noOverflow(page)
  expect(await page.getByRole('status').count()).toBeLessThanOrEqual(1)
  expect(await page.getByRole('alert').count()).toBeLessThanOrEqual(1)
  const theme = await page.locator('html').getAttribute('data-theme')
  expect(theme).toMatch(/^(light|dark)$/)
  const passwordFields = page.locator('input[type=password]')
  if (passwords === 'assert-empty') {
    for (const field of await passwordFields.all()) await expect(field).toHaveValue('')
  }
  await page.screenshot({
    path: fileURLToPath(new URL('../../../.artifacts/'+directory+'/'+name+'-'+theme+'-'+width+'.png', import.meta.url)),
    fullPage,
    animations: 'disabled',
    mask: [
      page.getByTestId('one-time-key'),
      page.getByTestId('binding-code'),
      page.getByTestId('totp-secret'),
      page.getByTestId('totp-qr'),
      page.getByTestId('recovery-codes'),
      page.getByRole('img', { name: '设备绑定二维码' }),
      ...(passwords === 'mask' ? [passwordFields] : []),
    ],
    maskColor: '#73726c',
  })
}
