import { createFileRoute } from '@tanstack/react-router'
import { subscriptionQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'

export const Route = createFileRoute('/_merchant/subscription')({
  loader: ({ context: { queryClient, session, hasMountedPortal }, cause }) => {
    const name = session.user.username
    return preparePortalRoute(queryClient, hasMountedPortal, cause, ({ query }) => [query(subscriptionQuery(name))])
  },
})
