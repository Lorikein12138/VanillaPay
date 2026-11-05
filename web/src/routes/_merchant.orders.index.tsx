import { createFileRoute } from '@tanstack/react-router'
import { OrdersPage } from '@/features/merchant/OrdersPage'

export const Route = createFileRoute('/_merchant/orders/')({ component: Orders })
function Orders() {
  const { session } = Route.useRouteContext()
  const filters = Route.useSearch()
  const navigate = Route.useNavigate()
  return <OrdersPage name={session.user.username} filters={filters} onFilters={(search) => { void navigate({ search }) }} />
}
