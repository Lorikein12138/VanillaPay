import { createFileRoute, redirect } from '@tanstack/react-router'
import { lazy } from 'react'
import { consoleApi } from '@/api/client'

const Page = lazy(() => import('@/features/console/SetupPage').then((module) => ({ default: module.SetupPage })))
export const Route = createFileRoute('/console_/setup')({
  beforeLoad: async () => { if ((await consoleApi.bootstrapStatus()).initialized) redirect({ to: '/console/login', search: { notice: 'initialized' }, replace: true, throw: true }) },
  component: Page,
})
