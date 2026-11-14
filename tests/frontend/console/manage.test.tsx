import { chooseSelect } from '../portal-controls'
import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { publicConfigHandler } from '../public-config-fixture'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import { api, consoleApi, onUnauthorized } from '@/api/client'
import { createI18n } from '@/lib/i18n'
import { bytes, planInput, validBanReason, validGrantReason, type Schema } from '@/features/console/manage-data'
import { order, unboundDevice } from '../merchant/fixtures'

const admin: Schema['SessionIdentity'] = { user: { id: '1', role: 'admin', username: 'admin', banned: false, banned_at: null, ban_reason: null, totp_enabled: false, locale: 'zh-CN', theme: 'system', created_at: new Date().toISOString() }, csrf_token: crypto.randomUUID(), expires_at: new Date(Date.now() + 3600000).toISOString() }
const plan: Schema['SubscriptionPlanDTO'] = { id: '3', revision: 2, name: { zh_CN: '专业套餐', en: 'Pro' }, channels: ['alipay', 'wxpay'], price_cents: 2999, duration_days: 30, purchase_limit: null, deleted: false }
const detail: Schema['ConsoleMerchantDetail'] = {
  user: { ...admin.user, role: 'merchant', id: '42', username: '<b>安全商户</b>', email: 'shop@example.test', pid: '100042' },
  subscription: { plan, channels: ['alipay'], expires_at: '2030-01-01T00:00:00Z', source: 'manual', revision: 5, active: true },
  device: unboundDevice,
  approval: { approval_state: 'approved', approval_reason: null, approval_reviewed_at: null },
  collection: { version: 1, order_ttl_seconds: null, float_limit_cents: null, float_direction: null,
    bounds: { float_min_cents: 0, float_max_cents: 999, ttl_max_seconds: 86400 },
    effective: { order_ttl_seconds: 300, float_limit_cents: 99, float_direction: 'up' } },
}
const merchant: Schema['ConsoleMerchant'] = { ...detail.user, subscription: detail.subscription, device: unboundDevice, approval_state: 'approved', approval_reason: null, approval_reviewed_at: null }
const success = <T,>(data: T) => HttpResponse.json({ data })
const failure = (code: Schema['ErrorCode']) => HttpResponse.json({ error: { code, message: code, fields: {}, details: {} } }, { status: 409 })
let current: Schema['ConsoleMerchantDetail'], currentPlan: Schema['SubscriptionPlanDTO']
const server = setupServer(
  publicConfigHandler,
  http.get('*/api/v1/console/me', () => success(admin)),
  http.get('*/api/v1/console/merchants', () => success({ items: [merchant], total: 1, next_cursor: null })),
  http.get('*/api/v1/console/merchants/42', () => success(current)),
  http.get('*/api/v1/console/plans', () => success({ items: [currentPlan], total: 1, next_cursor: null })),
  http.get('*/api/v1/console/purchases', () => success({ items: [], total: 0, next_cursor: null })),
)
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
beforeEach(() => { current = structuredClone(detail); currentPlan = structuredClone(plan) })
afterEach(() => { server.resetHandlers(); vi.restoreAllMocks() })
afterAll(() => server.close())
function mount(path = '/console/merchants') {
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) }), client = createQueryClient()
  const view = render(<AppProviders i18n={createI18n()} queryClient={client}><RouterProvider router={router} context={{ queryClient: client }} /></AppProviders>)
  return { router, client, ...view }
}
async function open(name: string) {
  fireEvent.click((await screen.findAllByRole('button', { name }))[0]!)
  return screen.findByRole('dialog', { name })
}
function fill(dialog: HTMLElement, name: string, value: string) {
  fireEvent.change(within(dialog).getByLabelText(name, { exact: true }), { target: { value } })
}
function submit(dialog: HTMLElement, name: string) { fireEvent.click(within(dialog).getByRole('button', { name })) }
async function grant() {
  const dialog = await open('调整订阅')
  await chooseSelect(userEvent.setup(), within(dialog).getByLabelText('套餐', { exact: true }), '3'); fill(dialog, '原因', '人工开通')
  return dialog
}

describe('management validation', () => {
  it('counts ban characters and subscription UTF-8 bytes separately', () => {
    expect(validBanReason('  ' + '😀'.repeat(500) + ' ')).toBe(true)
    expect(validBanReason('😀'.repeat(501))).toBe(false)
    expect(validBanReason(' ')).toBe(false)
    expect(validGrantReason('中'.repeat(166))).toBe(true)
    expect(validGrantReason('中'.repeat(167))).toBe(false)
    expect(bytes('😀')).toBe(4)
  })
  it.each([
    ['0', '30', ['alipay'], ''], ['10000000', '30', ['alipay'], ''],
    ['1.001', '30', ['alipay'], ''], ['1', '0', ['alipay'], ''],
    ['1', '36501', ['alipay'], ''], ['1', '1.1', ['alipay'], ''],
    ['1', '30', [], ''], ['1', '30', ['unknown'], ''], ['1', '30', ['alipay'], '0'],
  ])('rejects invalid plan input %s / %s / %s / %s', (price, days, selected, limit) => {
    expect(planInput('套餐', '', String(price), String(days), selected, String(limit))).toBeNull()
  })
  it('converts exact yuan and checks localized name byte limits', () => {
    expect(planInput('套餐', '', '9999999.99', '36500', ['alipay'], '')).toMatchObject({ price_cents: 999999999, purchase_limit: null })
    expect(planInput('中'.repeat(101), '', '1', '30', ['alipay'], '')).toBeNull()
    expect(planInput('套餐', 'x'.repeat(301), '1', '30', ['alipay'], '')).toBeNull()
  })
})

it('renders text safely, sends filters and restarts cursor pagination', async () => {
  const calls: URL[] = []
  server.use(http.get('*/api/v1/console/merchants', ({ request }) => {
    const url = new URL(request.url); calls.push(url)
    return success({ items: [{ ...merchant, id: url.searchParams.has('cursor') ? '43' : '42' }], total: 8, next_cursor: url.searchParams.has('cursor') ? null : 'next-page' })
  }))
  const pid = '1234567890123456'
  const { router } = mount('/console/merchants?query=' + pid), actor = userEvent.setup()
  expect((await screen.findAllByText('<b>安全商户</b>')).length).toBeGreaterThan(0)
  expect(document.querySelector('b')).toBeNull()
  expect(screen.getByLabelText('搜索')).toHaveValue(pid)
  expect(calls[0]?.searchParams.get('query')).toBe(pid)
  await actor.click(screen.getByRole('button', { name: '加载更多' }))
  await waitFor(() => expect(calls.some((url) => url.searchParams.get('cursor') === 'next-page')).toBe(true))
  const afterMore = calls.length
  await actor.click(screen.getByRole('button', { name: '应用筛选' }))
  await waitFor(() => expect(calls.length).toBeGreaterThan(afterMore))
  expect(calls.at(-1)?.searchParams.get('query')).toBe(pid)
  expect(calls.at(-1)?.searchParams.has('cursor')).toBe(false)
  await actor.click(await screen.findByRole('button', { name: '加载更多' }))
  await waitFor(() => expect(calls.at(-1)?.searchParams.get('cursor')).toBe('next-page'))
  fill(document.body, '搜索', '安全')
  await chooseSelect(actor, screen.getByLabelText('状态', { exact: true }), 'true')
  await chooseSelect(actor, screen.getByLabelText('订阅', { exact: true }), 'false')
  await actor.click(screen.getByRole('button', { name: '应用筛选' }))
  await waitFor(() => expect(router.state.location.search).toMatchObject({ query: '安全', banned: true, subscription_active: false }))
  await waitFor(() => expect(calls.at(-1)?.searchParams.get('query')).toBe('安全'))
  expect(calls.at(-1)?.searchParams.has('cursor')).toBe(false)
  await actor.click(await screen.findByRole('button', { name: '加载更多' }))
  await waitFor(() => expect(calls.at(-1)?.searchParams.get('cursor')).toBe('next-page'))
  await actor.click(screen.getByRole('button', { name: '重置筛选' }))
  await waitFor(() => expect(router.state.location.search).toEqual({}))
  await waitFor(() => expect(calls.at(-1)?.searchParams.has('query')).toBe(false))
  expect(calls.at(-1)?.searchParams.has('cursor')).toBe(false)
  expect(screen.getByLabelText('搜索')).toHaveValue('')
})

it.each(['/console/merchants', '/console/merchants/42'])('renders a subscription snapshot without a plan name on %s', async (path) => {
  const subscription = { ...detail.subscription, plan: { channels: ['alipay'] } }
  server.use(
    http.get('*/api/v1/console/merchants', () => success({ items: [{ ...merchant, subscription }], total: 1, next_cursor: null })),
    http.get('*/api/v1/console/merchants/42', () => success({ ...detail, subscription })),
  )
  mount(path)
  expect((await screen.findAllByText('无套餐')).length).toBeGreaterThan(0)
})

it.each(['USERNAME_TAKEN', 'EMAIL_TAKEN'] as const)('validates merchant creation and maps %s to a field', async (code) => {
  let sent = 0
  server.use(http.post('*/api/v1/console/merchants', ({ request }) => {
    expect(request.headers.get('X-CSRF-Token')).toBe(admin.csrf_token); sent++; return failure(code)
  }))
  mount()
  const dialog = await open('新建商户')
  expect(within(dialog).getByRole('button', { name: '新建商户' })).toBeDisabled()
  fill(dialog, '用户名', 'shop_new'); fill(dialog, '邮箱', 'new@example.test')
  const password = crypto.randomUUID()
  fill(dialog, '初始密码', password); fill(dialog, '确认密码', password)
  submit(dialog, '新建商户')
  await within(dialog).findByText(code === 'USERNAME_TAKEN' ? '用户名已被使用。' : '邮箱已被使用。')
  expect(within(dialog).getByLabelText(code === 'USERNAME_TAKEN' ? '用户名' : '邮箱')).toHaveAttribute('aria-invalid', 'true')
  expect(sent).toBe(1)
})

it('creates a merchant, clears the dialog and navigates to the new detail', async () => {
  const create = vi.spyOn(consoleApi, 'createMerchant')
  server.use(http.post('*/api/v1/console/merchants', async ({ request }) => {
    expect(await request.json()).toMatchObject({ email: 'shop@intranet' })
    return success(detail.user)
  }))
  const { router, client } = mount(), invalidate = vi.spyOn(client, 'invalidateQueries')
  const dialog = await open('新建商户'), password = crypto.randomUUID()
  fill(dialog, '用户名', 'shop_new'); fill(dialog, '邮箱', 'not-an-email')
  fill(dialog, '初始密码', password); fill(dialog, '确认密码', password)
  const email = within(dialog).getByLabelText<HTMLInputElement>('邮箱')
  expect(email.validity.typeMismatch).toBe(true)
  expect(email.validationMessage).not.toBe('')
  submit(dialog, '新建商户')
  expect(create).not.toHaveBeenCalled()
  fill(dialog, '邮箱', 'shop@intranet')
  expect(email).toBeValid()
  submit(dialog, '新建商户')
  await waitFor(() => expect(router.state.location.pathname).toBe('/console/merchants/42'))
  expect(screen.queryByLabelText('初始密码')).toBeNull()
  expect(screen.getByRole('navigation', { name: '管理控制台' }).querySelector('a[href="/console/merchants"]')).toHaveAttribute('aria-current', 'page')
  expect(await screen.findByRole('link', { name: '查看该商户购买记录' })).toHaveAttribute('href', '/console/purchases?merchant_id=42')
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ['console'] })
})

it('validates a ban reason, sends trimmed Unicode and reports expired orders; restores access', async () => {
  server.use(http.post('*/api/v1/console/merchants/42/ban', async ({ request }) => {
    expect(await request.json()).toEqual({ reason: '设备异常' })
    current.user.banned = true; return success({ expired_orders: 3, reason: '设备异常', banned_at: new Date().toISOString() })
  }), http.post('*/api/v1/console/merchants/42/unban', () => { current.user.banned = false; return success(current.user) }))
  mount('/console/merchants/42')
  const dialog = await open('停用商户')
  expect(within(dialog).getByRole('button', { name: '停用商户' })).toBeDisabled()
  fill(dialog, '原因', '😀'.repeat(501))
  expect(within(dialog).getByRole('button', { name: '停用商户' })).toBeDisabled()
  fill(dialog, '原因', ' 设备异常 '); submit(dialog, '停用商户')
  await screen.findByText('已停用商户，已关闭 3 笔待支付订单。')
  const restore = await open('恢复商户'); submit(restore, '恢复商户')
  await screen.findByText('已恢复商户。')
})

describe('registration review', () => {
  const pending = { approval_state: 'pending', approval_reason: null, approval_reviewed_at: null } as const
  it('filters merchants by approval state through the URL', async () => {
    const calls: URL[] = []
    server.use(http.get('*/api/v1/console/merchants', ({ request }) => {
      const url = new URL(request.url); calls.push(url)
      return success({ items: [{ ...merchant, ...pending }], total: 1, next_cursor: null })
    }))
    const { router } = mount(), actor = userEvent.setup()
    expect((await screen.findAllByText('待审核')).length).toBeGreaterThan(0)
    await chooseSelect(actor, screen.getByLabelText('审核', { exact: true }), 'pending')
    await actor.click(screen.getByRole('button', { name: '应用筛选' }))
    await waitFor(() => expect(router.state.location.search).toMatchObject({ approval_state: 'pending' }))
    await waitFor(() => expect(calls.at(-1)?.searchParams.get('approval_state')).toBe('pending'))
  })
  it('rejects a pending merchant with a trimmed reason and then approves it', async () => {
    current.approval = { ...pending }
    const rejects: unknown[] = []
    let approves = 0
    server.use(
      http.post('*/api/v1/console/merchants/42/reject', async ({ request }) => {
        rejects.push(await request.json())
        current.approval = { approval_state: 'rejected', approval_reason: '资料不完整', approval_reviewed_at: '2026-11-13T08:00:00Z' }
        return success(current.approval)
      }),
      http.post('*/api/v1/console/merchants/42/approve', () => {
        approves += 1; current.approval = { approval_state: 'approved', approval_reason: null, approval_reviewed_at: '2026-11-13T09:00:00Z' }
        return success(current.approval)
      }),
    )
    mount('/console/merchants/42')
    await screen.findByText('该商户可登录查看审核状态，通过审核后才能创建订单。')
    expect(screen.getByRole('button', { name: '通过审核' })).toBeInTheDocument()
    const dialog = await open('拒绝注册')
    expect(within(dialog).getByRole('button', { name: '拒绝注册' })).toBeDisabled()
    fill(dialog, '原因', '   ')
    expect(within(dialog).getByRole('button', { name: '拒绝注册' })).toBeDisabled()
    fill(dialog, '原因', ' 资料不完整 '); submit(dialog, '拒绝注册')
    await screen.findByText('已拒绝注册，已通知商户。')
    expect(rejects).toEqual([{ reason: '资料不完整' }])
    await screen.findByText('资料不完整')
    expect(screen.queryByText('该商户可登录查看审核状态，通过审核后才能创建订单。')).toBeNull()
    expect(screen.queryByRole('button', { name: '拒绝注册' })).toBeNull()
    const approve = await open('通过审核'); submit(approve, '通过审核')
    await screen.findByText('已通过审核，已通知商户。')
    expect(approves).toBe(1)
    await waitFor(() => expect(screen.queryByRole('button', { name: '通过审核' })).toBeNull())
  })
  it('locks the reject dialog and refreshes after a review conflict', async () => {
    current.approval = { ...pending }
    let details = 0
    server.use(
      http.get('*/api/v1/console/merchants/42', () => { details += 1; return success(current) }),
      http.post('*/api/v1/console/merchants/42/reject', () => {
        current.approval = { approval_state: 'approved', approval_reason: null, approval_reviewed_at: '2026-11-13T09:00:00Z' }
        return failure('VERSION_CONFLICT')
      }),
    )
    mount('/console/merchants/42')
    const dialog = await open('拒绝注册')
    fill(dialog, '原因', '重复注册'); submit(dialog, '拒绝注册')
    await within(dialog).findByText('审核状态已被其他操作修改，已刷新详情。')
    expect(within(dialog).getByRole('button', { name: '拒绝注册' })).toBeDisabled()
    expect(within(dialog).getByLabelText('原因', { exact: true })).toBeDisabled()
    await waitFor(() => expect(details).toBeGreaterThan(1))
  })
})

it('resets a password without retaining the form after success', async () => {
  const password = crypto.randomUUID()
  server.use(http.post('*/api/v1/console/merchants/42/reset-password', async ({ request }) => {
    expect(await request.json()).toEqual({ password, password_confirmation: password }); return success(null)
  }))
  mount('/console/merchants/42')
  const dialog = await open('重置密码')
  fill(dialog, '新密码', password); fill(dialog, '确认密码', password); submit(dialog, '重置密码')
  await screen.findByText('密码已重置。')
  expect(screen.queryByLabelText('新密码')).toBeNull()
})

it('uses only plan channels, byte validation and the latest subscription revision after conflict', async () => {
  const clock = vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2029-12-31T23:59:00Z'))
  const revisions: number[] = []
  server.use(http.put('*/api/v1/console/merchants/42/subscription', async ({ request }) => {
    const body = await request.json() as Schema['SubscriptionGrantRequest']; revisions.push(body.expected_revision)
    expect(body.channels).toEqual(['alipay', 'wxpay']); expect(body.expires_at).toMatch(/Z$/)
    if (revisions.length === 1) { current.subscription.revision = 6; return failure('VERSION_CONFLICT') }
    return success(current.subscription)
  }))
  mount('/console/merchants/42')
  const dialog = await grant()
  fill(dialog, '原因', '中'.repeat(167))
  expect(within(dialog).getByRole('button', { name: '调整订阅' })).toBeDisabled()
  fill(dialog, '原因', '人工开通'); submit(dialog, '调整订阅')
  await within(dialog).findByText('订阅已被修改，已刷新详情，请检查并重新提交。')
  expect(within(dialog).getByLabelText('原因')).toHaveValue('人工开通')
  expect(within(dialog).getByLabelText('套餐')).toHaveAttribute('data-value', '3')
  await waitFor(() => expect(within(dialog).getByRole('button', { name: '调整订阅' })).toBeEnabled())
  clock.mockReturnValue(Date.parse('2030-01-01T00:01:00Z'))
  const grantRequest = vi.spyOn(consoleApi, 'grantSubscription')
  submit(dialog, '调整订阅')
  expect(within(dialog).getByLabelText('到期时间')).toHaveAccessibleDescription('请将到期时间调整到未来。')
  expect(grantRequest).not.toHaveBeenCalled()
  fill(dialog, '到期时间', '2030-01-03T00:00')
  submit(dialog, '调整订阅'); await screen.findByText('订阅已更新。')
  expect(revisions).toEqual([5, 6])
})

it.each(['click', 'Enter'] as const)('rechecks a subscription expiry on %s after time passes in the dialog', async (method) => {
  const clock = vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2029-12-31T23:59:00Z'))
  const grantRequest = vi.spyOn(consoleApi, 'grantSubscription')
  server.use(http.put('*/api/v1/console/merchants/42/subscription', async ({ request }) => {
    expect(await request.json()).toMatchObject({ expected_revision: 5, expires_at: new Date('2030-01-03T00:00').toISOString() })
    return success(current.subscription)
  }))
  mount('/console/merchants/42')
  const dialog = await grant()
  const button = within(dialog).getByRole('button', { name: '调整订阅' })
  const expiry = within(dialog).getByLabelText<HTMLInputElement>('到期时间')
  const original = expiry.value
  expect(button).toBeEnabled()
  clock.mockReturnValue(Date.parse('2030-01-01T00:01:00Z'))
  if (method === 'click') await userEvent.click(button)
  else {
    button.focus()
    await userEvent.keyboard('{Enter}')
  }
  expect(expiry).toHaveAccessibleDescription('请将到期时间调整到未来。')
  expect(expiry).toHaveValue(original)
  expect(within(dialog).getByLabelText('原因')).toHaveValue('人工开通')
  expect(grantRequest).not.toHaveBeenCalled()
  fill(dialog, '到期时间', '2030-01-03T00:00')
  expect(expiry).not.toHaveAttribute('aria-invalid')
  submit(dialog, '调整订阅')
  await screen.findByText('订阅已更新。')
  expect(grantRequest).toHaveBeenCalledTimes(1)
})

it('refreshes plans and blocks a no-longer-available selection', async () => {
  server.use(http.put('*/api/v1/console/merchants/42/subscription', () => { currentPlan.deleted = true; return failure('PLAN_UNAVAILABLE') }))
  mount('/console/merchants/42')
  const dialog = await grant(); submit(dialog, '调整订阅')
  await within(dialog).findByText('套餐已下架，已刷新套餐列表，请重新选择。')
  await waitFor(() => expect(within(dialog).getByRole('button', { name: '调整订阅' })).toBeDisabled())
  expect(within(dialog).getByLabelText('原因')).toHaveValue('人工开通')
})

it('creates a plan with cents and unlimited quantity, then edits with optimistic revision and reconfirms conflict', async () => {
  const writes: Schema['SubscriptionPlanCommand'][] = []
  server.use(http.post('*/api/v1/console/plans', async ({ request }) => { writes.push(await request.json() as Schema['SubscriptionPlanCommand']); return success(currentPlan) }),
    http.put('*/api/v1/console/plans/3', async ({ request }) => {
      writes.push(await request.json() as Schema['SubscriptionPlanCommand'])
      if (writes.length === 2) { currentPlan.revision = 3; currentPlan.price_cents = 4999; return failure('VERSION_CONFLICT') }
      return success(currentPlan)
    }))
  mount('/console/plans')
  let dialog = await open('新建套餐')
  fill(dialog, '中文名称', '新套餐'); fill(dialog, '价格（元）', '0.01'); fill(dialog, '时长（天）', '1')
  fireEvent.click(within(dialog).getByRole('checkbox', { name: '支付宝' }))
  submit(dialog, '新建套餐'); await screen.findByText('套餐已保存。')
  expect(writes[0]).toMatchObject({ price_cents: 1, purchase_limit: null, channels: ['alipay'] })
  fireEvent.click(screen.getAllByRole('button', { name: '编辑' })[0]!)
  dialog = await screen.findByRole('dialog', { name: '编辑套餐' })
  fill(dialog, '价格（元）', '39.99'); submit(dialog, '保存修改')
  await within(dialog).findByText('套餐已被修改，已刷新最新版本。请检查最新信息后重新确认。')
  expect(within(dialog).getByLabelText('价格（元）')).toHaveValue('39.99')
  expect(within(dialog).getByRole('button', { name: '保存修改' })).toBeDisabled()
  fireEvent.click(within(dialog).getByRole('checkbox', { name: '我已检查最新套餐信息，确认按当前输入保存' }))
  submit(dialog, '保存修改'); await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(writes[1]).toMatchObject({ id: '3', revision: 2 }); expect(writes[2]).toMatchObject({ revision: 3, price_cents: 3999 })
})

describe.each(['新建套餐', '编辑套餐'] as const)('%s purchase limit', (title) => {
  it.each([
    ['2147483647', 2147483647],
    ['2147483648', undefined],
    ['', null],
  ] as const)('validates %j and sends %j when allowed', async (value, expected) => {
    const editing = title === '编辑套餐'
    const writes: Schema['SubscriptionPlanCommand'][] = []
    const save = vi.spyOn(consoleApi, 'savePlan')
    const capture = async ({ request }: { request: Request }) => {
      writes.push(await request.json() as Schema['SubscriptionPlanCommand'])
      return success(currentPlan)
    }
    currentPlan.purchase_limit = 10
    server.use(http.post('*/api/v1/console/plans', capture), http.put('*/api/v1/console/plans/3', capture))
    mount('/console/plans')
    if (editing) fireEvent.click((await screen.findAllByRole('button', { name: '编辑' }))[0]!)
    const dialog = editing ? await screen.findByRole('dialog', { name: title }) : await open(title)
    if (!editing) {
      fill(dialog, '中文名称', '新套餐'); fill(dialog, '价格（元）', '0.01'); fill(dialog, '时长（天）', '1')
      fireEvent.click(within(dialog).getByRole('checkbox', { name: '支付宝' }))
    }
    fill(dialog, '限购数量', value)
    const button = within(dialog).getByRole('button', { name: editing ? '保存修改' : title })
    if (expected === undefined) {
      expect(button).toBeDisabled()
      expect(within(dialog).getByLabelText('限购数量')).toHaveAccessibleDescription('限购数量须为 1 至 2147483647 的整数，留空表示不限。')
      await userEvent.click(button)
      fireEvent.submit(button.closest('form')!)
      expect(save).not.toHaveBeenCalled()
      expect(writes).toEqual([])
      return
    }
    expect(button).toBeEnabled()
    await userEvent.click(button)
    await screen.findByText('套餐已保存。')
    expect(writes).toHaveLength(1)
    expect(writes[0]).toMatchObject({ purchase_limit: expected, ...(editing ? { id: '3', revision: 2 } : {}) })
  })
})

it('deletes with revision and shows the cancellation count; deleted plans remain read-only', async () => {
  server.use(http.delete('*/api/v1/console/plans/3', async ({ request }) => {
    expect(await request.json()).toEqual({ revision: 2 }); currentPlan.deleted = true
    return success({ deleted: true, cancelled_pending_purchases: 2 })
  }))
  mount('/console/plans'); await screen.findAllByText('专业套餐')
  fireEvent.click(screen.getAllByRole('button', { name: '删除' })[0]!)
  const dialog = await screen.findByRole('dialog', { name: '删除套餐' })
  expect(within(dialog).getByText('删除后待支付的购买会被取消，已生效订阅保留。')).toBeInTheDocument()
  submit(dialog, '删除套餐'); await screen.findByText('已删除套餐，已取消 2 笔待支付购买。')
  expect(screen.queryByRole('button', { name: '编辑' })).toBeNull()
})

it.each([
  ['900719925474099312345', '900719925474099312345'],
  ['0001234567890123456', '0001234567890123456'],
  ['%22900719925474099312345%22', '900719925474099312345'],
])('preserves the console order URL query %s in the form, list and export', async (raw, query) => {
  const lists: URL[] = [], exports: URL[] = []
  server.use(http.get('*/api/v1/console/orders', ({ request }) => {
    const url = new URL(request.url)
    lists.push(url)
    return success({ items: [], total: 0, next_cursor: url.searchParams.has('cursor') ? null : 'order-cursor' })
  }), http.get('*/api/v1/console/orders/export.csv', ({ request }) => {
    exports.push(new URL(request.url))
    return failure('VERSION_CONFLICT')
  }))
  const { router } = mount('/console/orders?query=' + raw)
  const actor = userEvent.setup()
  expect(await screen.findByLabelText('搜索订单')).toHaveValue(query)
  await actor.click(await screen.findByRole('button', { name: '加载更多' }))
  await waitFor(() => expect(lists.at(-1)?.searchParams.get('cursor')).toBe('order-cursor'))
  await actor.click(screen.getByRole('button', { name: '应用筛选' }))
  await waitFor(() => expect(lists).toHaveLength(3))
  expect(lists.at(-1)?.searchParams.has('cursor')).toBe(false)
  await actor.click(screen.getByRole('button', { name: '导出 CSV' }))
  await screen.findByRole('alert')
  expect(exports[0]?.searchParams.get('query')).toBe(query)
  expect(exports[0]?.searchParams.has('cursor')).toBe(false)
  await actor.click(await screen.findByRole('button', { name: '加载更多' }))
  await waitFor(() => expect(lists.at(-1)?.searchParams.get('cursor')).toBe('order-cursor'))
  expect(lists.map((url) => url.searchParams.get('query'))).toEqual([query, query, query, query])
  await actor.click(screen.getByRole('button', { name: '重置' }))
  await waitFor(() => expect(lists).toHaveLength(5))
  expect(lists.at(-1)?.searchParams.has('cursor')).toBe(false)
  expect(lists.at(-1)?.searchParams.has('query')).toBe(false)
  expect(screen.getByLabelText('搜索订单')).toHaveValue('')
  expect(router.state.location.search).toEqual({})
})

it('clears only the console CSRF token after a console CSV 401', async () => {
  const merchantToken = crypto.randomUUID()
  const preferences = { locale: 'zh-CN', theme: 'system' } as const
  const seen = { console: [] as (string | null)[], merchant: [] as (string | null)[] }
  const unauthorized = vi.fn()
  onUnauthorized(unauthorized)
  server.use(http.get('*/api/v1/merchant/me', () => success({ ...admin, user: detail.user, csrf_token: merchantToken })),
    http.get('*/api/v1/console/orders/export.csv', () => new HttpResponse(null, { status: 401 })),
    http.patch('*/api/v1/console/preferences', ({ request }) => {
      seen.console.push(request.headers.get('X-CSRF-Token')); return success(preferences)
    }),
    http.patch('*/api/v1/merchant/preferences', ({ request }) => {
      seen.merchant.push(request.headers.get('X-CSRF-Token')); return success(preferences)
    }))
  try {
    await api.me()
    await consoleApi.me()
    await consoleApi.preferences(preferences)
    await expect(consoleApi.exportOrders({})).rejects.toMatchObject({ status: 401 })
    await consoleApi.preferences(preferences)
    await api.preferences(preferences)
    expect(seen).toEqual({ console: [admin.csrf_token, null], merchant: [merchantToken] })
    expect(unauthorized).toHaveBeenCalledOnce()
  } finally {
    api.forgetSession()
    consoleApi.forgetSession()
  }
})

it('renders shared console orders read-only with merchant links and role-specific export errors', async () => {
  const item = order('paid', 'alipay')
  const dto: Schema['OrderDetail'] = { order: item, callback_attempts: [], refund: null, notify_url: null, return_url: null, key_version: null, receipt_id: null, clientip: null,
    allowed_channels_snapshot: ['alipay'], callback_replays_used: 0, callback_replay_limit: 2147483646 }
  server.use(http.get('*/api/v1/console/orders', () => success({ items: [item], total: 1, next_cursor: null })),
    http.get('*/api/v1/console/orders/export.csv', () => failure('VERSION_CONFLICT')),
    http.get('*/api/v1/console/orders/:tradeNo', () => success(dto)))
  const { router } = mount('/console/orders')
  expect((await screen.findAllByText(item.merchant_username)).length).toBeGreaterThan(0)
  fireEvent.click(screen.getByRole('button', { name: '导出 CSV' }))
  await screen.findByRole('alert')
  await act(() => router.navigate({ to: '/console/orders/$tradeNo', params: { tradeNo: item.trade_no } }))
  await screen.findByRole('heading', { name: '订单详情' })
  for (const name of ['补单', '退款', '重新发送回调']) expect(screen.queryByRole('button', { name })).toBeNull()
  expect(screen.getByRole('link', { name: item.merchant_username })).toHaveAttribute('href', '/console/merchants/' + item.merchant_id)
})

it('syncs purchase filters to the URL, preserves and clears merchant_id, and links buyer and payment order', async () => {
  const calls: URL[] = [], trade = crypto.randomUUID()
  server.use(http.get('*/api/v1/console/purchases', ({ request }) => {
    calls.push(new URL(request.url))
    return success({ items: [{ id: '9', buyer_id: '42', billing_merchant_id: '1', cashier_url: null, created_at: '2026-11-05T00:00:00Z', expires_at: '2030-01-01T00:00:00Z', paid_at: null, fulfillment_at: null, plan_snapshot: plan, price_cents: 2999, purchase_kind: 'new', state: 'pending', trade_no: trade }], total: 1, next_cursor: null })
  }))
  const { router } = mount('/console/purchases?merchant_id=42')
  expect((await screen.findAllByRole('link', { name: '#42' }))[0]).toHaveAttribute('href', '/console/merchants/42')
  await chooseSelect(userEvent.setup(), screen.getByLabelText('状态', { exact: true }), 'paid')
  await chooseSelect(userEvent.setup(), screen.getByLabelText('套餐', { exact: true }), '3')
  fireEvent.click(screen.getByRole('button', { name: '应用筛选' }))
  await waitFor(() => expect(router.state.location.search).toMatchObject({ merchant_id: 42, state: 'paid', plan_id: 3 }))
  expect(calls.at(-1)?.searchParams.get('merchant_id')).toBe('42')
  expect((await screen.findAllByRole('link', { name: trade }))[0]).toHaveAttribute('href', '/console/orders/' + trade)
  fireEvent.click(screen.getByRole('button', { name: '清除商户筛选' }))
  await waitFor(() => expect(router.state.location.search.merchant_id).toBeUndefined())
})

it('does not navigate or reopen a dialog when a create response arrives after leaving', async () => {
  let release!: () => void
  server.use(http.post('*/api/v1/console/merchants', async () => { await new Promise<void>((resolve) => { release = resolve }); return success(detail.user) }))
  const { router } = mount(), dialog = await open('新建商户'), password = crypto.randomUUID()
  fill(dialog, '用户名', 'shop_new'); fill(dialog, '邮箱', 'new@example.test'); fill(dialog, '初始密码', password); fill(dialog, '确认密码', password)
  submit(dialog, '新建商户'); await waitFor(() => expect(release).toBeTypeOf('function'))
  await act(() => router.navigate({ to: '/console/plans' }))
  await act(async () => { release(); await new Promise((resolve) => setTimeout(resolve, 30)) })
  expect(router.state.location.pathname).toBe('/console/plans')
  expect(screen.queryByRole('dialog')).toBeNull()
})


it('resets channels to the selected plan subset before submitting', async () => {
  const other = { ...plan, id: '4', name: { zh_CN: '微信套餐', en: 'WeChat' }, channels: ['wxpay'] }
  server.use(http.get('*/api/v1/console/plans', () => success({ items: [plan, other], total: 2, next_cursor: null })),
    http.put('*/api/v1/console/merchants/42/subscription', async ({ request }) => {
      expect(await request.json()).toMatchObject({ plan_id: '4', channels: ['wxpay'], expected_revision: 5 })
      return success(current.subscription)
    }))
  mount('/console/merchants/42')
  const dialog = await grant()
  await chooseSelect(userEvent.setup(), within(dialog).getByLabelText('套餐', { exact: true }), '4')
  expect(within(dialog).queryByRole('checkbox', { name: '支付宝' })).toBeNull()
  expect(within(dialog).getByRole('checkbox', { name: '微信支付' })).toBeChecked()
  submit(dialog, '调整订阅'); await screen.findByText('订阅已更新。')
})

it('refreshes a deletion conflict and requires confirmation with the new revision', async () => {
  const revisions: number[] = []
  server.use(http.delete('*/api/v1/console/plans/3', async ({ request }) => {
    revisions.push((await request.json() as { revision: number }).revision)
    if (revisions.length === 1) { currentPlan.revision = 7; return failure('VERSION_CONFLICT') }
    currentPlan.deleted = true; return success({ deleted: true, cancelled_pending_purchases: 0 })
  }))
  mount('/console/plans'); await screen.findAllByText('专业套餐')
  fireEvent.click(screen.getAllByRole('button', { name: '删除' })[0]!)
  const dialog = await screen.findByRole('dialog', { name: '删除套餐' })
  submit(dialog, '删除套餐')
  await within(dialog).findByText('套餐已被修改，已刷新最新版本。请检查最新信息后重新确认。')
  expect(within(dialog).getByRole('button', { name: '删除套餐' })).toBeDisabled()
  fireEvent.click(within(dialog).getByRole('checkbox', { name: '我已检查最新套餐信息，确认按当前输入保存' }))
  submit(dialog, '删除套餐'); await screen.findByText('已删除套餐，已取消 0 笔待支付购买。')
  expect(revisions).toEqual([2, 7])
})

it('requests include_deleted and keeps deleted rows read-only', async () => {
  let included = false
  server.use(http.get('*/api/v1/console/plans', ({ request }) => {
    included = new URL(request.url).searchParams.get('include_deleted') === 'true'
    return success({ items: included ? [{ ...plan, deleted: true }] : [], total: included ? 1 : 0, next_cursor: null })
  }))
  mount('/console/plans')
  fireEvent.click(await screen.findByRole('checkbox', { name: '显示已删除' }))
  await screen.findAllByText('已删除')
  expect(included).toBe(true)
  expect(screen.queryByRole('button', { name: '编辑' })).toBeNull()
})

it('shows a missing merchant without opening management actions', async () => {
  server.use(http.get('*/api/v1/console/merchants/42', () => HttpResponse.json({ error: { code: 'NOT_FOUND' } }, { status: 404 })))
  mount('/console/merchants/42')
  await screen.findByText('商户不存在')
  expect(screen.queryByRole('button', { name: '停用商户' })).toBeNull()
})
