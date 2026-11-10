import { createFileRoute } from '@tanstack/react-router'
import { settingsQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'
import { settingsSearch } from '@/features/console/settings-data'

export const Route = createFileRoute('/console/settings')({
  validateSearch: settingsSearch,
  loader: ({ context: { queryClient, hasMountedPortal }, cause }) => preparePortalRoute(hasMountedPortal, cause, () => [queryClient.ensureQueryData(settingsQuery())]),
})
