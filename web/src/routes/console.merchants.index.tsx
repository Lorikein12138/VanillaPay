import { createFileRoute } from '@tanstack/react-router'
import { MerchantsPage } from '@/features/console/MerchantsPage'
import { merchantSearch } from '@/features/console/manage-data'
export const Route = createFileRoute('/console/merchants/')({ validateSearch: merchantSearch, component: Merchants })
function Merchants() {
  const filters = Route.useSearch(), navigate = Route.useNavigate()
  return <MerchantsPage filters={filters} onFilters={(search) => { void navigate({ search }) }} />
}
