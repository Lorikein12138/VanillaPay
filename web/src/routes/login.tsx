import { createFileRoute } from '@tanstack/react-router'
import { lazy } from 'react'
import { safePath } from '@/lib/redirect'

export const Route = createFileRoute('/login')({
  validateSearch: (search: Record<string, unknown>): { redirect?: string; notice?: 'registered' | 'password_reset' | 'password_changed' } => ({ redirect: safePath(search.redirect), notice: search.notice === 'registered' || search.notice === 'password_reset' || search.notice === 'password_changed' ? search.notice : undefined }),
  component: Login,
})
const Page = lazy(() => import('@/features/auth/LoginPage').then((module) => ({ default: module.LoginPage })))
function Login() { const search = Route.useSearch(); return <Page redirect={search.redirect} notice={search.notice} /> }
