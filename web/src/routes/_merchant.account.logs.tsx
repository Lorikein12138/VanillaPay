import { createFileRoute } from '@tanstack/react-router'
import { AuditLogsPage } from '@/features/merchant/AuditLogsPage'
import { auditSearch } from '@/features/merchant/audit-data'
export const Route = createFileRoute('/_merchant/account/logs')({ validateSearch: auditSearch, component: Logs })
function Logs() {
  const navigate = Route.useNavigate()
  return <AuditLogsPage
    name={Route.useRouteContext().session.user.username}
    filters={Route.useSearch({ select: auditSearch })}
    onFilters={(search) => void navigate({ search })}
  />
}
