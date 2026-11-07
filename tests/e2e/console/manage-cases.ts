import { test, expect, type Page, type APIRequestContext } from '@playwright/test'
import { login, noOverflow, polishShot } from '../merchant/fixtures'

type Admin = { username: string; password: string; pid?: string; totpSeed: string; turnstileToken: string }
type Helpers = {
  seed: (request: APIRequestContext) => Promise<Admin>
  captcha: (page: Page, token: string) => Promise<void>
  signIn: (page: Page, admin: Admin, redirect?: string) => Promise<void>
  control: (page: Page, name: string) => Promise<void>
}
/** Registered inside console.spec.ts's single serial suite: no second admin worker. */
export function registerManagementTests({ seed, captcha, signIn, control }: Helpers) {
  async function shots(page: Page, state: string, prepare?: () => Promise<void>) {
    for (const width of [360, 1280]) {
      await page.setViewportSize({ width, height: width === 360 ? 780 : 800 })
      for (const [theme, label] of [['light', '浅色'], ['dark', '深色']] as const) {
        const saved = page.waitForResponse((response) => response.url().endsWith('/console/preferences') && response.request().method() === 'PATCH')
        await control(page, label)
        expect((await saved).status()).toBe(200)
        await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
        if (prepare) await prepare()
        await polishShot(page, state, !prepare, 'console-manage-shots')
        if (prepare) await page.getByRole('dialog').getByRole('button', { name: '取消', exact: true }).click()
      }
    }
  }
  test('manages merchants, subscriptions and plans, and reads console orders and purchases', async ({ page, request }) => {
    test.setTimeout(180000)
    const admin = await seed(request); await captcha(page, admin.turnstileToken)
    await signIn(page, admin)
    await expect(page).toHaveURL(/\/console$/)
    const suffix = crypto.randomUUID().slice(0, 8)
    const planName = '标准收款套餐 ' + suffix
    await page.goto('/console/plans')
    await expect(page.getByRole('heading', { name: '套餐', exact: true })).toBeVisible()
    const planForm = async () => {
      await page.getByRole('button', { name: '新建套餐', exact: true }).click()
      const dialog = page.getByRole('dialog')
      await dialog.getByLabel('中文名称').fill(planName)
      await dialog.getByLabel('英文名称').fill('Standard')
      await dialog.getByLabel('价格（元）').fill('29.99')
      await dialog.getByLabel('时长（天）').fill('30')
      await dialog.getByRole('checkbox', { name: '支付宝', exact: true }).check()
      await dialog.getByRole('checkbox', { name: '微信支付', exact: true }).check()
    }
    await shots(page, 'create-plan-dialog', () => planForm())
    await planForm()
    const creatingPlan = page.waitForResponse((response) => response.url().endsWith('/console/plans') && response.request().method() === 'POST')
    await page.getByRole('dialog').getByRole('button', { name: '新建套餐', exact: true }).click()
    const createdPlan = await creatingPlan
    expect(createdPlan.status()).toBe(201)
    const { data: plan } = await createdPlan.json() as { data: { id: string; revision: number } }
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await shots(page, 'plans')
    // Use the responsive card action at 360; the table row action at desktop.
    const editPlan = async () => {
      const row = page.viewportSize()!.width < 768
        ? page.getByRole('list', { name: '套餐', exact: true }).getByRole('listitem').filter({ hasText: planName })
        : page.getByRole('row').filter({ hasText: planName })
      await row.getByRole('button', { name: '编辑', exact: true }).click()
      await page.getByRole('dialog').getByLabel('价格（元）').fill('39.99')
    }
    await shots(page, 'edit-plan-dialog', editPlan); await editPlan()
    const updatingPlan = page.waitForResponse((response) => response.url().endsWith('/console/plans/' + plan.id) && response.request().method() === 'PUT')
    await page.getByRole('dialog').getByRole('button', { name: '编辑套餐' }).click()
    const updatedPlan = await updatingPlan
    expect(updatedPlan.status()).toBe(200)
    const { data: edited } = await updatedPlan.json() as { data: { revision: number } }
    await expect(page.getByRole('dialog')).toHaveCount(0)

    await page.goto('/console/merchants')
    const username = 'pine_shop_' + suffix, password = crypto.randomUUID(), newPassword = crypto.randomUUID()
    const createMerchant = async () => {
      await page.getByRole('button', { name: '新建商户', exact: true }).click()
      const dialog = page.getByRole('dialog')
      await dialog.getByLabel('用户名', { exact: true }).fill(username)
      await dialog.getByLabel('邮箱', { exact: true }).fill(username + '@example.test')
      await dialog.getByLabel('初始密码', { exact: true }).fill(password)
      await dialog.getByLabel('确认密码', { exact: true }).fill(password)
    }
    await shots(page, 'create-merchant-dialog', createMerchant); await createMerchant()
    const creating = page.waitForResponse((response) => response.url().endsWith('/console/merchants') && response.request().method() === 'POST')
    await page.getByRole('dialog').getByRole('button', { name: '新建商户' }).click()
    const created = await creating
    expect(created.status()).toBe(201)
    const headers = await created.request().allHeaders()
    expect(headers.origin).toBe(new URL(page.url()).origin)
    expect(headers['x-csrf-token']).toBeTruthy()
    const { data: merchant } = await created.json() as { data: { id: string; pid: string } }
    await expect(page).toHaveURL(new RegExp('/console/merchants/' + merchant.id + '$'))
    const grant = async () => {
      await page.getByRole('button', { name: '调整订阅', exact: true }).click()
      const dialog = page.getByRole('dialog')
      await dialog.getByLabel('套餐', { exact: true }).selectOption(plan.id)
      await dialog.getByLabel('到期时间', { exact: true }).fill('2030-01-01T12:00')
      await dialog.getByLabel('原因', { exact: true }).fill('为商户开通标准收款服务')
    }
    await shots(page, 'subscription-dialog', grant); await grant()
    await page.getByRole('dialog').getByRole('button', { name: '调整订阅' }).click()
    await expect(page.getByRole('dialog')).toHaveCount(0)
    await shots(page, 'merchant-detail')

    const pendingTrade = crypto.randomUUID(), paidTrade = crypto.randomUUID(), at = new Date().toISOString()
    const seeded = await request.post('/__e2e/orders', { data: { pid: merchant.pid, orders: [
      { trade_no: pendingTrade, name: '待付款商品', channel: 'alipay', status: 'pending', original_cents: 2999, actual_cents: 2999, created_at: at },
      { trade_no: paidTrade, name: '收款服务订单', channel: 'alipay', status: 'paid', original_cents: 2999, actual_cents: 2999, created_at: at, paid_at: at, notify_url: 'https://merchant.example.test/notify', callback_state: 'failed' },
    ] } })
    expect(seeded.status()).toBe(204)
    const ban = async () => {
      await page.getByRole('button', { name: '停用商户', exact: true }).click()
      await page.getByRole('dialog').getByLabel('原因', { exact: true }).fill('商户申请暂停服务')
    }
    await shots(page, 'ban-dialog', ban); await ban()
    await page.getByRole('dialog').getByRole('button', { name: '停用商户' }).click()
    await expect(page.getByText('已停用商户，已关闭 1 笔待支付订单。')).toBeVisible()
    const unban = async () => { await page.getByRole('button', { name: '恢复商户', exact: true }).click() }
    await shots(page, 'unban-dialog', unban); await unban()
    await page.getByRole('dialog').getByRole('button', { name: '恢复商户' }).click()
    await expect(page.getByText('已恢复商户。')).toBeVisible()
    const reset = async () => {
      await page.getByRole('button', { name: '重置密码', exact: true }).click()
      const dialog = page.getByRole('dialog')
      await dialog.getByLabel('新密码', { exact: true }).fill(newPassword)
      await dialog.getByLabel('确认密码', { exact: true }).fill(newPassword)
    }
    await shots(page, 'reset-password-dialog', reset); await reset()
    await page.getByRole('dialog').getByRole('button', { name: '重置密码' }).click()
    await expect(page.getByText('密码已重置。')).toBeVisible()
    await page.goto('/console/merchants?query=' + username)
    await expect(page.getByRole('link', { name: '管理', exact: true })).toBeVisible()
    await shots(page, 'merchants')
    await page.goto('/console/orders?query=' + paidTrade)
    await expect(page.getByRole('link', { name: paidTrade, exact: true })).toBeVisible()
    await shots(page, 'orders')
    await page.getByRole('link', { name: paidTrade, exact: true }).click()
    await expect(page.getByRole('heading', { name: '订单详情' })).toBeVisible()
    await expect(page.getByRole('button', { name: /补单|退款|重发回调/ })).toHaveCount(0)
    await shots(page, 'order-detail')

    expect((await request.post('/__e2e/subscription', { data: { pid: merchant.pid } })).status()).toBe(200)
    // A separate browser context verifies the reset password and creates a real purchase.
    const buyer = await page.context().browser()!.newContext({ baseURL: new URL(page.url()).origin })
    try {
      const buyerPage = await buyer.newPage(); await captcha(buyerPage, admin.turnstileToken)
      await buyerPage.goto('/login')
      await login(buyerPage, { username, password: newPassword, pid: merchant.pid, totpSeed: '', turnstileToken: admin.turnstileToken })
      await expect(buyerPage).toHaveURL(/\/dashboard$/)
      const identity = (await (await buyer.request.get('/api/v1/merchant/me')).json() as { data: { csrf_token: string } }).data
      const subscription = (await (await buyer.request.get('/api/v1/merchant/subscription')).json() as { data: { current: { revision: number } } }).data.current
      const purchase = await buyer.request.post('/api/v1/merchant/subscription/purchases', {
        headers: { Origin: new URL(page.url()).origin, 'X-CSRF-Token': identity.csrf_token, 'Idempotency-Key': crypto.randomUUID() },
        data: { plan_id: plan.id, plan_revision: edited.revision, expected_subscription_revision: subscription.revision, confirm_replace: false },
      })
      expect(purchase.status()).toBe(201)
    } finally { await buyer.close() }
    await page.goto('/console/merchants/' + merchant.id)
    await page.getByRole('link', { name: '查看该商户购买记录' }).click()
    await expect(page).toHaveURL(new RegExp('merchant_id=' + merchant.id))
    await page.getByLabel('套餐', { exact: true }).selectOption(plan.id)
    await page.getByLabel('状态', { exact: true }).selectOption('pending')
    await page.getByRole('button', { name: '应用筛选' }).click()
    await expect(page.getByRole('link', { name: '#' + merchant.id, exact: true })).toBeVisible()
    await shots(page, 'purchases')
    await page.getByRole('button', { name: '清除商户筛选' }).click()
    await expect(page).not.toHaveURL(/merchant_id=/)

    await page.goto('/console/plans')
    const remove = async () => {
      const row = page.viewportSize()!.width < 768
        ? page.getByRole('list', { name: '套餐', exact: true }).getByRole('listitem').filter({ hasText: planName })
        : page.getByRole('row').filter({ hasText: planName })
      await row.getByRole('button', { name: '删除', exact: true }).click()
    }
    await shots(page, 'delete-plan-dialog', remove); await remove()
    await page.getByRole('dialog').getByRole('button', { name: '删除套餐' }).click()
    await expect(page.getByText('已删除套餐，已取消 1 笔待支付购买。')).toBeVisible()
    await page.getByRole('checkbox', { name: '显示已删除' }).check()
    const row = page.getByRole('row').filter({ hasText: planName })
    await expect(row.getByText('已删除').first()).toBeVisible()
    await expect(row.getByRole('button')).toHaveCount(0)
    await page.setViewportSize({ width: 360, height: 780 }); await noOverflow(page)
  })
}
