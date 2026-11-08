import { createFileRoute } from '@tanstack/react-router'
import { AnnouncementsPage } from '@/features/console/AnnouncementsPage'

export const Route = createFileRoute('/console/announcements')({ component: AnnouncementsPage })
