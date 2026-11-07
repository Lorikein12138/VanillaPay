import { createFileRoute } from '@tanstack/react-router'
import { PurchasesPage } from '@/features/console/PurchasesPage'
import { purchaseSearch } from '@/features/console/manage-data'
export const Route = createFileRoute('/console/purchases')({ validateSearch: purchaseSearch, component: Purchases })
function Purchases() {
  const filters = Route.useSearch(), navigate = Route.useNavigate()
  return <PurchasesPage filters={filters} onFilters={(search) => { void navigate({ search }) }} />
}
