import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'
import { SiteBrand } from '@/components/SiteBrand'
import { buttonVariants } from '@/components/ui/button'
import { parsePublicURL, usePublicConfig } from '@/lib/public-config'
import { cn } from '@/lib/utils'

function footerURL(value: string | null) {
  const url = parsePublicURL(value)
  if (url && ['http:', 'https:', 'mailto:'].includes(url.protocol)) return url
}

export function PublicFooter({ children }: { children?: ReactNode }) {
  const { t, i18n } = useTranslation()
  const { data } = usePublicConfig()
  const linkClass = buttonVariants({ variant: 'link' })

  return (
    <footer className="border-t-[0.5px] border-border">
      <div className="mx-auto flex max-w-[1200px] flex-col gap-4 px-4 py-6 text-label text-text-2 sm:px-6">
        <div className="flex min-w-0 items-center gap-2.5"><SiteBrand compact /></div>
        {!!data?.footer?.length && (
          <ul className="m-0 grid list-none gap-x-8 gap-y-2 p-0 sm:grid-cols-2 lg:grid-cols-3">
            {data.footer.map((item) => {
              const label = i18n.resolvedLanguage === 'en' ? item.label.en.trim() || item.label.zh_CN : item.label.zh_CN
              const url = footerURL(item.url)
              const external = url && url.protocol !== 'mailto:'
              return (
                <li key={item.id} className="min-w-0 [overflow-wrap:anywhere]">
                  {url ? (
                    <a
                      className={cn(linkClass, 'whitespace-normal')}
                      href={url.href}
                      target={external ? '_blank' : undefined}
                      rel={external ? 'noopener noreferrer' : undefined}
                    >
                      {label}
                    </a>
                  ) : <span className="text-text">{label}</span>}
                  {item.text.trim() && <p className="m-0 whitespace-pre-wrap text-caption text-text-3">{item.text}</p>}
                </li>
              )
            })}
          </ul>
        )}
        <div className="flex flex-wrap items-center gap-x-6">
          <Link className={linkClass} to="/legal/$kind" params={{ kind: 'terms' }}>{t('polish.termsOfService')}</Link>
          <Link className={linkClass} to="/legal/$kind" params={{ kind: 'privacy' }}>{t('polish.privacyPolicy')}</Link>
          {children}
        </div>
      </div>
    </footer>
  )
}
