import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

export const Route = createFileRoute('/console/account')({
  component: lazyRouteComponent(() => import('@/features/console/AccountPage'), 'ConsoleAccountPage'),
})
