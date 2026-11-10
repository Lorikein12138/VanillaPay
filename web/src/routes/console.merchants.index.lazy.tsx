import { createLazyFileRoute } from '@tanstack/react-router'
import { MerchantsPage } from '@/features/console/MerchantsPage'

export const Route = createLazyFileRoute('/console/merchants/')({ component: Merchants })

function Merchants() {
  const filters = Route.useSearch(), navigate = Route.useNavigate()
  return <MerchantsPage filters={filters} onFilters={(search) => { void navigate({ search }) }} />
}
