import { createFileRoute, notFound } from '@tanstack/react-router'
import { lazy } from 'react'
const Page = lazy(() => import('@/features/auth/LegalPage').then((module) => ({ default: module.LegalPage })))
export const Route = createFileRoute('/legal/$kind')({ beforeLoad: ({ params }): { kind: 'terms' | 'privacy' } => { if (params.kind !== 'terms' && params.kind !== 'privacy') notFound({ throw: true }); return { kind: params.kind as 'terms' | 'privacy' } }, component: Legal })
function Legal() { return <Page kind={Route.useRouteContext().kind} /> }
