import { createFileRoute, notFound, redirect } from '@tanstack/react-router'
import { lazy } from 'react'
import { api, APIError } from '@/api/client'
import { isMerchantPath } from '@/features/merchant/navigation'

export const Route = createFileRoute('/_merchant')({
  beforeLoad: async ({ location }) => {
    if (!isMerchantPath(location.pathname)) notFound({ routeId: '__root__', throw: true })
    try {
      const session = await api.me(false)
      if (session.user.banned && location.pathname !== '/banned') redirect({ to: '/banned', replace: true, throw: true })
      if (!session.user.banned && location.pathname === '/banned') redirect({ to: '/dashboard', replace: true, throw: true })
      return { session }
    } catch (error) {
      if (error instanceof APIError && (error.status === 401 || (error.status === 403 && error.problem?.code === 'FORBIDDEN'))) redirect({ to: '/login', search: { redirect: location.href }, replace: true, throw: true })
      throw error
    }
  },
  component: Merchant,
})
const Layout = lazy(() => import('@/features/merchant/MerchantLayout').then((module) => ({ default: module.MerchantLayout })))
function Merchant() { return <Layout session={Route.useRouteContext().session} /> }
