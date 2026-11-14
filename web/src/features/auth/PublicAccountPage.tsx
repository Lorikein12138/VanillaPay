import { clearPortalSession } from '@/app/portal-session'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useCallback, useState, type ReactNode } from 'react'
import { api, APIError } from '@/api/client'
import { PublicFooter } from '@/components/PublicFooter'
import { useSiteBrand } from '@/components/SiteBrand'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/input'
import { Notice } from '@/components/ui/notice'
import { TopBar } from '@/components/TopBar'
import { useLocalAction } from '@/features/merchant/CollectionParts'
import { publicConfigQuery, usePublicConfig } from '@/lib/public-config'
import { safePath } from '@/lib/redirect'
import { allowedEmail } from '@/lib/email-domain'
import { AccountError, AccountField, EmailCodeButton, EmailCodeNotice, useEmailCode, validPassword, validUsername } from './account-shared'
import { Turnstile } from './Turnstile'

export function PublicAccountShell({ title, children }: { title: string; children: ReactNode }) {
  const { t } = useTranslation()
  const { name } = useSiteBrand()
  return (
    <div className="flex min-h-dvh flex-col">
      <TopBar />
      <main id="main" className="mx-auto grid w-full max-w-[1160px] flex-1 items-center gap-10 px-4 py-12 sm:px-8 lg:grid-cols-[1fr_440px] lg:gap-20">
        <section className="min-w-0">
          <p className="mb-5 break-words text-label font-[500] text-accent-text">
            {t('polish.siteAccountServices', { name })}
          </p>
          <h1 className="font-serif text-[clamp(2.5rem,5vw,3.5rem)] font-normal leading-[1.08] tracking-[-0.02em] text-balance">{title}</h1>
          <p className="mt-6 max-w-md text-lg leading-relaxed text-text-2">{t('polish.manageEveryPaymentWithYourOwnQr')}</p>
          <a className="mt-8 inline-flex min-h-11 items-center text-label text-accent-text underline decoration-accent-text/30 underline-offset-4 hover:decoration-accent-text" href="/login">{t('polish.backToSignIn')}</a>
        </section>
        <section className="min-w-0 rounded-[24px] border-[0.5px] border-border bg-surface p-6 shadow-[0_1px_2px_rgba(20,20,19,0.03)] sm:p-9">{children}</section>
      </main>
      <PublicFooter />
    </div>
  )
}
export function PublicAccountPage({ mode }: { mode: 'register' | 'reset_password' }) {
  const { t } = useTranslation()
  const config = usePublicConfig()
  const title = mode === 'register' ? t('polish.createAMerchantAccount') : t('polish.resetPassword')
  return <PublicAccountShell title={title}>{config.isPending ? <p aria-live="polite">{t('polish.loading')}</p> : config.error ? <><AccountError error={config.error} /><Button variant="primary" onClick={() => { void config.refetch() }}>{t('polish.tryAgain')}</Button></> : mode === 'register' && !config.data.registration_open ? <Notice tone="info">{t('polish.registrationIsClosed')}</Notice> : <PublicAccountForm mode={mode} config={config.data} />}</PublicAccountShell>
}
function PublicAccountForm({ mode, config }: { mode: 'register' | 'reset_password'; config: Awaited<ReturnType<typeof api.config>> }) {
  const { t, i18n } = useTranslation()
  const register = mode === 'register', navigate = useNavigate(), client = useQueryClient()
  const action = useLocalAction(), mail = useEmailCode()
  const [username, setUsername] = useState(''), [email, setEmail] = useState(''), [code, setCode] = useState('')
  const [password, setPassword] = useState(''), [confirmation, setConfirmation] = useState('')
  const [terms, setTerms] = useState(false), [privacy, setPrivacy] = useState(false)
  const [token, setToken] = useState(''), [attempt, setAttempt] = useState(0), [captchaError, setCaptchaError] = useState(false), [invalid, setInvalid] = useState('')
  const onToken = useCallback((value: string) => { setToken(value); if (value) setCaptchaError(false) }, [])
  const onError = useCallback(() => { setToken(''); setCaptchaError(true) }, [])
  const enabled = config.turnstile.enabled
  const needsToken = enabled && !token
  const verification = enabled ? { turnstile_token: token } : {}
  const busy = action.busy || mail.busy
  const emailValid = /^[^\s@]+@[^\s@]+$/.test(email.trim()) && (!register || allowedEmail(email, config.email_domain_restriction))
  function resetCaptcha() { if (action.active.current) { setToken(''); setAttempt((value) => value + 1) } }
  async function send() {
    if (needsToken || !emailValid || (register && !validUsername(username))) return
    try { await mail.send({ email: email.trim(), purpose: mode, ...verification, ...(register ? { target_username: username.trim() } : {}) }) }
    finally { resetCaptcha() }
  }
  async function submit() {
    if (!emailValid || (register && (!validUsername(username) || !terms || !privacy)) || !code.trim() || !validPassword(password, confirmation)) { setInvalid(t('polish.checkYourUsernameEmailVerificationCodeAnd')); return }
    if (needsToken) return
    setInvalid('')
    await action.run(async () => {
      try {
        if (register) {
          const result = await api.register({ username: username.trim(), email: email.trim(), email_code: code.trim(), password, password_confirmation: confirmation, accept_terms: terms, accept_privacy: privacy, terms_revision: config.legal_revisions.terms, privacy_revision: config.legal_revisions.privacy, ...verification })
          if (action.active.current) { setPassword(''); setConfirmation(''); setCode(''); await navigate({ to: safePath(result.next_path) ?? '/login', search: { notice: result.pending_review ? 'pending_review' : 'registered' }, replace: true }) }
        } else {
          await api.resetPassword({ email: email.trim(), email_code: code.trim(), password, password_confirmation: confirmation, ...verification })
          api.forgetSession(); clearPortalSession(client, 'merchant')
          if (action.active.current) { setPassword(''); setConfirmation(''); setCode(''); await navigate({ to: '/login', search: { notice: 'password_reset' }, replace: true }) }
        }
      } catch (error) {
        if (error instanceof APIError && error.problem?.code === 'LEGAL_VERSION_CHANGED') {
          await client.invalidateQueries({ queryKey: publicConfigQuery.queryKey })
        }
        throw error
      } finally { resetCaptcha() }
    })
  }
  return <form className="space-y-5" aria-label={register ? t('polish.registerAsAMerchant') : t('polish.resetPassword')} onSubmit={(event) => { event.preventDefault(); void submit() }}>
    <h2 className="font-serif text-[1.75rem] leading-[1.2] font-normal tracking-[-0.01em]">{register ? t('polish.registerAsAMerchant') : t('polish.setANewPassword')}</h2>
    {register && config.registration_review && <Notice tone="info">{t('polish.registrationReviewNotice')}</Notice>}
    {register && <AccountField label={t('polish.username')} autoComplete="username" required disabled={busy} value={username} onChange={(event) => { setUsername(event.target.value); setCode('') }} pattern="[A-Za-z0-9_-]{3,32}" title={t('polish.332LettersNumbersUnderscoresOrHyphens')} />}
    <AccountField label={t('polish.email')} type="email" autoComplete="email" required disabled={busy} value={email} onChange={(event) => { setEmail(event.target.value); setCode('') }} />
    {register && config.email_domain_restriction.enabled && !!config.email_domain_restriction.domains?.length && <p className="text-caption text-text-2">{t('polish.emailDomainHint', { domains: config.email_domain_restriction.domains.join(i18n.resolvedLanguage === 'en' ? ', ' : '、') })}</p>}
    <AccountField label={t('polish.emailVerificationCode')} autoComplete="one-time-code" inputMode="numeric" required disabled={busy} value={code} onChange={(event) => setCode(event.target.value)} action={<EmailCodeButton mail={mail} disabled={busy || !emailValid || needsToken || (register && !validUsername(username))} onSend={() => { void send() }} />} />
    <AccountField label={t('polish.newPassword')} type="password" autoComplete="new-password" required disabled={busy} value={password} onChange={(event) => setPassword(event.target.value)} />
    <AccountField label={t('polish.confirmNewPassword')} type="password" autoComplete="new-password" required disabled={busy} value={confirmation} onChange={(event) => setConfirmation(event.target.value)} />
    <p className="text-caption text-text-3">{t('polish.use12128CharactersAndEnterThe')}</p>
    {register && <div className="space-y-2"><Checkbox aria-label={t('polish.iAgreeToTheTermsOfService')} required checked={terms} disabled={busy} onChange={(event) => setTerms(event.target.checked)}>{t('polish.iAgreeTo')}<a href="/legal/terms" target="_blank" rel="noopener noreferrer" className="text-accent-text underline">{t('polish.termsOfService')}</a></Checkbox><Checkbox aria-label={t('polish.iAgreeToThePrivacyPolicy')} required checked={privacy} disabled={busy} onChange={(event) => setPrivacy(event.target.checked)}>{t('polish.iAgreeTo')}<a href="/legal/privacy" target="_blank" rel="noopener noreferrer" className="text-accent-text underline">{t('polish.privacyPolicy')}</a></Checkbox></div>}
    {enabled && <Turnstile key={attempt} siteKey={config.turnstile.site_key} action={register ? config.turnstile.register_action : 'reset_password'} onToken={onToken} onError={onError} />}
    {enabled && captchaError && <><Notice tone="error">{t('polish.humanVerificationFailedPleaseTryAgain')}</Notice><Button onClick={resetCaptcha}>{t('polish.retryVerification')}</Button></>}
    {enabled && captchaError ? null : invalid ? <Notice tone="error">{invalid}</Notice> : action.error != null ? <AccountError error={action.error} /> : <EmailCodeNotice mails={[mail]} />}
    <Button type="submit" variant="primary" className="w-full" disabled={busy || needsToken}>{busy ? t('polish.submitting') : register ? t('polish.createAccount') : t('polish.confirmPasswordReset')}</Button>
  </form>
}
