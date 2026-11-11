import { useEffect } from 'react'
import type { QueryClient } from '@tanstack/react-query'
import { useRouter } from '@tanstack/react-router'

export function usePreloadPortalCode(items: readonly { path: string; enabled: boolean }[], enabled = true) {
  const router = useRouter()
  useEffect(() => {
    if (!enabled) return
    function preload() {
      const ids = new Set(items.filter((item) => item.enabled).flatMap(({ path }) => (
        router.matchRoutes(path, {}).map((match) => match.routeId)
      )))
      for (const id of ids) void router.loadRouteChunk(router.routesById[id])?.catch(() => {
        // Navigation handles a failed chunk download if the user visits the route.
      })
    }
    if (typeof window.requestIdleCallback === 'function') {
      const id = window.requestIdleCallback(preload, { timeout: 2000 })
      return () => window.cancelIdleCallback(id)
    }
    const id = window.setTimeout(preload, 0)
    return () => window.clearTimeout(id)
  }, [enabled, items, router])
}

type PortalQueries = {
  query: QueryClient['ensureQueryData']
  infinite: QueryClient['ensureInfiniteQueryData']
}

export function preparePortalRoute(
  client: QueryClient,
  hasMountedPortal: () => boolean,
  cause: 'enter' | 'stay' | 'preload',
  fetchQueries: (queries: PortalQueries) => Promise<unknown>[],
) {
  // The mounted query observer handles search changes and retains the previous rows.
  if (cause === 'stay') return
  const prefetch = Promise.allSettled(fetchQueries({
    query: (options) => client.ensureQueryData({ ...options, revalidateIfStale: true }),
    infinite: (options) => client.ensureInfiniteQueryData({ ...options, revalidateIfStale: true }),
  }))
  // Cached queries resolve immediately. On initial entry the page can show its own loading state.
  if (cause === 'preload' || hasMountedPortal()) {
    return prefetch.then(() => {})
  }
}
