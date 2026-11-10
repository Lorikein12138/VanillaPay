import { createLazyFileRoute } from '@tanstack/react-router'
import { Overview } from '@/features/merchant/Overview'

export const Route = createLazyFileRoute('/console/')({ component: Dashboard })

function Dashboard() { return <Overview name={Route.useRouteContext().session.user.username} role="console" /> }
