import { createFileRoute } from '@tanstack/react-router'
import { SubscriptionPage } from '@/features/merchant/SubscriptionPage'
export const Route = createFileRoute('/_merchant/subscription')({ component: Subscription })
function Subscription() { return <SubscriptionPage name={Route.useRouteContext().session.user.username} /> }
