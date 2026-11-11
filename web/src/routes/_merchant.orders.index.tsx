import { createFileRoute } from '@tanstack/react-router'
import { ordersQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'
import { orderSearch } from '@/features/merchant/orders-data'

export const Route = createFileRoute('/_merchant/orders/')({
  loaderDeps: ({ search }) => orderSearch(search),
  loader: ({ context: { queryClient, session, hasMountedPortal }, cause, deps }) => {
    const name = session.user.username
    return preparePortalRoute(queryClient, hasMountedPortal, cause, ({ infinite }) => [infinite(ordersQuery('merchant', name, deps))])
  },
})
