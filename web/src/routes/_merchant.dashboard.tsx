import { createFileRoute } from '@tanstack/react-router'
import { overviewQueries, deviceQuery, recentOrdersQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'

export const Route = createFileRoute('/_merchant/dashboard')({
  loader: ({ context: { queryClient, session, hasMountedPortal }, cause }) => {
    const name = session.user.username
    return preparePortalRoute(queryClient, hasMountedPortal, cause, ({ query }) => [
      ...overviewQueries('merchant', name).map((options) => query(options)),
      query(deviceQuery(name)),
      query(recentOrdersQuery(name)),
    ])
  },
})
