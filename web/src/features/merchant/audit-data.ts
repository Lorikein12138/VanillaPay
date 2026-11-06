import type { operations } from '@/api/schema'

export type AuditFilters = Pick<NonNullable<operations['get_api_v1_merchant_audit_logs']['parameters']['query']>, 'start' | 'end' | 'action' | 'result'>
export function auditSearch(search: Record<string, unknown>): AuditFilters {
  const filters: AuditFilters = {}
  for (const key of ['start', 'end', 'action', 'result'] as const) {
    if (typeof search[key] === 'string' && search[key].trim()) filters[key] = search[key].trim()
  }
  return filters
}
// Names found in backend audit emitters. Unknown future names remain visible.
export const auditActions = [
  'subscription.purchase', 'subscription.cancel', 'subscription.fulfilled', 'subscription.refunded', 'subscription.manual_grant',
  'auth.login', 'auth.login_challenge', 'auth.identity_change', 'auth.password_reset', 'auth.totp_setup', 'auth.totp_confirm', 'auth.totp_disable',
  'account.preferences', 'merchant.register', 'merchant.reset_password', 'merchant.reset_key', 'merchant.set_rsa_public_key', 'merchant.update_ip_whitelist',
  'device.binding_issue', 'device.bind', 'device.replace', 'device.revoke', 'collection.save', 'collection.bounds_reset', 'media.upload',
  'order.create', 'order.paid', 'order.refunded', 'merchant.orders.supplement', 'orders.export', 'refund.request', 'refund.reject', 'callback.replay',
  'merchant.admin_created', 'merchant.ban', 'merchant.unban', 'plan.save', 'plan.delete', 'announcement.save', 'announcement.delete', 'settings.update_group',
  'admin.bootstrap', 'auth.admin_password_change', 'auth.console_recovery_issue', 'auth.console_recovery',
] as const
