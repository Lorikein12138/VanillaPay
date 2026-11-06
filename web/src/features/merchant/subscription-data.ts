import type { QueryClient } from '@tanstack/react-query'
import { APIError } from '@/api/client'
import type { components } from '@/api/schema'
import { moduleError } from './collection-data'

export type Plan = components['schemas']['SubscriptionPlanDTO']
export type Subscription = components['schemas']['SubscriptionSubscriptionDTO']
export function purchaseKind(current: Subscription, plan: Plan) {
  if (!current.active || !current.plan) return 'new'
  return current.plan.id === plan.id ? 'renew' : 'switch'
}
export function localized(value: { zh_CN: string; en: string }, language: string) {
  return language.startsWith('zh') ? value.zh_CN : value.en
}
export function subscriptionError(error: unknown) {
  if (error instanceof APIError) {
    const code = error.problem?.code
    if (code && ['BILLING_CREDENTIAL_STALE', 'BILLING_NOT_READY', 'PLAN_UNAVAILABLE', 'VERSION_CONFLICT', 'PURCHASE_PENDING', 'PURCHASE_LIMIT_REACHED', 'IDEMPOTENCY_CONFLICT'].includes(code)) return `subscription.errors.${code}`
  }
  return moduleError(error)
}
export async function invalidateSubscription(client: QueryClient, name: string) {
  await Promise.all(['subscription', 'audit-logs', 'stats', 'orders', 'order', 'collection', 'codes'].map((part) => client.invalidateQueries({ queryKey: ['merchant', name, part] })))
}
