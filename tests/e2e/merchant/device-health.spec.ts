import { expect, test, type Page } from '@playwright/test'
import type { components } from '../../../web/src/api/schema'
import { boundDevice, emptyStats } from '../../frontend/merchant/fixtures'
import { defaultPublicConfig } from '../../frontend/public-config-fixture'
import { merchantVisualShots, noOverflow, polishShot, selectScreenshotTheme } from './fixtures'

type Schema = components['schemas']
const screenshotDirectory = 'device-health-shots'
const createdAt = '2026-11-05T00:00:00Z'
const merchant: Schema['AuthUserDTO'] = {
  id: '42', role: 'merchant', username: 'fixture_shop', email: 'shop@example.test', pid: '100042',
  banned: false, banned_at: null, ban_reason: null, totp_enabled: false,
  locale: 'zh-CN', theme: 'system', created_at: createdAt,
}
const emptyPage = { items: [], total: 0, next_cursor: null }

async function mockSession(page: Page, role: 'merchant' | 'console', user: Schema['AuthUserDTO']) {
  const session: Schema['SessionIdentity'] = {
    user: { ...user }, csrf_token: crypto.randomUUID(), expires_at: new Date(Date.now() + 3600_000).toISOString(),
  }
  await page.route('**/api/v1/public/config', (route) => route.fulfill({ json: { data: defaultPublicConfig } }))
  await page.route(`**/api/v1/${role}/me`, (route) => route.fulfill({ json: { data: session } }))
  await page.route(`**/api/v1/${role}/preferences`, (route) => {
    const preferences = route.request().postDataJSON() as Schema['Preferences']
    Object.assign(session.user, preferences)
    return route.fulfill({ json: { data: { locale: session.user.locale, theme: session.user.theme } } })
  })
}

test('shows device health on the merchant devices page and overview', async ({ page }) => {
  const now = new Date()
  const at = (secondsAgo: number) => new Date(now.getTime() - secondsAgo * 1000)
  let device = boundDevice('abnormal', at(120))
  await page.clock.setFixedTime(now)
  await mockSession(page, 'merchant', merchant)
  await page.route('**/api/v1/merchant/device', (route) => route.fulfill({ json: { data: device } }))
  await page.route(/\/api\/v1\/merchant\/(?:orders|receipts)(?:\?|$)/, (route) => route.fulfill({ json: { data: emptyPage } }))
  await page.route(/\/api\/v1\/merchant\/stats(?:\?|$)/, (route) => route.fulfill({ json: { data: emptyStats() } }))
  await page.route(/\/api\/v1\/merchant\/notifications(?:\?|$)/, (route) => route.fulfill({
    json: { data: { ...emptyPage, unread_count: 0, popup_announcements: [] } },
  }))
  await page.route('**/downloads/latest.json', (route) => route.fulfill({ status: 404 }))

  await page.goto('/devices')
  const details = page.getByRole('region', { name: '设备状态' })
  await expect(details.getByText('异常', { exact: true }).locator('.bg-dot-wait')).toBeVisible()
  await expect(details.locator('time')).toHaveAttribute('datetime', device.last_seen_at!)
  await expect(details.getByText(/连续 90 秒未收到心跳.*达到 300 秒.*站内通知.*邮件提醒.*连续 3 次/)).toBeVisible()
  await noOverflow(page)
  await merchantVisualShots(page, () => polishShot(page, 'merchant-device-abnormal', true, screenshotDirectory))

  await page.goto('/dashboard')
  const overview = page.getByRole('region', { name: '监控设备', exact: true })
  await expect(overview.getByText('异常', { exact: true }).locator('.bg-dot-wait')).toBeVisible()
  await expect(overview).toContainText('心跳 2分钟前')
  await merchantVisualShots(page, () => polishShot(page, 'merchant-overview-abnormal', true, screenshotDirectory))

  device = boundDevice('offline', at(360))
  await page.reload()
  await expect(overview.getByText('离线', { exact: true })).toBeVisible()
  await expect(overview).toContainText('离线于 1分钟前')
  await merchantVisualShots(page, () => polishShot(page, 'merchant-overview-offline', true, screenshotDirectory))

  device = boundDevice('abnormal', at(5))
  await page.reload()
  await expect(overview.getByText('异常', { exact: true })).toBeVisible()
  await expect(overview).toContainText('心跳 5秒钟前')

  device = boundDevice('online', at(5))
  await page.reload()
  await expect(overview.getByText('在线', { exact: true })).toBeVisible()
  await noOverflow(page)
})

test('shows device health on the console merchant list and detail', async ({ page }) => {
  const admin: Schema['AuthUserDTO'] = {
    id: '1', role: 'admin', username: 'fixture_admin', banned: false, banned_at: null,
    ban_reason: null, totp_enabled: false, locale: 'zh-CN', theme: 'system', created_at: createdAt,
  }
  const device = boundDevice('abnormal', new Date(Date.now() - 5000))
  const subscription: Schema['ConsoleMerchantDetail']['subscription'] = {
    plan: null, channels: [], expires_at: null, source: 'manual', revision: 0, active: false,
  }
  const approval: Schema['ConsoleMerchantDetail']['approval'] = {
    approval_state: 'approved', approval_reason: null, approval_reviewed_at: null,
  }
  const detail: Schema['ConsoleMerchantDetail'] = {
    user: merchant, device, subscription, approval,
    collection: {
      version: 1, order_ttl_seconds: null, float_limit_cents: null, float_direction: null,
      bounds: { ttl_max_seconds: 600, float_min_cents: 1, float_max_cents: 50 },
      effective: { order_ttl_seconds: 300, float_limit_cents: 10, float_direction: 'up' },
    },
  }
  const row: Schema['ConsoleMerchant'] = { ...merchant, device, subscription, ...approval }
  await mockSession(page, 'console', admin)
  await page.route(/\/api\/v1\/console\/merchants(?:\?|$)/, (route) => route.fulfill({
    json: { data: { ...emptyPage, items: [row], total: 1 } },
  }))
  await page.route('**/api/v1/console/merchants/42', (route) => route.fulfill({ json: { data: detail } }))

  for (const [path, shot] of [
    ['/console/merchants', 'console-merchant-list-abnormal'],
    ['/console/merchants/42', 'console-merchant-detail-abnormal'],
  ] as const) {
    await page.goto(path)
    await expect(page.getByText('异常', { exact: true }).filter({ visible: true }).locator('.bg-dot-wait')).toBeVisible()
    await noOverflow(page)
    if (process.env.WEB_VISUAL_POLISH === '1' && [360, 1280].includes(page.viewportSize()!.width)) {
      for (const theme of ['light', 'dark'] as const) {
        await selectScreenshotTheme(page, theme, 'console')
        await polishShot(page, shot, true, screenshotDirectory)
      }
    }
  }
})
