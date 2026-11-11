import { createFileRoute } from '@tanstack/react-router'
import { merchantsQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'
import { merchantSearch } from '@/features/console/manage-data'

export const Route = createFileRoute('/console/merchants/')({
  validateSearch: merchantSearch,
  loaderDeps: ({ search }) => merchantSearch(search),
  loader: ({ context: { queryClient, hasMountedPortal }, cause, deps }) => preparePortalRoute(queryClient, hasMountedPortal, cause, ({ infinite }) => [infinite(merchantsQuery(deps))]),
})
