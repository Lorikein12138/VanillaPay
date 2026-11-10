import type { Locator, Page } from '@playwright/test'
import type { components } from '@/api/schema'
import { chooseMenu, expect, login, noOverflow, openMenu, polishShot, selectScreenshotTheme, test } from './fixtures'

const screenshots = process.env.WEB_VISUAL_POLISH === '1'

async function capture(page: Page, name: string) {
  if (screenshots) await polishShot(page, name, false, 'portal-ux-shots')
}

async function expectFullPid(user: Locator, pid: string) {
  const role = user.getByText(pid, { exact: true })
  await expect(role).toHaveText(pid)
  await expect(role).toBeVisible()
  await expect(role).toHaveCSS('font-size', '12px')
  await expect.poll(() => role.evaluate((element) => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(0)
}

test('keeps the desktop sidebar reachable and fixed through scrolling and reload', async ({ page, account, request }) => {
  const orders = Array.from({ length: 20 }, (_, index) => ({
    trade_no: crypto.randomUUID(), out_trade_no: `portal-${index}`, name: `测试订单 ${index + 1}`,
    channel: 'alipay', status: 'paid', original_cents: 2000, actual_cents: 2000, created_at: new Date().toISOString(), paid_at: new Date().toISOString(),
  }))
  expect((await request.post('/__e2e/orders', { data: { pid: account.pid, orders } })).status()).toBe(204)
  await page.goto('/login?redirect=%2Forders')
  await login(page, account)
  await expect(page.getByRole('heading', { name: '订单', exact: true })).toBeVisible()

  const expand = page.getByRole('button', { name: '展开侧栏' })
  const collapse = page.getByRole('button', { name: '收起侧栏' })
  if (page.viewportSize()!.width < 1024) {
    await expect(collapse).toHaveCount(0)
    await openMenu(page)
    const drawer = page.getByRole('dialog', { name: '商户导航' })
    await expectFullPid(drawer.getByRole('link', { name: account.username, exact: true }), account.pid)
    await page.keyboard.press('Escape')
    if (!screenshots) return
    for (const theme of ['light', 'dark'] as const) {
      await selectScreenshotTheme(page, theme)
      await openMenu(page)
      await polishShot(page, 'merchant-drawer', false, 'sidebar-nav-shots')
      await page.keyboard.press('Escape')
    }
    return
  }

  const sidebar = page.locator('aside').filter({ has: collapse.or(expand) })
  const navigation = sidebar.getByRole('navigation')
  const user = sidebar.getByRole('link', { name: account.username, exact: true })
  for (const theme of ['light', 'dark'] as const) {
    await selectScreenshotTheme(page, theme)
    await expect(sidebar).toHaveCSS('width', '224px')
    await expectFullPid(user, account.pid)
    const toggleBox = (await collapse.boundingBox())!
    expect(toggleBox.y + toggleBox.height).toBeLessThan((await user.boundingBox())!.y)
    if (screenshots) await polishShot(page, 'merchant-sidebar-expanded', false, 'sidebar-nav-shots')

    await collapse.focus()
    await page.keyboard.press('Enter')
    await expect(expand).toBeFocused()
    await expect(expand).toHaveAttribute('aria-expanded', 'false')
    await expect(sidebar).toHaveCSS('width', '64px')
    if (screenshots) await polishShot(page, 'merchant-sidebar-collapsed', false, 'sidebar-nav-shots')
    const overview = navigation.getByRole('link', { name: '概览' })
    await sidebar.getByRole('link').first().focus()
    await page.keyboard.press('Tab')
    await expect(overview).toBeFocused()
    await expect(page.getByRole('tooltip')).toHaveText('概览')
    await page.keyboard.press('Escape')
    await user.focus()
    await expect(page.getByRole('tooltip')).toContainText(account.username)
    await expect(page.getByRole('tooltip')).toContainText(account.pid)
    await page.keyboard.press('Escape')

    const before = await user.boundingBox()
    await page.evaluate(() => scrollTo(0, document.documentElement.scrollHeight))
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(0)
    expect((await user.boundingBox())!.y).toBeCloseTo(before!.y, 0)
    await capture(page, 'merchant-sidebar-scrolled')
    await page.reload()
    await expect(expand).toHaveAttribute('aria-expanded', 'false')
    await expect(sidebar).toHaveCSS('width', '64px')
    await page.mouse.move(0, 0)
    await expand.hover()
    await expect(page.getByRole('tooltip')).toHaveText('展开侧栏')
    await expand.click()
    await expect(collapse).toHaveAttribute('aria-expanded', 'true')
    await page.evaluate(() => scrollTo(0, 0))
  }
})

test('preloads from keyboard intent and shows a content skeleton during a cold navigation', async ({ page, account }) => {
  let release!: () => void
  let started!: () => void
  const gate = new Promise<void>((resolve) => { release = resolve })
  const requestStarted = new Promise<void>((resolve) => { started = resolve })
  let requests = 0
  await page.route('**/api/v1/merchant/orders?*', async (route) => {
    if (new URL(route.request().url()).searchParams.get('limit') !== '20') return route.continue()
    requests++
    started()
    await gate
    await route.continue()
  })
  await page.goto('/login')
  await login(page, account)
  await expect(page).toHaveURL(/\/dashboard$/)
  await expect.poll(() => page.evaluate(() => performance.getEntriesByType('resource').some(
    (entry) => entry.name.includes('/assets/_merchant.integration.lazy-'),
  ))).toBe(true)
  expect(requests).toBe(0)
  const header = page.getByRole('banner')
  await header.evaluate((element) => element.setAttribute('data-retained', 'true'))
  await openMenu(page)
  const orders = page.getByRole('link', { name: '订单', exact: true })
  await orders.focus()
  try {
    await requestStarted
    await expect(page).toHaveURL(/\/dashboard$/)
    await page.keyboard.press('Enter')
    await expect(page.getByRole('main').getByRole('status', { name: '正在加载…' })).toBeVisible()
    await expect(header).toHaveAttribute('data-retained', 'true')
    if (page.viewportSize()!.width >= 1024) await expect(page.getByRole('navigation', { name: '商户导航' })).toBeVisible()
  } finally {
    release()
  }
  await expect(page.getByRole('heading', { name: '订单', exact: true })).toBeVisible()
  await expect(page.getByText('没有符合筛选条件的订单')).toBeVisible()
  expect(requests).toBe(1)
})

test('operates order controls with the keyboard and round-trips and clears URL filters', async ({ page, account }) => {
  await page.goto('/login?redirect=%2Forders%3Fstart%3D2026-11-02T01%253A02%253A03Z')
  await login(page, account)
  await page.waitForURL(/\/orders\?/)
  const status = page.getByRole('combobox', { name: '状态', exact: true })
  const start = page.getByLabel('开始时间', { exact: true })
  await expect(start).not.toHaveValue('')
  await status.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('listbox')).toBeVisible()
  await expect(page.getByRole('option', { name: '全部状态' })).toBeFocused()
  await page.keyboard.press('ArrowDown')
  await expect(page.getByRole('option', { name: '待支付' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(status).toHaveAttribute('data-value', 'pending')
  const calendar = page.getByRole('button', { name: '选择日期时间', exact: true }).first()
  await calendar.focus()
  await page.keyboard.press('Enter')
  const dialog = page.getByRole('dialog', { name: '选择日期时间' })
  const day = dialog.getByRole('button', { pressed: true })
  await day.focus()
  await page.keyboard.press('ArrowRight')
  await expect(day).toBeFocused()
  const time = dialog.getByLabel('时间', { exact: true })
  await time.fill('12:34:56')
  await time.press('Tab')
  await page.keyboard.press('Enter')
  await expect(start).toHaveValue(/ 12:34:56$/)
  await page.getByRole('button', { name: '应用筛选' }).click()
  await expect(page).toHaveURL(/status=pending/)
  const expected = await start.evaluate((element: HTMLInputElement) => new Date(element.value.replace(' ', 'T')).toISOString())
  expect(new URL(page.url()).searchParams.get('start')).toBe(expected)
  await page.reload()
  await expect(start).toHaveValue(/ 12:34:56$/)
  await expect(status).toHaveAttribute('data-value', 'pending')
  await status.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('option', { name: '待支付' })).toBeFocused()
  await page.keyboard.press('ArrowUp')
  await expect(page.getByRole('option', { name: '全部状态' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(status).toHaveAttribute('data-value', '')
  const clear = page.getByRole('button', { name: '清空日期时间', exact: true })
  await clear.focus()
  await page.keyboard.press('Enter')
  await expect(start).toBeFocused()
  await expect(start).toHaveValue('')
  await page.getByRole('button', { name: '应用筛选' }).click()
  await expect(page).toHaveURL(/\/orders$/)
  await noOverflow(page)

  if (screenshots) {
    for (const theme of ['light', 'dark'] as const) {
      await selectScreenshotTheme(page, theme)
      await status.click()
      await capture(page, 'orders-select-open')
      await page.keyboard.press('Escape')
      await calendar.click()
      await capture(page, 'orders-date-time-open')
      await page.keyboard.press('Escape')
    }
  }
})

test('shows homepage entry points according to registration and merchant session', async ({ page, account }) => {
  let registrationOpen = false
  let customHome = false
  await page.route('**/api/v1/public/config', async (route) => {
    const response = await route.fetch()
    const body = await response.json() as { data: components['schemas']['PublicConfig'] }
    body.data.registration_open = registrationOpen
    if (customHome) body.data.home = { mode: 'html', html: '<h1>自定义首页</h1>', url: null }
    await route.fulfill({ response, json: body })
  })
  await page.goto('/')
  await expect(page.getByRole('link', { name: '登录', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: '注册', exact: true })).toHaveCount(0)
  registrationOpen = true
  await page.reload()
  await expect(page.getByRole('link', { name: '注册', exact: true })).toHaveAttribute('href', '/register')
  await noOverflow(page)

  if (screenshots) {
    for (const [theme, label] of [['light', '浅色'], ['dark', '深色']] as const) {
      await chooseMenu(page, '主题', label)
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      await polishShot(page, 'home-signed-out', false, 'sidebar-nav-shots')
      for (const menu of ['语言', '主题']) {
        await page.getByRole('button', { name: menu, exact: true }).click()
        await expect(page.getByRole('menu')).toBeVisible()
        await capture(page, menu === '语言' ? 'language-menu-open' : 'theme-menu-open')
        await page.keyboard.press('Escape')
      }
    }
  }

  customHome = true
  await page.reload()
  await expect(page.frameLocator('iframe').getByRole('heading', { name: '自定义首页' })).toBeVisible()
  await expect(page.getByRole('link', { name: '登录', exact: true })).toBeVisible()
  const header = (await page.getByRole('banner').boundingBox())!
  const iframe = (await page.locator('iframe').boundingBox())!
  expect(iframe.y).toBeGreaterThanOrEqual(Math.floor(header.y + header.height))
  await noOverflow(page)
  await capture(page, 'home-html-topbar')

  await page.getByRole('link', { name: '登录', exact: true }).click()
  await login(page, account)
  await expect(page).toHaveURL(/\/dashboard$/)
  for (const html of [true, false]) {
    customHome = html
    await page.goto('/')
    const bar = page.getByRole('banner')
    const dashboard = bar.getByRole('link', { name: '进入后台' })
    await expect(dashboard).toHaveAttribute('href', '/dashboard')
    await expect(dashboard).not.toHaveClass(/\bbg-primary\b/)
    await expect(bar.getByRole('link', { name: '注册', exact: true })).toHaveCount(0)
    const action = (await dashboard.boundingBox())!
    for (const label of ['语言', '主题']) {
      const control = (await bar.getByRole('button', { name: label, exact: true }).boundingBox())!
      expect(control.x + control.width).toBeLessThan(action.x)
      expect(control.height).toBe(action.height)
    }
    await noOverflow(page)
  }
  if (screenshots) {
    for (const [theme, label] of [['light', '浅色'], ['dark', '深色']] as const) {
      await chooseMenu(page, '主题', label)
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      await polishShot(page, 'home-signed-in', false, 'sidebar-nav-shots')
    }
  }
})
