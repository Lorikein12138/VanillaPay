import { createFileRoute, notFound, redirect } from '@tanstack/react-router'
import { MerchantLayout } from '@/features/merchant/MerchantLayout'
import { ensurePortalSession, isSessionRejected } from '@/app/portal-session'
import { isMerchantPath, restrictedMerchantPath } from '@/features/merchant/navigation'

export const Route = createFileRoute('/_merchant')({
  beforeLoad: async ({ context: { queryClient }, location }) => {
    if (!isMerchantPath(location.pathname)) notFound({ routeId: '__root__', throw: true })
    try {
      const session = await ensurePortalSession(queryClient, 'merchant')
      const restrictedPath = restrictedMerchantPath(session.user)
      if (restrictedPath && location.pathname !== restrictedPath) redirect({ to: restrictedPath, replace: true, throw: true })
      if (!restrictedPath && (location.pathname === '/banned' || location.pathname === '/registration-review')) redirect({ to: '/dashboard', replace: true, throw: true })
      return { session }
    } catch (error) {
      if (isSessionRejected(error)) redirect({ to: '/login', search: { redirect: location.href }, replace: true, throw: true })
      throw error
    }
  },
  component: Merchant,
})
function Merchant() { return <MerchantLayout session={Route.useRouteContext().session} /> }
