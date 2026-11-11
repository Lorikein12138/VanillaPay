import { createFileRoute, redirect } from '@tanstack/react-router'
import { ConsoleLayout } from '@/features/console/ConsoleLayout'
import { ensurePortalSession, isSessionRejected } from '@/app/portal-session'
import { consoleSafePath } from '@/lib/redirect'

export const Route = createFileRoute('/console')({
  beforeLoad: async ({ context: { queryClient }, location }) => {
    try { return { session: await ensurePortalSession(queryClient, 'console') } }
    catch (error) {
      if (isSessionRejected(error)) redirect({ to: '/console/login', search: { redirect: consoleSafePath(location.href) }, replace: true, throw: true })
      throw error
    }
  },
  component: Console,
})
function Console() { return <ConsoleLayout session={Route.useRouteContext().session} /> }
