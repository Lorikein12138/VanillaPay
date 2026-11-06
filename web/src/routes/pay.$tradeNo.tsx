import { lazy } from 'react'
import { createFileRoute } from '@tanstack/react-router'

const Page = lazy(() => import('@/features/checkout/CheckoutPage').then((module) => ({ default: module.CheckoutPage })))
export const Route = createFileRoute('/pay/$tradeNo')({ component: Checkout })
function Checkout() {
  const { tradeNo } = Route.useParams()
  return <Page key={tradeNo} tradeNo={tradeNo} />
}
