import { createFileRoute } from '@tanstack/react-router'
import { OrderDetail } from '@/features/merchant/OrderDetail'
export const Route = createFileRoute('/console/orders/$tradeNo')({ component: Detail })
function Detail() {
  const { tradeNo } = Route.useParams()
  return <OrderDetail key={tradeNo} role="console" name={Route.useRouteContext().session.user.username} tradeNo={tradeNo} filters={Route.useSearch()} />
}
