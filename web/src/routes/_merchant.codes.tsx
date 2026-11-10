import { createFileRoute } from '@tanstack/react-router'
import { codesQuery, collectionQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'

export const Route = createFileRoute('/_merchant/codes')({
  loader: ({ context: { queryClient, session, hasMountedPortal }, cause }) => {
    const name = session.user.username
    return preparePortalRoute(hasMountedPortal, cause, () => [queryClient.ensureQueryData(codesQuery(name)), queryClient.ensureQueryData(collectionQuery(name))])
  },
})
