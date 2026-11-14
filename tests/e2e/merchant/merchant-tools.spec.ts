import { prepareCheckout } from '../checkout/fixtures'
import { clickControl, expect, login, merchantVisualShots, polishShot, test } from './fixtures'

test('saves callback privacy and creates a replacement test payment', async ({ page, request, account }) => {
  const fixture = await prepareCheckout(request)
  await page.goto('/login?redirect=/integration')
  await login(page, { ...account, username: fixture.username, password: fixture.password })

  const callback = page.getByRole('region', { name: '回调设置' })
  const hideName = callback.getByRole('checkbox', { name: '回调中隐藏商品名称' })
  await hideName.check()
  await callback.getByRole('button', { name: '保存回调设置' }).click()
  await expect(callback.getByText('已保存回调设置。')).toBeVisible()
  await page.reload()
  await expect(hideName).toBeChecked()

  const card = page.getByRole('region', { name: '测试支付' })
  await card.getByLabel('金额（元）').fill('1.25')
  const create = async () => {
    const response = page.waitForResponse((item) => item.url().endsWith('/merchant/test-payment') && item.request().method() === 'POST')
    await card.getByRole('button', { name: '创建测试订单' }).click()
    expect((await response).status()).toBe(201)
    await expect(card.getByRole('link', { name: '打开收银台' })).toHaveAttribute('href', /\/pay\//)
    return await card.getByRole('link', { name: '查看订单' }).getAttribute('href')
  }
  const first = await create()
  const second = await create()
  expect(first).toBeTruthy()
  expect(second).not.toBe(first)

  await merchantVisualShots(page, async () => {
    await callback.scrollIntoViewIfNeeded()
    await polishShot(page, 'callback-settings', false, 'merchant-tools-shots')
    await card.scrollIntoViewIfNeeded()
    await polishShot(page, 'test-payment', false, 'merchant-tools-shots')
  })

  await card.getByRole('link', { name: '查看订单' }).click()
  await expect(page.getByText('测试', { exact: true })).toBeVisible()
  await page.goto(first!)
  await expect(page.getByText('测试', { exact: true })).toBeVisible()
  await expect(page.getByText('订单已关闭', { exact: true })).toBeVisible()
})

test.describe('registration review waiting page', () => {
  test.use({ pendingReview: true })

  test('allows login and redirects protected routes to the waiting page', async ({ page, account }) => {
    await page.goto('/login?redirect=/orders')
    await login(page, account)
    await expect(page).toHaveURL(/\/registration-review$/)
    await expect(page.getByRole('heading', { name: '注册审核中' })).toBeVisible()
    await expect(page.getByRole('navigation', { name: '商户导航' })).toHaveCount(0)
    await page.goto('/integration')
    await expect(page).toHaveURL(/\/registration-review$/)
    const response = page.waitForResponse((item) => item.url().endsWith('/merchant/me'))
    await page.getByRole('button', { name: '刷新审核状态' }).click()
    expect((await response).status()).toBe(200)
    await merchantVisualShots(page, async () => { await polishShot(page, 'registration-review', true, 'merchant-tools-shots') })
    await clickControl(page, '退出登录')
    await expect(page).toHaveURL(/\/login$/)
  })
})
