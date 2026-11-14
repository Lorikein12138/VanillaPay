import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

export const Route = createFileRoute('/_merchant/registration-review')({
  component: lazyRouteComponent(() => import('@/features/merchant/RegistrationReviewPage'), 'RegistrationReviewPage'),
})
