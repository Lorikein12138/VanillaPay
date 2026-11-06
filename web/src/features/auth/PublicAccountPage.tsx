import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useCallback, useState, type ReactNode } from 'react'
import { api, APIError } from '@/api/client'
import { FormError } from '@/components/FormError'
import { TopBar } from '@/components/TopBar'
import { useLocalAction } from '@/features/merchant/CollectionParts'
import { orderPrimary } from '@/features/merchant/OrderParts'
import { safePath } from '@/lib/redirect'
import { AccountError, AccountField, allowedEmail, EmailCodeButton, useEmailCode, validPassword, validUsername } from './account-shared'
import { Turnstile } from './Turnstile'

export function PublicAccountShell({ title, children }: { title: string; children: ReactNode }) {
  return <div className="flex min-h-dvh flex-col"><TopBar /><main id="main" className="mx-auto grid w-full max-w-[1160px] flex-1 items-center gap-10 px-4 py-12 sm:px-8 lg:grid-cols-[1fr_440px] lg:gap-20"><section><p className="mb-5 font-mono text-caption uppercase tracking-widest text-accent-text">VanillaPay · 账户服务</p><h1 className="font-serif text-[clamp(2.5rem,5vw,3.5rem)] font-[330] leading-tight">{title}</h1><p className="mt-6 max-w-md text-lg leading-relaxed text-text-2">用自己的收款码，安心管理每一笔收款。保护账户，也保护你的生意。</p><a className="mt-8 inline-flex min-h-11 items-center text-label text-accent-text" href="/login">返回登录</a></section><section className="min-w-0 rounded-card border-[0.5px] border-border bg-surface p-5 sm:p-8">{children}</section></main></div>
}
export function PublicAccountPage({ mode }: { mode: 'register' | 'reset_password' }) {
  const config = useQuery({ queryKey: ['public-config'], queryFn: api.config })
  const title = mode === 'register' ? '创建商户账户' : '找回密码'
  return <PublicAccountShell title={title}>{config.isPending ? <p role="status">正在加载…</p> : config.error ? <><AccountError error={config.error} /><button className={orderPrimary} onClick={() => { void config.refetch() }}>重试</button></> : mode === 'register' && !config.data.registration_open ? <p role="status">注册未开放</p> : <PublicAccountForm mode={mode} config={config.data} />}</PublicAccountShell>
}
function PublicAccountForm({ mode, config }: { mode: 'register' | 'reset_password'; config: Awaited<ReturnType<typeof api.config>> }) {
  const register = mode === 'register', navigate = useNavigate(), client = useQueryClient()
  const action = useLocalAction(), mail = useEmailCode()
  const [username, setUsername] = useState(''), [email, setEmail] = useState(''), [code, setCode] = useState('')
  const [password, setPassword] = useState(''), [confirmation, setConfirmation] = useState('')
  const [terms, setTerms] = useState(false), [privacy, setPrivacy] = useState(false)
  const [token, setToken] = useState(''), [attempt, setAttempt] = useState(0), [captchaError, setCaptchaError] = useState(false), [invalid, setInvalid] = useState('')
  const onToken = useCallback((value: string) => { setToken(value); if (value) setCaptchaError(false) }, [])
  const onError = useCallback(() => { setToken(''); setCaptchaError(true) }, [])
  const busy = action.busy || mail.busy
  const emailValid = /^[^\s@]+@[^\s@]+$/.test(email.trim()) && (!register || allowedEmail(email, config.email_domain_restriction))
  function resetCaptcha() { if (action.active.current) { setToken(''); setAttempt((value) => value + 1) } }
  async function send() {
    if (!token || !emailValid || (register && !validUsername(username))) return
    try { await mail.send({ email: email.trim(), purpose: mode, turnstile_token: token, ...(register ? { target_username: username.trim() } : {}) }) }
    finally { resetCaptcha() }
  }
  async function submit() {
    if (!emailValid || (register && (!validUsername(username) || !terms || !privacy)) || !code.trim() || !validPassword(password, confirmation)) { setInvalid('请检查用户名、邮箱、验证码、协议，以及两次输入一致的 12–128 位密码。'); return }
    if (!token) return
    setInvalid('')
    await action.run(async () => {
      try {
        if (register) {
          const result = await api.register({ username: username.trim(), email: email.trim(), email_code: code.trim(), password, password_confirmation: confirmation, accept_terms: terms, accept_privacy: privacy, terms_revision: config.legal_revisions.terms, privacy_revision: config.legal_revisions.privacy, turnstile_token: token })
          if (action.active.current) { setPassword(''); setConfirmation(''); setCode(''); await navigate({ to: safePath(result.next_path) ?? '/login', search: { notice: 'registered' }, replace: true }) }
        } else {
          await api.resetPassword({ email: email.trim(), email_code: code.trim(), password, password_confirmation: confirmation, turnstile_token: token })
          api.forgetSession(); client.clear()
          if (action.active.current) { setPassword(''); setConfirmation(''); setCode(''); await navigate({ to: '/login', search: { notice: 'password_reset' }, replace: true }) }
        }
      } catch (error) {
        if (error instanceof APIError && error.problem?.code === 'LEGAL_VERSION_CHANGED') await client.invalidateQueries({ queryKey: ['public-config'] })
        throw error
      } finally { resetCaptcha() }
    })
  }
  return <form className="space-y-5" aria-label={register ? '注册商户' : '找回密码'} onSubmit={(event) => { event.preventDefault(); void submit() }}>
    <h2 className="font-serif text-2xl">{register ? '注册商户' : '设置新密码'}</h2>
    {register && <AccountField label="用户名" autoComplete="username" required disabled={busy} value={username} onChange={(event) => { setUsername(event.target.value); setCode('') }} pattern="[A-Za-z0-9_-]{3,32}" title="3–32 位字母、数字、下划线或连字符" />}
    <AccountField label="邮箱" type="email" autoComplete="email" required disabled={busy} value={email} onChange={(event) => { setEmail(event.target.value); setCode('') }} />
    {register && config.email_domain_restriction.enabled && !!config.email_domain_restriction.domains?.length && <p className="text-caption text-text-2">允许的邮箱域名：{config.email_domain_restriction.domains.join('、')}（不包含子域名）</p>}
    <EmailCodeButton mail={mail} disabled={busy || !emailValid || !token || (register && !validUsername(username))} onSend={() => { void send() }} />
    <AccountField label="邮箱验证码" autoComplete="one-time-code" inputMode="numeric" required disabled={busy} value={code} onChange={(event) => setCode(event.target.value)} />
    <AccountField label="新密码" type="password" autoComplete="new-password" required disabled={busy} value={password} onChange={(event) => setPassword(event.target.value)} />
    <AccountField label="确认新密码" type="password" autoComplete="new-password" required disabled={busy} value={confirmation} onChange={(event) => setConfirmation(event.target.value)} />
    <p className="text-caption text-text-3">密码为 12–128 个字符，两次输入须一致。</p>
    {register && <div className="space-y-3 text-label"><label className="flex min-h-11 items-center gap-2"><input type="checkbox" aria-label="我同意服务条款" required checked={terms} disabled={busy} onChange={(event) => setTerms(event.target.checked)} />我同意<a href="/legal/terms" target="_blank" rel="noopener noreferrer" className="text-accent-text underline">服务条款</a></label><label className="flex min-h-11 items-center gap-2"><input type="checkbox" aria-label="我同意隐私政策" required checked={privacy} disabled={busy} onChange={(event) => setPrivacy(event.target.checked)} />我同意<a href="/legal/privacy" target="_blank" rel="noopener noreferrer" className="text-accent-text underline">隐私政策</a></label></div>}
    {config.turnstile.configured ? <Turnstile key={attempt} siteKey={config.turnstile.site_key} action={register ? config.turnstile.register_action : 'reset_password'} onToken={onToken} onError={onError} /> : <FormError>站点尚未完成人机验证配置，请联系平台管理员。</FormError>}
    {captchaError && <><FormError>人机验证失败，请重新验证。</FormError><button type="button" className={orderPrimary} onClick={resetCaptcha}>重试验证</button></>}
    {invalid && <FormError>{invalid}</FormError>}{action.error != null && <AccountError error={action.error} />}
    <button type="submit" className={`${orderPrimary} w-full`} disabled={busy || !token}>{busy ? '提交中…' : register ? '创建账户' : '确认重置密码'}</button>
  </form>
}
