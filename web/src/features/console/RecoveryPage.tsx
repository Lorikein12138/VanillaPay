import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { APIError, consoleApi, type ConsoleRecovery } from '@/api/client'
import { AuthScreen, FormHint, FormStack } from '@/components/ui/auth-screen'
import { Button } from '@/components/ui/button'
import { formatCountdown, useCountdown } from '@/components/ui/countdown'
import { Notice } from '@/components/ui/notice'
import { AccountError, AccountField } from '@/features/auth/account-shared'
import { useLocalAction } from '@/features/merchant/CollectionParts'
import { publicConfigQuery } from '@/lib/public-config'

export function RecoveryPage() {
  const { t } = useTranslation(), navigate = useNavigate(), action = useLocalAction()
  const queryClient = useQueryClient()
  const [session, setSession] = useState<Omit<ConsoleRecovery, 'csrf_token'> | null>(null)
  const csrf = useRef<string | null>(null), controller = useRef<AbortController | null>(null)
  const [code, setCode] = useState(''), [siteKey, setSiteKey] = useState(''), [hostname, setHostname] = useState(''), [secret, setSecret] = useState('')
  const [message, setMessage] = useState<'' | 'console.recoveryExpired' | 'console.recoveryRestart'>('')
  const remaining = useCountdown(session?.expires_at)
  useEffect(() => () => { csrf.current = null; controller.current?.abort() }, [])
  useEffect(() => {
    if (!session) return
    const timer = window.setTimeout(() => {
      csrf.current = null; controller.current?.abort(); setSession(null); setSecret(''); setCode(''); setMessage('console.recoveryExpired')
    }, Math.max(0, Date.parse(session.expires_at) - Date.now()))
    return () => window.clearTimeout(timer)
  }, [session])
  const valid = session ? !!siteKey.trim() && !!hostname.trim() && (session.security.turnstile_secret_configured || !!secret.trim()) && remaining > 0 : !!code.trim()
  async function submit() {
    if (!valid || (session && Date.parse(session.expires_at) <= Date.now())) return
    await action.run(async () => {
      const active = new AbortController(); controller.current = active; setMessage('')
      try {
        if (!session) {
          const result = await consoleApi.recover(code.trim(), active.signal)
          if (!action.active.current || active.signal.aborted) return
          const { csrf_token, ...display } = result
          csrf.current = csrf_token; setCode(''); setSession(display); setSiteKey(result.security.turnstile_site_key); setHostname(result.security.allowed_hostname); setSecret('')
        } else {
          if (!csrf.current) return
          await consoleApi.repair(csrf.current, { expected_version: session.settings_version, value: { turnstile_site_key: siteKey.trim(), allowed_hostname: hostname.trim(), ...(secret.trim() ? { turnstile_secret: secret.trim() } : {}) } }, active.signal)
          csrf.current = null
          if (!action.active.current || active.signal.aborted) return
          setSession(null)
          setSecret('')
          await queryClient.invalidateQueries({ queryKey: publicConfigQuery.queryKey })
          if (!action.active.current || active.signal.aborted) return
          await navigate({ to: '/console/login', search: { notice: 'repaired' }, replace: true })
        }
      } catch (error) {
        if (!action.active.current || active.signal.aborted) return
        if (error instanceof APIError && (error.status === 401 || error.problem?.code === 'VERSION_CONFLICT' || error.problem?.code === 'CSRF_FAILED')) {
          csrf.current = null; setSession(null); setSecret(''); setCode(''); setMessage('console.recoveryRestart')
        } else throw error
      } finally { if (controller.current === active) controller.current = null }
    })
  }
  return <AuthScreen title={t('console.recovery')} description={t(session ? 'console.repairDescription' : 'console.recoveryDescription')}><FormStack onSubmit={(event) => { event.preventDefault(); void submit() }}>
    {message && <Notice tone="warning">{t(message)}</Notice>}
    {session ? <>
      <FormHint>{t('console.repairRemaining', { time: formatCountdown(remaining) })}</FormHint>
      <AccountField label={t('console.siteKey')} value={siteKey} disabled={action.busy} onChange={(event) => setSiteKey(event.target.value)} />
      <AccountField label={t('console.hostname')} value={hostname} disabled={action.busy} onChange={(event) => setHostname(event.target.value)} />
      <AccountField label={t('console.secret')} type="password" autoComplete="new-password" required={!session.security.turnstile_secret_configured} hint={t(session.security.turnstile_secret_configured ? 'console.keepSecret' : 'console.needSecret')} value={secret} disabled={action.busy} onChange={(event) => setSecret(event.target.value)} />
    </> : <><Notice tone="info">{t('console.recoveryHelp')}</Notice><AccountField label={t('console.terminalCode')} type="password" autoComplete="off" value={code} disabled={action.busy} onChange={(event) => setCode(event.target.value)} /></>}
    {action.error != null && <AccountError error={action.error} />}
    <Button type="submit" variant="primary" loading={action.busy} disabled={!valid}>{t(session ? 'console.repair' : 'console.continueRecovery')}</Button>
  </FormStack></AuthScreen>
}
