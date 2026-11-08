import { queryOptions, useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'

export const publicConfigQuery = queryOptions({
  queryKey: ['public-config'] as const,
  queryFn: api.config,
  staleTime: 60_000,
  refetchOnWindowFocus: true,
  retryOnMount: false,
})

export function usePublicConfig() {
  return useQuery(publicConfigQuery)
}

export function parsePublicURL(value: string | null | undefined) {
  if (!value) return undefined
  try { return new URL(value) } catch { return undefined }
}
