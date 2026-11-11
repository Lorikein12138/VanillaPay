import { createFileRoute } from '@tanstack/react-router'
import { notificationsQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'

export const Route = createFileRoute('/_merchant/notifications')({
  loader: ({ context: { queryClient, session, hasMountedPortal }, cause }) => {
    const name = session.user.username
    return preparePortalRoute(queryClient, hasMountedPortal, cause, ({ infinite }) => [infinite(notificationsQuery(name))])
  },
})
