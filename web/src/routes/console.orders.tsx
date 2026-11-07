import { createFileRoute, Outlet } from '@tanstack/react-router'
import { orderSearch } from '@/features/merchant/orders-data'
export const Route = createFileRoute('/console/orders')({ validateSearch: orderSearch, component: Outlet })
