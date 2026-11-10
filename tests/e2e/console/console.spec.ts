import { registerManagementTests } from './manage-cases'
import { registerAnnouncementAuditTests } from './announcements-audit-cases'
import { registerPublicSiteTests } from './public-site-cases'
import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
import QRCode from 'qrcode'
import type { ConsoleRecovery } from '../../../web/src/api/client'
import { chooseSelect, clickControl, selectScreenshotTheme, login, noOverflow, polishShot, totp } from '../merchant/fixtures'

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
const control = clickControl
async function shots(
  page: Page,
  state: string,
  authenticated = false,
  drawer = false,
  directory = 'console-auth-shots',
  passwords: 'mask' | 'assert-empty' = 'mask',
) {
  for (const width of drawer ? [360] : [360, 1280]) {
    await page.setViewportSize({ width, height: width === 360 ? 780 : 800 })
    for (const [theme, label] of [['light', '浅色'], ['dark', '深色']] as const) {
      if (authenticated) await selectScreenshotTheme(page, theme, 'console')
      else await control(page, label)
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      if (drawer) await page.getByRole('button', { name: '打开菜单', exact: true }).click()
      await polishShot(page, state, !drawer, directory, passwords)
      if (state === 'settings-footer_legal' && process.env.WEB_VISUAL_POLISH === '1') {
        await polishShot(page, 'settings-subsection-titles', true, 'web-visual-polish-shots', passwords)
      }
      if (drawer) await page.keyboard.press('Escape')
    }
  }
}
async function settingsStateShots(page: Page, name: string, fullPage = false, passwords: 'mask' | 'assert-empty' = 'mask') {
  if (await page.getByRole('dialog').count()) {
    for (const width of [360, 1280]) {
      await page.setViewportSize({ width, height: width === 360 ? 780 : 800 })
      await polishShot(page, `settings-${name}`, false, 'console-settings-shots', 'assert-empty')
    }
    await page.keyboard.press('Escape')
    await selectScreenshotTheme(page, 'dark', 'console')
    await page.getByRole('button', { name: '清除人机验证密钥' }).click()
    await expect(page.getByRole('dialog', { name: '清除人机验证密钥' })).toBeVisible()
    for (const width of [360, 1280]) {
      await page.setViewportSize({ width, height: width === 360 ? 780 : 800 })
      await polishShot(page, `settings-${name}`, false, 'console-settings-shots', 'assert-empty')
    }
    return
  }
  for (const width of [360, 1280]) {
    await page.setViewportSize({ width, height: width === 360 ? 780 : 800 })
    for (const theme of ['light', 'dark'] as const) {
      await selectScreenshotTheme(page, theme, 'console')
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      await polishShot(page, `settings-${name}`, fullPage, 'console-settings-shots', passwords)
    }
  }
}

test('collapses the console sidebar and keeps its account in view on long settings pages', async ({ page, request }) => {
  const admin = await seed(request)
  await captcha(page, admin.turnstileToken)
  await signIn(page, admin, '/console/settings?group=footer_legal')
  await expect(page.getByRole('heading', { name: '系统设置', exact: true })).toBeVisible()
  const collapse = page.getByRole('button', { name: '收起侧栏' })
  const expand = page.getByRole('button', { name: '展开侧栏' })
  const sidebar = page.locator('aside').filter({ has: collapse.or(expand) })
  const user = sidebar.getByRole('link', { name: admin.username, exact: true })
  for (const theme of ['light', 'dark'] as const) {
    await selectScreenshotTheme(page, theme, 'console')
    if (process.env.WEB_VISUAL_POLISH === '1') await polishShot(page, 'console-sidebar-expanded', false, 'sidebar-nav-shots')
    await collapse.click()
    await expect(sidebar).toHaveCSS('width', '64px')
    if (process.env.WEB_VISUAL_POLISH === '1') await polishShot(page, 'console-sidebar-collapsed', false, 'sidebar-nav-shots')
    await user.focus()
    await expect(page.getByRole('tooltip')).toContainText(admin.username)
    await expect(page.getByRole('tooltip')).toContainText('管理员')
    await page.keyboard.press('Escape')
    const before = await user.boundingBox()
    await page.evaluate(() => scrollTo(0, document.documentElement.scrollHeight))
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(0)
    expect((await user.boundingBox())!.y).toBeCloseTo(before!.y, 0)
    if (process.env.WEB_VISUAL_POLISH === '1') await polishShot(page, 'console-sidebar-scrolled', false, 'portal-ux-shots')
    await expand.click()
    await page.evaluate(() => scrollTo(0, 0))
  }
  if (process.env.WEB_VISUAL_POLISH === '1') {
    await page.setViewportSize({ width: 360, height: 780 })
    for (const theme of ['light', 'dark'] as const) {
      await selectScreenshotTheme(page, theme, 'console')
      await page.getByRole('button', { name: '打开菜单', exact: true }).click()
      await polishShot(page, 'console-drawer', false, 'sidebar-nav-shots')
      await page.keyboard.press('Escape')
    }
  }
})

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
  await expect(page.getByRole('navigation', { name: '管理控制台' }).getByRole('link')).toHaveText([
    '概览', '商户', '订单', '套餐', '公告', '购买记录', '系统设置', '审计日志',
  ])
  await control(page, '退出登录')
  await expect(page).toHaveURL(/\/console\/login$/)
  await page.goto('/console/setup')
  await expect(page).toHaveURL(/\/console\/login\?notice=initialized$/)
})

test('manages platform settings, protects stored secrets and captures every group', async ({ page, request }) => {
  test.setTimeout(120000)
  const admin = await seed(request); await captcha(page, admin.turnstileToken)
  await signIn(page, admin, '/console/settings')
  await expect(page.getByRole('heading', { name: '站点信息', exact: true })).toBeVisible()
  await page.getByLabel('站点名称').fill('VanillaPay E2E')
  const siteSave = page.waitForResponse((response) => response.url().endsWith('/console/settings/site') && response.request().method() === 'PUT')
  await page.getByRole('button', { name: '保存站点信息' }).click()
  expect((await siteSave).status()).toBe(200)
  await page.reload()
  await expect(page.getByLabel('站点名称')).toHaveValue('VanillaPay E2E')

  const logoUpload = page.waitForResponse((response) => response.url().endsWith('/console/assets/logo') && response.request().method() === 'POST')
  const logoPNG = await QRCode.toBuffer('https://vanillapay.example.test/logo', { width: 64 })
  await page.locator('#logo-file').setInputFiles({
    name: 'platform-logo.png',
    mimeType: 'image/png',
    buffer: logoPNG,
  })
  const logoResponse = await logoUpload
  expect(logoResponse.status()).toBe(201)
  const logoAsset = (await logoResponse.json() as { data: { asset_id: string } }).data.asset_id
  await expect(page.getByRole('img', { name: '站点标识' })).toBeVisible()
  const logoSave = page.waitForResponse((response) => response.url().endsWith('/console/settings/site') && response.request().method() === 'PUT')
  await page.getByRole('button', { name: '保存站点信息' }).click()
  expect((await logoSave).status()).toBe(200)
  await page.reload()
  await expect(page.getByRole('main').getByRole('img', { name: '站点标识' })).toHaveAttribute('src', `/media/${logoAsset}`)

  await page.goto('/console/settings?group=mail')
  await page.getByLabel('邮件服务器').fill('mail.e2e.example.test')
  await page.getByLabel('端口').fill('465')
  await page.getByLabel('用户名').fill('mailer')
  await page.getByLabel('发件邮箱').fill('sender@example.test')
  await page.getByLabel('发件人名称').fill('E2E Sender')
  const mailPassword = page.getByLabel('邮件密码', { exact: true })
  const replacementPassword = crypto.randomUUID()
  await mailPassword.fill(replacementPassword)
  const firstMailSave = page.waitForResponse((response) => response.url().endsWith('/console/settings/mail') && response.request().method() === 'PUT')
  await page.getByRole('button', { name: '保存邮件设置' }).click()
  const firstMailResponse = await firstMailSave
  expect(firstMailResponse.status()).toBe(200)
  const firstMailBody = firstMailResponse.request().postDataJSON() as { value: Record<string, unknown> }
  expect(firstMailBody.value.password).toBe(replacementPassword)
  await expect(mailPassword).toHaveValue('')
  await page.reload()
  await expect(mailPassword).toHaveAccessibleDescription('已配置；留空会保留当前密钥。')
  await page.getByLabel('邮件服务器').fill('mail-updated.e2e.example.test')
  const secondMailSave = page.waitForResponse((response) => response.url().endsWith('/console/settings/mail') && response.request().method() === 'PUT')
  await page.getByRole('button', { name: '保存邮件设置' }).click()
  const secondMailResponse = await secondMailSave
  expect(secondMailResponse.status()).toBe(200)
  const secondMailBody = secondMailResponse.request().postDataJSON() as { value: Record<string, unknown> }
  expect(secondMailBody.value).not.toHaveProperty('password')

  for (const group of ['site', 'home', 'footer_legal', 'mail', 'security', 'payment', 'billing']) {
    await page.goto(`/console/settings?group=${group}`)
    await expect(page.locator('main')).toBeVisible()
    if (group === 'footer_legal') {
      await expect(page.getByText('暂无页脚链接，访客页面底部将不显示链接。')).toBeVisible()
      await settingsStateShots(page, 'footer-empty', true)
      const addFooter = page.getByRole('button', { name: '新增页脚项目' })
      await addFooter.click()
      await addFooter.click()
      const footerLabels = page.getByLabel('中文名称', { exact: true })
      await footerLabels.nth(0).fill('帮助中心')
      await footerLabels.nth(1).fill('服务时间')
      const englishLabels = page.getByLabel('英文名称', { exact: true })
      await englishLabels.nth(0).fill('Help center')
      await englishLabels.nth(1).fill('Support hours')
      await page.getByLabel('说明文字', { exact: true }).nth(1).fill('周一至周五 09:00–18:00')
      const urls = page.getByLabel('链接地址', { exact: true })
      await urls.nth(0).fill('https://support.example.test')
      await expect(urls.nth(1)).toBeVisible()
      await expect(urls.nth(1)).toHaveValue('')
    }
    await shots(page, `settings-${group}`, true, false, 'console-settings-shots', 'assert-empty')
  }

  await page.goto('/console/settings?group=site')
  await page.getByLabel('站点名称').fill('Conflict candidate')
  await page.route('**/api/v1/console/settings', async (route) => {
    const response = await route.fetch()
    const body = await response.json() as { data: { version: number; site: { site_name: string } } }
    body.data.version++
    body.data.site.site_name = '另一位管理员更新的站点'
    await route.fulfill({ response, json: body })
  })
  await page.route('**/api/v1/console/settings/site', (route) => route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ error: { code: 'VERSION_CONFLICT', message: 'version conflict' } }) }))
  await page.getByRole('button', { name: '保存站点信息' }).click()
  await expect(page.getByLabel('我已查看最新内容')).toBeVisible()
  await settingsStateShots(page, 'version-conflict')
  await page.unroute('**/api/v1/console/settings/site')
  await page.unroute('**/api/v1/console/settings')

  await page.route('**/api/v1/console/settings', async (route) => {
    const response = await route.fetch(), body = await response.json() as { data: { security: Record<string, unknown> } }
    body.data.security.secret_configured = true
    await route.fulfill({ response, json: body })
  })
  await page.goto('/console/settings?group=security')
  const lightTheme = page.waitForResponse((response) => response.url().endsWith('/console/preferences') && response.request().method() === 'PATCH')
  await control(page, '浅色')
  expect((await lightTheme).status()).toBe(200)
  await page.getByRole('button', { name: '清除人机验证密钥' }).click()
  await expect(page.getByRole('dialog', { name: '清除人机验证密钥' })).toBeVisible()
  await expect(page.getByRole('link', { name: '恢复入口' })).toHaveAttribute('href', '/console/recovery')
  await settingsStateShots(page, 'clear-secret-confirmation')
  await page.getByRole('button', { name: '确认清除' }).click()
  await expect(page.getByPlaceholder('保存后将清除')).toBeDisabled()
  await expect(page.getByRole('button', { name: '撤销清除' })).toBeVisible()
  await settingsStateShots(page, 'secret-pending-clear', true, 'assert-empty')
  await page.getByRole('button', { name: '撤销清除' }).click()
  await page.getByRole('checkbox', { name: '限制注册邮箱域名' }).check()
  await page.getByLabel('允许的邮箱域名（每行一个）').fill('example.test\nshop.example.test')
  await settingsStateShots(page, 'security-restricted', true)
  await page.unroute('**/api/v1/console/settings')

  await page.goto('/console/settings?group=mail')
  await page.getByPlaceholder('收件人邮箱').fill('ops@example.test')
  await page.route('**/api/v1/console/settings/mail/test', (route) => route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify({ data: { delivery_id: 'e2e-delivery', state: 'queued' } }) }))
  await page.route('**/api/v1/console/settings/mail/test/*', (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { id: 'e2e-delivery', state: 'sent', error_code: null } }) }))
  await page.getByRole('button', { name: '发送测试邮件' }).click()
  const mailTest = page.getByRole('region', { name: '测试邮件' })
  const mailSent = mailTest.getByText('测试邮件已发出。', { exact: true })
  const mailFailed = mailTest.getByText('邮件发送失败，后台会自动重试几次。', { exact: true })
  await expect(mailSent).toBeVisible({ timeout: 8000 })
  await expect(mailFailed).toHaveCount(0)
  await settingsStateShots(page, 'mail-test-sent', true, 'assert-empty')
  await page.unroute('**/api/v1/console/settings/mail/test/*')
  await page.route('**/api/v1/console/settings/mail/test/*', (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { id: 'e2e-delivery', state: 'failed', error_code: 'smtp_delivery_failed' } }) }))
  await page.getByRole('button', { name: '发送测试邮件' }).click()
  await expect(mailFailed).toBeVisible({ timeout: 8000 })
  await expect(mailSent).toHaveCount(0)
  await settingsStateShots(page, 'mail-test-failed', true, 'assert-empty')
  await page.unroute('**/api/v1/console/settings/mail/test')
  await page.unroute('**/api/v1/console/settings/mail/test/*')

  await page.reload()
  await page.getByRole('button', { name: '清除邮件密码' }).click()
  await page.getByRole('button', { name: '确认清除' }).click()
  await page.route('**/api/v1/console/settings', async (route) => {
    const response = await route.fetch()
    const body = await response.json() as { data: { version: number } }
    body.data.version++
    await route.fulfill({ response, json: body })
  })
  await page.route('**/api/v1/console/settings/mail', (route) => route.fulfill({
    status: 409,
    json: { error: { code: 'VERSION_CONFLICT', message: 'version conflict' } },
  }))
  const saveMail = page.getByRole('button', { name: '保存邮件设置' })
  await saveMail.click()
  await expect(page.getByLabel('我已查看最新内容')).toBeVisible()
  await expect(page.getByText('已配置', { exact: true })).toBeVisible()
  await expect(saveMail).toBeDisabled()
  await settingsStateShots(page, 'secret-version-conflict', true, 'assert-empty')
  await page.unroute('**/api/v1/console/settings/mail')
  await page.unroute('**/api/v1/console/settings')

  await page.route('**/api/v1/console/settings', async (route) => {
    const response = await route.fetch(), body = await response.json() as { data: { billing: Record<string, unknown> } }
    Object.assign(body.data.billing, {
      credential_valid: false,
      merchant_id: 51,
      merchant_pid: '1000000000000051',
      key_configured: true,
    })
    await route.fulfill({ response, json: body })
  })
  await page.goto('/console/settings?group=billing')
  await expect(page.getByText('订阅收款当前不可用，请重新填写商户号和密钥。')).toBeVisible()
  await expect(page.getByLabel('商户号')).toHaveAccessibleDescription('已关联商户 #51')
  await expect(page.getByRole('link', { name: '#51' })).toHaveAttribute('href', '/console/merchants/51')
  await expect(page.getByLabel('商户密钥')).toHaveAccessibleDescription(
    '已配置；留空会保留当前密钥。 需先清空商户号，才能清除已配置的密钥。',
  )
  await expect(page.getByRole('button', { name: '清除商户密钥' })).toHaveCount(0)
  await settingsStateShots(page, 'billing-invalid', false, 'assert-empty')
  await page.unroute('**/api/v1/console/settings')

  await page.goto('/console/settings?group=site')
  await expect(page.locator('#site-icon')).toHaveAttribute('href', /^\/favicon\.ico\?v=/)
  const beforeIcon = await page.locator('#site-icon').getAttribute('href')
  const image = await QRCode.toBuffer('fixture icon', { width: 144 })
  const uploadedIcon = page.waitForResponse((response) => response.url().endsWith('/console/assets/favicon'))
  await page.getByLabel('网站图标', { exact: true }).setInputFiles({ name: 'website-icon.png', mimeType: 'image/png', buffer: image })
  expect((await uploadedIcon).status()).toBe(201)
  await expect(page.getByRole('img', { name: '网站图标', exact: true })).toBeVisible()
  await expect(page.locator('#site-icon')).toHaveAttribute('href', beforeIcon!)
  await page.getByRole('button', { name: '保存站点信息' }).click()
  await expect(page.getByText('站点信息已保存。')).toBeVisible()
  await expect(page.locator('#site-icon')).not.toHaveAttribute('href', beforeIcon!)
  await expect(page.locator('#apple-icon')).toHaveAttribute('href', /^\/apple-touch-icon\.png\?v=/)
  await settingsStateShots(page, 'favicon-published', true)
  await page.getByRole('button', { name: '移除网站图标' }).click()
  await expect(page.getByRole('img', { name: '网站图标', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '撤销网站图标更改' }).click()
  await expect(page.getByRole('img', { name: '网站图标', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '移除网站图标' }).click()
  await page.getByRole('button', { name: '移除标识' }).click()
  await expect(page.getByRole('img', { name: '站点标识' })).toHaveCount(0)
  await settingsStateShots(page, 'logo-removed')
  const removeLogoSave = page.waitForResponse((response) =>
    response.url().endsWith('/console/settings/site') && response.request().method() === 'PUT',
  )
  await page.getByRole('button', { name: '保存站点信息' }).click()
  const removalResponse = await removeLogoSave
  expect(removalResponse.status()).toBe(200)
  expect(removalResponse.request().postDataJSON()).toMatchObject({ value: { logo_asset_id: null, favicon_asset_id: null } })
})

test('shares login, preserves console redirect, isolates role guards, and offers recovery when verification is off', async ({ page, request }) => {
  const admin = await seed(request); await captcha(page, admin.turnstileToken)
  await page.goto('/console/not-implemented?state=paid')
  await expect(page).toHaveURL(/\/console\/login\?redirect=/)
  await expect(page.getByRole('button', { name: '完成验证' })).toBeVisible()
  await shots(page, 'login-turnstile-enabled')
  await page.route('**/api/v1/public/config', async (route) => {
    const response = await route.fetch()
    const body = await response.json() as { data: { turnstile: { enabled: boolean } } }
    body.data.turnstile.enabled = false
    await route.fulfill({ response, json: body })
  })
  await page.reload()
  await expect(page.getByRole('button', { name: '登录', exact: true })).toBeEnabled()
  await expect(page.getByRole('link', { name: '恢复管理端访问' })).toBeVisible()
  await expect(page.getByRole('button', { name: '完成验证' })).toHaveCount(0)
  await shots(page, 'login-turnstile-disabled')
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

test('repairs only the existing security values and consumes both the terminal code and repair session', async ({ page, request, context }) => {
  await seed(request)
  const issued = await request.post('/__e2e/console/recovery', { data: {} })
  expect(issued.status()).toBe(200)
  const { recovery_code: code } = await issued.json() as { recovery_code: string }
  await page.goto('/console/recovery')
  await expect(page.getByText('docker exec vanillapay /usr/local/bin/server console-recovery', { exact: true })).toBeVisible()
  await shots(page, 'recovery-code')
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.getByRole('button', { name: '复制', exact: true }).click()
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('docker exec vanillapay /usr/local/bin/server console-recovery')
  const recovered = page.waitForResponse((response) => response.url().endsWith('/console/recovery'))
  await page.getByLabel('终端恢复码').fill(code)
  await page.getByRole('button', { name: '验证恢复码' }).click()
  const recovery = (await (await recovered).json() as { data: ConsoleRecovery }).data
  await expect(page.getByLabel('Turnstile Site Key')).toHaveValue(recovery.security.turnstile_site_key)
  expect(await page.getByRole('checkbox', { name: '要求人机验证（Turnstile）' }).isChecked()).toBe(recovery.security.turnstile_enabled)
  await expect(page.getByLabel('Turnstile Secret')).toHaveValue('')
  await shots(page, 'recovery-settings')
  const repair = page.waitForResponse((response) => response.url().endsWith('/console/settings/security'))
  await page.getByRole('button', { name: '保存并结束修复' }).click()
  const saved = await repair
  expect(saved.status()).toBe(200)
  const body = saved.request().postDataJSON() as { value: Record<string, string> }
  expect(Object.keys(body.value).sort()).toEqual(['turnstile_enabled', 'turnstile_site_key'])
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
  await chooseSelect(page, page.getByLabel('账户语言'), 'en')
  await expect(page.getByRole('heading', { name: 'Administrator account', exact: true })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Administrator account', exact: true })).toBeVisible()
  await chooseSelect(page, page.getByLabel('Account language'), 'zh-CN')
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
registerAnnouncementAuditTests({ seed, captcha, signIn })
registerPublicSiteTests({ seed, captcha, signIn })
