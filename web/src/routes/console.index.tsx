import { createFileRoute } from '@tanstack/react-router'
import { overviewQueries } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'

export const Route = createFileRoute('/console/')({
  loader: ({ context: { queryClient, session, hasMountedPortal }, cause }) => {
    const name = session.user.username
    return preparePortalRoute(hasMountedPortal, cause, () => overviewQueries('console', name).map((query) => queryClient.ensureQueryData(query)))
  },
})
