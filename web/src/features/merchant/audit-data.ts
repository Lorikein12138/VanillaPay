import { readRFC3339 } from '@/lib/date-time'
import type { operations } from '@/api/schema'
import type { ConsoleAuditQuery } from '@/api/client'

type MerchantAuditFilters = Pick<
  NonNullable<operations['get_api_v1_merchant_audit_logs']['parameters']['query']>, 'start' | 'end' | 'action' | 'result'
>
export type AuditFilters = MerchantAuditFilters & Pick<ConsoleAuditQuery, 'merchant_id' | 'actor_role'>
export const auditRoles = ['merchant', 'admin', 'system', 'device'] as const satisfies NonNullable<AuditFilters['actor_role']>[]

export function auditSearch(search: Record<string, unknown>): MerchantAuditFilters {
  const filters: MerchantAuditFilters = {}
  for (const key of ['start', 'end'] as const) {
    const value = search[key]
    const date = readRFC3339(value)
    if (date) filters[key] = date
  }
  for (const key of ['action', 'result'] as const) {
    if (typeof search[key] === 'string' && search[key].trim()) filters[key] = search[key].trim()
  }
  return filters
}

export function auditMerchantId(value: unknown) {
  if (typeof value === 'number') return Number.isSafeInteger(value) && value > 0 ? String(value) : undefined
  if (typeof value !== 'string' || !/^\+?\d+$/.test(value)) return undefined
  const digits = value.replace(/^\+?0*/, '')
  if (!digits || digits.length > 19 || (digits.length === 19 && digits > '9223372036854775807')) return undefined
  return digits
}

export function consoleAuditSearch(search: Record<string, unknown>): AuditFilters {
  return {
    ...auditSearch(search),
    merchant_id: auditMerchantId(search.merchant_id),
    actor_role: auditRoles.find((role) => role === search.actor_role),
  }
}
// Names found in backend audit emitters. Unknown future names remain visible.
export const auditActions = [
  'subscription.purchase', 'subscription.cancel', 'subscription.fulfilled', 'subscription.refunded', 'subscription.manual_grant',
  'auth.login', 'auth.login_challenge', 'auth.identity_change', 'auth.password_reset', 'auth.totp_setup', 'auth.totp_confirm', 'auth.totp_disable',
  'account.preferences', 'merchant.register', 'merchant.reset_password', 'merchant.reset_key', 'merchant.set_rsa_public_key', 'merchant.update_ip_whitelist', 'merchant.update_notify_domains', 'merchant.update_callback_settings',
  'device.binding_issue', 'device.bind', 'device.replace', 'device.revoke', 'collection.save', 'collection.bounds_reset', 'media.upload',
  'order.create', 'order.paid', 'order.refunded', 'order.close', 'merchant.orders.supplement', 'orders.export', 'refund.request', 'refund.reject', 'callback.replay',
  'merchant.admin_created', 'merchant.approve', 'merchant.reject', 'merchant.ban', 'merchant.unban', 'plan.save', 'plan.delete', 'announcement.save', 'announcement.delete', 'settings.update_group',
  'admin.bootstrap', 'auth.admin_password_change', 'auth.console_recovery_issue', 'auth.console_recovery',
] as const
