import { createFileRoute } from '@tanstack/react-router'
import { codesQuery, collectionQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'

export const Route = createFileRoute('/_merchant/codes')({
  loader: ({ context: { queryClient, session, hasMountedPortal }, cause }) => {
    const name = session.user.username
    return preparePortalRoute(queryClient, hasMountedPortal, cause, ({ query }) => [query(codesQuery(name)), query(collectionQuery(name))])
  },
})
