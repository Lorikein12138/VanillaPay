import { infiniteQueryOptions, keepPreviousData, queryOptions } from '@tanstack/react-query'
import { api, consoleApi } from '@/api/client'
import type { AuditFilters } from '@/features/merchant/audit-data'
import type { ReceiptFilters } from '@/features/merchant/collection-data'
import type { OrderFilters } from '@/features/merchant/orders-data'
import { previousRange, statsRange, type StatsRange } from '@/features/merchant/overview-data'
import type { MerchantFilters, PurchaseFilters } from '@/features/console/manage-data'

type PortalRole = 'merchant' | 'console'
const freshness = { staleTime: 15_000 }

function cursorQuery<T extends { next_cursor: string | null }>(
  queryKey: readonly unknown[],
  load: (cursor: string | undefined, signal: AbortSignal) => Promise<T>,
) {
  return infiniteQueryOptions({
    ...freshness,
    gcTime: 30_000,
    queryKey,
    queryFn: ({ pageParam, signal }) => load(pageParam, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    placeholderData: keepPreviousData,
  })
}

export const ordersQuery = (role: PortalRole, name: string, filters: OrderFilters) =>
  cursorQuery([role, name, 'orders', 'list', filters], (cursor) => (role === 'console' ? consoleApi : api).orderPage({ ...filters, limit: 20, cursor }))

export const receiptsQuery = (name: string, filters: ReceiptFilters) =>
  cursorQuery(['merchant', name, 'receipts', 'list', filters], (cursor) => api.receipts({ ...filters, limit: 20, cursor }))

export const notificationsQuery = (name: string, unread = false) =>
  cursorQuery(['merchant', name, 'notifications', 'list', { unread }], (cursor) => api.notifications({ limit: 20, cursor, unread_only: unread }))

export const auditQuery = (role: PortalRole, name: string, filters: AuditFilters) =>
  cursorQuery([role, name, 'audit-logs', filters], (cursor, signal) => {
    const params = { ...filters, limit: 20, cursor }
    return role === 'console' ? consoleApi.auditLogs(params, signal) : api.auditLogs(params)
  })

export const merchantsQuery = (filters: MerchantFilters) =>
  cursorQuery(['console', 'merchants', filters], (cursor, signal) => consoleApi.merchants({ ...filters, cursor, limit: 20 }, signal))

export const purchasesQuery = (filters: PurchaseFilters) =>
  cursorQuery(['console', 'purchases', filters], (cursor, signal) => consoleApi.purchases({ ...filters, cursor, limit: 20 }, signal))

export const announcementsQuery = () =>
  cursorQuery(['console', 'announcements'], (cursor, signal) => consoleApi.announcements({ limit: 20, cursor }, signal))

export const plansQuery = (includeDeleted: boolean) => queryOptions({
  ...freshness, queryKey: ['console', 'plans', includeDeleted], queryFn: ({ signal }) => consoleApi.plans(includeDeleted, signal),
})

export const settingsQuery = () => queryOptions({
  ...freshness, queryKey: ['console', 'settings'] as const, queryFn: ({ signal }) => consoleApi.settings(signal),
})

export const codesQuery = (name: string) => queryOptions({
  ...freshness, queryKey: ['merchant', name, 'codes'], queryFn: api.qrSlots,
})

export const collectionQuery = (name: string) => queryOptions({
  ...freshness, queryKey: ['merchant', name, 'collection'], queryFn: api.collection,
})

export const deviceQuery = (name: string) => queryOptions({
  ...freshness, queryKey: ['merchant', name, 'device'], queryFn: api.device,
})

export const integrationQuery = (name: string) => queryOptions({
  ...freshness, queryKey: ['merchant', name, 'integration'], gcTime: 30_000,
  queryFn: async () => {
    const data = await api.integration()
    // Discard the secret before entering the cache.
    return { ...data, key: '' }
  },
})

export const subscriptionQuery = (name: string) => queryOptions({
  ...freshness, queryKey: ['merchant', name, 'subscription'], queryFn: api.subscription,
})

export const statsQuery = (role: PortalRole, name: string, range: StatsRange) => queryOptions({
  staleTime: 60_000, queryKey: [role, name, 'stats', range], queryFn: () => (role === 'console' ? consoleApi : api).stats(range),
})

export const recentOrdersQuery = (name: string) => queryOptions({
  ...freshness, queryKey: ['merchant', name, 'orders', 6], queryFn: () => api.orders(6),
})

export function overviewQueries(role: PortalRole, name: string) {
  const now = new Date(), timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const today = statsRange(now, 1, timezone)
  return [today, previousRange(today), statsRange(now, 7, timezone)].map((range) => statsQuery(role, name, range))
}
