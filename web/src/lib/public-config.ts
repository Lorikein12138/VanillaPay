import { queryOptions, useQuery, type QueryClient } from '@tanstack/react-query'
import { api } from '@/api/client'

export const publicConfigQuery = queryOptions({
  queryKey: ['public-config'] as const,
  queryFn: ({ signal }) => api.config(signal),
  staleTime: 60_000,
  refetchOnWindowFocus: true,
  retryOnMount: false,
})

export function usePublicConfig() {
  return useQuery(publicConfigQuery)
}

export async function refreshPublicConfig(client: QueryClient) {
  const filter = { queryKey: publicConfigQuery.queryKey, exact: true }
  await client.cancelQueries(filter)
  // Drop the pre-write snapshot even if the post-write GET fails.
  await client.resetQueries(filter, { throwOnError: true })
  // Reset refetches active queries. If nobody is observing, fetch before returning.
  return client.ensureQueryData(publicConfigQuery)
}

export function parsePublicURL(value: string | null | undefined, base?: string) {
  if (!value) return undefined
  try { return new URL(value, base) } catch { return undefined }
}
