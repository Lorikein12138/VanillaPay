import { createRouter, defaultParseSearch, type RouterHistory } from '@tanstack/react-router'
import { routeTree } from '@/routeTree.gen'
import { onUnauthorized } from '@/api/client'
import { consoleSafePath, safePath } from '@/lib/redirect'

function parseSearch(search: string) {
  const params = new URLSearchParams(search)
  const queries = params.getAll('query')
  if (!queries.length) return defaultParseSearch(search)

  // Keep search terms out of the router's number and JSON coercion.
  params.delete('query')
  const parsed: Record<string, unknown> = defaultParseSearch(params.toString())
  if (queries.length !== 1) return parsed
  const query = queries[0]!
  parsed.query = query
  if (query.startsWith('"')) {
    try {
      const decoded: unknown = JSON.parse(query)
      if (typeof decoded === 'string') parsed.query = decoded
    } catch { /* Preserve literal quotes in search terms. */ }
  }
  return parsed
}

export function createAppRouter(options: { history?: RouterHistory } = {}) {
  const router = createRouter({
    routeTree,
    history: options.history,
    parseSearch,
    // Pages and their data load only when visited.
    defaultPreload: false,
    scrollRestoration: true,
  })
  onUnauthorized(() => {
    const path = router.state.location.pathname
    if (path === '/console' || path.startsWith('/console/')) {
      if (!['/console/login', '/console/setup', '/console/recovery'].includes(path)) void router.navigate({ to: '/console/login', search: { redirect: consoleSafePath(router.state.location.href) }, replace: true })
      return
    }
    if (router.state.location.pathname !== '/login') {
      void router.navigate({ to: '/login', search: { redirect: safePath(router.state.location.href) }, replace: true })
    }
  })
  return router
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof createAppRouter>
  }
}
