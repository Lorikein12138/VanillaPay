import type { components } from '@/api/schema'
import { expect, login, test } from './fixtures'

type Schema = components['schemas']

test('logout invalidates a previously working merchant cookie', async ({ page, account, request }) => {
  await page.goto('/login')
  await login(page, account)
  await expect(page).toHaveURL(/\/dashboard$/)
  const cookies = await page.context().cookies()
  expect(cookies.some((cookie) => cookie.name === 'vp_merchant' || cookie.name === '__Host-vp_merchant')).toBe(true)
  const headers = { Cookie: cookies.map((cookie) => `${cookie.name}=${cookie.value}`).join('; ') }
  expect((await request.get('/api/v1/merchant/me', { headers })).status()).toBe(200)
  await page.getByRole('button', { name: '退出登录' }).click()
  await expect(page).toHaveURL(/\/login$/)
  const replay = await request.get('/api/v1/merchant/me', { headers })
  expect(replay.status()).toBe(401)
  expect((await replay.json() as Schema['ErrorEnvelope']).error.code).toBe('UNAUTHENTICATED')
})

test.describe('same SPA account switch', () => {
  test.use({ banned: true })
  test('isolates identity, ban reason and CSRF between two merchants without refreshing', async ({ page, account, request }) => {
    const reason = `Second restriction ${crypto.randomUUID()}`
    const response = await request.post('/__e2e/merchant', { data: { banned: true, ban_reason: reason } })
    expect(response.ok()).toBe(true)
    const second = await response.json() as typeof account
    await page.goto('/login')
    const marker = crypto.randomUUID()
    await page.evaluate((value) => { document.documentElement.dataset.spaMarker = value }, marker)
    const firstLogin = page.waitForResponse((result) => result.url().endsWith('/api/v1/auth/login'))
    await login(page, account)
    const firstSession = (await (await firstLogin).json() as { data: Schema['AuthSessionResult'] }).data
    await expect(page).toHaveURL(/\/banned$/)
    await expect(page.getByText('Browser test restriction', { exact: true })).toBeVisible()
    await expect(page.getByText(account.pid, { exact: true })).toBeVisible()
    await page.getByRole('button', { name: '退出登录' }).click()
    await expect(page).toHaveURL(/\/login$/)
    const secondLogin = page.waitForResponse((result) => result.url().endsWith('/api/v1/auth/login'))
    await login(page, second)
    const secondSession = (await (await secondLogin).json() as { data: Schema['AuthSessionResult'] }).data
    await expect(page).toHaveURL(/\/banned$/)
    await expect(page.getByText(reason, { exact: true })).toBeVisible()
    await expect(page.getByText('Browser test restriction', { exact: true })).toHaveCount(0)
    await expect(page.getByText(second.username, { exact: true })).toBeVisible()
    await expect(page.getByText(second.pid, { exact: true })).toBeVisible()
    await expect(page.getByText(account.username, { exact: true })).toHaveCount(0)
    await expect(page.getByText(account.pid, { exact: true })).toHaveCount(0)
    expect(await page.evaluate((value) => document.documentElement.dataset.spaMarker === value, marker)).toBe(true)
    expect(firstSession.user.id !== secondSession.user.id).toBe(true)
    expect(firstSession.csrf_token !== secondSession.csrf_token).toBe(true)
    const me = await page.request.get('/api/v1/merchant/me')
    expect(me.status()).toBe(200)
    const identity = (await me.json() as { data: Schema['SessionIdentity'] }).data
    expect(identity.user.id === secondSession.user.id && identity.csrf_token === secondSession.csrf_token).toBe(true)
    const logout = page.waitForRequest((result) => result.url().endsWith('/api/v1/auth/logout'))
    const loggedOut = page.waitForResponse((result) => result.url().endsWith('/api/v1/auth/logout'))
    await page.getByRole('button', { name: '退出登录' }).click()
    expect((await logout).headers()['x-csrf-token'] === secondSession.csrf_token).toBe(true)
    expect((await loggedOut).status()).toBe(200)
    await expect(page).toHaveURL(/\/login$/)
  })
})

for (const nextPath of [
  'https://evil.example/path', '//evil.example/path', 'javascript:alert(1)',
  '/console', '/console/settings', '/%63onsole', '/dashboard/../console',
  '/%2F%2Fevil.example', '/%5Cevil.example',
]) {
  test(`rejects malicious backend next_path ${nextPath}`, async ({ page, account }) => {
    // Preserve the real login response/cookies; change only the untrusted destination.
    await page.route('**/api/v1/auth/login', async (route) => {
      const response = await route.fetch()
      expect(response.status()).toBe(200)
      const body = await response.json() as { data: Schema['AuthSessionResult'] }
      body.data.next_path = nextPath
      await route.fulfill({ response, json: body })
    })
    await page.goto('/login')
    await login(page, account)
    await expect(page).toHaveURL(/\/dashboard$/)
    await expect(page.getByRole('heading', { name: '概览', exact: true })).toBeVisible()
  })
}

for (const signedIn of [false, true]) {
  test(`holds all protected content until merchant/me resolves (signedIn=${signedIn})`, async ({ page, account }) => {
    await page.clock.install()
    if (signedIn) {
      await page.goto('/login')
      await login(page, account)
      await expect(page).toHaveURL(/\/dashboard$/)
    }
    let release: () => void = () => { throw new Error('uninitialized response gate') }
    const gate = new Promise<void>((resolve) => { release = resolve })
    let received = false
    const scripts = new Set<string>()
    let scriptChanges = 0
    page.on('request', (request) => { if (request.resourceType() === 'script') { scripts.add(request.url()); scriptChanges++ } })
    page.on('requestfinished', (request) => { scripts.delete(request.url()) })
    page.on('requestfailed', (request) => { scripts.delete(request.url()) })
    await page.addInitScript(() => {
      new MutationObserver(() => {
        if (document.documentElement.dataset.meReleased !== 'true' && document.querySelector('aside')) document.documentElement.dataset.earlyProtectedContent = 'true'
      }).observe(document, { childList: true, subtree: true })
    })
    await page.route('**/api/v1/merchant/me', async (route) => {
      const response = await route.fetch()
      received = true
      await gate
      await route.fulfill({ response })
    })
    try {
      await page.goto('/dashboard', { waitUntil: 'domcontentloaded' })
      await expect.poll(() => received).toBe(true)
      // Drain actual script loads and rendering, including the lazy layout import.
      // An immediate absence assertion can pass before an incorrectly eager layout mounts.
      for (;;) {
        const before = scriptChanges
        // Drive router/Suspense pending timers without a real-time sleep.
        await page.clock.runFor(2000)
        await expect.poll(() => scripts.size).toBe(0)
        await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
        if (before === scriptChanges && scripts.size === 0) break
      }
      expect(await page.locator('html').getAttribute('data-early-protected-content')).toBeNull()
      await expect(page.getByRole('navigation', { name: '商户导航' })).toHaveCount(0)
      await expect(page.getByRole('heading', { name: '概览', exact: true })).toHaveCount(0)
      await expect(page.getByRole('button', { name: '退出登录' })).toHaveCount(0)
      await expect(page.getByText(account.username, { exact: true })).toHaveCount(0)
    } finally { await page.evaluate(() => { document.documentElement.dataset.meReleased = 'true' }); release() }
    if (signedIn) await expect(page.getByRole('heading', { name: '概览', exact: true })).toBeVisible()
    else await expect(page).toHaveURL(/\/login\?redirect=/)
  })
}
