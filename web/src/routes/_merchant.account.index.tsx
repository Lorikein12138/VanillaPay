import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

export const Route = createFileRoute('/_merchant/account/')({
  component: lazyRouteComponent(() => import('@/features/merchant/AccountPage'), 'AccountPage'),
})
