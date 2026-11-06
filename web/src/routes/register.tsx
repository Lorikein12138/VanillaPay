import { createFileRoute } from '@tanstack/react-router'
import { lazy } from 'react'
const Page = lazy(() => import('@/features/auth/PublicAccountPage').then((module) => ({ default: module.PublicAccountPage })))
export const Route = createFileRoute('/register')({ component: () => <Page mode="register" /> })
