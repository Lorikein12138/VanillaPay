import { createFileRoute } from '@tanstack/react-router'
import { deviceQuery, receiptsQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'
import { receiptSearch } from '@/features/merchant/collection-data'

export const Route = createFileRoute('/_merchant/devices')({
  validateSearch: receiptSearch,
  loaderDeps: ({ search }) => receiptSearch(search),
  loader: ({ context: { queryClient, session, hasMountedPortal }, cause, deps }) => {
    const name = session.user.username
    return preparePortalRoute(hasMountedPortal, cause, () => [queryClient.ensureQueryData(deviceQuery(name)), queryClient.ensureInfiniteQueryData(receiptsQuery(name, deps))])
  },
})
