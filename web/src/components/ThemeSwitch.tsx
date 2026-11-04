import { Monitor, Moon, Sun, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { setTheme, themePreferences, useTheme, type ThemePreference } from '@/lib/theme'
import { cn } from '@/lib/utils'

const icons: Record<ThemePreference, LucideIcon> = {
  system: Monitor,
  light: Sun,
  dark: Moon,
}

/** Segmented system / light / dark control; switching never reloads the page. */
export function ThemeSwitch() {
  const { t } = useTranslation()
  const theme = useTheme()

  return (
    <div role="group" aria-label={t('theme.label')} className="inline-flex gap-0.5 rounded-control p-0.5">
      {themePreferences.map((preference) => {
        const Icon = icons[preference]
        const selected = theme === preference
        return (
          <button
            key={preference}
            type="button"
            aria-pressed={selected}
            aria-label={t(`theme.${preference}`)}
            title={t(`theme.${preference}`)}
            onClick={() => setTheme(preference)}
            className={cn(
              'grid size-11 place-items-center rounded-[6px] text-text-2 transition-colors hover:text-text',
              selected && 'bg-hover text-text',
            )}
          >
            <Icon className="size-4" strokeWidth={1.8} aria-hidden="true" />
          </button>
        )
      })}
    </div>
  )
}
