import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { LanguageSwitch } from '@/components/LanguageSwitch'
import { SiteBrand, useSiteBrand } from '@/components/SiteBrand'
import { ThemeSwitch } from '@/components/ThemeSwitch'
import { cn } from '@/lib/utils'

/** Shared page header: brand, optional page navigation, language and theme switches. */
export function TopBar({ children, compact = false }: { children?: ReactNode; compact?: boolean }) {
  const { t } = useTranslation()
  const { name } = useSiteBrand()

  return (
    <header className="border-b-[0.5px] border-border">
      <div className={cn("mx-auto flex max-w-[1200px] items-center justify-between gap-2 px-3 sm:gap-3 sm:px-6", compact ? "min-h-12" : "min-h-[68px]")}>
        <Link to="/" aria-label={t('common.home', { name })} className="flex min-h-11 min-w-0 items-center gap-2.5 rounded-control text-text">
          <SiteBrand labelClassName="text-lg sm:text-xl" />
        </Link>
        <div className="flex shrink-0 items-center gap-1">
          {children}
          <LanguageSwitch />
          <ThemeSwitch />
        </div>
      </div>
    </header>
  )
}
