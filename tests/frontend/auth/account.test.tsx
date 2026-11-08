import { emptyNotifications } from '../merchant/notification-fixture'
import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { delay, http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { useEffect, useState, type ReactNode } from 'react'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { APIError, api, onUnauthorized } from '@/api/client'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import { allowedEmail, type AccountSchema } from '@/features/auth/account-shared'
import { FactorDialog } from '@/features/merchant/AccountPage'
import { isMerchantPath } from '@/features/merchant/navigation'
import { createI18n } from '@/lib/i18n'
import { initTheme } from '@/lib/theme'

const captcha = vi.hoisted(() => ({ mounts: 0 }))
vi.mock('@/features/auth/Turnstile', () => ({ Turnstile: ({ onToken, action }: { onToken: (value: string) => void; action: string }) => {
  useEffect(() => { captcha.mounts++ }, [])
  return <button type="button" data-action={action} onClick={() => onToken(crypto.randomUUID())}>完成验证</button>
} }))
vi.mock('qrcode', () => ({ default: { toDataURL: () => Promise.resolve('data:image/png;base64,AA==') } }))
const baseUser: AccountSchema['AuthUserDTO'] = { id: '1', role: 'merchant', username: 'account_shop', email: 'account@example.test', pid: '9000000000000001', banned: false, banned_at: null, ban_reason: null, totp_enabled: false, locale: 'zh-CN', theme: 'light', created_at: new Date().toISOString() }
const identity = (user = baseUser): AccountSchema['SessionIdentity'] => ({ user, csrf_token: crypto.randomUUID(), expires_at: new Date(Date.now() + 3600000).toISOString() })
const config: AccountSchema['PublicConfig'] = { site_name: 'VanillaPay', logo_url: '', turnstile: { configured: true, site_key: 'fixture', login_action: 'login', register_action: 'register' }, registration_open: true, email_domain_restriction: { enabled: false, domains: [] }, legal_revisions: { terms: 7, privacy: 9 }, home: { mode: 'default', html: null, url: null }, footer: [] }
const success = <T,>(data: T) => HttpResponse.json({ data, error: null, request_id: crypto.randomUUID() })
const failure = (status: number, code: AccountSchema['ErrorCode'], details = {}) => HttpResponse.json({ error: { code, message: code, fields: {}, details }, request_id: crypto.randomUUID() }, { status })
const server = setupServer(emptyNotifications, http.get('*/api/v1/public/config', () => success(config)), http.get('*/api/v1/merchant/me', () => success(identity())))
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => { server.resetHandlers(); vi.useRealTimers() })
afterAll(() => server.close())
function wrapper(children: ReactNode) {
  initTheme()
  const client = createQueryClient()
  return { client, ...render(<AppProviders i18n={createI18n('zh-CN')} queryClient={client}>{children}</AppProviders>) }
}
function mount(path: string) {
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) })
  return { router, ...wrapper(<RouterProvider router={router} />) }
}
const fill = (label: string, value: string) => fireEvent.change(screen.getByLabelText(label, { exact: true }), { target: { value } })

describe('public account flows', () => {
  it('does not render a registration form when closed', async () => {
    server.use(http.get('*/api/v1/public/config', () => success({ ...config, registration_open: false })))
    mount('/register')
    expect(await screen.findByText('注册未开放')).toBeVisible()
    expect(screen.queryByRole('form')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('新密码')).not.toBeInTheDocument()
  })
  it('shows exact email domain restrictions and prevents sending to subdomains', async () => {
    const restriction = { enabled: true, domains: ['example.test'] }
    server.use(http.get('*/api/v1/public/config', () => success({ ...config, email_domain_restriction: restriction })))
    mount('/register')
    await screen.findByText(/允许的邮箱域名/)
    fill('用户名', 'new_shop'); fill('邮箱', 'shop@sub.example.test')
    fireEvent.click(screen.getByRole('button', { name: '完成验证' }))
    expect(screen.getByRole('button', { name: '获取邮箱验证码' })).toBeDisabled()
    fill('邮箱', 'shop@example.test')
    expect(screen.getByRole('button', { name: '获取邮箱验证码' })).toBeEnabled()
    expect(allowedEmail('shop@any.test', { enabled: true, domains: [] })).toBe(true)
    expect(allowedEmail('shop@example.test.', restriction)).toBe(true)
    expect(allowedEmail('shop@例子.test', { enabled: true, domains: ['xn--fsqu00a.test'] })).toBe(true)
  })
  it('consumes separate Turnstile tokens for email and registration, uses live legal revisions and resets after failure', async () => {
    const requests: AccountSchema['AuthEmailCodeRequest'][] = []
    let registration: AccountSchema['AuthRegisterRequest'] | undefined
    server.use(http.post('*/api/v1/auth/email-codes', async ({ request }) => { requests.push(await request.json() as AccountSchema['AuthEmailCodeRequest']); return success({ sent: true, retry_after_seconds: 3 }) }), http.post('*/api/v1/auth/register', async ({ request }) => { registration = await request.json() as AccountSchema['AuthRegisterRequest']; return failure(400, 'TURNSTILE_FAILED') }))
    mount('/register'); await screen.findByLabelText('用户名')
    fill('用户名', 'new_shop'); fill('邮箱', 'new@example.test')
    const secret = crypto.randomUUID(); fill('新密码', secret); fill('确认新密码', secret)
    fill('邮箱验证码', '123456')
    fireEvent.click(screen.getByRole('checkbox', { name: '我同意服务条款' })); fireEvent.click(screen.getByRole('checkbox', { name: '我同意隐私政策' }))
    expect(within(screen.getByRole('form', { name: '注册商户' })).getByRole('link', { name: '服务条款' })).toHaveAttribute('target', '_blank')
    fireEvent.click(screen.getByRole('button', { name: '完成验证' }))
    fireEvent.click(screen.getByRole('button', { name: '获取邮箱验证码' }))
    await screen.findByText('0:03')
    expect(requests[0]).toMatchObject({ purpose: 'register', email: 'new@example.test', target_username: 'new_shop' })
    expect(screen.getByRole('button', { name: '创建账户' })).toBeDisabled()
    const mounts = captcha.mounts
    fireEvent.click(screen.getByRole('button', { name: '完成验证' })); fireEvent.click(screen.getByRole('button', { name: '创建账户' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('人机验证未完成或已过期')
    expect(registration).toMatchObject({ username: 'new_shop', terms_revision: 7, privacy_revision: 9, accept_terms: true, accept_privacy: true })
    expect(registration?.turnstile_token).not.toBe(requests[0]?.turnstile_token)
    expect(captcha.mounts).toBeGreaterThan(mounts)
    expect(screen.getByRole('button', { name: '创建账户' })).toBeDisabled()
  })
  it('resets Turnstile after email failure and honors the 429 retry delay', async () => {
    server.use(http.post('*/api/v1/auth/email-codes', () => failure(429, 'EMAIL_SEND_TOO_SOON', { retry_after_seconds: 2 })))
    mount('/reset-password'); await screen.findByLabelText('邮箱')
    fill('邮箱', 'unknown@example.test'); fireEvent.click(screen.getByRole('button', { name: '完成验证' }))
    fireEvent.click(screen.getByRole('button', { name: '获取邮箱验证码' }))
    expect(await screen.findByText('0:02')).toBeDisabled()
    expect(screen.getByRole('button', { name: '确认重置密码' })).toBeDisabled()
    await screen.findByRole('button', { name: '获取邮箱验证码' }, { timeout: 3500 })
  })
  it('uses reset_password, clears cached sessions on success and shows a login notice', async () => {
    let sent: AccountSchema['AuthEmailCodeRequest'] | undefined
    server.use(http.post('*/api/v1/auth/email-codes', async ({ request }) => { sent = await request.json() as AccountSchema['AuthEmailCodeRequest']; return success({ sent: true, retry_after_seconds: 60 }) }), http.post('*/api/v1/auth/reset-password', () => success(null)))
    const { client, router } = mount('/reset-password'); client.setQueryData(['merchant', 'old', 'orders'], { private: true })
    await screen.findByLabelText('邮箱'); fill('邮箱', 'unknown@example.test')
    fireEvent.click(screen.getByRole('button', { name: '完成验证' })); fireEvent.click(screen.getByRole('button', { name: '获取邮箱验证码' }))
    await screen.findByText(/如邮箱可接收验证码/)
    expect(sent).toMatchObject({ purpose: 'reset_password', email: 'unknown@example.test' })
    const secret = crypto.randomUUID(); fill('新密码', secret); fill('确认新密码', secret); fill('邮箱验证码', '123456')
    fireEvent.click(screen.getByRole('button', { name: '完成验证' })); fireEvent.click(screen.getByRole('button', { name: '确认重置密码' }))
    expect(await screen.findByText(/密码已重置，旧会话已失效/)).toBeVisible()
    expect(router.state.location.pathname).toBe('/login')
    expect(client.getQueryData(['merchant', 'old', 'orders'])).toBeUndefined()
  })
  it.each(['known@example.test', 'unknown@example.test'])('does not reveal account existence for a failed reset: %s', async (email) => {
    server.use(http.post('*/api/v1/auth/reset-password', () => failure(400, 'INVALID_CODE')))
    mount('/reset-password'); await screen.findByLabelText('邮箱'); fill('邮箱', email)
    const password = crypto.randomUUID(); fill('新密码', password); fill('确认新密码', password); fill('邮箱验证码', '123456')
    fireEvent.click(screen.getByRole('button', { name: '完成验证' })); fireEvent.click(screen.getByRole('button', { name: '确认重置密码' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('验证码或恢复码错误，请重试。')
  })
  it('renders legal content as plain text and handles unpublished versions', async () => {
    const content = '<img src=x onerror=alert(1)>\n# Terms'
    server.use(http.get('*/api/v1/public/legal/terms', () => success({ kind: 'terms', revision: 3, updated_at: new Date().toISOString(), content: { zh_CN: content, en: '' } })))
    const { unmount } = mount('/legal/terms')
    expect(await screen.findByText(/<img src=x/)).toHaveTextContent('# Terms')
    expect(document.querySelector('main img')).toBeNull(); unmount()
    server.use(http.get('*/api/v1/public/legal/privacy', () => failure(404, 'NOT_FOUND')))
    mount('/legal/privacy'); expect(await screen.findByText('暂未发布')).toBeVisible()
  })
})

describe('identity and preferences', () => {
  it.each(['username', 'email', 'password'] as const)('assembles %s email proofs and requires TOTP when enabled', async (mode) => {
    const user = { ...baseUser, totp_enabled: true }, session = identity(user)
    const sent: AccountSchema['AuthEmailCodeRequest'][] = []
    let saved: unknown, csrf: string | null = null
    server.use(http.get('*/api/v1/merchant/me', () => success(session)), http.post('*/api/v1/auth/email-codes', async ({ request }) => { sent.push(await request.json() as AccountSchema['AuthEmailCodeRequest']); return success({ sent: true, retry_after_seconds: 60 }) }), http.patch(`*/api/v1/merchant/profile/${mode}`, async ({ request }) => { saved = await request.json(); csrf = request.headers.get('X-CSRF-Token'); return success(mode === 'password' ? null : user) }))
    const { client, router } = mount('/account')
    const label = mode === 'username' ? '用户名' : mode === 'email' ? '邮箱' : '密码'
    await screen.findByRole('heading', { name: '账户安全' }); fireEvent.click(screen.getByRole('button', { name: `修改${label}` }))
    const value = mode === 'username' ? 'new_shop' : mode === 'email' ? 'next@example.test' : crypto.randomUUID()
    fill(`新${label}`, value); if (mode === 'password') fill('确认新密码', value)
    fireEvent.click(screen.getByRole('button', { name: mode === 'email' ? '获取旧邮箱验证码' : '获取邮箱验证码' }))
    await waitFor(() => expect(sent).toHaveLength(1))
    expect(sent[0]).toMatchObject({ email: user.email, purpose: `change_${mode === 'email' ? 'email_old' : mode}`, ...(mode === 'username' ? { target_username: value } : { target_email: user.email }) })
    fill(mode === 'email' ? '旧邮箱验证码' : '邮箱验证码', '123456')
    if (mode === 'email') { fireEvent.click(screen.getByRole('button', { name: '获取新邮箱验证码' })); await waitFor(() => expect(sent).toHaveLength(2)); expect(sent[1]).toMatchObject({ purpose: 'change_email_new', email: value, target_email: value }); fill('新邮箱验证码', '654321') }
    expect(screen.getByRole('button', { name: '确认保存' })).toBeDisabled()
    fill('动态验证码', '123456')
    client.setQueryData(['merchant', 'private'], { secret: crypto.randomUUID() })
    fireEvent.click(screen.getByRole('button', { name: '确认保存' }))
    await waitFor(() => expect(saved).toBeDefined())
    expect(saved).toMatchObject(mode === 'email' ? { old_email_code: '123456', new_email_code: '654321', totp_code: '123456' } : { security_proof: { email_code: '123456', totp_code: '123456' } })
    expect(csrf).toBe(session.csrf_token)
    if (mode === 'password') { await waitFor(() => expect(router.state.location.pathname).toBe('/login')); expect(client.getQueryData(['merchant', 'private'])).toBeUndefined(); expect(await screen.findByText(/密码已修改，所有会话已失效/)).toBeVisible() }
    else await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })
  it('applies server preferences at entry and rolls back a failed header save with a visible error', async () => {
    server.use(http.patch('*/api/v1/merchant/preferences', async () => { await delay(100); return failure(500, 'INTERNAL_ERROR') }))
    mount('/account'); await screen.findByRole('heading', { name: '账户安全' })
    await waitFor(() => expect(document.documentElement).toHaveAttribute('data-theme', 'light'))
    fireEvent.click(screen.getByRole('button', { name: '深色' }))
    await waitFor(() => expect(document.documentElement).toHaveAttribute('data-theme', 'dark'))
    expect(await screen.findByRole('alert')).toHaveTextContent('服务暂时不可用')
    expect(document.documentElement).toHaveAttribute('data-theme', 'light')
    expect(localStorage.getItem('vp-theme')).toBe('light')
  })
  it('rolls language back after failure and persists account controls through the same endpoint', async () => {
    let saves = 0
    server.use(http.patch('*/api/v1/merchant/preferences', async ({ request }) => { saves++; if (saves === 1) return failure(500, 'INTERNAL_ERROR'); return success(await request.json()) }))
    mount('/account'); await screen.findByRole('heading', { name: '账户安全' })
    fireEvent.click(screen.getByRole('button', { name: '切换语言为 English' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('服务暂时不可用')
    expect(localStorage.getItem('vp-language')).toBe('zh-CN')
    fireEvent.change(screen.getByLabelText('账户主题'), { target: { value: 'dark' } })
    await waitFor(() => expect(screen.getByLabelText('账户主题')).toHaveValue('dark'))
    expect(localStorage.getItem('vp-theme')).toBe('dark')
  })
  it('recognizes /account, guards anonymous entry and redirects banned users', async () => {
    expect(isMerchantPath('/account')).toBe(true); expect(isMerchantPath('/accounting')).toBe(false)
    server.use(http.get('*/api/v1/merchant/me', () => failure(401, 'UNAUTHENTICATED')))
    const first = mount('/account'); await screen.findByLabelText('用户名 / 邮箱'); expect(first.router.state.location.pathname).toBe('/login'); first.unmount()
    server.use(http.get('*/api/v1/merchant/me', () => success(identity({ ...baseUser, banned: true }))), http.get('*/api/v1/merchant/banned', () => success({ banned: true, reason: 'restricted', banned_at: new Date().toISOString(), contact: { zh_CN: '联系管理员', en: '' } })))
    const second = mount('/account'); await waitFor(() => expect(second.router.state.location.pathname).toBe('/banned'))
    expect(screen.queryByRole('heading', { name: '账户安全' })).not.toBeInTheDocument()
  })
  it.each(['INVALID_CREDENTIALS', 'CHALLENGE_EXPIRED'] as const)('keeps valid sessions on proof error %s', async (code) => {
    const unauthorized = vi.fn(); onUnauthorized(unauthorized)
    server.use(http.post('*/api/v1/merchant/2fa/setup', () => failure(401, code)))
    await expect(api.setupTOTP({ password: crypto.randomUUID() })).rejects.toBeInstanceOf(APIError)
    expect(unauthorized).not.toHaveBeenCalled()
  })
})

describe('TOTP and one-time recovery secrets', () => {
  function setup(seconds = 300): AccountSchema['AuthTOTPSetup'] { return { setup_id: crypto.randomUUID(), secret: crypto.randomUUID(), otpauth_url: `otpauth://totp/account?secret=${crypto.randomUUID()}`, expires_at: new Date(Date.now() + seconds * 1000).toISOString() } }
  function Host() { const [open, setOpen] = useState(true); return <><button onClick={() => setOpen(true)}>打开设置</button>{open && <FactorDialog mode="setup" onClose={() => setOpen(false)} onChanged={() => Promise.resolve()} />}</> }
  async function generate() { fill('当前密码', crypto.randomUUID()); fireEvent.click(screen.getByRole('button', { name: '验证密码并生成' })); return screen.findByTestId('totp-secret') }
  it('destroys setup secrets and recovery codes on close without placing them in caches or storage', async () => {
    const value = setup(), codes = [crypto.randomUUID(), crypto.randomUUID()]
    vi.spyOn(api, 'setupTOTP').mockResolvedValue(value); vi.spyOn(api, 'confirmTOTP').mockResolvedValue({ enabled: true, recovery_codes: codes })
    const { client } = wrapper(<Host />)
    expect(await generate()).toHaveTextContent(value.secret)
    expect(screen.queryByLabelText('当前密码')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '关闭' })); fireEvent.click(screen.getByRole('button', { name: '打开设置' }))
    expect(screen.queryByTestId('totp-secret')).not.toBeInTheDocument(); expect(screen.getByLabelText('当前密码')).toHaveValue('')
    await generate(); fill('动态验证码', '123456'); fireEvent.click(screen.getByRole('button', { name: '确认开启' }))
    expect(await screen.findByTestId('recovery-codes')).toHaveTextContent(codes[0]!)
    expect(screen.queryByTestId('totp-secret')).not.toBeInTheDocument()
    expect(screen.queryByTestId('totp-qr')).not.toBeInTheDocument()
    const cache = JSON.stringify(client.getQueryCache().getAll()) + JSON.stringify(localStorage)
    for (const secret of [value.secret, value.otpauth_url, ...codes]) expect(cache).not.toContain(secret)
    expect(client.getMutationCache().getAll()).toHaveLength(0)
    fireEvent.click(screen.getByRole('button', { name: '关闭' })); fireEvent.click(screen.getByRole('button', { name: '打开设置' }))
    expect(screen.queryByTestId('recovery-codes')).not.toBeInTheDocument(); expect(screen.getByLabelText('当前密码')).toHaveValue('')
  })
  it.each(['setup', 'confirm'] as const)('ignores a late %s response after unmount', async (phase) => {
    const value = setup(), changed = vi.fn().mockResolvedValue(undefined)
    let resolveSetup: (value: AccountSchema['AuthTOTPSetup']) => void = () => {}, resolveConfirm: (value: AccountSchema['TOTPConfirmed']) => void = () => {}
    vi.spyOn(api, 'setupTOTP').mockImplementation(() => phase === 'setup' ? new Promise((resolve) => { resolveSetup = resolve }) : Promise.resolve(value))
    vi.spyOn(api, 'confirmTOTP').mockImplementation(() => new Promise((resolve) => { resolveConfirm = resolve }))
    const { unmount, client } = wrapper(<FactorDialog mode="setup" onClose={() => {}} onChanged={changed} />)
    fill('当前密码', crypto.randomUUID()); fireEvent.click(screen.getByRole('button', { name: '验证密码并生成' }))
    if (phase === 'confirm') { await screen.findByTestId('totp-secret'); fill('动态验证码', '123456'); fireEvent.click(screen.getByRole('button', { name: '确认开启' })) }
    unmount()
    await act(async () => { resolveSetup(value); resolveConfirm({ enabled: true, recovery_codes: [crypto.randomUUID()] }); await Promise.resolve() })
    expect(changed).not.toHaveBeenCalled(); expect(client.getMutationCache().getAll()).toHaveLength(0)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
  it('clears an expired setup and requires a new password proof', async () => {
    vi.spyOn(api, 'setupTOTP').mockResolvedValue(setup(1))
    wrapper(<Host />); await generate()
    await screen.findByText('设置已过期，请重新输入密码生成。', {}, { timeout: 2500 })
    expect(screen.queryByTestId('totp-secret')).not.toBeInTheDocument(); expect(screen.queryByTestId('totp-qr')).not.toBeInTheDocument()
    expect(screen.getByLabelText('当前密码')).toHaveValue('')
  })
  it('disables with password and TOTP only, never asks for email', async () => {
    const disable = vi.spyOn(api, 'disableTOTP').mockResolvedValue({ enabled: false }), close = vi.fn(), changed = vi.fn().mockResolvedValue(undefined)
    wrapper(<FactorDialog mode="disable" onClose={close} onChanged={changed} />)
    const password = crypto.randomUUID(); fill('当前密码', password)
    expect(screen.getByRole('button', { name: '确认关闭两步验证' })).toBeDisabled()
    fill('动态验证码', '123456'); fireEvent.click(screen.getByRole('button', { name: '确认关闭两步验证' }))
    await waitFor(() => expect(close).toHaveBeenCalledOnce())
    expect(disable).toHaveBeenCalledWith({ password, totp_code: '123456' }); expect(changed).toHaveBeenCalledOnce()
    expect(screen.queryByLabelText('邮箱验证码')).not.toBeInTheDocument()
  })
})
