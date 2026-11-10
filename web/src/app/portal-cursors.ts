import { hashKey, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'

/** Keep an unvisited preload briefly, but start at the first cursor after leaving a list or filter. */
export function useResetPortalCursors(queryKey: readonly unknown[]) {
  const client = useQueryClient()
  const queryHash = hashKey(queryKey)
  useEffect(() => () => {
    queueMicrotask(() => {
      client.removeQueries({ type: 'inactive', predicate: (query) => query.queryHash === queryHash })
    })
  }, [client, queryHash])
}
