import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'
import { SiteBrand } from '@/components/SiteBrand'
import { parsePublicURL, usePublicConfig } from '@/lib/public-config'

/** Secondary footer link (legal, register, recovery): touch-sized, no horizontal padding so rows align with the gutter. */
export const footerLinkClass =
  'inline-flex min-h-11 items-center text-text-2 underline-offset-4 transition-colors hover:text-text hover:underline'

const itemLinkClass = 'font-[500] text-text underline-offset-4 transition-colors hover:text-accent-text hover:underline'

function footerURL(value: string | null) {
  const url = parsePublicURL(value)
  if (url && ['http:', 'https:', 'mailto:'].includes(url.protocol)) return url
}

export function PublicFooter({ children }: { children?: ReactNode }) {
  const { t, i18n } = useTranslation()
  const { data } = usePublicConfig()

  return (
    <footer className="border-t-[0.5px] border-border">
      <div className="mx-auto flex max-w-[1200px] flex-col gap-8 px-4 pt-12 pb-4 text-label text-text-2 sm:px-6">
        <div className="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between lg:gap-16">
          <div className="flex min-w-0 shrink-0 items-center gap-2.5 text-text"><SiteBrand compact /></div>
          {!!data?.footer?.length && (
            <ul className="m-0 grid min-w-0 list-none grid-cols-1 gap-x-10 gap-y-5 p-0 sm:grid-cols-2 lg:max-w-[760px] lg:flex-1 lg:grid-cols-3">
              {data.footer.map((item) => {
                const label = i18n.resolvedLanguage === 'en' ? item.label.en.trim() || item.label.zh_CN : item.label.zh_CN
                const text = i18n.resolvedLanguage === 'en' && item.text.en.trim() ? item.text.en : item.text.zh_CN
                const url = footerURL(item.url)
                const external = url && url.protocol !== 'mailto:'
                return (
                  <li key={item.id} className="flex min-w-0 flex-col gap-1 [overflow-wrap:anywhere]">
                    {url ? (
                      <a
                        className={itemLinkClass}
                        href={url.href}
                        target={external ? '_blank' : undefined}
                        rel={external ? 'noopener noreferrer' : undefined}
                      >
                        {label}
                      </a>
                    ) : <span className="font-[500] text-text">{label}</span>}
                    {text.trim() && <p className="m-0 whitespace-pre-wrap text-caption leading-relaxed text-text-3">{text}</p>}
                  </li>
                )
              })}
            </ul>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-x-6 border-t-[0.5px] border-border text-caption">
          <Link className={footerLinkClass} to="/legal/$kind" params={{ kind: 'terms' }}>{t('polish.termsOfService')}</Link>
          <Link className={footerLinkClass} to="/legal/$kind" params={{ kind: 'privacy' }}>{t('polish.privacyPolicy')}</Link>
          {children}
        </div>
      </div>
    </footer>
  )
}
