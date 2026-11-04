import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

export const Route = createFileRoute('/_merchant/banned')({ component: lazyRouteComponent(() => import('@/features/merchant/BannedPage'), 'BannedPage') })
