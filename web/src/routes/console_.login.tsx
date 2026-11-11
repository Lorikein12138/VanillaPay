import { createFileRoute, redirect } from '@tanstack/react-router'
import { lazy } from 'react'
import { consoleApi } from '@/api/client'
import { ensurePortalSession, isSessionRejected } from '@/app/portal-session'
import { consoleDestination, consoleSafePath } from '@/lib/redirect'

export const Route = createFileRoute('/console_/login')({
  validateSearch: (search: Record<string, unknown>): { redirect?: string; notice?: 'initialized' | 'repaired' | 'password_changed' } => ({ redirect: consoleSafePath(search.redirect), notice: search.notice === 'initialized' || search.notice === 'repaired' || search.notice === 'password_changed' ? search.notice : undefined }),
  beforeLoad: async ({ context: { queryClient }, search }) => {
    const status = await consoleApi.bootstrapStatus()
    if (!status.initialized) redirect({ to: '/console/setup', replace: true, throw: true })
    try { await ensurePortalSession(queryClient, 'console') }
    catch (error) {
      if (isSessionRejected(error)) return
      throw error
    }
    redirect({ to: consoleDestination(search.redirect, undefined), replace: true, throw: true })
  },
  component: Login,
})
const Page = lazy(() => import('@/features/auth/LoginPage').then((module) => ({ default: module.LoginPage })))
function Login() { const search = Route.useSearch(); return <Page role="console" redirect={search.redirect} notice={search.notice} /> }
