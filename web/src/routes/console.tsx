import { createFileRoute, redirect } from '@tanstack/react-router'
import { lazy } from 'react'
import { APIError, consoleApi } from '@/api/client'
import { consoleSafePath } from '@/lib/redirect'

export const Route = createFileRoute('/console')({
  beforeLoad: async ({ location }) => {
    try { return { session: await consoleApi.me(false) } }
    catch (error) {
      if (error instanceof APIError && (error.status === 401 || (error.status === 403 && error.problem?.code === 'FORBIDDEN'))) redirect({ to: '/console/login', search: { redirect: consoleSafePath(location.href) }, replace: true, throw: true })
      throw error
    }
  },
  component: Console,
})
const Layout = lazy(() => import('@/features/console/ConsoleLayout').then((module) => ({ default: module.ConsoleLayout })))
function Console() { return <Layout session={Route.useRouteContext().session} /> }
