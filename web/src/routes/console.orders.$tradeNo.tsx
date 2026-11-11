import { createFileRoute } from '@tanstack/react-router'
import { preparePortalRoute } from '@/app/portal-preload'
import { orderDetailQuery } from '@/app/portal-queries'
import { OrderDetail } from '@/features/merchant/OrderDetail'
export const Route = createFileRoute('/console/orders/$tradeNo')({
  loader: ({ context: { queryClient, hasMountedPortal, session }, params, cause }) =>
    preparePortalRoute(queryClient, hasMountedPortal, cause, ({ query }) => [query(orderDetailQuery('console', session.user.username, params.tradeNo))]),
  component: Detail,
})
function Detail() {
  const { tradeNo } = Route.useParams()
  return <OrderDetail key={tradeNo} role="console" name={Route.useRouteContext().session.user.username} tradeNo={tradeNo} filters={Route.useSearch()} />
}
