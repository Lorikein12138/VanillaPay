import { createFileRoute } from '@tanstack/react-router'
import { DevicesPage } from '@/features/merchant/DevicesPage'
import { receiptSearch } from '@/features/merchant/collection-data'
export const Route = createFileRoute('/_merchant/devices')({ validateSearch: receiptSearch, component: Devices })
function Devices() {
  const navigate = Route.useNavigate()
  return <DevicesPage name={Route.useRouteContext().session.user.username} filters={Route.useSearch()} onFilters={(search) => { void navigate({ search }) }} />
}
