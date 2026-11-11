import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'
import { announcementsQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'

export const Route = createFileRoute('/console/announcements')({
  loader: ({ context: { queryClient, hasMountedPortal }, cause }) => preparePortalRoute(queryClient, hasMountedPortal, cause, ({ infinite }) => [infinite(announcementsQuery())]),
  component: lazyRouteComponent(() => import('@/features/console/AnnouncementsPage'), 'AnnouncementsPage'),
})
