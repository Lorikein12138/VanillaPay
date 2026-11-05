import { createFileRoute } from '@tanstack/react-router'
import { OrderDetail } from '@/features/merchant/OrderDetail'

export const Route = createFileRoute('/_merchant/orders/$tradeNo')({ component: Detail })
function Detail() {
  const { tradeNo } = Route.useParams()
  return <OrderDetail key={tradeNo} name={Route.useRouteContext().session.user.username} tradeNo={tradeNo} filters={Route.useSearch()} />
}
