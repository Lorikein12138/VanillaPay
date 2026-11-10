import { fileURLToPath } from 'node:url'
import type { APIRequestContext, Page } from '@playwright/test'
import type { components } from '@/api/schema'
import { chooseSelect, chooseMenu, expect, login, noOverflow, test, totp, clickControl, openMenu, closeMenu, polishShot } from './fixtures'

async function shot(page: Page, name: string) {
  const width = page.viewportSize()?.width
  if (width !== 360 && width !== 1280) return
  const theme = width === 360 ? 'dark' : 'light'
  await page.evaluate(() => document.fonts.ready)
  await noOverflow(page)
  await polishShot(page, name === 'binding' || name === 'one-time-key' || name === 'totp-setup' || name === 'recovery-codes' ? name+'-dialog' : name)
  await page.screenshot({ path: fileURLToPath(new URL(`../../../.artifacts/merchant-account-shots/${name}-${theme}.png`, import.meta.url)), fullPage: name !== 'totp-setup' && name !== 'recovery-codes', animations: 'disabled', mask: [page.getByTestId('totp-secret'), page.getByTestId('totp-qr'), page.getByTestId('recovery-codes'), page.locator('input[type=password]')], maskColor: '#73726c' })
}
async function chooseTheme(page: Page) { await clickControl(page, page.viewportSize()?.width === 360 ? '深色' : '浅色') }
async function signIn(page: Page, account: Parameters<typeof login>[1]) {
  await page.goto('/login?redirect=%2Faccount'); await login(page, account)
  await expect(page.getByRole('heading', { name: '账户安全' })).toBeVisible()
}
async function readMail(request: APIRequestContext, email: string) {
  const response = await request.post('/__e2e/mail', { data: { email } })
  expect(response.ok()).toBe(true)
  const result = await response.json() as { code: string }
  expect(/^\d{6}$/.test(result.code)).toBe(true)
  return result.code
}
async function sendCode(page: Page, request: APIRequestContext, email: string, button = '获取邮箱验证码', input = '邮箱验证码') {
  const response = page.waitForResponse((value) => value.url().endsWith('/auth/email-codes'))
  await page.getByRole('button', { name: button, exact: true }).click()
  expect((await response).status()).toBe(200)
  await page.getByLabel(input, { exact: true }).fill(await readMail(request, email))
}
async function newPassword(page: Page, password: string) { await page.getByLabel('新密码', { exact: true }).fill(password); await page.getByLabel('确认新密码', { exact: true }).fill(password) }

test('registers with separate single-use Turnstile tokens and signs in', async ({ page, account, request }) => {
  const username = `reg_${crypto.randomUUID().replaceAll('-', '').slice(0, 20)}`, email = `${username}@example.test`, password = crypto.randomUUID()
  await page.goto('/register'); await chooseTheme(page)
  await expect(page.getByRole('heading', { name: '注册商户' })).toBeVisible()
  await shot(page, 'register')
  await page.getByLabel('用户名', { exact: true }).fill(username); await page.getByLabel('邮箱', { exact: true }).fill(email)
  await page.getByRole('button', { name: '完成验证' }).click()
  await sendCode(page, request, email)
  await expect(page.getByRole('button', { name: '创建账户' })).toBeDisabled()
  await newPassword(page, password)
  await page.getByRole('checkbox', { name: '我同意服务条款' }).check(); await page.getByRole('checkbox', { name: '我同意隐私政策' }).check()
  const registration = page.getByRole('form', { name: '注册商户' })
  await expect(registration.getByRole('link', { name: '服务条款' })).toHaveAttribute('target', '_blank')
  await page.getByRole('button', { name: '完成验证' }).click()
  const registered = page.waitForResponse((value) => value.url().endsWith('/auth/register'))
  await page.getByRole('button', { name: '创建账户' }).click(); expect((await registered).status()).toBe(201)
  await expect(page.getByRole('status')).toHaveText('注册成功，请登录。')
  const summary = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/merchant/notifications' && new URL(response.url()).searchParams.get('limit') === '1')
  await login(page, { ...account, username, password })
  await expect(page).toHaveURL(/\/dashboard$/)
  // This real registration bypasses seedMerchant. Newly registered merchants
  // correctly receive published announcements, including other E2E fixtures.
  const { data } = await (await summary).json() as { data: components['schemas']['PortalNotificationPage'] }
  for (const announcement of data.popup_announcements ?? []) {
    await page.getByRole('dialog', { name: announcement.title.zh_CN, exact: true }).getByRole('button', { name: '知道了' }).click()
  }
  await openMenu(page); await expect(page.getByRole('link', { name: username, exact: true })).toBeVisible(); await closeMenu(page)
})

test('resets a password, revokes an existing session and signs in with the new password', async ({ page, account, request, playwright }) => {
  await signIn(page, account)
  const oldSession = await playwright.request.newContext({ baseURL: new URL(page.url()).origin, storageState: await page.context().storageState() })
  try {
    await page.goto('/reset-password'); await chooseTheme(page); await shot(page, 'reset-password')
    await page.getByLabel('邮箱', { exact: true }).fill(`${account.username}@example.test`)
    await page.getByRole('button', { name: '完成验证' }).click(); await sendCode(page, request, `${account.username}@example.test`)
    const password = crypto.randomUUID(); await newPassword(page, password)
    await page.getByRole('button', { name: '完成验证' }).click(); await page.getByRole('button', { name: '确认重置密码' }).click()
    await expect(page.getByRole('status')).toHaveText('密码已重置，旧会话已失效，请使用新密码登录。')
    expect((await oldSession.get('/api/v1/merchant/me')).status()).toBe(401)
    await login(page, { ...account, password }); await expect(page).toHaveURL(/\/dashboard$/)
  } finally { await oldSession.dispose() }
})

test('changes the username using a code bound to the new name', async ({ page, account, request }) => {
  await signIn(page, account); await chooseTheme(page); await shot(page, 'account')
  const username = `ren_${crypto.randomUUID().replaceAll('-', '').slice(0, 20)}`
  await page.getByRole('button', { name: '修改用户名', exact: true }).click(); await polishShot(page, 'username-dialog', false)
  await page.getByLabel('新用户名', { exact: true }).fill(username)
  await sendCode(page, request, `${account.username}@example.test`)
  await page.getByRole('button', { name: '确认保存', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await openMenu(page); await expect(page.getByRole('link', { name: username, exact: true })).toBeVisible(); await closeMenu(page)
  await page.reload(); await openMenu(page); await expect(page.getByRole('link', { name: username, exact: true })).toBeVisible(); await closeMenu(page)
})

test('changes the email using separate old and new mailbox proofs', async ({ page, account, request }) => {
  await signIn(page, account); await chooseTheme(page)
  const email = `mail_${crypto.randomUUID().replaceAll('-', '')}@example.test`
  await page.getByRole('button', { name: '修改邮箱', exact: true }).click(); await polishShot(page, 'email-dialog', false); await page.getByLabel('新邮箱', { exact: true }).fill(email)
  await sendCode(page, request, `${account.username}@example.test`, '获取旧邮箱验证码', '旧邮箱验证码')
  await sendCode(page, request, email, '获取新邮箱验证码', '新邮箱验证码')
  await page.getByRole('button', { name: '确认保存', exact: true }).click(); await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByText(email, { exact: true })).toBeVisible()
  await page.reload(); await expect(page.getByText(email, { exact: true })).toBeVisible()
})

test('changes the password, clears the session and requires a new login', async ({ page, account, request }) => {
  await signIn(page, account); await chooseTheme(page)
  await page.getByRole('button', { name: '修改密码', exact: true }).click(); await polishShot(page, 'password-dialog', false)
  const password = crypto.randomUUID(); await newPassword(page, password)
  await sendCode(page, request, `${account.username}@example.test`)
  await page.getByRole('button', { name: '确认保存', exact: true }).click()
  await expect(page.getByRole('status')).toHaveText('密码已修改，所有会话已失效，请重新登录。')
  expect((await page.request.get('/api/v1/merchant/me')).status()).toBe(401)
  await login(page, { ...account, password }); await expect(page).toHaveURL(/\/dashboard$/)
})

test('enables 2FA, logs in with TOTP and recovery, then disables 2FA', async ({ page, account }) => {
  test.setTimeout(90000)
  await signIn(page, account); await chooseTheme(page)
  await page.getByRole('button', { name: '开启两步验证', exact: true }).click()
  await polishShot(page, 'totp-password-dialog', false)
  await page.getByLabel('当前密码', { exact: true }).fill(account.password); await page.getByRole('button', { name: '验证密码并生成' }).click()
  const seed = (await page.getByTestId('totp-secret').textContent())!
  await expect(page.getByTestId('totp-qr')).toBeVisible(); await shot(page, 'totp-setup')
  const confirmedStep = Math.floor(Date.now() / 30000)
  await page.getByLabel('动态验证码', { exact: true }).fill(totp(seed, confirmedStep * 30000)); await page.getByRole('button', { name: '确认开启', exact: true }).click()
  const codes = (await page.getByTestId('recovery-codes').textContent())!.trim().split(/\s+/)
  expect(codes.length).toBe(10); await expect(page.getByTestId('totp-secret')).toHaveCount(0)
  await shot(page, 'recovery-codes')
  const downloading = page.waitForEvent('download'); await page.getByRole('button', { name: '下载恢复码 TXT' }).click()
  expect((await downloading).suggestedFilename()).toBe('vanillapay-recovery-codes.txt')
  await page.getByRole('button', { name: '关闭', exact: true }).click(); await expect(page.getByTestId('recovery-codes')).toHaveCount(0)
  await clickControl(page, '退出登录'); await login(page, account)
  await expect(page.getByRole('heading', { name: '二次验证' })).toBeVisible()
  // The backend accepts +/- one step and rejects reuse. Use a fresh valid step,
  // waiting at most one period before disabling instead of reusing a login code.
  const loginStep = Math.max(Math.floor(Date.now() / 30000), confirmedStep + 1)
  await page.getByLabel('动态验证码', { exact: true }).fill(totp(seed, loginStep * 30000)); await page.getByRole('button', { name: '验证并登录' }).click()
  await expect(page).toHaveURL(/\/dashboard$/)
  await clickControl(page, '退出登录'); await login(page, account)
  await page.getByRole('button', { name: '恢复码', exact: true }).click(); await page.getByLabel('恢复码', { exact: true }).fill(codes[0]!)
  await page.getByRole('button', { name: '验证并登录' }).click(); await expect(page).toHaveURL(/\/dashboard$/)
  await openMenu(page); await page.getByRole('link', { name: account.username, exact: true }).click(); await page.getByRole('button', { name: '关闭两步验证', exact: true }).click(); await polishShot(page, 'disable-totp-dialog', false)
  await page.getByLabel('当前密码', { exact: true }).fill(account.password)
  await expect.poll(() => Math.floor(Date.now() / 30000) >= loginStep, { timeout: 35000, intervals: [500] }).toBe(true)
  const disableStep = Math.max(Math.floor(Date.now() / 30000), loginStep + 1)
  await page.getByLabel('动态验证码', { exact: true }).fill(totp(seed, disableStep * 30000)); await page.getByRole('button', { name: '确认关闭两步验证' }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0); await expect(page.getByRole('button', { name: '开启两步验证', exact: true })).toBeVisible()
})

test('persists header and account preferences across sessions and overrides public local choices', async ({ page, account }) => {
  await signIn(page, account)
  const savedTheme = page.waitForResponse((response) => response.url().endsWith('/merchant/preferences'))
  await clickControl(page, '深色'); expect((await savedTheme).status()).toBe(200)
  await expect(page.getByLabel(/账户主题|Account theme/)).toHaveAttribute('data-value', 'dark')
  const savedLanguage = page.waitForResponse((response) => response.url().endsWith('/merchant/preferences'))
  await chooseSelect(page, page.getByLabel(/账户语言|Account language/), 'en'); expect((await savedLanguage).status()).toBe(200)
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
  await clickControl(page, 'Sign out')
  // Wait for the public page so this click cannot hit the departing account's
  // preference control while sign-out navigation is still in flight.
  await expect(page).toHaveURL(/\/login(?:\?|$)/)
  await chooseMenu(page, 'Language', '中文')
  await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN')
  await clickControl(page, '浅色')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await login(page, account); await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.locator('html')).toHaveAttribute('lang', 'en'); await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await openMenu(page); await page.getByRole('link', { name: account.username, exact: true }).click()
  await expect(page.getByLabel(/账户语言|Account language/)).toHaveAttribute('data-value', 'en'); await expect(page.getByLabel(/账户主题|Account theme/)).toHaveAttribute('data-value', 'dark')
  await noOverflow(page)
})
