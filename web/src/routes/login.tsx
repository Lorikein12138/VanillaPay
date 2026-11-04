import { createFileRoute } from '@tanstack/react-router'
import { lazy } from 'react'
import { safePath } from '@/lib/redirect'

export const Route = createFileRoute('/login')({
  validateSearch: (search: Record<string, unknown>): { redirect?: string } => ({ redirect: safePath(search.redirect) }),
  component: Login,
})
const Page = lazy(() => import('@/features/auth/LoginPage').then((module) => ({ default: module.LoginPage })))
function Login() { return <Page redirect={Route.useSearch().redirect} /> }
