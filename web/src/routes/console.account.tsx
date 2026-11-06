import { createFileRoute } from '@tanstack/react-router'
import { lazy } from 'react'

const Page = lazy(() => import('@/features/console/AccountPage').then((module) => ({ default: module.ConsoleAccountPage })))
export const Route = createFileRoute('/console/account')({ component: Page })
