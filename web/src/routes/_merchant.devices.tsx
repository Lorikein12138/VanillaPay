import { createFileRoute } from '@tanstack/react-router'
import { deviceQuery, receiptsQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'
import { receiptSearch } from '@/features/merchant/collection-data'

export const Route = createFileRoute('/_merchant/devices')({
  validateSearch: receiptSearch,
  loaderDeps: ({ search }) => receiptSearch(search),
  loader: ({ context: { queryClient, session, hasMountedPortal }, cause, deps }) => {
    const name = session.user.username
    return preparePortalRoute(queryClient, hasMountedPortal, cause, ({ query, infinite }) => [query(deviceQuery(name)), infinite(receiptsQuery(name, deps))])
  },
})
