import { hashKey, useQueryClient, type InfiniteData } from '@tanstack/react-query'
import { useEffect } from 'react'

/** Start at the first cursor when returning to a list or filter. Keep its first page cached. */
export function useResetPortalCursors(queryKey: readonly unknown[]) {
  const client = useQueryClient()
  const queryHash = hashKey(queryKey)
  useEffect(() => () => {
    queueMicrotask(() => {
      const query = client.getQueryCache().get(queryHash)
      if (!query || query.isActive()) return
      if (query.state.fetchMeta?.fetchMore) void query.cancel({ revert: true })
      const data = query.state.data as InfiniteData<unknown> | undefined
      if (data && data.pages.length > 1) {
        client.setQueryData(query.queryKey, {
          pages: data.pages.slice(0, 1),
          pageParams: data.pageParams.slice(0, 1),
        }, { updatedAt: query.state.dataUpdatedAt })
      }
    })
  }, [client, queryHash])
}
