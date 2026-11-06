import { useId, useState, type InputHTMLAttributes, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { APIError, api } from '@/api/client'
import type { components } from '@/api/schema'
import { Button } from '@/components/ui/button'
import { formatCountdown, useCountdown } from '@/components/ui/countdown'
import { Field } from '@/components/ui/field'
import { Input, InputGroup } from '@/components/ui/input'
import { Notice } from '@/components/ui/notice'
import { useLocalAction } from '@/features/merchant/CollectionParts'
import { authErrorKey } from './errors'

export type AccountSchema = components['schemas']
export function AccountError({ error }: { error: unknown }) {
  const { t } = useTranslation()
  return <Notice tone="error">{t(authErrorKey(error))}</Notice>
}
export function AccountField({ label, action, ...props }: InputHTMLAttributes<HTMLInputElement> & { label: string; action?: ReactNode }) {
  const id = useId()
  const input = <Input id={id} {...props} />
  return <Field htmlFor={id} label={label}>{action ? <InputGroup action={action}>{input}</InputGroup> : input}</Field>
}
export function validPassword(password: string, confirmation: string) { return Array.from(password).length >= 12 && Array.from(password).length <= 128 && password === confirmation }
export function validUsername(value: string) { return /^[A-Za-z0-9_-]{3,32}$/.test(value.trim()) }
export function allowedEmail(email: string, restriction: AccountSchema['PublicConfig']['email_domain_restriction']) {
  const domains = restriction.domains ?? []
  let domain = email.trim().toLowerCase().split('@')[1]
  try { domain = new URL(`http://${domain ?? ''}`).hostname.replace(/\.$/, '') } catch { return false }
  return !restriction.enabled || !domains.length || domains.some((item) => item.trim().toLowerCase().replace(/^@/, '') === domain)
}
export function useSecondsUntil(expiresAt: number) { return useCountdown(expiresAt || null) }
// Proofs and passwords stay in callers' component state, never in mutation caches.
export function useEmailCode() {
  const action = useLocalAction()
  const [retryAt, setRetryAt] = useState(0), [sent, setSent] = useState(false)
  const remaining = useSecondsUntil(retryAt)
  async function send(value: AccountSchema['AuthEmailCodeRequest']) {
    await action.run(async () => {
      try {
        const result = await api.emailCode(value)
        if (action.active.current) { setRetryAt(Date.now() + result.retry_after_seconds * 1000); setSent(result.sent) }
      } catch (error) {
        if (action.active.current && error instanceof APIError && error.status === 429) {
          const retry = error.problem?.details?.retry_after_seconds
          setRetryAt(Date.now() + (typeof retry === 'number' && retry > 0 ? retry : 60) * 1000)
        }
        throw error
      }
    })
  }
  return { ...action, send, remaining, sent }
}
export function EmailCodeButton({ mail, disabled, onSend, label }: { mail: ReturnType<typeof useEmailCode>; disabled?: boolean; onSend: () => void; label?: string }) {
  const { t } = useTranslation()
  return <Button loading={mail.busy} disabled={disabled || mail.busy || mail.remaining > 0} onClick={onSend}>{mail.busy ? t('polish.sending') : mail.remaining > 0 ? formatCountdown(mail.remaining) : label ?? t('polish.getEmailCode')}</Button>
}
export function EmailCodeNotice({ mails }: { mails: ReturnType<typeof useEmailCode>[] }) {
  const { t } = useTranslation()
  const error = mails.find((mail) => mail.error != null)?.error
  return error != null ? <AccountError error={error} /> : mails.some((mail) => mail.sent) ? <Notice tone="info">{t('polish.ifThisMailboxCanReceiveACode')}</Notice> : null
}
