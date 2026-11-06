import { createFileRoute } from '@tanstack/react-router'
import { lazy } from 'react'
const Page = lazy(() => import('@/features/merchant/AccountPage').then((module) => ({ default: module.AccountPage })))
export const Route = createFileRoute('/_merchant/account')({ component: Page })
