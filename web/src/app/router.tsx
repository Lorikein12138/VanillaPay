import { createRouter, defaultParseSearch, type RouterHistory } from '@tanstack/react-router'
import { routeTree } from '@/routeTree.gen'
import { onUnauthorized } from '@/api/client'
import { consoleSafePath, safePath } from '@/lib/redirect'

function searchText(value: string) {
  if (value.startsWith('"')) {
    try {
      const decoded: unknown = JSON.parse(value)
      if (typeof decoded === 'string') return decoded
    } catch { /* Preserve literal quotes in search terms. */ }
  }
  return value
}

function parseSearch(search: string) {
  const params = new URLSearchParams(search)
  const queries = params.getAll('query')
  const merchantIds = params.getAll('merchant_id')
  if (!queries.length && !merchantIds.length) return defaultParseSearch(search)

  // Keep search terms out of the router's number and JSON coercion.
  params.delete('query')
  params.delete('merchant_id')
  const parsed: Record<string, unknown> = defaultParseSearch(params.toString())
  // Preserve exact int64 IDs and leave non-decimal syntax for the audit validator to reject.
  if (merchantIds.length === 1) {
    const id = searchText(merchantIds[0]!)
    const numeric = Number(id)
    parsed.merchant_id = Number.isSafeInteger(numeric) && String(numeric) === id ? numeric : id
  }
  if (queries.length === 1) parsed.query = searchText(queries[0]!)
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
