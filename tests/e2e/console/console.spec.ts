import { registerManagementTests } from './manage-cases'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
import type { ConsoleRecovery } from '../../../web/src/api/client'
import { login, noOverflow, polishShot, totp } from '../merchant/fixtures'

test.describe.configure({ mode: 'serial' })
type Admin = { username: string; password: string; totpSeed: string; turnstileToken: string }

async function seed(request: APIRequestContext, options: { reset?: boolean; totp?: boolean } = {}) {
  const response = await request.post('/__e2e/console', { data: options })
  expect(response.status()).toBe(200)
  return response.json() as Promise<Admin & { initialization_code: string }>
}
async function captcha(page: Page, token: string) {
  await page.route('https://challenges.cloudflare.com/turnstile/v0/api.js*', (route) => route.fulfill({ contentType: 'application/javascript', body: `window.turnstile={render(node,options){const id=crypto.randomUUID();node.dataset.widgetId=id;const button=document.createElement('button');button.type='button';button.textContent='完成验证';button.dataset.action=options.action;button.style.minHeight='44px';button.onclick=()=>options.callback(${JSON.stringify(token)}+':'+options.action+':'+crypto.randomUUID());node.append(button);return id},remove(id){document.querySelector('[data-widget-id="'+id+'"]')?.replaceChildren()}};` }))
}
async function signIn(page: Page, admin: Admin, redirect = '/console') {
  await page.goto('/console/login?redirect=' + encodeURIComponent(redirect))
  await login(page, { ...admin, pid: '' })
}
async function control(page: Page, name: string) {
  const target = page.getByRole('button', { name, exact: true })
  await expect(target.or(page.getByRole('button', { name: '打开菜单', exact: true })).filter({ visible: true }).first()).toBeVisible()
  if (!(await target.isVisible())) await page.getByRole('button', { name: '打开菜单', exact: true }).click()
  await target.click()
  const drawer = page.getByRole('dialog', { name: '管理控制台', exact: true })
  if (await drawer.isVisible()) await page.keyboard.press('Escape')
}
async function shots(page: Page, state: string, authenticated = false, drawer = false) {
  for (const width of drawer ? [360] : [360, 1280]) {
    await page.setViewportSize({ width, height: width === 360 ? 780 : 800 })
    for (const [theme, label] of [['light', '浅色'], ['dark', '深色']] as const) {
      const saved = authenticated ? page.waitForResponse((response) => response.url().endsWith('/console/preferences') && response.request().method() === 'PATCH') : null
      await control(page, label)
      if (saved) expect((await saved).status()).toBe(200)
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      if (drawer) await page.getByRole('button', { name: '打开菜单', exact: true }).click()
      if (authenticated && width === 360 && !drawer) {
        await page.getByRole('button', { name: '打开菜单', exact: true }).click()
        await expect(page.getByRole('button', { name: label, exact: true })).toHaveAttribute('aria-pressed', 'true')
        await page.keyboard.press('Escape')
      } else await expect(page.getByRole('button', { name: label, exact: true })).toHaveAttribute('aria-pressed', 'true')
      await polishShot(page, state, !drawer, 'console-auth-shots')
      if (drawer) await page.keyboard.press('Escape')
    }
  }
}

test('initializes once, enters the shared overview and captures the responsive shell', async ({ page, request }) => {
  const fixture = await seed(request, { reset: true })
  await page.goto('/console/login')
  await expect(page).toHaveURL(/\/console\/setup$/)
  await shots(page, 'setup')
  const password = crypto.randomUUID()
  await page.getByLabel('初始化码').fill(fixture.initialization_code)
  await page.getByLabel('用户名', { exact: true }).fill('console_admin')
  await page.getByLabel('密码', { exact: true }).fill(password)
  await page.getByLabel('确认新密码', { exact: true }).fill(password)
  const created = page.waitForResponse((response) => response.url().endsWith('/console/bootstrap'))
  await page.getByRole('button', { name: '创建管理员' }).click()
  expect((await created).status()).toBe(201)
  await expect(page).toHaveURL(/\/console$/)
  await expect(page.getByRole('heading', { name: /console_admin/ })).toBeVisible()
  await shots(page, 'overview', true)
  // Visually hidden chart data must not stretch the page below the shell (sidebar background would stop short).
  expect(await page.evaluate(() => document.documentElement.scrollHeight - Math.ceil(Math.max(window.innerHeight, document.getElementById('main')!.getBoundingClientRect().bottom + window.scrollY)))).toBeLessThanOrEqual(0)
  await shots(page, 'drawer', true, true)
  await page.setViewportSize({ width: 1280, height: 800 })
  await expect(page.getByRole('navigation', { name: '管理控制台' }).getByRole('link')).toHaveText(['概览', '商户', '订单', '套餐', '购买记录'])
  await control(page, '退出登录')
  await expect(page).toHaveURL(/\/console\/login$/)
  await page.goto('/console/setup')
  await expect(page).toHaveURL(/\/console\/login\?notice=initialized$/)
})

test('shares login, preserves console redirect, isolates role guards, and offers recovery when unconfigured', async ({ page, request }) => {
  const admin = await seed(request); await captcha(page, admin.turnstileToken)
  await page.goto('/console/not-implemented?state=paid')
  await expect(page).toHaveURL(/\/console\/login\?redirect=/)
  await shots(page, 'login')
  await page.route('**/api/v1/public/config', async (route) => {
    const response = await route.fetch()
    const body = await response.json() as { data: { turnstile: { configured: boolean } } }
    body.data.turnstile.configured = false
    await route.fulfill({ response, json: body })
  })
  await page.reload()
  await expect(page.getByRole('button', { name: '登录', exact: true })).toBeDisabled()
  await expect(page.getByRole('link', { name: '恢复管理端访问' })).toBeVisible()
  await shots(page, 'turnstile-unconfigured')
  await page.unroute('**/api/v1/public/config'); await page.reload()
  await login(page, { ...admin, pid: '' })
  await expect(page).toHaveURL(/\/console\/not-implemented\?state=paid$/)
  await expect(page.getByText('页面不存在', { exact: true })).toBeVisible()
  await expect(page.getByRole('navigation', { name: '管理控制台' })).toBeVisible()
  await page.goto('/dashboard')
  await expect(page).toHaveURL(/\/login\?redirect=/)
  await page.goto('/console'); await expect(page).toHaveURL(/\/console$/)
  await control(page, '退出登录'); await expect(page).toHaveURL(/\/console\/login$/)
  const merchantResponse = await request.post('/__e2e/merchant', { data: {} })
  expect(merchantResponse.status()).toBe(200)
  const merchant = await merchantResponse.json() as Admin & { pid: string }
  await page.goto('/login'); await login(page, merchant); await expect(page).toHaveURL(/\/dashboard$/)
  await page.goto('/console'); await expect(page).toHaveURL(/\/console\/login\?redirect=/)
  await expect(page.getByRole('heading', { name: '管理员登录' })).toBeVisible()
  await noOverflow(page)
})

test('signs in with console TOTP using the shared second-factor form', async ({ page, request }) => {
  const admin = await seed(request, { totp: true }); await captcha(page, admin.turnstileToken)
  await signIn(page, admin)
  await expect(page.getByRole('heading', { name: '二次验证' })).toBeVisible()
  await shots(page, 'second-factor')
  await page.getByLabel('动态验证码', { exact: true }).fill(totp(admin.totpSeed))
  await page.getByRole('button', { name: '验证并登录' }).click()
  await expect(page).toHaveURL(/\/console$/)
})

test('repairs only the existing security values and consumes both the terminal code and repair session', async ({ page, request }) => {
  await seed(request)
  const issued = await request.post('/__e2e/console/recovery', { data: {} })
  expect(issued.status()).toBe(200)
  const { recovery_code: code } = await issued.json() as { recovery_code: string }
  await page.goto('/console/recovery'); await shots(page, 'recovery-code')
  const recovered = page.waitForResponse((response) => response.url().endsWith('/console/recovery'))
  await page.getByLabel('终端恢复码').fill(code)
  await page.getByRole('button', { name: '验证恢复码' }).click()
  const recovery = (await (await recovered).json() as { data: ConsoleRecovery }).data
  await expect(page.getByLabel('Turnstile Site Key')).toHaveValue(recovery.security.turnstile_site_key)
  await expect(page.getByLabel('允许的主机名')).toHaveValue(recovery.security.allowed_hostname)
  await expect(page.getByLabel('Turnstile Secret')).toHaveValue('')
  await shots(page, 'recovery-settings')
  const repair = page.waitForResponse((response) => response.url().endsWith('/console/settings/security'))
  await page.getByRole('button', { name: '保存并结束修复' }).click()
  const saved = await repair
  expect(saved.status()).toBe(200)
  const body = saved.request().postDataJSON() as { value: Record<string, string> }
  expect(Object.keys(body.value).sort()).toEqual(['allowed_hostname', 'turnstile_site_key'])
  await expect(page).toHaveURL(/\/console\/login\?notice=repaired$/)
  await expect(page.getByText('已修复，请登录。')).toBeVisible()
  const origin = new URL(page.url()).origin
  const replay = await request.post('/api/v1/console/recovery', { headers: { Origin: origin }, data: { recovery_code: code } })
  expect(replay.status()).toBe(401)
  const sessionReplay = await request.put('/api/v1/console/settings/security', { headers: { Origin: origin, 'X-CSRF-Token': recovery.csrf_token }, data: { expected_version: recovery.settings_version + 1, value: body.value } })
  expect(sessionReplay.status()).toBe(401)
})

test('changes password, revokes the session, persists preferences and enables/disables 2FA', async ({ page, request }) => {
  const admin = await seed(request); await captcha(page, admin.turnstileToken)
  await signIn(page, admin, '/console/account')
  await expect(page.getByRole('heading', { name: '管理员账户', exact: true })).toBeVisible()
  await shots(page, 'account', true)
  await page.getByLabel('账户语言').selectOption('en')
  await expect(page.getByRole('heading', { name: 'Administrator account', exact: true })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Administrator account', exact: true })).toBeVisible()
  await page.getByLabel('Account language').selectOption('zh-CN')
  await page.getByRole('button', { name: '修改密码', exact: true }).click()
  const nextPassword = crypto.randomUUID()
  await page.getByLabel('当前密码', { exact: true }).fill(admin.password)
  await page.getByLabel('新密码', { exact: true }).fill(nextPassword)
  await page.getByLabel('确认新密码', { exact: true }).fill(nextPassword)
  await page.getByRole('button', { name: '确认保存', exact: true }).click()
  await expect(page).toHaveURL(/\/console\/login\?notice=password_changed$/)
  await signIn(page, { ...admin, password: nextPassword }, '/console/account')
  await page.getByRole('button', { name: '开启两步验证', exact: true }).click()
  await page.getByLabel('当前密码', { exact: true }).fill(nextPassword)
  await page.getByRole('button', { name: '验证密码并生成' }).click()
  const secret = (await page.getByTestId('totp-secret').textContent())!
  await expect(page.getByTestId('totp-qr')).toBeVisible()
  const step = Math.floor(Date.now() / 30000)
  await page.getByLabel('动态验证码', { exact: true }).fill(totp(secret, step * 30000))
  await page.getByRole('button', { name: '确认开启', exact: true }).click()
  await expect(page.getByTestId('recovery-codes')).toBeVisible()
  const download = page.waitForEvent('download')
  await page.getByRole('button', { name: '下载恢复码 TXT' }).click()
  expect((await download).suggestedFilename()).toBe('vanillapay-recovery-codes.txt')
  await page.getByRole('button', { name: '关闭', exact: true }).click()
  await expect(page.getByTestId('recovery-codes')).toHaveCount(0)
  await page.getByRole('button', { name: '关闭两步验证', exact: true }).click()
  await page.getByLabel('当前密码', { exact: true }).fill(nextPassword)
  const freshStep = Math.max(Math.floor(Date.now() / 30000), step + 1)
  await page.getByLabel('动态验证码', { exact: true }).fill(totp(secret, freshStep * 30000))
  await page.getByRole('button', { name: '确认关闭两步验证' }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '开启两步验证', exact: true })).toBeVisible()
  await page.setViewportSize({ width: 360, height: 780 }); await noOverflow(page)
})

registerManagementTests({ seed, captcha, signIn })
