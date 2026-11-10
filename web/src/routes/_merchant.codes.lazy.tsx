import { createLazyFileRoute } from '@tanstack/react-router'
import { CodesPage } from '@/features/merchant/CodesPage'

export const Route = createLazyFileRoute('/_merchant/codes')({ component: Codes })

function Codes() { return <CodesPage name={Route.useRouteContext().session.user.username} /> }
