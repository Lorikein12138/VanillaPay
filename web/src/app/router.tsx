import { createRouter, type RouterHistory } from '@tanstack/react-router'
import { routeTree } from '@/routeTree.gen'
import { onUnauthorized } from '@/api/client'
import { safePath } from '@/lib/redirect'

export function createAppRouter(options: { history?: RouterHistory } = {}) {
  const router = createRouter({
    routeTree,
    history: options.history,
    // Pages and their data load only when visited.
    defaultPreload: false,
    scrollRestoration: true,
  })
  onUnauthorized(() => {
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
