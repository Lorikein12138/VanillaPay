import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useCallback, useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { api, APIError, consoleApi } from '@/api/client'
import type { components } from '@/api/schema'
import { Notice } from '@/components/ui/notice'
import { Button } from '@/components/ui/button'
import { Field } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { TopBar } from '@/components/TopBar'
import { authErrorKey } from '@/features/auth/errors'
import { Turnstile } from '@/features/auth/Turnstile'
import { consoleDestination, loginDestination } from '@/lib/redirect'

type Challenge = components['schemas']['AuthLoginResult']

export function LoginPage({ redirect, notice, role = 'merchant' }: { redirect?: string; notice?: 'registered' | 'password_reset' | 'password_changed' | 'initialized' | 'repaired'; role?: 'merchant' | 'console' }) {
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
  const admin = role === 'console'
  const client = admin ? consoleApi : api
  const errorKey = <K extends string>(key: K) => (admin && key === 'auth.errors.setup' ? 'console.setupMissing' : key)
  const hero = t(admin ? 'console.hero' : 'auth.hero')
  const notices = { registered: 'auth.notices.registered', password_reset: 'auth.notices.password_reset', password_changed: admin ? 'console.notices.password_changed' : 'auth.notices.password_changed', initialized: 'console.notices.initialized', repaired: 'console.notices.repaired' } as const

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
        ? await client.secondFactor({ challenge_id: challenge.challenge_id, ...(mode === 'totp' ? { totp_code: code } : { recovery_code: code.trim() }) })
        : await client.login({ identifier: identifier.trim(), password, turnstile_token: token })
      if ('requires_2fa' in result) {
        setChallenge(result)
        setPassword('')
      } else {
        setPassword('')
        setCode('')
        await navigate({ to: (admin ? consoleDestination : loginDestination)(redirect, result.next_path), replace: true })
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
          <p className="mt-7 max-w-[440px] text-lg leading-relaxed text-text-2">{t(admin ? 'console.description' : 'auth.description')}</p>
          <ol className="mt-10 list-none border-t-[0.5px] border-border-strong p-0">
            {(['codes', 'devices', 'callbacks'] as const).map((key, index) => <li key={key} className="flex items-baseline gap-5 border-b-[0.5px] border-border py-4"><span className="font-mono text-caption text-text-3">0{index + 1}</span><span className="text-label">{t(`${admin ? 'console' : 'auth'}.features.${key}`)}</span></li>)}
          </ol>
        </section>
        <form onSubmit={(event) => { void submit(event) }} noValidate aria-busy={busy} aria-labelledby="login-title" className="flex w-full min-w-0 flex-col gap-5 rounded-card border-[0.5px] border-border bg-surface p-5 sm:p-10">
          <div><h2 id="login-title" className="m-0 font-serif text-title font-normal">{t(challenge ? 'auth.factorTitle' : admin ? 'console.login' : 'auth.welcome')}</h2><p className="mt-1 mb-0 text-label text-text-2">{t(challenge ? 'auth.factorDescription' : admin ? 'console.loginDescription' : 'auth.subtitle')}</p></div>
          {notice && <Notice>{t(notices[notice])}</Notice>}
          {!challenge ? <>
            <Field label={t('auth.identifier')} htmlFor="identifier"><Input id="identifier" name="username" autoComplete="username" value={identifier} onChange={(event) => setIdentifier(event.target.value)} placeholder={t('auth.identifierPlaceholder')} disabled={busy} aria-invalid={!!fields.identifier} aria-describedby={fields.identifier ? 'identifier-error' : undefined} /></Field>

            <Field label={t('auth.password')} htmlFor="password"><Input id="password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} disabled={busy} aria-invalid={!!fields.password} aria-describedby={fields.password ? 'password-error' : undefined} /></Field>

            {!admin && <a href="/reset-password" className="inline-flex min-h-11 items-center justify-end text-label text-accent-text">{t('auth.forgotPassword')}</a>}
            {config.data?.turnstile.configured && !setupMissing && <Turnstile key={attempt} siteKey={config.data.turnstile.site_key} action={config.data.turnstile.login_action} onToken={onToken} onError={onCaptchaError} />}
            {config.isPending && <p role={notice ? undefined : 'status'} className="m-0 text-label text-text-2">{t('auth.loading')}</p>}
            {config.error && <><Button variant="ghost" onClick={() => { void config.refetch() }}>{t('error.retry')}</Button></>}

            {captchaError && <><Button variant="ghost" onClick={() => { setCaptchaError(false); setAttempt((value) => value + 1) }}>{t('error.retry')}</Button></>}
          </> : <>
            <div role="group" aria-label={t('auth.factorMethod')} className="flex rounded-control border-[0.5px] border-border p-1">{(['totp', 'recovery'] as const).map((method) => <Button key={method} variant="ghost" aria-pressed={mode === method} disabled={busy} className={`min-h-11 flex-1 rounded-control px-2 text-label ${mode === method ? 'bg-hover text-text' : 'text-text-2'}`} onClick={() => { setMode(method); setCode(''); setFields({}); setProblem(undefined) }}>{t(`auth.${method}`)}</Button>)}</div>
            <Field label={t(mode === 'totp' ? 'auth.totpCode' : 'auth.recoveryCode')} htmlFor="factor-code"><Input id="factor-code" autoComplete="one-time-code" inputMode={mode === 'totp' ? 'numeric' : 'text'} value={code} onChange={(event) => setCode(event.target.value)} disabled={busy} aria-invalid={!!fields.code} aria-describedby={fields.code ? 'code-error' : undefined} /></Field>

          </>}
          {(problem != null || config.error || setupMissing || captchaError || Object.values(fields).some(Boolean)) && <Notice tone="error">{setupMissing ? t(errorKey('auth.errors.setup')) : captchaError ? t('auth.errors.turnstile') : problem != null ? <>{t(errorKey(authErrorKey(problem)))}{fields.identifier && <p id="identifier-error">{t('auth.requiredIdentifier')}</p>}{fields.password && <p id="password-error">{t('auth.requiredPassword')}</p>}{fields.code && <p id="code-error">{t(mode === 'totp' ? 'auth.requiredTOTP' : 'auth.requiredRecovery')}</p>}</> : config.error ? t(errorKey(authErrorKey(config.error))) : <>{fields.identifier && <p id="identifier-error">{t('auth.requiredIdentifier')}</p>}{fields.password && <p id="password-error">{t('auth.requiredPassword')}</p>}{fields.code && <p id="code-error">{t(mode === 'totp' ? 'auth.requiredTOTP' : 'auth.requiredRecovery')}</p>}</>}</Notice>}
          <Button type="submit" variant="primary" loading={busy} disabled={busy || (!challenge && (!token || setupMissing || config.isPending))}>{t(busy ? 'auth.submitting' : challenge ? 'auth.verify' : 'auth.submit')}</Button>
          {challenge && <Button variant="ghost" disabled={busy} onClick={() => { setChallenge(undefined); setCode(''); setProblem(undefined); setFields({}); setToken(''); setAttempt((value) => value + 1) }}>{t('auth.restart')}</Button>}
        </form>
      </main>
      <footer className="flex flex-wrap items-center justify-between gap-3 border-t-[0.5px] border-border px-4 py-6 text-caption text-text-3 sm:px-8"><span>© {config.data?.site_name ?? t('common.brand')}</span>{admin ? <a href="/console/recovery">{t('console.recovery')}</a> : config.data?.registration_open ? <a href="/register" className="inline-flex min-h-11 items-center rounded-control px-2 text-accent-text hover:bg-hover">{t('auth.register')}</a> : <span>{t('auth.contactAdmin')}</span>}</footer>
    </div>
  )
}
