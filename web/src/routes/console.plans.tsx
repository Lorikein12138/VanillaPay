import { createFileRoute } from '@tanstack/react-router'
import { PlansPage } from '@/features/console/PlansPage'
export const Route = createFileRoute('/console/plans')({ component: PlansPage })
