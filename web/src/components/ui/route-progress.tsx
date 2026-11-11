import { useTranslation } from 'react-i18next'

export function RouteProgress({ active }: { active: boolean }) {
  const { t } = useTranslation()
  if (!active) return null
  return (
    <div className="sticky top-0 z-20 h-0">
      <div role="progressbar" aria-label={t('polish.loading')} className="h-0.5 w-full overflow-hidden bg-border">
        <div className="h-full w-1/3 bg-accent motion-safe:animate-route-progress" />
      </div>
    </div>
  )
}
