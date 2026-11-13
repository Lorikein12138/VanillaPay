import { readRFC3339 } from '@/lib/date-time'
import type { components, operations } from '@/api/schema'
import { APIError } from '@/api/client'

export type OrderFilters = Pick<NonNullable<operations['get_api_v1_merchant_orders']['parameters']['query']>, 'status' | 'channel' | 'start' | 'end' | 'query'>
export type OrderAction = 'supplement' | 'refund' | 'reject' | 'resolve' | 'replay'
export const orderStatuses = ['pending', 'paid', 'expired', 'refunded'] as const
export const orderChannels = ['alipay', 'wxpay'] as const

export function orderSearch(search: Record<string, unknown>): OrderFilters {
  const result: OrderFilters = {}
  for (const key of ['status', 'channel', 'start', 'end', 'query'] as const) {
    const value = search[key]
    const parsed = typeof value === 'string' ? value.trim() : ''
    const valid = key === 'start' || key === 'end' ? readRFC3339(parsed) : parsed
    if (valid) result[key] = valid
  }
  return result
}

export function availableOrderActions(detail: components['schemas']['OrderDetail']): OrderAction[] {
  const { order, refund, notify_url } = detail
  const actions: OrderAction[] = []
  if (order.status === 'pending' || (order.status === 'expired' && (order.expire_reason !== 'cancelled' || order.purpose !== 'external'))) actions.push('supplement')
  if (order.status === 'paid') {
    actions.push('refund')
    if (refund?.status === 'pending') actions.push('resolve', 'reject')
    if (order.purpose !== 'subscription' && notify_url && ['success', 'failed', 'cancelled'].includes(order.callback_status)) actions.push('replay')
  }
  return actions
}

export function orderErrorKey(error: unknown) {
  if (!(error instanceof APIError) || error.status === 0) return 'merchant.orders.errors.network'
  switch (error.problem?.code) {
    case 'CSRF_FAILED': return 'merchant.orders.errors.csrf'
    case 'CHANNEL_NOT_ALLOWED': return 'merchant.orders.errors.channel'
    case 'CHANNEL_LOCKED': return 'merchant.orders.errors.channelLocked'
    case 'CALLBACK_PENDING': return 'merchant.orders.errors.callbackPending'
    case 'CALLBACK_UNAVAILABLE': return 'merchant.orders.errors.callbackUnavailable'
    case 'IDEMPOTENCY_CONFLICT': return 'merchant.orders.errors.idempotency'
    case 'REFUND_ALREADY_HANDLED': return 'merchant.orders.errors.refundHandled'
    case 'ACCOUNT_BANNED': return 'auth.errors.banned'
  }
  if (error.status === 401) return 'auth.errors.session'
  if (error.status === 403) return 'auth.errors.forbidden'
  if (error.status === 404) return 'merchant.orders.notFound'
  if (error.status === 409) return 'merchant.orders.errors.conflict'
  if (error.status === 400) return 'merchant.orders.errors.validation'
  if (error.status === 429) return 'auth.errors.rateLimit'
  return 'merchant.orders.errors.unavailable'
}

export function validReason(reason: string, action: OrderAction) {
  return reason.trim().length > 0 && (action === 'supplement' ? Array.from(reason).length : new TextEncoder().encode(reason).length) <= 500
}
