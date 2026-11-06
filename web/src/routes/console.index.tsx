import { createFileRoute } from '@tanstack/react-router'
import { Overview } from '@/features/merchant/Overview'

export const Route = createFileRoute('/console/')({ component: Dashboard })
function Dashboard() { return <Overview name={Route.useRouteContext().session.user.username} role="console" /> }
