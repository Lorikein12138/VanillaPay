import { Globe } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { changeLanguage, type Language } from '@/lib/i18n'

/** Toggles between the two supported languages without reloading. */
export function LanguageSwitch() {
  const { t, i18n } = useTranslation()
  const next: Language = i18n.resolvedLanguage === 'en' ? 'zh-CN' : 'en'
  const nextLabel = t(`language.${next}`)

  return (
    <button
      type="button"
      lang={next}
      aria-label={t('language.switchTo', { language: nextLabel })}
      onClick={() => void changeLanguage(i18n, next)}
      className="inline-flex h-11 min-w-11 items-center justify-center gap-1.5 rounded-control px-2.5 text-label text-text-2 transition-colors hover:bg-hover hover:text-text"
    >
      <Globe className="size-4" strokeWidth={1.8} aria-hidden="true" />
      {/* Icon-only on narrow screens so the top bar fits 360px; aria-label keeps the name. */}
      <span className="hidden sm:inline">{nextLabel}</span>
    </button>
  )
}
