import { createLazyFileRoute } from '@tanstack/react-router'
import { ConsoleSettingsPage } from '@/features/console/SettingsPage'

export const Route = createLazyFileRoute('/console/settings')({ component: Settings })

function Settings() {
  const { group } = Route.useSearch()
  const navigate = Route.useNavigate()
  return <ConsoleSettingsPage group={group} onGroupChange={(next) => { void navigate({ search: { group: next } }) }} />
}
