import { useInfiniteQuery, type QueryClient } from '@tanstack/react-query'
import type { components, operations } from '@/api/schema'
import { yuanToCents } from '@/features/merchant/collection-data'

export type Schema = components['schemas']
export type MerchantFilters = Pick<NonNullable<operations['get_api_v1_console_merchants']['parameters']['query']>, 'query' | 'banned' | 'subscription_active'>
export type PurchaseFilters = Pick<NonNullable<operations['get_api_v1_console_purchases']['parameters']['query']>, 'start' | 'end' | 'state' | 'merchant_id' | 'plan_id'>
export const channels = ['alipay', 'wxpay', 'qqpay'] as const
export const purchaseStates = ['pending', 'paid', 'expired', 'cancelled', 'refunded'] as const
export const bytes = (value: string) => new TextEncoder().encode(value).length
export const validBanReason = (value: string) => Array.from(value.trim()).length >= 1 && Array.from(value.trim()).length <= 500
export const validGrantReason = (value: string) => !!value.trim() && bytes(value.trim()) <= 500
export function planName(plan: Schema['SubscriptionPlanDTO'] | null, language: string) {
  const name = plan?.name
  if (!name) return ''
  return language.startsWith('zh') ? name.zh_CN : name.en || name.zh_CN
}
export function merchantSearch(search: Record<string, unknown>): MerchantFilters {
  return {
    query: typeof search.query === 'string' ? Array.from(search.query.trim()).slice(0, 100).join('') || undefined : undefined,
    banned: search.banned === true || search.banned === 'true' ? true : search.banned === false || search.banned === 'false' ? false : undefined,
    subscription_active: search.subscription_active === true || search.subscription_active === 'true' ? true : search.subscription_active === false || search.subscription_active === 'false' ? false : undefined,
  }
}
export function purchaseSearch(search: Record<string, unknown>): PurchaseFilters {
  const id = (value: unknown) => /^\d+$/.test(String(value)) && Number.isSafeInteger(Number(value)) && Number(value) > 0 ? Number(value) : undefined
  return {
    start: typeof search.start === 'string' ? search.start : undefined,
    end: typeof search.end === 'string' ? search.end : undefined,
    state: purchaseStates.find((state) => state === search.state),
    merchant_id: id(search.merchant_id), plan_id: id(search.plan_id),
  }
}
export function planInput(zh: string, en: string, price: string, days: string, selected: string[], limit: string): Schema['SubscriptionPlanCommand'] | null {
  const cents = yuanToCents(price)
  if (!zh.trim() || bytes(zh.trim()) > 300 || bytes(en.trim()) > 300 || cents === null || cents < 1 || cents > 999999999 ||
    !/^\d+$/.test(days) || Number(days) < 1 || Number(days) > 36500 ||
    !selected.length || selected.some((c) => !channels.some((known) => known === c)) ||
    (limit !== '' && (!/^\d+$/.test(limit) || !Number.isSafeInteger(Number(limit)) || Number(limit) < 1))) return null
  return { name: { zh_CN: zh.trim(), en: en.trim() }, price_cents: cents, duration_days: Number(days), channels: selected, purchase_limit: limit ? Number(limit) : null }
}
export async function invalidateConsole(client: QueryClient) {
  await client.invalidateQueries({ queryKey: ['console'] })
}
export function useCursorList<T>(key: readonly unknown[], load: (cursor: string | undefined, signal: AbortSignal) => Promise<{ items: T[] | null; total: number; next_cursor: string | null }>) {
  return useInfiniteQuery({
    queryKey: key, gcTime: 0, initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) => load(pageParam, signal),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
  })
}
