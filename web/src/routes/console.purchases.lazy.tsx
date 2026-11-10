import { createLazyFileRoute } from '@tanstack/react-router'
import { PurchasesPage } from '@/features/console/PurchasesPage'

export const Route = createLazyFileRoute('/console/purchases')({ component: Purchases })

function Purchases() {
  const filters = Route.useSearch(), navigate = Route.useNavigate()
  return <PurchasesPage filters={filters} onFilters={(search) => { void navigate({ search }) }} />
}
