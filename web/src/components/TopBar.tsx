import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { LanguageSwitch } from '@/components/LanguageSwitch'
import { LogoMark } from '@/components/Logo'
import { ThemeSwitch } from '@/components/ThemeSwitch'

/** Shared page header: brand, optional page navigation, language and theme switches. */
export function TopBar({ children }: { children?: ReactNode }) {
  const { t } = useTranslation()

  return (
    <header className="border-b border-border">
      <div className="mx-auto flex min-h-[68px] max-w-[1200px] items-center justify-between gap-3 px-4 sm:px-6">
        <Link to="/" aria-label={t('common.home')} className="flex items-center gap-2.5 rounded-control text-text">
          <LogoMark />
          <span className="font-serif text-xl font-medium">{t('common.brand')}</span>
        </Link>
        <div className="flex items-center gap-1">
          {children}
          <LanguageSwitch />
          <ThemeSwitch />
        </div>
      </div>
    </header>
  )
}
