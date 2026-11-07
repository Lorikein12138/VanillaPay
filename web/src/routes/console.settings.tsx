import { createFileRoute } from '@tanstack/react-router'
import { ConsoleSettingsPage } from '@/features/console/SettingsPage'
import { settingsSearch } from '@/features/console/settings-data'

export const Route = createFileRoute('/console/settings')({
  validateSearch: settingsSearch,
  component: Settings,
})

function Settings() {
  const { group } = Route.useSearch()
  const navigate = Route.useNavigate()
  return <ConsoleSettingsPage group={group} onGroupChange={(next) => { void navigate({ search: { group: next } }) }} />
}
