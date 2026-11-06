import { useQuery, type QueryClient } from '@tanstack/react-query'
import { api } from '@/api/client'
import type { components } from '@/api/schema'

export function notificationTarget(action: components['schemas']['PortalNotificationAction'] | null) {
  if (action?.page === '/dashboard/collection') return '/devices'
  if (action?.page === '/dashboard/orders' && action.entity_id) return `/orders/${encodeURIComponent(action.entity_id)}`
  return null
}
export function useNotificationSummary(name: string, enabled = true) {
  return useQuery({ queryKey: ['merchant', name, 'notifications', 'summary'], queryFn: () => api.notifications({ limit: 1 }), enabled, refetchInterval: 60000, refetchOnWindowFocus: 'always' })
}
export async function invalidateNotifications(client: QueryClient, name: string) {
  await client.invalidateQueries({ queryKey: ['merchant', name, 'notifications'] })
}
