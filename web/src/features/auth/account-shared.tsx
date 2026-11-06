import { useEffect, useId, useState, type InputHTMLAttributes } from 'react'
import { useTranslation } from 'react-i18next'
import { APIError, api } from '@/api/client'
import type { components } from '@/api/schema'
import { FormError } from '@/components/FormError'
import { useLocalAction } from '@/features/merchant/CollectionParts'
import { orderButton, orderInput } from '@/features/merchant/OrderParts'
import { authErrorKey } from './errors'

export type AccountSchema = components['schemas']
export function AccountError({ error }: { error: unknown }) {
  const { t } = useTranslation()
  return <FormError>{t(authErrorKey(error))}</FormError>
}
export function AccountField({ label, ...props }: InputHTMLAttributes<HTMLInputElement> & { label: string }) {
  const id = useId()
  return <label htmlFor={id} className="flex min-w-0 flex-col gap-2 text-label text-text-2"><span>{label}</span><input id={id} className={orderInput} {...props} /></label>
}
export function validPassword(password: string, confirmation: string) { return Array.from(password).length >= 12 && Array.from(password).length <= 128 && password === confirmation }
export function validUsername(value: string) { return /^[A-Za-z0-9_-]{3,32}$/.test(value.trim()) }
export function allowedEmail(email: string, restriction: AccountSchema['PublicConfig']['email_domain_restriction']) {
  const domains = restriction.domains ?? []
  let domain = email.trim().toLowerCase().split('@')[1]
  try { domain = new URL(`http://${domain ?? ''}`).hostname.replace(/\.$/, '') } catch { return false }
  return !restriction.enabled || !domains.length || domains.some((item) => item.trim().toLowerCase().replace(/^@/, '') === domain)
}
export function useSecondsUntil(expiresAt: number) {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    if (!expiresAt) return
    const update = () => setNow(Date.now())
    update()
    const timer = setInterval(update, 1000)
    return () => clearInterval(timer)
  }, [expiresAt])
  return Math.max(0, Math.ceil((expiresAt - now) / 1000))
}
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
export function EmailCodeButton({ mail, disabled, onSend, label = '获取邮箱验证码' }: { mail: ReturnType<typeof useEmailCode>; disabled?: boolean; onSend: () => void; label?: string }) {
  return <div className="space-y-2"><button type="button" className={orderButton} disabled={disabled || mail.busy || mail.remaining > 0} onClick={onSend}>{mail.busy ? '发送中…' : mail.remaining > 0 ? `${mail.remaining} 秒后重发` : label}</button>{mail.sent && <p role="status" className="text-caption text-text-2">如邮箱可接收验证码，请查收邮件。</p>}{mail.error != null && <AccountError error={mail.error} />}</div>
}
