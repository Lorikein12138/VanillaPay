import { createLazyFileRoute } from '@tanstack/react-router'
import { DevicesPage } from '@/features/merchant/DevicesPage'

export const Route = createLazyFileRoute('/_merchant/devices')({ component: Devices })

function Devices() {
  const navigate = Route.useNavigate()
  return <DevicesPage name={Route.useRouteContext().session.user.username} filters={Route.useSearch()} onFilters={(search) => { void navigate({ search }) }} />
}
