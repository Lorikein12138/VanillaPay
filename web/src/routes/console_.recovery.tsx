import { createFileRoute } from '@tanstack/react-router'
import { lazy } from 'react'

const Page = lazy(() => import('@/features/console/RecoveryPage').then((module) => ({ default: module.RecoveryPage })))
export const Route = createFileRoute('/console_/recovery')({ component: Page })
