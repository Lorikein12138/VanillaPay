import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { LanguageSwitch } from '@/components/LanguageSwitch'
import { SiteBrand, useSiteBrand } from '@/components/SiteBrand'
import { ThemeSwitch } from '@/components/ThemeSwitch'

/** Shared page header: brand, optional page navigation, language and theme switches. */
export function TopBar({ children }: { children?: ReactNode }) {
  const { t } = useTranslation()
  const { name } = useSiteBrand()

  return (
    <header className="border-b-[0.5px] border-border">
      <div className="mx-auto flex min-h-[68px] max-w-[1200px] items-center justify-between gap-3 px-4 sm:px-6">
        <Link to="/" aria-label={t('common.home', { name })} className="flex min-h-11 min-w-0 items-center gap-2.5 rounded-control text-text">
          <SiteBrand />
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
