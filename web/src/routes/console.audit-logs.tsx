import { createFileRoute } from '@tanstack/react-router'
import { auditQuery } from '@/app/portal-queries'
import { preparePortalRoute } from '@/app/portal-preload'
import { consoleAuditSearch } from '@/features/merchant/audit-data'

export const Route = createFileRoute('/console/audit-logs')({
  validateSearch: consoleAuditSearch,
  loaderDeps: ({ search }) => consoleAuditSearch(search),
  loader: ({ context: { queryClient, session, hasMountedPortal }, cause, deps }) => {
    const name = session.user.username
    return preparePortalRoute(queryClient, hasMountedPortal, cause, ({ infinite }) => [infinite(auditQuery('console', name, deps))])
  },
})
