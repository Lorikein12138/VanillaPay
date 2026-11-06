import { createFileRoute, redirect } from '@tanstack/react-router'
import { lazy } from 'react'
import { APIError, consoleApi } from '@/api/client'
import { consoleDestination, consoleSafePath } from '@/lib/redirect'

export const Route = createFileRoute('/console_/login')({
  validateSearch: (search: Record<string, unknown>): { redirect?: string; notice?: 'initialized' | 'repaired' | 'password_changed' } => ({ redirect: consoleSafePath(search.redirect), notice: search.notice === 'initialized' || search.notice === 'repaired' || search.notice === 'password_changed' ? search.notice : undefined }),
  beforeLoad: async ({ search }) => {
    const status = await consoleApi.bootstrapStatus()
    if (!status.initialized) redirect({ to: '/console/setup', replace: true, throw: true })
    try { await consoleApi.me(false) }
    catch (error) {
      if (error instanceof APIError && (error.status === 401 || (error.status === 403 && error.problem?.code === 'FORBIDDEN'))) return
      throw error
    }
    redirect({ to: consoleDestination(search.redirect, undefined), replace: true, throw: true })
  },
  component: Login,
})
const Page = lazy(() => import('@/features/auth/LoginPage').then((module) => ({ default: module.LoginPage })))
function Login() { const search = Route.useSearch(); return <Page role="console" redirect={search.redirect} notice={search.notice} /> }
