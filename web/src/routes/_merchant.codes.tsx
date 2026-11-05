import { createFileRoute } from '@tanstack/react-router'
import { CodesPage } from '@/features/merchant/CodesPage'
export const Route = createFileRoute('/_merchant/codes')({ component: Codes })
function Codes() { return <CodesPage name={Route.useRouteContext().session.user.username} /> }
