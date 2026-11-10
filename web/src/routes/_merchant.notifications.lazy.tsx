import { createLazyFileRoute } from '@tanstack/react-router'
import { NotificationsPage } from '@/features/merchant/NotificationsPage'

export const Route = createLazyFileRoute('/_merchant/notifications')({ component: Notifications })

function Notifications() { return <NotificationsPage name={Route.useRouteContext().session.user.username} /> }
