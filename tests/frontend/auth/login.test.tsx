import { emptyNotifications } from '../merchant/notification-fixture'
import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { delay, http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import type { components } from '@/api/schema'
import { createI18n } from '@/lib/i18n'
import { emptyStats, unboundDevice } from '../merchant/fixtures'

vi.mock('@/features/auth/Turnstile', () => ({
  Turnstile: ({ onToken }: { onToken: (token: string) => void }) => <button type="button" onClick={() => onToken(crypto.randomUUID())}>完成人机验证</button>,
}))

type Schema = components['schemas']
const user: Schema['AuthUserDTO'] = { id: '1', role: 'merchant', username: 'test_shop', pid: '9000000000000001', banned: false, banned_at: null, ban_reason: null, totp_enabled: false, locale: 'zh-CN', theme: 'system', created_at: new Date().toISOString() }
const session: Schema['AuthSessionResult'] = { user, csrf_token: crypto.randomUUID(), expires_at: new Date(Date.now() + 3600000).toISOString(), next_path: '/dashboard' }
const config: Schema['PublicConfig'] = { site_name: 'Test Shop', logo_url: '', favicon_url: '/favicon.ico?v=default', turnstile: { site_key: crypto.randomUUID(), enabled: true, login_action: 'login', register_action: 'register' }, registration_open: true, email_domain_restriction: { enabled: false, domains: [] }, home: { mode: 'default', html: null, url: null }, footer: [], legal_revisions: { terms: 1, privacy: 1 } }
const success = <T,>(data: T) => HttpResponse.json({ data, error: null, request_id: crypto.randomUUID() })
const failure = (status: number, code: Schema['ErrorCode'], fields = {}) => HttpResponse.json({ error: { code, fields, details: {}, message: code }, request_id: crypto.randomUUID() }, { status })
const server = setupServer(emptyNotifications,
  http.get('*/api/v1/merchant/stats', () => success(emptyStats())),
  http.get('*/api/v1/merchant/device', () => success(unboundDevice)),
  http.get('*/api/v1/merchant/orders', () => success({ items: [], total: 0, next_cursor: null })),
  http.get('*/api/v1/public/config', () => success(config)),
  http.get('*/api/v1/merchant/me', () => success(session)),
  http.post('*/api/v1/auth/login', () => success(session)),
)
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

function renderApp(path = '/login', queryClient = createQueryClient()) {
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) })
  render(<AppProviders i18n={createI18n()} queryClient={queryClient}><RouterProvider router={router} /></AppProviders>)
  return router
}
async function fillLogin() {
  const actor = userEvent.setup()
  await actor.type(await screen.findByLabelText('用户名 / 邮箱'), 'test_shop')
  await actor.type(screen.getByLabelText('密码'), crypto.randomUUID())
  await actor.click(await screen.findByRole('button', { name: '完成人机验证' }))
  return actor
}

describe('merchant login', () => {
  it('keeps the password-change notice as the only status while public config is loading', async () => {
    let finish: (() => void) | undefined
    server.use(http.get('*/api/v1/public/config', async () => {
      await new Promise<void>((resolve) => { finish = resolve })
      return success(config)
    }))
    renderApp('/login?notice=password_changed')
    try {
      await screen.findByText('正在加载…')
      await waitFor(() => expect(finish).toBeDefined())
      expect(screen.getAllByRole('status')).toHaveLength(1)
      expect(screen.getByRole('status')).toHaveTextContent('密码已修改，所有会话已失效，请重新登录。')
    } finally { finish?.() }
    await screen.findByRole('button', { name: '完成人机验证' })
  })

  it('validates fields, waits for Turnstile and disables duplicate submissions', async () => {
    let submissions = 0
    let finish: (() => void) | undefined
    server.use(http.post('*/api/v1/auth/login', async () => { submissions++; await new Promise<void>((resolve) => { finish = resolve }); return success(session) }))
    const router = renderApp()
    expect(await screen.findByRole('button', { name: '登录' })).toBeDisabled()
    const actor = userEvent.setup()
    await actor.click(await screen.findByRole('button', { name: '完成人机验证' }))
    await actor.click(screen.getByRole('button', { name: '登录' }))
    expect(screen.getByText('请输入用户名或邮箱')).toBeVisible()
    expect(screen.getByText('请输入密码')).toBeVisible()
    await actor.type(screen.getByLabelText('用户名 / 邮箱'), 'test_shop')
    await actor.type(screen.getByLabelText('密码'), crypto.randomUUID())
    await actor.dblClick(screen.getByRole('button', { name: '登录' }))
    await waitFor(() => expect(submissions).toBe(1))
    expect(screen.getByRole('button', { name: '正在提交…' })).toBeDisabled()
    finish?.()
    await waitFor(() => expect(router.state.location.pathname).toBe('/dashboard'))
  })

  it('switches between TOTP and recovery codes and submits the selected field', async () => {
    const challenge = crypto.randomUUID()
    server.use(http.post('*/api/v1/auth/login', () => success({ requires_2fa: true, challenge_id: challenge, expires_at: session.expires_at })))
    let proof: Schema['AuthFactorRequest'] | undefined
    server.use(http.post('*/api/v1/auth/second-factor', async ({ request }) => { proof = await request.json() as Schema['AuthFactorRequest']; return success(session) }))
    renderApp()
    const actor = await fillLogin()
    await actor.click(screen.getByRole('button', { name: '登录' }))
    expect(await screen.findByLabelText('动态验证码')).toBeVisible()
    await actor.click(screen.getByRole('button', { name: '恢复码' }))
    expect(screen.getByLabelText('恢复码')).toBeVisible()
    await actor.type(screen.getByLabelText('恢复码'), crypto.randomUUID())
    await actor.click(screen.getByRole('button', { name: '验证器' }))
    expect(screen.getByLabelText('动态验证码')).toHaveValue('')
    await actor.click(screen.getByRole('button', { name: '验证并登录' }))
    expect(screen.getByText('请输入 6 位动态验证码')).toBeVisible()
    await actor.click(screen.getByRole('button', { name: '恢复码' }))
    const recovery = crypto.randomUUID()
    await actor.type(screen.getByLabelText('恢复码'), recovery)
    await actor.click(screen.getByRole('button', { name: '验证并登录' }))
    await waitFor(() => expect(proof).toEqual({ challenge_id: challenge, recovery_code: recovery }))
  })

  it.each([
    [401, 'INVALID_CREDENTIALS', '账号或密码错误'],
    [401, 'INVALID_2FA', '验证码或恢复码错误，请重试。'],
    [401, 'CHALLENGE_EXPIRED', '本次验证已过期，请重新登录。'],
    [400, 'TURNSTILE_FAILED', '人机验证未完成或已过期，请重新验证。'],
    [403, 'FORBIDDEN', '没有权限执行此操作，请联系平台管理员。'],
    [429, 'FORBIDDEN', '操作过于频繁，请稍后重试。'],
    [503, 'SETUP_REQUIRED', '站点尚未完成配置，请联系平台管理员。'],
    [503, 'TURNSTILE_UNAVAILABLE', '服务暂时不可用，请稍后重试。'],
    [500, 'INTERNAL_ERROR', '服务暂时不可用，请稍后重试。'],
  ] as const)('maps HTTP %s / %s to readable errors', async (status, code, message) => {
    server.use(http.post('*/api/v1/auth/login', () => failure(status, code)))
    renderApp()
    const actor = await fillLogin()
    await actor.click(screen.getByRole('button', { name: '登录' }))
    expect((await screen.findByText(message)).closest('[role="alert"]')).toHaveAttribute('aria-live', 'polite')
  })

  it('keeps an invalid second-factor challenge for retry and restarts an expired challenge', async () => {
    const challenge = crypto.randomUUID()
    let logins = 0
    server.use(http.post('*/api/v1/auth/login', () => { logins++; return success({ requires_2fa: true, challenge_id: challenge, expires_at: session.expires_at }) }))
    server.use(http.post('*/api/v1/auth/second-factor', () => failure(401, 'INVALID_2FA')))
    renderApp()
    const actor = await fillLogin()
    await actor.click(screen.getByRole('button', { name: '登录' }))
    await actor.type(await screen.findByLabelText('动态验证码'), '123456')
    await actor.click(screen.getByRole('button', { name: '验证并登录' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('验证码或恢复码错误，请重试。')
    expect(screen.getByLabelText('动态验证码')).toHaveValue('123456')
    expect(logins).toBe(1)
    server.use(http.post('*/api/v1/auth/second-factor', () => failure(401, 'CHALLENGE_EXPIRED')))
    await actor.click(screen.getByRole('button', { name: '验证并登录' }))
    expect(await screen.findByText('本次验证已过期，请重新登录。')).toBeVisible()
    await actor.click(screen.getByRole('button', { name: '重新登录' }))
    expect(await screen.findByLabelText('密码')).toHaveValue('')
    expect(screen.queryByLabelText('动态验证码')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '登录' })).toBeDisabled()
  })

  it('maps network errors and server field validation', async () => {
    server.use(http.post('*/api/v1/auth/login', () => failure(400, 'VALIDATION_ERROR', { identifier: 'invalid' })))
    renderApp()
    const actor = await fillLogin()
    await actor.click(screen.getByRole('button', { name: '登录' }))
    expect(await screen.findByText('请输入用户名或邮箱')).toBeVisible()
    expect(screen.getByLabelText('用户名 / 邮箱')).toHaveAttribute('aria-invalid', 'true')
    server.use(http.post('*/api/v1/auth/login', () => HttpResponse.error()))
    await actor.click(screen.getByRole('button', { name: '完成人机验证' }))
    await actor.click(screen.getByRole('button', { name: '登录' }))
    expect(await screen.findByText('网络连接失败，请检查网络后重试。')).toBeVisible()
  })

  it('permits login without a token or widget when Turnstile is disabled', async () => {
    server.use(http.get('*/api/v1/public/config', () => success({ ...config, registration_open: false, turnstile: { ...config.turnstile, enabled: false } })))
    renderApp()
    expect(await screen.findByText('需要开通商户？请联系平台管理员')).toBeVisible()
    expect(screen.queryByRole('button', { name: '完成人机验证' })).not.toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('button', { name: '登录' })).toBeEnabled())
    const requests: unknown[] = []
    server.use(http.post('*/api/v1/auth/login', async ({ request }) => {
      requests.push(await request.json())
      return failure(401, 'INVALID_CREDENTIALS')
    }))
    const actor = userEvent.setup()
    await actor.type(screen.getByLabelText('用户名 / 邮箱'), 'merchant')
    await actor.type(screen.getByLabelText('密码', { exact: true }), crypto.randomUUID())
    await actor.click(screen.getByRole('button', { name: '登录' }))
    await waitFor(() => expect(requests).toHaveLength(1))
    expect(requests[0]).not.toHaveProperty('turnstile_token')
    expect(screen.queryByRole('link', { name: '注册商户' })).not.toBeInTheDocument()
  })

  it('guards merchant orders without flashing content and restores redirect filters', async () => {
    server.use(http.get('*/api/v1/merchant/me', async () => { await delay(30); return failure(401, 'UNAUTHENTICATED') }))
    const router = renderApp('/orders?status=paid')
    expect(screen.queryByRole('navigation', { name: '商户导航' })).not.toBeInTheDocument()
    await screen.findByLabelText('用户名 / 邮箱')
    expect(router.state.location.search).toEqual({ redirect: '/orders?status=paid' })
    server.use(http.get('*/api/v1/merchant/me', () => success(session)))
    const actor = await fillLogin()
    await actor.click(screen.getByRole('button', { name: '登录' }))
    await waitFor(() => expect(router.state.location.href).toBe('/orders?status=paid'))
    expect(await screen.findByRole('navigation', { name: '商户导航' })).toBeVisible()
    expect(await screen.findByLabelText('状态', { exact: true })).toHaveValue('paid')
  })

  it.each(['pending', 500, 403, 'network', 200, 401] as const)('logout %s preserves private state until success, except the global 401 redirect', async (outcome) => {
    const restored = crypto.randomUUID()
    server.use(http.get('*/api/v1/merchant/me', () => success({ ...session, csrf_token: restored })))
    let csrf: string | null = null
    let finish: (() => void) | undefined
    server.use(http.post('*/api/v1/auth/logout', async ({ request }) => {
      csrf = request.headers.get('X-CSRF-Token')
      if (outcome === 'pending') await new Promise<void>((resolve) => { finish = resolve })
      if (outcome === 'network') return HttpResponse.error()
      if (outcome === 500) return failure(500, 'INTERNAL_ERROR')
      if (outcome === 403) return failure(403, 'CSRF_FAILED')
      if (outcome === 401) return failure(401, 'UNAUTHENTICATED')
      return success(null)
    }))
    const queryClient = createQueryClient()
    const privateKey = ['merchant-private', user.id]
    queryClient.setQueryData(privateKey, { merchantId: user.id })
    const router = renderApp('/dashboard', queryClient)
    const actor = userEvent.setup()
    await actor.click(await screen.findByRole('button', { name: '退出登录' }))
    await waitFor(() => expect(csrf).toBe(restored))
    if (outcome === 'pending') {
      try {
        await waitFor(() => expect(finish).toBeTypeOf('function'))
        expect(screen.getByRole('button', { name: '正在提交…' })).toBeDisabled()
        expect(queryClient.getQueryData(privateKey)).toEqual({ merchantId: user.id })
        expect(router.state.location.pathname).toBe('/dashboard')
      } finally { finish?.() }
    }
    if (outcome === 500 || outcome === 403 || outcome === 'network') {
      expect(await screen.findByRole('alert')).toHaveTextContent(outcome === 500 ? '服务暂时不可用' : outcome === 403 ? '安全验证失败' : '网络连接失败')
      expect(queryClient.getQueryData(privateKey)).toEqual({ merchantId: user.id })
      expect(router.state.location.pathname).toBe('/dashboard')
      return
    }
    await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
    expect(csrf).toBe(restored)
    if (outcome === 401) {
      // A protected-request 401 intentionally uses the global session-expiry redirect.
      expect(router.state.location.search).toEqual({ redirect: '/dashboard' })
      expect(queryClient.getQueryData(privateKey)).toEqual({ merchantId: user.id })
      return
    }
    expect(queryClient.getQueryData(privateKey)).toBeUndefined()
  })
})
