import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { delay, http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, consoleApi, onUnauthorized, type ConsoleRecovery } from '@/api/client'
import type { components } from '@/api/schema'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import { consoleDestination, consoleSafePath, safePath } from '@/lib/redirect'
import { createI18n } from '@/lib/i18n'
import { publicConfigQuery } from '@/lib/public-config'
import { emptyStats } from '../merchant/fixtures'

vi.mock('@/features/auth/Turnstile', () => ({
  Turnstile: ({ siteKey, onToken }: { siteKey: string; onToken: (token: string) => void }) => (
    <button type="button" data-site-key={siteKey} onClick={() => onToken(crypto.randomUUID())}>完成人机验证</button>
  ),
}))
type Schema = components['schemas']
const user: Schema['AuthUserDTO'] = { id: '1', role: 'admin', username: 'console_admin', banned: false, banned_at: null, ban_reason: null, totp_enabled: false, locale: 'zh-CN', theme: 'system', created_at: new Date().toISOString() }
const session: Schema['AuthSessionResult'] = { user, csrf_token: crypto.randomUUID(), expires_at: new Date(Date.now() + 43200000).toISOString(), next_path: '/console' }
const config: Schema['PublicConfig'] = { site_name: 'VanillaPay', logo_url: '', turnstile: { configured: true, site_key: 'public-site', login_action: 'login', register_action: 'register' }, registration_open: true, email_domain_restriction: { enabled: false, domains: [] }, home: { mode: 'default', html: null, url: null }, footer: [], legal_revisions: { terms: 1, privacy: 1 } }
const success = <T,>(data: T) => HttpResponse.json({ data })
const failure = (status: number, code: Schema['ErrorCode']) => HttpResponse.json({ error: { code, message: code } }, { status })
let signedIn = false
const server = setupServer(
  http.get('*/api/v1/public/config', () => success(config)),
  http.get('*/api/v1/console/bootstrap-status', () => success({ initialized: true, turnstile_configured: true })),
  http.get('*/api/v1/console/me', () => signedIn ? success(session) : failure(401, 'UNAUTHENTICATED')),
  http.get('*/api/v1/merchant/me', () => failure(403, 'FORBIDDEN')),
  http.get('*/api/v1/console/stats', () => success(emptyStats())),
  http.post('*/api/v1/console/login', () => { signedIn = true; return success(session) }),
)
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
beforeEach(() => { signedIn = false; api.forgetSession(); consoleApi.forgetSession() })
afterEach(() => { server.resetHandlers(); vi.useRealTimers() })
afterAll(() => server.close())
function mount(path = '/console/login') {
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) }), client = createQueryClient()
  const view = render(<AppProviders queryClient={client} i18n={createI18n()}><RouterProvider router={router} /></AppProviders>)
  return { router, client, ...view }
}
function recovery(overrides: Partial<ConsoleRecovery> = {}): ConsoleRecovery {
  return { csrf_token: crypto.randomUUID(), expires_at: new Date(Date.now() + 600000).toISOString(), repair_only: true, settings_version: 7, security: { turnstile_site_key: 'public-site', allowed_hostname: 'pay.example.test', turnstile_secret_configured: true }, ...overrides }
}
async function enterRecovery() {
  const actor = userEvent.setup()
  await actor.type(await screen.findByLabelText('终端恢复码'), crypto.randomUUID())
  await actor.click(screen.getByRole('button', { name: '验证恢复码' }))
  await screen.findByLabelText('Turnstile Site Key')
  return actor
}
async function enterLogin() {
  const actor = userEvent.setup()
  await actor.type(await screen.findByLabelText('用户名 / 邮箱'), user.username)
  await actor.type(screen.getByLabelText('密码', { exact: true }), crypto.randomUUID())
  await actor.click(await screen.findByRole('button', { name: '完成人机验证' }))
  await actor.click(screen.getByRole('button', { name: '登录' }))
  return actor
}

describe('console safe paths', () => {
  it.each(['//evil.test', 'https://evil.test', '/\\evil.test', '/console/%5c%5cevil.test', '/console/%0a/x', '/%2f%2fevil.test', '/console/%252f%252fevil.test', '/console/../login', '/console/%2e%2e/evil', '/console/login', '/console/setup?x=1', '/console/recovery/', '/login', '/dashboard', '/consoleevil', '/%', ''])('rejects %s', (value) => {
    expect(consoleSafePath(value)).toBeUndefined()
    expect(consoleDestination(value, value)).toBe('/console')
  })
  it('preserves safe query/hash and does not change merchant safePath', () => {
    expect(consoleDestination('/console/orders?state=paid#recent', '/console')).toBe('/console/orders?state=paid#recent')
    expect(consoleDestination('/login', '/console/account')).toBe('/console/account')
    expect(safePath('/console/account')).toBeUndefined()
  })
})

describe('role-scoped CSRF', () => {
  it('keeps merchant and console tokens isolated, including refresh and forgetting a role', async () => {
    const merchant = { ...session, csrf_token: crypto.randomUUID(), user: { ...user, role: 'merchant' } }
    signedIn = true
    server.use(http.get('*/api/v1/merchant/me', () => success(merchant)),
      http.patch('*/api/v1/merchant/preferences', ({ request }) => { expect(request.headers.get('X-CSRF-Token')).toBe(merchant.csrf_token); return success({ locale: 'en', theme: 'dark' }) }),
      http.patch('*/api/v1/console/preferences', ({ request }) => { expect(request.headers.get('X-CSRF-Token')).toBe(session.csrf_token); return success({ locale: 'en', theme: 'dark' }) }))
    await api.me(false); await consoleApi.me(false)
    await api.preferences({ locale: 'en', theme: 'dark' }); await consoleApi.preferences({ locale: 'en', theme: 'dark' })
    api.forgetSession(); await consoleApi.preferences({ locale: 'en', theme: 'dark' })
    await api.me(false); consoleApi.forgetSession(); await api.preferences({ locale: 'en', theme: 'dark' })
  })
  it('passes a repair token only to repair and never remembers it globally or triggers unauthorized', async () => {
    const proof = crypto.randomUUID(), unauthorized = vi.fn()
    signedIn = true; await consoleApi.me(false); onUnauthorized(unauthorized)
    server.use(http.post('*/api/v1/console/recovery', () => success(recovery({ csrf_token: proof }))),
      http.put('*/api/v1/console/settings/security', ({ request }) => { expect(request.headers.get('X-CSRF-Token')).toBe(proof); return failure(401, 'UNAUTHENTICATED') }),
      http.patch('*/api/v1/console/preferences', ({ request }) => { expect(request.headers.get('X-CSRF-Token')).toBe(session.csrf_token); return success({ locale: 'en', theme: 'dark' }) }))
    await consoleApi.recover(crypto.randomUUID())
    await expect(consoleApi.repair(proof, { expected_version: 1, value: { turnstile_site_key: 'site', allowed_hostname: 'pay.example.test' } })).rejects.toMatchObject({ status: 401 })
    expect(unauthorized).not.toHaveBeenCalled()
    await consoleApi.preferences({ locale: 'en', theme: 'dark' })
  })
  it('expires only the failed role token on protected 401', async () => {
    const merchant = { ...session, csrf_token: crypto.randomUUID() }, unauthorized = vi.fn()
    signedIn = true
    server.use(http.get('*/api/v1/merchant/me', () => success(merchant)), http.patch('*/api/v1/console/preferences', () => failure(401, 'UNAUTHENTICATED')),
      http.patch('*/api/v1/merchant/preferences', ({ request }) => { expect(request.headers.get('X-CSRF-Token')).toBe(merchant.csrf_token); return success({ locale: 'en', theme: 'dark' }) }))
    await api.me(false); await consoleApi.me(false); onUnauthorized(unauthorized)
    await expect(consoleApi.preferences({ locale: 'en', theme: 'dark' })).rejects.toMatchObject({ status: 401 })
    expect(unauthorized).toHaveBeenCalledTimes(1)
    await api.preferences({ locale: 'en', theme: 'dark' })
    server.use(http.patch('*/api/v1/console/preferences', ({ request }) => { expect(request.headers.get('X-CSRF-Token')).toBeNull(); return success({ locale: 'en', theme: 'dark' }) }))
    await consoleApi.preferences({ locale: 'en', theme: 'dark' })
  })
})

describe('console guards and login', () => {
  it.each([401, 403])('redirects console %i to console login with a safe redirect', async (status) => {
    server.use(http.get('*/api/v1/console/me', () => failure(status, status === 403 ? 'FORBIDDEN' : 'UNAUTHENTICATED')))
    const { router } = mount('/console/not-implemented?state=paid')
    await screen.findByRole('heading', { name: '管理员登录' })
    expect(router.state.location.pathname).toBe('/console/login')
    expect(router.state.location.search).toMatchObject({ redirect: '/console/not-implemented?state=paid' })
    expect(screen.queryByRole('navigation', { name: '管理控制台' })).not.toBeInTheDocument()
  })
  it.each([401, 403])('redirects merchant %i to merchant login', async (status) => {
    server.use(http.get('*/api/v1/merchant/me', () => failure(status, status === 403 ? 'FORBIDDEN' : 'UNAUTHENTICATED')))
    const { router } = mount('/dashboard')
    await screen.findByLabelText('用户名 / 邮箱')
    expect(router.state.location.pathname).toBe('/login')
  })
  it('redirects uninitialized login to setup', async () => {
    server.use(http.get('*/api/v1/console/bootstrap-status', () => success({ initialized: false, turnstile_configured: true })))
    const { router } = mount()
    await screen.findByRole('heading', { name: '初始化管理员' })
    expect(router.state.location.pathname).toBe('/console/setup')
  })
  it('redirects initialized setup to login and explains it', async () => {
    mount('/console/setup')
    expect(await screen.findByText('平台已初始化，请登录。')).toBeVisible()
  })
  it('restores an existing console session and loads only console stats', async () => {
    signedIn = true; mount()
    expect(await screen.findByRole('heading', { name: new RegExp(user.username) })).toBeVisible()
    const links = within(screen.getByRole('navigation', { name: '管理控制台' })).getAllByRole('link')
    expect(links.map((link) => link.textContent)).toEqual(['概览', '商户', '订单', '套餐', '公告', '购买记录', '系统设置', '审计日志'])
  })
  it('handles a setup race', async () => {
    let initialized = false
    server.use(http.get('*/api/v1/console/bootstrap-status', () => success({ initialized, turnstile_configured: true })),
      http.post('*/api/v1/console/bootstrap', () => { initialized = true; return failure(409, 'ALREADY_INITIALIZED') }))
    mount('/console/setup')
    const actor = userEvent.setup(), password = crypto.randomUUID()
    await actor.type(await screen.findByLabelText('初始化码'), crypto.randomUUID())
    await actor.type(screen.getByLabelText('用户名', { exact: true }), 'valid_admin')
    await actor.type(screen.getByLabelText('密码', { exact: true }), password)
    await actor.type(screen.getByLabelText('确认新密码'), password)
    await actor.click(screen.getByRole('button', { name: '创建管理员' }))
    expect(await screen.findByText('平台已初始化，请登录。')).toBeVisible()
  })
  it('initializes with matching validation and follows the returned console session', async () => {
    const password = crypto.randomUUID(), code = crypto.randomUUID()
    server.use(http.get('*/api/v1/console/bootstrap-status', () => success({ initialized: false, turnstile_configured: true })),
      http.post('*/api/v1/console/bootstrap', async ({ request }) => {
        const body = await request.json() as Schema['AuthBootstrapRequest']
        expect(body.username).toBe('valid_admin'); expect(body.initialization_code === code && body.password === password && body.password_confirmation === password).toBe(true)
        signedIn = true; return success(session)
      }))
    const { router } = mount('/console/setup'), actor = userEvent.setup()
    expect(await screen.findByRole('button', { name: '创建管理员' })).toBeDisabled()
    await actor.type(screen.getByLabelText('初始化码'), code)
    await actor.type(screen.getByLabelText('用户名', { exact: true }), ' valid_admin ')
    await actor.type(screen.getByLabelText('密码', { exact: true }), password)
    await actor.type(screen.getByLabelText('确认新密码'), password)
    await actor.click(screen.getByRole('button', { name: '创建管理员' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/console'))
    expect(await screen.findByRole('heading', { name: new RegExp(user.username) })).toBeVisible()
  })
  it('routes a console protected-request 401 to console login', async () => {
    signedIn = true
    server.use(http.patch('*/api/v1/console/preferences', () => { signedIn = false; return failure(401, 'UNAUTHENTICATED') }))
    const { router } = mount('/console/account'), actor = userEvent.setup()
    await actor.click(await screen.findByRole('button', { name: '深色' }))
    await screen.findByRole('heading', { name: '管理员登录' })
    expect(router.state.location.pathname).toBe('/console/login')
    expect(router.state.location.search).toMatchObject({ redirect: '/console/account' })
  })
  it.each(['totp', 'recovery'])('uses the shared login and console second factor (%s)', async (mode) => {
    const challenge = crypto.randomUUID()
    server.use(http.post('*/api/v1/console/login', () => success({ requires_2fa: true, challenge_id: challenge, expires_at: session.expires_at })),
      http.post('*/api/v1/console/second-factor', async ({ request }) => {
        const body = await request.json() as Schema['AuthFactorRequest']
        expect(body.challenge_id).toBe(challenge)
        expect(mode === 'totp' ? body.totp_code : body.recovery_code).toBeTruthy()
        expect(mode === 'totp' ? body.recovery_code : body.totp_code).toBeUndefined()
        signedIn = true; return success(session)
      }))
    const { router } = mount('/console/login?redirect=%2Fconsole%2Fnot-implemented')
    const actor = await enterLogin()
    if (mode === 'recovery') await actor.click(await screen.findByRole('button', { name: '恢复码' }))
    await actor.type(await screen.findByLabelText(mode === 'totp' ? '动态验证码' : '恢复码'), mode === 'totp' ? String(100000 + Math.floor(Math.random() * 900000)) : crypto.randomUUID())
    await actor.click(screen.getByRole('button', { name: '验证并登录' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/console/not-implemented'))
    expect(await screen.findByText('页面不存在')).toBeVisible()
    expect(screen.getByRole('navigation', { name: '管理控制台' })).toBeVisible()
  })
  it('shows recovery instead of merchant registration/reset links when Turnstile is unavailable', async () => {
    server.use(http.get('*/api/v1/public/config', () => success({ ...config, turnstile: { ...config.turnstile, configured: false } })))
    mount()
    expect(await screen.findByRole('link', { name: '恢复管理端访问' })).toHaveAttribute('href', '/console/recovery')
    expect(screen.queryByRole('link', { name: /注册|忘记密码/ })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '登录' })).toBeDisabled()
  })
  it('points administrators to recovery instead of a platform administrator when setup is required', async () => {
    server.use(http.get('*/api/v1/public/config', () => success({ ...config, turnstile: { ...config.turnstile, configured: false } })))
    mount()
    expect(await screen.findByText('站点尚未完成 Turnstile 配置，请通过页面底部的「恢复管理端访问」修复。')).toBeVisible()
    expect(screen.queryByText(/联系平台管理员/)).not.toBeInTheDocument()
  })
})

describe('repair-only recovery', () => {
  it('refreshes the Turnstile key before going to login in the same document', async () => {
    const result = recovery({ security: { turnstile_site_key: '', allowed_hostname: '', turnstile_secret_configured: false } })
    let published = { ...config.turnstile, configured: false, site_key: '' }
    const fetched: string[] = []
    server.use(
      http.get('*/api/v1/public/config', () => {
        fetched.push(published.site_key)
        return success({ ...config, turnstile: published })
      }),
      http.post('*/api/v1/console/recovery', () => success(result)),
      http.put('*/api/v1/console/settings/security', async ({ request }) => {
        const { value } = await request.json() as { value: { turnstile_site_key: string } }
        published = { ...published, configured: true, site_key: value.turnstile_site_key }
        return success({ version: 8, value: {} })
      }),
    )
    const { router, client } = mount('/console/recovery')
    await waitFor(() => expect(client.getQueryData(publicConfigQuery.queryKey)?.turnstile.configured).toBe(false))
    const actor = await enterRecovery()
    await actor.type(screen.getByLabelText('Turnstile Site Key'), 'repaired-site')
    await actor.type(screen.getByLabelText('允许的主机名'), 'pay.example.test')
    await actor.type(screen.getByLabelText('Turnstile Secret'), crypto.randomUUID())
    await actor.click(screen.getByRole('button', { name: '保存并结束修复' }))

    expect(await screen.findByText('已修复，请登录。')).toBeVisible()
    expect(router.state.location.pathname).toBe('/console/login')
    const captcha = await screen.findByRole('button', { name: '完成人机验证' })
    expect(captcha).toHaveAttribute('data-site-key', 'repaired-site')
    await actor.type(screen.getByLabelText('用户名 / 邮箱'), user.username)
    await actor.type(screen.getByLabelText('密码', { exact: true }), crypto.randomUUID())
    await actor.click(captcha)
    expect(screen.getByRole('button', { name: '登录' })).toBeEnabled()
    expect(fetched).toEqual(['', 'repaired-site'])
  })

  it.each([[true, 'Secret 已配置，留空保留原值。'], [false, '尚未配置 Secret，本次必须填写。']] as const)('describes the secret field when configured=%s', async (configured, hint) => {
    server.use(http.post('*/api/v1/console/recovery', () => success(recovery({ security: { turnstile_site_key: 'public-site', allowed_hostname: 'pay.example.test', turnstile_secret_configured: configured } }))))
    mount('/console/recovery')
    await enterRecovery()
    expect(screen.getByLabelText('Turnstile Secret')).toHaveAccessibleDescription(hint)
    expect(screen.queryAllByRole('status').some((node) => node.textContent?.includes(hint))).toBe(false)
  })
  it.each([401, 409])('clears invalid proof on %i without a global login redirect', async (status) => {
    const result = recovery()
    server.use(http.post('*/api/v1/console/recovery', () => success(result)), http.put('*/api/v1/console/settings/security', () => failure(status, status === 409 ? 'VERSION_CONFLICT' : 'UNAUTHENTICATED')))
    const { router } = mount('/console/recovery')
    const actor = await enterRecovery()
    await actor.click(screen.getByRole('button', { name: '保存并结束修复' }))
    expect(await screen.findByLabelText('终端恢复码')).toHaveValue('')
    expect(screen.getByText('修复会话已失效或配置版本已变化，请重新生成恢复码。')).toBeVisible()
    expect(router.state.location.pathname).toBe('/console/recovery')
    expect(screen.queryByLabelText('Turnstile Site Key')).not.toBeInTheDocument()
  })
  it('requires a missing secret, omits a preserved secret, prevents duplicates and clears proof on success', async () => {
    const result = recovery(), requests: string[] = []
    server.use(http.post('*/api/v1/console/recovery', () => success(result)), http.put('*/api/v1/console/settings/security', async ({ request }) => {
      requests.push(request.headers.get('X-CSRF-Token') ?? '')
      expect(await request.json()).toEqual({ expected_version: 7, value: { turnstile_site_key: 'public-site', allowed_hostname: 'pay.example.test' } })
      await delay(100); return success({ version: 8, value: {} })
    }))
    const { router } = mount('/console/recovery'); const actor = await enterRecovery()
    await actor.dblClick(screen.getByRole('button', { name: '保存并结束修复' }))
    await screen.findByText('已修复，请登录。')
    expect(requests).toEqual([result.csrf_token])
    expect(JSON.stringify(localStorage)).not.toContain(result.csrf_token)
    expect(screen.queryByLabelText('Turnstile Secret')).not.toBeInTheDocument()
    // A fresh component cannot submit the previous in-memory proof.
    await act(async () => { await router.navigate({ to: '/console/recovery' }) })
    await screen.findByLabelText('终端恢复码')
    expect(screen.queryByRole('button', { name: '保存并结束修复' })).not.toBeInTheDocument()
  })
  it('requires a new secret when none is configured', async () => {
    const result = recovery(); result.security.turnstile_secret_configured = false
    server.use(http.post('*/api/v1/console/recovery', () => success(result)))
    mount('/console/recovery'); const actor = await enterRecovery()
    expect(screen.getByRole('button', { name: '保存并结束修复' })).toBeDisabled()
    await actor.type(screen.getByLabelText('Turnstile Secret'), crypto.randomUUID())
    expect(screen.getByRole('button', { name: '保存并结束修复' })).toBeEnabled()
  })
  it('expires, discards proof and returns to step one', async () => {
    vi.useFakeTimers()
    server.use(http.post('*/api/v1/console/recovery', () => success(recovery({ expires_at: new Date(Date.now() + 2000).toISOString() }))))
    mount('/console/recovery')
    await act(async () => { await vi.advanceTimersByTimeAsync(100) })
    fireEvent.change(screen.getByLabelText('终端恢复码'), { target: { value: crypto.randomUUID() } })
    fireEvent.submit(screen.getByRole('button', { name: '验证恢复码' }).closest('form')!)
    await act(async () => { await vi.advanceTimersByTimeAsync(100) })
    expect(screen.getByLabelText('Turnstile Site Key')).toBeVisible()
    await act(async () => { await vi.advanceTimersByTimeAsync(2100) })
    expect(screen.getByLabelText('终端恢复码')).toHaveValue('')
    expect(screen.queryByRole('button', { name: '保存并结束修复' })).not.toBeInTheDocument()
    expect(screen.getByText('修复会话已过期，请重新生成恢复码。')).toBeVisible()
  })
  it('aborts a late recovery response on leave without retaining a token or navigating', async () => {
    let signal: AbortSignal | undefined
    server.use(http.post('*/api/v1/console/recovery', async ({ request }) => { signal = request.signal; await delay(100); return success(recovery()) }))
    const view = mount('/console/recovery'), actor = userEvent.setup()
    await actor.type(await screen.findByLabelText('终端恢复码'), crypto.randomUUID()); await actor.click(screen.getByRole('button', { name: '验证恢复码' }))
    await waitFor(() => expect(signal).toBeDefined()); view.unmount(); expect(signal!.aborted).toBe(true)
    await new Promise((resolve) => setTimeout(resolve, 150))
    expect(view.router.state.location.pathname).toBe('/console/recovery')
  })
})
