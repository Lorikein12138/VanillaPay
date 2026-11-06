import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import QRCode from 'qrcode'
import { useEffect, useId, useState } from 'react'
import { api, APIError } from '@/api/client'
import { StatusDot } from '@/components/StatusDot'
import { Button, buttonVariants } from '@/components/ui/button'
import { CopyField } from '@/components/ui/copy-button'
import { formatCountdown } from '@/components/ui/countdown'
import { DetailList } from '@/components/ui/detail-list'
import { Dialog } from '@/components/ui/dialog'
import { Field, FieldGrid } from '@/components/ui/field'
import { Select } from '@/components/ui/input'
import { Card, Page, PageHeader, SectionHeader } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { AccountError, AccountField, EmailCodeButton, EmailCodeNotice, useEmailCode, useSecondsUntil, validPassword, validUsername, type AccountSchema } from '@/features/auth/account-shared'
import { useMerchantAccount } from './AccountProvider'
import { useLocalAction } from './CollectionParts'
import { OrderTime } from './OrderParts'

export function AccountPage() {
  const { t } = useTranslation()
  const account = useMerchantAccount(), id = useId()
  const [dialog, setDialog] = useState<'username' | 'email' | 'password' | 'setup' | 'disable' | null>(null)
  if (!account) return null
  const { user } = account
  return <Page><PageHeader title={t('polish.accountSecurity')} description={t('polish.manageYourMerchantProfileSignInSecurity')} />
    <Card><SectionHeader title={t('audit.title')} description={t('audit.description')} actions={<Link to="/account/logs" className={buttonVariants()}>{t('audit.open')}</Link>} /></Card>
    <Card><SectionHeader title={t('polish.merchantProfile')} actions={<>{(['username', 'email', 'password'] as const).map((mode) => <Button key={mode} onClick={() => setDialog(mode)}>{mode === 'username' ? t('polish.changeUsername') : mode === 'email' ? t('polish.changeEmail') : t('polish.changePassword')}</Button>)}</>} /><DetailList columns={2} items={[{ label: t('polish.username'), value: user.username }, { label: t('polish.email'), value: user.email ?? '—' }, { label: 'PID', value: user.pid, mono: true }, { label: t('polish.createdAt'), value: <OrderTime value={user.created_at} /> }]} /></Card>
    <Card><SectionHeader title={t('polish.twoFactorAuthentication')} description={user.totp_enabled ? t('polish.enabledAnAuthenticatorCodeIsRequiredTo') : t('polish.notEnabledProtectSignInAndSensitive')} actions={<StatusDot tone={user.totp_enabled ? 'ok' : 'off'}>{user.totp_enabled ? t('polish.enabled') : t('polish.notEnabled')}</StatusDot>} /><Button variant="primary" onClick={() => setDialog(user.totp_enabled ? 'disable' : 'setup')}>{user.totp_enabled ? t('polish.disableTwoFactorAuthentication') : t('polish.enableTwoFactorAuthentication')}</Button></Card>
    <Card><SectionHeader title={t('polish.preferences')} description={t('polish.yourSavedPreferencesApplyAfterSignIn')} /><FieldGrid><Field label={t('polish.language')} htmlFor={id+'-language'}><Select id={id+'-language'} aria-label={t('polish.accountLanguage')} disabled={account.busy} value={account.preferences.locale} onChange={(event) => { void account.save({ ...account.preferences, locale: event.target.value === 'en' ? 'en' : 'zh-CN' }) }}><option value="zh-CN">{t('polish.chinese')}</option><option value="en">English</option></Select></Field><Field label={t('polish.theme')} htmlFor={id+'-theme'}><Select id={id+'-theme'} aria-label={t('polish.accountTheme')} disabled={account.busy} value={account.preferences.theme} onChange={(event) => { const theme = event.target.value; if (theme === 'system' || theme === 'light' || theme === 'dark') void account.save({ ...account.preferences, theme }) }}><option value="system">{t('polish.system')}</option><option value="light">{t('polish.light')}</option><option value="dark">{t('polish.dark')}</option></Select></Field></FieldGrid></Card>
    {dialog && (dialog === 'setup' || dialog === 'disable' ? <FactorDialog mode={dialog} onClose={() => setDialog(null)} onChanged={account.refresh} /> : <IdentityDialog mode={dialog} user={user} onClose={() => setDialog(null)} onChanged={account.refresh} />)}
  </Page>
}

export function IdentityDialog({ mode, user, onClose, onChanged }: { mode: 'username' | 'email' | 'password'; user: AccountSchema['AuthUserDTO']; onClose: () => void; onChanged: () => Promise<void> }) {
  const { t } = useTranslation()
  const formId = useId()
  const action = useLocalAction(), mail = useEmailCode(), newMail = useEmailCode(), client = useQueryClient(), navigate = useNavigate()
  const [value, setValue] = useState(''), [confirmation, setConfirmation] = useState(''), [code, setCode] = useState(''), [newCode, setNewCode] = useState(''), [totp, setTOTP] = useState(''), [invalid, setInvalid] = useState('')
  const title = mode === 'username' ? t('polish.changeUsername') : mode === 'email' ? t('polish.changeEmail') : t('polish.changePassword')
  const busy = action.busy || mail.busy || newMail.busy
  const valid = mode === 'username' ? validUsername(value) : mode === 'email' ? /^[^\s@]+@[^\s@]+$/.test(value.trim()) && value.trim().toLowerCase() !== user.email?.toLowerCase() : validPassword(value, confirmation)
  async function sendOld() {
    if (!user.email || (mode === 'username' && !valid)) return
    await mail.send({ email: user.email, purpose: mode === 'username' ? 'change_username' : mode === 'email' ? 'change_email_old' : 'change_password', ...(mode === 'username' ? { target_username: value.trim() } : { target_email: user.email }) })
  }
  async function submit() {
    if (!valid || !code.trim() || (mode === 'email' && !newCode.trim()) || (user.totp_enabled && !/^\d{6}$/.test(totp))) { setInvalid(t('polish.checkYourDetailsEmailCodeAndThe')); return }
    setInvalid('')
    await action.run(async () => {
      const proof: AccountSchema['AuthSecurityProof'] = { email_code: code.trim(), ...(user.totp_enabled ? { totp_code: totp } : {}) }
      if (mode === 'password') {
        await api.changePassword({ password: value, password_confirmation: confirmation, security_proof: proof })
        api.forgetSession(); client.clear()
        if (action.active.current) { setValue(''); setConfirmation(''); setCode(''); setTOTP(''); await navigate({ to: '/login', search: { notice: 'password_changed' }, replace: true }) }
      } else {
        if (mode === 'username') await api.changeUsername({ username: value.trim(), security_proof: proof })
        else await api.changeEmail({ email: value.trim(), old_email_code: code.trim(), new_email_code: newCode.trim(), ...(user.totp_enabled ? { totp_code: totp } : {}) })
        if (action.active.current) { setCode(''); setNewCode(''); setTOTP(''); await onChanged(); if (action.active.current) onClose() }
      }
    })
  }
  return <Dialog title={title} description={mode === 'password' ? t('polish.savingImmediatelyRevokesAllSessionsYouWill') : t('polish.verifyWithYourCurrentEmailAnAuthenticator')} busy={busy} onClose={onClose} footer={<Button type="submit" form={formId} variant="primary" loading={busy} disabled={!valid || !code.trim() || (mode === 'email' && !newCode.trim()) || (user.totp_enabled && !/^\d{6}$/.test(totp))}>{busy ? t('polish.saving') : t('polish.confirmSave')}</Button>}><form id={formId} className="flex flex-col gap-5" onSubmit={(event) => { event.preventDefault(); void submit() }}>
    <AccountField label={mode === 'username' ? t('polish.newUsername') : mode === 'email' ? t('polish.newEmail') : t('polish.newPassword')} type={mode === 'password' ? 'password' : mode === 'email' ? 'email' : 'text'} autoComplete={mode === 'password' ? 'new-password' : mode === 'email' ? 'email' : 'username'} required disabled={busy} value={value} onChange={(event) => { setValue(event.target.value); if (mode === 'username') setCode(''); if (mode === 'email') setNewCode('') }} />
    {mode === 'password' && <AccountField label={t('polish.confirmNewPassword')} type="password" autoComplete="new-password" required disabled={busy} value={confirmation} onChange={(event) => setConfirmation(event.target.value)} />}
    <p className="break-all text-caption text-text-2">{t('polish.currentEmail')}{user.email ?? t('polish.unavailablePleaseSignInAgain')}</p>
    <AccountField label={mode === 'email' ? t('polish.oldEmailVerificationCode') : t('polish.emailVerificationCode')} autoComplete="one-time-code" inputMode="numeric" required disabled={busy} value={code} onChange={(event) => setCode(event.target.value)} action={<EmailCodeButton mail={mail} label={mode === 'email' ? t('polish.getOldEmailCode') : undefined} disabled={busy || !user.email || (mode === 'username' && !valid)} onSend={() => { void sendOld() }} />} />
    {mode === 'email' && <AccountField label={t('polish.newEmailVerificationCode')} autoComplete="one-time-code" inputMode="numeric" required disabled={busy} value={newCode} onChange={(event) => setNewCode(event.target.value)} action={<EmailCodeButton mail={newMail} label={t('polish.getNewEmailCode')} disabled={busy || !valid} onSend={() => { void newMail.send({ email: value.trim(), target_email: value.trim(), purpose: 'change_email_new' }) }} />} />}
    {user.totp_enabled && <AccountField label={t('polish.authenticatorCode')} autoComplete="one-time-code" inputMode="numeric" maxLength={6} required disabled={busy} value={totp} onChange={(event) => setTOTP(event.target.value)} />}
    {invalid ? <Notice tone="error">{invalid}</Notice> : action.error != null ? <AccountError error={action.error} /> : <EmailCodeNotice mails={[mail, newMail]} />}
  </form></Dialog>
}

export function FactorDialog({ mode, onClose, onChanged }: { mode: 'setup' | 'disable'; onClose: () => void; onChanged: () => Promise<void> }) {
  const { t } = useTranslation()
  const action = useLocalAction(), formId = useId()
  const [password, setPassword] = useState(''), [code, setCode] = useState('')
  const [setup, setSetup] = useState<AccountSchema['AuthTOTPSetup'] | null>(null), [recovery, setRecovery] = useState<string[] | null>(null), [expired, setExpired] = useState(false)
  const remaining = useSecondsUntil(setup ? Date.parse(setup.expires_at) : 0)
  useEffect(() => {
    if (!setup) return
    const timer = setTimeout(() => { setSetup(null); setCode(''); setExpired(true) }, Math.max(0, Date.parse(setup.expires_at) - Date.now()))
    return () => clearTimeout(timer)
  }, [setup])
  function close() { setSetup(null); setRecovery(null); setPassword(''); setCode(''); onClose() }
  async function submit() {
    if ((mode === 'disable' || !setup) && !password) return
    if ((mode === 'disable' || setup) && !/^\d{6}$/.test(code)) return
    await action.run(async () => {
      if (mode === 'disable') {
        await api.disableTOTP({ password, totp_code: code })
        if (action.active.current) { setPassword(''); setCode(''); await onChanged(); if (action.active.current) close() }
      } else if (!setup) {
        const result = await api.setupTOTP({ password })
        if (action.active.current) { setPassword(''); const valid = Date.parse(result.expires_at) > Date.now(); setSetup(valid ? result : null); setExpired(!valid) }
      } else {
        try {
          const result = await api.confirmTOTP({ setup_id: setup.setup_id, totp_code: code })
          if (action.active.current) { setSetup(null); setCode(''); setRecovery(result.recovery_codes ?? []); await onChanged() }
        } catch (error) {
          if (action.active.current && error instanceof APIError && error.problem?.code === 'CHALLENGE_EXPIRED') { setSetup(null); setCode(''); setExpired(true) }
          throw error
        }
      }
    })
  }
  function download() {
    if (!recovery) return
    const url = URL.createObjectURL(new Blob([recovery.join('\n') + '\n'], { type: 'text/plain;charset=utf-8' }))
    const link = document.createElement('a'); link.href = url; link.download = 'vanillapay-recovery-codes.txt'; link.click()
    setTimeout(() => URL.revokeObjectURL(url), 0)
  }
  return <Dialog title={recovery ? t('polish.saveRecoveryCodes') : mode === 'setup' ? t('polish.enableTwoFactorAuthentication') : t('polish.disableTwoFactorAuthentication')} description={recovery ? t('polish.theseRecoveryCodesAreShownOnceAnd') : mode === 'setup' ? t('polish.verifyYourCurrentPasswordScanTheQr') : t('polish.enterYourCurrentPasswordAndAFresh')} busy={action.busy} onClose={close} footer={recovery ? <Button onClick={download}>{t('polish.downloadRecoveryCodesTxt')}</Button> : <Button form={formId} type="submit" variant="primary" loading={action.busy} disabled={(!setup && !password) || ((setup || mode === 'disable') && !/^\d{6}$/.test(code))}>{action.busy ? t('polish.processing') : mode === 'disable' ? t('polish.confirmDisableTwoFactorAuthentication') : setup ? t('polish.confirmEnable') : t('polish.verifyPasswordAndGenerate')}</Button>}>
    {recovery ? <CopyField value={recovery.join('\n')} label={t('polish.copyRecoveryCodes')}><div data-testid="recovery-codes" className="grid grid-cols-2 gap-x-4 gap-y-2">{recovery.map((value) => <span key={value}>{value}{'\n'}</span>)}</div></CopyField> : <form id={formId} className="flex flex-col gap-5" onSubmit={(event) => { event.preventDefault(); void submit() }}>
      {expired && <Notice tone="info">{t('polish.setupExpiredEnterYourPasswordAgainTo')}</Notice>}
      {setup ? <><TOTPSecret setup={setup} /><p className="text-center text-caption text-text-3">{t('polish.setupRemaining', { time: formatCountdown(remaining) })}</p></> : <AccountField label={t('polish.currentPassword')} type="password" autoComplete="current-password" required value={password} disabled={action.busy} onChange={(event) => setPassword(event.target.value)} />}
      {(setup || mode === 'disable') && <AccountField label={t('polish.authenticatorCode')} autoComplete="one-time-code" inputMode="numeric" maxLength={6} required value={code} disabled={action.busy} onChange={(event) => setCode(event.target.value)} />}
      {action.error != null && <AccountError error={action.error} />}
    </form>}
  </Dialog>
}

function TOTPSecret({ setup }: { setup: AccountSchema['AuthTOTPSetup'] }) {
  const { t } = useTranslation()
  const [qr, setQR] = useState(''), [failed, setFailed] = useState(false)
  useEffect(() => {
    let active = true
    void QRCode.toDataURL(setup.otpauth_url, { width: 240, margin: 3 }).then((result) => { if (active) setQR(result) }, () => { if (active) setFailed(true) })
    return () => { active = false }
  }, [setup])
  return <div className="space-y-4 text-center">{qr && <img data-testid="totp-qr" className="mx-auto w-60 max-w-full rounded-control" src={qr} alt={t('polish.authenticatorSetupQrCode')} />}{failed && <Notice tone="error">{t('polish.couldNotGenerateTheQrCodeEnter')}</Notice>}<p className="text-caption text-text-2">{t('polish.manualSetupSecret')}</p><CopyField testId="totp-secret" value={setup.secret} label={t('polish.copyAuthenticatorSecret')} /></div>
}
