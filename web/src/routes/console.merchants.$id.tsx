import { createFileRoute } from '@tanstack/react-router'
import { preparePortalRoute } from '@/app/portal-preload'
import { merchantDetailQuery } from '@/app/portal-queries'
import { MerchantDetail } from '@/features/console/MerchantDetail'
export const Route = createFileRoute('/console/merchants/$id')({
  loader: ({ context: { queryClient, hasMountedPortal }, params, cause }) =>
    preparePortalRoute(queryClient, hasMountedPortal, cause, ({ query }) => [query(merchantDetailQuery(params.id))]),
  component: Detail,
})
function Detail() { const { id } = Route.useParams(); return <MerchantDetail key={id} id={id} /> }
