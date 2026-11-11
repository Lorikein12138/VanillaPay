import { clearPortalSession } from '@/app/portal-session'
import { useId, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { consoleApi } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { DetailList } from '@/components/ui/detail-list'
import { FormStack } from '@/components/ui/auth-screen'
import { Card, Page, PageHeader, SectionHeader } from '@/components/ui/layout'
import { AccountError, AccountField, validPassword } from '@/features/auth/account-shared'
import { FactorDialog, PreferencesCard } from '@/features/merchant/AccountPage'
import { useMerchantAccount } from '@/features/merchant/AccountProvider'
import { useLocalAction } from '@/features/merchant/CollectionParts'

export function ConsoleAccountPage() {
  const { t } = useTranslation(), account = useMerchantAccount()!
  const [dialog, setDialog] = useState<'password' | 'setup' | 'disable' | null>(null)
  return <Page><PageHeader title={t('console.account')} description={t('console.accountDescription')} />
    <Card><SectionHeader title={t('console.admin')} actions={<Button onClick={() => setDialog('password')}>{t('polish.changePassword')}</Button>} /><DetailList columns={2} items={[{ label: t('polish.username'), value: account.user.username }, { label: t('console.role'), value: t('console.admin') }]} /></Card>
    <Card><SectionHeader title={t('polish.twoFactorAuthentication')} description={t(account.user.totp_enabled ? 'polish.enabledAnAuthenticatorCodeIsRequiredTo' : 'polish.notEnabledProtectSignInAndSensitive')} /><Button variant="primary" onClick={() => setDialog(account.user.totp_enabled ? 'disable' : 'setup')}>{t(account.user.totp_enabled ? 'polish.disableTwoFactorAuthentication' : 'polish.enableTwoFactorAuthentication')}</Button></Card>
    <PreferencesCard />
    {dialog === 'password' ? <PasswordDialog enabled={account.user.totp_enabled} onClose={() => setDialog(null)} /> : dialog && <FactorDialog mode={dialog} client={consoleApi} onClose={() => setDialog(null)} onChanged={account.refresh} />}
  </Page>
}

function PasswordDialog({ enabled, onClose }: { enabled: boolean; onClose: () => void }) {
  const { t } = useTranslation(), formId = useId(), action = useLocalAction(), client = useQueryClient(), navigate = useNavigate()
  const [current, setCurrent] = useState(''), [password, setPassword] = useState(''), [confirmation, setConfirmation] = useState(''), [code, setCode] = useState('')
  const valid = !!current && validPassword(password, confirmation) && (!enabled || /^\d{6}$/.test(code))
  async function submit() {
    if (!valid) return
    await action.run(async () => {
      await consoleApi.changePassword({ current_password: current, password, password_confirmation: confirmation, ...(enabled ? { totp_code: code } : {}) })
      consoleApi.forgetSession(); clearPortalSession(client, 'console')
      if (action.active.current) { setCurrent(''); setPassword(''); setConfirmation(''); setCode('') }
      await navigate({ to: '/console/login', search: { notice: 'password_changed' }, replace: true })
    })
  }
  return <Dialog title={t('polish.changePassword')} description={t('polish.savingImmediatelyRevokesAllSessionsYouWill')} busy={action.busy} onClose={onClose} footer={<Button type="submit" form={formId} variant="primary" loading={action.busy} disabled={!valid}>{t('polish.confirmSave')}</Button>}><FormStack id={formId} onSubmit={(event) => { event.preventDefault(); void submit() }}>
    <AccountField label={t('polish.currentPassword')} autoComplete="current-password" type="password" value={current} disabled={action.busy} onChange={(event) => setCurrent(event.target.value)} />
    <AccountField label={t('polish.newPassword')} autoComplete="new-password" type="password" value={password} disabled={action.busy} onChange={(event) => setPassword(event.target.value)} />
    <AccountField label={t('polish.confirmNewPassword')} autoComplete="new-password" type="password" value={confirmation} disabled={action.busy} onChange={(event) => setConfirmation(event.target.value)} />
    {enabled && <AccountField label={t('polish.authenticatorCode')} autoComplete="one-time-code" inputMode="numeric" value={code} disabled={action.busy} onChange={(event) => setCode(event.target.value)} />}
    {action.error != null && <AccountError error={action.error} />}
  </FormStack></Dialog>
}
