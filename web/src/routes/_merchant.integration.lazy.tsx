import { createLazyFileRoute } from '@tanstack/react-router'
import { IntegrationPage } from '@/features/merchant/IntegrationPage'

export const Route = createLazyFileRoute('/_merchant/integration')({ component: Integration })

function Integration() { return <IntegrationPage user={Route.useRouteContext().session.user} /> }
