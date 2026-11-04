import { createFileRoute } from '@tanstack/react-router'
import { Overview } from '@/features/merchant/Overview'

export const Route = createFileRoute('/_merchant/dashboard')({ component: Dashboard })
function Dashboard() { return <Overview name={Route.useRouteContext().session.user.username} /> }
