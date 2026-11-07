import { createFileRoute } from '@tanstack/react-router'
import { MerchantDetail } from '@/features/console/MerchantDetail'
export const Route = createFileRoute('/console/merchants/$id')({ component: Detail })
function Detail() { const { id } = Route.useParams(); return <MerchantDetail key={id} id={id} /> }
