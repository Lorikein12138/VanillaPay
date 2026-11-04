import { expect, login, noOverflow, test, totp } from './fixtures'

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
  await expect(page.getByRole('heading', { name: '概览', exact: true })).toBeVisible()
  const links = page.getByRole('navigation', { name: '商户导航' }).getByRole('link')
  await expect(links).toHaveText(['概览'])
  await expect(links.first()).toHaveAttribute('aria-current', 'page')
  await expect(page.getByText(account.pid, { exact: true })).toBeVisible()
  await noOverflow(page)
  await page.reload()
  const logout = page.waitForResponse((response) => response.url().endsWith('/api/v1/auth/logout'))
  await page.getByRole('button', { name: '退出登录' }).click()
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
  await page.goto('/orders?filter=paid')
  await expect(page).toHaveURL(/\/login\?redirect=/)
  await expect(page.getByRole('navigation', { name: '商户导航' })).toHaveCount(0)
  expect(new URL(page.url()).searchParams.get('redirect')).toBe('/orders?filter=paid')
  await expect(page.locator('html')).toHaveAttribute('data-seen-protected-paths', '[]')
  await login(page, account)
  await expect(page).toHaveURL(/\/orders\?filter=paid$/)
  await expect(page.getByRole('heading', { name: '页面不存在' })).toBeVisible()
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
  expect(await page.locator('html').evaluate((element) => getComputedStyle(element).getPropertyValue('--vp-danger').trim())).toBe('#e0857a')
  await page.getByRole('button', { name: '浅色', exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await noOverflow(page)
  await page.screenshot({ path: testInfo.outputPath('login-light.png'), fullPage: true })
  await page.getByRole('button', { name: '深色', exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  expect(await page.locator('html').evaluate((element) => getComputedStyle(element).getPropertyValue('--vp-danger').trim())).toBe('#e0857a')
  await noOverflow(page)
  await page.screenshot({ path: testInfo.outputPath('login-dark.png'), fullPage: true })
  await login(page, account)
  await expect(page.getByRole('heading', { name: '概览', exact: true })).toBeVisible()
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
    await expect(page.getByRole('heading', { name: '概览', exact: true })).toBeVisible()
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
    await noOverflow(page)
    await page.getByRole('button', { name: '退出登录' }).click()
    await expect(page).toHaveURL(/\/login$/)
  })
})
