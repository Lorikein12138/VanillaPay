import { createFileRoute } from '@tanstack/react-router'
import { OrdersPage } from '@/features/merchant/OrdersPage'
export const Route = createFileRoute('/console/orders/')({ component: Orders })
function Orders() {
  const { session } = Route.useRouteContext(), filters = Route.useSearch(), navigate = Route.useNavigate()
  return <OrdersPage role="console" name={session.user.username} filters={filters} onFilters={(search) => { void navigate({ search }) }} />
}
