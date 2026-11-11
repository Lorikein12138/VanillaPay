import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { APIError, consoleApi } from '@/api/client'
import { setPortalSession } from '@/app/portal-session'
import { AuthScreen, FormHint, FormStack } from '@/components/ui/auth-screen'
import { Button } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'
import { AccountError, AccountField, validPassword, validUsername } from '@/features/auth/account-shared'
import { useLocalAction } from '@/features/merchant/CollectionParts'
import { consoleDestination } from '@/lib/redirect'

export function SetupPage() {
  const client = useQueryClient()
  const { t } = useTranslation(), navigate = useNavigate(), action = useLocalAction()
  const [code, setCode] = useState(''), [username, setUsername] = useState(''), [password, setPassword] = useState(''), [confirmation, setConfirmation] = useState('')
  const valid = !!code.trim() && validUsername(username) && validPassword(password, confirmation)
  async function submit() {
    if (!valid) return
    await action.run(async () => {
      try {
        const result = await consoleApi.bootstrap({ initialization_code: code.trim(), username: username.trim(), password, password_confirmation: confirmation })
        setPortalSession(client, 'console', result)
        if (action.active.current) { setCode(''); setPassword(''); setConfirmation(''); await navigate({ to: consoleDestination(undefined, result.next_path), replace: true }) }
      } catch (error) {
        if (action.active.current && error instanceof APIError && error.problem?.code === 'ALREADY_INITIALIZED') await navigate({ to: '/console/login', search: { notice: 'initialized' }, replace: true })
        else throw error
      }
    })
  }
  return <AuthScreen title={t('console.setup')} description={t('console.setupDescription')}><FormStack onSubmit={(event) => { event.preventDefault(); void submit() }}>
    <Notice tone="info">{t('console.initializationHelp')}</Notice>
    <AccountField label={t('console.initializationCode')} type="password" autoComplete="off" value={code} disabled={action.busy} onChange={(event) => setCode(event.target.value)} />
    <AccountField label={t('polish.username')} autoComplete="username" value={username} disabled={action.busy} onChange={(event) => setUsername(event.target.value)} />
    <FormHint>{t('console.validationHelp')}</FormHint>
    <AccountField label={t('auth.password')} type="password" autoComplete="new-password" value={password} disabled={action.busy} onChange={(event) => setPassword(event.target.value)} />
    <AccountField label={t('polish.confirmNewPassword')} type="password" autoComplete="new-password" value={confirmation} disabled={action.busy} onChange={(event) => setConfirmation(event.target.value)} />
    {action.error != null && <AccountError error={action.error} />}
    <Button type="submit" variant="primary" disabled={!valid} loading={action.busy}>{t('console.initialize')}</Button>
  </FormStack></AuthScreen>
}
