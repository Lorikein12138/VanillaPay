import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'
import { plansQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'

export const Route = createFileRoute('/console/plans')({
  loader: ({ context: { queryClient, hasMountedPortal }, cause }) => preparePortalRoute(queryClient, hasMountedPortal, cause, ({ query }) => [query(plansQuery(false))]),
  component: lazyRouteComponent(() => import('@/features/console/PlansPage'), 'PlansPage'),
})
