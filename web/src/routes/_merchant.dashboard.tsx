import { createFileRoute } from '@tanstack/react-router'
import { overviewQueries, deviceQuery, recentOrdersQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'

export const Route = createFileRoute('/_merchant/dashboard')({
  loader: ({ context: { queryClient, session, hasMountedPortal }, cause }) => {
    const name = session.user.username
    return preparePortalRoute(hasMountedPortal, cause, () => [
      ...overviewQueries('merchant', name).map((query) => queryClient.ensureQueryData(query)),
      queryClient.ensureQueryData(deviceQuery(name)),
      queryClient.ensureQueryData(recentOrdersQuery(name)),
    ])
  },
})
