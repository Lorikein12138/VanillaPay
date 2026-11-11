import { createFileRoute } from '@tanstack/react-router'
import { auditQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'
import { auditSearch } from '@/features/merchant/audit-data'

export const Route = createFileRoute('/_merchant/account/logs')({
  validateSearch: auditSearch,
  loaderDeps: ({ search }) => auditSearch(search),
  loader: ({ context: { queryClient, session, hasMountedPortal }, cause, deps }) => {
    const name = session.user.username
    return preparePortalRoute(queryClient, hasMountedPortal, cause, ({ infinite }) => [infinite(auditQuery('merchant', name, deps))])
  },
})
