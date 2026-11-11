import { createFileRoute } from '@tanstack/react-router'
import { plansQuery, purchasesQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'
import { purchaseSearch } from '@/features/console/manage-data'

export const Route = createFileRoute('/console/purchases')({
  validateSearch: purchaseSearch,
  loaderDeps: ({ search }) => purchaseSearch(search),
  loader: ({ context: { queryClient, hasMountedPortal }, cause, deps }) => preparePortalRoute(queryClient, hasMountedPortal, cause, ({ query, infinite }) => [infinite(purchasesQuery(deps)), query(plansQuery(true))]),
})
