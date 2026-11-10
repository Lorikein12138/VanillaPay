import { expect, login, noOverflow, test, totp, clickControl, openMenu, closeMenu, polishShot } from './fixtures'
import type { components } from '@/api/schema'

test('signs in, restores CSRF after refresh, and signs out securely', async ({ page, account }) => {
  await page.goto('/login')
  await expect(page.getByRole('button', { name: '登录', exact: true })).toBeDisabled()
  await noOverflow(page)
  for (const target of await page.locator('button, a, input').all()) {
    const bounds = await target.boundingBox()
    if (bounds) { expect(bounds.width).toBeGreaterThanOrEqual(44); expect(bounds.height).toBeGreaterThanOrEqual(44) }
  }
  await login(page, account)
  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.getByRole('heading', { name: new RegExp(account.username) })).toBeVisible()
  await openMenu(page)
  const links = page.getByRole('navigation', { name: '商户导航' }).getByRole('link')
  await expect(links).toHaveText(['概览', '订单', '通知', '收款码', '监控设备', '接入配置', '订阅套餐'])
  await expect(links.first()).toHaveAttribute('aria-current', 'page')
  await expect(page.getByText(account.pid, { exact: true }).filter({ visible: true })).toBeVisible()
  await noOverflow(page)
  await clickControl(page, page.viewportSize()?.width === 360 ? '深色' : '浅色')
  await openMenu(page)
  if (page.viewportSize()!.width < 1024) {
    await polishShot(page, 'drawer', false)
    await page.keyboard.press('Escape')
    await expect(page.getByRole('button', { name: '打开菜单' })).toBeFocused()
    await clickControl(page, '浅色')
    await openMenu(page)
    await polishShot(page, 'drawer', false)
  } else await polishShot(page, 'sidebar')
  await closeMenu(page)
  await page.reload()
  const logout = page.waitForResponse((response) => response.url().endsWith('/api/v1/auth/logout'))
  await clickControl(page, '退出登录')
  expect((await logout).status()).toBe(200)
  await expect(page).toHaveURL(/\/login$/)
  await page.goto('/dashboard')
  await expect(page).toHaveURL(/\/login\?redirect=/)
  await expect(page.getByRole('navigation', { name: '商户导航' })).toHaveCount(0)
})

test('shows the same readable credential error for a wrong password', async ({ page, account }) => {
  await page.goto('/login')
  await login(page, { ...account, password: crypto.randomUUID() })
  await expect(page.getByRole('alert')).toHaveText('账号或密码错误')
  await expect(page.getByRole('navigation', { name: '商户导航' })).toHaveCount(0)
  await noOverflow(page)
})

test('guards /orders before rendering and returns there after login', async ({ page, account }) => {
  await page.addInitScript(() => {
    const seen: string[] = []
    new MutationObserver(() => {
      if (document.querySelector('nav[aria-label="商户导航"]')) seen.push(location.pathname)
      document.documentElement.dataset.seenProtectedPaths = JSON.stringify(seen)
    }).observe(document, { subtree: true, childList: true })
  })
  await page.goto('/orders?status=paid')
  await expect(page).toHaveURL(/\/login\?redirect=/)
  await expect(page.getByRole('navigation', { name: '商户导航' })).toHaveCount(0)
  expect(new URL(page.url()).searchParams.get('redirect')).toBe('/orders?status=paid')
  await expect(page.locator('html')).toHaveAttribute('data-seen-protected-paths', '[]')
  await login(page, account)
  await expect(page).toHaveURL(/\/orders\?status=paid$/)
  await expect(page.getByRole('heading', { name: '订单', exact: true })).toBeVisible()
  await expect(page.getByLabel('状态', { exact: true })).toHaveAttribute('data-value', 'paid')
  await noOverflow(page)
})

test('rejects unsafe redirect values and uses the backend dashboard path', async ({ page, account }) => {
  await page.goto('/login?redirect=%2F%2Fevil.com')
  await login(page, account)
  await expect(page).toHaveURL(/\/dashboard$/)
})

test('switches system and explicit dark themes without horizontal overflow', async ({ page, account }, testInfo) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto('/login')
  await expect(page.getByRole('heading', { name: '让每一笔收款都有着落' })).toBeVisible()
  const heroLines = page.getByRole('heading', { name: '让每一笔收款都有着落' }).locator('span')
  await expect(heroLines).toHaveText(['让每一笔收款', '都有着落'])
  for (const line of await heroLines.all()) {
    const singleLine = await line.evaluate((element) => {
      const style = getComputedStyle(element)
      return Math.abs(element.getBoundingClientRect().height - Number.parseFloat(style.lineHeight)) < 1
    })
    expect(singleLine).toBe(true)
  }
  expect(await page.locator('html').evaluate((element) => getComputedStyle(element).getPropertyValue('--vp-danger').trim())).toBe('#e0857a')
  await clickControl(page, '浅色')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await noOverflow(page)
  if (page.viewportSize()?.width === 1280) await polishShot(page, 'login')
  await page.screenshot({ path: testInfo.outputPath('login-light.png'), fullPage: true })
  await clickControl(page, '深色')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  expect(await page.locator('html').evaluate((element) => getComputedStyle(element).getPropertyValue('--vp-danger').trim())).toBe('#e0857a')
  await noOverflow(page)
  if (page.viewportSize()?.width === 360) await polishShot(page, 'login')
  await page.screenshot({ path: testInfo.outputPath('login-dark.png'), fullPage: true })
  await login(page, account)
  await expect(page.getByRole('heading', { name: new RegExp(account.username) })).toBeVisible()
  await noOverflow(page)
  await page.screenshot({ path: testInfo.outputPath('dashboard-dark.png'), fullPage: true })
})

test.describe('second factor', () => {
  test.use({ twoFactor: true })
  test('completes a real TOTP challenge', async ({ page, account }) => {
    await page.goto('/login')
    await login(page, account)
    await expect(page.getByRole('heading', { name: '二次验证' })).toBeVisible()
    await page.getByLabel('动态验证码').fill(totp(account.totpSeed))
    await page.getByRole('button', { name: '验证并登录' }).click()
    await expect(page).toHaveURL(/\/dashboard$/)
    await expect(page.getByRole('heading', { name: new RegExp(account.username) })).toBeVisible()
    await noOverflow(page)
  })

  test('retries wrong TOTP and recovery proofs in the same real challenge', async ({ page, account }) => {
    const challenges = new Set<string>()
    page.on('request', (request) => {
      if (request.url().endsWith('/api/v1/auth/second-factor')) challenges.add((request.postDataJSON() as components['schemas']['AuthFactorRequest']).challenge_id)
    })
    await page.goto('/login')
    await login(page, account)
    await expect(page.getByLabel('动态验证码')).toBeVisible()
    const now = Date.now()
    const validCodes = new Set([-30000, 0, 30000].map((offset) => totp(account.totpSeed, now + offset)))
    let wrongCode: string
    do { wrongCode = (crypto.getRandomValues(new Uint32Array(1))[0] ?? 0).toString().slice(-6).padStart(6, '0') } while (validCodes.has(wrongCode))
    await page.getByLabel('动态验证码').fill(wrongCode)
    const wrongTOTP = page.waitForResponse((response) => response.url().endsWith('/api/v1/auth/second-factor'))
    await page.getByRole('button', { name: '验证并登录' }).click()
    const rejectedTOTP = await wrongTOTP
    expect(rejectedTOTP.status()).toBe(401)
    expect((await rejectedTOTP.json() as components['schemas']['ErrorEnvelope']).error.code).toBe('INVALID_2FA')
    await expect(page.getByRole('alert')).toHaveText('验证码或恢复码错误，请重试。')
    await expect(page.getByText('登录已失效，请重新登录。')).toHaveCount(0)
    await page.getByRole('button', { name: '恢复码', exact: true }).click()
    await page.getByLabel('恢复码', { exact: true }).fill(crypto.randomUUID())
    const wrongRecovery = page.waitForResponse((response) => response.url().endsWith('/api/v1/auth/second-factor'))
    await page.getByRole('button', { name: '验证并登录' }).click()
    const rejectedRecovery = await wrongRecovery
    expect(rejectedRecovery.status()).toBe(401)
    expect((await rejectedRecovery.json() as components['schemas']['ErrorEnvelope']).error.code).toBe('INVALID_2FA')
    await expect(page.getByRole('alert')).toHaveText('验证码或恢复码错误，请重试。')
    await expect(page.getByText('登录已失效，请重新登录。')).toHaveCount(0)
    await page.getByRole('button', { name: '验证器', exact: true }).click()
    await page.getByLabel('动态验证码').fill(totp(account.totpSeed))
    await page.getByRole('button', { name: '验证并登录' }).click()
    await expect(page).toHaveURL(/\/dashboard$/)
    expect(challenges.size).toBe(1)
    await noOverflow(page)
  })
})

test.describe('disabled merchant', () => {
  test.use({ banned: true })
  test('shows the backend ban reason and allows logout', async ({ page, account }) => {
    await page.goto('/login')
    await login(page, account)
    await expect(page).toHaveURL(/\/banned$/)
    await expect(page.getByText('Browser test restriction')).toBeVisible()
    await expect(page.getByRole('navigation', { name: '商户导航' })).toHaveCount(0)
    await clickControl(page, page.viewportSize()?.width === 360 ? '深色' : '浅色')
    await polishShot(page, 'banned')
    await noOverflow(page)
    await clickControl(page, '退出登录')
    await expect(page).toHaveURL(/\/login$/)
  })
})
