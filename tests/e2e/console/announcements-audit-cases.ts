import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
import type { components } from '../../../web/src/api/schema'
import { login, polishShot } from '../merchant/fixtures'

type Announcement = components['schemas']['PortalAnnouncementDTO']
type Account = Parameters<typeof login>[1]
type Helpers = {
  seed: (request: APIRequestContext) => Promise<Omit<Account, 'pid'>>
  captcha: (page: Page, token: string) => Promise<void>
  signIn: (page: Page, admin: Omit<Account, 'pid'>, redirect?: string) => Promise<void>
  control: (page: Page, name: string) => Promise<void>
}
const announcements = '/api/v1/console/announcements'
const initialTitle = '平台结算服务维护通知'
const updatedTitle = '平台结算服务维护时间调整'
const body = '平台将于周日凌晨进行例行维护。\n维护期间可继续查看历史订单，结算服务将在维护结束后恢复。'

async function createForm(page: Page) {
  await page.getByRole('main').getByRole('button', { name: '新建公告', exact: true }).first().click()
  const dialog = page.getByRole('dialog', { name: '新建公告' })
  await dialog.getByLabel('中文标题').fill(initialTitle)
  await dialog.getByLabel('中文正文').fill(body)
  await dialog.getByRole('checkbox', { name: '登录后弹窗提醒' }).check()
  return dialog
}

function announcementRow(page: Page, title: string) {
  const label = '公告'
  return (page.viewportSize()!.width < 768
    ? page.getByRole('list', { name: label, exact: true }).getByRole('listitem')
    : page.getByRole('table', { name: label, exact: true }).getByRole('row')
  ).filter({ hasText: title })
}

export function registerAnnouncementAuditTests({ seed, captcha, signIn, control }: Helpers) {
  async function shots(page: Page, state: string, prepare?: () => Promise<void>) {
    for (const width of [360, 1280]) {
      await page.setViewportSize({ width, height: width === 360 ? 780 : 800 })
      for (const [theme, label] of [['light', '浅色'], ['dark', '深色']] as const) {
        const saved = page.waitForResponse((response) =>
          response.url().endsWith('/console/preferences') && response.request().method() === 'PATCH',
        )
        await control(page, label)
        expect((await saved).status()).toBe(200)
        await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
        await prepare?.()
        await polishShot(page, state, !prepare, 'console-announcements-audit-shots')
        if (prepare) {
          await page.keyboard.press('Escape')
          await expect(page.getByRole('dialog')).toHaveCount(0)
        }
      }
    }
  }

  test('publishes, edits and removes a real merchant popup and filters its audit trail', {
    tag: '@global-announcement',
  }, async ({ page, request, browser }) => {
    test.setTimeout(150000)
    const admin = await seed(request)
    await captcha(page, admin.turnstileToken)
    await signIn(page, admin, '/console/announcements')
    await expect(page.getByRole('heading', { name: '公告', exact: true })).toBeVisible()

    const accountResponse = await request.post('/__e2e/merchant', { data: {} })
    expect(accountResponse.ok()).toBe(true)
    const account = await accountResponse.json() as Account
    const origin = new URL(page.url()).origin
    const merchantContext = await browser.newContext({ baseURL: origin })
    const merchant = await merchantContext.newPage()
    await captcha(merchant, account.turnstileToken)
    async function signInMerchant() {
      await merchantContext.clearCookies()
      await merchant.goto('/login?redirect=%2Fnotifications')
      const summary = merchant.waitForResponse((response) =>
        new URL(response.url()).pathname === '/api/v1/merchant/notifications' && new URL(response.url()).searchParams.get('limit') === '1',
      )
      await login(merchant, account)
      await expect(merchant).toHaveURL(/\/notifications$/)
      const response = await summary
      expect(response.status()).toBe(200)
      return (await response.json() as { data: components['schemas']['PortalNotificationPage'] }).data.popup_announcements ?? []
    }

    let live: Announcement | undefined
    let csrf = ''
    let edited: Announcement
    try {
      const dialog = await createForm(page)
      const creating = page.waitForResponse((response) => response.url().endsWith(announcements) && response.request().method() === 'POST')
      await dialog.getByRole('button', { name: '发布公告' }).click()
      const created = await creating
      expect(created.status()).toBe(201)
      const first = (await created.json() as { data: Announcement }).data
      live = first
      const headers = await created.request().allHeaders()
      csrf = headers['x-csrf-token'] ?? ''
      expect(headers.origin).toBe(origin)
      expect(csrf).toBeTruthy()
      await expect(page.getByText('公告已发布。')).toBeVisible()

      const firstSummary = await signInMerchant()
      const published = firstSummary.find((item) => item.id === first.id)
      expect(published).toBeDefined()
      const popup = merchant.getByRole('dialog', { name: initialTitle })
      await expect(popup).toContainText('平台将于周日凌晨进行例行维护。')
      const reading = merchant.waitForResponse((response) => response.url().endsWith(`/announcements/${first.id}/read`))
      await popup.getByRole('button', { name: '知道了' }).click()
      const read = await reading
      expect(read.status()).toBe(200)
      expect(read.request().postDataJSON()).toEqual({ revision: first.revision })
      await expect(popup).toHaveCount(0)

      await announcementRow(page, initialTitle).getByRole('button', { name: '编辑' }).click()
      const edit = page.getByRole('dialog', { name: '编辑公告' })
      await expect(edit.getByText(/商户将再次看到弹窗/)).toBeVisible()
      await edit.getByLabel('中文标题').fill(updatedTitle)
      const target = `${announcements}/${first.id}`
      const updating = page.waitForResponse((response) => response.url().endsWith(target) && response.request().method() === 'PUT')
      await edit.getByRole('button', { name: '保存修改' }).click()
      const updated = await updating
      expect(updated.status()).toBe(200)
      edited = (await updated.json() as { data: Announcement }).data
      live = edited
      expect(edited.revision).toBe(first.revision + 1)
      expect(edited.published_at).toBe(published!.published_at)
      await expect(page.getByText('公告已保存。')).toBeVisible()

      await signInMerchant()
      await expect(merchant.getByRole('dialog', { name: updatedTitle })).toBeVisible()
      await announcementRow(page, updatedTitle).getByRole('button', { name: '删除' }).click()
      const deleting = page.waitForResponse((response) => response.url().endsWith(target) && response.request().method() === 'DELETE')
      await page.getByRole('dialog', { name: '删除公告' }).getByRole('button', { name: '删除', exact: true }).click()
      expect((await deleting).status()).toBe(200)
      live = undefined
      await expect(page.getByText('公告已删除。')).toBeVisible()

      const remaining = await signInMerchant()
      expect(remaining.some((item) => item.id === first.id)).toBe(false)
      await expect(merchant.getByRole('dialog', { name: updatedTitle })).toHaveCount(0)
    } finally {
      try {
        if (live) {
          const cleanup = await page.request.delete(`${announcements}/${live.id}`, {
            headers: { Origin: origin, 'X-CSRF-Token': csrf },
            data: { revision: live.revision },
          })
          expect(cleanup.ok(), 'remove the global test popup even if a browser assertion fails').toBe(true)
        }
      } finally {
        await merchantContext.close()
      }
    }

    await page.goto('/console/audit-logs')
    await expect(page.getByRole('heading', { name: '审计日志' })).toBeVisible()
    async function filter(action: 'announcement.save' | 'announcement.delete') {
      await page.getByLabel('操作类型').selectOption(action)
      await page.getByLabel('操作者角色').selectOption('admin')
      const loading = page.waitForResponse((response) => {
        const url = new URL(response.url())
        return url.pathname.endsWith('/console/audit-logs') && url.searchParams.get('action') === action
      })
      await page.getByRole('button', { name: '应用筛选' }).click()
      const response = await loading
      expect(response.status()).toBe(200)
      const { data } = await response.json() as { data: components['schemas']['AuditsPage'] }
      return data.items?.filter((item) => item.target_id === edited.id) ?? []
    }
    const deleted = await filter('announcement.delete')
    expect(deleted.map((item) => item.action)).toEqual(['announcement.delete'])
    const saves = await filter('announcement.save')
    expect(saves.map((item) => item.action)).toEqual(['announcement.save', 'announcement.save'])
    const auditTable = page.getByRole('table', { name: '审计日志' })
    for (const entry of saves) await expect(auditTable.getByText(entry.request_id, { exact: true })).toBeVisible()
    await shots(page, 'audit-list')
    await shots(page, 'audit-detail-dialog', async () => {
      const records = page.viewportSize()!.width < 768
        ? page.getByRole('list', { name: '操作记录卡片' })
        : page.getByRole('table', { name: '审计日志' })
      await records.getByRole('button', { name: '详情' }).first().click()
      await expect(page.getByRole('dialog', { name: '操作详情' })).toContainText('管理员')
    })

    // Render visual fixtures after the real lifecycle so the global popup stays live for the shortest time.
    let sample: Announcement | undefined
    await page.route('**/api/v1/console/announcements?*', (route) =>
      route.fulfill({ json: { data: { items: sample ? [sample] : [], next_cursor: null, total: sample ? 1 : 0 } } }),
    )
    await page.goto('/console/announcements')
    await expect(page.getByText('还没有公告')).toBeVisible()
    await shots(page, 'announcements-empty')
    await shots(page, 'announcement-create-dialog', async () => { await createForm(page) })
    sample = edited
    await page.reload()
    await expect(announcementRow(page, updatedTitle)).toBeVisible()
    await shots(page, 'announcements-list')
    await shots(page, 'announcement-edit-dialog', async () => {
      await announcementRow(page, updatedTitle).getByRole('button', { name: '编辑' }).click()
      const warning = page.getByRole('dialog').getByText(/商户将再次看到弹窗/)
      await expect(warning).toBeVisible()
      await warning.scrollIntoViewIfNeeded()
    })
    await shots(page, 'announcement-delete-dialog', async () => {
      await announcementRow(page, updatedTitle).getByRole('button', { name: '删除' }).click()
      await expect(page.getByRole('dialog', { name: '删除公告' })).toBeVisible()
    })
    await page.route('**' + announcements + '/' + edited.id, (route) => {
      sample = {
        ...edited,
        revision: (sample?.revision ?? edited.revision) + 1,
        title: { ...edited.title, en: 'Settlement maintenance' },
        body: { zh_CN: '维护改至周一凌晨。', en: 'Maintenance moves to early Monday.' },
        popup: route.request().method() !== 'DELETE',
      }
      return route.fulfill({ status: 409, json: { error: { code: 'VERSION_CONFLICT', message: 'version conflict' } } })
    })
    await shots(page, 'announcement-conflict-dialog', async () => {
      await announcementRow(page, updatedTitle).getByRole('button', { name: '编辑' }).click()
      const dialog = page.getByRole('dialog', { name: '编辑公告' })
      await dialog.getByRole('button', { name: '保存修改' }).click()
      const conflict = dialog.getByText('公告已被其他管理员修改，已刷新为最新内容')
      await expect(conflict).toBeVisible()
      await conflict.scrollIntoViewIfNeeded()
      await expect(dialog.getByRole('button', { name: '保存修改' })).toBeDisabled()
    })
    await shots(page, 'announcement-delete-conflict-dialog', async () => {
      await announcementRow(page, updatedTitle).getByRole('button', { name: '删除' }).click()
      const dialog = page.getByRole('dialog', { name: '删除公告' })
      const remove = dialog.getByRole('button', { name: '删除', exact: true })
      await remove.click()
      await expect(dialog.getByText('公告已被其他管理员修改，已刷新为最新内容')).toBeInViewport()
      await expect(dialog.getByText('维护改至周一凌晨。')).toBeInViewport()
      await expect(dialog.getByText('Maintenance moves to early Monday.')).toBeInViewport()
      await expect(dialog.getByText('未开启', { exact: true })).toBeInViewport()
      await expect(dialog.getByRole('checkbox', { name: '我已查看最新内容' })).not.toBeChecked()
      await expect(remove).toBeDisabled()
    })
  })
}
