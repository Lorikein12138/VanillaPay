import { createFileRoute } from '@tanstack/react-router'
import { AuditLogsPage } from '@/features/merchant/AuditLogsPage'
import { consoleAuditSearch } from '@/features/merchant/audit-data'

export const Route = createFileRoute('/console/audit-logs')({ validateSearch: consoleAuditSearch, component: AuditLogs })

function AuditLogs() {
  const { session } = Route.useRouteContext()
  const navigate = Route.useNavigate()
  return (
    <AuditLogsPage
      role="console"
      name={session.user.username}
      filters={Route.useSearch({ select: consoleAuditSearch })}
      onFilters={(search) => void navigate({ search })}
    />
  )
}
