import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useCallback, useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { api, APIError } from '@/api/client'
import type { components } from '@/api/schema'
import { FormError } from '@/components/FormError'
import { TopBar } from '@/components/TopBar'
import { authErrorKey } from '@/features/auth/errors'
import { Turnstile } from '@/features/auth/Turnstile'
import { loginDestination } from '@/lib/redirect'

const inputClass = 'h-11 w-full min-w-0 rounded-control border border-border-strong bg-surface px-3 text-body text-text'
const buttonClass = 'inline-flex min-h-11 items-center justify-center rounded-control bg-primary px-5 text-label font-medium text-primary-fg transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50'
type Challenge = components['schemas']['AuthLoginResult']

export function LoginPage({ redirect, notice }: { redirect?: string; notice?: 'registered' | 'password_reset' | 'password_changed' }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const config = useQuery({ queryKey: ['public-config'], queryFn: api.config })
  const [identifier, setIdentifier] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [mode, setMode] = useState<'totp' | 'recovery'>('totp')
  const [challenge, setChallenge] = useState<Challenge>()
  const [token, setToken] = useState('')
  const [attempt, setAttempt] = useState(0)
  const [busy, setBusy] = useState(false)
  const submitting = useRef(false)
  const [problem, setProblem] = useState<unknown>()
  const [fields, setFields] = useState<Record<string, boolean>>({})
  const [captchaError, setCaptchaError] = useState(false)
  const onToken = useCallback((value: string) => { setToken(value); if (value) setCaptchaError(false) }, [])
  const onCaptchaError = useCallback(() => { setCaptchaError(true); setToken('') }, [])
  const setupMissing = config.data?.turnstile.configured === false || (problem instanceof APIError && problem.problem?.code === 'SETUP_REQUIRED')
  const hero = t('auth.hero')

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (submitting.current) return
    const invalid: Record<string, boolean> = challenge
      ? { code: mode === 'totp' ? !/^\d{6}$/.test(code) : !code.trim() }
      : { identifier: !identifier.trim(), password: !password }
    setFields(invalid)
    setProblem(undefined)
    if (Object.values(invalid).some(Boolean) || (!challenge && !token)) return
    submitting.current = true
    setBusy(true)
    try {
      const result = challenge
        ? await api.secondFactor({ challenge_id: challenge.challenge_id, ...(mode === 'totp' ? { totp_code: code } : { recovery_code: code.trim() }) })
        : await api.login({ identifier: identifier.trim(), password, turnstile_token: token })
      if ('requires_2fa' in result) {
        setChallenge(result)
        setPassword('')
      } else {
        setPassword('')
        setCode('')
        await navigate({ to: loginDestination(redirect, result.next_path), replace: true })
      }
    } catch (error) {
      setProblem(error)
      if (error instanceof APIError && error.problem?.fields) {
        const names = Object.keys(error.problem.fields)
        setFields(Object.fromEntries(names.map((name) => [name === 'totp_code' || name === 'recovery_code' ? 'code' : name, true])))
      }
      if (!challenge) { setToken(''); setAttempt((value) => value + 1) }
    } finally {
      submitting.current = false
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-dvh flex-col">
      <TopBar siteName={config.data?.site_name} />
      <main id="main" className="mx-auto grid w-full max-w-[1160px] flex-1 items-center gap-12 px-4 pt-12 pb-16 sm:px-8 lg:grid-cols-[1fr_400px] lg:gap-20 lg:pt-16 lg:pb-24">
        <section className="min-w-0">
          <h1 aria-label={hero.replaceAll('\n', '')} className="m-0 max-w-[520px] font-serif text-[clamp(2.5rem,5vw,3.5rem)] leading-[1.1] font-[330] tracking-normal text-balance">{hero.split('\n').map((line) => <span key={line} className={hero.includes('\n') ? 'block whitespace-nowrap' : undefined}>{line}</span>)}</h1>
          <p className="mt-7 max-w-[440px] text-lg leading-relaxed text-text-2">{t('auth.description')}</p>
          <ol className="mt-10 list-none border-t-[0.5px] border-border-strong p-0">
            {(['codes', 'devices', 'callbacks'] as const).map((key, index) => <li key={key} className="flex items-baseline gap-5 border-b-[0.5px] border-border py-4"><span className="font-mono text-caption text-text-3">0{index + 1}</span><span className="text-label">{t(`auth.features.${key}`)}</span></li>)}
          </ol>
        </section>
        <form onSubmit={(event) => { void submit(event) }} noValidate aria-busy={busy} aria-labelledby="login-title" className="flex w-full min-w-0 flex-col gap-5 rounded-card border-[0.5px] border-border bg-surface p-10">
          <div><h2 id="login-title" className="m-0 font-serif text-title font-normal">{t(challenge ? 'auth.factorTitle' : 'auth.welcome')}</h2><p className="mt-1 mb-0 text-label text-text-2">{t(challenge ? 'auth.factorDescription' : 'auth.subtitle')}</p></div>
          {notice && <p role="status" className="text-label text-accent-text">{t(`auth.notices.${notice}`)}</p>}
          {!challenge ? <>
            <label className="flex flex-col gap-2 text-label text-text-2" htmlFor="identifier">{t('auth.identifier')}<input id="identifier" name="username" autoComplete="username" value={identifier} onChange={(event) => setIdentifier(event.target.value)} placeholder={t('auth.identifierPlaceholder')} disabled={busy} className={inputClass} aria-invalid={!!fields.identifier} aria-describedby={fields.identifier ? 'identifier-error' : undefined} /></label>
            {fields.identifier && <FormError id="identifier-error">{t('auth.requiredIdentifier')}</FormError>}
            <label className="flex flex-col gap-2 text-label text-text-2" htmlFor="password">{t('auth.password')}<input id="password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} disabled={busy} className={inputClass} aria-invalid={!!fields.password} aria-describedby={fields.password ? 'password-error' : undefined} /></label>
            {fields.password && <FormError id="password-error">{t('auth.requiredPassword')}</FormError>}
            <a href="/reset-password" className="inline-flex min-h-11 items-center justify-end text-label text-accent-text">{t('auth.forgotPassword')}</a>
            {config.data?.turnstile.configured && !setupMissing && <Turnstile key={attempt} siteKey={config.data.turnstile.site_key} action={config.data.turnstile.login_action} onToken={onToken} onError={onCaptchaError} />}
            {config.isPending && <p role="status" className="m-0 text-label text-text-2">{t('auth.loading')}</p>}
            {config.error && <><FormError>{t(authErrorKey(config.error))}</FormError><button type="button" className="min-h-11 rounded-control text-label text-accent-text hover:bg-hover" onClick={() => { void config.refetch() }}>{t('error.retry')}</button></>}
            {setupMissing && <FormError>{t('auth.errors.setup')}</FormError>}
            {captchaError && <><FormError>{t('auth.errors.turnstile')}</FormError><button type="button" className="min-h-11 rounded-control text-label text-accent-text hover:bg-hover" onClick={() => { setCaptchaError(false); setAttempt((value) => value + 1) }}>{t('error.retry')}</button></>}
          </> : <>
            <div role="group" aria-label={t('auth.factorMethod')} className="flex rounded-control border-[0.5px] border-border p-1">{(['totp', 'recovery'] as const).map((method) => <button key={method} type="button" aria-pressed={mode === method} disabled={busy} className={`min-h-11 flex-1 rounded-control px-2 text-label ${mode === method ? 'bg-hover text-text' : 'text-text-2'}`} onClick={() => { setMode(method); setCode(''); setFields({}); setProblem(undefined) }}>{t(`auth.${method}`)}</button>)}</div>
            <label htmlFor="factor-code" className="flex flex-col gap-2 text-label text-text-2">{t(mode === 'totp' ? 'auth.totpCode' : 'auth.recoveryCode')}<input id="factor-code" autoComplete="one-time-code" inputMode={mode === 'totp' ? 'numeric' : 'text'} value={code} onChange={(event) => setCode(event.target.value)} disabled={busy} className={inputClass} aria-invalid={!!fields.code} aria-describedby={fields.code ? 'code-error' : undefined} /></label>
            {fields.code && <FormError id="code-error">{t(mode === 'totp' ? 'auth.requiredTOTP' : 'auth.requiredRecovery')}</FormError>}
          </>}
          {problem != null && !setupMissing && <FormError>{t(authErrorKey(problem))}</FormError>}
          <button type="submit" className={buttonClass} disabled={busy || (!challenge && (!token || setupMissing || config.isPending))}>{t(busy ? 'auth.submitting' : challenge ? 'auth.verify' : 'auth.submit')}</button>
          {challenge && <button type="button" disabled={busy} className="min-h-11 rounded-control text-label text-text-2 hover:bg-hover" onClick={() => { setChallenge(undefined); setCode(''); setProblem(undefined); setFields({}); setToken(''); setAttempt((value) => value + 1) }}>{t('auth.restart')}</button>}
        </form>
      </main>
      <footer className="flex flex-wrap items-center justify-between gap-3 border-t-[0.5px] border-border px-4 py-6 text-caption text-text-3 sm:px-8"><span>© {config.data?.site_name ?? t('common.brand')}</span>{config.data?.registration_open ? <a href="/register" className="inline-flex min-h-11 items-center rounded-control px-2 text-accent-text hover:bg-hover">{t('auth.register')}</a> : <span>{t('auth.contactAdmin')}</span>}</footer>
    </div>
  )
}
